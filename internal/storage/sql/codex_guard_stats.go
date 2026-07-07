package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ccLoad/internal/model"
	"ccLoad/internal/util"
)

const codexGuardTopLimit = 5

func codexGuardLikeArgs() (hitLike, retryLike string) {
	return "%" + model.CodexGuardLogMarker + "%", "%" + model.CodexGuardRetrySuccessMarker + "%"
}

func codexGuardHitCondition(alias string) (string, []any) {
	if alias != "" {
		alias += "."
	}
	hitLike, retryLike := codexGuardLikeArgs()
	return fmt.Sprintf("(%sstatus_code = ? OR (%smessage LIKE ? AND %smessage NOT LIKE ?))", alias, alias, alias),
		[]any{util.StatusCodexReasoningGuard, hitLike, retryLike}
}

func codexGuardRetrySuccessCondition(alias string) (string, []any) {
	if alias != "" {
		alias += "."
	}
	_, retryLike := codexGuardLikeArgs()
	return fmt.Sprintf("(%sstatus_code >= 200 AND %sstatus_code < 300 AND %smessage LIKE ?)", alias, alias, alias),
		[]any{retryLike}
}

func buildCodexGuardBaseWhere(alias string, channelIDs []int64) (string, []any) {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	conditions := []string{
		prefix + "time >= ?",
		prefix + "time <= ?",
		prefix + "status_code != 499",
		prefix + "log_source = ?",
		prefix + "channel_id > 0",
	}
	args := make([]any, 0, 3+len(channelIDs))
	args = append(args, model.LogSourceProxy)

	if len(channelIDs) > 0 {
		placeholders := make([]string, len(channelIDs))
		for i, id := range channelIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		conditions = append(conditions, fmt.Sprintf("%schannel_id IN (%s)", prefix, strings.Join(placeholders, ",")))
	}

	return strings.Join(conditions, " AND "), args
}

func codexGuardBaseArgs(startTime, endTime time.Time, tail []any) []any {
	args := make([]any, 0, 2+len(tail))
	args = append(args, startTime.UnixMilli(), endTime.UnixMilli())
	args = append(args, tail...)
	return args
}

// GetCodexGuardSummary 汇总 Codex Guard 命中与后续重试恢复情况。
func (s *SQLStore) GetCodexGuardSummary(ctx context.Context, startTime, endTime time.Time) (*model.CodexGuardSummary, error) {
	summary := &model.CodexGuardSummary{}

	channelIDs, isEmpty, err := s.resolveChannelFilter(ctx, &model.LogFilter{
		ChannelType: util.ChannelTypeCodex,
		LogSource:   model.LogSourceProxy,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve codex channels: %w", err)
	}
	if isEmpty {
		return summary, nil
	}

	if err := s.fillCodexGuardSummaryCounts(ctx, summary, startTime, endTime, channelIDs); err != nil {
		return nil, err
	}
	if err := s.fillCodexGuardRequestCounts(ctx, summary, startTime, endTime, channelIDs); err != nil {
		return nil, err
	}
	if summary.HitCount > summary.RetrySuccessCount {
		summary.FinalFailureCount = summary.HitCount - summary.RetrySuccessCount
	}
	if summary.TotalCodexRequests > 0 {
		summary.HitRate = float64(summary.HitCount) / float64(summary.TotalCodexRequests)
	}
	if summary.HitCount > 0 {
		summary.RetrySuccessRate = float64(summary.RetrySuccessCount) / float64(summary.HitCount)
	}
	if summary.RequestHitCount > 0 {
		summary.RequestRescueRate = float64(summary.RequestRescuedCount) / float64(summary.RequestHitCount)
	}

	var aggErr error
	if summary.ByReasoningTokens, aggErr = s.codexGuardReasoningTokenCounts(ctx, startTime, endTime, channelIDs); aggErr != nil {
		return nil, aggErr
	}
	if summary.ByToken, aggErr = s.codexGuardTokenCounts(ctx, startTime, endTime, channelIDs); aggErr != nil {
		return nil, aggErr
	}
	if summary.ByChannel, aggErr = s.codexGuardChannelCounts(ctx, startTime, endTime, channelIDs); aggErr != nil {
		return nil, aggErr
	}
	if summary.ByModel, aggErr = s.codexGuardModelCounts(ctx, startTime, endTime, channelIDs); aggErr != nil {
		return nil, aggErr
	}

	return summary, nil
}

func (s *SQLStore) fillCodexGuardSummaryCounts(ctx context.Context, summary *model.CodexGuardSummary, startTime, endTime time.Time, channelIDs []int64) error {
	baseWhere, baseTailArgs := buildCodexGuardBaseWhere("l", channelIDs)
	hitCond, hitArgs := codexGuardHitCondition("l")
	retryCond, retryArgs := codexGuardRetrySuccessCondition("l")

	query := fmt.Sprintf(`
		SELECT
			COUNT(*) AS total_codex_requests,
			COALESCE(SUM(CASE WHEN %s THEN 1 ELSE 0 END), 0) AS hit_count,
			COALESCE(SUM(CASE WHEN %s THEN 1 ELSE 0 END), 0) AS retry_success_count
		FROM logs l
		WHERE %s
	`, hitCond, retryCond, baseWhere)

	args := make([]any, 0, len(hitArgs)+len(retryArgs)+2+len(baseTailArgs))
	args = append(args, hitArgs...)
	args = append(args, retryArgs...)
	args = append(args, codexGuardBaseArgs(startTime, endTime, baseTailArgs)...)

	if err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&summary.TotalCodexRequests,
		&summary.HitCount,
		&summary.RetrySuccessCount,
	); err != nil {
		return fmt.Errorf("query codex guard counts: %w", err)
	}
	return nil
}

