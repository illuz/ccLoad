package sql

import (
	"context"
	"fmt"

	"ccLoad/internal/model"
)

const defaultRecentCacheRequestLimit = 50

func normalizeRecentCacheRequestLimit(limit int) int {
	if limit <= 0 {
		return defaultRecentCacheRequestLimit
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

// GetRecentCacheStats aggregates cache usage from each entity's latest requests.
// The cutoff is found with a correlated indexed seek rather than a window
// function so the query remains compatible with MySQL 5.6.
func (s *SQLStore) GetRecentCacheStats(ctx context.Context, requestLimit int) (*model.RecentCacheStats, error) {
	requestLimit = normalizeRecentCacheRequestLimit(requestLimit)

	channels, err := s.getRecentCacheStatsByEntity(ctx, "channels", "channel_id", requestLimit)
	if err != nil {
		return nil, fmt.Errorf("query recent channel cache stats: %w", err)
	}
	tokens, err := s.getRecentCacheStatsByEntity(ctx, "auth_tokens", "auth_token_id", requestLimit)
	if err != nil {
		return nil, fmt.Errorf("query recent token cache stats: %w", err)
	}

	return &model.RecentCacheStats{
		RequestLimit: requestLimit,
		Channels:     channels,
		Tokens:       tokens,
	}, nil
}

// getRecentCacheStatsByEntity uses the entity table as a scope so each entity
// gets its own latest N rows. A NULL cutoff means the entity has fewer than N
// proxy requests, in which case all of its available rows are included.
func (s *SQLStore) getRecentCacheStatsByEntity(ctx context.Context, entityTable, logEntityColumn string, requestLimit int) ([]model.RecentCacheStat, error) {
	// entityTable and logEntityColumn are internal constants selected by the
	// caller; only the numeric limit and log source are query parameters.
	//nolint:gosec // SQL identifiers are fixed internal constants
	query := fmt.Sprintf(`
		SELECT entity_scope.entity_id,
			COUNT(log_entry.id) AS request_count,
			SUM(CASE WHEN log_entry.status_code >= 200 AND log_entry.status_code < 300 THEN 1 ELSE 0 END) AS success_count,
			SUM(CASE WHEN (log_entry.status_code < 200 OR log_entry.status_code >= 300) AND log_entry.status_code != 499 THEN 1 ELSE 0 END) AS failure_count,
			COALESCE(SUM(log_entry.input_tokens), 0) AS input_tokens,
			COALESCE(SUM(log_entry.cache_read_input_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(log_entry.cache_creation_input_tokens), 0) AS cache_creation_tokens
		FROM (
			SELECT e.id AS entity_id,
				(
					SELECT recent.id
					FROM logs recent
					WHERE recent.%s = e.id
						AND recent.log_source = ?
					ORDER BY recent.time DESC, recent.id DESC
					LIMIT 1 OFFSET ?
				) AS cutoff_id
			FROM %s e
		) entity_scope
		JOIN logs log_entry ON log_entry.%s = entity_scope.entity_id
		LEFT JOIN logs cutoff ON cutoff.id = entity_scope.cutoff_id
		WHERE log_entry.log_source = ?
			AND (
				entity_scope.cutoff_id IS NULL
				OR log_entry.time > cutoff.time
				OR (log_entry.time = cutoff.time AND log_entry.id >= cutoff.id)
			)
		GROUP BY entity_scope.entity_id
		ORDER BY entity_scope.entity_id ASC
	`, logEntityColumn, entityTable, logEntityColumn)

	rows, err := s.db.QueryContext(ctx, query, model.LogSourceProxy, requestLimit-1, model.LogSourceProxy)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	stats := make([]model.RecentCacheStat, 0)
	for rows.Next() {
		var stat model.RecentCacheStat
		if err := rows.Scan(
			&stat.ID,
			&stat.RequestCount,
			&stat.SuccessCount,
			&stat.FailureCount,
			&stat.InputTokens,
			&stat.CacheReadTokens,
			&stat.CacheCreationTokens,
		); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stats, nil
}
