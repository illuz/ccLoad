package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ccLoad/internal/model"
	"ccLoad/internal/storage"
	"ccLoad/internal/util"
)

const (
	defaultTelegramAPIBase = "https://api.telegram.org"

	telegramAlertQueueSize   = 32
	telegramAlertSendTimeout = 5 * time.Second

	defaultTokenUsageAlertThreshold = 0.99
	defaultRecentFailureWindow      = 20
	defaultRecentFailureThreshold   = 0.50
)

type alertNotifier interface {
	Send(ctx context.Context, text string) error
}

type telegramAlertConfig struct {
	BotToken string
	ChatID   string
	APIBase  string
	Disabled bool
}

type telegramNotifier struct {
	botToken string
	chatID   string
	apiBase  string
	client   *http.Client
}

type recentLogOutcome struct {
	failure bool
}

// AlertService observes request logs and token usage, then sends Telegram alerts.
// It is intentionally in-memory: alerts are best-effort operational signals, not
// part of the billing/logging persistence path.
type AlertService struct {
	store       storage.Store
	authService *AuthService
	notifier    alertNotifier

	enabled        bool
	sendCh         chan string
	shutdownCh     <-chan struct{}
	isShuttingDown *atomic.Bool
	wg             *sync.WaitGroup

	usageThreshold   float64
	failureWindow    int
	failureThreshold float64
	now              func() time.Time

	mu                       sync.Mutex
	tokenUsageAlerts         map[string]bool
	recentLogOutcomes        []recentLogOutcome
	recentFailureAlertActive bool
}

// NewAlertServiceFromEnv creates and starts Telegram alerting when the required
// environment variables are present. Missing config leaves alerting disabled.
func NewAlertServiceFromEnv(store storage.Store, authService *AuthService, shutdownCh <-chan struct{}, isShuttingDown *atomic.Bool, wg *sync.WaitGroup) *AlertService {
	cfg := loadTelegramAlertConfigFromEnv()
	if cfg.Disabled {
		return nil
	}
	if cfg.BotToken == "" && cfg.ChatID == "" {
		return nil
	}
	if cfg.BotToken == "" || cfg.ChatID == "" {
		log.Print("[WARN] Telegram告警未启用：需要同时设置 CCLOAD_TELEGRAM_BOT_TOKEN 和 CCLOAD_TELEGRAM_CHAT_ID")
		return nil
	}

	notifier := &telegramNotifier{
		botToken: cfg.BotToken,
		chatID:   cfg.ChatID,
		apiBase:  cfg.APIBase,
		client: &http.Client{
			Timeout: telegramAlertSendTimeout,
		},
	}

	svc := newAlertService(store, authService, notifier, shutdownCh, isShuttingDown, wg)
	svc.Start()
	log.Printf("[INFO] Telegram告警已启用（令牌用量阈值 %.0f%%；近 %d 次日志失败率阈值 > %.0f%%）",
		svc.usageThreshold*100, svc.failureWindow, svc.failureThreshold*100)
	return svc
}

func newAlertService(store storage.Store, authService *AuthService, notifier alertNotifier, shutdownCh <-chan struct{}, isShuttingDown *atomic.Bool, wg *sync.WaitGroup) *AlertService {
	return &AlertService{
		store:             store,
		authService:       authService,
		notifier:          notifier,
		enabled:           notifier != nil,
		sendCh:            make(chan string, telegramAlertQueueSize),
		shutdownCh:        shutdownCh,
		isShuttingDown:    isShuttingDown,
		wg:                wg,
		usageThreshold:    defaultTokenUsageAlertThreshold,
		failureWindow:     defaultRecentFailureWindow,
		failureThreshold:  defaultRecentFailureThreshold,
		now:               time.Now,
		tokenUsageAlerts:  make(map[string]bool),
		recentLogOutcomes: make([]recentLogOutcome, 0, defaultRecentFailureWindow),
	}
}

func loadTelegramAlertConfigFromEnv() telegramAlertConfig {
	if isFalseEnv(os.Getenv("CCLOAD_TELEGRAM_ALERT_ENABLED")) {
		return telegramAlertConfig{Disabled: true}
	}

	apiBase := strings.TrimSpace(os.Getenv("CCLOAD_TELEGRAM_API_BASE"))
	if apiBase == "" {
		apiBase = defaultTelegramAPIBase
	}

	return telegramAlertConfig{
		BotToken: firstNonEmptyEnv("CCLOAD_TELEGRAM_BOT_TOKEN", "TELEGRAM_BOT_TOKEN"),
		ChatID:   firstNonEmptyEnv("CCLOAD_TELEGRAM_CHAT_ID", "TELEGRAM_CHAT_ID"),
		APIBase:  strings.TrimRight(apiBase, "/"),
	}
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func isFalseEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "false", "no", "off", "disabled":
		return true
	default:
		return false
	}
}

