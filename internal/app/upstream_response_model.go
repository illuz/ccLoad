package app

import (
	"encoding/json"
	"strings"
)

const maxUpstreamResponseModelLength = 191

// upstreamResponseModelObserver records the model declared by one upstream
// response attempt. A terminal Responses event takes precedence over an
// earlier interim event, while conflicting declarations remain diagnostic only.
type upstreamResponseModelObserver struct {
	first    string
	terminal string
	conflict bool
}

func newUpstreamResponseModelObserver() *upstreamResponseModelObserver {
	return &upstreamResponseModelObserver{}
}

func (o *upstreamResponseModelObserver) ObserveJSON(data []byte, eventType string) {
	if o == nil || len(data) == 0 {
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}
	o.ObservePayload(payload, eventType)
}

func (o *upstreamResponseModelObserver) ObservePayload(payload map[string]any, eventType string) {
	if o == nil || payload == nil {
		return
	}

	responseModel, isGeminiModelVersion := upstreamResponseModelFromPayload(payload)
	if responseModel == "" {
		return
	}

	if o.first == "" {
		o.first = responseModel
	} else if !strings.EqualFold(o.first, responseModel) {
		o.conflict = true
	}

	if isGeminiModelVersion || isUpstreamResponseTerminalEvent(eventType, payload) {
		if o.terminal != "" && !strings.EqualFold(o.terminal, responseModel) {
			o.conflict = true
		}
		o.terminal = responseModel
	}
}

func (o *upstreamResponseModelObserver) Result() (model string, conflict bool) {
	if o == nil {
		return "", false
	}
	if o.terminal != "" {
		return o.terminal, o.conflict
	}
	return o.first, o.conflict
}

func (rc *requestContext) ensureUpstreamResponseModelObserver() *upstreamResponseModelObserver {
	if rc == nil {
		return nil
	}
	if rc.responseModelObserver == nil {
		rc.responseModelObserver = newUpstreamResponseModelObserver()
	}
	return rc.responseModelObserver
}

func (rc *requestContext) observeUpstreamResponseModelJSON(data []byte, eventType string) {
	if observer := rc.ensureUpstreamResponseModelObserver(); observer != nil {
		observer.ObserveJSON(data, eventType)
	}
}

func applyUpstreamResponseModelAudit(reqCtx *requestContext, result *fwResult) {
	if result == nil || reqCtx == nil {
		return
	}
	result.UpstreamResponseModel, result.UpstreamResponseModelConflict = reqCtx.ensureUpstreamResponseModelObserver().Result()
}

func upstreamResponseModelFromPayload(payload map[string]any) (string, bool) {
	response, _ := payload["response"].(map[string]any)
	if model := upstreamResponseModelValue(response, "model"); model != "" {
		return model, false
	}
	if message, _ := payload["message"].(map[string]any); message != nil {
		if model := upstreamResponseModelValue(message, "model"); model != "" {
			return model, false
		}
	}
	if model := upstreamResponseModelValue(payload, "model"); model != "" {
		return model, false
	}
	if modelVersion := upstreamResponseModelValue(payload, "modelVersion"); modelVersion != "" {
		return modelVersion, true
	}
	if modelVersion := upstreamResponseModelValue(response, "modelVersion"); modelVersion != "" {
		return modelVersion, true
	}
	return "", false
}

func upstreamResponseModelValue(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return truncateUpstreamResponseModel(value)
}

func truncateUpstreamResponseModel(value string) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) <= maxUpstreamResponseModelLength {
		return value
	}
	return string([]rune(value)[:maxUpstreamResponseModelLength])
}

func isUpstreamResponseTerminalEvent(eventType string, payload map[string]any) bool {
	if isUpstreamResponseTerminalName(eventType) {
		return true
	}
	if payloadType, _ := payload["type"].(string); isUpstreamResponseTerminalName(payloadType) {
		return true
	}
	response, _ := payload["response"].(map[string]any)
	if status, _ := response["status"].(string); isUpstreamResponseTerminalName(status) {
		return true
	}
	return false
}

func isUpstreamResponseTerminalName(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled",
		"completed", "done", "failed", "incomplete", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func upstreamSentModel(requestModel, actualModel string) string {
	if actualModel = strings.TrimSpace(actualModel); actualModel != "" {
		return actualModel
	}
	return strings.TrimSpace(requestModel)
}

func upstreamResponseModelMismatch(sentModel, responseModel string) *bool {
	responseModel = strings.TrimSpace(responseModel)
	if responseModel == "" {
		return nil
	}
	mismatch := !strings.EqualFold(strings.TrimSpace(sentModel), responseModel)
	return &mismatch
}
