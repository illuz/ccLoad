package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"ccLoad/internal/model"
)

const channelGroupSelectColumns = `
	id, name, description, color, created_at, updated_at
`

func scanChannelGroup(scanner interface {
	Scan(...any) error
}) (*model.ChannelGroup, error) {
	group := &model.ChannelGroup{}
	var createdAtMs int64
	var updatedAtMs int64
	if err := scanner.Scan(
		&group.ID,
		&group.Name,
		&group.Description,
		&group.Color,
		&createdAtMs,
		&updatedAtMs,
	); err != nil {
		return nil, err
	}
	group.Color = model.CanonicalAuthTokenGroupColor(group.Color)
	group.CreatedAt = time.UnixMilli(createdAtMs)
	group.UpdatedAt = time.UnixMilli(updatedAtMs)
	return group, nil
}

func prepareChannelGroupForWrite(group *model.ChannelGroup) error {
	if group == nil {
		return errors.New("group cannot be nil")
	}
	group.Name = strings.TrimSpace(group.Name)
	group.Description = strings.TrimSpace(group.Description)
	if group.Name == "" {
		return errors.New("name is required")
	}
	group.Color = model.CanonicalAuthTokenGroupColor(group.Color)
	return nil
}

// CreateChannelGroup 创建渠道分组。
func (s *SQLStore) CreateChannelGroup(ctx context.Context, group *model.ChannelGroup) error {
	if err := prepareChannelGroupForWrite(group); err != nil {
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
		INSERT INTO channel_groups (name, description, color, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`
	args := []any{group.Name, group.Description, group.Color, group.CreatedAt.UnixMilli(), group.UpdatedAt.UnixMilli()}
	if group.ID > 0 {
		query = `
			INSERT INTO channel_groups (id, name, description, color, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`
		args = append([]any{group.ID}, args...)
	}

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("create channel group: %w", err)
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

// GetChannelGroup 根据ID获取渠道分组。
func (s *SQLStore) GetChannelGroup(ctx context.Context, id int64) (*model.ChannelGroup, error) {
	group, err := scanChannelGroup(s.db.QueryRowContext(
		ctx,
		fmt.Sprintf("SELECT %s FROM channel_groups WHERE id = ?", channelGroupSelectColumns),
		id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("channel group not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get channel group: %w", err)
	}
	if err := s.fillChannelGroupCounts(ctx, []*model.ChannelGroup{group}); err != nil {
		return nil, err
	}
	return group, nil
}

// ListChannelGroups 列出全部渠道分组。
func (s *SQLStore) ListChannelGroups(ctx context.Context) ([]*model.ChannelGroup, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(
		"SELECT %s FROM channel_groups ORDER BY name ASC, id ASC",
		channelGroupSelectColumns,
	))
	if err != nil {
		return nil, fmt.Errorf("list channel groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	groups := make([]*model.ChannelGroup, 0)
	for rows.Next() {
		group, err := scanChannelGroup(rows)
		if err != nil {
			return nil, fmt.Errorf("scan channel group: %w", err)
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.fillChannelGroupCounts(ctx, groups); err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *SQLStore) fillChannelGroupCounts(ctx context.Context, groups []*model.ChannelGroup) error {
	if len(groups) == 0 {
		return nil
	}
	countByID := make(map[int64]int, len(groups))
	rows, err := s.db.QueryContext(ctx, `
		SELECT group_id, COUNT(*)
		FROM channels
		WHERE group_id > 0
		GROUP BY group_id
	`)
	if err != nil {
		return fmt.Errorf("count channel groups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var groupID int64
		var count int
		if err := rows.Scan(&groupID, &count); err != nil {
			return fmt.Errorf("scan channel group count: %w", err)
		}
		countByID[groupID] = count
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, group := range groups {
		if group != nil {
			group.ChannelCount = countByID[group.ID]
		}
	}
	return nil
}

// UpdateChannelGroup 更新渠道分组。
func (s *SQLStore) UpdateChannelGroup(ctx context.Context, group *model.ChannelGroup) error {
	if group == nil || group.ID <= 0 {
		return errors.New("group id is required")
	}
	if err := prepareChannelGroupForWrite(group); err != nil {
		return err
	}
	group.UpdatedAt = time.Now()
	result, err := s.db.ExecContext(ctx, `
		UPDATE channel_groups
		SET name = ?, description = ?, color = ?, updated_at = ?
		WHERE id = ?
	`, group.Name, group.Description, group.Color, group.UpdatedAt.UnixMilli(), group.ID)
	if err != nil {
		return fmt.Errorf("update channel group: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("channel group not found")
	}
	return nil
}

// DeleteChannelGroup 删除空渠道分组；非空分组拒绝删除。
func (s *SQLStore) DeleteChannelGroup(ctx context.Context, id int64) error {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM channels WHERE group_id = ?", id).Scan(&count); err != nil {
		return fmt.Errorf("count channels by group: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("channel group is not empty")
	}

	result, err := s.db.ExecContext(ctx, "DELETE FROM channel_groups WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete channel group: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("channel group not found")
	}
	return nil
}

// BatchUpdateChannelGroup 批量移动渠道到指定分组，groupID=0 表示未分组。
func (s *SQLStore) BatchUpdateChannelGroup(ctx context.Context, ids []int64, groupID int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if groupID < 0 {
		return 0, errors.New("group_id must be >= 0")
	}
	if groupID > 0 {
		if _, err := s.GetChannelGroup(ctx, groupID); err != nil {
			return 0, err
		}
	}

	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids)+2)
	args = append(args, groupID, time.Now().UnixMilli())
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	if len(placeholders) == 0 {
		return 0, nil
	}

	query := fmt.Sprintf("UPDATE channels SET group_id = ?, updated_at = ? WHERE id IN (%s)", strings.Join(placeholders, ","))
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("batch update channel group: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}
