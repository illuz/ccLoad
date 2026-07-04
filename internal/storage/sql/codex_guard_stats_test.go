package sql_test

import (
	"context"
	"testing"
	"time"

	"ccLoad/internal/model"
	"ccLoad/internal/util"
)

func TestCodexGuardSummaryAndLogFilter(t *testing.T) {
	store := newTestStore(t, "codex_guard_stats.db")
	ctx := context.Background()

	codexCfg, err := store.CreateConfig(ctx, &model.Config{
		Name:        "codex-main",
		URL:         "https://example.com",
		Priority:    10,
		Enabled:     true,
		ChannelType: util.ChannelTypeCodex,
		ModelEntries: []model.ModelEntry{
			{Model: "gpt-5-codex"},
		},
	})
	if err != nil {
		t.Fatalf("CreateConfig codex failed: %v", err)
	}
	openaiCfg, err := store.CreateConfig(ctx, &model.Config{
		Name:        "openai-main",
		URL:         "https://example.com",
		Priority:    20,
		Enabled:     true,
		ChannelType: util.ChannelTypeOpenAI,
		ModelEntries: []model.ModelEntry{
			{Model: "gpt-4o"},
		},
	})
	if err != nil {
		t.Fatalf("CreateConfig openai failed: %v", err)
	}

	now := time.Now()
	start := now.Add(-time.Minute)
	end := now.Add(time.Minute)
	if err := store.BatchAddLogs(ctx, []*model.LogEntry{
		{
			Time:            model.JSONTime{Time: now},
			Model:           "gpt-5-codex",
			ChannelID:       codexCfg.ID,
			LogSource:       model.LogSourceProxy,
			StatusCode:      util.StatusCodexReasoningGuard,
			Message:         "upstream status 595 [codex_guard reasoning_tokens=516 match=518n-2]",
			ReasoningTokens: 516,
		},
		{
			Time:       model.JSONTime{Time: now},
			Model:      "gpt-5-codex",
			ChannelID:  codexCfg.ID,
			LogSource:  model.LogSourceProxy,
			StatusCode: 200,
			Message:    "ok [retried_after_codex_guard]",
		},
		{
			Time:       model.JSONTime{Time: now},
			Model:      "gpt-5-codex",
			ChannelID:  codexCfg.ID,
			LogSource:  model.LogSourceProxy,
			StatusCode: 200,
			Message:    "ok",
		},
		{
			Time:            model.JSONTime{Time: now},
			Model:           "gpt-4o",
			ChannelID:       openaiCfg.ID,
			LogSource:       model.LogSourceProxy,
			StatusCode:      util.StatusCodexReasoningGuard,
			Message:         "upstream status 595 [codex_guard reasoning_tokens=516 match=518n-2]",
			ReasoningTokens: 516,
		},
		{
			Time:       model.JSONTime{Time: now},
			Model:      "gpt-5-codex",
			ChannelID:  codexCfg.ID,
			LogSource:  model.LogSourceProxy,
			StatusCode: 499,
			Message:    "client cancelled",
		},
	}); err != nil {
		t.Fatalf("BatchAddLogs failed: %v", err)
	}

	summary, err := store.GetCodexGuardSummary(ctx, start, end)
	if err != nil {
		t.Fatalf("GetCodexGuardSummary failed: %v", err)
	}
	if summary.TotalCodexRequests != 3 || summary.HitCount != 1 || summary.RetrySuccessCount != 1 || summary.FinalFailureCount != 0 {
		t.Fatalf("unexpected summary counts: %+v", summary)
	}
	if summary.HitRate < 0.33 || summary.HitRate > 0.34 {
		t.Fatalf("hit_rate=%v, want ~0.333", summary.HitRate)
	}
	if summary.RetrySuccessRate != 1 {
		t.Fatalf("retry_success_rate=%v, want 1", summary.RetrySuccessRate)
	}
	if len(summary.ByReasoningTokens) != 1 || summary.ByReasoningTokens[0].Key != "516" || summary.ByReasoningTokens[0].Count != 1 {
		t.Fatalf("unexpected reasoning breakdown: %+v", summary.ByReasoningTokens)
	}
	if len(summary.ByChannel) != 1 || summary.ByChannel[0].Name != "codex-main" {
		t.Fatalf("unexpected channel breakdown: %+v", summary.ByChannel)
	}

	hitFilter := &model.LogFilter{LogSource: model.LogSourceProxy, CodexGuardMode: model.CodexGuardFilterHit}
	_, hitTotal, err := store.ListLogsRangeWithCount(ctx, start, end, 20, 0, hitFilter)
	if err != nil {
		t.Fatalf("ListLogsRangeWithCount(hit) failed: %v", err)
	}
	if hitTotal != 2 {
		t.Fatalf("hit filter total=%d, want 2 (codex + openai hits)", hitTotal)
	}

	retryFilter := &model.LogFilter{LogSource: model.LogSourceProxy, CodexGuardMode: model.CodexGuardFilterRetrySuccess}
	_, retryTotal, err := store.ListLogsRangeWithCount(ctx, start, end, 20, 0, retryFilter)
	if err != nil {
		t.Fatalf("ListLogsRangeWithCount(retry) failed: %v", err)
	}
	if retryTotal != 1 {
		t.Fatalf("retry filter total=%d, want 1", retryTotal)
	}
}
