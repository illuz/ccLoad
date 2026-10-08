package app

import (
	"context"
	"fmt"
	"math"
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
		{
			Time:                     model.JSONTime{Time: now},
			Model:                    "claude-test",
			ChannelID:                1,
			StatusCode:               http.StatusOK,
			AuthTokenID:              token.ID,
			InputTokens:              20,
			OutputTokens:             10,
			CacheReadInputTokens:     8,
			CacheCreationInputTokens: 5,
			Cost:                     0.05,
			CostMultiplier:           1,
		},
		{
			Time:           model.JSONTime{Time: now},
			Model:          "gpt-test",
			ChannelID:      1,
			StatusCode:     499,
			AuthTokenID:    token.ID,
			InputTokens:    9999,
			OutputTokens:   9999,
			Cost:           99,
			CostMultiplier: 1,
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
				Today      PublicKeyTodayUsage        `json:"today"`
				CostQuota  PublicKeyCostQuota         `json:"cost_quota"`
				ModelUsage []PublicKeyModelUsage      `json:"model_usage"`
				Trend      []PublicKeyUsageTrendPoint `json:"trend"`
			} `json:"data"`
		}
		mustUnmarshalJSON(t, w.Body.Bytes(), &response)
		if !response.Success {
			t.Fatalf("unexpected response: %+v", response)
		}
		if response.Data.Today.RequestCount != 2 || response.Data.Today.TotalTokens != 100 || math.Abs(response.Data.Today.EffectiveCost-0.15) > 1e-9 {
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
			if point.TotalTokens == 100 && math.Abs(point.EffectiveCost-0.15) <= 1e-9 {
				nonEmptyPoint = true
				break
			}
		}
		if !nonEmptyPoint {
			t.Fatalf("trend missing expected usage point: %+v", response.Data.Trend)
		}
		modelUsage := make(map[string]PublicKeyModelUsage, len(response.Data.ModelUsage))
		for _, item := range response.Data.ModelUsage {
			modelUsage[item.Model] = item
		}
		if len(modelUsage) != 2 {
			t.Fatalf("unexpected model usage: %+v", response.Data.ModelUsage)
		}
		if got := modelUsage["gpt-test"]; got.TotalTokens != 57 || math.Abs(got.EffectiveCost-0.1) > 1e-9 {
			t.Fatalf("unexpected gpt model usage: %+v", got)
		}
		if got := modelUsage["claude-test"]; got.TotalTokens != 43 || math.Abs(got.EffectiveCost-0.05) > 1e-9 {
			t.Fatalf("unexpected claude model usage: %+v", got)
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

func TestPublicKeyUsageBalanceModeUsesActualChargesAndGroupFilter(t *testing.T) {
	server := newInMemoryServer(t)
	ctx := context.Background()
	key := "sk-public-balance-usage"
	groupA := &model.BillingGroup{Name: "折扣 A", Slug: "discount-a", Multiplier: 0.1, Enabled: true}
	groupB := &model.BillingGroup{Name: "折扣 B", Slug: "discount-b", Multiplier: 0.5, Enabled: true}
	for _, group := range []*model.BillingGroup{groupA, groupB} {
		if err := server.store.CreateBillingGroup(ctx, group); err != nil {
			t.Fatalf("CreateBillingGroup failed: %v", err)
		}
	}
	token := &model.AuthToken{
		Token:                 model.HashToken(key),
		PlainToken:            key,
		IsActive:              true,
		BalanceEnabled:        true,
		BalanceMicroUSD:       util.USDToMicroUSD(10),
		DefaultBillingGroupID: groupA.ID,
	}
	if err := server.store.CreateAuthToken(ctx, token); err != nil {
		t.Fatalf("CreateAuthToken failed: %v", err)
	}
	if _, err := server.store.AdjustAuthTokenBalance(ctx, token.ID, util.USDToMicroUSD(1), model.BalanceTransactionManualCredit, "测试充值"); err != nil {
		t.Fatalf("AdjustAuthTokenBalance failed: %v", err)
	}
	if _, err := server.store.ChargeAuthTokenBalance(ctx, token.Token, groupA.ID, groupA.Slug, "gpt-test", groupA.Multiplier, "req-a", util.USDToMicroUSD(2), util.USDToMicroUSD(0.2), 100); err != nil {
		t.Fatalf("charge group A failed: %v", err)
	}
	if _, err := server.store.ChargeAuthTokenBalance(ctx, token.Token, groupB.ID, groupB.Slug, "claude-test", groupB.Multiplier, "req-b", util.USDToMicroUSD(1.5), util.USDToMicroUSD(0.75), 200); err != nil {
		t.Fatalf("charge group B failed: %v", err)
	}

	router := gin.New()
	server.SetupRoutes(router)
	type balanceChange struct {
		Type     string  `json:"type"`
		DeltaUSD float64 `json:"delta_usd"`
	}
	type responseData struct {
		Today                  PublicKeyTodayUsage        `json:"today"`
		ModelUsage             []PublicKeyModelUsage      `json:"model_usage"`
		Trend                  []PublicKeyUsageTrendPoint `json:"trend"`
		SelectedBillingGroupID int64                      `json:"selected_billing_group_id"`
		BalanceTransactions    []balanceChange            `json:"balance_transactions"`
	}
	getUsage := func(target string) responseData {
		t.Helper()
		w := serveHTTP(t, router, newRequest(http.MethodGet, target, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
		}
		var response struct {
			Success bool         `json:"success"`
			Data    responseData `json:"data"`
		}
		mustUnmarshalJSON(t, w.Body.Bytes(), &response)
		if !response.Success {
			t.Fatalf("unexpected response: %s", w.Body.String())
		}
		return response.Data
	}

	all := getUsage("/public/key-usage?key=" + key)
	if all.Today.RequestCount != 2 || all.Today.TotalTokens != 300 || math.Abs(all.Today.EffectiveCost-0.95) > 1e-9 {
		t.Fatalf("unexpected all-group usage: %+v", all.Today)
	}
	if len(all.ModelUsage) != 2 {
		t.Fatalf("unexpected all-group model usage: %+v", all.ModelUsage)
	}
	if len(all.BalanceTransactions) != 1 || all.BalanceTransactions[0].Type != model.BalanceTransactionManualCredit || all.BalanceTransactions[0].DeltaUSD != 1 {
		t.Fatalf("public balance changes include consumption or omit recharge: %+v", all.BalanceTransactions)
	}

	filtered := getUsage(fmt.Sprintf("/public/key-usage?key=%s&billing_group_id=%d", key, groupA.ID))
	if filtered.SelectedBillingGroupID != groupA.ID {
		t.Fatalf("selected group=%d, want %d", filtered.SelectedBillingGroupID, groupA.ID)
	}
	if filtered.Today.RequestCount != 1 || filtered.Today.TotalTokens != 100 || math.Abs(filtered.Today.EffectiveCost-0.2) > 1e-9 {
		t.Fatalf("unexpected filtered usage: %+v", filtered.Today)
	}
	if len(filtered.ModelUsage) != 1 || filtered.ModelUsage[0].Model != "gpt-test" || filtered.ModelUsage[0].TotalTokens != 100 || math.Abs(filtered.ModelUsage[0].EffectiveCost-0.2) > 1e-9 {
		t.Fatalf("unexpected filtered model usage: %+v", filtered.ModelUsage)
	}
	foundTrendUsage := false
	for _, point := range filtered.Trend {
		if point.TotalTokens == 100 && math.Abs(point.EffectiveCost-0.2) <= 1e-9 {
			foundTrendUsage = true
			break
		}
	}
	if !foundTrendUsage {
		t.Fatalf("filtered trend missing actual balance usage: %+v", filtered.Trend)
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
