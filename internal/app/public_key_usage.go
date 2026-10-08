package app

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"ccLoad/internal/model"
	"ccLoad/internal/util"

	"github.com/gin-gonic/gin"
)

const publicKeyUsageBucket = 30 * time.Minute

type PublicKeyTodayUsage struct {
	RequestCount  int     `json:"request_count"`
	TotalTokens   int64   `json:"total_tokens"`
	EffectiveCost float64 `json:"effective_cost"`
}

type PublicKeyCostQuota struct {
	UsedUSD         float64  `json:"used_usd"`
	LimitUSD        *float64 `json:"limit_usd,omitempty"`
	UsagePercentage *float64 `json:"usage_percentage,omitempty"`
}

type PublicKeyUsageTrendPoint struct {
	Timestamp     int64   `json:"timestamp"`
	EffectiveCost float64 `json:"effective_cost"`
	TotalTokens   int64   `json:"total_tokens"`
}

type PublicKeyModelUsage struct {
	Model         string  `json:"model"`
	TotalTokens   int64   `json:"total_tokens"`
	EffectiveCost float64 `json:"effective_cost"`
}

// PublicKeyUsageResponse intentionally omits token metadata and channel data.
type PublicKeyUsageResponse struct {
	TodayStart             int64                       `json:"today_start"`
	Today                  PublicKeyTodayUsage         `json:"today"`
	CostQuota              PublicKeyCostQuota          `json:"cost_quota"`
	ModelUsage             []PublicKeyModelUsage       `json:"model_usage"`
	Trend                  []PublicKeyUsageTrendPoint  `json:"trend"`
	UpdatedAt              int64                       `json:"updated_at"`
	BalanceEnabled         bool                        `json:"balance_enabled"`
	BalanceUSD             float64                     `json:"balance_usd"`
	DefaultBillingGroupID  int64                       `json:"default_billing_group_id,omitempty"`
	SelectedBillingGroupID int64                       `json:"selected_billing_group_id,omitempty"`
	BillingGroups          []customerBillingGroup      `json:"billing_groups,omitempty"`
	BillingGroupUsage      []model.BillingGroupUsage   `json:"billing_group_usage,omitempty"`
	BalanceTransactions    []*model.BalanceTransaction `json:"balance_transactions,omitempty"`
}

