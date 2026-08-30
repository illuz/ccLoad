package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ccLoad/internal/config"
	"ccLoad/internal/model"
	"ccLoad/internal/protocol"
	"ccLoad/internal/testutil"
	"ccLoad/internal/util"
	"ccLoad/internal/version"

	"github.com/bytedance/sonic"
	"github.com/gin-gonic/gin"
)

const (
	imageGenerationPath                = "/v1/images/generations"
	imageGenerationDiagnosticBodyLimit = 64 << 10
	imageGenerationAPIImages           = "images"
	imageGenerationAPIChatCompletions  = "chat_completions"
)

type imageGenerationTestRequest struct {
	GenerationAPI string `json:"generation_api"`
	Model         string `json:"model"`
	Prompt        string `json:"prompt"`
	Size          string `json:"size,omitempty"`
	Quality       string `json:"quality,omitempty"`
	Background    string `json:"background,omitempty"`
	OutputFormat  string `json:"output_format,omitempty"`
	KeyIndex      int    `json:"key_index,omitempty"`
}

func (r *imageGenerationTestRequest) Validate() error {
	if r == nil {
		return errors.New("request is required")
	}
	r.Model = strings.TrimSpace(r.Model)
	r.Prompt = strings.TrimSpace(r.Prompt)
	r.GenerationAPI = strings.ToLower(strings.TrimSpace(r.GenerationAPI))
	r.Size = strings.ToLower(strings.TrimSpace(r.Size))
	r.Quality = strings.ToLower(strings.TrimSpace(r.Quality))
	r.Background = strings.ToLower(strings.TrimSpace(r.Background))
	r.OutputFormat = strings.ToLower(strings.TrimSpace(r.OutputFormat))

	if r.Model == "" || len(r.Model) > 191 {
		return errors.New("model is required and must not exceed 191 characters")
	}
	if r.Prompt == "" || len(r.Prompt) > 32*1024 {
		return errors.New("prompt is required and must not exceed 32 KiB")
	}
	if r.KeyIndex < 0 {
		return errors.New("key_index must not be negative")
	}
	if !imageOptionIsOneOf(r.GenerationAPI, imageGenerationAPIImages, imageGenerationAPIChatCompletions) {
		return errors.New("generation_api must be images or chat_completions")
	}
	if r.GenerationAPI == imageGenerationAPIChatCompletions {
		if !validChatImageGenerationSize(r.Size) {
			return errors.New("invalid Chat Completions image size")
		}
		if !imageOptionIsOneOf(r.Quality, "", "auto") ||
			!imageOptionIsOneOf(r.Background, "", "auto") ||
			!imageOptionIsOneOf(r.OutputFormat, "", "auto") {
			return errors.New("chat completions image generation does not support quality, background, or output_format")
		}
	} else if !validImageGenerationSize(r.Size) {
		return errors.New("invalid Images API image size")
	}
	if !validImageGenerationOption(r.Quality) {
		return errors.New("invalid image quality")
	}
	if !imageOptionIsOneOf(r.Background, "", "auto", "opaque", "transparent") {
		return errors.New("invalid image background")
	}
	if !imageOptionIsOneOf(r.OutputFormat, "", "auto", "png", "jpeg", "webp") {
		return errors.New("invalid image output format")
	}
	return nil
}

func validImageGenerationSize(value string) bool {
	if value == "" || value == "auto" {
		return true
	}
	widthText, heightText, ok := strings.Cut(value, "x")
	if !ok || strings.Contains(heightText, "x") {
		return false
	}
	width, widthErr := strconv.Atoi(widthText)
	height, heightErr := strconv.Atoi(heightText)
	return widthErr == nil && heightErr == nil && width >= 64 && width <= 8192 && height >= 64 && height <= 8192
}

func validChatImageGenerationSize(value string) bool {
	if value == "" || value == "auto" {
		return true
	}
	aspectRatio, imageSize, ok := strings.Cut(value, "@")
	return ok && imageOptionIsOneOf(aspectRatio, "1:1", "16:9", "9:16", "3:2", "2:3") &&
		imageOptionIsOneOf(imageSize, "1k", "2k")
}

