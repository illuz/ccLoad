package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"ccLoad/internal/model"
	"ccLoad/internal/util"
)

func scanBillingGroup(scanner interface{ Scan(...any) error }) (*model.BillingGroup, error) {
	g := &model.BillingGroup{}
	var createdAt, updatedAt int64
	var enabled int
	if err := scanner.Scan(&g.ID, &g.Name, &g.Slug, &g.Description, &g.Multiplier, &enabled, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	g.Enabled = enabled != 0
	g.CreatedAt = time.UnixMilli(createdAt)
	g.UpdatedAt = time.UnixMilli(updatedAt)
	return g, nil
}

const billingGroupColumns = `id, name, slug, description, multiplier, enabled, created_at, updated_at`

func (s *SQLStore) CreateBillingGroup(ctx context.Context, group *model.BillingGroup) error {
	if err := group.Validate(); err != nil {
		return err
	}
	now := time.Now()
	if group.CreatedAt.IsZero() {
		group.CreatedAt = now
	}
	if group.UpdatedAt.IsZero() {
		group.UpdatedAt = now
	}
	q := `INSERT INTO billing_groups (name, slug, description, multiplier, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
	args := []any{group.Name, group.Slug, group.Description, group.Multiplier, boolToInt(group.Enabled), group.CreatedAt.UnixMilli(), group.UpdatedAt.UnixMilli()}
	if group.ID > 0 {
		q = `INSERT INTO billing_groups (id, name, slug, description, multiplier, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
		args = append([]any{group.ID}, args...)
	}
	result, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("create billing group: %w", err)
	}
	if group.ID == 0 {
		group.ID, err = result.LastInsertId()
		if err != nil {
			return fmt.Errorf("get billing group id: %w", err)
		}
	}
	return nil
}

func (s *SQLStore) GetBillingGroup(ctx context.Context, id int64) (*model.BillingGroup, error) {
	g, err := scanBillingGroup(s.db.QueryRowContext(ctx, "SELECT "+billingGroupColumns+" FROM billing_groups WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("billing group not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get billing group: %w", err)
	}
	if err := s.fillBillingGroupChannels(ctx, []*model.BillingGroup{g}); err != nil {
		return nil, err
	}
	return g, nil
}

func (s *SQLStore) GetBillingGroupBySlug(ctx context.Context, slug string) (*model.BillingGroup, error) {
	slug = strings.TrimSpace(strings.ToLower(slug))
	g, err := scanBillingGroup(s.db.QueryRowContext(ctx, "SELECT "+billingGroupColumns+" FROM billing_groups WHERE slug = ?", slug))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("billing group not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get billing group by slug: %w", err)
	}
	if err := s.fillBillingGroupChannels(ctx, []*model.BillingGroup{g}); err != nil {
		return nil, err
	}
	return g, nil
}

