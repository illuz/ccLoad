package app

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"ccLoad/internal/util"

	"github.com/gin-gonic/gin"
)

func TestHandleActiveRequests(t *testing.T) {
	t.Parallel()

	m := newActiveRequestManager()
	id := m.Register(time.Now(), "m1", "1.2.3.4", true)
	m.Update(id, 10, "ch", "openai", "sk-test", 7, 1.5) //nolint:gosec // 测试用假凭证
	m.AddBytes(id, 123)
	m.SetClientFirstByteTime(id, 50*time.Millisecond)

	s := &Server{activeRequests: m}

	c, w := newTestContext(t, newRequest(http.MethodGet, "/admin/active_requests", nil))

	s.HandleActiveRequests(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Success bool            `json:"success"`
		Data    []ActiveRequest `json:"data"`
		Count   int             `json:"count"`
	}
	mustUnmarshalJSON(t, w.Body.Bytes(), &resp)
	if !resp.Success || resp.Count != 1 || len(resp.Data) != 1 {
		t.Fatalf("unexpected resp: %+v", resp)
	}
	if resp.Data[0].BytesReceived != 123 {
		t.Fatalf("bytes_received=%d, want 123", resp.Data[0].BytesReceived)
	}
	if resp.Data[0].ClientFirstByteTime <= 0 {
		t.Fatalf("client_first_byte_time=%v, want >0", resp.Data[0].ClientFirstByteTime)
	}
	if resp.Data[0].CostMultiplier != 1.5 {
		t.Fatalf("cost_multiplier=%v, want 1.5", resp.Data[0].CostMultiplier)
	}
}

func TestHandleActiveRequests_PreservesZeroCostMultiplier(t *testing.T) {
	t.Parallel()

	m := newActiveRequestManager()
	id := m.Register(time.Now(), "m1", "1.2.3.4", true)
	m.Update(id, 10, "free-channel", "openai", "sk-test", 7, 0) //nolint:gosec // 测试用假凭证

	s := &Server{activeRequests: m}

	c, w := newTestContext(t, newRequest(http.MethodGet, "/admin/active_requests", nil))

	s.HandleActiveRequests(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Success bool             `json:"success"`
		Data    []map[string]any `json:"data"`
		Count   int              `json:"count"`
	}
	mustUnmarshalJSON(t, w.Body.Bytes(), &resp)
	if !resp.Success || resp.Count != 1 || len(resp.Data) != 1 {
		t.Fatalf("unexpected resp: %+v", resp)
	}

	value, ok := resp.Data[0]["cost_multiplier"]
	if !ok {
		t.Fatalf("cost_multiplier missing in response: %+v", resp.Data[0])
	}
	if got, ok := value.(float64); !ok || got != 0 {
		t.Fatalf("cost_multiplier=%v, want 0", value)
	}
}

func TestHandleFailoverActiveRequest(t *testing.T) {
	m := newActiveRequestManager()
	id := m.Register(time.Now(), "m1", "1.2.3.4", true)
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	m.StartAttempt(id, cancel)

	s := &Server{activeRequests: m}
	c, w := newTestContext(t, newRequest(http.MethodPost, "/admin/active-requests/1/failover", nil))
	c.Params = gin.Params{{Key: "request_id", Value: strconv.FormatInt(id, 10)}}

	s.HandleFailoverActiveRequest(c)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status=%d, want %d; body=%s", w.Code, http.StatusAccepted, w.Body.String())
	}
	if !errors.Is(context.Cause(ctx), util.ErrManualFailover) {
		t.Fatalf("context cause=%v, want ErrManualFailover", context.Cause(ctx))
	}
}

func TestHandleFailoverActiveRequest_RejectsStartedResponse(t *testing.T) {
	m := newActiveRequestManager()
	id := m.Register(time.Now(), "m1", "1.2.3.4", true)
	_, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	m.StartAttempt(id, cancel)
	m.AddBytes(id, 1)

	s := &Server{activeRequests: m}
	c, w := newTestContext(t, newRequest(http.MethodPost, "/admin/active-requests/1/failover", nil))
	c.Params = gin.Params{{Key: "request_id", Value: strconv.FormatInt(id, 10)}}

	s.HandleFailoverActiveRequest(c)

	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d, want %d; body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
}
