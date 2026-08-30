package model

import "testing"

func TestParseThinkingSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantBase   string
		wantMode   ThinkingSuffixMode
		wantLevel  string
		wantBudget int
		wantOK     bool
	}{
		{name: "level", input: "gpt-5.6-luna(HIGH)", wantBase: "gpt-5.6-luna", wantMode: ThinkingSuffixLevel, wantLevel: "high", wantOK: true},
		{name: "max", input: "gpt-5.6-luna(max)", wantBase: "gpt-5.6-luna", wantMode: ThinkingSuffixLevel, wantLevel: "max", wantOK: true},
		{name: "auto", input: "gpt-5.6-luna(auto)", wantBase: "gpt-5.6-luna", wantMode: ThinkingSuffixAuto, wantBudget: -1, wantOK: true},
		{name: "numeric auto", input: "gpt-5.6-luna(-1)", wantBase: "gpt-5.6-luna", wantMode: ThinkingSuffixAuto, wantBudget: -1, wantOK: true},
		{name: "none", input: "gpt-5.6-luna(none)", wantBase: "gpt-5.6-luna", wantMode: ThinkingSuffixNone, wantOK: true},
		{name: "zero", input: "gpt-5.6-luna(0)", wantBase: "gpt-5.6-luna", wantMode: ThinkingSuffixNone, wantOK: true},
		{name: "budget", input: "claude-sonnet(08192)", wantBase: "claude-sonnet", wantMode: ThinkingSuffixBudget, wantBudget: 8192, wantOK: true},
		{name: "unknown remains identity", input: "vendor-model(beta)", wantBase: "vendor-model(beta)", wantOK: false},
		{name: "negative invalid", input: "vendor-model(-2)", wantBase: "vendor-model(-2)", wantOK: false},
		{name: "unterminated", input: "vendor-model(high", wantBase: "vendor-model(high", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, cfg, ok := ParseThinkingSuffix(tt.input)
			if base != tt.wantBase || ok != tt.wantOK || cfg.Mode != tt.wantMode || cfg.Level != tt.wantLevel || cfg.Budget != tt.wantBudget {
				t.Fatalf("ParseThinkingSuffix(%q) = (%q, %+v, %v), want (%q, mode=%v level=%q budget=%d, %v)", tt.input, base, cfg, ok, tt.wantBase, tt.wantMode, tt.wantLevel, tt.wantBudget, tt.wantOK)
			}
			if got := RoutingModelName(tt.input); got != tt.wantBase {
				t.Fatalf("RoutingModelName(%q)=%q, want %q", tt.input, got, tt.wantBase)
			}
		})
	}
}

func TestConfig_ThinkingSuffixUsesRoutingIdentity(t *testing.T) {
	t.Parallel()

	cfg := &Config{ModelEntries: []ModelEntry{
		{Model: "gpt-5.6-luna(max)", RedirectModel: "upstream-luna(high)", FixedCostPerRequest: 0.25},
		{Model: "claude-sonnet(high)"},
	}}

	models := cfg.GetModels()
	if len(models) != 2 || models[0] != "gpt-5.6-luna" || models[1] != "claude-sonnet" {
		t.Fatalf("GetModels()=%v, want deduplicated routing identities", models)
	}
	if !cfg.SupportsModel("claude-sonnet(xhigh)") {
		t.Fatal("suffixed request did not match configured routing identity")
	}
	if redirect, ok := cfg.GetRedirectModel("gpt-5.6-luna(max)"); !ok || redirect != "upstream-luna(high)" {
		t.Fatalf("GetRedirectModel()=(%q,%v), want configured redirect", redirect, ok)
	}
	if matched, ok := cfg.FuzzyMatchModel("sonnet(max)"); !ok || matched != "claude-sonnet" {
		t.Fatalf("FuzzyMatchModel()=(%q,%v), want claude-sonnet", matched, ok)
	}
}

func TestConfig_ExplicitBaseModelTakesPrecedenceOverSuffixedEntry(t *testing.T) {
	t.Parallel()

	cfg := &Config{ModelEntries: []ModelEntry{
		{Model: "gpt-5.6-luna(max)", RedirectModel: "upstream-luna(high)", FixedCostPerRequest: 0.25},
		{Model: "gpt-5.6-luna"},
	}}

	if redirect, ok := cfg.GetRedirectModel("gpt-5.6-luna(high)"); ok || redirect != "" {
		t.Fatalf("GetRedirectModel()=(%q,%v), want explicit base entry without redirect", redirect, ok)
	}
	if cost, ok := cfg.GetFixedCostPerRequest("gpt-5.6-luna(xhigh)"); ok || cost != 0 {
		t.Fatalf("GetFixedCostPerRequest()=(%v,%v), want explicit base entry without fixed cost", cost, ok)
	}
}