func validImageGenerationOption(value string) bool {
	if len(value) > 32 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func imageOptionIsOneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

// HandleChannelImageGeneration tests image generation through one channel and API key.
func (s *Server) HandleChannelImageGeneration(c *gin.Context) {
	id, err := ParseInt64Param(c, "id")
	if err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid channel id")
		return
	}

	var imageReq imageGenerationTestRequest
	if err := BindAndValidate(c, &imageReq); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid image generation request")
		return
	}

	cfg, err := s.store.GetConfig(c.Request.Context(), id)
	if err != nil {
		RespondError(c, http.StatusNotFound, fmt.Errorf("channel not found"))
		return
	}
	if !imageGenerationModelSupported(cfg, imageReq.Model) {
		RespondJSON(c, http.StatusOK, gin.H{
			"success":          false,
			"error":            "模型 " + imageReq.Model + " 不在此渠道的支持列表中",
			"model":            imageReq.Model,
			"supported_models": cfg.GetModels(),
		})
		return
	}

	apiKeys, err := s.store.GetAPIKeys(c.Request.Context(), id)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, err)
		return
	}
	if len(apiKeys) == 0 {
		RespondJSON(c, http.StatusOK, gin.H{
			"success": false,
			"error":   "渠道未配置有效的 API Key",
		})
		return
	}
	keySelection, err := s.selectChannelTestKey(apiKeys, imageReq.KeyIndex, "")
	if err != nil {
		RespondJSON(c, http.StatusOK, gin.H{
			"success":    false,
			"error":      err.Error(),
			"total_keys": len(apiKeys),
		})
		return
	}

	requestedModel := imageReq.Model
	actualModel := s.resolveFinalUpstreamModel(
		cfg,
		requestedModel,
		resolveTestUpstreamProtocol(cfg, util.ChannelTypeOpenAI),
	)
	result := s.testChannelImageGenerationByAPI(c.Request.Context(), cfg, keySelection.apiKey, &imageReq)
	if reportedModel := getResultString(result, "actual_model"); reportedModel != "" {
		actualModel = reportedModel
	} else {
		result["actual_model"] = actualModel
	}
	result = s.applyChannelTestResultCooldown(
		c.Request.Context(), cfg, keySelection.keyIndex, actualModel,
		keySelection.updatePersistedCooldown, result,
	)
	result["tested_key_index"] = keySelection.keyIndex
	result["total_keys"] = len(apiKeys)
	result["generation_api"] = imageReq.GenerationAPI

	s.persistDetectionLog(c.Request.Context(), detectionLogFromResult(
		cfg,
		model.LogSourceManualTest,
		requestedModel,
		actualModel,
		keySelection.apiKey,
		c.ClientIP(),
		0,
		"",
		result,
	))
	delete(result, "debug_data")
	RespondJSON(c, http.StatusOK, result)
}

func imageGenerationModelSupported(cfg *model.Config, requestedModel string) bool {
	return cfg != nil && (cfg.SupportsModel(requestedModel) || cfg.SupportsModel("*"))
}

func (s *Server) testChannelImageGenerationByAPI(
	ctx context.Context,
	cfg *model.Config,
	apiKey string,
	imageReq *imageGenerationTestRequest,
) map[string]any {
	if imageReq.GenerationAPI == imageGenerationAPIChatCompletions {
		result := s.testChannelAPI(ctx, cfg, apiKey, imageGenerationChannelTestRequest(imageReq))
		return normalizeChatCompletionsImageResult(result)
	}
	return s.testChannelImageGeneration(ctx, cfg, apiKey, imageReq)
}

func imageGenerationChannelTestRequest(imageReq *imageGenerationTestRequest) *testutil.TestChannelRequest {
	testReq := &testutil.TestChannelRequest{
		Model:             imageReq.Model,
		Content:           imageReq.Prompt,
		ProtocolTransform: util.ChannelTypeOpenAI,
		Stream:            false,
	}
	if imageReq.GenerationAPI == imageGenerationAPIChatCompletions {
		testReq.ImageGeneration = chatCompletionsImageOptions(imageReq.Size)
	}
	return testReq
}

