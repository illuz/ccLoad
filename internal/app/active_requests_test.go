package app

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"ccLoad/internal/cooldown"
	"ccLoad/internal/model"
	"ccLoad/internal/protocol"
	"ccLoad/internal/util"
)

func TestActiveRequestManager_ListSnapshotAndSort(t *testing.T) {
	m := newActiveRequestManager()

	id1 := m.Register(time.UnixMilli(100), "m1", "1.1.1.1", false)
	id2 := m.Register(time.UnixMilli(200), "m2", "2.2.2.2", true)

	got := m.List()
	if len(got) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(got))
	}
	if got[0].ID != id2 || got[1].ID != id1 {
		t.Fatalf("expected order [%d,%d], got [%d,%d]", id2, id1, got[0].ID, got[1].ID)
	}

	// List() 必须返回快照：改返回值不应影响内部状态
	got[0].Model = "hacked"
	got2 := m.List()
	if got2[0].Model != "m2" {
		t.Fatalf("expected snapshot copy, got model=%q", got2[0].Model)
	}
}

func TestActiveRequestManager_UpdateMasksKey(t *testing.T) {
	m := newActiveRequestManager()

	startTime := time.UnixMilli(100)
	id := m.Register(startTime, "m", "1.1.1.1", false)
	rawKey := "sk-1234567890abcdef"
	m.Update(id, 1, "ch", "anthropic", rawKey, 0, 1.0)

	got := m.List()
	if len(got) != 1 {
		t.Fatalf("expected 1 request, got %d", len(got))
	}
	if got[0].APIKeyUsed == rawKey {
		t.Fatalf("expected masked key, got raw")
	}
	if got[0].APIKeyUsed != "****" && !strings.Contains(got[0].APIKeyUsed, ".") {
		t.Fatalf("expected masked key format, got %q", got[0].APIKeyUsed)
	}
	if got[0].StartTime != startTime.UnixMilli() {
		t.Fatalf("start_time=%d, want original request start %d", got[0].StartTime, startTime.UnixMilli())
	}
}

func TestActiveRequestManager_BytesAndFirstByteTime(t *testing.T) {
	m := newActiveRequestManager()

	id := m.Register(time.UnixMilli(100), "m", "1.1.1.1", true)

	m.AddBytes(id, 10)
	m.AddBytes(id, 0) // no-op

	m.SetClientFirstByteTime(id, -1*time.Second)        // must not poison the value
	m.SetClientFirstByteTime(id, 750*time.Millisecond)  // first set wins
	m.SetClientFirstByteTime(id, 1250*time.Millisecond) // ignored

	got := m.List()
	if len(got) != 1 {
		t.Fatalf("expected 1 request, got %d", len(got))
	}
	if got[0].BytesReceived != 10 {
		t.Fatalf("expected bytes_received=10, got %d", got[0].BytesReceived)
	}
	if math.Abs(got[0].ClientFirstByteTime-0.75) > 1e-6 {
		t.Fatalf("expected client_first_byte_time≈0.75, got %f", got[0].ClientFirstByteTime)
	}
}

func TestActiveRequestManager_RequestFailoverCancelsCurrentAttempt(t *testing.T) {
	m := newActiveRequestManager()
	id := m.Register(time.Now(), "m", "1.1.1.1", true)

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	clear := m.StartAttempt(id, cancel)
	defer clear()

	if got := m.List()[0].CanFailover; !got {
		t.Fatal("CanFailover=false, want true before response starts")
	}
	if err := m.RequestFailover(id); err != nil {
		t.Fatalf("RequestFailover: %v", err)
	}
	if !errors.Is(context.Cause(ctx), util.ErrManualFailover) {
		t.Fatalf("context cause=%v, want ErrManualFailover", context.Cause(ctx))
	}
	if got := m.List()[0].CanFailover; got {
		t.Fatal("CanFailover=true, want false after failover was requested")
	}
}

