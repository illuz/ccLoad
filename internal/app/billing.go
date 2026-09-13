package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ccLoad/internal/model"

	"github.com/gin-gonic/gin"
)

var errBillingGroupRequired = errors.New("no billing group configured for this token")

type customerBillingGroup struct {
	ID                    int64   `json:"id"`
	Name                  string  `json:"name"`
	Slug                  string  `json:"slug"`
	Description           string  `json:"description"`
	Multiplier            float64 `json:"multiplier"`
	Enabled               bool    `json:"enabled"`
	ChannelCount          int     `json:"channel_count"`
	AvailableChannelCount int     `json:"available_channel_count"`
	SuccessRate           float64 `json:"success_rate"`
	HealthSampleCount     int64   `json:"health_sample_count"`
	RequestCount          int64   `json:"request_count"`
	TotalTokens           int64   `json:"total_tokens"`
	ChargedUSD            float64 `json:"charged_usd"`
}

func customerBillingGroups(groups []*model.BillingGroup) []customerBillingGroup {
	result := make([]customerBillingGroup, 0, len(groups))
	for _, group := range groups {
		if group == nil {
			continue
		}
		result = append(result, customerBillingGroup{
			ID: group.ID, Name: group.Name, Slug: group.Slug, Description: group.Description,
			Multiplier: group.Multiplier, Enabled: group.Enabled, ChannelCount: group.ChannelCount,
			AvailableChannelCount: group.AvailableChannelCount, SuccessRate: group.SuccessRate,
			HealthSampleCount: group.HealthSampleCount, RequestCount: group.RequestCount,
			TotalTokens: group.TotalTokens, ChargedUSD: group.ChargedUSD,
		})
	}
	return result
}

// resolveRequestBillingGroup resolves the suffix/default group for a request.
// A group is optional for legacy tokens, but mandatory once the new wallet
// logic is enabled. The resolved values are stored on the Gin context for
// routing, logging, and asynchronous settlement.
func (s *Server) resolveRequestBillingGroup(ctx context.Context, c *gin.Context, tokenHash string) (*model.BillingGroup, error) {
	if tokenHash == "" || s.store == nil {
		return nil, nil
	}
	token, err := s.store.GetAuthTokenByValue(ctx, tokenHash)
	if err != nil {
		// Some embedding/integration tests (and legacy in-memory callers) set
		// the authenticated hash without persisting a full token row. Preserve
		// the historical unrestricted routing in that case.
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, nil
		}
		return nil, err
	}
	slug := c.GetString("billing_group_slug")
	var group *model.BillingGroup
	if strings.TrimSpace(slug) != "" {
		group, err = s.store.GetBillingGroupBySlug(ctx, slug)
	} else if token.DefaultBillingGroupID > 0 {
		group, err = s.store.GetBillingGroup(ctx, token.DefaultBillingGroupID)
	}
	if err != nil {
		return nil, err
	}
	if group == nil {
		if token.BalanceEnabled {
			return nil, errBillingGroupRequired
		}
		return nil, nil
	}
	if !group.Enabled {
		return nil, fmt.Errorf("billing group %q is disabled", group.Slug)
	}
	group.Multiplier = normalizeBillingMultiplier(group.Multiplier)
	c.Set("billing_group_id", group.ID)
	c.Set("billing_group_slug", group.Slug)
	c.Set("billing_group_multiplier", group.Multiplier)
	c.Set("balance_enabled", token.BalanceEnabled)
	return group, nil
}

func normalizeBillingMultiplier(value float64) float64 {
	if value < 0 {
		return 1
	}
	return value
}

func billingGroupIDFromContext(c *gin.Context) int64 {
	if c == nil {
		return 0
	}
	if value, ok := c.Get("billing_group_id"); ok {
		if id, ok := value.(int64); ok {
			return id
		}
	}
	return 0
}

func billingMultiplierFromContext(c *gin.Context) float64 {
	if c == nil {
		return 0
	}
	if value, ok := c.Get("billing_group_multiplier"); ok {
		if multiplier, ok := value.(float64); ok {
			return multiplier
		}
	}
	return 0
}

func balanceEnabledFromContext(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if value, ok := c.Get("balance_enabled"); ok {
		enabled, _ := value.(bool)
		return enabled
	}
	return false
}

func filterBillingGroupChannels(cands []*model.Config, group *model.BillingGroup) []*model.Config {
	if group == nil {
		return cands
	}
	allowed := make(map[int64]struct{}, len(group.ChannelIDs))
	for _, id := range group.ChannelIDs {
		allowed[id] = struct{}{}
	}
	filtered := make([]*model.Config, 0, len(cands))
	for _, cfg := range cands {
		if cfg == nil {
			continue
		}
		if _, ok := allowed[cfg.ID]; ok {
			filtered = append(filtered, cfg)
		}
	}
	return filtered
}

func billingGroupAvailability(s *Server, ctx context.Context, group *model.BillingGroup) {
	if group == nil {
		return
	}
	group.ChannelCount = len(group.ChannelIDs)
	available := 0
	var successWeighted float64
	var samples int64
	now := time.Now()
	for _, id := range group.ChannelIDs {
		cfg, err := s.store.GetConfig(ctx, id)
		if err != nil || cfg == nil {
			continue
		}
		if cfg.Enabled && !cfg.IsCoolingDown(now) {
			keys, keyErr := s.store.GetAPIKeys(ctx, id)
			if keyErr == nil {
				for _, key := range keys {
					if key != nil && !key.Disabled && !key.IsCoolingDown(now) {
						available++
						break
					}
				}
			}
		}
		if s.healthCache != nil {
			stats := s.healthCache.GetHealthStats(id)
			successWeighted += stats.SuccessRate * float64(stats.SampleCount)
			samples += stats.SampleCount
		}
	}
	group.AvailableChannelCount = available
	group.HealthSampleCount = samples
	if samples > 0 {
		group.SuccessRate = successWeighted / float64(samples)
	} else {
		group.SuccessRate = 1
	}
}

func applyBillingGroupUsage(groups []*model.BillingGroup, usage []model.BillingGroupUsage) {
	byID := make(map[int64]model.BillingGroupUsage, len(usage))
	for _, item := range usage {
		byID[item.BillingGroupID] = item
	}
	for _, group := range groups {
		if group == nil {
			continue
		}
		if item, ok := byID[group.ID]; ok {
			group.RequestCount = item.RequestCount
			group.TotalTokens = item.TotalTokens
			group.ChargedUSD = item.ChargedUSD
		}
	}
}

func (s *Server) rejectBillingGroup(c *gin.Context, err error, startTime time.Time, modelName string, streaming bool, thinkingEffort string) {
	status := http.StatusForbidden
	if errors.Is(err, errBillingGroupRequired) {
		status = http.StatusServiceUnavailable
	}
	message := err.Error()
	c.JSON(status, gin.H{"error": gin.H{"message": message, "type": "billing_group_error", "code": "billing_group_unavailable"}})
	s.recordProxyRejection(c, startTime, modelName, status, message, streaming, thinkingEffort)
}
