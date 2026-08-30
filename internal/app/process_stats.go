package app

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

type cpuUsageTracker struct {
	mu          sync.Mutex
	sampled     bool
	lastCPU     float64
	lastAt      time.Time
	lastPercent float64
}

// percent reports top-style CPU usage, which can exceed 100 on multi-core hosts.
func (t *cpuUsageTracker) percent(totalCPUSeconds float64, now time.Time, uptimeSeconds float64) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.sampled {
		t.sampled = true
		t.lastCPU = totalCPUSeconds
		t.lastAt = now
		if uptimeSeconds > 0 {
			t.lastPercent = totalCPUSeconds / uptimeSeconds * 100
		}
		return t.lastPercent
	}

	window := now.Sub(t.lastAt).Seconds()
	if window < 1 {
		return t.lastPercent
	}
	percent := (totalCPUSeconds - t.lastCPU) / window * 100
	if percent < 0 {
		percent = 0
	}
	t.lastCPU = totalCPUSeconds
	t.lastAt = now
	t.lastPercent = percent
	return percent
}

func parseStatmResidentBytes(statm string, pageSize int) uint64 {
	fields := strings.Fields(statm)
	if len(fields) < 2 || pageSize <= 0 {
		return 0
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return pages * uint64(pageSize)
}
