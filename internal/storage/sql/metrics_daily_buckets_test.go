package sql_test

import (
	"context"
	"testing"
	"time"

	"ccLoad/internal/model"
)

func TestAggregateRangeWithFilter_DailyBucketsUseLocalCalendarDays(t *testing.T) {
	store := newTestStore(t, "metrics_daily_buckets.db")
	ctx := context.Background()

	channel, err := store.CreateConfig(ctx, &model.Config{
		Name:     "daily-metrics",
		URL:      "https://example.com",
		Priority: 1,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("CreateConfig: %v", err)
	}

	loc := time.FixedZone("UTC+8", 8*60*60)
	dayOne := time.Date(2026, time.July, 6, 0, 0, 0, 0, loc)
	dayTwo := dayOne.AddDate(0, 0, 1)
	until := dayTwo.AddDate(0, 0, 1).Add(-time.Nanosecond)
	logs := []*model.LogEntry{
		{Time: model.JSONTime{Time: dayOne.Add(30 * time.Minute)}, ChannelID: channel.ID, Model: "model", StatusCode: 200, InputTokens: 10},
		{Time: model.JSONTime{Time: dayOne.Add(23*time.Hour + 30*time.Minute)}, ChannelID: channel.ID, Model: "model", StatusCode: 200, InputTokens: 20},
		{Time: model.JSONTime{Time: dayTwo.Add(30 * time.Minute)}, ChannelID: channel.ID, Model: "model", StatusCode: 200, InputTokens: 30},
	}
	if err := store.BatchAddLogs(ctx, logs); err != nil {
		t.Fatalf("BatchAddLogs: %v", err)
	}

	points, err := store.AggregateRangeWithFilter(ctx, dayOne, until, 24*time.Hour, nil)
	if err != nil {
		t.Fatalf("AggregateRangeWithFilter: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("point count=%d, want 2", len(points))
	}
	if !points[0].Ts.Equal(dayOne) || !points[1].Ts.Equal(dayTwo) {
		t.Fatalf("point timestamps=%v, %v; want %v, %v", points[0].Ts, points[1].Ts, dayOne, dayTwo)
	}
	if points[0].InputTokens != 30 || points[1].InputTokens != 30 {
		t.Fatalf("input tokens=%d, %d; want 30, 30", points[0].InputTokens, points[1].InputTokens)
	}
}