func TestActiveRequestManager_RequestFailoverRejectsStartedResponse(t *testing.T) {
	m := newActiveRequestManager()
	id := m.Register(time.Now(), "m", "1.1.1.1", true)
	_, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	m.StartAttempt(id, cancel)
	m.AddBytes(id, 1)

	err := m.RequestFailover(id)
	if !errors.Is(err, errActiveRequestNotFailoverable) {
		t.Fatalf("RequestFailover error=%v, want errActiveRequestNotFailoverable", err)
	}
}

func TestActiveRequestManager_RequestFailoverRejectsCommittedResponse(t *testing.T) {
	m := newActiveRequestManager()
	id := m.Register(time.Now(), "m", "1.1.1.1", false)
	_, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	m.StartAttempt(id, cancel)
	if err := m.TryCommitResponse(id); err != nil {
		t.Fatalf("TryCommitResponse: %v", err)
	}

	err := m.RequestFailover(id)
	if !errors.Is(err, errActiveRequestNotFailoverable) {
		t.Fatalf("RequestFailover error=%v, want errActiveRequestNotFailoverable", err)
	}
}

func TestForwardAttempt_ManualFailoverAllowsNextChannel(t *testing.T) {
	srv := newInMemoryServer(t)

	started := make(chan struct{})
	slowUpstream := newTestHTTPServer(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	goodUpstream := newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"fallback"}`))
	}))

	activeID := srv.activeRequests.Register(time.Now(), "gpt-test", "1.1.1.1", false)
	reqCtx := &proxyRequestContext{
		originalModel:  "gpt-test",
		clientProtocol: protocol.OpenAI,
		requestMethod:  http.MethodPost,
		requestPath:    "/v1/chat/completions",
		body:           []byte(`{"model":"gpt-test","messages":[]}`),
		header:         http.Header{"Content-Type": []string{"application/json"}},
		activeReqID:    activeID,
		startTime:      time.Now(),
	}
	slowCfg := &model.Config{ID: 101, Name: "slow", URL: slowUpstream.URL, ChannelType: util.ChannelTypeOpenAI}
	goodCfg := &model.Config{ID: 102, Name: "good", URL: goodUpstream.URL, ChannelType: util.ChannelTypeOpenAI}

	type attemptOutcome struct {
		result *proxyResult
		action cooldown.Action
		err    error
	}
	firstDone := make(chan attemptOutcome, 1)
	go func() {
		result, action, err := srv.forwardAttempt(context.Background(), slowCfg, 0, "sk-test", reqCtx,
			"gpt-test", reqCtx.body, reqCtx.requestPath, slowCfg.URL, newRecorder(), false)
		firstDone <- attemptOutcome{result: result, action: action, err: err}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("slow upstream was not called")
	}
	if err := srv.activeRequests.RequestFailover(activeID); err != nil {
		t.Fatalf("RequestFailover: %v", err)
	}

	select {
	case outcome := <-firstDone:
		if outcome.err != nil {
			t.Fatalf("slow forwardAttempt error: %v", outcome.err)
		}
		if outcome.result == nil || outcome.result.status != http.StatusBadGateway {
			t.Fatalf("slow result=%+v, want 502 failure", outcome.result)
		}
		if outcome.action != cooldown.ActionRetryChannel {
			t.Fatalf("slow action=%v, want ActionRetryChannel", outcome.action)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("manual failover did not stop the slow upstream")
	}

	result, action, err := srv.forwardAttempt(context.Background(), goodCfg, 0, "sk-test", reqCtx,
		"gpt-test", reqCtx.body, reqCtx.requestPath, goodCfg.URL, newRecorder(), false)
	if err != nil {
		t.Fatalf("fallback forwardAttempt error: %v", err)
	}
	if result == nil || !result.succeeded || action != cooldown.ActionReturnClient {
		t.Fatalf("fallback result=%+v action=%v, want successful next-channel attempt", result, action)
	}
}
