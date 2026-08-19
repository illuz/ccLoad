package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"ccLoad/internal/config"
	"ccLoad/internal/cooldown"
	"ccLoad/internal/model"
	"ccLoad/internal/protocol"
	"ccLoad/internal/util"

	"github.com/bytedance/sonic"
	"github.com/gin-gonic/gin"
)

var errUnknownChannelType = errors.New("unknown channel type for path")
var errBodyTooLarge = errors.New("request body too large")

type requestBodyReadError struct {
	cause            error
	method           string
	path             string
	contentLength    int64
	bytesRead        int64
	transferEncoding []string
}

func (e *requestBodyReadError) Error() string {
	transferEncoding := "identity"
	if len(e.transferEncoding) > 0 {
		transferEncoding = strings.Join(e.transferEncoding, ",")
	}
	return fmt.Sprintf(
		"failed to read body: method=%s path=%s content_length=%d bytes_read=%d transfer_encoding=%s: %v",
		e.method,
		e.path,
		e.contentLength,
		e.bytesRead,
		transferEncoding,
		e.cause,
	)
}

func (e *requestBodyReadError) Unwrap() error {
	return e.cause
}

type countingReader struct {
	reader    io.Reader
	bytesRead int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.bytesRead += int64(n)
	return n, err
}

func incomingRequestErrorStatus(err error) int {
	if errors.Is(err, errBodyTooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return http.StatusRequestTimeout
	}
	return http.StatusBadRequest
}

// ErrAllKeysUnavailable 表示所有渠道密钥都不可用
var ErrAllKeysUnavailable = errors.New("all channel keys unavailable")

// ErrAllKeysExhausted 表示所有密钥都已耗尽
var ErrAllKeysExhausted = errors.New("all keys exhausted")

// ErrChannelRPMExceeded 表示渠道RPM限制已达到
var ErrChannelRPMExceeded = errors.New("channel rpm limit exceeded")

// ErrChannelConcurrencyExceeded 表示渠道并发限制已达到
var ErrChannelConcurrencyExceeded = errors.New("channel concurrency limit exceeded")

const codexPromoMessageHeader = "X-Codex-Promo-Message"

func summarizeNoAvailableUpstream(
	ctx context.Context,
	s *Server,
	originalModel string,
	clientProtocol protocol.Protocol,
	tokenHash string,
) (message string, apiKeyHint string) {
	message = "no available upstream (all cooled or none)"

	rawCands, rawErr := s.getEnabledChannelsByModelAndProtocol(ctx, originalModel, string(clientProtocol))
	if rawErr != nil {
		return message + "; diag=query_failed", ""
	}
	if len(rawCands) == 0 {
		return message + "; diag=no_matching_channel", "(no-matching-channel)"
	}

	if tokenHash != "" {
		filtered, restricted := s.authService.FilterAllowedChannels(tokenHash, rawCands)
		if restricted && len(filtered) == 0 {
			names := make([]string, 0, len(rawCands))
			for _, cfg := range rawCands {
				if cfg != nil {
					names = append(names, cfg.Name)
				}
			}
			return message + "; diag=token_filtered; matched=" + strings.Join(names, ","), "(token-filtered)"
		}
	}

	names := make([]string, 0, len(rawCands))
	disabledCount := 0
	coolingCount := 0
	totalKeyCount := 0
	diagAPIKey := ""
	for _, cfg := range rawCands {
		if cfg == nil {
			continue
		}
		names = append(names, cfg.Name)
		keys, keyErr := s.getAPIKeys(ctx, cfg.ID)
		if keyErr != nil {
			continue
		}
		for _, key := range keys {
			if key == nil {
				continue
			}
			totalKeyCount++
			if key.Disabled {
				disabledCount++
				if diagAPIKey == "" && key.APIKey != "" {
					diagAPIKey = key.APIKey
				}
				continue
			}
			if key.IsCoolingDown(time.Now()) {
				coolingCount++
				if diagAPIKey == "" && key.APIKey != "" {
					diagAPIKey = key.APIKey
				}
			}
		}
	}

	reason := "all_cooled_or_none"
	switch {
	case totalKeyCount > 0 && disabledCount == totalKeyCount:
		reason = "all_keys_disabled"
	case totalKeyCount > 0 && coolingCount == totalKeyCount:
		reason = "all_keys_cooled"
	case totalKeyCount == 0:
		reason = "no_keys_configured"
	}

	if diagAPIKey == "" {
		switch len(rawCands) {
		case 0:
			diagAPIKey = "(no-matching-channel)"
		case 1:
			diagAPIKey = "(no-usable-key)"
		default:
			diagAPIKey = "(multiple-candidate-keys)"
		}
	}

	return fmt.Sprintf("%s; diag=%s; matched=%s", message, reason, strings.Join(names, ",")), diagAPIKey
}