// Start launches the Telegram send worker.
func (s *AlertService) Start() {
	if s == nil || !s.enabled || s.notifier == nil || s.sendCh == nil || s.shutdownCh == nil || s.wg == nil {
		return
	}

	s.wg.Add(1)
	go s.worker()
}

func (s *AlertService) worker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.shutdownCh:
			return
		case text := <-s.sendCh:
			s.send(text)
		}
	}
}

func (s *AlertService) send(text string) {
	ctx, cancel := context.WithTimeout(context.Background(), telegramAlertSendTimeout)
	defer cancel()

	if err := s.notifier.Send(ctx, text); err != nil {
		log.Printf("[WARN] Telegram告警发送失败: %v", err)
	}
}

func (s *AlertService) enqueue(text string) {
	if s == nil || !s.enabled || strings.TrimSpace(text) == "" || s.sendCh == nil {
		return
	}
	if s.isShuttingDown != nil && s.isShuttingDown.Load() {
		return
	}

	select {
	case s.sendCh <- text:
	default:
		log.Printf("[ERROR] Telegram告警队列已满，丢弃告警: %s", firstAlertLine(text))
	}
}

// ObserveLog updates the recent-log failure window and alerts once when the
// latest 20 non-499 proxy logs have a failure rate greater than 50%.
func (s *AlertService) ObserveLog(entry *model.LogEntry) {
	if s == nil || !s.enabled || entry == nil {
		return
	}
	if entry.LogSource != "" && entry.LogSource != model.LogSourceProxy {
		return
	}
	// Keep alert semantics aligned with existing stats: client-cancelled 499 logs
	// are excluded from failure-rate denominators.
	if entry.StatusCode == util.StatusClientClosedRequest {
		return
	}

	failure := entry.StatusCode < 200 || entry.StatusCode >= 300
	var alertText string

	s.mu.Lock()
	s.recentLogOutcomes = append(s.recentLogOutcomes, recentLogOutcome{failure: failure})
	if len(s.recentLogOutcomes) > s.failureWindow {
		copy(s.recentLogOutcomes, s.recentLogOutcomes[len(s.recentLogOutcomes)-s.failureWindow:])
		s.recentLogOutcomes = s.recentLogOutcomes[:s.failureWindow]
	}

	if len(s.recentLogOutcomes) == s.failureWindow {
		failures := 0
		for _, item := range s.recentLogOutcomes {
			if item.failure {
				failures++
			}
		}
		rate := float64(failures) / float64(s.failureWindow)
		if rate > s.failureThreshold {
			if !s.recentFailureAlertActive {
				s.recentFailureAlertActive = true
				alertText = s.buildRecentFailureAlert(entry, failures, rate)
			}
		} else {
			s.recentFailureAlertActive = false
		}
	}
	s.mu.Unlock()

	if alertText != "" {
		s.enqueue(alertText)
	}
}

// CheckTokenUsage alerts once per token/limit when usage reaches 99%.
func (s *AlertService) CheckTokenUsage(tokenHash string) {
	if s == nil || !s.enabled || tokenHash == "" || s.authService == nil {
		return
	}

	if used, limit, _ := s.authService.IsDailyCostLimitExceeded(tokenHash); limit > 0 {
		s.maybeAlertTokenUsage(tokenHash, "daily", model.CurrentLocalDayKey(), used, limit)
	}
	if used, limit, _ := s.authService.IsMonthlyCostLimitExceeded(tokenHash); limit > 0 {
		s.maybeAlertTokenUsage(tokenHash, "monthly", model.CurrentLocalMonthKey(), used, limit)
	}
	if used, limit, _ := s.authService.IsCostLimitExceeded(tokenHash); limit > 0 {
		s.maybeAlertTokenUsage(tokenHash, "total", 0, used, limit)
	}
}

func (s *AlertService) maybeAlertTokenUsage(tokenHash, limitType string, dayKey int, usedMicroUSD, limitMicroUSD int64) {
	if limitMicroUSD <= 0 {
		return
	}
	ratio := float64(usedMicroUSD) / float64(limitMicroUSD)
	stateKey := tokenUsageAlertKey(tokenHash, limitType, dayKey)

	shouldAlert := false
	s.mu.Lock()
	if limitType == "daily" {
		s.cleanupOldDailyTokenAlertsLocked(tokenHash, stateKey)
	}
	if ratio >= s.usageThreshold {
		if !s.tokenUsageAlerts[stateKey] {
			s.tokenUsageAlerts[stateKey] = true
			shouldAlert = true
		}
	} else {
		delete(s.tokenUsageAlerts, stateKey)
	}
	s.mu.Unlock()

	if !shouldAlert {
		return
	}

	s.enqueue(s.buildTokenUsageAlert(tokenHash, limitType, usedMicroUSD, limitMicroUSD, ratio))
}