// HandlePublicKeyUsage returns today's live totals, model breakdown, and half-hour trend.
// GET /public/key-usage?key=...
func (s *Server) HandlePublicKeyUsage(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	token, ok := s.findPublicUsageToken(ctx, c.Query("key"))
	if !ok {
		respondPublicUsageNotFound(c)
		return
	}

	now := time.Now()
	todayStart := beginningOfDay(now)
	selectedGroupID, err := parsePublicBillingGroupID(c.Query("billing_group_id"))
	if err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}

	groups, groupsErr := s.store.ListBillingGroups(ctx)
	if selectedGroupID > 0 {
		if !token.BalanceEnabled {
			RespondErrorMsg(c, http.StatusBadRequest, "billing group filter requires balance mode")
			return
		}
		if groupsErr != nil {
			RespondError(c, http.StatusInternalServerError, groupsErr)
			return
		}
		if !containsBillingGroup(groups, selectedGroupID) {
			RespondErrorMsg(c, http.StatusBadRequest, "billing group is unavailable")
			return
		}
	}
	if groupsErr != nil {
		groups = nil
	}

	var today PublicKeyTodayUsage
	var trend []PublicKeyUsageTrendPoint
	var modelUsage []PublicKeyModelUsage
	if token.BalanceEnabled {
		usageRows, usageErr := s.store.AggregateAuthTokenBalanceUsage(ctx, token.ID, selectedGroupID, todayStart, now, publicKeyUsageBucket)
		if usageErr != nil {
			RespondError(c, http.StatusInternalServerError, usageErr)
			return
		}
		today, modelUsage, trend = buildPublicKeyBalanceUsage(usageRows, todayStart, now, publicKeyUsageBucket)
	} else {
		authTokenID := token.ID
		filter := &model.LogFilter{
			AuthTokenID: &authTokenID,
			LogSource:   model.LogSourceAll,
		}
		points, pointsErr := s.store.AggregateRangeWithFilter(ctx, todayStart, now, publicKeyUsageBucket, filter)
		if pointsErr != nil {
			RespondError(c, http.StatusInternalServerError, pointsErr)
			return
		}
		modelRows, modelErr := s.store.GetModelUsage(ctx, todayStart, now, filter)
		if modelErr != nil {
			RespondError(c, http.StatusInternalServerError, modelErr)
			return
		}
		today, trend = buildPublicKeyTodayUsage(points)
		modelUsage = buildPublicKeyModelUsage(modelRows)
	}

	costQuota := s.buildPublicKeyCostQuota(ctx, token)
	for _, group := range groups {
		billingGroupAvailability(s, ctx, group)
	}
	var groupUsage []model.BillingGroupUsage
	var transactions []*model.BalanceTransaction
	if token.BalanceEnabled {
		groupUsage, _ = s.store.GetAuthTokenBillingGroupUsage(ctx, token.ID, time.UnixMilli(0), now)
		transactions, _ = s.store.ListAuthTokenBalanceAdjustments(ctx, token.ID, 500, 0)
	}
	applyBillingGroupUsage(groups, groupUsage)

	RespondJSON(c, http.StatusOK, PublicKeyUsageResponse{
		TodayStart:             todayStart.UnixMilli(),
		Today:                  today,
		CostQuota:              costQuota,
		ModelUsage:             modelUsage,
		Trend:                  trend,
		UpdatedAt:              now.UnixMilli(),
		BalanceEnabled:         token.BalanceEnabled,
		BalanceUSD:             token.BalanceUSD(),
		DefaultBillingGroupID:  token.DefaultBillingGroupID,
		SelectedBillingGroupID: selectedGroupID,
		BillingGroups:          customerBillingGroups(groups),
		BillingGroupUsage:      groupUsage,
		BalanceTransactions:    transactions,
	})
}

func parsePublicBillingGroupID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	groupID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || groupID < 0 {
		return 0, fmt.Errorf("billing_group_id must be a non-negative integer")
	}
	return groupID, nil
}

func containsBillingGroup(groups []*model.BillingGroup, groupID int64) bool {
	for _, group := range groups {
		if group != nil && group.ID == groupID {
			return true
		}
	}
	return false
}

// HandlePublicKeyUsagePage serves /key-usage only to a URL that
// carries an existing key. Missing or unknown keys deliberately look like a
// nonexistent page rather than an authentication error.
func (s *Server) HandlePublicKeyUsagePage(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	if _, ok := s.findPublicUsageToken(ctx, c.Query("key")); !ok || embedFS == nil {
		respondPublicUsageNotFound(c)
		return
	}

	serveHTMLWithVersionFrom(c, embedFS, "key-usage.html")
}

