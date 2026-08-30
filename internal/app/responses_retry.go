package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"ccLoad/internal/protocol"

	"github.com/bytedance/sonic"
)

const (
	stripMissingRequiredInputStrategy    = "strip_missing_required_input"
	stripMissingStoredInputItemStrategy  = "strip_missing_stored_input_item"
	stripUnknownInputParameterStrategy   = "strip_unknown_input_parameter"
	responsesMissingStoredItemRetryLimit = 1
)

// UseNumber is required because these helpers round-trip the whole request and
// Responses payloads may contain integer seeds or tool arguments above 2^53.
var responsesJSON = sonic.Config{UseNumber: true}.Froze()

func responsesRetryBodyForMissingRequiredParameter(plan protocol.TransformPlan, res *fwResult) ([]byte, string, bool) {
	if res == nil || res.ResponseCommitted || res.Status != http.StatusBadRequest ||
		plan.ClientProtocol != protocol.Codex || plan.RequestFamily != protocol.RequestFamilyResponses {
		return nil, "", false
	}
	index, ok := missingRequiredInputIndex(res.Body)
	if !ok {
		return nil, "", false
	}
	retryBody, ok := responsesBodyWithoutInputIndex(plan.TranslatedBody, index)
	if !ok {
		return nil, "", false
	}
	return retryBody, stripMissingRequiredInputStrategy, true
}

func responsesRetryBodyForUnknownParameter(upstreamProtocol protocol.Protocol, plan protocol.TransformPlan, res *fwResult) ([]byte, string, bool) {
	if res == nil || res.ResponseCommitted || upstreamProtocol != protocol.Codex ||
		plan.ClientProtocol != protocol.Codex || plan.RequestFamily != protocol.RequestFamilyResponses {
		return nil, "", false
	}
	errorBody, status := forwardResultErrorPayload(res)
	if status != http.StatusBadRequest || !matchesUnknownInputStatusError(errorBody) {
		return nil, "", false
	}
	retryBody := stripResponsesInputItemStatus(plan.TranslatedBody)
	if bytes.Equal(retryBody, plan.TranslatedBody) {
		return nil, "", false
	}
	return retryBody, stripUnknownInputParameterStrategy, true
}

func matchesUnknownInputStatusError(body []byte) bool {
	root, ok := responsesJSONObject(body)
	if !ok {
		return false
	}
	code := strings.ToLower(firstNonEmptyString(
		responsesJSONString(root, "error", "code"),
		responsesJSONString(root, "code"),
	))
	if code != "" && code != "unknown_parameter" && code != "unsupported_parameter" {
		return false
	}
	param := firstNonEmptyString(
		responsesJSONString(root, "error", "param"),
		responsesJSONString(root, "param"),
	)
	message := firstNonEmptyString(
		responsesJSONString(root, "error", "message"),
		responsesJSONString(root, "message"),
	)
	if code == "" {
		lowerMessage := strings.ToLower(message)
		if !strings.Contains(lowerMessage, "unknown parameter") && !strings.Contains(lowerMessage, "unsupported parameter") {
			return false
		}
	}
	if param != "" {
		return isResponsesInputStatusPath(param, true)
	}
	return isResponsesInputStatusPath(message, false)
}

func isResponsesInputStatusPath(value string, exact bool) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	start := strings.Index(lower, "input[")
	if start < 0 {
		return false
	}
	rest := lower[start+len("input["):]
	end := strings.IndexByte(rest, ']')
	if end <= 0 {
		return false
	}
	if _, err := strconv.Atoi(rest[:end]); err != nil {
		return false
	}
	rest = rest[end+1:]
	if !strings.HasPrefix(rest, ".status") {
		return false
	}
	rest = rest[len(".status"):]
	if rest == "" {
		return true
	}
	if exact {
		return false
	}
	return rest[0] != '.' && rest[0] != '[' && !isResponsesPathIdentifierByte(rest[0])
}

func isResponsesPathIdentifierByte(value byte) bool {
	return value == '_' || value >= '0' && value <= '9' || value >= 'a' && value <= 'z'
}

