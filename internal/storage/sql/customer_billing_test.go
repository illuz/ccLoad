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

	group := &model.BillingGroup{Name: "GPT 0.1", Slug: "gpt0.1", Multiplier: 0.1, Enabled: true, ChannelKeys: map[int64][]int{7: {0, 2}}}
	if err := store.CreateBillingGroup(ctx, group); err != nil {
		t.Fatalf("create billing group: %v", err)
	}
	reloadedGroup, err := store.GetBillingGroup(ctx, group.ID)
	if err != nil || len(reloadedGroup.ChannelKeys[7]) != 2 {
		t.Fatalf("billing group channel keys were not persisted: group=%#v err=%v", reloadedGroup, err)
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
	charge, err := store.ChargeAuthTokenBalance(ctx, token.Token, group.ID, group.Slug, "gpt-test", group.Multiplier, "req-1", util.USDToMicroUSD(2), util.USDToMicroUSD(0.2), 1234)
	if err != nil {
		t.Fatalf("charge balance: %v", err)
	}
	if charge == nil || charge.BalanceAfterMicroUSD != util.USDToMicroUSD(59.8) {
		t.Fatalf("unexpected charge: %#v", charge)
	}
	if charge.Model != "gpt-test" {
		t.Fatalf("charge model=%q, want gpt-test", charge.Model)
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
	buckets, err := store.AggregateAuthTokenBalanceUsage(ctx, token.ID, group.ID, time.Now().Add(-time.Hour), time.Now().Add(time.Second), 30*time.Minute)
	if err != nil {
		t.Fatalf("aggregate balance usage: %v", err)
	}
	if len(buckets) != 1 || buckets[0].Model != "gpt-test" || buckets[0].RequestCount != 1 || buckets[0].TotalTokens != 1234 || buckets[0].ChargedMicroUSD != util.USDToMicroUSD(0.2) {
		t.Fatalf("unexpected balance usage buckets: %#v", buckets)
	}
	adjustments, err := store.ListAuthTokenBalanceAdjustments(ctx, token.ID, 10, 0)
	if err != nil {
		t.Fatalf("list balance adjustments: %v", err)
	}
	if len(adjustments) != 1 || adjustments[0].Type != model.BalanceTransactionManualCredit {
		t.Fatalf("unexpected balance adjustments: %#v", adjustments)
	}
}
