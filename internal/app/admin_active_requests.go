package app

import (
	"errors"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"ccLoad/internal/storage"

	"github.com/gin-gonic/gin"
)

type processRuntimeMetrics struct {
	UptimeSeconds         int64   `json:"uptime_seconds"`
	ConcurrencySlotsInUse int     `json:"concurrency_slots_in_use"`
	MaxConcurrency        int     `json:"max_concurrency"`
	Goroutines            int     `json:"goroutines"`
	CPUUsagePercent       float64 `json:"cpu_usage_percent"`
	CPUUserSeconds        float64 `json:"cpu_user_seconds"`
	CPUSystemSeconds      float64 `json:"cpu_system_seconds"`
	RSSBytes              uint64  `json:"rss_bytes"`
	MaxRSSBytes           uint64  `json:"max_rss_bytes"`
	HeapAllocBytes        uint64  `json:"heap_alloc_bytes"`
	HeapSysBytes          uint64  `json:"heap_sys_bytes"`
	GCCount               uint32  `json:"gc_count"`
	GCPauseTotalNs        uint64  `json:"gc_pause_total_ns"`
	GCCPUPercent          float64 `json:"gc_cpu_percent"`
}

func (s *Server) activeRequestCount() int {
	if s == nil || s.concurrencySem == nil {
		return 0
	}
	return len(s.concurrencySem)
}

func (s *Server) processRuntimeMetrics(now time.Time) processRuntimeMetrics {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)

	uptime := time.Duration(0)
	if !s.startedAt.IsZero() && now.After(s.startedAt) {
		uptime = now.Sub(s.startedAt)
	}
	metrics := processRuntimeMetrics{
		UptimeSeconds:         int64(uptime.Seconds()),
		ConcurrencySlotsInUse: s.activeRequestCount(),
		MaxConcurrency:        s.maxConcurrency,
		Goroutines:            runtime.NumGoroutine(),
		RSSBytes:              readCurrentRSSBytes(),
		HeapAllocBytes:        memory.HeapAlloc,
		HeapSysBytes:          memory.HeapSys,
		GCCount:               memory.NumGC,
		GCPauseTotalNs:        memory.PauseTotalNs,
		GCCPUPercent:          memory.GCCPUFraction * 100,
	}
	if user, system, maxRSS, ok := readProcessRusage(); ok {
		metrics.CPUUserSeconds = user
		metrics.CPUSystemSeconds = system
		metrics.MaxRSSBytes = maxRSS
		metrics.CPUUsagePercent = s.cpuUsage.percent(user+system, now, uptime.Seconds())
	}
	return metrics
}

// HandleActiveRequests 返回当前进行中的请求列表（内存状态，不持久化）
func (s *Server) HandleActiveRequests(c *gin.Context) {
	var requests []*ActiveRequest
	if s.activeRequests != nil {
		requests = s.activeRequests.List()
	}
	RespondJSONWithCount(c, http.StatusOK, requests, len(requests))
}

// HandleRuntimeMetrics returns the current process and runtime subsystem state.
func (s *Server) HandleRuntimeMetrics(c *gin.Context) {
	data := gin.H{
		"process":    s.processRuntimeMetrics(time.Now()),
		"http_proxy": s.httpRuntime.stats(),
	}
	if provider, ok := s.store.(storage.HybridRuntimeMetricsProvider); ok {
		data["storage"] = provider.RuntimeMetrics()
	}
	if s.logService != nil {
		data["logs"] = s.logService.runtimeMetrics()
	}
	RespondJSON(c, http.StatusOK, data)
}

// HandleGetActiveRequestDebugLog 返回运行中请求的调试日志快照。
// GET /admin/active-requests/:request_id/debug-log
func (s *Server) HandleGetActiveRequestDebugLog(c *gin.Context) {
	requestIDStr := c.Param("request_id")
	requestID, err := strconv.ParseInt(requestIDStr, 10, 64)
	if err != nil || requestID <= 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid request_id")
		return
	}

	if s.activeRequests == nil {
		RespondErrorWithData(c, http.StatusNotFound, "debug log unavailable", s.buildDebugLogUnavailableInfo(c.Request.Context()))
		return
	}

	entry, ok := s.activeRequests.GetDebugLogSnapshot(requestID)
	if !ok || entry == nil {
		RespondErrorWithData(c, http.StatusNotFound, "debug log unavailable", s.buildDebugLogUnavailableInfo(c.Request.Context()))
		return
	}

	RespondJSON(c, http.StatusOK, debugLogResponse(entry))
}

// HandleFailoverActiveRequest 中断一条尚未输出响应的上游尝试，并继续下一个候选渠道。
// POST /admin/active-requests/:request_id/failover
func (s *Server) HandleFailoverActiveRequest(c *gin.Context) {
	requestID, err := strconv.ParseInt(c.Param("request_id"), 10, 64)
	if err != nil || requestID <= 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid request_id")
		return
	}

	if s.activeRequests == nil {
		RespondErrorMsg(c, http.StatusNotFound, "active request not found")
		return
	}

	err = s.activeRequests.RequestFailover(requestID)
	if errors.Is(err, errActiveRequestNotFound) {
		RespondErrorMsg(c, http.StatusNotFound, "active request not found")
		return
	}
	if errors.Is(err, errActiveRequestNotFailoverable) {
		RespondErrorMsg(c, http.StatusConflict, "active request has already started responding")
		return
	}
	if err != nil {
		RespondErrorMsg(c, http.StatusInternalServerError, "failed to request upstream failover")
		return
	}

	RespondJSON(c, http.StatusAccepted, gin.H{"request_id": requestID})
}
