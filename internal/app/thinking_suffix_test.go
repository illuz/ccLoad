package app

import (
	"encoding/json"
	"testing"

	"ccLoad/internal/protocol"
)

func TestApplyThinkingSuffixWritesClientProtocolFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		proto     protocol.Protocol
		model     string
		body      string
		assertion func(*testing.T, map[string]any)
	}{
		{
			name:  "openai max becomes xhigh",
			proto: protocol.OpenAI,
			model: "gpt-5.6-luna(max)",
			body:  `{"model":"gpt-5.6-luna(max)","reasoning_effort":"low"}`,
			assertion: func(t *testing.T, body map[string]any) {
				if body["reasoning_effort"] != "xhigh" {
					t.Fatalf("reasoning_effort=%v, want xhigh", body["reasoning_effort"])
				}
			},
		},
		{
			name:  "openai auto removes unsupported enum",
			proto: protocol.OpenAI,
			model: "gpt-5.6-luna(auto)",
			body:  `{"model":"gpt-5.6-luna(auto)","reasoning_effort":"high"}`,
			assertion: func(t *testing.T, body map[string]any) {
				if _, exists := body["reasoning_effort"]; exists {
					t.Fatalf("reasoning_effort should be absent: %v", body)
				}
			},
		},
		{
			name:  "codex writes nested effort",
			proto: protocol.Codex,
			model: "gpt-5.6-luna(high)",
			body:  `{"model":"gpt-5.6-luna(high)","reasoning":{"summary":"auto","effort":"low"}}`,
			assertion: func(t *testing.T, body map[string]any) {
				reasoning, _ := body["reasoning"].(map[string]any)
				if reasoning["effort"] != "high" || reasoning["summary"] != "auto" {
					t.Fatalf("reasoning=%v, want preserved summary and high effort", reasoning)
				}
			},
		},
		{
			name:  "anthropic numeric budget",
			proto: protocol.Anthropic,
			model: "claude-sonnet(8192)",
			body:  `{"model":"claude-sonnet(8192)","messages":[],"output_config":{"effort":"low"}}`,
			assertion: func(t *testing.T, body map[string]any) {
				thinking, _ := body["thinking"].(map[string]any)
				if thinking["type"] != "enabled" || thinking["budget_tokens"] != float64(8192) {
					t.Fatalf("thinking=%v, want enabled budget 8192", thinking)
				}
				if _, exists := body["output_config"]; exists {
					t.Fatalf("output_config should be absent: %v", body)
				}
			},
		},
		{
			name:  "anthropic xhigh becomes max",
			proto: protocol.Anthropic,
			model: "claude-sonnet(xhigh)",
			body:  `{"model":"claude-sonnet(xhigh)","messages":[]}`,
			assertion: func(t *testing.T, body map[string]any) {
				thinking, _ := body["thinking"].(map[string]any)
				output, _ := body["output_config"].(map[string]any)
				if thinking["type"] != "adaptive" || output["effort"] != "max" {
					t.Fatalf("thinking/output=%v/%v", thinking, output)
				}
			},
		},
		{
			name:  "anthropic none removes thinking",
			proto: protocol.Anthropic,
			model: "claude-sonnet(none)",
			body:  `{"model":"claude-sonnet(none)","messages":[],"thinking":{"type":"adaptive"},"output_config":{"effort":"high"}}`,
			assertion: func(t *testing.T, body map[string]any) {
				if _, exists := body["thinking"]; exists {
					t.Fatalf("thinking should be absent: %v", body)
				}
				if _, exists := body["output_config"]; exists {
					t.Fatalf("output_config should be absent: %v", body)
				}
			},
		},
		{
			name:  "gemini xhigh becomes high",
			proto: protocol.Gemini,
			model: "gemini-3-pro(xhigh)",
			body:  `{"contents":[],"generationConfig":{"thinkingConfig":{"thinkingBudget":10,"includeThoughts":true}}}`,
			assertion: func(t *testing.T, body map[string]any) {
				generation, _ := body["generationConfig"].(map[string]any)
				thinking, _ := generation["thinkingConfig"].(map[string]any)
				if thinking["thinkingLevel"] != "high" || thinking["includeThoughts"] != true {
					t.Fatalf("thinkingConfig=%v, want high and preserved includeThoughts", thinking)
				}
				if _, exists := thinking["thinkingBudget"]; exists {
					t.Fatalf("thinkingBudget should be absent: %v", thinking)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := applyThinkingSuffix([]byte(tt.body), tt.proto, tt.model)
			var body map[string]any
			if err := json.Unmarshal(out, &body); err != nil {
				t.Fatalf("invalid output JSON: %v; body=%s", err, out)
			}
			tt.assertion(t, body)
		})
	}
}

func TestApplyThinkingSuffixPreservesLargeIntegersAndUnknownSuffixes(t *testing.T) {
	t.Parallel()

	body := []byte(`{"model":"gpt-5.6-luna(high)","seed":9223372036854775807}`)
	out := applyThinkingSuffix(body, protocol.OpenAI, "gpt-5.6-luna(high)")
	if string(out) == string(body) {
		t.Fatal("recognized suffix did not modify body")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["seed"]) != "9223372036854775807" {
		t.Fatalf("large integer changed: %s", raw["seed"])
	}

	unknown := []byte(`{"model":"vendor(beta)"}`)
	if got := applyThinkingSuffix(unknown, protocol.OpenAI, "vendor(beta)"); string(got) != string(unknown) {
		t.Fatalf("unknown suffix changed body: %s", got)
	}
}

func TestThinkingSuffixUpstreamModelAndPathUseBaseName(t *testing.T) {
	t.Parallel()

	body := replaceJSONRequestModel([]byte(`{"model":"gpt-5.6-luna(max)","seed":9223372036854775807}`), "gpt-5.6-luna")
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["model"]) != `"gpt-5.6-luna"` || string(raw["seed"]) != "9223372036854775807" {
		t.Fatalf("rewritten body=%s", body)
	}

	path := rewriteUpstreamRequestPath("/v1beta/models/gemini-3-pro(high):generateContent", "gemini-3-pro")
	if path != "/v1beta/models/gemini-3-pro:generateContent" {
		t.Fatalf("rewritten path=%q", path)
	}
}

func TestChannelTestLogIdentityStripsThinkingSuffix(t *testing.T) {
	t.Parallel()

	logModel, effort := channelTestLogIdentity("gpt-5.6-luna(max)", "low")
	if logModel != "gpt-5.6-luna" || effort != "max" {
		t.Fatalf("channelTestLogIdentity()=(%q,%q), want base model and max effort", logModel, effort)
	}
}
