package sql

import (
	"context"
	"database/sql"
	"time"

	"ccLoad/internal/model"
)

// AddDebugLog 插入一条调试日志
func (s *SQLStore) AddDebugLog(ctx context.Context, e *model.DebugLogEntry) error {
	if e.CreatedAt == 0 {
		e.CreatedAt = time.Now().Unix()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO debug_logs (log_id, created_at, req_method, req_url, req_headers, req_body, resp_status, resp_headers, resp_body)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.LogID, e.CreatedAt, e.ReqMethod, e.ReqURL, e.ReqHeaders, e.ReqBody, e.RespStatus, e.RespHeaders, e.RespBody,
	)
	return err
}

// GetDebugLogByLogID 根据 log_id 查询调试日志
func (s *SQLStore) GetDebugLogByLogID(ctx context.Context, logID int64) (*model.DebugLogEntry, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT log_id, created_at, req_method, req_url, req_headers, req_body, resp_status, resp_headers, resp_body
		FROM debug_logs WHERE log_id = ? LIMIT 1`, logID)

	var e model.DebugLogEntry
	err := row.Scan(&e.LogID, &e.CreatedAt, &e.ReqMethod, &e.ReqURL, &e.ReqHeaders, &e.ReqBody, &e.RespStatus, &e.RespHeaders, &e.RespBody)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// CleanupDebugLogsBefore 清理过期的调试日志
func (s *SQLStore) CleanupDebugLogsBefore(ctx context.Context, cutoff time.Time) error {
	// debug_logs 里保存完整请求/响应体，单次大 DELETE 可能因为几十 GB 的
	// BLOB 数据而长时间持有写锁，甚至超过调用方的 context 超时。这里分批
	// 删除，让每批独立提交，降低锁占用并保证后续清理能持续推进。
	const (
		batchSize         int64 = 5
		maxBatchesPerCall int64 = 1
	)
	var (
		deleted int64
		batches int64
	)

	for {
		if err := ctx.Err(); err != nil {
			if deleted > 0 {
				return nil
			}
			return err
		}

		var (
			result sql.Result
			err    error
		)
		if s.IsSQLite() {
			result, err = s.db.ExecContext(ctx, `
				DELETE FROM debug_logs
				WHERE log_id IN (
					SELECT log_id FROM debug_logs
					WHERE created_at < ?
					ORDER BY created_at
					LIMIT ?
				)`, cutoff.Unix(), batchSize)
		} else {
			result, err = s.db.ExecContext(ctx, `
				DELETE FROM debug_logs
				WHERE created_at < ?
				ORDER BY created_at
				LIMIT ?`, cutoff.Unix(), batchSize)
		}
		if err != nil {
			s.runSQLiteIncrementalVacuum(ctx, deleted)
			if ctx.Err() != nil && deleted > 0 {
				return nil
			}
			return err
		}

		affected, _ := result.RowsAffected()
		deleted += affected
		batches++
		if affected < batchSize {
			break
		}
		if batches >= maxBatchesPerCall {
			break
		}
	}

	s.runSQLiteIncrementalVacuum(ctx, deleted)
	return nil
}

// TruncateDebugLogs 清空所有调试日志
func (s *SQLStore) TruncateDebugLogs(ctx context.Context) error {
	const batchSize int64 = 25
	var deleted int64

	for {
		if err := ctx.Err(); err != nil {
			if deleted > 0 {
				return nil
			}
			return err
		}

		var (
			result sql.Result
			err    error
		)
		if s.IsSQLite() {
			result, err = s.db.ExecContext(ctx, `
				DELETE FROM debug_logs
				WHERE log_id IN (
					SELECT log_id FROM debug_logs
					ORDER BY created_at
					LIMIT ?
				)`, batchSize)
		} else {
			result, err = s.db.ExecContext(ctx, `DELETE FROM debug_logs LIMIT ?`, batchSize)
		}
		if err != nil {
			s.runSQLiteIncrementalVacuum(ctx, deleted)
			if ctx.Err() != nil && deleted > 0 {
				return nil
			}
			return err
		}

		affected, _ := result.RowsAffected()
		deleted += affected
		if affected < batchSize {
			break
		}
	}

	s.runSQLiteIncrementalVacuum(ctx, deleted)
	return nil
}
