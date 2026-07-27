package model

import (
	"encoding/json"
	"math"
	"testing"
)

func TestAuthToken_IsModelAllowed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		allowed      []string
		model        string
		expectedBool bool
	}{
		{name: "empty_allowed_models_allows_any", allowed: nil, model: "gpt-4", expectedBool: true},
		{name: "case_insensitive_match", allowed: []string{"GPT-4", "claude"}, model: "gpt-4", expectedBool: true},
		{name: "no_match", allowed: []string{"gpt-4", "claude"}, model: "gemini", expectedBool: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &AuthToken{AllowedModels: tt.allowed}
			if got := token.IsModelAllowed(tt.model); got != tt.expectedBool {
				t.Fatalf("IsModelAllowed(%q) = %v, want %v", tt.model, got, tt.expectedBool)
			}
		})
	}
}

func TestAuthToken_IsChannelAllowed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		mode         string
		channels     []int64
		channelID    int64
		expectedBool bool
	}{
		{name: "nil_allow_list_allows_any", mode: ChannelRestrictionModeAllow, channels: nil, channelID: 42, expectedBool: true},
		{name: "empty_deny_list_allows_any", mode: ChannelRestrictionModeDeny, channels: []int64{}, channelID: 42, expectedBool: true},
		{name: "allow_list_accepts_listed", mode: ChannelRestrictionModeAllow, channels: []int64{2, 42}, channelID: 42, expectedBool: true},
		{name: "allow_list_rejects_missing", mode: ChannelRestrictionModeAllow, channels: []int64{2, 7}, channelID: 42, expectedBool: false},
		{name: "deny_list_rejects_listed", mode: ChannelRestrictionModeDeny, channels: []int64{2, 42}, channelID: 42, expectedBool: false},
		{name: "deny_list_accepts_missing", mode: ChannelRestrictionModeDeny, channels: []int64{2, 7}, channelID: 42, expectedBool: true},
		{name: "invalid_mode_fails_closed_with_empty_list", mode: "denyy", channels: nil, channelID: 42, expectedBool: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &AuthToken{AllowedChannelIDs: tt.channels, ChannelRestrictionMode: tt.mode}
			if got := token.IsChannelAllowed(tt.channelID); got != tt.expectedBool {
				t.Fatalf("IsChannelAllowed(%d) = %v, want %v", tt.channelID, got, tt.expectedBool)
			}
		})
	}
}

func TestNormalizeChannelRestrictionMode(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		mode    string
		want    string
		wantErr bool
	}{
		{mode: "", want: ChannelRestrictionModeAllow},
		{mode: ChannelRestrictionModeAllow, want: ChannelRestrictionModeAllow},
		{mode: ChannelRestrictionModeDeny, want: ChannelRestrictionModeDeny},
		{mode: "ALLOW", wantErr: true},
		{mode: "denyy", wantErr: true},
	} {
		got, err := NormalizeChannelRestrictionMode(tc.mode)
		if (err != nil) != tc.wantErr {
			t.Fatalf("NormalizeChannelRestrictionMode(%q) error=%v, wantErr=%v", tc.mode, err, tc.wantErr)
		}
		if got != tc.want {
			t.Fatalf("NormalizeChannelRestrictionMode(%q)=%q, want %q", tc.mode, got, tc.want)
		}
	}
}

func TestAuthToken_ApplyGroupEffective_InheritsChannelRestrictionMode(t *testing.T) {
	t.Parallel()

	token := &AuthToken{
		GroupID:                9,
		InheritChannels:        true,
		AllowedChannelIDs:      []int64{1},
		ChannelRestrictionMode: ChannelRestrictionModeAllow,
	}
	group := &AuthTokenGroup{
		ID:                     9,
		Name:                   "G",
		AllowedChannelIDs:      []int64{2},
		ChannelRestrictionMode: ChannelRestrictionModeDeny,
	}

	token.ApplyGroupEffective(group)
	if token.EffectiveChannelRestrictionMode != ChannelRestrictionModeDeny {
		t.Fatalf("effective mode=%q, want deny", token.EffectiveChannelRestrictionMode)
	}
	token.ApplyEffectiveValuesToRawForRuntime()
	if token.IsChannelAllowed(2) {
		t.Fatal("inherited deny list should reject channel 2")
	}
	if !token.IsChannelAllowed(3) {
		t.Fatal("inherited deny list should allow channel 3")
	}
}