func (s *SQLStore) ListBillingGroups(ctx context.Context) ([]*model.BillingGroup, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+billingGroupColumns+" FROM billing_groups ORDER BY name ASC, id ASC")
	if err != nil {
		return nil, fmt.Errorf("list billing groups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	groups := make([]*model.BillingGroup, 0)
	for rows.Next() {
		g, scanErr := scanBillingGroup(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.fillBillingGroupChannels(ctx, groups); err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *SQLStore) fillBillingGroupChannels(ctx context.Context, groups []*model.BillingGroup) error {
	if len(groups) == 0 {
		return nil
	}
	ids := make([]any, 0, len(groups))
	placeholders := make([]string, 0, len(groups))
	byID := make(map[int64]*model.BillingGroup, len(groups))
	for _, g := range groups {
		if g == nil {
			continue
		}
		placeholders = append(placeholders, "?")
		ids = append(ids, g.ID)
		byID[g.ID] = g
		g.ChannelIDs = []int64{}
	}
	if len(placeholders) == 0 {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, "SELECT billing_group_id, channel_id FROM billing_group_channels WHERE billing_group_id IN ("+strings.Join(placeholders, ",")+") ORDER BY channel_id", ids...)
	if err != nil {
		return fmt.Errorf("list billing group channels: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var groupID, channelID int64
		if err := rows.Scan(&groupID, &channelID); err != nil {
			return err
		}
		if g := byID[groupID]; g != nil {
			g.ChannelIDs = append(g.ChannelIDs, channelID)
			g.ChannelCount++
		}
	}
	return rows.Err()
}

func (s *SQLStore) GetBillingGroupChannels(ctx context.Context, groupID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT channel_id FROM billing_group_channels WHERE billing_group_id = ? ORDER BY channel_id", groupID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *SQLStore) UpdateBillingGroup(ctx context.Context, group *model.BillingGroup) error {
	if group == nil || group.ID <= 0 {
		return errors.New("billing group id is required")
	}
	if err := group.Validate(); err != nil {
		return err
	}
	group.UpdatedAt = time.Now()
	result, err := s.db.ExecContext(ctx, `UPDATE billing_groups SET name = ?, slug = ?, description = ?, multiplier = ?, enabled = ?, updated_at = ? WHERE id = ?`, group.Name, group.Slug, group.Description, group.Multiplier, boolToInt(group.Enabled), group.UpdatedAt.UnixMilli(), group.ID)
	if err != nil {
		return fmt.Errorf("update billing group: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return errors.New("billing group not found")
	}
	return nil
}

func (s *SQLStore) DeleteBillingGroup(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE auth_tokens SET default_billing_group_id = 0 WHERE default_billing_group_id = ?`, id); err != nil {
		return fmt.Errorf("clear billing group defaults: %w", err)
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM billing_groups WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete billing group: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return errors.New("billing group not found")
	}
	return nil
}

func (s *SQLStore) SetBillingGroupChannels(ctx context.Context, groupID int64, channelIDs []int64) error {
	return s.WithTransaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM billing_group_channels WHERE billing_group_id = ?", groupID); err != nil {
			return err
		}
		for _, channelID := range channelIDs {
			if channelID <= 0 {
				continue
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO billing_group_channels (billing_group_id, channel_id) VALUES (?, ?)", groupID, channelID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SQLStore) AdjustAuthTokenBalance(ctx context.Context, tokenID, deltaMicroUSD int64, txType, note string) (*model.BalanceTransaction, error) {
	if tokenID <= 0 || deltaMicroUSD == 0 {
		return nil, errors.New("token id and non-zero balance delta are required")
	}
	if strings.TrimSpace(txType) == "" {
		if deltaMicroUSD > 0 {
			txType = model.BalanceTransactionManualCredit
		} else {
			txType = model.BalanceTransactionManualDebit
		}
	}
	var entry *model.BalanceTransaction
	err := s.WithTransaction(ctx, func(tx *sql.Tx) error {
		var enabled int
		var before int64
		query := "SELECT balance_enabled, balance_microusd FROM auth_tokens WHERE id = ?"
		if !s.IsSQLite() {
			query += " FOR UPDATE"
		}
		if err := tx.QueryRowContext(ctx, query, tokenID).Scan(&enabled, &before); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("auth token not found")
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE auth_tokens SET balance_microusd = balance_microusd + ? WHERE id = ?", deltaMicroUSD, tokenID); err != nil {
			return err
		}
		after := before + deltaMicroUSD
		now := time.Now()
		result, err := tx.ExecContext(ctx, `INSERT INTO auth_token_balance_transactions (auth_token_id, type, delta_microusd, balance_before_microusd, balance_after_microusd, note, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, tokenID, txType, deltaMicroUSD, before, after, strings.TrimSpace(note), now.UnixMilli())
		if err != nil {
			return err
		}
		id, _ := result.LastInsertId()
		entry = &model.BalanceTransaction{ID: id, AuthTokenID: tokenID, Type: txType, DeltaMicroUSD: deltaMicroUSD, BalanceBeforeMicroUSD: before, BalanceAfterMicroUSD: after, Note: strings.TrimSpace(note), CreatedAt: now}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// ChargeAuthTokenBalance 原子扣减余额并写入消费流水。余额不足时仍允许扣成负数，
// 由请求开始时的余额检查阻止后续新请求，兼容在途请求最终结算。
func (s *SQLStore) ChargeAuthTokenBalance(ctx context.Context, tokenHash string, groupID int64, groupSlug string, multiplier float64, requestID string, standardCostMicroUSD, deltaMicroUSD, totalTokens int64) (*model.BalanceTransaction, error) {
	if tokenHash == "" || deltaMicroUSD <= 0 {
		return nil, nil
	}
	var entry *model.BalanceTransaction
	err := s.WithTransaction(ctx, func(tx *sql.Tx) error {
		var tokenID int64
		var enabled int
		var before int64
		query := "SELECT id, balance_enabled, balance_microusd FROM auth_tokens WHERE token = ?"
		if !s.IsSQLite() {
			query += " FOR UPDATE"
		}
		if err := tx.QueryRowContext(ctx, query, tokenHash).Scan(&tokenID, &enabled, &before); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("token not found")
			}
			return err
		}
		if enabled == 0 {
			return nil
		}
		after := before - deltaMicroUSD
		if _, err := tx.ExecContext(ctx, "UPDATE auth_tokens SET balance_microusd = ? WHERE id = ?", after, tokenID); err != nil {
			return err
		}
		now := time.Now()
		result, err := tx.ExecContext(ctx, `INSERT INTO auth_token_balance_transactions (auth_token_id, billing_group_id, billing_group_slug, type, delta_microusd, balance_before_microusd, balance_after_microusd, standard_cost_microusd, total_tokens, multiplier, request_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, tokenID, groupID, strings.TrimSpace(groupSlug), model.BalanceTransactionConsumption, -deltaMicroUSD, before, after, standardCostMicroUSD, totalTokens, multiplier, strings.TrimSpace(requestID), now.UnixMilli())
		if err != nil {
			return err
		}
		id, _ := result.LastInsertId()
		entry = &model.BalanceTransaction{ID: id, AuthTokenID: tokenID, BillingGroupID: groupID, BillingGroupSlug: groupSlug, Type: model.BalanceTransactionConsumption, DeltaMicroUSD: -deltaMicroUSD, BalanceBeforeMicroUSD: before, BalanceAfterMicroUSD: after, StandardCostMicroUSD: standardCostMicroUSD, TotalTokens: totalTokens, Multiplier: multiplier, RequestID: requestID, CreatedAt: now}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entry, nil
}

func (s *SQLStore) ListAuthTokenBalanceTransactions(ctx context.Context, tokenID int64, limit, offset int) ([]*model.BalanceTransaction, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, auth_token_id, billing_group_id, billing_group_slug, type, delta_microusd, balance_before_microusd, balance_after_microusd, standard_cost_microusd, total_tokens, multiplier, request_id, note, created_at FROM auth_token_balance_transactions WHERE auth_token_id = ? ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, tokenID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]*model.BalanceTransaction, 0)
	for rows.Next() {
		entry := &model.BalanceTransaction{}
		var createdAt int64
		if err := rows.Scan(&entry.ID, &entry.AuthTokenID, &entry.BillingGroupID, &entry.BillingGroupSlug, &entry.Type, &entry.DeltaMicroUSD, &entry.BalanceBeforeMicroUSD, &entry.BalanceAfterMicroUSD, &entry.StandardCostMicroUSD, &entry.TotalTokens, &entry.Multiplier, &entry.RequestID, &entry.Note, &createdAt); err != nil {
			return nil, err
		}
		entry.CreatedAt = time.UnixMilli(createdAt)
		result = append(result, entry)
	}
	return result, rows.Err()
}

func (s *SQLStore) GetAuthTokenBillingGroupUsage(ctx context.Context, tokenID int64, since, until time.Time) ([]model.BillingGroupUsage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT billing_group_id, COUNT(DISTINCT request_id), COALESCE(SUM(total_tokens), 0), COALESCE(SUM(-delta_microusd), 0) FROM auth_token_balance_transactions WHERE auth_token_id = ? AND type = ? AND created_at >= ? AND created_at <= ? GROUP BY billing_group_id ORDER BY 4 DESC`, tokenID, model.BalanceTransactionConsumption, since.UnixMilli(), until.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]model.BillingGroupUsage, 0)
	for rows.Next() {
		var item model.BillingGroupUsage
		var chargedMicro int64
		if err := rows.Scan(&item.BillingGroupID, &item.RequestCount, &item.TotalTokens, &chargedMicro); err != nil {
			return nil, err
		}
		item.ChargedUSD = util.MicroUSDToUSD(chargedMicro)
		result = append(result, item)
	}
	return result, rows.Err()
}
