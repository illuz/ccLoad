package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ccLoad/internal/model"
)

func TestRefreshChannelBalanceBlocking_GETScript(t *testing.T) {
	srv := newInMemoryServer(t)

	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"is_active":true,"balance":42.5,"daily_used":1.25}`))
	}))
	defer upstream.Close()
	srv.client = upstream.Client()

	cfg := createBalanceTestChannel(t, srv, ChannelRequest{
		Name:        "balance-get",
		APIKey:      "sk-balance-get",
		ChannelType: "anthropic",
		URL:         upstream.URL,
		Models:      []model.ModelEntry{{Model: "claude-3-7-sonnet"}},
		Enabled:     true,
		BalanceQueryScript: `({
			request: {
				url: "{{baseUrl}}/user/balance",
				method: "GET",
				headers: {
					"Authorization": "Bearer {{apiKey}}"
				}
			},
			extractor: function(response, meta) {
				return {
					isValid: response.is_active || true,
					remaining: response.balance,
					used: response.daily_used,
					unit: "USD",
					extra: meta.status === 200 ? "ok" : "bad"
				};
			}
		})`,
	})

	snapshot := srv.refreshChannelBalanceBlocking(context.Background(), cfg)
	if snapshot == nil {
		t.Fatal("snapshot is nil")
	}
	if snapshot.Status != "ready" {
		t.Fatalf("status=%q", snapshot.Status)
	}
	if snapshot.IsValid == nil || !*snapshot.IsValid {
		t.Fatalf("is_valid=%v", snapshot.IsValid)
	}
	assertAnyApprox(t, snapshot.Remaining, 42.5)
	assertAnyApprox(t, snapshot.Used, 1.25)
	if snapshot.Unit != "USD" {
		t.Fatalf("unit=%q", snapshot.Unit)
	}
	if snapshot.Extra != "ok" {
		t.Fatalf("extra=%q", snapshot.Extra)
	}
	if gotAuth != "Bearer sk-balance-get" {
		t.Fatalf("authorization=%q", gotAuth)
	}
}

func TestRefreshChannelBalanceBlocking_POSTScript(t *testing.T) {
	srv := newInMemoryServer(t)

	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":null,"balance":12.3,"used":4.5,"plan":"starter"}`))
	}))
	defer upstream.Close()
	srv.client = upstream.Client()

	cfg := createBalanceTestChannel(t, srv, ChannelRequest{
		Name:        "balance-post",
		APIKey:      "sk-balance-post",
		ChannelType: "anthropic",
		URL:         upstream.URL,
		Models:      []model.ModelEntry{{Model: "claude-3-7-sonnet"}},
		Enabled:     true,
		BalanceQueryScript: `({
			request: {
				url: "{{baseUrl}}/api/usage",
				method: "POST",
				headers: {
					"Authorization": "Bearer {{apiKey}}"
				}
			},
			extractor: function(response) {
				return {
					isValid: !response.error,
					remaining: response.balance,
					used: response.used,
					unit: "USD",
					planName: response.plan
				};
			}
		})`,
	})

	snapshot := srv.refreshChannelBalanceBlocking(context.Background(), cfg)
	if snapshot == nil {
		t.Fatal("snapshot is nil")
	}
	if snapshot.Status != "ready" {
		t.Fatalf("status=%q", snapshot.Status)
	}
	if snapshot.IsValid == nil || !*snapshot.IsValid {
		t.Fatalf("is_valid=%v", snapshot.IsValid)
	}
	assertAnyApprox(t, snapshot.Remaining, 12.3)
	assertAnyApprox(t, snapshot.Used, 4.5)
	if snapshot.PlanName != "starter" {
		t.Fatalf("plan_name=%q", snapshot.PlanName)
	}
	if snapshot.Unit != "USD" {
		t.Fatalf("unit=%q", snapshot.Unit)
	}
	if gotAuth != "Bearer sk-balance-post" {
		t.Fatalf("authorization=%q", gotAuth)
	}
}

func TestHandleListAndGetChannel_AttachUpstreamBalance(t *testing.T) {
	srv := newInMemoryServer(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"balance":7.5,"used":2.5,"plan":"pro"}`))
	}))
	defer upstream.Close()
	srv.client = upstream.Client()

	cfg := createBalanceTestChannel(t, srv, ChannelRequest{
		Name:        "balance-list",
		APIKey:      "sk-balance-list",
		ChannelType: "anthropic",
		URL:         upstream.URL,
		Models:      []model.ModelEntry{{Model: "claude-3-7-sonnet"}},
		Enabled:     true,
		BalanceQueryScript: `({
			request: { url: "{{baseUrl}}/user/balance", method: "GET" },
			extractor: function(response) {
				return {
					isValid: true,
					remaining: response.balance,
					total: "-",
					used: response.used,
					unit: "USD",
					planName: response.plan,
					extra: "无限制"
				};
			}
		})`,
	})

	snapshot := srv.forceRefreshChannelBalance(context.Background(), cfg)
	if snapshot == nil || snapshot.Status != "ready" {
		t.Fatalf("refresh snapshot=%#v", snapshot)
	}

	cList, wList := newTestContext(t, newRequest(http.MethodGet, "/admin/channels", nil))
	srv.handleListChannels(cList)
	if wList.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", wList.Code, wList.Body.String())
	}
	listResp := mustParseAPIResponse[[]ChannelWithCooldown](t, wList.Body.Bytes())
	if len(listResp.Data) != 1 {
		t.Fatalf("len(list)=%d", len(listResp.Data))
	}
	if listResp.Data[0].UpstreamBalance == nil {
		t.Fatal("list upstream_balance is nil")
	}
	if listResp.Data[0].UpstreamBalance.PlanName != "pro" {
		t.Fatalf("list plan_name=%q", listResp.Data[0].UpstreamBalance.PlanName)
	}
	if listResp.Data[0].UpstreamBalance.Extra != "无限制" {
		t.Fatalf("list extra=%q", listResp.Data[0].UpstreamBalance.Extra)
	}

	cGet, wGet := newTestContext(t, newRequest(http.MethodGet, "/admin/channels/1", nil))
	srv.handleGetChannel(cGet, cfg.ID)
	if wGet.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", wGet.Code, wGet.Body.String())
	}
	getResp := mustParseAPIResponse[ChannelWithCooldown](t, wGet.Body.Bytes())
	if getResp.Data.UpstreamBalance == nil {
		t.Fatal("detail upstream_balance is nil")
	}
	assertAnyApprox(t, getResp.Data.UpstreamBalance.Remaining, 7.5)
	assertAnyApprox(t, getResp.Data.UpstreamBalance.Used, 2.5)
}

func createBalanceTestChannel(t *testing.T, srv *Server, req ChannelRequest) *model.Config {
	t.Helper()
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	cfg, err := srv.createChannelFromRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("createChannelFromRequest failed: %v", err)
	}
	return cfg
}

func assertAnyApprox(t *testing.T, got any, want float64) {
	t.Helper()
	switch value := got.(type) {
	case float64:
		assertApprox(t, value, want)
	case float32:
		assertApprox(t, float64(value), want)
	case int:
		assertApprox(t, float64(value), want)
	case int64:
		assertApprox(t, float64(value), want)
	default:
		t.Fatalf("number = %#v, want %.6f", got, want)
	}
}