// ============================================================================
// 并发控制
// ============================================================================

// acquireConcurrencySlot 获取并发槽位，返回release函数和状态
// ok=false 表示客户端已取消请求
func (s *Server) acquireConcurrencySlot(c *gin.Context) (release func(), ok bool) {
	select {
	case s.concurrencySem <- struct{}{}:
		return func() { <-s.concurrencySem }, true
	case <-c.Request.Context().Done():
		ctxErr := c.Request.Context().Err()
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": "request timeout while waiting for slot"})
			return nil, false
		}
		c.JSON(StatusClientClosedRequest, gin.H{"error": "request cancelled while waiting for slot"})
		return nil, false
	}
}

// ============================================================================
// 请求解析
// ============================================================================

// parseIncomingRequest 返回 (originalModel, body, isStreaming, error)
func parseIncomingRequest(c *gin.Context) (string, []byte, bool, error) {
	requestPath := c.Request.URL.Path
	requestMethod := c.Request.Method

	// 读取请求体（带上限，防止大包打爆内存）
	// 默认 10MB，images 路径 20MB，可通过 CCLOAD_MAX_BODY_BYTES 覆盖
	maxBody := int64(config.DefaultMaxBodyBytes)
	if strings.HasPrefix(requestPath, "/v1/images/") {
		maxBody = int64(config.DefaultMaxImageBodyBytes)
	}
	if v := os.Getenv("CCLOAD_MAX_BODY_BYTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxBody = int64(n)
		}
	}
	counted := &countingReader{reader: c.Request.Body}
	limited := io.LimitReader(counted, maxBody+1)
	all, err := io.ReadAll(limited)
	_ = c.Request.Body.Close()
	if err != nil {
		return "", nil, false, &requestBodyReadError{
			cause:            err,
			method:           requestMethod,
			path:             requestPath,
			contentLength:    c.Request.ContentLength,
			bytesRead:        counted.bytesRead,
			transferEncoding: append([]string(nil), c.Request.TransferEncoding...),
		}
	}
	if int64(len(all)) > maxBody {
		return "", nil, false, errBodyTooLarge
	}

	var reqModel struct {
		Model string `json:"model"`
	}
	_ = sonic.Unmarshal(all, &reqModel)

	// multipart/form-data 支持：当 JSON 解析无 model 时，尝试从 multipart 表单字段提取
	if reqModel.Model == "" {
		if ct := c.Request.Header.Get("Content-Type"); ct != "" {
			mediaType, params, _ := mime.ParseMediaType(ct)
			if mediaType == "multipart/form-data" {
				if boundary := params["boundary"]; boundary != "" {
					reqModel.Model = extractModelFromMultipart(all, boundary)
				}
			}
		}
	}

	// 智能检测流式请求
	isStreaming := isStreamingRequest(requestPath, all)

	// 多源模型名称获取：优先请求体，其次URL路径
	originalModel := reqModel.Model
	if originalModel == "" {
		originalModel = extractModelFromPath(requestPath)
	}

	// 对于GET请求，如果无法提取模型名称，使用通配符
	if originalModel == "" {
		if requestMethod == http.MethodGet {
			originalModel = "*"
		} else {
			return "", nil, false, fmt.Errorf("invalid JSON or missing model")
		}
	}

	return originalModel, all, isStreaming, nil
}