func TestAuthToken_CostConversions(t *testing.T) {
	t.Parallel()

	token := &AuthToken{
		CostUsedMicroUSD:       1_230_000, // $1.23
		CostLimitMicroUSD:      4_500_000, // $4.50
		DailyCostUsedMicroUSD:  120_000,   // $0.12
		DailyCostLimitMicroUSD: 900_000,   // $0.90
	}
	if got := token.CostUsedUSD(); math.Abs(got-1.23) > 1e-9 {
		t.Fatalf("CostUsedUSD() = %v, want 1.23", got)
	}
	if got := token.CostLimitUSD(); math.Abs(got-4.5) > 1e-9 {
		t.Fatalf("CostLimitUSD() = %v, want 4.5", got)
	}
	if got := token.DailyCostUsedUSD(); math.Abs(got-0.12) > 1e-9 {
		t.Fatalf("DailyCostUsedUSD() = %v, want 0.12", got)
	}
	if got := token.DailyCostLimitUSD(); math.Abs(got-0.9) > 1e-9 {
		t.Fatalf("DailyCostLimitUSD() = %v, want 0.9", got)
	}

	token.SetCostLimitUSD(0)
	if token.CostLimitMicroUSD != 0 {
		t.Fatalf("SetCostLimitUSD(0) should reset to 0 microUSD, got %d", token.CostLimitMicroUSD)
	}

	token.SetCostLimitUSD(1.5)
	if token.CostLimitMicroUSD != 1_500_000 {
		t.Fatalf("SetCostLimitUSD(1.5) microUSD = %d, want 1500000", token.CostLimitMicroUSD)
	}

	token.SetDailyCostLimitUSD(0.75)
	if token.DailyCostLimitMicroUSD != 750_000 {
		t.Fatalf("SetDailyCostLimitUSD(0.75) microUSD = %d, want 750000", token.DailyCostLimitMicroUSD)
	}
}

