package sql_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"ccLoad/internal/model"
)

func newJSONTime(t time.Time) model.JSONTime {
	return model.JSONTime{Time: t}
}

func TestLog_AddAndList(t *testing.T) {
	t.Parallel()

	store := newTestStore(t, "logs.db")

	ctx := context.Background()
	channelID := createTestChannel(t, ctx, store, "log-test-channel")

	now := time.Now()
	log := &model.LogEntry{
		Time:                  newJSONTime(now),
		Model:                 "gpt-4",
		ChannelID:             channelID,
		StatusCode:            200,
		Message:               "success",
		Duration:              1.5,
		IsStreaming:           true,
		FirstByteTime:         0.25,
		RequestID:             "123e4567-e89b-42d3-a456-426614174000",
		AttemptNumber:         2,
		EndToEndFirstByteTime: 1.25,
		APIKeyUsed:            "abcd...efgh",
	}
	if err := store.AddLog(ctx, log); err != nil {
		t.Fatalf("add log: %v", err)
	}
	// AddLog 方法不返回 ID，不需要检查

	since := now.Add(-1 * time.Hour)
	logs, err := store.ListLogs(ctx, since, 10, 0, nil)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 log, got %d", len(logs))
	}
	if len(logs) > 0 && logs[0].Model != "gpt-4" {
		t.Errorf("model: got %q, want %q", logs[0].Model, "gpt-4")
	}
	if len(logs) > 0 {
		got := logs[0]
		if got.RequestID != log.RequestID {
			t.Errorf("request_id: got %q, want %q", got.RequestID, log.RequestID)
		}
		if got.AttemptNumber != log.AttemptNumber {
			t.Errorf("attempt_number: got %d, want %d", got.AttemptNumber, log.AttemptNumber)
		}
		if got.FirstByteTime != log.FirstByteTime {
			t.Errorf("first_byte_time: got %v, want %v", got.FirstByteTime, log.FirstByteTime)
		}
		if got.EndToEndFirstByteTime != log.EndToEndFirstByteTime {
			t.Errorf("end_to_end_first_byte_time: got %v, want %v", got.EndToEndFirstByteTime, log.EndToEndFirstByteTime)
		}
	}
}

func TestLog_AddAndListPersistsReasoningTokens(t *testing.T) {
	t.Parallel()

	store := newTestStore(t, "logs_reasoning_tokens.db")

	ctx := context.Background()
	channelID := createTestChannel(t, ctx, store, "log-reasoning-token-channel")

	now := time.Now()
	if err := store.AddLog(ctx, &model.LogEntry{
		Time:            newJSONTime(now),
		Model:           "gpt-5-codex",
		ChannelID:       channelID,
		StatusCode:      200,
		Message:         "success",
		ThinkingEffort:  "xhigh",
		ReasoningTokens: 1234,
	}); err != nil {
		t.Fatalf("add log: %v", err)
	}

	logs, err := store.ListLogs(ctx, now.Add(-time.Hour), 10, 0, nil)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("len(logs)=%d, want 1", len(logs))
	}
	if logs[0].ReasoningTokens != 1234 {
		t.Fatalf("reasoning_tokens=%d, want 1234", logs[0].ReasoningTokens)
	}
}