// HandlePublicKeyUsageDefaultGroup changes the default group from the
// customer-facing usage page. Possession of the key URL is the capability,
// matching the existing public usage endpoint semantics.
func (s *Server) HandlePublicKeyUsageDefaultGroup(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	token, ok := s.findPublicUsageToken(ctx, c.Query("key"))
	if !ok {
		respondPublicUsageNotFound(c)
		return
	}
	var req struct {
		GroupID int64 `json:"group_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.GroupID <= 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "group_id is required")
		return
	}
	group, err := s.store.GetBillingGroup(ctx, req.GroupID)
	if err != nil || group == nil || !group.Enabled {
		RespondErrorMsg(c, http.StatusBadRequest, "billing group is unavailable")
		return
	}
	token.DefaultBillingGroupID = req.GroupID
	if err := s.store.UpdateAuthToken(ctx, token); err != nil {
		RespondError(c, http.StatusInternalServerError, err)
		return
	}
	RespondJSON(c, http.StatusOK, gin.H{"default_billing_group_id": req.GroupID, "default_billing_group_slug": group.Slug})
}

func (s *Server) findPublicUsageToken(ctx context.Context, key string) (*model.AuthToken, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, false
	}

	token, err := s.store.GetAuthTokenByValue(ctx, model.HashToken(key))
	if err != nil {
		if base, _, ok := splitBillingGroupVariant(key); ok {
			token, err = s.store.GetAuthTokenByValue(ctx, model.HashToken(base))
		}
	}
	if err != nil || token == nil {
		return nil, false
	}
	return token, true
}

func respondPublicUsageNotFound(c *gin.Context) {
	c.Status(http.StatusNotFound)
	c.Abort()
}

func buildPublicKeyTodayUsage(points []model.MetricPoint) (PublicKeyTodayUsage, []PublicKeyUsageTrendPoint) {
	today := PublicKeyTodayUsage{}
	trend := make([]PublicKeyUsageTrendPoint, 0, len(points))
	for _, point := range points {
		cost := 0.0
		if point.EffectiveCost != nil {
			cost = *point.EffectiveCost
		}
		tokens := point.InputTokens + point.OutputTokens + point.CacheReadTokens + point.CacheCreationTokens
		today.RequestCount += point.Success + point.Error
		today.TotalTokens += tokens
		today.EffectiveCost += cost
		trend = append(trend, PublicKeyUsageTrendPoint{
			Timestamp:     point.Ts.UnixMilli(),
			EffectiveCost: cost,
			TotalTokens:   tokens,
		})
	}
	return today, trend
}

func buildPublicKeyBalanceUsage(rows []model.BalanceUsageBucket, since, until time.Time, bucket time.Duration) (PublicKeyTodayUsage, []PublicKeyModelUsage, []PublicKeyUsageTrendPoint) {
	type totals struct {
		tokens  int64
		charged int64
	}

	todayMicro := int64(0)
	modelTotals := make(map[string]*totals)
	trendTotals := make(map[int64]*totals)
	today := PublicKeyTodayUsage{}
	for _, row := range rows {
		today.RequestCount += row.RequestCount
		today.TotalTokens += row.TotalTokens
		todayMicro += row.ChargedMicroUSD

		name := strings.TrimSpace(row.Model)
		if name == "" {
			name = "未知模型"
		}
		modelTotal := modelTotals[name]
		if modelTotal == nil {
			modelTotal = &totals{}
			modelTotals[name] = modelTotal
		}
		modelTotal.tokens += row.TotalTokens
		modelTotal.charged += row.ChargedMicroUSD

		bucketKey := row.Timestamp.UnixMilli()
		trendTotal := trendTotals[bucketKey]
		if trendTotal == nil {
			trendTotal = &totals{}
			trendTotals[bucketKey] = trendTotal
		}
		trendTotal.tokens += row.TotalTokens
		trendTotal.charged += row.ChargedMicroUSD
	}
	today.EffectiveCost = util.MicroUSDToUSD(todayMicro)

	modelUsage := make([]PublicKeyModelUsage, 0, len(modelTotals))
	for name, item := range modelTotals {
		if item.tokens <= 0 && item.charged <= 0 {
			continue
		}
		modelUsage = append(modelUsage, PublicKeyModelUsage{
			Model:         name,
			TotalTokens:   item.tokens,
			EffectiveCost: util.MicroUSDToUSD(item.charged),
		})
	}
	sort.Slice(modelUsage, func(i, j int) bool {
		if modelUsage[i].TotalTokens != modelUsage[j].TotalTokens {
			return modelUsage[i].TotalTokens > modelUsage[j].TotalTokens
		}
		if modelUsage[i].EffectiveCost != modelUsage[j].EffectiveCost {
			return modelUsage[i].EffectiveCost > modelUsage[j].EffectiveCost
		}
		return modelUsage[i].Model < modelUsage[j].Model
	})

	if bucket <= 0 {
		bucket = publicKeyUsageBucket
	}
	trend := make([]PublicKeyUsageTrendPoint, 0, int(until.Sub(since)/bucket)+1)
	for timestamp := since; !timestamp.After(until); timestamp = timestamp.Add(bucket) {
		item := trendTotals[timestamp.UnixMilli()]
		point := PublicKeyUsageTrendPoint{Timestamp: timestamp.UnixMilli()}
		if item != nil {
			point.TotalTokens = item.tokens
			point.EffectiveCost = util.MicroUSDToUSD(item.charged)
		}
		trend = append(trend, point)
	}
	return today, modelUsage, trend
}

func buildPublicKeyModelUsage(rows []model.ModelUsageStat) []PublicKeyModelUsage {
	type totals struct {
		tokens int64
		cost   float64
	}

	byModel := make(map[string]*totals, len(rows))
	for _, row := range rows {
		name := strings.TrimSpace(row.Model)
		if name == "" {
			name = "未知模型"
		}
		item := byModel[name]
		if item == nil {
			item = &totals{}
			byModel[name] = item
		}
		item.tokens += row.TotalTokens
		item.cost += row.EffectiveCost
	}

	usage := make([]PublicKeyModelUsage, 0, len(byModel))
	for name, item := range byModel {
		if item.tokens <= 0 && item.cost <= 0 {
			continue
		}
		usage = append(usage, PublicKeyModelUsage{
			Model:         name,
			TotalTokens:   item.tokens,
			EffectiveCost: item.cost,
		})
	}
	sort.Slice(usage, func(i, j int) bool {
		if usage[i].TotalTokens != usage[j].TotalTokens {
			return usage[i].TotalTokens > usage[j].TotalTokens
		}
		if usage[i].EffectiveCost != usage[j].EffectiveCost {
			return usage[i].EffectiveCost > usage[j].EffectiveCost
		}
		return usage[i].Model < usage[j].Model
	})
	return usage
}

func (s *Server) buildPublicKeyCostQuota(ctx context.Context, token *model.AuthToken) PublicKeyCostQuota {
	if token == nil {
		return PublicKeyCostQuota{}
	}
	if token.BalanceEnabled {
		return PublicKeyCostQuota{}
	}
	token.NormalizeDailyCostForToday()
	token.NormalizeMonthlyCostForCurrentMonth()

	if token.GroupID > 0 {
		group, err := s.store.GetAuthTokenGroup(ctx, token.GroupID)
		if err == nil {
			token.ApplyGroupEffective(group)
		} else {
			token.ApplyGroupEffective(nil)
		}
	} else {
		token.ApplyGroupEffective(nil)
	}

	usedMicro := int64(0)
	limitMicro := effectiveDailyLimitMicro(token)
	if limitMicro > 0 {
		usedMicro = token.DailyCostUsedMicroUSD
		if s.authService != nil {
			if used, limit, _ := s.authService.IsDailyCostLimitExceeded(token.Token); limit > 0 {
				usedMicro = used
				limitMicro = limit
			}
		}
	} else if monthlyLimit := effectiveMonthlyLimitMicro(token); monthlyLimit > 0 {
		usedMicro = token.MonthlyCostUsedMicroUSD
		limitMicro = monthlyLimit
		if s.authService != nil {
			if used, limit, _ := s.authService.IsMonthlyCostLimitExceeded(token.Token); limit > 0 {
				usedMicro = used
				limitMicro = limit
			}
		}
	} else {
		usedMicro = token.CostUsedMicroUSD
		limitMicro = effectiveCostLimitMicro(token)
		if s.authService != nil {
			if used, limit, _ := s.authService.IsCostLimitExceeded(token.Token); limit > 0 {
				usedMicro = used
				limitMicro = limit
			}
		}
	}

	quota := PublicKeyCostQuota{UsedUSD: util.MicroUSDToUSD(usedMicro)}
	if limitMicro <= 0 {
		return quota
	}
	limitUSD := util.MicroUSDToUSD(limitMicro)
	percentage := float64(usedMicro) * 100 / float64(limitMicro)
	quota.LimitUSD = &limitUSD
	quota.UsagePercentage = &percentage
	return quota
}
