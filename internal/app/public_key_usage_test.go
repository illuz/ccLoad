package app

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"ccLoad/internal/model"
	"ccLoad/internal/util"

	"github.com/gin-gonic/gin"
)

func TestPublicKeyUsageRequiresAValidKey(t *testing.T) {
	server := newInMemoryServer(t)
	ctx := context.Background()
	key := "sk-public-usage-test"
	now := time.Now()
	token := &model.AuthToken{
		Token:       model.HashToken(key),
		PlainToken:  key,
		Description: "must not be returned",
		CreatedAt:   now.Add(-24 * time.Hour),
		IsActive:    true,
	}
	token.SetCostLimitUSD(2)
	if err := server.store.CreateAuthToken(ctx, token); err != nil {
		t.Fatalf("CreateAuthToken failed: %v", err)
	}
	if err := server.store.UpdateTokenStats(ctx, token.Token, true, true, 0.2, true, 0.12, 12, 34, 5, 6, 0.05, 0.1); err != nil {
		t.Fatalf("UpdateTokenStats failed: %v", err)
	}
	if err := server.store.BatchAddLogs(ctx, []*model.LogEntry{
		{
			Time:                     model.JSONTime{Time: now},
			Model:                    "gpt-test",
			ChannelID:                1,
			StatusCode:               http.StatusOK,
			IsStreaming:              true,
			FirstByteTime:            0.12,
			AuthTokenID:              token.ID,
			InputTokens:              12,
			OutputTokens:             34,
			CacheReadInputTokens:     5,
			CacheCreationInputTokens: 6,
			Cost:                     0.05,
			CostMultiplier:           2,
		},
	}); err != nil {
		t.Fatalf("BatchAddLogs failed: %v", err)
	}

	router := gin.New()
	server.SetupRoutes(router)

	t.Run("valid key returns only usage data", func(t *testing.T) {
		w := serveHTTP(t, router, newRequest(http.MethodGet, "/public/key-usage?key="+key, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
		}
		if body := w.Body.String(); containsAny(body, key, "must not be returned") {
			t.Fatalf("public response leaked token metadata: %s", body)
		}

		var response struct {
			Success bool `json:"success"`
			Data    struct {
				Today     PublicKeyTodayUsage        `json:"today"`
				CostQuota PublicKeyCostQuota         `json:"cost_quota"`
				Trend     []PublicKeyUsageTrendPoint `json:"trend"`
			} `json:"data"`
		}
		mustUnmarshalJSON(t, w.Body.Bytes(), &response)
		if !response.Success {
			t.Fatalf("unexpected response: %+v", response)
		}
		if response.Data.Today.RequestCount != 1 || response.Data.Today.TotalTokens != 57 || response.Data.Today.EffectiveCost != 0.1 {
			t.Fatalf("unexpected today stats: %+v", response.Data.Today)
		}
		if response.Data.CostQuota.LimitUSD == nil || *response.Data.CostQuota.LimitUSD != 2 {
			t.Fatalf("unexpected cost limit: %+v", response.Data.CostQuota)
		}
		if response.Data.CostQuota.UsagePercentage == nil || *response.Data.CostQuota.UsagePercentage != 5 {
			t.Fatalf("unexpected usage percentage: %+v", response.Data.CostQuota)
		}
		nonEmptyPoint := false
		for _, point := range response.Data.Trend {
			if point.TotalTokens == 57 && point.EffectiveCost == 0.1 {
				nonEmptyPoint = true
				break
			}
		}
		if !nonEmptyPoint {
			t.Fatalf("trend missing expected usage point: %+v", response.Data.Trend)
		}
		if body := w.Body.String(); containsAny(body, `"history"`, `"total"`, `"success_count"`, `"recent_rpm"`) {
			t.Fatalf("response contains removed statistics: %s", body)
		}
	})

	for _, target := range []string{
		"/public/key-usage",
		"/public/key-usage?key=unknown",
	} {
		t.Run(target, func(t *testing.T) {
			w := serveHTTP(t, router, newRequest(http.MethodGet, target, nil))
			if w.Code != http.StatusNotFound {
				t.Fatalf("status=%d, want %d, body=%s", w.Code, http.StatusNotFound, w.Body.String())
			}
		})
	}
}

func TestBuildPublicKeyCostQuotaPrefersDailyThenTotal(t *testing.T) {
	server, _, cleanup := setupAdminTestServer(t)
	defer cleanup()

	token := &model.AuthToken{
		Token:                 model.HashToken("quota-test"),
		CostUsedMicroUSD:      util.USDToMicroUSD(1),
		DailyCostUsedMicroUSD: util.USDToMicroUSD(0.25),
		DailyCostDayKey:       model.CurrentLocalDayKey(),
	}
	token.SetCostLimitUSD(4)
	token.SetDailyCostLimitUSD(1)

	daily := server.buildPublicKeyCostQuota(context.Background(), token)
	if daily.LimitUSD == nil || *daily.LimitUSD != 1 || daily.UsedUSD != 0.25 {
		t.Fatalf("unexpected daily quota: %+v", daily)
	}
	if daily.UsagePercentage == nil || *daily.UsagePercentage != 25 {
		t.Fatalf("unexpected daily percentage: %+v", daily)
	}

	token.SetDailyCostLimitUSD(0)
	total := server.buildPublicKeyCostQuota(context.Background(), token)
	if total.LimitUSD == nil || *total.LimitUSD != 4 || total.UsedUSD != 1 {
		t.Fatalf("unexpected total quota: %+v", total)
	}
	if total.UsagePercentage == nil || *total.UsagePercentage != 25 {
		t.Fatalf("unexpected total percentage: %+v", total)
	}
}

func TestPublicKeyUsagePageRequiresAValidKey(t *testing.T) {
	originalFS := embedFS
	defer func() { embedFS = originalFS }()
	SetEmbedFS(os.DirFS("../.."), "web")

	server := newInMemoryServer(t)
	key := "sk-public-page-test"
	if err := server.store.CreateAuthToken(context.Background(), &model.AuthToken{
		Token:      model.HashToken(key),
		PlainToken: key,
		IsActive:   true,
	}); err != nil {
		t.Fatalf("CreateAuthToken failed: %v", err)
	}

	router := gin.New()
	server.SetupRoutes(router)

	w := serveHTTP(t, router, newRequest(http.MethodGet, "/key-usage?key="+key, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type=%q, want html", got)
	}
	if body := w.Body.String(); !strings.Contains(body, "今日用量趋势") || containsAny(body, "历史统计", "累计总量", "历史区间") {
		t.Fatalf("unexpected public usage page content: %s", body)
	}

	for _, target := range []string{
		"/key-usage",
		"/key-usage?key=unknown",
		"/web/key-usage.html?key=" + key,
	} {
		w := serveHTTP(t, router, newRequest(http.MethodGet, target, nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d, want %d", target, w.Code, http.StatusNotFound)
		}
	}
}

func containsAny(value string, values ...string) bool {
	for _, candidate := range values {
		if candidate != "" && strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