func missingRequiredInputIndex(body []byte) (int, bool) {
	root, ok := responsesJSONObject(body)
	if !ok {
		return 0, false
	}
	code := firstNonEmptyString(
		responsesJSONString(root, "error", "code"),
		responsesJSONString(root, "code"),
	)
	if !strings.EqualFold(strings.TrimSpace(code), "missing_required_parameter") {
		return 0, false
	}
	param := firstNonEmptyString(
		responsesJSONString(root, "error", "param"),
		responsesJSONString(root, "param"),
		responsesJSONString(root, "error", "message"),
		responsesJSONString(root, "message"),
	)
	return parseResponsesInputIndex(param)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

func responsesJSONObject(body []byte) (map[string]any, bool) {
	var root map[string]any
	if err := responsesJSON.Unmarshal(body, &root); err != nil || root == nil {
		return nil, false
	}
	return root, true
}

func responsesJSONString(root map[string]any, path ...string) string {
	var value any = root
	for _, key := range path {
		object, ok := value.(map[string]any)
		if !ok {
			return ""
		}
		value, ok = object[key]
		if !ok {
			return ""
		}
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func responsesJSONStatus(root map[string]any) int {
	for _, key := range []string{"status", "status_code"} {
		switch value := root[key].(type) {
		case json.Number:
			status, err := strconv.Atoi(value.String())
			if err == nil {
				return status
			}
		case float64:
			return int(value)
		case int:
			return value
		case int64:
			return int(value)
		}
	}
	return 0
}

func parseResponsesInputIndex(param string) (int, bool) {
	param = strings.TrimSpace(param)
	start := strings.Index(strings.ToLower(param), "input[")
	if start < 0 {
		return 0, false
	}
	rest := param[start+len("input["):]
	end := strings.IndexByte(rest, ']')
	if end <= 0 {
		return 0, false
	}
	index, err := strconv.Atoi(rest[:end])
	return index, err == nil && index >= 0
}

func responsesBodyWithoutInputIndex(body []byte, index int) ([]byte, bool) {
	var root map[string]any
	if err := responsesJSON.Unmarshal(body, &root); err != nil {
		return nil, false
	}
	input, ok := root["input"].([]any)
	if !ok || index < 0 || index >= len(input) {
		return nil, false
	}
	root["input"] = append(append(make([]any, 0, len(input)-1), input[:index]...), input[index+1:]...)
	encoded, err := sonic.Marshal(root)
	return encoded, err == nil
}

func responsesRetryBodyForMissingStoredInputItem(plan protocol.TransformPlan, res *fwResult) ([]byte, string, bool) {
	if res == nil || res.ResponseCommitted || plan.ClientProtocol != protocol.Codex ||
		plan.RequestFamily != protocol.RequestFamilyResponses {
		return nil, "", false
	}
	errorBody, status := forwardResultErrorPayload(res)
	if status != http.StatusBadRequest && status != http.StatusNotFound {
		return nil, "", false
	}
	id, ok := missingStoredInputItemID(errorBody)
	if !ok {
		return nil, "", false
	}
	retryBody, ok := responsesBodyWithoutMissingReasoningID(plan.TranslatedBody, id)
	if !ok {
		return nil, "", false
	}
	return retryBody, stripMissingStoredInputItemStrategy + ":" + id, true
}

func forwardResultErrorPayload(res *fwResult) ([]byte, int) {
	if res == nil {
		return nil, 0
	}
	if len(res.SSEErrorEvent) > 0 {
		root, _ := responsesJSONObject(res.SSEErrorEvent)
		status := responsesJSONStatus(root)
		if status >= 400 && status <= 599 {
			return res.SSEErrorEvent, status
		}
		return res.SSEErrorEvent, classifySSEErrorStatus(res.SSEErrorEvent)
	}
	return res.Body, res.Status
}

func missingStoredInputItemID(body []byte) (string, bool) {
	root, ok := responsesJSONObject(body)
	if !ok {
		return "", false
	}
	message := firstNonEmptyString(
		responsesJSONString(root, "error", "message"),
		responsesJSONString(root, "message"),
		responsesJSONString(root, "response", "error", "message"),
	)
	return parseMissingStoredInputItemID(message)
}

func parseMissingStoredInputItemID(message string) (string, bool) {
	lower := strings.ToLower(message)
	if !strings.Contains(lower, "not found") {
		return "", false
	}
	for _, marker := range []string{"item with id '", `item with id "`} {
		start := strings.Index(lower, marker)
		if start < 0 {
			continue
		}
		rest := message[start+len(marker):]
		end := strings.IndexByte(rest, marker[len(marker)-1])
		if end > 0 {
			if id := strings.TrimSpace(rest[:end]); id != "" {
				return id, true
			}
		}
	}
	return "", false
}

func responsesBodyWithoutMissingReasoningID(body []byte, id string) ([]byte, bool) {
	if id == "" {
		return nil, false
	}
	var root map[string]any
	if err := responsesJSON.Unmarshal(body, &root); err != nil {
		return nil, false
	}
	input, ok := root["input"].([]any)
	if !ok {
		return nil, false
	}
	filtered := make([]any, 0, len(input))
	removed := false
	for _, item := range input {
		obj, ok := item.(map[string]any)
		if ok && obj["id"] == id && obj["type"] == "reasoning" {
			removed = true
			continue
		}
		filtered = append(filtered, item)
	}
	if !removed {
		return nil, false
	}
	root["input"] = filtered
	encoded, err := sonic.Marshal(root)
	return encoded, err == nil
}

func stripResponsesInputItemStatus(body []byte) []byte {
	if !bytes.Contains(body, []byte(`"status"`)) {
		return body
	}
	var root map[string]any
	if err := responsesJSON.Unmarshal(body, &root); err != nil {
		return body
	}
	items, ok := root["input"].([]any)
	if !ok {
		return body
	}
	changed := false
	for _, item := range items {
		if obj, ok := item.(map[string]any); ok {
			if _, exists := obj["status"]; exists {
				delete(obj, "status")
				changed = true
			}
		}
	}
	if !changed {
		return body
	}
	out, err := sonic.Marshal(root)
	if err != nil {
		return body
	}
	return out
}

func responsesBodyForHTTPTransport(reqCtx *requestContext, body []byte) []byte {
	if reqCtx == nil || reqCtx.transformPlan.ClientProtocol != protocol.Codex ||
		reqCtx.transformPlan.UpstreamProtocol != protocol.Codex ||
		reqCtx.transformPlan.RequestFamily != protocol.RequestFamilyResponses {
		return body
	}
	return stripResponsesInputItemStatus(body)
}

func hasRetryStrategy(strategies []string, strategy string) bool {
	for _, existing := range strategies {
		if existing == strategy {
			return true
		}
	}
	return false
}
