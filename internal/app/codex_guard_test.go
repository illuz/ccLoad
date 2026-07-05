package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ccLoad/internal/cooldown"
	"ccLoad/internal/model"
	"ccLoad/internal/protocol"
	"ccLoad/internal/util"
)

func TestCodexReasoningGuard_NonStreamTokenSwitch(t *testing.T) {
	body := `{"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":2,"output_tokens_details":{"reasoning_tokens":516}}},"usage":{"input_tokens":1,"output_tokens":2,"output_tokens_details":{"reasoning_tokens":516}}}`

	t.Run("disabled forwards response", func(t *testing.T) {
		reqCtx := &requestContext{
			ctx:              context.Background(),
			startTime:        time.Now(),
			upstreamProtocol: protocol.Codex,
		}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}
		rec := newRecorder()
		res, _, err := (&Server{}).handleResponse(reqCtx, resp, rec, "codex", &model.Config{ID: 1}, "sk-test", nil)
		if err != nil {
			t.Fatalf("handleResponse error: %v", err)
		}
		if res.Status != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.Status)
		}
		if rec.Body.String() != body {
			t.Fatalf("body not forwarded when guard disabled: %q", rec.Body.String())
		}
	})

	t.Run("enabled blocks before client commit", func(t *testing.T) {
		reqCtx := &requestContext{
			ctx:               context.Background(),
			startTime:         time.Now(),
			upstreamProtocol:  protocol.Codex,
			codexGuardEnabled: true,
		}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}
		rec := newRecorder()
		res, _, err := (&Server{}).handleResponse(reqCtx, resp, rec, "codex", &model.Config{ID: 1}, "sk-test", nil)
		if err != nil {
			t.Fatalf("handleResponse error: %v", err)
		}
		if res.Status != util.StatusCodexReasoningGuard {
			t.Fatalf("status=%d, want %d", res.Status, util.StatusCodexReasoningGuard)
		}
		if res.ResponseCommitted {
			t.Fatal("guarded response must not be committed")
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("guarded body leaked to client: %q", rec.Body.String())
		}
		if res.ReasoningTokens != 516 || !strings.Contains(res.StreamDiagMsg, "reasoning_tokens=516") {
			t.Fatalf("unexpected guard result: reasoning=%d msg=%q", res.ReasoningTokens, res.StreamDiagMsg)
		}
	})
}

func TestCodexReasoningGuard_ReasoningFormula518NMinus2(t *testing.T) {
	tests := []struct {
		reasoning int
		want      bool
	}{
		{reasoning: 0, want: false},
		{reasoning: 515, want: false},
		{reasoning: 516, want: true},
		{reasoning: 517, want: false},
		{reasoning: 1034, want: true},
		{reasoning: 1552, want: true},
		{reasoning: 2070, want: true},
		{reasoning: 2588, want: true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("reasoning_%d", tt.reasoning), func(t *testing.T) {
			if got := codexGuardReasoningMatched(tt.reasoning); got != tt.want {
				t.Fatalf("codexGuardReasoningMatched(%d)=%v, want %v", tt.reasoning, got, tt.want)
			}
		})
	}
}