type codexGuardRequestState struct {
	hit     bool
	rescued bool
}

func (s *SQLStore) fillCodexGuardRequestCounts(ctx context.Context, summary *model.CodexGuardSummary, startTime, endTime time.Time, channelIDs []int64) error {
	baseWhere, baseTailArgs := buildCodexGuardBaseWhere("l", channelIDs)
	hitCond, hitArgs := codexGuardHitCondition("l")
	retryCond, retryArgs := codexGuardRetrySuccessCondition("l")

	query := fmt.Sprintf(`
		SELECT l.status_code, l.message
		FROM logs l
		WHERE %s AND (%s OR %s)
	`, baseWhere, hitCond, retryCond)

	args := codexGuardBaseArgs(startTime, endTime, baseTailArgs)
	args = append(args, hitArgs...)
	args = append(args, retryArgs...)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query codex guard request counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	byTrace := make(map[string]*codexGuardRequestState)
	for rows.Next() {
		var statusCode int
		var message string
		if err := rows.Scan(&statusCode, &message); err != nil {
			return err
		}
		traceID := extractCodexGuardTraceID(message)
		if traceID == "" {
			continue
		}
		state := byTrace[traceID]
		if state == nil {
			state = &codexGuardRequestState{}
			byTrace[traceID] = state
		}
		if isCodexGuardHitLog(statusCode, message) {
			state.hit = true
		}
		if isCodexGuardRetrySuccessLog(statusCode, message) {
			state.rescued = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, state := range byTrace {
		if !state.hit {
			continue
		}
		summary.RequestHitCount++
		if state.rescued {
			summary.RequestRescuedCount++
		}
	}
	if summary.RequestHitCount > summary.RequestRescuedCount {
		summary.RequestFailureCount = summary.RequestHitCount - summary.RequestRescuedCount
	}
	return nil
}

func isCodexGuardHitLog(statusCode int, message string) bool {
	if statusCode == util.StatusCodexReasoningGuard {
		return true
	}
	return strings.Contains(message, model.CodexGuardLogMarker) &&
		!strings.Contains(message, model.CodexGuardRetrySuccessMarker)
}

func isCodexGuardRetrySuccessLog(statusCode int, message string) bool {
	return statusCode >= 200 && statusCode < 300 &&
		strings.Contains(message, model.CodexGuardRetrySuccessMarker)
}

func extractCodexGuardTraceID(message string) string {
	needle := model.CodexGuardTraceMarker + "="
	idx := strings.Index(message, needle)
	if idx < 0 {
		return ""
	}
	start := idx + len(needle)
	end := start
	for end < len(message) && isCodexGuardTraceChar(message[end]) {
		end++
	}
	if end == start {
		return ""
	}
	return message[start:end]
}

func isCodexGuardTraceChar(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '-' || b == '_'
}

func (s *SQLStore) codexGuardReasoningTokenCounts(ctx context.Context, startTime, endTime time.Time, channelIDs []int64) ([]model.CodexGuardCountEntry, error) {
	baseWhere, baseTailArgs := buildCodexGuardBaseWhere("l", channelIDs)
	hitCond, hitArgs := codexGuardHitCondition("l")
	query := fmt.Sprintf(`
		SELECT COALESCE(l.reasoning_tokens, 0) AS reasoning_tokens, COUNT(*) AS count
		FROM logs l
		WHERE %s AND %s AND COALESCE(l.reasoning_tokens, 0) > 0
		GROUP BY COALESCE(l.reasoning_tokens, 0)
		ORDER BY count DESC, reasoning_tokens DESC
		LIMIT ?
	`, baseWhere, hitCond)

	args := codexGuardBaseArgs(startTime, endTime, baseTailArgs)
	args = append(args, hitArgs...)
	args = append(args, codexGuardTopLimit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query codex guard reasoning counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []model.CodexGuardCountEntry
	for rows.Next() {
		var tokens int64
		var count int64
		if err := rows.Scan(&tokens, &count); err != nil {
			return nil, err
		}
		key := strconv.FormatInt(tokens, 10)
		out = append(out, model.CodexGuardCountEntry{Key: key, Name: key, Count: count})
	}
	return out, rows.Err()
}

func (s *SQLStore) codexGuardTokenCounts(ctx context.Context, startTime, endTime time.Time, channelIDs []int64) ([]model.CodexGuardCountEntry, error) {
	baseWhere, baseTailArgs := buildCodexGuardBaseWhere("l", channelIDs)
	hitCond, hitArgs := codexGuardHitCondition("l")
	query := fmt.Sprintf(`
		SELECT l.auth_token_id, COALESCE(NULLIF(MAX(at.description), ''), '') AS token_name, COUNT(*) AS count
		FROM logs l
		LEFT JOIN auth_tokens at ON at.id = l.auth_token_id
		WHERE %s AND %s
		GROUP BY l.auth_token_id
		ORDER BY count DESC, l.auth_token_id ASC
		LIMIT ?
	`, baseWhere, hitCond)

	args := codexGuardBaseArgs(startTime, endTime, baseTailArgs)
	args = append(args, hitArgs...)
	args = append(args, codexGuardTopLimit)
	return scanCodexGuardIDNameCounts(ctx, s.db, query, args, "Token")
}

func (s *SQLStore) codexGuardChannelCounts(ctx context.Context, startTime, endTime time.Time, channelIDs []int64) ([]model.CodexGuardCountEntry, error) {
	baseWhere, baseTailArgs := buildCodexGuardBaseWhere("l", channelIDs)
	hitCond, hitArgs := codexGuardHitCondition("l")
	query := fmt.Sprintf(`
		SELECT l.channel_id, COALESCE(NULLIF(MAX(c.name), ''), '') AS channel_name, COUNT(*) AS count
		FROM logs l
		LEFT JOIN channels c ON c.id = l.channel_id
		WHERE %s AND %s
		GROUP BY l.channel_id
		ORDER BY count DESC, l.channel_id ASC
		LIMIT ?
	`, baseWhere, hitCond)

	args := codexGuardBaseArgs(startTime, endTime, baseTailArgs)
	args = append(args, hitArgs...)
	args = append(args, codexGuardTopLimit)
	return scanCodexGuardIDNameCounts(ctx, s.db, query, args, "Channel")
}

func (s *SQLStore) codexGuardModelCounts(ctx context.Context, startTime, endTime time.Time, channelIDs []int64) ([]model.CodexGuardCountEntry, error) {
	baseWhere, baseTailArgs := buildCodexGuardBaseWhere("l", channelIDs)
	hitCond, hitArgs := codexGuardHitCondition("l")
	query := fmt.Sprintf(`
		SELECT COALESCE(NULLIF(l.model, ''), '(unknown)') AS model_name, COUNT(*) AS count
		FROM logs l
		WHERE %s AND %s
		GROUP BY COALESCE(NULLIF(l.model, ''), '(unknown)')
		ORDER BY count DESC, model_name ASC
		LIMIT ?
	`, baseWhere, hitCond)

	args := codexGuardBaseArgs(startTime, endTime, baseTailArgs)
	args = append(args, hitArgs...)
	args = append(args, codexGuardTopLimit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query codex guard model counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []model.CodexGuardCountEntry
	for rows.Next() {
		var name string
		var count int64
		if err := rows.Scan(&name, &count); err != nil {
			return nil, err
		}
		out = append(out, model.CodexGuardCountEntry{Key: name, Name: name, Count: count})
	}
	return out, rows.Err()
}

type codexGuardQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func scanCodexGuardIDNameCounts(ctx context.Context, db codexGuardQueryer, query string, args []any, fallbackPrefix string) ([]model.CodexGuardCountEntry, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query codex guard %s counts: %w", strings.ToLower(fallbackPrefix), err)
	}
	defer func() { _ = rows.Close() }()

	var out []model.CodexGuardCountEntry
	for rows.Next() {
		var id int64
		var name sql.NullString
		var count int64
		if err := rows.Scan(&id, &name, &count); err != nil {
			return nil, err
		}
		key := strconv.FormatInt(id, 10)
		display := strings.TrimSpace(name.String)
		if !name.Valid || display == "" {
			display = fallbackPrefix + " #" + key
		}
		out = append(out, model.CodexGuardCountEntry{Key: key, Name: display, Count: count})
	}
	return out, rows.Err()
}