func TestAuthToken_MarshalJSON_ExposesCostFields(t *testing.T) {
	t.Parallel()

	token := AuthToken{
		ID:                     123,
		Token:                  "hash",
		IsActive:               true,
		CostUsedMicroUSD:       250_000, // $0.25
		CostLimitMicroUSD:      2_000_000,
		DailyCostUsedMicroUSD:  50_000,
		DailyCostLimitMicroUSD: 800_000,
		AllowedModels:          []string{"gpt-4"},
		AllowedChannelIDs:      []int64{11, 22},
		ChannelRestrictionMode: ChannelRestrictionModeDeny,
		MaxConcurrency:         3,
	}

	b, err := json.Marshal(token)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var got struct {
		CostUsedUSD       float64 `json:"cost_used_usd"`
		CostLimitUSD      float64 `json:"cost_limit_usd"`
		DailyCostUsedUSD  float64 `json:"daily_cost_used_usd"`
		DailyCostLimitUSD float64 `json:"daily_cost_limit_usd"`
		AllowedChannelID  []int64 `json:"allowed_channel_ids"`
		ChannelMode       string  `json:"channel_restriction_mode"`
		MaxConcurrency    int     `json:"max_concurrency"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if math.Abs(got.CostUsedUSD-0.25) > 1e-9 {
		t.Fatalf("cost_used_usd = %#v, want 0.25", got.CostUsedUSD)
	}
	if math.Abs(got.CostLimitUSD-2.0) > 1e-9 {
		t.Fatalf("cost_limit_usd = %#v, want 2.0", got.CostLimitUSD)
	}
	if math.Abs(got.DailyCostUsedUSD-0.05) > 1e-9 {
		t.Fatalf("daily_cost_used_usd = %#v, want 0.05", got.DailyCostUsedUSD)
	}
	if math.Abs(got.DailyCostLimitUSD-0.8) > 1e-9 {
		t.Fatalf("daily_cost_limit_usd = %#v, want 0.8", got.DailyCostLimitUSD)
	}
	if len(got.AllowedChannelID) != 2 || got.AllowedChannelID[0] != 11 || got.AllowedChannelID[1] != 22 {
		t.Fatalf("allowed_channel_ids = %#v, want [11 22]", got.AllowedChannelID)
	}
	if got.ChannelMode != ChannelRestrictionModeDeny {
		t.Fatalf("channel_restriction_mode=%q, want deny", got.ChannelMode)
	}
	if got.MaxConcurrency != 3 {
		t.Fatalf("max_concurrency = %#v, want 3", got.MaxConcurrency)
	}
}

func TestChannelRestrictionMode_InvalidValuesFailJSON(t *testing.T) {
	t.Parallel()

	if _, err := json.Marshal(AuthToken{ChannelRestrictionMode: "denyy"}); err == nil {
		t.Fatal("expected invalid token channel restriction mode to fail JSON marshaling")
	}
	if _, err := json.Marshal(AuthTokenGroup{Name: "G", ChannelRestrictionMode: "denyy"}); err == nil {
		t.Fatal("expected invalid group channel restriction mode to fail JSON marshaling")
	}
}

func TestAuthToken_MarshalJSON_ExposesEffectiveDailyCostLimit(t *testing.T) {
	t.Parallel()

	token := AuthToken{
		DailyCostLimitMicroUSD:          800_000,
		EffectiveSet:                    true,
		EffectiveDailyCostLimitMicroUSD: 1_500_000,
	}

	b, err := json.Marshal(token)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var got struct {
		DailyCostLimitUSD          float64 `json:"daily_cost_limit_usd"`
		EffectiveDailyCostLimitUSD float64 `json:"effective_daily_cost_limit_usd"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if math.Abs(got.DailyCostLimitUSD-0.8) > 1e-9 {
		t.Fatalf("daily_cost_limit_usd = %#v, want 0.8", got.DailyCostLimitUSD)
	}
	if math.Abs(got.EffectiveDailyCostLimitUSD-1.5) > 1e-9 {
		t.Fatalf("effective_daily_cost_limit_usd = %#v, want 1.5", got.EffectiveDailyCostLimitUSD)
	}
}

func TestAuthToken_ApplyGroupEffective_DoublesDailyLimitForToday(t *testing.T) {
	t.Parallel()

	token := &AuthToken{
		GroupID:                9,
		InheritQuota:           true,
		DailyLimitDoubleDayKey: CurrentLocalDayKey(),
		DailyCostLimitMicroUSD: 300_000,
		MaxConcurrency:         1,
	}
	group := &AuthTokenGroup{
		ID:                     9,
		Name:                   "G",
		DailyCostLimitMicroUSD: 700_000,
		MaxConcurrency:         2,
	}

	token.ApplyGroupEffective(group)

	if got := token.EffectiveDailyCostLimitMicroUSD; got != 1_400_000 {
		t.Fatalf("EffectiveDailyCostLimitMicroUSD = %d, want 1400000", got)
	}
}

func TestAuthTokenGroup_DailyCostConversionsAndValidation(t *testing.T) {
	t.Parallel()

	group := &AuthTokenGroup{
		Name:                   "Group A",
		DailyCostLimitMicroUSD: 1_250_000,
		MaxConcurrency:         3,
	}
	if got := group.DailyCostLimitUSD(); math.Abs(got-1.25) > 1e-9 {
		t.Fatalf("DailyCostLimitUSD() = %v, want 1.25", got)
	}

	group.SetDailyCostLimitUSD(2.5)
	if group.DailyCostLimitMicroUSD != 2_500_000 {
		t.Fatalf("SetDailyCostLimitUSD(2.5) microUSD = %d, want 2500000", group.DailyCostLimitMicroUSD)
	}

	group.MaxConcurrency = 0
	if err := group.ValidateUsageLimits(); err == nil || err.Error() != "cost-limited auth token group requires max_concurrency > 0" {
		t.Fatalf("ValidateUsageLimits() error = %v, want cost-limited auth token group requires max_concurrency > 0", err)
	}
}

func TestAuthTokenGroup_MarshalJSON_ExposesDailyCostLimit(t *testing.T) {
	t.Parallel()

	group := AuthTokenGroup{
		ID:                     7,
		Name:                   "Group B",
		DailyCostLimitMicroUSD: 3_500_000,
		MaxConcurrency:         9,
	}

	b, err := json.Marshal(group)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var got struct {
		DailyCostLimitUSD float64 `json:"daily_cost_limit_usd"`
		MaxConcurrency    int     `json:"max_concurrency"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if math.Abs(got.DailyCostLimitUSD-3.5) > 1e-9 {
		t.Fatalf("daily_cost_limit_usd = %#v, want 3.5", got.DailyCostLimitUSD)
	}
	if got.MaxConcurrency != 9 {
		t.Fatalf("max_concurrency = %#v, want 9", got.MaxConcurrency)
	}
}