// extractModelFromMultipart 从 multipart/form-data 原始字节中提取 model 字段
func extractModelFromMultipart(body []byte, boundary string) string {
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		if part.FormName() == "model" {
			val, err := io.ReadAll(io.LimitReader(part, 256))
			_ = part.Close()
			if err == nil {
				return strings.TrimSpace(string(val))
			}
			break
		}
		_ = part.Close()
	}
	return ""
}

// ============================================================================
// 路由选择
// ============================================================================

// selectRouteCandidates 根据请求选择路由候选
// 从proxy.go提取，遵循SRP原则
func (s *Server) selectRouteCandidates(ctx context.Context, c *gin.Context, originalModel string, channelType string) ([]*model.Config, error) {
	requestMethod := c.Request.Method

	// 智能路由选择：根据请求类型选择不同的路由策略
	if requestMethod == http.MethodGet && channelType == util.ChannelTypeGemini {
		// 按渠道类型筛选Gemini渠道
		return s.selectCandidatesByChannelType(ctx, util.ChannelTypeGemini)
	}

	if channelType == "" {
		return nil, errUnknownChannelType
	}

	return s.selectCandidatesByModelAndType(ctx, originalModel, channelType)
}

// ============================================================================
// 主请求处理器
// ============================================================================

// handleSpecialRoutes 处理特殊路由（模型列表、token计数等）
// 返回 true 表示已处理，调用方应直接返回
func (s *Server) handleSpecialRoutes(c *gin.Context) bool {
	path := c.Request.URL.Path
	method := c.Request.Method

	switch {
	case method == http.MethodGet && path == "/v1/models":
		s.handleListOpenAIModels(c)
		return true
	case method == http.MethodGet && path == "/v1beta/models":
		s.handleListGeminiModels(c)
		return true
	case method == http.MethodPost && path == "/v1/messages/count_tokens":
		s.handleCountTokens(c)
		return true
	}
	return false
}