func TestCodexReasoningGuard_StreamStrictBuffer(t *testing.T) {
	body := "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":7,\"output_tokens\":4,\"output_tokens_details\":{\"reasoning_tokens\":516},\"total_tokens\":11}}}\n\n"

	reqCtx := &requestContext{
		ctx:               context.Background(),
		startTime:         time.Now(),
		isStreaming:       true,
		upstreamProtocol:  protocol.Codex,
		codexGuardEnabled: true,
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	rec := newRecorder()
	res, _, err := (&Server{}).handleResponse(reqCtx, resp, rec, "codex", &model.Config{ID: 1}, "sk-test", nil)
	if err != nil {
		t.Fatalf("handleResponse error: %v", err)
	}
	if res.Status != util.StatusCodexReasoningGuard {
		t.Fatalf("status=%d, want %d", res.Status, util.StatusCodexReasoningGuard)
	}
	if res.ResponseCommitted {
		t.Fatal("guarded stream must not be committed")
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("guarded stream leaked to client: %q", rec.Body.String())
	}
}

func TestCodexReasoningGuard_RetriesNextKeyBeforeClientCommit(t *testing.T) {
	srv := newInMemoryServer(t)
	srv.maxKeyRetries = 2

	var attempts atomic.Int32
	var authHeaders []string
	upstream := newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch attempts.Add(1) {
		case 1:
			_, _ = w.Write([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":2,"output_tokens_details":{"reasoning_tokens":516}}},"usage":{"input_tokens":1,"output_tokens":2,"output_tokens_details":{"reasoning_tokens":516}}}`))
		default:
			_, _ = w.Write([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":4,"output_tokens_details":{"reasoning_tokens":0}}},"usage":{"input_tokens":3,"output_tokens":4,"output_tokens_details":{"reasoning_tokens":0}}}`))
		}
	}))
	srv.client = upstream.Client()

	req := ChannelRequest{
		Name:        "codex-guard-retry",
		APIKey:      "sk-guarded,sk-good",
		ChannelType: "codex",
		URL:         upstream.URL,
		Models:      []model.ModelEntry{{Model: "gpt-5-codex"}},
		Enabled:     true,
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate channel request: %v", err)
	}
	cfg, err := srv.createChannelFromRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("createChannelFromRequest: %v", err)
	}

	rec := newRecorder()
	body := []byte(`{"model":"gpt-5-codex","input":"hi","stream":false}`)
	res, err := srv.tryChannelWithKeys(context.Background(), cfg, &proxyRequestContext{
		originalModel:     "gpt-5-codex",
		clientProtocol:    protocol.Codex,
		requestMethod:     http.MethodPost,
		requestPath:       "/v1/responses",
		body:              body,
		translatedBody:    body,
		header:            http.Header{"Content-Type": []string{"application/json"}},
		startTime:         time.Now(),
		codexGuardEnabled: true,
	}, rec)
	if err != nil {
		t.Fatalf("tryChannelWithKeys error: %v", err)
	}
	if res == nil || !res.succeeded || res.status != http.StatusOK {
		t.Fatalf("result=%+v, want successful 200", res)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("attempts=%d, want 2", got)
	}
	if len(authHeaders) != 2 || authHeaders[0] != "Bearer sk-guarded" || authHeaders[1] != "Bearer sk-good" {
		t.Fatalf("auth headers=%v, want first guarded key then good key", authHeaders)
	}
	bodyText := rec.Body.String()
	if strings.Contains(bodyText, `"reasoning_tokens":516`) {
		t.Fatalf("guarded first response leaked to client: %s", bodyText)
	}
	if !strings.Contains(bodyText, `"reasoning_tokens":0`) {
		t.Fatalf("final client body=%s, want second successful response", bodyText)
	}
}

func TestCodexReasoningGuard_DoesNotWriteCooldown(t *testing.T) {
	srv := newInMemoryServer(t)
	srv.maxKeyRetries = 1

	upstream := newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":2,"output_tokens_details":{"reasoning_tokens":516}}},"usage":{"input_tokens":1,"output_tokens":2,"output_tokens_details":{"reasoning_tokens":516}}}`))
	}))
	srv.client = upstream.Client()

	req := ChannelRequest{
		Name:        "codex-guard-no-cooldown",
		APIKey:      "sk-single",
		ChannelType: "codex",
		URL:         upstream.URL,
		Models:      []model.ModelEntry{{Model: "gpt-5-codex"}},
		Enabled:     true,
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate channel request: %v", err)
	}
	cfg, err := srv.createChannelFromRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("createChannelFromRequest: %v", err)
	}

	body := []byte(`{"model":"gpt-5-codex","input":"hi","stream":false}`)
	res, err := srv.tryChannelWithKeys(context.Background(), cfg, &proxyRequestContext{
		originalModel:     "gpt-5-codex",
		clientProtocol:    protocol.Codex,
		requestMethod:     http.MethodPost,
		requestPath:       "/v1/responses",
		body:              body,
		translatedBody:    body,
		header:            http.Header{"Content-Type": []string{"application/json"}},
		startTime:         time.Now(),
		codexGuardEnabled: true,
	}, newRecorder())
	if err != nil {
		t.Fatalf("tryChannelWithKeys error: %v", err)
	}
	if res == nil || res.status != util.StatusCodexReasoningGuard || res.nextAction != cooldown.ActionRetryKey {
		t.Fatalf("result=%+v, want guard failure with ActionRetryKey", res)
	}

	channelCooldowns, err := srv.store.GetAllChannelCooldowns(context.Background())
	if err != nil {
		t.Fatalf("GetAllChannelCooldowns failed: %v", err)
	}
	if len(channelCooldowns) != 0 {
		t.Fatalf("channel cooldowns=%v, want none", channelCooldowns)
	}

	keyCooldowns, err := srv.store.GetAllKeyCooldowns(context.Background())
	if err != nil {
		t.Fatalf("GetAllKeyCooldowns failed: %v", err)
	}
	if len(keyCooldowns) != 0 {
		t.Fatalf("key cooldowns=%v, want none", keyCooldowns)
	}
}