func TestLog_UpstreamResponseModelAuditPersistenceAndFilter(t *testing.T) {
	t.Parallel()

	store := newTestStore(t, "logs_upstream_model_audit.db")
	ctx := context.Background()
	channelID := createTestChannel(t, ctx, store, "log-upstream-model-audit-channel")
	now := time.Now()
	mismatch := true
	matched := false

	if err := store.BatchAddLogs(ctx, []*model.LogEntry{
		{
			Time:                  newJSONTime(now),
			Model:                 "gpt-5.4",
			ActualModel:           "gpt-5.4-upstream",
			UpstreamResponseModel: "gpt-5.4-fallback",
			UpstreamModelMismatch: &mismatch,
			ChannelID:             channelID,
			StatusCode:            200,
			Message:               "mismatch",
		},
		{
			Time:                  newJSONTime(now.Add(time.Millisecond)),
			Model:                 "gpt-5.4",
			UpstreamResponseModel: "gpt-5.4",
			UpstreamModelMismatch: &matched,
			ChannelID:             channelID,
			StatusCode:            200,
			Message:               "match",
		},
	}); err != nil {
		t.Fatalf("add logs: %v", err)
	}

	logs, err := store.ListLogs(ctx, now.Add(-time.Hour), 10, 0, &model.LogFilter{UpstreamModelMismatch: &mismatch})
	if err != nil {
		t.Fatalf("list mismatches: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("mismatch logs=%d, want 1", len(logs))
	}
	got := logs[0]
	if got.UpstreamResponseModel != "gpt-5.4-fallback" {
		t.Fatalf("upstream_response_model=%q", got.UpstreamResponseModel)
	}
	if got.UpstreamModelMismatch == nil || !*got.UpstreamModelMismatch {
		t.Fatalf("upstream_model_mismatch=%v, want true", got.UpstreamModelMismatch)
	}

	logs, err = store.ListLogs(ctx, now.Add(-time.Hour), 10, 0, &model.LogFilter{UpstreamModelMismatch: &matched})
	if err != nil {
		t.Fatalf("list matches: %v", err)
	}
	if len(logs) != 1 || logs[0].UpstreamModelMismatch == nil || *logs[0].UpstreamModelMismatch {
		t.Fatalf("matched logs=%+v, want one false audit result", logs)
	}
}

func TestLog_AddLogWithDebugDataAssignsIDWithoutPersistingBody(t *testing.T) {
	t.Parallel()

	store := newTestStore(t, "add_log_debug.db")
	ctx := context.Background()
	channelID := createTestChannel(t, ctx, store, "add-log-debug-channel")

	now := time.Now()
	entry := &model.LogEntry{
		Time:       newJSONTime(now),
		Model:      "gpt-4",
		ChannelID:  channelID,
		StatusCode: 200,
		Message:    "ok",
		DebugData: &model.DebugLogEntry{
			CreatedAt:   now.Unix(),
			ReqMethod:   http.MethodPost,
			ReqURL:      "https://api.example.com/v1/chat/completions",
			ReqHeaders:  `{"Content-Type":"application/json"}`,
			ReqBody:     []byte(`{"model":"gpt-4"}`),
			RespStatus:  200,
			RespHeaders: `{"Content-Type":"application/json"}`,
			RespBody:    []byte(`{"ok":true}`),
		},
	}
	if err := store.AddLog(ctx, entry); err != nil {
		t.Fatalf("add log with debug data: %v", err)
	}
	if entry.ID <= 0 {
		t.Fatalf("entry.ID=%d, want assigned database ID", entry.ID)
	}

	logs, err := store.ListLogsRange(ctx, now.Add(-time.Minute), now.Add(time.Minute), 10, 0, nil)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("len(logs)=%d, want 1", len(logs))
	}
	if logs[0].ID != entry.ID {
		t.Fatalf("stored log ID=%d, entry ID=%d", logs[0].ID, entry.ID)
	}
}

func TestLog_BatchAdd(t *testing.T) {
	t.Parallel()

	store := newTestStore(t, "batch_logs.db")

	ctx := context.Background()
	channelID := createTestChannel(t, ctx, store, "batch-log-channel")

	now := time.Now()
	logs := []*model.LogEntry{
		{
			Time:       newJSONTime(now),
			Model:      "gpt-4",
			ChannelID:  channelID,
			StatusCode: 200,
			Message:    "success 1",
			Duration:   1.0,
			APIKeyUsed: "key1...1key",
		},
		{
			Time:       newJSONTime(now),
			Model:      "claude-3",
			ChannelID:  channelID,
			StatusCode: 200,
			Message:    "success 2",
			Duration:   2.0,
			APIKeyUsed: "key2...2key",
		},
		{
			Time:       newJSONTime(now),
			Model:      "gpt-4",
			ChannelID:  channelID,
			StatusCode: 500,
			Message:    "error",
			Duration:   0.5,
			APIKeyUsed: "key3...3key",
		},
	}

	if err := store.BatchAddLogs(ctx, logs); err != nil {
		t.Fatalf("batch add logs: %v", err)
	}
	// BatchAddLogs 方法不返回 ID，不需要检查

	since := now.Add(-1 * time.Hour)
	count, err := store.CountLogs(ctx, since, nil)
	if err != nil {
		t.Fatalf("count logs: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 logs, got %d", count)
	}
}