// HandleProxyRequest 通用透明代理处理器
func (s *Server) HandleProxyRequest(c *gin.Context) {
	handlerStart := time.Now()
	startTime := proxyTimingStartTime(c, handlerStart)
	timing := newProxyTimingTrace(startTime, handlerStart)
	requestID := ensureProxyRequestID(c)

	// 并发控制
	queueStart := time.Now()
	release, ok := s.acquireConcurrencySlot(c)
	if timing != nil {
		timing.MarkQueueWait(time.Since(queueStart))
	}
	if !ok {
		s.recordProxyRejection(c, startTime, "", c.Writer.Status(), "request cancelled while waiting for concurrency slot", false, "")
		return
	}
	defer release()

	// 特殊路由优先处理
	if s.handleSpecialRoutes(c) {
		return
	}

	requestMethod := c.Request.Method

	originalModel, all, isStreaming, err := parseIncomingRequest(c)
	if err != nil {
		statusCode := incomingRequestErrorStatus(err)
		c.JSON(statusCode, gin.H{"error": err.Error()})
		s.recordProxyRejection(c, startTime, "", statusCode, err.Error(), false, "")
		return
	}

	clientProtocol, effectiveRequestPath := clientRequestMetadata(c)
	if err := validateClientBodyMatchesProtocol(clientProtocol, all); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		s.recordProxyRejection(c, startTime, originalModel, http.StatusBadRequest, err.Error(), isStreaming, "")
		return
	}

	// 清理 Anthropic 请求中注入的 billing header 元数据
	if clientProtocol == protocol.Anthropic {
		all = stripAnthropicBillingHeaders(all)
	}
	thinkingEffort := extractThinkingEffortFromJSON(all)

	tokenHashStr := ""
	if v, ok := c.Get("token_hash"); ok {
		tokenHashStr, _ = v.(string)
	}
	tokenKeyStr := ""
	if v, ok := c.Get("token_key"); ok {
		tokenKeyStr, _ = v.(string)
	}

	// 从context提取tokenID（用于统计和日志，2025-12新增tokenID）
	tokenID, _ := c.Get("token_id")
	tokenIDInt64, _ := tokenID.(int64)

	if !s.enforceTokenLimits(c, clientProtocol, tokenHashStr, originalModel, startTime, isStreaming, thinkingEffort) {
		return
	}

	codexGuardEnabled := false
	if v, ok := c.Get("codex_guard_enabled"); ok {
		codexGuardEnabled, _ = v.(bool)
	}

	// 注册活跃请求（内存状态，用于前端实时显示）
	activeID := s.activeRequests.Register(startTime, originalModel, c.ClientIP(), isStreaming)
	s.activeRequests.SetThinkingEffort(activeID, thinkingEffort)
	defer s.activeRequests.Remove(activeID)

	timeout := parseTimeout(c.Request.URL.Query(), c.Request.Header)
	ctx := c.Request.Context()
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	ctx = contextWithEstimatedInputTokens(ctx, estimateRequestInputTokens(all))
	ctx = contextWithTokenHash(ctx, tokenHashStr)

	cands, err := s.selectRouteCandidates(ctx, c, originalModel, string(clientProtocol))
	if err != nil {
		if errors.Is(err, errUnknownChannelType) {
			c.JSON(http.StatusNotFound, gin.H{"error": "unsupported path"})
			s.recordProxyRejection(c, startTime, originalModel, http.StatusNotFound, "unsupported path", isStreaming, thinkingEffort)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		s.recordProxyRejection(c, startTime, originalModel, http.StatusInternalServerError, "route selection failed", isStreaming, thinkingEffort)
		return
	}

	if len(cands) == 0 {
		diagMessage, diagAPIKey := summarizeNoAvailableUpstream(ctx, s, originalModel, clientProtocol, tokenHashStr)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no available upstream (all cooled or none)"})
		duration := time.Since(startTime).Seconds()
		s.AddLogAsync(&model.LogEntry{
			Time:                  model.JSONTime{Time: startTime},
			RequestID:             requestID,
			Model:                 originalModel,
			LogSource:             model.LogSourceProxy,
			StatusCode:            503,
			Message:               diagMessage,
			Duration:              duration,
			EndToEndFirstByteTime: duration,
			IsStreaming:           isStreaming,
			APIKeyUsed:            diagAPIKey,
			AuthTokenID:           tokenIDInt64,
			ClientIP:              c.ClientIP(),
			ThinkingEffort:        thinkingEffort,
		})
		return
	}

	if tokenHashStr != "" {
		filtered, restricted := s.authService.FilterAllowedChannels(tokenHashStr, cands)
		if restricted {
			cands = filtered
			if len(cands) == 0 {
				c.JSON(http.StatusForbidden, gin.H{
					"error": "no allowed upstream channel for this token",
				})
				s.recordProxyRejection(c, startTime, originalModel, http.StatusForbidden, "no allowed upstream channel for this token", isStreaming, thinkingEffort)
				return
			}
		}
	}

	reqCtx := &proxyRequestContext{
		requestID:         requestID,
		originalModel:     originalModel,
		clientProtocol:    clientProtocol,
		requestMethod:     requestMethod,
		requestPath:       effectiveRequestPath,
		rawQuery:          c.Request.URL.RawQuery,
		body:              all,
		translatedBody:    all,
		header:            c.Request.Header,
		isStreaming:       isStreaming,
		tokenHash:         tokenHashStr,
		tokenKey:          tokenKeyStr,
		tokenID:           tokenIDInt64,
		codexGuardEnabled: codexGuardEnabled,
		clientIP:          c.ClientIP(),
		activeReqID:       activeID,
		startTime:         startTime,
		thinkingEffort:    thinkingEffort,
		timing:            timing,
	}
	reqCtx.observer = &ForwardObserver{
		OnBytesRead: func(n int64) {
			s.activeRequests.AddBytes(activeID, n)
		},
		OnFirstClientWrite: func() {
			reqCtx.markEndToEndFirstByte()
			s.activeRequests.SetClientFirstByteTime(activeID, time.Since(reqCtx.startTime))
		},
		BeforeClientResponseCommit: func() error {
			return s.activeRequests.TryCommitResponse(activeID)
		},
		OnDebugCapture: func(dc *debugCapture) {
			s.activeRequests.SetDebugCapture(activeID, dc)
		},
		Timing: timing,
	}

	lastResult, succeeded := s.runProxyAttemptLoop(ctx, cands, reqCtx, c.Writer)
	if succeeded {
		return
	}

	s.writeFinalProxyResponse(c, reqCtx, originalModel, isStreaming, lastResult, len(cands))
}

