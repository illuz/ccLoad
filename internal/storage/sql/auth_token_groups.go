package sql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ccLoad/internal/model"
)

const authTokenGroupSelectColumns = `
	id, name, description, color, created_at, updated_at, cost_limit_microusd, allowed_models, allowed_channel_ids, max_concurrency
`

func scanAuthTokenGroup(scanner interface {
	Scan(...any) error
}) (*model.AuthTokenGroup, error) {
	group := &model.AuthTokenGroup{}
	var createdAtMs int64
	var updatedAtMs int64
	var allowedModelsJSON string
	var allowedChannelIDsJSON string

	if err := scanner.Scan(
		&group.ID,
		&group.Name,
		&group.Description,
		&group.Color,
		&createdAtMs,
		&updatedAtMs,
		&group.CostLimitMicroUSD,
		&allowedModelsJSON,
		&allowedChannelIDsJSON,
		&group.MaxConcurrency,
	); err != nil {
		return nil, err
	}

	group.CreatedAt = time.UnixMilli(createdAtMs)
	group.UpdatedAt = time.UnixMilli(updatedAtMs)
	if allowedModelsJSON != "" {
		if err := jsonUnmarshalStringSlice(allowedModelsJSON, &group.AllowedModels); err != nil {
			return nil, fmt.Errorf("invalid group allowed_models json: %w", err)
		}
	}
	if allowedChannelIDsJSON != "" {
		if err := jsonUnmarshalInt64Slice(allowedChannelIDsJSON, &group.AllowedChannelIDs); err != nil {
			return nil, fmt.Errorf("invalid group allowed_channel_ids json: %w", err)
		}
	}
	if err := group.ValidateUsageLimits(); err != nil {
		return nil, err
	}
	return group, nil
}

func jsonUnmarshalStringSlice(raw string, out *[]string) error {
	return json.Unmarshal([]byte(raw), out)
}

func jsonUnmarshalInt64Slice(raw string, out *[]int64) error {
	return json.Unmarshal([]byte(raw), out)
}

func prepareAuthTokenGroupForWrite(group *model.AuthTokenGroup) (allowedModelsJSON, allowedChannelIDsJSON string, err error) {
	if group == nil {
		return "", "", errors.New("group cannot be nil")
	}
	group.Name = strings.TrimSpace(group.Name)
	if err := group.ValidateUsageLimits(); err != nil {
		return "", "", err
	}
	allowedModelsJSON, err = marshalAllowedModels(group.AllowedModels)
	if err != nil {
		return "", "", err
	}
	allowedChannelIDsJSON, err = marshalAllowedChannelIDs(group.AllowedChannelIDs)
	if err != nil {
		return "", "", err
	}
	return allowedModelsJSON, allowedChannelIDsJSON, nil
}

// CreateAuthTokenGroup 创建令牌分组。
func (s *SQLStore) CreateAuthTokenGroup(ctx context.Context, group *model.AuthTokenGroup) error {
	allowedModelsJSON, allowedChannelIDsJSON, err := prepareAuthTokenGroupForWrite(group)
	if err != nil {
		return err
	}
	now := time.Now()
	if group.CreatedAt.IsZero() {
		group.CreatedAt = now
	}
	if group.UpdatedAt.IsZero() {
		group.UpdatedAt = now
	}

	query := `
		INSERT INTO auth_token_groups (
			name, description, color, created_at, updated_at, cost_limit_microusd, allowed_models, allowed_channel_ids, max_concurrency
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	args := []any{group.Name, group.Description, group.Color, group.CreatedAt.UnixMilli(), group.UpdatedAt.UnixMilli(), group.CostLimitMicroUSD, allowedModelsJSON, allowedChannelIDsJSON, group.MaxConcurrency}
	if group.ID > 0 {
		query = `
			INSERT INTO auth_token_groups (
				id, name, description, color, created_at, updated_at, cost_limit_microusd, allowed_models, allowed_channel_ids, max_concurrency
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		args = append([]any{group.ID}, args...)
	}

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("create auth token group: %w", err)
	}
	if group.ID == 0 {
		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("get last insert id: %w", err)
		}
		group.ID = id
	}
	return nil
}

