package sql_test

import (
	"context"
	"testing"
	"time"

	"ccLoad/internal/model"
)

func TestGetRecentCacheStatsUsesIndependentWindowsPerEntity(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, "recent_cache_stats.db")
	ctx := context.Background()

	channelA := createTestChannel(t, ctx, store, "recent-channel-a")
	channelB := createTestChannel(t, ctx, store, "recent-channel-b")
	tokenA := &model.AuthToken{Token: "recent-token-a", Description: "Recent A", IsActive: true}
	tokenB := &model.AuthToken{Token: "recent-token-b", Description: "Recent B", IsActive: true}
	if err := store.CreateAuthToken(ctx, tokenA); err != nil {
		t.Fatalf("create token A: %v", err)
	}
	if err := store.CreateAuthToken(ctx, tokenB); err != nil {
		t.Fatalf("create token B: %v", err)
	}

	base := time.UnixMilli(1_700_000_000_000)
	logs := make([]*model.LogEntry, 0, 120)
	for i := 0; i < 60; i++ {
		// A is older than B. A must still get its own latest 50 rows rather
		// than disappearing when a global latest-50 query is used.
		logs = append(logs, recentCacheTestLog(base.Add(time.Duration(i)*time.Millisecond), channelA, tokenA.ID, 100, 25, 5))
	}
	for i := 0; i < 60; i++ {
		logs = append(logs, recentCacheTestLog(base.Add(2*time.Hour+time.Duration(i)*time.Millisecond), channelB, tokenB.ID, 200, 80, 20))
	}
	if err := store.BatchAddLogs(ctx, logs); err != nil {
		t.Fatalf("add logs: %v", err)
	}

	stats, err := store.GetRecentCacheStats(ctx, 50)
	if err != nil {
		t.Fatalf("get recent cache stats: %v", err)
	}
	if stats.RequestLimit != 50 {
		t.Fatalf("request limit = %d, want 50", stats.RequestLimit)
	}

	channels := recentCacheStatsByID(stats.Channels)
	tokens := recentCacheStatsByID(stats.Tokens)
	assertRecentCacheStat(t, channels[channelA], 50, 5_000, 1_250, 250)
	assertRecentCacheStat(t, channels[channelB], 50, 10_000, 4_000, 1_000)
	assertRecentCacheStat(t, tokens[tokenA.ID], 50, 5_000, 1_250, 250)
	assertRecentCacheStat(t, tokens[tokenB.ID], 50, 10_000, 4_000, 1_000)
}

func recentCacheTestLog(at time.Time, channelID, tokenID int64, input, cacheRead, cacheCreation int) *model.LogEntry {
	return &model.LogEntry{
		Time:                     model.JSONTime{Time: at},
		Model:                    "gpt-4",
		LogSource:                model.LogSourceProxy,
		ChannelID:                channelID,
		AuthTokenID:              tokenID,
		StatusCode:               200,
		Message:                  "ok",
		InputTokens:              input,
		CacheReadInputTokens:     cacheRead,
		CacheCreationInputTokens: cacheCreation,
		Cache5mInputTokens:       cacheCreation,
	}
}

func recentCacheStatsByID(stats []model.RecentCacheStat) map[int64]model.RecentCacheStat {
	result := make(map[int64]model.RecentCacheStat, len(stats))
	for _, stat := range stats {
		result[stat.ID] = stat
	}
	return result
}

func assertRecentCacheStat(t *testing.T, stat model.RecentCacheStat, requestCount int, input, cacheRead, cacheCreation int64) {
	t.Helper()
	if stat.RequestCount != requestCount || stat.InputTokens != input || stat.CacheReadTokens != cacheRead || stat.CacheCreationTokens != cacheCreation {
		t.Fatalf("stat = %+v, want count=%d input=%d read=%d creation=%d", stat, requestCount, input, cacheRead, cacheCreation)
	}
}