func determineFinalClientStatus(lastResult *proxyResult) int {
	if lastResult == nil || lastResult.status == 0 {
		return http.StatusServiceUnavailable
	}

	status := lastResult.status

	// 499处理：区分客户端取消 vs 上游返回的499
	if status == util.StatusClientClosedRequest {
		if lastResult.isClientCanceled {
			return status // 真正的客户端取消，透传499
		}
		return http.StatusBadGateway // 上游499，映射为502
	}

	// 仅映射内部状态码（596-599），其他全部透传
	return util.ClientStatusFor(status)
}

func shouldStopTryingChannels(result *proxyResult) bool {
	if result == nil {
		return true
	}
	// 客户端取消：立即停止
	if result.isClientCanceled {
		return true
	}
	return result.nextAction == cooldown.ActionReturnClient
}

// enforceTokenLimits 检查 token 的模型限制与费用限额。
// 违规时已写响应并返回 false，调用方应直接 return。
func (s *Server) enforceTokenLimits(
	c *gin.Context,
	clientProtocol protocol.Protocol,
	tokenHash string,
	originalModel string,
	startTime time.Time,
	isStreaming bool,
	thinkingEffort string,
) bool {
	// 检查令牌模型限制（2026-01新增）
	if tokenHash != "" && originalModel != "" {
		if !s.authService.IsModelAllowed(tokenHash, originalModel) {
			message := fmt.Sprintf("model '%s' is not allowed for this token", originalModel)
			c.JSON(http.StatusForbidden, gin.H{
				"error": message,
			})
			s.recordProxyRejection(c, startTime, originalModel, http.StatusForbidden, message, isStreaming, thinkingEffort)
			return false
		}
	}

	// 检查令牌费用限额（2026-01新增）
	// 设计决策：在请求开始时检查，费用在请求完成后记账。
	// 超额窗口：预检（IsCostLimitExceeded/RLock）与记账（AddCostToCache/Lock）之间是
	// check-then-act。设了 max_concurrency 时最多超额并发上限个请求；未设上限时 N 个并发
	// 请求可同时通过预检后全部超额——费用最终都会记账，限额是“滞后 N 个请求才封顶”，非永久绕过。
	// 原因：费用只有在请求完成后才能精确计算（token数量由上游返回），此处只能做预检查。
	// 严格“先扣费后请求”需复杂的预估+退款机制，不值得（YAGNI）。
	if tokenHash != "" {
		usedMicro, limitMicro, exceeded := s.authService.IsCostLimitExceeded(tokenHash)
		if exceeded {
			used := util.MicroUSDToUSD(usedMicro)
			limit := util.MicroUSDToUSD(limitMicro)
			message := fmt.Sprintf("Cost limit exceeded: $%.2f used of $%.2f limit", used, limit)
			writeTokenQuotaError(c, clientProtocol, message, "cost_limit_exceeded")
			s.recordProxyRejection(c, startTime, originalModel, http.StatusTooManyRequests, message, isStreaming, thinkingEffort)
			return false
		}

		dailyUsedMicro, dailyLimitMicro, dailyExceeded := s.authService.IsDailyCostLimitExceeded(tokenHash)
		if dailyExceeded {
			used := util.MicroUSDToUSD(dailyUsedMicro)
			limit := util.MicroUSDToUSD(dailyLimitMicro)
			message := fmt.Sprintf("Daily cost limit exceeded: $%.2f used of $%.2f daily limit", used, limit)
			writeTokenQuotaError(c, clientProtocol, message, "daily_cost_limit_exceeded")
			s.recordProxyRejection(c, startTime, originalModel, http.StatusTooManyRequests, message, isStreaming, thinkingEffort)
			return false
		}
	}

	return true
}

