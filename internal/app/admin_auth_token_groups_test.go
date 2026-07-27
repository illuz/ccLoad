package app

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"testing"

	"ccLoad/internal/model"

	"github.com/gin-gonic/gin"
)

func TestAdminAPI_CreateAuthTokenGroup_WithColor(t *testing.T) {
	server := newInMemoryServer(t)

	c, w := newTestContext(t, newJSONRequest(t, http.MethodPost, "/admin/auth-token-groups", map[string]any{
		"name":                     "Premium",
		"color":                    "#3b82f6",
		"allowed_channel_ids":      []int64{3},
		"channel_restriction_mode": "deny",
	}))

	server.HandleCreateAuthTokenGroup(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	resp := mustParseAPIResponse[model.AuthTokenGroup](t, w.Body.Bytes())
	if resp.Data.Color != "#3b82f6" {
		t.Fatalf("color=%q, want %q", resp.Data.Color, "#3b82f6")
	}
	if resp.Data.ChannelRestrictionMode != model.ChannelRestrictionModeDeny {
		t.Fatalf("channel_restriction_mode=%q, want deny", resp.Data.ChannelRestrictionMode)
	}
}

func TestAdminAPI_AuthTokenGroup_InvalidChannelRestrictionMode(t *testing.T) {
	server := newInMemoryServer(t)
	c, w := newTestContext(t, newJSONRequest(t, http.MethodPost, "/admin/auth-token-groups", map[string]any{
		"name":                     "Invalid",
		"channel_restriction_mode": "denyy",
	}))
	server.HandleCreateAuthTokenGroup(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400, body=%s", w.Code, w.Body.String())
	}
}

func TestAdminAPI_UpdateAuthTokenGroup_InvalidColor(t *testing.T) {
	server := newInMemoryServer(t)
	ctx := context.Background()

	group := &model.AuthTokenGroup{
		Name:  "Premium",
		Color: model.DefaultAuthTokenGroupColor,
	}
	if err := server.store.CreateAuthTokenGroup(ctx, group); err != nil {
		t.Fatalf("CreateAuthTokenGroup failed: %v", err)
	}

	c, w := newTestContext(t, newJSONRequest(t, http.MethodPut, "/admin/auth-token-groups/1", map[string]any{
		"color": "#123123",
	}))
	c.Params = gin.Params{{Key: "id", Value: "1"}}

	server.HandleUpdateAuthTokenGroup(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestAdminAPI_CreateAndUpdateAuthTokenGroup_DailyCostLimit(t *testing.T) {
	server := newInMemoryServer(t)

	t.Run("create", func(t *testing.T) {
		c, w := newTestContext(t, newJSONRequest(t, http.MethodPost, "/admin/auth-token-groups", map[string]any{
			"name":                 "Daily Limit Group",
			"daily_cost_limit_usd": 12.34,
			"max_concurrency":      5,
		}))

		server.HandleCreateAuthTokenGroup(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status=%d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
		}

		var resp struct {
			DailyCostLimitUSD float64 `json:"daily_cost_limit_usd"`
			MaxConcurrency    int     `json:"max_concurrency"`
		}
		parsed := mustParseAPIResponse[struct {
			DailyCostLimitUSD float64 `json:"daily_cost_limit_usd"`
			MaxConcurrency    int     `json:"max_concurrency"`
		}](t, w.Body.Bytes())
		resp = parsed.Data
		if math.Abs(resp.DailyCostLimitUSD-12.34) > 1e-9 {
			t.Fatalf("daily_cost_limit_usd=%v, want 12.34", resp.DailyCostLimitUSD)
		}
		if resp.MaxConcurrency != 5 {
			t.Fatalf("max_concurrency=%d, want 5", resp.MaxConcurrency)
		}
	})

	t.Run("update", func(t *testing.T) {
		ctx := context.Background()
		group := &model.AuthTokenGroup{Name: "Editable Group", MaxConcurrency: 2}
		if err := server.store.CreateAuthTokenGroup(ctx, group); err != nil {
			t.Fatalf("CreateAuthTokenGroup failed: %v", err)
		}

		c, w := newTestContext(t, newJSONRequest(t, http.MethodPut, fmt.Sprintf("/admin/auth-token-groups/%d", group.ID), map[string]any{
			"daily_cost_limit_usd": 40,
			"max_concurrency":      8,
		}))
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", group.ID)}}

		server.HandleUpdateAuthTokenGroup(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status=%d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
		}

		resp := mustParseAPIResponse[struct {
			DailyCostLimitUSD float64 `json:"daily_cost_limit_usd"`
			MaxConcurrency    int     `json:"max_concurrency"`
		}](t, w.Body.Bytes())
		if math.Abs(resp.Data.DailyCostLimitUSD-40) > 1e-9 {
			t.Fatalf("daily_cost_limit_usd=%v, want 40", resp.Data.DailyCostLimitUSD)
		}
		if resp.Data.MaxConcurrency != 8 {
			t.Fatalf("max_concurrency=%d, want 8", resp.Data.MaxConcurrency)
		}

		stored, err := server.store.GetAuthTokenGroup(ctx, group.ID)
		if err != nil {
			t.Fatalf("GetAuthTokenGroup failed: %v", err)
		}
		if stored.DailyCostLimitMicroUSD != 40_000_000 {
			t.Fatalf("DailyCostLimitMicroUSD=%d, want 40000000", stored.DailyCostLimitMicroUSD)
		}
		if stored.MaxConcurrency != 8 {
			t.Fatalf("stored max_concurrency=%d, want 8", stored.MaxConcurrency)
		}
	})
}
