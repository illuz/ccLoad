package sql_test

import (
	"context"
	"testing"
	"time"

	"ccLoad/internal/model"
	"ccLoad/internal/util"
)

func TestCustomerBillingGroupAndTokenBalance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, "customer_billing.db")
	ctx := context.Background()

	group := &model.BillingGroup{Name: "GPT 0.1", Slug: "gpt0.1", Multiplier: 0.1, Enabled: true}
	if err := store.CreateBillingGroup(ctx, group); err != nil {
		t.Fatalf("create billing group: %v", err)
	}
	token := &model.AuthToken{Token: "billing-token-hash", Description: "customer", IsActive: true, BalanceEnabled: true, BalanceMicroUSD: util.USDToMicroUSD(50), DefaultBillingGroupID: group.ID}
	if err := store.CreateAuthToken(ctx, token); err != nil {
		t.Fatalf("create auth token: %v", err)
	}

	credit, err := store.AdjustAuthTokenBalance(ctx, token.ID, util.USDToMicroUSD(10), model.BalanceTransactionManualCredit, "wechat")
	if err != nil {
		t.Fatalf("credit balance: %v", err)
	}
	if credit.BalanceBeforeMicroUSD != util.USDToMicroUSD(50) || credit.BalanceAfterMicroUSD != util.USDToMicroUSD(60) {
		t.Fatalf("unexpected credit balances: %#v", credit)
	}
	charge, err := store.ChargeAuthTokenBalance(ctx, token.Token, group.ID, group.Slug, group.Multiplier, "req-1", util.USDToMicroUSD(2), util.USDToMicroUSD(0.2), 1234)
	if err != nil {
		t.Fatalf("charge balance: %v", err)
	}
	if charge == nil || charge.BalanceAfterMicroUSD != util.USDToMicroUSD(59.8) {
		t.Fatalf("unexpected charge: %#v", charge)
	}

	got, err := store.GetAuthToken(ctx, token.ID)
	if err != nil {
		t.Fatalf("get auth token: %v", err)
	}
	if !got.BalanceEnabled || got.BalanceMicroUSD != util.USDToMicroUSD(59.8) || got.DefaultBillingGroupID != group.ID {
		t.Fatalf("unexpected token wallet: %#v", got)
	}
	usage, err := store.GetAuthTokenBillingGroupUsage(ctx, token.ID, time.UnixMilli(0), time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("get group usage: %v", err)
	}
	if len(usage) != 1 || usage[0].RequestCount != 1 || usage[0].TotalTokens != 1234 || usage[0].ChargedUSD != 0.2 {
		t.Fatalf("unexpected group usage: %#v", usage)
	}
}