func chatCompletionsImageOptions(size string) *testutil.ImageGenerationOptions {
	options := &testutil.ImageGenerationOptions{}
	normalizedSize := strings.ToLower(strings.TrimSpace(size))
	if normalizedSize == "" || normalizedSize == "auto" {
		return options
	}
	aspectRatio, imageSize, ok := strings.Cut(normalizedSize, "@")
	if !ok {
		return options
	}
	options.AspectRatio = aspectRatio
	options.ImageSize = strings.ToUpper(imageSize)
	return options
}

func normalizeChatCompletionsImageResult(result map[string]any) map[string]any {
	if result == nil {
		return map[string]any{"success": false, "error": "渠道测试失败: 上游返回空结果"}
	}
	delete(result, "cost_usd")
	if success, _ := result["success"].(bool); !success {
		return result
	}
	apiResponse, _ := result["api_response"].(map[string]any)
	images := extractChatCompletionsImageData(apiResponse)
	if len(images) == 0 {
		result["success"] = false
		result["error"] = "Chat Completions 响应中没有可显示图片"
		delete(result, "message")
		return result
	}
	result["images"] = images
	result["message"] = "图片生成成功"
	if outputFormat := imageOutputFormatFromMIMEType(getResultString(images[0], "mime_type")); outputFormat != "" {
		result["output_format"] = outputFormat
	}
	delete(result, "api_response")
	delete(result, "upstream_response_body")
	delete(result, "raw_response")
	return result
}

func extractChatCompletionsImageData(apiResponse map[string]any) []map[string]any {
	if apiResponse == nil {
		return nil
	}
	choices, _ := apiResponse["choices"].([]any)
	images := make([]map[string]any, 0, len(choices))
	for _, rawChoice := range choices {
		choice, _ := rawChoice.(map[string]any)
		if choice == nil {
			continue
		}
		for _, containerKey := range []string{"message", "delta"} {
			container, _ := choice[containerKey].(map[string]any)
			if container == nil {
				continue
			}
			before := len(images)
			appendChatCompletionsImages(&images, container["images"])
			if len(images) == before {
				appendChatCompletionsContentImages(&images, container["content"])
			}
			if len(images) > before {
				break
			}
		}
	}
	return images
}

func appendChatCompletionsImages(images *[]map[string]any, raw any) {
	items, _ := raw.([]any)
	for _, rawItem := range items {
		item, _ := rawItem.(map[string]any)
		if item == nil {
			continue
		}
		imageURL, _ := item["image_url"].(map[string]any)
		url, _ := imageURL["url"].(string)
		if url == "" {
			url, _ = item["url"].(string)
		}
		if image, ok := normalizeChatCompletionsImageURL(url); ok {
			*images = append(*images, image)
		}
	}
}

func appendChatCompletionsContentImages(images *[]map[string]any, raw any) {
	items, _ := raw.([]any)
	for _, rawItem := range items {
		item, _ := rawItem.(map[string]any)
		if item == nil || !strings.EqualFold(getResultString(item, "type"), "image_url") {
			continue
		}
		imageURL, _ := item["image_url"].(map[string]any)
		if image, ok := normalizeChatCompletionsImageURL(getResultString(imageURL, "url")); ok {
			*images = append(*images, image)
		}
	}
}

func normalizeChatCompletionsImageURL(raw string) (map[string]any, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, false
	}
	comma := strings.IndexByte(value, ',')
	if comma < 0 || !strings.HasPrefix(strings.ToLower(value), "data:") {
		return map[string]any{"url": value}, true
	}
	metadata := strings.TrimSpace(value[len("data:"):comma])
	lowerMetadata := strings.ToLower(metadata)
	if !strings.HasPrefix(lowerMetadata, "image/") || !strings.HasSuffix(lowerMetadata, ";base64") {
		return nil, false
	}
	mimeType := strings.TrimSpace(metadata[:len(metadata)-len(";base64")])
	base64Data := strings.TrimSpace(value[comma+1:])
	if mimeType == "" || base64Data == "" {
		return nil, false
	}
	return map[string]any{"b64_json": base64Data, "mime_type": strings.ToLower(mimeType)}, true
}