func (s *AlertService) cleanupOldDailyTokenAlertsLocked(tokenHash, keepKey string) {
	prefix := tokenHash + ":daily:"
	for key := range s.tokenUsageAlerts {
		if strings.HasPrefix(key, prefix) && key != keepKey {
			delete(s.tokenUsageAlerts, key)
		}
	}
}

func tokenUsageAlertKey(tokenHash, limitType string, dayKey int) string {
	if limitType == "daily" {
		return fmt.Sprintf("%s:daily:%d", tokenHash, dayKey)
	}
	return tokenHash + ":total"
}

func (s *AlertService) buildTokenUsageAlert(tokenHash, limitType string, usedMicroUSD, limitMicroUSD int64, ratio float64) string {
	limitName := "总额度"
	if limitType == "daily" {
		limitName = "每日额度"
	}

	return fmt.Sprintf("🚨 ccLoad 告警：令牌使用量达到 %.0f%%\n\n令牌：%s\n类型：%s\n用量：$%.4f / $%.4f (%.1f%%)\n阈值：>= %.0f%%\n时间：%s",
		s.usageThreshold*100,
		s.tokenLabel(tokenHash),
		limitName,
		util.MicroUSDToUSD(usedMicroUSD),
		util.MicroUSDToUSD(limitMicroUSD),
		ratio*100,
		s.usageThreshold*100,
		s.now().Format(time.RFC3339),
	)
}

func (s *AlertService) buildRecentFailureAlert(entry *model.LogEntry, failures int, rate float64) string {
	return fmt.Sprintf("🚨 ccLoad 告警：近 %d 次日志失败率过高\n\n失败率：%.1f%% (%d/%d)\n阈值：> %.0f%%\n最新状态：HTTP %d\n渠道：%s\n模型：%s\n消息：%s\n时间：%s",
		s.failureWindow,
		rate*100,
		failures,
		s.failureWindow,
		s.failureThreshold*100,
		entry.StatusCode,
		logEntryChannelLabel(entry),
		strings.TrimSpace(entry.Model),
		truncateAlertField(entry.Message),
		s.now().Format(time.RFC3339),
	)
}

func (s *AlertService) tokenLabel(tokenHash string) string {
	fallback := "hash " + maskTokenHash(tokenHash)
	if s.store == nil {
		return fallback
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	token, err := s.store.GetAuthTokenByValue(ctx, tokenHash)
	if err != nil || token == nil {
		return fallback
	}

	desc := strings.TrimSpace(token.Description)
	if desc == "" {
		desc = "未命名令牌"
	}
	return fmt.Sprintf("#%d %s (%s)", token.ID, desc, maskTokenHash(tokenHash))
}

func logEntryChannelLabel(entry *model.LogEntry) string {
	if entry == nil {
		return "-"
	}
	name := strings.TrimSpace(entry.ChannelName)
	if name != "" {
		return fmt.Sprintf("%s (#%d)", name, entry.ChannelID)
	}
	if entry.ChannelID > 0 {
		return fmt.Sprintf("#%d", entry.ChannelID)
	}
	return "-"
}

func maskTokenHash(tokenHash string) string {
	if len(tokenHash) <= 12 {
		return tokenHash
	}
	return tokenHash[:8] + "…" + tokenHash[len(tokenHash)-4:]
}

func truncateAlertField(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	const maxRunes = 240
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "…"
}

func firstAlertLine(text string) string {
	text = strings.TrimSpace(text)
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = text[:idx]
	}
	return truncateAlertField(text)
}

func (n *telegramNotifier) Send(ctx context.Context, text string) error {
	if n == nil || n.botToken == "" || n.chatID == "" {
		return fmt.Errorf("telegram notifier is not configured")
	}
	client := n.client
	if client == nil {
		client = http.DefaultClient
	}
	apiBase := strings.TrimRight(n.apiBase, "/")
	if apiBase == "" {
		apiBase = defaultTelegramAPIBase
	}

	payload := struct {
		ChatID                string `json:"chat_id"`
		Text                  string `json:"text"`
		DisableWebPagePreview bool   `json:"disable_web_page_preview"`
	}{
		ChatID:                n.chatID,
		Text:                  text,
		DisableWebPagePreview: true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telegram payload: %w", err)
	}

	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", apiBase, n.botToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send telegram request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("telegram sendMessage status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}