func writeTokenQuotaError(c *gin.Context, clientProtocol protocol.Protocol, message, code string) {
	errorType := "insufficient_quota"
	if clientProtocol == protocol.Codex {
		// Codex handles 429 quota responses specially only for this error type.
		errorType = "usage_limit_reached"
		setCodexPromoMessage(c.Writer.Header(), message)
	}
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error": gin.H{
			"message": message,
			"type":    errorType,
			"code":    code,
		},
	})
}

type codex429ErrorDetails struct {
	message      string
	errorType    string
	code         string
	httpEnvelope bool
}

func parseCodex429JSON(data []byte) (codex429ErrorDetails, bool) {
	type errorObject struct {
		Message any `json:"message"`
		Type    any `json:"type"`
		Code    any `json:"code"`
		Status  any `json:"status"`
	}
	var payload struct {
		Error    json.RawMessage `json:"error"`
		Message  any             `json:"message"`
		Type     any             `json:"type"`
		Code     any             `json:"code"`
		Status   any             `json:"status"`
		Response struct {
			Error errorObject `json:"error"`
		} `json:"response"`
	}
	if err := sonic.Unmarshal(data, &payload); err != nil {
		return codex429ErrorDetails{}, false
	}

	details := codex429ErrorDetails{}
	if len(payload.Error) > 0 && string(payload.Error) != "null" {
		var apiError errorObject
		if err := sonic.Unmarshal(payload.Error, &apiError); err == nil {
			details.message = codex429ScalarString(apiError.Message)
			details.errorType = codex429ScalarString(apiError.Type)
			details.code = codex429ScalarString(apiError.Code)
			if details.code == "" {
				details.code = codex429ScalarString(apiError.Status)
			}
			details.httpEnvelope = true
		} else {
			var message string
			if err := sonic.Unmarshal(payload.Error, &message); err == nil {
				details.message = strings.TrimSpace(message)
			}
		}
	}
	if details.message == "" {
		details.message = codex429ScalarString(payload.Response.Error.Message)
	}
	if details.errorType == "" {
		details.errorType = codex429ScalarString(payload.Response.Error.Type)
	}
	if details.code == "" {
		details.code = codex429ScalarString(payload.Response.Error.Code)
	}
	if details.code == "" {
		details.code = codex429ScalarString(payload.Response.Error.Status)
	}
	if details.message == "" {
		details.message = codex429ScalarString(payload.Message)
	}
	if details.errorType == "" {
		topLevelType := codex429ScalarString(payload.Type)
		normalizedType := strings.ToLower(topLevelType)
		if normalizedType != "error" && !strings.HasPrefix(normalizedType, "response.") {
			details.errorType = topLevelType
		}
	}
	if details.code == "" {
		details.code = codex429ScalarString(payload.Code)
	}
	if details.code == "" {
		details.code = codex429ScalarString(payload.Status)
	}
	return details, true
}

// codex429ScalarString accepts the scalar forms commonly used by upstream APIs
// (for example, Gemini returns error.code as the number 429).
func codex429ScalarString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return strings.TrimSpace(v.String())
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case int:
		return strconv.Itoa(v)
	case int8:
		return strconv.FormatInt(int64(v), 10)
	case int16:
		return strconv.FormatInt(int64(v), 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint8:
		return strconv.FormatUint(uint64(v), 10)
	case uint16:
		return strconv.FormatUint(uint64(v), 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case bool:
		return strconv.FormatBool(v)
	default:
		return ""
	}
}

func codex429DetailsFromBody(body []byte) codex429ErrorDetails {
	if details, ok := parseCodex429JSON(bytes.TrimSpace(body)); ok {
		return details
	}

	for remaining := body; len(remaining) > 0; {
		eventEnd := firstSSEEventEnd(remaining)
		if eventEnd < 0 {
			eventEnd = len(remaining)
		}
		_, data := parseSSEEventChunk(remaining[:eventEnd])
		if details, ok := parseCodex429JSON(bytes.TrimSpace(data)); ok {
			details.httpEnvelope = false
			return details
		}
		if eventEnd >= len(remaining) {
			break
		}
		remaining = remaining[eventEnd:]
	}

	message := strings.TrimSpace(safeBodyToString(body))
	if message == "[binary/compressed response]" {
		message = ""
	}
	return codex429ErrorDetails{message: message}
}

