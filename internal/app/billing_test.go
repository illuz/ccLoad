package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ccLoad/internal/model"
	"ccLoad/internal/protocol"
	"ccLoad/internal/util"

	"github.com/gin-gonic/gin"
)

func TestSplitBillingGroupVariant(t *testing.T) {
	base, slug, ok := splitBillingGroupVariant("sk-test~gpt0.1")
	if !ok || base != "sk-test" || slug != "gpt0.1" {
		t.Fatalf("unexpected variant: base=%q slug=%q ok=%v", base, slug, ok)
	}
	if _, _, ok := splitBillingGroupVariant("sk-test@gpt0.1"); ok {
		t.Fatal("@ suffix must not be treated as a billing group variant")
	}
}

func TestBillingGroupVariantAuthenticatesAsBaseToken(t *testing.T) {
	srv := newInMemoryServer(t)
	plain := "sk-variant"
	token := &model.AuthToken{Token: model.HashToken(plain), PlainToken: plain, Description: "variant", IsActive: true}
	if err := srv.store.CreateAuthToken(context.Background(), token); err != nil {
		t.Fatalf("create token: %v", err)
	}
	if err := srv.authService.ReloadAuthTokens(); err != nil {
		t.Fatalf("reload tokens: %v", err)
	}
	router := gin.New()
	router.GET("/probe", srv.authService.RequireAPIAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"hash": c.GetString("token_hash"), "slug": c.GetString("billing_group_slug")})
	})
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Authorization", "Bearer "+plain+"~gpt0.1")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["hash"] != token.Token || payload["slug"] != "gpt0.1" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestBalanceModeBypassesLegacyCostLimitAndRejectsEmptyBalance(t *testing.T) {
	srv := newInMemoryServer(t)
	token := &model.AuthToken{Token: model.HashToken("sk-balance"), Description: "wallet", IsActive: true, BalanceEnabled: true, BalanceMicroUSD: util.USDToMicroUSD(1)}
	token.SetCostLimitUSD(1)
	token.CostUsedMicroUSD = util.USDToMicroUSD(1)
	if err := srv.store.CreateAuthToken(context.Background(), token); err != nil {
		t.Fatalf("create token: %v", err)
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if !srv.enforceTokenLimits(c, protocol.OpenAI, token.Token, "gpt-test", time.Now(), false, "") {
		t.Fatal("balance mode should bypass an exhausted legacy cost limit")
	}
	usage, err := srv.buildAuthTokenUsageResponse(context.Background(), token.Token)
	if err != nil {
		t.Fatalf("build usage: %v", err)
	}
	if usage.LimitType != "balance" || usage.Balance != float64(1) || !usage.IsValid {
		t.Fatalf("unexpected balance usage response: %#v", usage)
	}

	token.BalanceMicroUSD = 0
	if err := srv.store.UpdateAuthToken(context.Background(), token); err != nil {
		t.Fatalf("update token: %v", err)
	}
	recorder := httptest.NewRecorder()
	c, _ = gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if srv.enforceTokenLimits(c, protocol.OpenAI, token.Token, "gpt-test", time.Now(), false, "") {
		t.Fatal("empty balance must reject new requests")
	}
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
}

func TestTokenStatsSettlementChargesBillingGroupMultiplier(t *testing.T) {
	srv := newInMemoryServer(t)
	ctx := context.Background()
	group := &model.BillingGroup{Name: "GPT 0.1", Slug: "gpt0.1", Multiplier: 0.1, Enabled: true}
	if err := srv.store.CreateBillingGroup(ctx, group); err != nil {
		t.Fatalf("create group: %v", err)
	}
	token := &model.AuthToken{Token: model.HashToken("sk-settle"), Description: "settle", IsActive: true, BalanceEnabled: true, BalanceMicroUSD: util.USDToMicroUSD(1), DefaultBillingGroupID: group.ID}
	if err := srv.store.CreateAuthToken(ctx, token); err != nil {
		t.Fatalf("create token: %v", err)
	}
	srv.applyTokenStatsUpdate(tokenStatsUpdate{
		tokenHash: token.Token, isSuccess: true, isBillable: true, costUSD: 2,
		billingGroupID: group.ID, billingGroupSlug: group.Slug, billingMultiplier: group.Multiplier,
		requestID: "req-settle", balanceEnabled: true, promptTokens: 100, completionTokens: 20,
	})
	got, err := srv.store.GetAuthToken(ctx, token.ID)
	if err != nil {
		t.Fatalf("get token: %v", err)
	}
	if got.BalanceMicroUSD != util.USDToMicroUSD(0.8) {
		t.Fatalf("balance=%d, want %d", got.BalanceMicroUSD, util.USDToMicroUSD(0.8))
	}
	entries, err := srv.store.ListAuthTokenBalanceTransactions(ctx, token.ID, 10, 0)
	if err != nil || len(entries) != 1 {
		t.Fatalf("transactions=%#v err=%v", entries, err)
	}
	if entries[0].BillingGroupID != group.ID || entries[0].TotalTokens != 120 || entries[0].DeltaMicroUSD != -util.USDToMicroUSD(0.2) {
		t.Fatalf("unexpected transaction: %#v", entries[0])
	}
}
