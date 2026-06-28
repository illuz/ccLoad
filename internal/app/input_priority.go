package app

import (
	"encoding/json"
	"math"
	"strings"
	"unicode/utf8"
)

const charsPerEstimatedToken = 4

// estimateRequestInputTokens 轻量估算请求输入量，用于路由前优先级加成。
// 优先读取显式 usage/token 字段；否则提取常见文本输入字段并按约 4 chars/token 估算。
func estimateRequestInputTokens(body []byte) int {
	if len(body) == 0 {
		return 0
	}
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return 0
	}
	if m, ok := root.(map[string]any); ok {
		for _, key := range []string{"input_tokens", "prompt_tokens", "promptTokenCount"} {
			if n := jsonNumberAsInt(m[key]); n > 0 {
				return n
			}
		}
		if usage, ok := m["usage"].(map[string]any); ok {
			for _, key := range []string{"input_tokens", "prompt_tokens"} {
				if n := jsonNumberAsInt(usage[key]); n > 0 {
					return n
				}
			}
		}
		if usage, ok := m["usageMetadata"].(map[string]any); ok {
			if n := jsonNumberAsInt(usage["promptTokenCount"]); n > 0 {
				return n
			}
		}
	}
	chars := countTextInputChars(root)
	if chars <= 0 {
		return 0
	}
	return int(math.Ceil(float64(chars) / charsPerEstimatedToken))
}

func jsonNumberAsInt(v any) int {
	switch n := v.(type) {
	case float64:
		if n > 0 {
			return int(n)
		}
	case int:
		if n > 0 {
			return n
		}
	}
	return 0
}

func countTextInputChars(v any) int {
	switch x := v.(type) {
	case string:
		return utf8.RuneCountInString(x)
	case []any:
		total := 0
		for _, item := range x {
			total += countTextInputChars(item)
		}
		return total
	case map[string]any:
		if shouldSkipInputObject(x) {
			return 0
		}
		total := 0
		for k, val := range x {
			lk := strings.ToLower(k)
			switch lk {
			case "model", "stream", "max_tokens", "max_output_tokens", "temperature", "top_p", "n", "metadata", "tools", "tool_choice", "response_format", "stop":
				continue
			case "url", "image_url", "file_id", "filename", "mime_type":
				continue
			}
			total += countTextInputChars(val)
		}
		return total
	default:
		return 0
	}
}

func shouldSkipInputObject(m map[string]any) bool {
	if typ, _ := m["type"].(string); typ != "" {
		switch strings.ToLower(typ) {
		case "input_image", "image_url", "input_file", "file", "computer_screenshot":
			return true
		}
	}
	return false
}