func imageOutputFormatFromMIMEType(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg":
		return "jpeg"
	case "image/png":
		return "png"
	case "image/webp":
		return "webp"
	default:
		return ""
	}
}

func (s *Server) testChannelImageGeneration(
	ctx context.Context,
	cfg *model.Config,
	apiKey string,
	imageReq *imageGenerationTestRequest,
) map[string]any {
	urls := cfg.GetURLs()
	if len(urls) == 0 {
		return map[string]any{"success": false, "error": "渠道URL为空"}
	}

	var selector *URLSelector
	if len(urls) > 1 && s.urlSelector != nil {
		selector = s.urlSelector
	}
	orderedURLs := orderURLsWithSelector(selector, cfg.ID, urls)

	var lastResult map[string]any
	for idx, entry := range orderedURLs {
		lastResult = s.testChannelImageGenerationWithURL(ctx, cfg, apiKey, imageReq, entry.url)
		lastResult["base_url"] = entry.url
		if success, _ := lastResult["success"].(bool); success {
			if selector != nil {
				selector.RecordLatency(cfg.ID, entry.url, pickURLSelectorLatency(lastResult))
			}
			return lastResult
		}
		if clientCanceled, _ := lastResult["client_canceled"].(bool); clientCanceled {
			return lastResult
		}
		if idx == len(orderedURLs)-1 {
			break
		}
		continueFallback, shouldCooldown := shouldFallbackToNextURL(lastResult)
		if shouldCooldown && selector != nil {
			selector.CooldownURL(cfg.ID, entry.url)
		}
		if !continueFallback {
			break
		}
	}
	if lastResult != nil {
		return lastResult
	}
	return map[string]any{"success": false, "error": "渠道测试失败: 未找到可用 URL"}
}

