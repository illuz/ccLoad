package model

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"ccLoad/internal/util"
)

// BillingGroup 是面向客户的计费与路由分组。
// 每个请求只能使用一个计费分组；分组倍率只影响客户扣费，不改变渠道内部成本统计。
type BillingGroup struct {
	ID                    int64     `json:"id"`
	Name                  string    `json:"name"`
	Slug                  string    `json:"slug"`
	Description           string    `json:"description"`
	Multiplier            float64   `json:"multiplier"`
	Enabled               bool      `json:"enabled"`
	ChannelIDs            []int64   `json:"channel_ids,omitempty"`
	ChannelCount          int       `json:"channel_count"`
	AvailableChannelCount int       `json:"available_channel_count"`
	SuccessRate           float64   `json:"success_rate"`
	HealthSampleCount     int64     `json:"health_sample_count"`
	RequestCount          int64     `json:"request_count"`
	TotalTokens           int64     `json:"total_tokens"`
	ChargedUSD            float64   `json:"charged_usd"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// BillingGroupUsage 汇总令牌在某个计费分组下的消费情况。
type BillingGroupUsage struct {
	BillingGroupID int64   `json:"billing_group_id"`
	RequestCount   int64   `json:"request_count"`
	TotalTokens    int64   `json:"total_tokens"`
	ChargedUSD     float64 `json:"charged_usd"`
}

func (g *BillingGroup) Validate() error {
	if g == nil {
		return errors.New("billing group cannot be nil")
	}
	g.Name = strings.TrimSpace(g.Name)
	g.Slug = strings.TrimSpace(strings.ToLower(g.Slug))
	if g.Name == "" {
		return errors.New("name is required")
	}
	if g.Slug == "" {
		return errors.New("slug is required")
	}
	for _, r := range g.Slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return errors.New("slug contains unsupported characters")
	}
	if g.Multiplier < 0 {
		return errors.New("multiplier must be >= 0")
	}
	if g.Multiplier > 1000 {
		return errors.New("multiplier must be <= 1000")
	}
	return nil
}

// BalanceTransaction 记录令牌余额的每一次变化。
// DeltaMicroUSD 为正表示增加余额，为负表示扣减余额。
type BalanceTransaction struct {
	ID                    int64     `json:"id"`
	AuthTokenID           int64     `json:"auth_token_id"`
	BillingGroupID        int64     `json:"billing_group_id,omitempty"`
	BillingGroupSlug      string    `json:"billing_group_slug,omitempty"`
	Type                  string    `json:"type"`
	DeltaMicroUSD         int64     `json:"-"`
	BalanceBeforeMicroUSD int64     `json:"-"`
	BalanceAfterMicroUSD  int64     `json:"-"`
	StandardCostMicroUSD  int64     `json:"-"`
	TotalTokens           int64     `json:"total_tokens,omitempty"`
	Multiplier            float64   `json:"multiplier,omitempty"`
	RequestID             string    `json:"request_id,omitempty"`
	Note                  string    `json:"note,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
}

const (
	BalanceTransactionManualCredit = "manual_credit"
	BalanceTransactionManualDebit  = "manual_debit"
	BalanceTransactionConsumption  = "consumption"
	BalanceTransactionRefund       = "refund"
	BalanceTransactionManualAdjust = "manual_adjust"
)

func (t BalanceTransaction) MarshalJSON() ([]byte, error) {
	type view struct {
		ID               int64     `json:"id"`
		AuthTokenID      int64     `json:"auth_token_id"`
		BillingGroupID   int64     `json:"billing_group_id,omitempty"`
		BillingGroupSlug string    `json:"billing_group_slug,omitempty"`
		Type             string    `json:"type"`
		DeltaUSD         float64   `json:"delta_usd"`
		BalanceBeforeUSD float64   `json:"balance_before_usd"`
		BalanceAfterUSD  float64   `json:"balance_after_usd"`
		StandardCostUSD  float64   `json:"standard_cost_usd,omitempty"`
		TotalTokens      int64     `json:"total_tokens,omitempty"`
		Multiplier       float64   `json:"multiplier,omitempty"`
		RequestID        string    `json:"request_id,omitempty"`
		Note             string    `json:"note,omitempty"`
		CreatedAt        time.Time `json:"created_at"`
	}
	return json.Marshal(view{
		ID: t.ID, AuthTokenID: t.AuthTokenID, BillingGroupID: t.BillingGroupID,
		BillingGroupSlug: t.BillingGroupSlug, Type: t.Type,
		DeltaUSD:         util.MicroUSDToUSD(t.DeltaMicroUSD),
		BalanceBeforeUSD: util.MicroUSDToUSD(t.BalanceBeforeMicroUSD),
		BalanceAfterUSD:  util.MicroUSDToUSD(t.BalanceAfterMicroUSD),
		StandardCostUSD:  util.MicroUSDToUSD(t.StandardCostMicroUSD), Multiplier: t.Multiplier,
		TotalTokens: t.TotalTokens,
		RequestID:   t.RequestID, Note: t.Note, CreatedAt: t.CreatedAt,
	})
}
