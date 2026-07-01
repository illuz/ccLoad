package app

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"testing"

	"ccLoad/internal/model"

	"github.com/gin-gonic/gin"
)

func TestHandleAuthTokenUsage_DailyLimit(t *testing.T) {
	srv := newInMemoryServer(t)
	engine := gin.New()
	srv.SetupRoutes(engine)

	plain := "sk-usage-daily"
	hash := model.HashToken(plain)
	ctx := context.Background()
	token := &model.AuthToken{
		Token:       hash,
		PlainToken:  plain,
		Description: "daily-limit-token",
		IsActive:    true,
	}
	token.SetDailyCostLimitUSD(10)
	if err := srv.store.CreateAuthToken(ctx, token); err != nil {
		t.Fatalf("CreateAuthToken failed: %v", err)
	}
	if err := srv.store.UpdateTokenStats(ctx, hash, true, 0, false, 0, 0, 0, 0, 0, 2.5); err != nil {
		t.Fatalf("UpdateTokenStats failed: %v", err)
	}
	if err := srv.authService.ReloadAuthTokens(); err != nil {
		t.Fatalf("ReloadAuthTokens failed: %v", err)
	}

	req := newRequest(http.MethodGet, "/user/balance", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	w := serveHTTP(t, engine, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	mustUnmarshalJSON(t, w.Body.Bytes(), &resp)
	assertBool(t, resp["is_active"], true)
	assertBool(t, resp["isValid"], true)
	assertApprox(t, resp["balance"], 7.5)
	assertApprox(t, resp["remaining"], 7.5)
	assertApprox(t, resp["total"], 10)
	assertApprox(t, resp["used"], 2.5)
	assertApprox(t, resp["daily_used"], 2.5)
	assertApprox(t, resp["daily_limit"], 10)
	assertApprox(t, resp["daily_remaining"], 7.5)
	assertString(t, resp["unit"], "USD")
	assertString(t, resp["extra"], "已使用 25.0%")
	assertString(t, resp["limit_type"], "daily")
	assertString(t, resp["plan_name"], "daily-limit-token")
	if _, exists := resp["error"]; exists {
		t.Fatalf("unexpected error field: %#v", resp["error"])
	}
}

func TestHandleAuthTokenUsage_Unlimited(t *testing.T) {
	srv := newInMemoryServer(t)
	engine := gin.New()
	srv.SetupRoutes(engine)

	plain := "sk-usage-unlimited"
	hash := model.HashToken(plain)
	ctx := context.Background()
	token := &model.AuthToken{
		Token:       hash,
		PlainToken:  plain,
		Description: "unlimited-token",
		IsActive:    true,
	}
	if err := srv.store.CreateAuthToken(ctx, token); err != nil {
		t.Fatalf("CreateAuthToken failed: %v", err)
	}
	if err := srv.store.UpdateTokenStats(ctx, hash, true, 0, false, 0, 0, 0, 0, 0, 3.2); err != nil {
		t.Fatalf("UpdateTokenStats failed: %v", err)
	}
	if err := srv.authService.ReloadAuthTokens(); err != nil {
		t.Fatalf("ReloadAuthTokens failed: %v", err)
	}

	req := newRequest(http.MethodPost, "/api/usage", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	w := serveHTTP(t, engine, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	mustUnmarshalJSON(t, w.Body.Bytes(), &resp)
	assertBool(t, resp["is_active"], true)
	assertBool(t, resp["isValid"], true)
	assertApprox(t, resp["used"], 3.2)
	assertApprox(t, resp["daily_used"], 3.2)
	assertApprox(t, resp["cost_used"], 3.2)
	assertString(t, resp["unit"], "USD")
	assertString(t, resp["extra"], "无限制")
	assertString(t, resp["limit_type"], "unlimited")
	assertNull(t, resp["balance"])
	assertNull(t, resp["remaining"])
	assertNull(t, resp["total"])
	if _, exists := resp["error"]; exists {
		t.Fatalf("unexpected error field: %#v", resp["error"])
	}
}

func TestHandleAuthTokenUsage_CostLimitExceeded(t *testing.T) {
	srv := newInMemoryServer(t)
	engine := gin.New()
	srv.SetupRoutes(engine)

	plain := "sk-usage-cost"
	hash := model.HashToken(plain)
	ctx := context.Background()
	token := &model.AuthToken{
		Token:       hash,
		PlainToken:  plain,
		Description: "cost-limit-token",
		IsActive:    true,
	}
	token.SetCostLimitUSD(10)
	if err := srv.store.CreateAuthToken(ctx, token); err != nil {
		t.Fatalf("CreateAuthToken failed: %v", err)
	}
	if err := srv.store.UpdateTokenStats(ctx, hash, true, 0, false, 0, 0, 0, 0, 0, 12); err != nil {
		t.Fatalf("UpdateTokenStats failed: %v", err)
	}
	if err := srv.authService.ReloadAuthTokens(); err != nil {
		t.Fatalf("ReloadAuthTokens failed: %v", err)
	}

	req := newRequest(http.MethodGet, "/balance", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	w := serveHTTP(t, engine, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	mustUnmarshalJSON(t, w.Body.Bytes(), &resp)
	assertBool(t, resp["is_active"], false)
	assertBool(t, resp["isValid"], false)
	assertApprox(t, resp["balance"], 0)
	assertApprox(t, resp["remaining"], 0)
	assertApprox(t, resp["total"], 10)
	assertApprox(t, resp["used"], 12)
	assertApprox(t, resp["cost_used"], 12)
	assertApprox(t, resp["cost_limit"], 10)
	assertApprox(t, resp["cost_remaining"], 0)
	assertString(t, resp["limit_type"], "total")
	assertString(t, resp["extra"], "已使用 120.0%")
	assertContains(t, resp["error"], "Cost limit exceeded")
	assertContains(t, resp["invalid_message"], "Cost limit exceeded")
}

func assertBool(t *testing.T, got any, want bool) {
	t.Helper()
	value, ok := got.(bool)
	if !ok || value != want {
		t.Fatalf("bool = %#v, want %v", got, want)
	}
}

func assertString(t *testing.T, got any, want string) {
	t.Helper()
	value, ok := got.(string)
	if !ok || value != want {
		t.Fatalf("string = %#v, want %q", got, want)
	}
}

func assertNull(t *testing.T, got any) {
	t.Helper()
	if got != nil {
		payload, _ := json.Marshal(got)
		t.Fatalf("value = %s, want null", payload)
	}
}

func assertContains(t *testing.T, got any, wantSubstr string) {
	t.Helper()
	value, ok := got.(string)
	if !ok || !strings.Contains(value, wantSubstr) {
		t.Fatalf("string = %#v, want substring %q", got, wantSubstr)
	}
}

func assertApprox(t *testing.T, got any, want float64) {
	t.Helper()
	value, ok := got.(float64)
	if !ok || math.Abs(value-want) > 1e-9 {
		payload, _ := json.Marshal(got)
		t.Fatalf("number = %s, want %.6f", payload, want)
	}
}