func TestLog_ListRange(t *testing.T) {
	t.Parallel()

	store := newTestStore(t, "range_logs.db")

	ctx := context.Background()
	channelID := createTestChannel(t, ctx, store, "range-log-channel")

	now := time.Now()
	logs := []*model.LogEntry{
		{
			Time:       newJSONTime(now.Add(-2 * time.Hour)),
			Model:      "old-model",
			ChannelID:  channelID,
			StatusCode: 200,
			Message:    "old log",
			Duration:   1.0,
			APIKeyUsed: "key1...1key",
		},
		{
			Time:       newJSONTime(now.Add(-30 * time.Minute)),
			Model:      "recent-model",
			ChannelID:  channelID,
			StatusCode: 200,
			Message:    "recent log",
			Duration:   1.0,
			APIKeyUsed: "key2...2key",
		},
	}
	if err := store.BatchAddLogs(ctx, logs); err != nil {
		t.Fatalf("batch add logs: %v", err)
	}

	startTime := now.Add(-1 * time.Hour)
	endTime := now

	rangeLogs, err := store.ListLogsRange(ctx, startTime, endTime, 100, 0, nil)
	if err != nil {
		t.Fatalf("list logs range: %v", err)
	}
	if len(rangeLogs) != 1 {
		t.Errorf("expected 1 log in range, got %d", len(rangeLogs))
	}
	if len(rangeLogs) > 0 && rangeLogs[0].Model != "recent-model" {
		t.Errorf("model: got %q, want %q", rangeLogs[0].Model, "recent-model")
	}

	rangeCount, err := store.CountLogsRange(ctx, startTime, endTime, nil)
	if err != nil {
		t.Fatalf("count logs range: %v", err)
	}
	if rangeCount != 1 {
		t.Errorf("expected 1 log in range count, got %d", rangeCount)
	}
}

func TestLog_Pagination(t *testing.T) {
	t.Parallel()

	store := newTestStore(t, "pagination_logs.db")

	ctx := context.Background()
	channelID := createTestChannel(t, ctx, store, "pagination-channel")

	now := time.Now()
	logs := make([]*model.LogEntry, 10)
	for i := 0; i < 10; i++ {
		logs[i] = &model.LogEntry{
			Time:       newJSONTime(now),
			Model:      "gpt-4",
			ChannelID:  channelID,
			StatusCode: 200,
			Message:    "log " + string(rune('0'+i)),
			Duration:   float64(i),
			APIKeyUsed: "key...key",
		}
	}
	if err := store.BatchAddLogs(ctx, logs); err != nil {
		t.Fatalf("batch add logs: %v", err)
	}

	since := now.Add(-1 * time.Hour)

	page1, err := store.ListLogs(ctx, since, 5, 0, nil)
	if err != nil {
		t.Fatalf("list logs page 1: %v", err)
	}
	if len(page1) != 5 {
		t.Errorf("page 1: expected 5 logs, got %d", len(page1))
	}

	page2, err := store.ListLogs(ctx, since, 5, 5, nil)
	if err != nil {
		t.Fatalf("list logs page 2: %v", err)
	}
	if len(page2) != 5 {
		t.Errorf("page 2: expected 5 logs, got %d", len(page2))
	}

	seen := make(map[int64]struct{}, len(page1))
	for _, entry := range page1 {
		seen[entry.ID] = struct{}{}
	}
	for _, entry := range page2 {
		if _, ok := seen[entry.ID]; ok {
			t.Fatalf("pages should not overlap, overlapping id=%d", entry.ID)
		}
	}
}

func TestLog_ListRangeWithCount_PreservesZeroCostMultiplier(t *testing.T) {
	t.Parallel()

	store := newTestStore(t, "logs_zero_multiplier.db")
	ctx := context.Background()
	channelID := createTestChannel(t, ctx, store, "free-log-channel")

	now := time.Now()
	if err := store.AddLog(ctx, &model.LogEntry{
		Time:           newJSONTime(now),
		Model:          "gpt-5.4-mini",
		ChannelID:      channelID,
		StatusCode:     200,
		Message:        "success",
		Duration:       1.2,
		APIKeyUsed:     "key...key",
		Cost:           0.019,
		CostMultiplier: 0,
	}); err != nil {
		t.Fatalf("add log: %v", err)
	}

	startTime := now.Add(-1 * time.Minute)
	endTime := now.Add(1 * time.Minute)

	logs, total, err := store.ListLogsRangeWithCount(ctx, startTime, endTime, 10, 0, nil)
	if err != nil {
		t.Fatalf("ListLogsRangeWithCount failed: %v", err)
	}
	if total != 1 {
		t.Fatalf("total=%d, want 1", total)
	}
	if len(logs) != 1 {
		t.Fatalf("len(logs)=%d, want 1", len(logs))
	}
	if logs[0].CostMultiplier != 0 {
		t.Fatalf("cost_multiplier=%v, want 0", logs[0].CostMultiplier)
	}
}