func sanitizeCodexPromoMessage(message string) string {
	message = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, message)
	message = strings.Join(strings.Fields(message), " ")
	const maxBytes = 1024
	if len(message) <= maxBytes {
		return message
	}
	end := maxBytes
	for end > 0 && !utf8.RuneStart(message[end]) {
		end--
	}
	return message[:end] + "..."
}

func setCodexPromoMessage(header http.Header, message string) {
	if header == nil {
		return
	}
	if message = sanitizeCodexPromoMessage(message); message != "" {
		header.Set(codexPromoMessageHeader, message)
	}
}

func codexCompatible429Response(header http.Header, body []byte) (http.Header, []byte) {
	details := codex429DetailsFromBody(body)
	responseHeader := header.Clone()
	if responseHeader == nil {
		responseHeader = make(http.Header)
	}

	if details.httpEnvelope && (details.errorType == "usage_limit_reached" || details.errorType == "usage_not_included") {
		if details.errorType == "usage_limit_reached" {
			setCodexPromoMessage(responseHeader, details.message)
		}
		return responseHeader, body
	}

	message := details.message
	if message == "" {
		message = responseHeader.Get(codexPromoMessageHeader)
	}
	if message == "" {
		message = "Request rate limit exceeded"
	}
	code := details.code
	if code == "" {
		code = details.errorType
	}
	if code == "" {
		code = "rate_limit_exceeded"
	}
	responseBody, err := sonic.Marshal(gin.H{
		"error": gin.H{
			"message": message,
			"type":    "usage_limit_reached",
			"code":    code,
		},
	})
	if err != nil {
		return responseHeader, body
	}

	responseHeader.Del("Content-Encoding")
	responseHeader.Del("Content-Length")
	responseHeader.Set("Content-Type", "application/json; charset=utf-8")
	setCodexPromoMessage(responseHeader, message)
	return responseHeader, responseBody
}

// runProxyAttemptLoop 按优先级遍历候选渠道。
// 返回最后一次结果（可能 nil），调用方据此决定是否兜底响应。
// succeeded 时内部已写响应，调用方应停止后续 writeFinal 步骤。
func (s *Server) runProxyAttemptLoop(
	ctx context.Context,
	cands []*model.Config,
	reqCtx *proxyRequestContext,
	w gin.ResponseWriter,
) (lastResult *proxyResult, succeeded bool) {
	if reqCtx != nil && reqCtx.requestID == "" {
		reqCtx.requestID = util.NewUUIDv4()
	}
	for _, cfg := range cands {
		result, err := s.tryChannelWithKeys(ctx, cfg, reqCtx, w)

		// 所有Key冷却：触发渠道级冷却(503)，防止后续请求重复尝试
		// 使用 cooldownManager.HandleError 统一处理（DRY原则）
		if err != nil && errors.Is(err, ErrAllKeysUnavailable) {
			// 统一走 applyCooldownDecision：断开取消链+按决策执行缓存失效
			s.applyCooldownDecision(ctx, cfg, httpErrorInputFromParts(cfg.ID, cooldown.NoKeyIndex, 503, nil, nil))
			continue
		}

		// [WARN] 所有Key验证失败，尝试下一个渠道
		if err != nil && errors.Is(err, ErrAllKeysExhausted) {
			log.Printf("[WARN] 渠道 %s (ID=%d) 所有Key验证失败，跳过该渠道", cfg.Name, cfg.ID)
			continue
		}

		if err != nil && errors.Is(err, ErrChannelRPMExceeded) {
			log.Printf("[INFO] 渠道 %s (ID=%d) 已达到RPM限制，跳过该渠道", cfg.Name, cfg.ID)
			continue
		}

		if err != nil && errors.Is(err, ErrChannelConcurrencyExceeded) {
			active, limit, _ := channelConcurrencyLimit(err)
			log.Printf("[INFO] request_id=%s 渠道 %s (ID=%d) 已达到并发限制 (%d/%d)，立即跳过该渠道",
				reqCtx.requestID, cfg.Name, cfg.ID, active, limit)
			continue
		}

		if result != nil {
			if result.succeeded {
				return nil, true
			}

			lastResult = result

			// 客户端已取消：别再浪费资源“重试”了。
			if result.isClientCanceled {
				break
			}

			if shouldStopTryingChannels(result) {
				break
			}
		}
	}

	return lastResult, false
}

