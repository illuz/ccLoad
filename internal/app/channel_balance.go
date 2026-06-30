package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"ccLoad/internal/model"

	"github.com/dop251/goja"
	"github.com/gin-gonic/gin"
)

const (
	defaultChannelBalanceRefreshIntervalSeconds = 300
	defaultChannelBalanceRequestTimeout         = 15 * time.Second
	defaultChannelBalanceScriptTimeout          = 500 * time.Millisecond
	maxChannelBalanceResponseBytes              = 1 << 20
	maxChannelBalanceScriptBytes                = 64 * 1024
)

// ChannelUpstreamBalance 表示渠道上游余额/额度查询结果快照。
type ChannelUpstreamBalance struct {
	Status         string     `json:"status,omitempty"`
	IsValid        *bool      `json:"is_valid,omitempty"`
	InvalidMessage string     `json:"invalid_message,omitempty"`
	Remaining      any        `json:"remaining,omitempty"`
	Unit           string     `json:"unit,omitempty"`
	PlanName       string     `json:"plan_name,omitempty"`
	Total          any        `json:"total,omitempty"`
	Used           any        `json:"used,omitempty"`
	Extra          string     `json:"extra,omitempty"`
	Error          string     `json:"error,omitempty"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
}

type channelBalanceCacheEntry struct {
	Snapshot      *ChannelUpstreamBalance
	LastAttemptAt time.Time
	Refreshing    bool
}

type channelBalanceRequestConfig struct {
	URL       string         `json:"url"`
	Method    string         `json:"method,omitempty"`
	Headers   map[string]any `json:"headers,omitempty"`
	Body      any            `json:"body,omitempty"`
	TimeoutMS int            `json:"timeout_ms,omitempty"`
}

type parsedChannelBalanceScript struct {
	vm        *goja.Runtime
	request   channelBalanceRequestConfig
	extractor goja.Callable
}

func normalizeChannelBalanceRefreshIntervalSeconds(seconds int) int {
	if seconds < 0 {
		return 0
	}
	return seconds
}

func trimChannelBalanceScript(script string) string {
	return strings.TrimSpace(script)
}

func validateChannelBalanceQueryScript(script string) error {
	script = trimChannelBalanceScript(script)
	if script == "" {
		return nil
	}
	if len(script) > maxChannelBalanceScriptBytes {
		return fmt.Errorf("balance_query_script too large (max %d bytes)", maxChannelBalanceScriptBytes)
	}
	parsed, err := parseChannelBalanceScript(script)
	if err != nil {
		return err
	}
	_ = parsed
	return nil
}

func parseChannelBalanceScript(script string) (*parsedChannelBalanceScript, error) {
	script = trimChannelBalanceScript(script)
	if script == "" {
		return nil, errors.New("balance_query_script cannot be empty")
	}
	if len(script) > maxChannelBalanceScriptBytes {
		return nil, fmt.Errorf("balance_query_script too large (max %d bytes)", maxChannelBalanceScriptBytes)
	}

	vm := goja.New()
	value, err := runGojaWithTimeout(vm, defaultChannelBalanceScriptTimeout, func() (goja.Value, error) {
		return vm.RunString(script)
	})
	if err != nil {
		return nil, fmt.Errorf("invalid balance_query_script: %w", err)
	}

	obj := value.ToObject(vm)
	if obj == nil {
		return nil, errors.New("invalid balance_query_script: script must evaluate to an object literal")
	}

	requestValue := obj.Get("request")
	if goja.IsUndefined(requestValue) || goja.IsNull(requestValue) {
		return nil, errors.New("invalid balance_query_script: request is required")
	}
	request, err := exportChannelBalanceRequestConfig(requestValue)
	if err != nil {
		return nil, fmt.Errorf("invalid balance_query_script request: %w", err)
	}
	if err := normalizeChannelBalanceRequestConfig(&request); err != nil {
		return nil, err
	}

	extractorValue := obj.Get("extractor")
	extractor, ok := goja.AssertFunction(extractorValue)
	if !ok {
		return nil, errors.New("invalid balance_query_script: extractor must be a function")
	}

	return &parsedChannelBalanceScript{vm: vm, request: request, extractor: extractor}, nil
}

func exportChannelBalanceRequestConfig(value goja.Value) (channelBalanceRequestConfig, error) {
	exported := value.Export()
	requestMap, ok := exported.(map[string]any)
	if !ok {
		return channelBalanceRequestConfig{}, errors.New("request must be an object")
	}

	cfg := channelBalanceRequestConfig{}
	if raw, ok := requestMap["url"]; ok {
		cfg.URL = fmt.Sprint(raw)
	}
	if raw, ok := requestMap["method"]; ok {
		cfg.Method = fmt.Sprint(raw)
	}
	if raw, ok := requestMap["headers"]; ok {
		if headers, ok := raw.(map[string]any); ok {
			cfg.Headers = headers
		} else {
			return channelBalanceRequestConfig{}, errors.New("request.headers must be an object")
		}
	}
	if raw, ok := requestMap["body"]; ok {
		cfg.Body = raw
	}
	if raw, ok := requestMap["timeout_ms"]; ok {
		timeoutMS, err := channelBalanceInt(raw)
		if err != nil {
			return channelBalanceRequestConfig{}, errors.New("request.timeout_ms must be an integer")
		}
		cfg.TimeoutMS = timeoutMS
	}
	return cfg, nil
}

func normalizeChannelBalanceRequestConfig(cfg *channelBalanceRequestConfig) error {
	if cfg == nil {
		return errors.New("invalid balance_query_script: request is required")
	}
	cfg.URL = strings.TrimSpace(cfg.URL)
	if cfg.URL == "" {
		return errors.New("invalid balance_query_script: request.url is required")
	}
	method := strings.ToUpper(strings.TrimSpace(cfg.Method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead:
	default:
		return fmt.Errorf("invalid balance_query_script: unsupported request.method %q", cfg.Method)
	}
	cfg.Method = method
	if cfg.TimeoutMS < 0 {
		return errors.New("invalid balance_query_script: request.timeout_ms must be >= 0")
	}
	return nil
}

func runGojaWithTimeout(vm *goja.Runtime, timeout time.Duration, fn func() (goja.Value, error)) (goja.Value, error) {
	if vm == nil {
		return nil, errors.New("javascript runtime is nil")
	}
	var timer *time.Timer
	if timeout > 0 {
		timer = time.AfterFunc(timeout, func() {
			vm.Interrupt(errChannelBalanceScriptTimeout)
		})
	}
	defer func() {
		if timer != nil {
			timer.Stop()
		}
		vm.ClearInterrupt()
	}()

	value, err := fn()
	if err != nil {
		var interrupted *goja.InterruptedError
		if errors.As(err, &interrupted) {
			if interrupted.Value() == errChannelBalanceScriptTimeout {
				return nil, errChannelBalanceScriptTimeout
			}
			return nil, fmt.Errorf("script interrupted: %v", interrupted.Value())
		}
		return nil, err
	}
	return value, nil
}

var errChannelBalanceScriptTimeout = errors.New("balance script execution timed out")

func replaceChannelBalanceTemplate(text string, vars map[string]string) string {
	if text == "" || len(vars) == 0 {
		return text
	}
	replacerArgs := make([]string, 0, len(vars)*2)
	for key, value := range vars {
		replacerArgs = append(replacerArgs, "{{"+key+"}}", value)
	}
	return strings.NewReplacer(replacerArgs...).Replace(text)
}

func applyChannelBalanceTemplates(value any, vars map[string]string) any {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return replaceChannelBalanceTemplate(v, vars)
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = applyChannelBalanceTemplates(v[i], vars)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = applyChannelBalanceTemplates(item, vars)
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = replaceChannelBalanceTemplate(item, vars)
		}
		return out
	default:
		return value
	}
}

func buildChannelBalanceRequest(script *parsedChannelBalanceScript, baseURL string, vars map[string]string) (*http.Request, time.Duration, string, error) {
	if script == nil {
		return nil, 0, "", errors.New("balance script is nil")
	}

	resolvedURL := replaceChannelBalanceTemplate(script.request.URL, vars)
	resolvedURL = strings.TrimSpace(resolvedURL)
	if resolvedURL == "" {
		return nil, 0, "", errors.New("balance request url is empty")
	}
	fullURL, err := resolveChannelBalanceURL(baseURL, resolvedURL)
	if err != nil {
		return nil, 0, "", err
	}

	var bodyReader io.Reader
	appliedBody := applyChannelBalanceTemplates(script.request.Body, vars)
	bodyBytes, contentType, err := marshalChannelBalanceRequestBody(appliedBody)
	if err != nil {
		return nil, 0, "", err
	}
	if len(bodyBytes) > 0 {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(script.request.Method, fullURL, bodyReader)
	if err != nil {
		return nil, 0, "", fmt.Errorf("build balance request: %w", err)
	}

	for key, raw := range script.request.Headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		applied := applyChannelBalanceTemplates(raw, vars)
		req.Header.Set(key, fmt.Sprint(applied))
	}
	if contentType != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", contentType)
	}

	timeout := defaultChannelBalanceRequestTimeout
	if script.request.TimeoutMS > 0 {
		timeout = time.Duration(script.request.TimeoutMS) * time.Millisecond
	}
	return req, timeout, fullURL, nil
}

func resolveChannelBalanceURL(baseURL, rawURL string) (string, error) {
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid balance request url %q: %w", rawURL, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		if strings.TrimSpace(baseURL) == "" {
			return "", fmt.Errorf("invalid balance request url %q: absolute URL required", rawURL)
		}
		base, err := neturl.Parse(strings.TrimSpace(baseURL))
		if err != nil {
			return "", fmt.Errorf("invalid base url %q: %w", baseURL, err)
		}
		parsed = base.ResolveReference(parsed)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid balance request url scheme %q", parsed.Scheme)
	}
	return parsed.String(), nil
}

func marshalChannelBalanceRequestBody(body any) ([]byte, string, error) {
	switch value := body.(type) {
	case nil:
		return nil, "", nil
	case string:
		return []byte(value), "", nil
	default:
		payload, err := json.Marshal(value)
		if err != nil {
			return nil, "", fmt.Errorf("marshal balance request body: %w", err)
		}
		return payload, "application/json", nil
	}
}

func maybeDecodeChannelBalanceResponse(body []byte, contentType string) any {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil
	}
	if strings.Contains(strings.ToLower(contentType), "json") || looksLikeJSONValue(trimmed) {
		var payload any
		if err := json.Unmarshal(trimmed, &payload); err == nil {
			return payload
		}
	}
	return string(body)
}

func looksLikeJSONValue(trimmed []byte) bool {
	if len(trimmed) == 0 {
		return false
	}
	switch trimmed[0] {
	case '{', '[', '"':
		return true
	case 't', 'f', 'n':
		return true
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return true
	default:
		return false
	}
}

func executeChannelBalanceExtractor(script *parsedChannelBalanceScript, payload any, meta map[string]any) (*ChannelUpstreamBalance, error) {
	if script == nil || script.vm == nil || script.extractor == nil {
		return nil, errors.New("balance extractor is unavailable")
	}
	resultValue, err := runGojaWithTimeout(script.vm, defaultChannelBalanceScriptTimeout, func() (goja.Value, error) {
		return script.extractor(goja.Undefined(), script.vm.ToValue(payload), script.vm.ToValue(meta))
	})
	if err != nil {
		return nil, fmt.Errorf("run balance extractor: %w", err)
	}
	if goja.IsUndefined(resultValue) || goja.IsNull(resultValue) {
		return &ChannelUpstreamBalance{Status: "ready"}, nil
	}

	exported := resultValue.Export()
	resultMap, ok := exported.(map[string]any)
	if !ok {
		return nil, errors.New("balance extractor must return an object")
	}

	snapshot := &ChannelUpstreamBalance{Status: "ready"}
	if value, ok := lookupChannelBalanceValue(resultMap, "isValid", "is_valid"); ok {
		if parsed, ok := channelBalanceBool(value); ok {
			snapshot.IsValid = &parsed
		}
	}
	if value, ok := lookupChannelBalanceValue(resultMap, "invalidMessage", "invalid_message"); ok {
		snapshot.InvalidMessage = fmt.Sprint(value)
	}
	if value, ok := lookupChannelBalanceValue(resultMap, "remaining"); ok {
		snapshot.Remaining = normalizeChannelBalanceValue(value)
	}
	if value, ok := lookupChannelBalanceValue(resultMap, "unit"); ok {
		snapshot.Unit = fmt.Sprint(value)
	}
	if value, ok := lookupChannelBalanceValue(resultMap, "planName", "plan_name"); ok {
		snapshot.PlanName = fmt.Sprint(value)
	}
	if value, ok := lookupChannelBalanceValue(resultMap, "total"); ok {
		snapshot.Total = normalizeChannelBalanceValue(value)
	}
	if value, ok := lookupChannelBalanceValue(resultMap, "used"); ok {
		snapshot.Used = normalizeChannelBalanceValue(value)
	}
	if value, ok := lookupChannelBalanceValue(resultMap, "extra"); ok {
		snapshot.Extra = fmt.Sprint(value)
	}
	return snapshot, nil
}

func lookupChannelBalanceValue(values map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func channelBalanceBool(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "ok":
			return true, true
		case "false", "0", "no":
			return false, true
		}
	}
	return false, false
}

func channelBalanceInt(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int8:
		return int(v), nil
	case int16:
		return int(v), nil
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case uint:
		return int(v), nil
	case uint8:
		return int(v), nil
	case uint16:
		return int(v), nil
	case uint32:
		return int(v), nil
	case uint64:
		return int(v), nil
	case float64:
		return int(v), nil
	case float32:
		return int(v), nil
	default:
		return 0, fmt.Errorf("unsupported integer value %T", value)
	}
}

func normalizeChannelBalanceValue(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return v
	case bool:
		return v
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func cloneChannelUpstreamBalance(snapshot *ChannelUpstreamBalance) *ChannelUpstreamBalance {
	if snapshot == nil {
		return nil
	}
	clone := *snapshot
	if snapshot.UpdatedAt != nil {
		updatedAt := *snapshot.UpdatedAt
		clone.UpdatedAt = &updatedAt
	}
	return &clone
}

func newChannelBalanceErrorSnapshot(err error) *ChannelUpstreamBalance {
	message := "unknown balance query error"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	now := time.Now()
	return &ChannelUpstreamBalance{
		Status:    "error",
		Error:     message,
		UpdatedAt: &now,
	}
}

func primaryChannelBalanceBaseURL(cfg *model.Config) (string, error) {
	if cfg == nil {
		return "", errors.New("channel config is nil")
	}
	for _, raw := range cfg.GetURLs() {
		resolved := strings.TrimSpace(model.StripExactUpstreamURLMarker(raw))
		if resolved != "" {
			return resolved, nil
		}
	}
	return "", errors.New("channel has no usable base URL")
}

func selectChannelBalanceKey(apiKeys []*model.APIKey) (int, string, error) {
	for _, apiKey := range apiKeys {
		if apiKey == nil || apiKey.Disabled || strings.TrimSpace(apiKey.APIKey) == "" {
			continue
		}
		return apiKey.KeyIndex, apiKey.APIKey, nil
	}
	for _, apiKey := range apiKeys {
		if apiKey == nil || strings.TrimSpace(apiKey.APIKey) == "" {
			continue
		}
		return apiKey.KeyIndex, apiKey.APIKey, nil
	}
	return 0, "", errors.New("channel has no usable API key")
}

func (s *Server) getChannelBalanceRefreshIntervalSeconds() int {
	if s == nil || s.configService == nil {
		return defaultChannelBalanceRefreshIntervalSeconds
	}
	return normalizeChannelBalanceRefreshIntervalSeconds(
		s.configService.GetInt("channel_balance_refresh_interval_seconds", defaultChannelBalanceRefreshIntervalSeconds),
	)
}

func (s *Server) attachChannelBalanceInfo(target *ChannelWithCooldown, cfg *model.Config) {
	if s == nil || target == nil || cfg == nil {
		return
	}
	if trimChannelBalanceScript(cfg.BalanceQueryScript) == "" {
		target.UpstreamBalance = nil
		s.clearChannelBalanceCache(cfg.ID)
		return
	}
	s.maybeTriggerChannelBalanceRefresh(cfg)
	if snapshot := s.getChannelBalanceCacheSnapshot(cfg.ID); snapshot != nil {
		target.UpstreamBalance = snapshot
		return
	}
	if s.getChannelBalanceRefreshIntervalSeconds() <= 0 {
		target.UpstreamBalance = &ChannelUpstreamBalance{Status: "disabled"}
		return
	}
	target.UpstreamBalance = &ChannelUpstreamBalance{Status: "pending"}
}

func (s *Server) getChannelBalanceCacheSnapshot(channelID int64) *ChannelUpstreamBalance {
	if s == nil {
		return nil
	}
	s.channelBalanceCacheMu.RLock()
	defer s.channelBalanceCacheMu.RUnlock()
	entry := s.channelBalanceCache[channelID]
	if entry == nil || entry.Snapshot == nil {
		return nil
	}
	return cloneChannelUpstreamBalance(entry.Snapshot)
}

func (s *Server) clearChannelBalanceCache(channelID int64) {
	if s == nil {
		return
	}
	s.channelBalanceCacheMu.Lock()
	defer s.channelBalanceCacheMu.Unlock()
	delete(s.channelBalanceCache, channelID)
}

func (s *Server) ensureChannelBalanceRefresh(channelID int64, setPending bool, force bool, minInterval time.Duration) bool {
	if s == nil {
		return false
	}
	now := time.Now()
	s.channelBalanceCacheMu.Lock()
	defer s.channelBalanceCacheMu.Unlock()
	if s.channelBalanceCache == nil {
		s.channelBalanceCache = make(map[int64]*channelBalanceCacheEntry)
	}

	entry := s.channelBalanceCache[channelID]
	if entry == nil {
		entry = &channelBalanceCacheEntry{}
		s.channelBalanceCache[channelID] = entry
	}
	if entry.Refreshing {
		if setPending && entry.Snapshot == nil {
			entry.Snapshot = &ChannelUpstreamBalance{Status: "pending"}
		}
		return false
	}
	if !force && !entry.LastAttemptAt.IsZero() && minInterval > 0 && now.Sub(entry.LastAttemptAt) < minInterval {
		return false
	}
	entry.Refreshing = true
	entry.LastAttemptAt = now
	if setPending && entry.Snapshot == nil {
		entry.Snapshot = &ChannelUpstreamBalance{Status: "pending"}
	}
	return true
}

func (s *Server) finishChannelBalanceRefresh(channelID int64, snapshot *ChannelUpstreamBalance) {
	if s == nil {
		return
	}
	if snapshot == nil {
		snapshot = newChannelBalanceErrorSnapshot(errors.New("empty balance snapshot"))
	}
	s.channelBalanceCacheMu.Lock()
	defer s.channelBalanceCacheMu.Unlock()
	if s.channelBalanceCache == nil {
		s.channelBalanceCache = make(map[int64]*channelBalanceCacheEntry)
	}
	entry := s.channelBalanceCache[channelID]
	if entry == nil {
		entry = &channelBalanceCacheEntry{}
		s.channelBalanceCache[channelID] = entry
	}
	entry.Refreshing = false
	entry.Snapshot = cloneChannelUpstreamBalance(snapshot)
}

func (s *Server) maybeTriggerChannelBalanceRefresh(cfg *model.Config) {
	if s == nil || cfg == nil || trimChannelBalanceScript(cfg.BalanceQueryScript) == "" {
		return
	}
	intervalSeconds := s.getChannelBalanceRefreshIntervalSeconds()
	if intervalSeconds <= 0 {
		return
	}
	if !s.ensureChannelBalanceRefresh(cfg.ID, true, false, time.Duration(intervalSeconds)*time.Second) {
		return
	}
	cfgCopy := cfg.Clone()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ctx := context.Background()
		if s.baseCtx != nil {
			ctx = s.baseCtx
		}
		snapshot := s.refreshChannelBalanceBlocking(ctx, cfgCopy)
		s.finishChannelBalanceRefresh(cfgCopy.ID, snapshot)
	}()
}

func (s *Server) refreshChannelBalanceBlocking(ctx context.Context, cfg *model.Config) *ChannelUpstreamBalance {
	if s == nil || cfg == nil {
		return newChannelBalanceErrorSnapshot(errors.New("channel config is nil"))
	}
	if trimChannelBalanceScript(cfg.BalanceQueryScript) == "" {
		return nil
	}

	apiKeys, err := s.store.GetAPIKeys(ctx, cfg.ID)
	if err != nil {
		return newChannelBalanceErrorSnapshot(fmt.Errorf("load channel API keys: %w", err))
	}
	keyIndex, apiKey, err := selectChannelBalanceKey(apiKeys)
	if err != nil {
		return newChannelBalanceErrorSnapshot(err)
	}
	baseURL, err := primaryChannelBalanceBaseURL(cfg)
	if err != nil {
		return newChannelBalanceErrorSnapshot(err)
	}
	parsedScript, err := parseChannelBalanceScript(cfg.BalanceQueryScript)
	if err != nil {
		return newChannelBalanceErrorSnapshot(err)
	}

	vars := map[string]string{
		"apiKey":      apiKey,
		"baseUrl":     baseURL,
		"channelId":   fmt.Sprintf("%d", cfg.ID),
		"channelName": cfg.Name,
		"keyIndex":    fmt.Sprintf("%d", keyIndex),
	}
	request, timeout, requestURL, err := buildChannelBalanceRequest(parsedScript, baseURL, vars)
	if err != nil {
		return newChannelBalanceErrorSnapshot(err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request = request.WithContext(requestCtx)

	client := s.getClientForChannel(cfg)
	response, err := client.Do(request)
	if err != nil {
		return newChannelBalanceErrorSnapshot(fmt.Errorf("balance request failed: %w", err))
	}
	defer func() { _ = response.Body.Close() }()

	limitedReader := io.LimitReader(response.Body, maxChannelBalanceResponseBytes+1)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return newChannelBalanceErrorSnapshot(fmt.Errorf("read balance response: %w", err))
	}
	if len(bodyBytes) > maxChannelBalanceResponseBytes {
		return newChannelBalanceErrorSnapshot(fmt.Errorf("balance response too large (max %d bytes)", maxChannelBalanceResponseBytes))
	}

	payload := maybeDecodeChannelBalanceResponse(bodyBytes, response.Header.Get("Content-Type"))
	meta := map[string]any{
		"status":  response.StatusCode,
		"ok":      response.StatusCode >= 200 && response.StatusCode < 300,
		"headers": flattenHeader(response.Header),
		"url":     requestURL,
		"rawBody": string(bodyBytes),
	}
	snapshot, err := executeChannelBalanceExtractor(parsedScript, payload, meta)
	if err != nil {
		return newChannelBalanceErrorSnapshot(err)
	}
	now := time.Now()
	snapshot.UpdatedAt = &now
	if snapshot.Status == "" {
		snapshot.Status = "ready"
	}
	return snapshot
}

func (s *Server) forceRefreshChannelBalance(ctx context.Context, cfg *model.Config) *ChannelUpstreamBalance {
	if s == nil || cfg == nil {
		return newChannelBalanceErrorSnapshot(errors.New("channel config is nil"))
	}
	if trimChannelBalanceScript(cfg.BalanceQueryScript) == "" {
		s.clearChannelBalanceCache(cfg.ID)
		return &ChannelUpstreamBalance{Status: "disabled"}
	}
	if !s.ensureChannelBalanceRefresh(cfg.ID, true, true, 0) {
		if snapshot := s.getChannelBalanceCacheSnapshot(cfg.ID); snapshot != nil {
			return snapshot
		}
		return &ChannelUpstreamBalance{Status: "pending"}
	}
	snapshot := s.refreshChannelBalanceBlocking(ctx, cfg.Clone())
	s.finishChannelBalanceRefresh(cfg.ID, snapshot)
	return cloneChannelUpstreamBalance(snapshot)
}

func (s *Server) startChannelBalanceRefreshLoop(interval time.Duration) {
	if s == nil || interval <= 0 {
		return
	}
	log.Printf("[INFO] 渠道余额定时刷新已启用：间隔=%s（启动后立即执行首轮）", interval)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.triggerChannelBalanceRefreshLoop()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.shutdownCh:
				log.Print("[INFO] 渠道余额定时刷新已停止")
				return
			case <-ticker.C:
				s.triggerChannelBalanceRefreshLoop()
			}
		}
	}()
}

func (s *Server) triggerChannelBalanceRefreshLoop() bool {
	if s == nil {
		return false
	}
	if !s.channelBalanceRefreshRunning.CompareAndSwap(false, true) {
		log.Print("[WARN] 跳过本轮渠道余额刷新：上一轮仍在执行")
		return false
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.channelBalanceRefreshRunning.Store(false)

		ctx := context.Background()
		if s.baseCtx != nil {
			ctx = s.baseCtx
		}
		if err := s.refreshConfiguredChannelBalances(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[WARN] 渠道余额刷新失败: %v", err)
		}
	}()
	return true
}

func (s *Server) refreshConfiguredChannelBalances(ctx context.Context) error {
	if s == nil || s.store == nil {
		return nil
	}
	configs, err := s.store.ListConfigs(ctx)
	if err != nil {
		return err
	}
	for _, cfg := range configs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if cfg == nil || trimChannelBalanceScript(cfg.BalanceQueryScript) == "" {
			if cfg != nil {
				s.clearChannelBalanceCache(cfg.ID)
			}
			continue
		}
		if !s.ensureChannelBalanceRefresh(cfg.ID, true, true, 0) {
			continue
		}
		snapshot := s.refreshChannelBalanceBlocking(ctx, cfg.Clone())
		s.finishChannelBalanceRefresh(cfg.ID, snapshot)
	}
	return nil
}

// HandleRefreshChannelBalance 手动刷新单个渠道的上游余额快照。
func (s *Server) HandleRefreshChannelBalance(c *gin.Context) {
	id, err := ParseInt64Param(c, "id")
	if err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid channel id")
		return
	}
	cfg, err := s.store.GetConfig(c.Request.Context(), id)
	if err != nil {
		RespondError(c, http.StatusNotFound, fmt.Errorf("channel not found"))
		return
	}
	snapshot := s.forceRefreshChannelBalance(c.Request.Context(), cfg)
	RespondJSON(c, http.StatusOK, snapshot)
}