func (s *Server) testChannelImageGenerationWithURL(
	parent context.Context,
	cfg *model.Config,
	apiKey string,
	imageReq *imageGenerationTestRequest,
	selectedURL string,
) map[string]any {
	start := time.Now()
	actualModel := s.resolveFinalUpstreamModel(cfg, imageReq.Model, util.ChannelTypeOpenAI)
	body, err := imageGenerationRequestBody(actualModel, imageReq)
	if err != nil {
		result := imageGenerationErrorResult(start, err)
		result["error"] = err.Error()
		return annotateImageGenerationResult(result, actualModel)
	}
	body = applyBodyRules("application/json", body, cfg.BodyRules())
	actualModel = resolveModelAfterBodyRules(actualModel, cfg.BodyRules())

	ctx, timeout := s.newChannelTestTimeoutContextWithTimeouts(parent, false, s.resolveProtocolTimeouts(cfg, protocol.TransformPlan{
		UpstreamProtocol: protocol.OpenAI,
	}))
	defer timeout.cancelAll()

	upstreamURL := buildUpstreamURL(selectedURL, imageGenerationPath, "")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return annotateImageGenerationResult(imageGenerationErrorResult(start, fmt.Errorf("创建HTTP请求失败: %w", err)), actualModel)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", version.OutboundUserAgent())
	injectAPIKeyHeaders(req, apiKey, imageGenerationPath)
	applyHeaderRules(req.Header, cfg.HeaderRules())

	var debugCapture *debugCapture
	if s.configService != nil {
		debugCapture = s.captureDebugRequest(req, body)
	}
	resp, err := s.doUpstreamRequest(cfg, req)
	if err != nil {
		if debugCapture != nil && debugCapture.respBuf != nil {
			_, _ = debugCapture.respBuf.Write([]byte("upstream request failed: " + err.Error()))
		}
		result := imageGenerationErrorResult(start, err)
		switch {
		case errors.Is(err, ErrChannelRPMExceeded):
			result = channelRPMExceededTestResult(start, channelRPMRetryAfter(err))
		case errors.Is(err, ErrChannelConcurrencyExceeded):
			result = channelConcurrencyExceededTestResult(start, err)
		case errors.Is(err, context.DeadlineExceeded):
			result["status_code"] = http.StatusGatewayTimeout
			result["error"] = "非流式请求超时: " + err.Error()
		case errors.Is(err, context.Canceled):
			result["status_code"] = util.StatusClientClosedRequest
			result["error"] = "客户端已取消请求"
			result["client_canceled"] = true
		}
		if debugCapture != nil {
			result["debug_data"] = debugCapture.buildEntry(nil)
		}
		return annotateImageGenerationResult(result, actualModel)
	}
	if resp == nil || resp.Body == nil {
		return annotateImageGenerationResult(imageGenerationErrorResult(start, errors.New("上游返回空响应")), actualModel)
	}
	defer func() { _ = resp.Body.Close() }()

	firstByteDuration := time.Since(start).Milliseconds()
	if debugCapture != nil {
		debugCapture.captureResponseMeta(resp)
	}
	responseBody, readErr := readLimitedImageGenerationResponse(resp.Body, int64(config.DefaultMaxImageBodyBytes))
	result := map[string]any{
		"success":                false,
		"status_code":            resp.StatusCode,
		"duration_ms":            time.Since(start).Milliseconds(),
		"first_byte_duration_ms": firstByteDuration,
		"is_streaming":           false,
		"client_protocol":        util.ChannelTypeOpenAI,
		"upstream_protocol":      util.ChannelTypeOpenAI,
		"response_headers":       flattenHeader(resp.Header),
	}
	if readErr != nil {
		switch {
		case errors.Is(readErr, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
			result["status_code"] = http.StatusGatewayTimeout
			result["error"] = "非流式请求超时: " + readErr.Error()
		case errors.Is(readErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled):
			result["status_code"] = util.StatusClientClosedRequest
			result["error"] = "客户端已取消请求"
			result["client_canceled"] = true
		default:
			result["error"] = readErr.Error()
		}
	} else {
		parseImageGenerationResponse(result, resp, responseBody, imageReq.OutputFormat)
	}
	if success, _ := result["success"].(bool); !success && len(responseBody) > 0 {
		diagnosticBody := imageGenerationDiagnosticBody(responseBody)
		if _, hasStructuredError := result["api_error"]; !hasStructuredError {
			result["raw_response"] = diagnosticBody
		}
		if debugCapture != nil && debugCapture.respBuf != nil {
			_, _ = debugCapture.respBuf.Write([]byte(diagnosticBody))
		}
	}
	result["duration_ms"] = time.Since(start).Milliseconds()
	if debugCapture != nil {
		result["debug_data"] = debugCapture.buildEntry(resp)
	}
	return annotateImageGenerationResult(result, actualModel)
}

func imageGenerationRequestBody(actualModel string, imageReq *imageGenerationTestRequest) ([]byte, error) {
	payload := map[string]any{
		"model":  actualModel,
		"prompt": imageReq.Prompt,
	}
	if imageReq.Size != "" && imageReq.Size != "auto" {
		payload["size"] = imageReq.Size
	}
	if imageReq.Quality != "" && imageReq.Quality != "auto" {
		payload["quality"] = imageReq.Quality
	}
	if imageReq.Background != "" && imageReq.Background != "auto" {
		payload["background"] = imageReq.Background
	}
	if imageReq.OutputFormat != "" && imageReq.OutputFormat != "auto" {
		payload["output_format"] = imageReq.OutputFormat
	}
	return sonic.Marshal(payload)
}

func readLimitedImageGenerationResponse(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return body, fmt.Errorf("读取响应失败: %w", err)
	}
	if int64(len(body)) > limit {
		return body[:limit], fmt.Errorf("生图响应超过 %d 字节上限", limit)
	}
	return body, nil
}

func imageGenerationDiagnosticBody(body []byte) string {
	if len(body) <= imageGenerationDiagnosticBodyLimit {
		return string(body)
	}
	return string(body[:imageGenerationDiagnosticBodyLimit]) + "\n... response truncated"
}