// writeFinalProxyResponse 所有渠道失败时写最终响应：
// 计算 finalStatus、决定 skipLog、透传 body 或 JSON 错误。
func (s *Server) writeFinalProxyResponse(
	c *gin.Context,
	reqCtx *proxyRequestContext,
	originalModel string,
	isStreaming bool,
	lastResult *proxyResult,
	candidateCount int,
) {
	// 所有渠道都失败：返回“最后一次实际失败”的状态码（并映射内部状态码），避免一律伪装成503。
	finalStatus := determineFinalClientStatus(lastResult)

	msg := "exhausted backends"
	if lastResult != nil && lastResult.isClientCanceled {
		msg = "client closed request (context canceled)"
	} else if lastResult != nil && lastResult.status == 499 && finalStatus != 499 {
		// 上游返回 499 没有任何“客户端取消”的语义价值：对外统一视为网关错误。
		msg = "upstream returned 499 (mapped)"
	} else if finalStatus != http.StatusServiceUnavailable {
		msg = fmt.Sprintf("upstream status %d", finalStatus)
	}
	if lastResult != nil && lastResult.status == util.StatusCodexReasoningGuard {
		markCodexGuardExhausted(reqCtx)
	}
	msg = appendCodexGuardSummaryLogTags(msg, reqCtx)
	hasCodexGuardFinalTrace := reqCtx != nil &&
		reqCtx.codexGuardTraceID != "" &&
		lastResult != nil &&
		lastResult.status == util.StatusCodexReasoningGuard

	// 过滤不需要汇总日志的场景
	// - 客户端取消（499）：已在 handleNetworkError 中记录渠道级日志
	// - 客户端错误（400）：已在渠道级日志记录，汇总日志冗余
	// - 候选池 ≤1：实际只尝试了 1 个渠道，渠道级日志已完整反映失败原因，汇总日志冗余
	skipLog := lastResult != nil && (lastResult.isClientCanceled || finalStatus == http.StatusBadRequest)
	skipLog = skipLog || (candidateCount <= 1 && !hasCodexGuardFinalTrace)

	if lastResult != nil && lastResult.status != 0 {
		// 透明代理原则：透传所有上游响应（状态码+header+body）
		responseHeader, responseBody := lastResult.header, lastResult.body
		if finalStatus == http.StatusTooManyRequests && reqCtx.clientProtocol == protocol.Codex {
			responseHeader, responseBody = codexCompatible429Response(responseHeader, responseBody)
		}
		writeResponseWithHeaders(c.Writer, finalStatus, responseHeader, responseBody)
	} else {
		disableResponseWriteTimeout(c.Writer, "最终响应")
		c.JSON(finalStatus, gin.H{"error": "no upstream available"})
	}
	reqCtx.markEndToEndFirstByte()

	if !skipLog {
		s.AddLogAsync(&model.LogEntry{
			Time:                  model.JSONTime{Time: reqCtx.startTime},
			RequestID:             reqCtx.requestID,
			AttemptNumber:         reqCtx.attemptNumber,
			EndToEndFirstByteTime: reqCtx.getEndToEndFirstByteTime(),
			Model:                 originalModel,
			LogSource:             model.LogSourceProxy,
			StatusCode:            finalStatus,
			Message:               msg,
			Duration:              time.Since(reqCtx.startTime).Seconds(),
			IsStreaming:           isStreaming,
			ClientIP:              reqCtx.clientIP,
		})
	}
}
