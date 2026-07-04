package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"ccLoad/internal/model"
)

type noopAlertNotifier struct{}

func (noopAlertNotifier) Send(context.Context, string) error { return nil }

func TestAlertServiceObserveLogFailureRate(t *testing.T) {
	svc := newAlertService(nil, nil, noopAlertNotifier{}, nil, nil, nil)
	svc.now = func() time.Time { return time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC) }

	for i := 0; i < 9; i++ {
		svc.ObserveLog(&model.LogEntry{StatusCode: 200, Model: "m"})
	}
	for i := 0; i < 10; i++ {
		svc.ObserveLog(&model.LogEntry{StatusCode: 500, Model: "m", Message: "upstream failed"})
	}
	assertNoAlert(t, svc)

	svc.ObserveLog(&model.LogEntry{StatusCode: 500, Model: "m", Message: "upstream failed", ChannelID: 12})
	msg := readAlert(t, svc)
	for _, want := range []string{
		"近 20 次日志失败率过高",
		"失败率：55.0% (11/20)",
		"最新状态：HTTP 500",
		"渠道：#12",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("alert %q does not contain %q", msg, want)
		}
	}

	// While the rate remains above threshold, do not spam duplicate alerts.
	svc.ObserveLog(&model.LogEntry{StatusCode: 502, Model: "m", Message: "still failing"})
	assertNoAlert(t, svc)
}

func TestAlertServiceCheckTokenUsageThreshold(t *testing.T) {
	const tokenHash = "0123456789abcdef"
	auth := &AuthService{
		authTokenCostLimits: map[string]tokenCostLimit{
			tokenHash: {usedMicroUSD: 980_000, limitMicroUSD: 1_000_000},
		},
	}
	svc := newAlertService(nil, auth, noopAlertNotifier{}, nil, nil, nil)
	svc.now = func() time.Time { return time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC) }

	svc.CheckTokenUsage(tokenHash)
	assertNoAlert(t, svc)

	auth.authTokensMux.Lock()
	auth.authTokenCostLimits[tokenHash] = tokenCostLimit{usedMicroUSD: 990_000, limitMicroUSD: 1_000_000}
	auth.authTokensMux.Unlock()
	svc.CheckTokenUsage(tokenHash)
	msg := readAlert(t, svc)
	for _, want := range []string{
		"令牌使用量达到 99%",
		"类型：总额度",
		"用量：$0.9900 / $1.0000 (99.0%)",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("alert %q does not contain %q", msg, want)
		}
	}

	// Crossing once only alerts once until usage drops below the threshold.
	svc.CheckTokenUsage(tokenHash)
	assertNoAlert(t, svc)

	auth.authTokensMux.Lock()
	auth.authTokenCostLimits[tokenHash] = tokenCostLimit{usedMicroUSD: 500_000, limitMicroUSD: 1_000_000}
	auth.authTokensMux.Unlock()
	svc.CheckTokenUsage(tokenHash)
	assertNoAlert(t, svc)

	auth.authTokensMux.Lock()
	auth.authTokenCostLimits[tokenHash] = tokenCostLimit{usedMicroUSD: 995_000, limitMicroUSD: 1_000_000}
	auth.authTokensMux.Unlock()
	svc.CheckTokenUsage(tokenHash)
	_ = readAlert(t, svc)
}

func readAlert(t *testing.T, svc *AlertService) string {
	t.Helper()
	select {
	case msg := <-svc.sendCh:
		return msg
	default:
		t.Fatal("expected alert, got none")
		return ""
	}
}

func assertNoAlert(t *testing.T, svc *AlertService) {
	t.Helper()
	select {
	case msg := <-svc.sendCh:
		t.Fatalf("unexpected alert: %s", msg)
	default:
	}
}