// GetAuthTokenGroup 根据ID获取令牌分组。
func (s *SQLStore) GetAuthTokenGroup(ctx context.Context, id int64) (*model.AuthTokenGroup, error) {
	group, err := scanAuthTokenGroup(s.db.QueryRowContext(
		ctx,
		fmt.Sprintf("SELECT %s FROM auth_token_groups WHERE id = ?", authTokenGroupSelectColumns),
		id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("auth token group not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get auth token group: %w", err)
	}
	if err := s.fillAuthTokenGroupTokenCounts(ctx, []*model.AuthTokenGroup{group}); err != nil {
		return nil, err
	}
	return group, nil
}

// ListAuthTokenGroups 列出全部令牌分组。
func (s *SQLStore) ListAuthTokenGroups(ctx context.Context) ([]*model.AuthTokenGroup, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(
		"SELECT %s FROM auth_token_groups ORDER BY name ASC, id ASC",
		authTokenGroupSelectColumns,
	))
	if err != nil {
		return nil, fmt.Errorf("list auth token groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	groups := make([]*model.AuthTokenGroup, 0)
	for rows.Next() {
		group, err := scanAuthTokenGroup(rows)
		if err != nil {
			return nil, fmt.Errorf("scan auth token group: %w", err)
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.fillAuthTokenGroupTokenCounts(ctx, groups); err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *SQLStore) fillAuthTokenGroupTokenCounts(ctx context.Context, groups []*model.AuthTokenGroup) error {
	if len(groups) == 0 {
		return nil
	}
	countByID := make(map[int64]int, len(groups))
	rows, err := s.db.QueryContext(ctx, `
		SELECT group_id, COUNT(*)
		FROM auth_tokens
		WHERE group_id > 0
		GROUP BY group_id
	`)
	if err != nil {
		return fmt.Errorf("count auth token groups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var groupID int64
		var count int
		if err := rows.Scan(&groupID, &count); err != nil {
			return fmt.Errorf("scan auth token group count: %w", err)
		}
		countByID[groupID] = count
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, group := range groups {
		if group != nil {
			group.TokenCount = countByID[group.ID]
		}
	}
	return nil
}

// UpdateAuthTokenGroup 更新令牌分组。
func (s *SQLStore) UpdateAuthTokenGroup(ctx context.Context, group *model.AuthTokenGroup) error {
	if group == nil || group.ID <= 0 {
		return errors.New("group id is required")
	}
	allowedModelsJSON, allowedChannelIDsJSON, err := prepareAuthTokenGroupForWrite(group)
	if err != nil {
		return err
	}
	group.UpdatedAt = time.Now()
	result, err := s.db.ExecContext(ctx, `
		UPDATE auth_token_groups
		SET name = ?,
		    description = ?,
		    color = ?,
		    updated_at = ?,
		    cost_limit_microusd = ?,
		    allowed_models = ?,
		    allowed_channel_ids = ?,
		    max_concurrency = ?
		WHERE id = ?
	`, group.Name, group.Description, group.Color, group.UpdatedAt.UnixMilli(), group.CostLimitMicroUSD, allowedModelsJSON, allowedChannelIDsJSON, group.MaxConcurrency, group.ID)
	if err != nil {
		return fmt.Errorf("update auth token group: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("auth token group not found")
	}
	return nil
}

// DeleteAuthTokenGroup 删除空令牌分组；非空分组拒绝删除。
func (s *SQLStore) DeleteAuthTokenGroup(ctx context.Context, id int64) error {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM auth_tokens WHERE group_id = ?", id).Scan(&count); err != nil {
		return fmt.Errorf("count auth tokens by group: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("auth token group is not empty")
	}

	result, err := s.db.ExecContext(ctx, "DELETE FROM auth_token_groups WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete auth token group: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("auth token group not found")
	}
	return nil
}
