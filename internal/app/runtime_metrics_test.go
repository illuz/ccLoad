package app

import (
	"net/http"
	"testing"
	"time"
)

func TestHTTPProxyRuntimeMetrics(t *testing.T) {
	t.Parallel()

	var metrics httpProxyRuntimeMetrics
	request := metrics.begin()
	if got := metrics.stats().ActiveRequests; got != 1 {
		t.Fatalf("active requests=%d, want 1", got)
	}
	request.observeRequest(true, 12)
	request.finish(http.StatusOK, 34)

	clientError := metrics.begin()
	clientError.observeRequest(false, 5)
	clientError.finish(http.StatusBadRequest, 7)

	serverError := metrics.begin()
	serverError.finish(http.StatusBadGateway, -1)

	got := metrics.stats()
	if got.ActiveRequests != 0 || got.CompletedRequests != 3 {
		t.Fatalf("request counts=%+v", got)
	}
	if got.NonErrorResponses != 1 || got.ClientErrorResponses != 1 || got.ServerErrorResponses != 1 {
		t.Fatalf("response status counts=%+v", got)
	}
	if got.StreamingRequests != 1 || got.NonStreamingRequests != 1 {
		t.Fatalf("stream counts=%+v", got)
	}
	if got.RequestBodyBytes != 17 || got.ResponseBodyBytes != 41 {
		t.Fatalf("byte counts=%+v", got)
	}
}

func TestCPUUsageTrackerPercent(t *testing.T) {
	t.Parallel()

	base := time.Unix(1_700_000_000, 0)
	tracker := &cpuUsageTracker{}
	if got := tracker.percent(30, base, 120); got != 25 {
		t.Fatalf("initial CPU percent=%v, want 25", got)
	}
	if got := tracker.percent(31, base.Add(500*time.Millisecond), 120.5); got != 25 {
		t.Fatalf("short-window CPU percent=%v, want 25", got)
	}
	if got := tracker.percent(35, base.Add(10*time.Second), 130); got != 50 {
		t.Fatalf("sampled CPU percent=%v, want 50", got)
	}
	if got := tracker.percent(30, base.Add(20*time.Second), 140); got != 0 {
		t.Fatalf("negative CPU delta=%v, want 0", got)
	}
}

func TestParseStatmResidentBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		statm    string
		pageSize int
		want     uint64
	}{
		{name: "valid", statm: "12345 678 90\n", pageSize: 4096, want: 678 * 4096},
		{name: "missing resident", statm: "12345", pageSize: 4096},
		{name: "invalid resident", statm: "12345 nope", pageSize: 4096},
		{name: "invalid page size", statm: "12345 678", pageSize: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseStatmResidentBytes(tc.statm, tc.pageSize); got != tc.want {
				t.Fatalf("parseStatmResidentBytes()=%d, want %d", got, tc.want)
			}
		})
	}
}

func TestHandleRuntimeMetrics(t *testing.T) {
	t.Parallel()

	s := &Server{
		startedAt:      time.Now().Add(-2 * time.Minute),
		maxConcurrency: 3,
		concurrencySem: make(chan struct{}, 3),
	}
	s.concurrencySem <- struct{}{}

	c, w := newTestContext(t, newRequest(http.MethodGet, "/admin/runtime-metrics", nil))
	s.HandleRuntimeMetrics(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}

	resp := mustParseAPIResponse[map[string]any](t, w.Body.Bytes())
	process, ok := resp.Data["process"].(map[string]any)
	if !ok {
		t.Fatalf("process metrics missing: %#v", resp.Data)
	}
	if process["concurrency_slots_in_use"] != float64(1) || process["max_concurrency"] != float64(3) {
		t.Fatalf("unexpected concurrency metrics: %#v", process)
	}
	if uptime, ok := process["uptime_seconds"].(float64); !ok || uptime < 119 {
		t.Fatalf("unexpected uptime: %#v", process["uptime_seconds"])
	}
	for _, key := range []string{"goroutines", "heap_alloc_bytes", "heap_sys_bytes"} {
		if value, ok := process[key].(float64); !ok || value <= 0 {
			t.Fatalf("%s=%v, want positive", key, process[key])
		}
	}
	if _, ok := resp.Data["http_proxy"].(map[string]any); !ok {
		t.Fatalf("HTTP proxy metrics missing: %#v", resp.Data)
	}
}
