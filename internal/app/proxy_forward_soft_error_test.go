package app

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"ccLoad/internal/model"
)

func TestCheckSoftError(t *testing.T) {
	t.Parallel()
	prefixes := parseSoftErrorTextPrefixes(defaultSoftErrorTextPrefixes)

	tests := []struct {
		name        string
		contentType string
		data        []byte
		want        bool
	}{
		{
			name:        "json_top_level_type_error",
			contentType: "application/json; charset=utf-8",
			data:        []byte(`{"type":"error","message":"boom"}`),
			want:        true,
		},
		{
			name:        "json_top_level_error_field",
			contentType: "application/json",
			data:        []byte(`{"error":{"message":"boom"}}`),
			want:        true,
		},
		{
			name:        "json_success_contains_keywords_should_not_match",
			contentType: "application/json",
			data:        []byte(`{"type":"message","content":"api_error 当前模型负载过高"}`),
			want:        false,
		},
		{
			name:        "json_truncated_object_should_not_guess",
			contentType: "application/json",
			data:        []byte(`{"type":"error"`),
			want:        false,
		},
		{
			name:        "json_content_type_but_plain_text_prefix_can_match",
			contentType: "application/json",
			data:        []byte("当前模型负载过高，请稍后再试"),
			want:        true,
		},
		{
			name:        "text_plain_prefix_short_match",
			contentType: "text/plain; charset=utf-8",
			data:        []byte("当前模型负载过高，请稍后再试"),
			want:        true,
		},
		{
			name:        "text_plain_contains_but_not_prefix_should_not_match",
			contentType: "text/plain",
			data:        []byte("回答里提到 当前模型负载过高 但这不是错误"),
			want:        false,
		},
		{
			name:        "text_plain_public_key_client_restriction",
			contentType: "text/plain; charset=utf-8",
			data:        []byte("本公益key仅支持在AI编程客户端使用"),
			want:        true,
		},
		{
			name:        "text_plain_sse_should_not_match",
			contentType: "text/plain",
			data:        []byte("data: {\"type\":\"message\"}\n\n"),
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := checkSoftError(tt.data, tt.contentType, prefixes); got != tt.want {
				t.Fatalf("checkSoftError()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldCheckSoftErrorForChannelType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		channelType string
		want        bool
	}{
		{name: "anthropic", channelType: "anthropic", want: true},
		{name: "codex", channelType: "codex", want: true},
		{name: "anthropic_default_empty", channelType: "", want: true},
		{name: "openai", channelType: "openai", want: false},
		{name: "gemini", channelType: "gemini", want: false},
		{name: "unknown", channelType: "something", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldCheckSoftErrorForChannelType(tt.channelType); got != tt.want {
				t.Fatalf("shouldCheckSoftErrorForChannelType()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldProbeSoftErrorUsesConfigChannelTypeFallback(t *testing.T) {
	t.Parallel()

	reqCtx := &requestContext{isStreaming: false}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
	}
	cfg := &model.Config{ChannelType: "codex"}

	if !shouldProbeSoftError(reqCtx, resp, cfg, "openai") {
		t.Fatal("shouldProbeSoftError()=false, want true for OpenAI entrypoint on Codex channel")
	}
}

func TestProbeSoftErrorResponseReadsPastPrependedFirstByte(t *testing.T) {
	t.Parallel()

	body := []byte("本公益key仅支持在AI编程客户端使用")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
		Body:       io.NopCloser(bytes.NewReader(body[1:])),
	}
	prependToBody(resp, body[:1])

	reqCtx := &requestContext{ctx: t.Context(), isStreaming: false}
	cfg := &model.Config{ID: 53, ChannelType: "codex"}
	rec := httptest.NewRecorder()

	handled, res, _, err := (&Server{}).probeSoftErrorResponse(reqCtx, resp, resp.Header.Clone(), cfg, "openai", &streamReadStats{})
	if err != nil {
		t.Fatalf("probeSoftErrorResponse() err=%v", err)
	}
	if !handled {
		t.Fatal("probeSoftErrorResponse() handled=false, want true")
	}
	if res == nil || res.Status != 597 {
		t.Fatalf("result status=%v, want 597", res)
	}
	_ = rec
}