func parseImageGenerationResponse(
	result map[string]any,
	resp *http.Response,
	body []byte,
	requestedOutputFormat string,
) {
	var apiResponse map[string]any
	if err := sonic.Unmarshal(body, &apiResponse); err != nil {
		result["error"] = "上游返回了无效的 JSON 响应"
		return
	}
	if rawError, exists := apiResponse["error"]; exists && rawError != nil {
		message := extractTestAPIErrorMessage(apiResponse)
		if message == "" {
			message = "上游返回了结构化错误"
		}
		result["error"] = message
		result["api_error"] = apiResponse
		return
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message := extractTestAPIErrorMessage(apiResponse)
		if message == "" {
			message = "API返回错误状态: " + resp.Status
		}
		result["error"] = message
		result["api_error"] = apiResponse
		return
	}

	images := extractImageGenerationData(apiResponse)
	if len(images) == 0 {
		result["error"] = "上游响应中没有可显示图片"
		return
	}
	result["success"] = true
	result["message"] = "图片生成成功"
	result["images"] = images
	for _, key := range []string{"created", "background", "output_format", "quality", "size", "usage"} {
		if value, ok := apiResponse[key]; ok {
			result[key] = value
		}
	}
	if _, ok := result["output_format"]; !ok && requestedOutputFormat != "" && requestedOutputFormat != "auto" {
		result["output_format"] = requestedOutputFormat
	}
	if _, ok := result["output_format"]; !ok {
		if outputFormat := imageOutputFormatFromMIMEType(getResultString(images[0], "mime_type")); outputFormat != "" {
			result["output_format"] = outputFormat
		}
	}
}

func extractImageGenerationData(apiResponse map[string]any) []map[string]any {
	rawImages, ok := apiResponse["data"].([]any)
	if !ok {
		return nil
	}
	images := make([]map[string]any, 0, len(rawImages))
	for _, rawImage := range rawImages {
		image, ok := rawImage.(map[string]any)
		if !ok {
			continue
		}
		url, _ := image["url"].(string)
		base64JSON, _ := image["b64_json"].(string)
		if strings.TrimSpace(url) == "" && strings.TrimSpace(base64JSON) == "" {
			continue
		}
		normalized := map[string]any{}
		if url != "" {
			normalized["url"] = url
		}
		if base64JSON != "" {
			normalized["b64_json"] = base64JSON
			if mimeType := imageMIMETypeFromBase64(base64JSON); mimeType != "" {
				normalized["mime_type"] = mimeType
			}
		}
		if revisedPrompt, _ := image["revised_prompt"].(string); revisedPrompt != "" {
			normalized["revised_prompt"] = revisedPrompt
		}
		images = append(images, normalized)
	}
	return images
}

func imageMIMETypeFromBase64(encoded string) string {
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(strings.TrimSpace(encoded)))
	header := make([]byte, 16)
	n, err := decoder.Read(header)
	if n == 0 || (err != nil && !errors.Is(err, io.EOF)) {
		return ""
	}
	switch http.DetectContentType(header[:n]) {
	case "image/png":
		return "image/png"
	case "image/jpeg":
		return "image/jpeg"
	case "image/webp":
		return "image/webp"
	case "image/gif":
		return "image/gif"
	default:
		return ""
	}
}

func imageGenerationErrorResult(start time.Time, err error) map[string]any {
	return map[string]any{
		"success":           false,
		"error":             "网络请求失败: " + err.Error(),
		"duration_ms":       time.Since(start).Milliseconds(),
		"is_streaming":      false,
		"client_protocol":   util.ChannelTypeOpenAI,
		"upstream_protocol": util.ChannelTypeOpenAI,
	}
}

func annotateImageGenerationResult(result map[string]any, actualModel string) map[string]any {
	result["actual_model"] = actualModel
	result["client_protocol"] = util.ChannelTypeOpenAI
	result["upstream_protocol"] = util.ChannelTypeOpenAI
	result["is_streaming"] = false
	return result
}
