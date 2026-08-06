package app

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitForChannelRequestDelay(t *testing.T) {
	t.Run("waits configured duration", func(t *testing.T) {
		start := time.Now()
		if err := waitForChannelRequestDelay(context.Background(), 20*time.Millisecond); err != nil {
			t.Fatalf("waitForChannelRequestDelay() error = %v", err)
		}
		if elapsed := time.Since(start); elapsed < 15*time.Millisecond {
			t.Fatalf("elapsed = %v, want at least 15ms", elapsed)
		}
	})

	t.Run("stops when context is canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		start := time.Now()
		err := waitForChannelRequestDelay(ctx, time.Hour)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
		if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
			t.Fatalf("canceled wait took %v", elapsed)
		}
	})
}

func TestProxy_RequestDelayHonorsClientTimeout(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"unexpected"}}]}`))
	}))

	env := setupProxyTestEnv(t, []testChannel{{
		name:                "delayed-channel",
		models:              "gpt-delay",
		requestDelaySeconds: 1,
	}}, map[int]string{0: upstream.URL})

	start := time.Now()
	resp := doProxyRequest(t, env.engine, http.MethodPost, "/v1/chat/completions?timeout_ms=40", map[string]any{
		"model":    "gpt-delay",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}, nil)

	if resp.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusGatewayTimeout, resp.Body.String())
	}
	if upstreamCalls.Load() != 0 {
		t.Fatalf("upstream calls = %d, want 0", upstreamCalls.Load())
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("request took %v, delay did not stop on timeout", elapsed)
	}
}
