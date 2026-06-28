package testutil

import (
	"regexp"
	"strings"
	"testing"

	"ccLoad/internal/model"

	"github.com/bytedance/sonic"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestOpenAITesterBuild_ExactURLMarkerSkipsEndpointPath(t *testing.T) {
	cfg := &model.Config{URL: "https://api.example.com/custom/chat#"}
	req := &TestChannelRequest{Model: "gpt-test", Content: "hello"}

	fullURL, _, _, err := (&OpenAITester{}).Build(cfg, "sk-test", req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if fullURL != "https://api.example.com/custom/chat" {
		t.Fatalf("fullURL = %q, want %q", fullURL, "https://api.example.com/custom/chat")
	}
	if strings.Contains(fullURL, "/v1/chat/completions") {
		t.Fatalf("fullURL should not append OpenAI endpoint path: %q", fullURL)
	}
}

func TestOpenAITesterBuild_AddsSessionIDHeader(t *testing.T) {
	cfg := &model.Config{URL: "https://api.example.com"}
	req := &TestChannelRequest{Model: "gpt-test", Content: "hello"}

	_, headers, body, err := (&OpenAITester{}).Build(cfg, "sk-test", req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	sessionID := headers.Get("Session_id")
	if !uuidPattern.MatchString(sessionID) {
		t.Fatalf("Session_id header missing or invalid: %q", sessionID)
	}
	if got := headers.Get("Session-Id"); got != "" {
		t.Fatalf("Session-Id header should be omitted, got %q", got)
	}

	var payload map[string]any
	if err := sonic.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body failed: %v; body=%s", err, body)
	}
	if got, _ := payload["user"].(string); got != sessionID {
		t.Fatalf("body user = %q, want session id %q; body=%s", got, sessionID, body)
	}
	if got, _ := payload["prompt_cache_key"].(string); got != sessionID {
		t.Fatalf("body prompt_cache_key = %q, want session id %q; body=%s", got, sessionID, body)
	}
}

func TestOpenAITesterBuild_AppliesThinkingEffortAndWebSearchOptions(t *testing.T) {
	cfg := &model.Config{URL: "https://api.example.com"}
	req := &TestChannelRequest{
		Model:          "gpt-test",
		Content:        "hello",
		ThinkingEffort: "high",
		BuiltinSearch:  true,
	}

	_, _, body, err := (&OpenAITester{}).Build(cfg, "sk-test", req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	var payload map[string]any
	if err := sonic.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body failed: %v; body=%s", err, body)
	}
	if got, _ := payload["reasoning_effort"].(string); got != "high" {
		t.Fatalf("reasoning_effort = %q, want high; body=%s", got, body)
	}
	searchOptions, ok := payload["web_search_options"].(map[string]any)
	if !ok {
		t.Fatalf("web_search_options missing or invalid; body=%s", body)
	}
	if len(searchOptions) != 0 {
		t.Fatalf("web_search_options = %#v, want empty object; body=%s", searchOptions, body)
	}
	if _, ok := payload["tools"]; ok {
		t.Fatalf("OpenAI chat search must use web_search_options, not tools; body=%s", body)
	}
}

func TestOpenAITesterBuild_AppliesSamplingAndSystemPrompt(t *testing.T) {
	cfg := &model.Config{URL: "https://api.example.com"}
	temperature := 0.7
	topP := 0.9
	req := &TestChannelRequest{
		Model:        "gpt-test",
		Content:      "hello",
		SystemPrompt: "answer tersely",
		Temperature:  &temperature,
		TopP:         &topP,
		MaxTokens:    2048,
	}

	_, _, body, err := (&OpenAITester{}).Build(cfg, "sk-test", req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	var payload map[string]any
	if err := sonic.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body failed: %v; body=%s", err, body)
	}
	if got, _ := payload["temperature"].(float64); got != 0.7 {
		t.Fatalf("temperature = %v, want 0.7; body=%s", got, body)
	}
	if got, _ := payload["top_p"].(float64); got != 0.9 {
		t.Fatalf("top_p = %v, want 0.9; body=%s", got, body)
	}
	if got, _ := payload["max_tokens"].(float64); got != 2048 {
		t.Fatalf("max_tokens = %v, want 2048; body=%s", got, body)
	}
	messages, ok := payload["messages"].([]any)
	if !ok || len(messages) < 2 {
		t.Fatalf("messages invalid: %#v; body=%s", payload["messages"], body)
	}
	system, ok := messages[0].(map[string]any)
	if !ok || system["role"] != "system" || system["content"] != "answer tersely" {
		t.Fatalf("first message should be system prompt, got %#v; body=%s", messages[0], body)
	}
}

func TestOpenAITesterBuild_SupportsStructuredImageMessages(t *testing.T) {
	cfg := &model.Config{URL: "https://api.example.com"}
	req := &TestChannelRequest{
		Model: "gpt-test",
		Messages: []ChatMessage{
			{
				Role: "user",
				ContentBlocks: []chatContentBlock{
					{Type: "text", Text: "describe"},
					{Type: "image_url", ImageURL: &chatImageURL{URL: "data:image/png;base64,aW1n"}},
				},
			},
		},
	}

	_, _, body, err := (&OpenAITester{}).Build(cfg, "sk-test", req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	bodyText := string(body)
	if !strings.Contains(bodyText, `"type":"text"`) || !strings.Contains(bodyText, `"text":"describe"`) {
		t.Fatalf("openai body missing text block: %s", bodyText)
	}
	if !strings.Contains(bodyText, `"type":"image_url"`) || !strings.Contains(bodyText, `"url":"data:image/png;base64,aW1n"`) {
		t.Fatalf("openai body missing image_url block: %s", bodyText)
	}
}

func TestCodexTesterBuild_UsesCurrentCodexClientHeaders(t *testing.T) {
	cfg := &model.Config{URL: "https://api.example.com"}
	req := &TestChannelRequest{Model: "gpt-5.5", Content: "hello", Stream: true}

	fullURL, headers, _, err := (&CodexTester{}).Build(cfg, "sk-test", req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if fullURL != "https://api.example.com/v1/responses" {
		t.Fatalf("fullURL = %q, want %q", fullURL, "https://api.example.com/v1/responses")
	}
	if got := headers.Get("Authorization"); got != "Bearer sk-test" {
		t.Fatalf("Authorization = %q, want Bearer sk-test", got)
	}
	if got := headers.Get("X-Api-Key"); got != "sk-test" {
		t.Fatalf("X-Api-Key = %q, want sk-test", got)
	}
	if got := headers.Get("Originator"); got != "codex-tui" {
		t.Fatalf("Originator = %q, want codex-tui", got)
	}
	sessionID := headers.Get("Session-Id")
	if !uuidPattern.MatchString(sessionID) {
		t.Fatalf("Session-Id header missing or invalid: %q", sessionID)
	}
	if got := headers.Get("Thread-Id"); got != sessionID {
		t.Fatalf("Thread-Id = %q, want session id %q", got, sessionID)
	}
	if got := headers.Get("X-Client-Request-Id"); got != sessionID {
		t.Fatalf("X-Client-Request-Id = %q, want session id %q", got, sessionID)
	}
	if got := headers.Get("X-Codex-Window-Id"); got != sessionID+":0" {
		t.Fatalf("X-Codex-Window-Id = %q, want %q", got, sessionID+":0")
	}
	if got := headers.Get("X-Codex-Beta-Features"); got != "terminal_resize_reflow" {
		t.Fatalf("X-Codex-Beta-Features = %q, want terminal_resize_reflow", got)
	}
	if got := headers.Get("X-Codex-Turn-Metadata"); !strings.Contains(got, sessionID) {
		t.Fatalf("X-Codex-Turn-Metadata should contain session id %q, got %q", sessionID, got)
	}
	if got := headers.Get("Openai-Beta"); got != "" {
		t.Fatalf("Openai-Beta header should be omitted, got %q", got)
	}
	if got := headers.Get("User-Agent"); !strings.HasPrefix(got, "codex-tui/0.137.0 ") {
		t.Fatalf("User-Agent = %q, want codex-tui/0.137.0 prefix", got)
	}
	if got := headers.Get("Accept"); got != "text/event-stream" {
		t.Fatalf("Accept = %q, want text/event-stream", got)
	}
}

func TestCodexTesterBuild_UsesCurrentCodexClientBodyShape(t *testing.T) {
	cfg := &model.Config{URL: "https://api.example.com"}
	req := &TestChannelRequest{Model: "gpt-5.5", Content: "hello", Stream: true}

	_, headers, body, err := (&CodexTester{}).Build(cfg, "sk-test", req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	var payload map[string]any
	if err := sonic.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body failed: %v; body=%s", err, body)
	}

	sessionID := headers.Get("Session-Id")
	if got, _ := payload["prompt_cache_key"].(string); got != sessionID {
		t.Fatalf("prompt_cache_key = %q, want session id %q; body=%s", got, sessionID, body)
	}
	if got, _ := payload["tool_choice"].(string); got != "auto" {
		t.Fatalf("tool_choice = %q, want auto; body=%s", got, body)
	}
	if got, _ := payload["parallel_tool_calls"].(bool); !got {
		t.Fatalf("parallel_tool_calls = %v, want true; body=%s", got, body)
	}

	reasoning, ok := payload["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("reasoning missing or invalid; body=%s", body)
	}
	if got, _ := reasoning["effort"].(string); got != "low" {
		t.Fatalf("reasoning.effort = %q, want low; body=%s", got, body)
	}

	textConfig, ok := payload["text"].(map[string]any)
	if !ok {
		t.Fatalf("text missing or invalid; body=%s", body)
	}
	if got, _ := textConfig["verbosity"].(string); got != "low" {
		t.Fatalf("text.verbosity = %q, want low; body=%s", got, body)
	}

	clientMetadata, ok := payload["client_metadata"].(map[string]any)
	if !ok {
		t.Fatalf("client_metadata missing or invalid; body=%s", body)
	}
	if got, _ := clientMetadata["x-codex-installation-id"].(string); !uuidPattern.MatchString(got) {
		t.Fatalf("x-codex-installation-id missing or invalid: %q; body=%s", got, body)
	}

	tools, ok := payload["tools"].([]any)
	if !ok {
		t.Fatalf("tools missing or invalid; body=%s", body)
	}
	if len(tools) != 0 {
		t.Fatalf("tools length = %d, want 0; body=%s", len(tools), body)
	}
}

func TestAnthropicTesterBuild_ExactURLMarkerSkipsEndpointPath(t *testing.T) {
	cfg := &model.Config{URL: "https://api.example.com/custom/messages#"}
	req := &TestChannelRequest{Model: "claude-test", Content: "hello"}

	fullURL, _, _, err := (&AnthropicTester{}).Build(cfg, "sk-test", req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if fullURL != "https://api.example.com/custom/messages" {
		t.Fatalf("fullURL = %q, want %q", fullURL, "https://api.example.com/custom/messages")
	}
	if strings.Contains(fullURL, "/v1/messages") {
		t.Fatalf("fullURL should not append Anthropic endpoint path: %q", fullURL)
	}
}
