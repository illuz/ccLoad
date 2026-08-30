package app

import (
	"encoding/json"
	"strings"

	"ccLoad/internal/model"
	"ccLoad/internal/protocol"

	"github.com/bytedance/sonic"
)

// applyThinkingSuffix writes a recognized model suffix into the original
// client-protocol body. Protocol translation then carries that explicit setting
// to the selected upstream protocol.
func applyThinkingSuffix(body []byte, clientProtocol protocol.Protocol, requestedModel string) []byte {
	_, cfg, ok := model.ParseThinkingSuffix(requestedModel)
	if !ok || len(body) == 0 {
		return body
	}

	switch clientProtocol {
	case protocol.OpenAI:
		return applyOpenAIThinkingSuffix(body, cfg)
	case protocol.Codex:
		return applyCodexThinkingSuffix(body, cfg)
	case protocol.Anthropic:
		return applyAnthropicThinkingSuffix(body, cfg)
	case protocol.Gemini:
		return applyGeminiThinkingSuffix(body, cfg)
	default:
		return body
	}
}

func thinkingEffortFromRequest(requestedModel string, body []byte) string {
	if _, cfg, ok := model.ParseThinkingSuffix(requestedModel); ok {
		return thinkingSuffixEffort(cfg)
	}
	return extractThinkingEffortFromJSON(body)
}

func thinkingSuffixEffort(cfg model.ThinkingSuffixConfig) string {
	switch cfg.Mode {
	case model.ThinkingSuffixNone:
		return "none"
	case model.ThinkingSuffixAuto:
		return "auto"
	case model.ThinkingSuffixLevel:
		return strings.ToLower(strings.TrimSpace(cfg.Level))
	case model.ThinkingSuffixBudget:
		switch {
		case cfg.Budget <= 512:
			return "minimal"
		case cfg.Budget <= 1024:
			return "low"
		case cfg.Budget <= 8192:
			return "medium"
		case cfg.Budget <= 24576:
			return "high"
		default:
			return "xhigh"
		}
	default:
		return ""
	}
}

func applyOpenAIThinkingSuffix(body []byte, cfg model.ThinkingSuffixConfig) []byte {
	return mutateRawJSONObject(body, func(root map[string]json.RawMessage) {
		if cfg.Mode == model.ThinkingSuffixAuto {
			delete(root, "reasoning_effort")
			return
		}
		effort := thinkingSuffixEffort(cfg)
		if effort == "max" {
			effort = "xhigh"
		}
		setRawJSONValue(root, "reasoning_effort", effort)
	})
}

func applyCodexThinkingSuffix(body []byte, cfg model.ThinkingSuffixConfig) []byte {
	return mutateRawJSONObject(body, func(root map[string]json.RawMessage) {
		reasoning := rawJSONObject(root["reasoning"])
		if cfg.Mode == model.ThinkingSuffixAuto {
			delete(reasoning, "effort")
		} else {
			effort := thinkingSuffixEffort(cfg)
			if effort == "max" {
				effort = "xhigh"
			}
			setRawJSONValue(reasoning, "effort", effort)
		}
		setOrDeleteRawJSONObject(root, "reasoning", reasoning)
	})
}

func applyAnthropicThinkingSuffix(body []byte, cfg model.ThinkingSuffixConfig) []byte {
	return mutateRawJSONObject(body, func(root map[string]json.RawMessage) {
		if cfg.Mode == model.ThinkingSuffixNone {
			delete(root, "thinking")
			deleteRawJSONObjectField(root, "output_config", "effort")
			return
		}

		thinking := rawJSONObject(root["thinking"])
		if cfg.Mode == model.ThinkingSuffixBudget {
			setRawJSONValue(thinking, "type", "enabled")
			setRawJSONValue(thinking, "budget_tokens", cfg.Budget)
			setOrDeleteRawJSONObject(root, "thinking", thinking)
			deleteRawJSONObjectField(root, "output_config", "effort")
			return
		}

		setRawJSONValue(thinking, "type", "adaptive")
		delete(thinking, "budget_tokens")
		setOrDeleteRawJSONObject(root, "thinking", thinking)
		if cfg.Mode == model.ThinkingSuffixAuto {
			deleteRawJSONObjectField(root, "output_config", "effort")
			return
		}

		effort := thinkingSuffixEffort(cfg)
		switch effort {
		case "minimal":
			effort = "low"
		case "xhigh":
			effort = "max"
		}
		outputConfig := rawJSONObject(root["output_config"])
		setRawJSONValue(outputConfig, "effort", effort)
		setOrDeleteRawJSONObject(root, "output_config", outputConfig)
	})
}

func applyGeminiThinkingSuffix(body []byte, cfg model.ThinkingSuffixConfig) []byte {
	return mutateRawJSONObject(body, func(root map[string]json.RawMessage) {
		generationConfig := rawJSONObject(root["generationConfig"])
		thinkingConfig := rawJSONObject(generationConfig["thinkingConfig"])

		switch cfg.Mode {
		case model.ThinkingSuffixNone:
			delete(thinkingConfig, "thinkingLevel")
			setRawJSONValue(thinkingConfig, "thinkingBudget", 0)
		case model.ThinkingSuffixAuto:
			delete(thinkingConfig, "thinkingLevel")
			setRawJSONValue(thinkingConfig, "thinkingBudget", -1)
		case model.ThinkingSuffixBudget:
			delete(thinkingConfig, "thinkingLevel")
			setRawJSONValue(thinkingConfig, "thinkingBudget", cfg.Budget)
		case model.ThinkingSuffixLevel:
			delete(thinkingConfig, "thinkingBudget")
			level := thinkingSuffixEffort(cfg)
			switch level {
			case "minimal", "low":
				level = "low"
			case "xhigh", "max":
				level = "high"
			}
			setRawJSONValue(thinkingConfig, "thinkingLevel", level)
		}

		setOrDeleteRawJSONObject(generationConfig, "thinkingConfig", thinkingConfig)
		setOrDeleteRawJSONObject(root, "generationConfig", generationConfig)
	})
}

func mutateRawJSONObject(body []byte, mutate func(map[string]json.RawMessage)) []byte {
	var root map[string]json.RawMessage
	if err := sonic.Unmarshal(body, &root); err != nil || root == nil {
		return body
	}
	mutate(root)
	out, err := sonic.Marshal(root)
	if err != nil {
		return body
	}
	return out
}

func rawJSONObject(raw json.RawMessage) map[string]json.RawMessage {
	var value map[string]json.RawMessage
	if len(raw) > 0 {
		_ = sonic.Unmarshal(raw, &value)
	}
	if value == nil {
		value = make(map[string]json.RawMessage)
	}
	return value
}

func setRawJSONValue(object map[string]json.RawMessage, key string, value any) {
	raw, err := sonic.Marshal(value)
	if err == nil {
		object[key] = raw
	}
}

func setOrDeleteRawJSONObject(parent map[string]json.RawMessage, key string, object map[string]json.RawMessage) {
	if len(object) == 0 {
		delete(parent, key)
		return
	}
	setRawJSONValue(parent, key, object)
}

func deleteRawJSONObjectField(parent map[string]json.RawMessage, key, field string) {
	object := rawJSONObject(parent[key])
	delete(object, field)
	setOrDeleteRawJSONObject(parent, key, object)
}
