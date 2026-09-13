package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ccLoad/internal/model"
	"ccLoad/internal/util"

	"github.com/gin-gonic/gin"
)

type authTokenUsageResponse struct {
	IsActive              bool                        `json:"is_active"`
	IsValid               bool                        `json:"isValid"`
	Balance               any                         `json:"balance"`
	Remaining             any                         `json:"remaining"`
	Total                 any                         `json:"total"`
	Used                  float64                     `json:"used"`
	Unit                  string                      `json:"unit"`
	Extra                 string                      `json:"extra"`
	PlanName              string                      `json:"plan_name,omitempty"`
	PlanNameCamel         string                      `json:"planName,omitempty"`
	InvalidMessage        string                      `json:"invalid_message,omitempty"`
	InvalidMessageCamel   string                      `json:"invalidMessage,omitempty"`
	Error                 string                      `json:"error,omitempty"`
	LimitType             string                      `json:"limit_type,omitempty"`
	LimitTypeCamel        string                      `json:"limitType,omitempty"`
	DailyUsed             float64                     `json:"daily_used"`
	DailyUsedCamel        float64                     `json:"dailyUsed"`
	DailyLimit            *float64                    `json:"daily_limit,omitempty"`
	DailyLimitCamel       *float64                    `json:"dailyLimit,omitempty"`
	DailyRemaining        *float64                    `json:"daily_remaining,omitempty"`
	DailyRemainingCamel   *float64                    `json:"dailyRemaining,omitempty"`
	MonthlyUsed           float64                     `json:"monthly_used"`
	MonthlyUsedCamel      float64                     `json:"monthlyUsed"`
	MonthlyLimit          *float64                    `json:"monthly_limit,omitempty"`
	MonthlyLimitCamel     *float64                    `json:"monthlyLimit,omitempty"`
	MonthlyRemaining      *float64                    `json:"monthly_remaining,omitempty"`
	MonthlyRemainingCamel *float64                    `json:"monthlyRemaining,omitempty"`
	CostUsed              float64                     `json:"cost_used"`
	CostUsedCamel         float64                     `json:"costUsed"`
	CostLimit             *float64                    `json:"cost_limit,omitempty"`
	CostLimitCamel        *float64                    `json:"costLimit,omitempty"`
	CostRemaining         *float64                    `json:"cost_remaining,omitempty"`
	CostRemainingCamel    *float64                    `json:"costRemaining,omitempty"`
	UsagePercentage       *float64                    `json:"usage_percentage,omitempty"`
	UsagePercentageCamel  *float64                    `json:"usagePercentage,omitempty"`
	BalanceEnabled        bool                        `json:"balance_enabled"`
	BalanceUSD            float64                     `json:"balance_usd"`
	DefaultBillingGroupID int64                       `json:"default_billing_group_id,omitempty"`
	BillingGroups         []customerBillingGroup      `json:"billing_groups,omitempty"`
	BillingGroupUsage     []model.BillingGroupUsage   `json:"billing_group_usage,omitempty"`
	BalanceTransactions   []*model.BalanceTransaction `json:"balance_transactions,omitempty"`
}

// HandleAuthTokenUsage 返回当前 API Key 的额度/用量摘要。
// 兼容：
// - GET/POST /user/balance
// - GET/POST /api/usage
// - 以及 /balance、/usage 的短别名
func (s *Server) HandleAuthTokenUsage(c *gin.Context) {
	tokenHash := c.GetString("token_hash")
	if tokenHash == "" {
		c.JSON(http.StatusUnauthorized, authTokenUsageResponse{
			IsActive:            false,
			IsValid:             false,
			Balance:             nil,
			Remaining:           nil,
			Total:               nil,
			Used:                0,
			Unit:                "USD",
			Extra:               "无限制",
			InvalidMessage:      "invalid or missing authorization",
			InvalidMessageCamel: "invalid or missing authorization",
			Error:               "invalid or missing authorization",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	resp, err := s.buildAuthTokenUsageResponse(ctx, tokenHash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, authTokenUsageResponse{
			IsActive:            false,
			IsValid:             false,
			Balance:             nil,
			Remaining:           nil,
			Total:               nil,
			Used:                0,
			Unit:                "USD",
			Extra:               "无限制",
			InvalidMessage:      err.Error(),
			InvalidMessageCamel: err.Error(),
			Error:               err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// HandleSetAuthTokenDefaultBillingGroup lets a customer choose the group used
// by the unsuffixed token. Group visibility remains global for now.
func (s *Server) HandleSetAuthTokenDefaultBillingGroup(c *gin.Context) {
	tokenHash := c.GetString("token_hash")
	if tokenHash == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or missing authorization"})
		return
	}
	var req struct {
		GroupID int64 `json:"group_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.GroupID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group_id is required"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	group, err := s.store.GetBillingGroup(ctx, req.GroupID)
	if err != nil || group == nil || !group.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "billing group is unavailable"})
		return
	}
	token, err := s.store.GetAuthTokenByValue(ctx, tokenHash)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
		return
	}
	token.DefaultBillingGroupID = req.GroupID
	if err := s.store.UpdateAuthToken(ctx, token); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if s.authService != nil {
		_ = s.authService.ReloadAuthTokens()
	}
	c.JSON(http.StatusOK, gin.H{"default_billing_group_id": req.GroupID, "default_billing_group_slug": group.Slug})
}

func (s *Server) buildAuthTokenUsageResponse(ctx context.Context, tokenHash string) (authTokenUsageResponse, error) {
	token, err := s.store.GetAuthTokenByValue(ctx, tokenHash)
	if err != nil {
		return authTokenUsageResponse{}, err
	}
	token.NormalizeDailyCostForToday()
	token.NormalizeMonthlyCostForCurrentMonth()
	if token.GroupID > 0 {
		group, groupErr := s.store.GetAuthTokenGroup(ctx, token.GroupID)
		if groupErr == nil {
			token.ApplyGroupEffective(group)
		} else {
			token.ApplyGroupEffective(nil)
		}
	} else {
		token.ApplyGroupEffective(nil)
	}

	planName := strings.TrimSpace(token.GroupName)
	if planName == "" {
		planName = strings.TrimSpace(token.Description)
	}

	dailyUsedMicro := token.DailyCostUsedMicroUSD
	monthlyUsedMicro := token.MonthlyCostUsedMicroUSD
	costUsedMicro := token.CostUsedMicroUSD
	effectiveDailyLimitMicro := effectiveDailyLimitMicro(token)
	effectiveMonthlyLimitMicro := effectiveMonthlyLimitMicro(token)
	effectiveCostLimitMicro := effectiveCostLimitMicro(token)

	if s.authService != nil {
		if used, limit, _ := s.authService.IsDailyCostLimitExceeded(tokenHash); limit > 0 {
			dailyUsedMicro = used
			effectiveDailyLimitMicro = limit
		}
		if used, limit, _ := s.authService.IsMonthlyCostLimitExceeded(tokenHash); limit > 0 {
			monthlyUsedMicro = used
			effectiveMonthlyLimitMicro = limit
		}
		if used, limit, _ := s.authService.IsCostLimitExceeded(tokenHash); limit > 0 {
			costUsedMicro = used
			effectiveCostLimitMicro = limit
		}
	}

	dailyLimitPtr := microUSDPtr(effectiveDailyLimitMicro)
	dailyRemainingPtr := remainingMicroUSDPtr(effectiveDailyLimitMicro, dailyUsedMicro)
	monthlyLimitPtr := microUSDPtr(effectiveMonthlyLimitMicro)
	monthlyRemainingPtr := remainingMicroUSDPtr(effectiveMonthlyLimitMicro, monthlyUsedMicro)
	costLimitPtr := microUSDPtr(effectiveCostLimitMicro)
	costRemainingPtr := remainingMicroUSDPtr(effectiveCostLimitMicro, costUsedMicro)

	resp := authTokenUsageResponse{
		IsActive:              token.IsValid(),
		IsValid:               token.IsValid(),
		Balance:               nil,
		Remaining:             nil,
		Total:                 nil,
		Used:                  util.MicroUSDToUSD(dailyUsedMicro),
		Unit:                  "USD",
		Extra:                 "无限制",
		PlanName:              planName,
		PlanNameCamel:         planName,
		DailyUsed:             util.MicroUSDToUSD(dailyUsedMicro),
		DailyUsedCamel:        util.MicroUSDToUSD(dailyUsedMicro),
		DailyLimit:            dailyLimitPtr,
		DailyLimitCamel:       dailyLimitPtr,
		DailyRemaining:        dailyRemainingPtr,
		DailyRemainingCamel:   dailyRemainingPtr,
		MonthlyUsed:           util.MicroUSDToUSD(monthlyUsedMicro),
		MonthlyUsedCamel:      util.MicroUSDToUSD(monthlyUsedMicro),
		MonthlyLimit:          monthlyLimitPtr,
		MonthlyLimitCamel:     monthlyLimitPtr,
		MonthlyRemaining:      monthlyRemainingPtr,
		MonthlyRemainingCamel: monthlyRemainingPtr,
		CostUsed:              util.MicroUSDToUSD(costUsedMicro),
		CostUsedCamel:         util.MicroUSDToUSD(costUsedMicro),
		CostLimit:             costLimitPtr,
		CostLimitCamel:        costLimitPtr,
		CostRemaining:         costRemainingPtr,
		CostRemainingCamel:    costRemainingPtr,
		BalanceEnabled:        token.BalanceEnabled,
		BalanceUSD:            util.MicroUSDToUSD(token.BalanceMicroUSD),
		DefaultBillingGroupID: token.DefaultBillingGroupID,
	}
	var billingGroups []*model.BillingGroup
	if groups, groupsErr := s.store.ListBillingGroups(ctx); groupsErr == nil {
		for _, group := range groups {
			billingGroupAvailability(s, ctx, group)
		}
		billingGroups = groups
		resp.BillingGroups = customerBillingGroups(groups)
	}
	if token.BalanceEnabled {
		if usage, usageErr := s.store.GetAuthTokenBillingGroupUsage(ctx, token.ID, time.UnixMilli(0), time.Now()); usageErr == nil {
			resp.BillingGroupUsage = usage
			applyBillingGroupUsage(billingGroups, usage)
			resp.BillingGroups = customerBillingGroups(billingGroups)
		}
		if txs, txErr := s.store.ListAuthTokenBalanceTransactions(ctx, token.ID, 500, 0); txErr == nil {
			resp.BalanceTransactions = txs
		}
		resp.Balance = resp.BalanceUSD
		resp.Remaining = resp.BalanceUSD
		resp.Total = nil
		resp.LimitType = "balance"
		resp.LimitTypeCamel = "balance"
		resp.Extra = fmt.Sprintf("余额 $%.6f", resp.BalanceUSD)
		resp.UsagePercentage = nil
		resp.UsagePercentageCamel = nil
		if token.BalanceMicroUSD <= 0 {
			resp.IsActive = false
			resp.IsValid = false
			resp.InvalidMessage = "Balance exhausted"
			resp.InvalidMessageCamel = resp.InvalidMessage
			resp.Error = resp.InvalidMessage
		}
	}

	displayUsedMicro := int64(0)
	displayLimitMicro := int64(0)
	switch {
	case token.BalanceEnabled:
		// 钱包模式已在上方填充，旧限额仅保留为后台配置，不参与展示。
	case effectiveDailyLimitMicro > 0:
		resp.LimitType = "daily"
		resp.LimitTypeCamel = "daily"
		resp.Total = dailyLimitPtr
		resp.Balance = dailyRemainingPtr
		resp.Remaining = dailyRemainingPtr
		displayUsedMicro = dailyUsedMicro
		displayLimitMicro = effectiveDailyLimitMicro
	case effectiveMonthlyLimitMicro > 0:
		resp.LimitType = "monthly"
		resp.LimitTypeCamel = "monthly"
		resp.Total = monthlyLimitPtr
		resp.Balance = monthlyRemainingPtr
		resp.Remaining = monthlyRemainingPtr
		displayUsedMicro = monthlyUsedMicro
		displayLimitMicro = effectiveMonthlyLimitMicro
	case effectiveCostLimitMicro > 0:
		resp.LimitType = "total"
		resp.LimitTypeCamel = "total"
		resp.Total = costLimitPtr
		resp.Balance = costRemainingPtr
		resp.Remaining = costRemainingPtr
		displayUsedMicro = costUsedMicro
		displayLimitMicro = effectiveCostLimitMicro
	default:
		resp.LimitType = "unlimited"
		resp.LimitTypeCamel = "unlimited"
	}

	if displayLimitMicro > 0 {
		percentage := float64(displayUsedMicro) * 100 / float64(displayLimitMicro)
		if percentage < 0 {
			percentage = 0
		}
		resp.Extra = fmt.Sprintf("已使用 %.1f%%", percentage)
		resp.UsagePercentage = &percentage
		resp.UsagePercentageCamel = &percentage
	}

	if !token.IsActive {
		resp.IsActive = false
		resp.IsValid = false
		resp.InvalidMessage = "token is inactive"
		resp.InvalidMessageCamel = resp.InvalidMessage
		resp.Error = resp.InvalidMessage
	} else if token.IsExpired() {
		resp.IsActive = false
		resp.IsValid = false
		resp.InvalidMessage = "token expired"
		resp.InvalidMessageCamel = resp.InvalidMessage
		resp.Error = resp.InvalidMessage
	} else if !token.BalanceEnabled && effectiveCostLimitMicro > 0 && costUsedMicro >= effectiveCostLimitMicro {
		resp.IsActive = false
		resp.IsValid = false
		resp.InvalidMessage = fmt.Sprintf("Cost limit exceeded: $%.2f used of $%.2f limit", util.MicroUSDToUSD(costUsedMicro), util.MicroUSDToUSD(effectiveCostLimitMicro))
		resp.InvalidMessageCamel = resp.InvalidMessage
		resp.Error = resp.InvalidMessage
	} else if !token.BalanceEnabled && effectiveMonthlyLimitMicro > 0 && monthlyUsedMicro >= effectiveMonthlyLimitMicro {
		resp.IsActive = false
		resp.IsValid = false
		resp.InvalidMessage = fmt.Sprintf("Monthly cost limit exceeded: $%.2f used of $%.2f monthly limit", util.MicroUSDToUSD(monthlyUsedMicro), util.MicroUSDToUSD(effectiveMonthlyLimitMicro))
		resp.InvalidMessageCamel = resp.InvalidMessage
		resp.Error = resp.InvalidMessage
	} else if !token.BalanceEnabled && effectiveDailyLimitMicro > 0 && dailyUsedMicro >= effectiveDailyLimitMicro {
		resp.IsActive = false
		resp.IsValid = false
		resp.InvalidMessage = fmt.Sprintf("Daily cost limit exceeded: $%.2f used of $%.2f daily limit", util.MicroUSDToUSD(dailyUsedMicro), util.MicroUSDToUSD(effectiveDailyLimitMicro))
		resp.InvalidMessageCamel = resp.InvalidMessage
		resp.Error = resp.InvalidMessage
	}

	return resp, nil
}

func effectiveDailyLimitMicro(token *model.AuthToken) int64 {
	if token == nil {
		return 0
	}
	if token.EffectiveSet {
		return token.EffectiveDailyCostLimitMicroUSD
	}
	return token.DailyCostLimitMicroUSD
}

func effectiveCostLimitMicro(token *model.AuthToken) int64 {
	if token == nil {
		return 0
	}
	if token.EffectiveSet {
		return token.EffectiveCostLimitMicroUSD
	}
	return token.CostLimitMicroUSD
}

func effectiveMonthlyLimitMicro(token *model.AuthToken) int64 {
	if token == nil {
		return 0
	}
	if token.EffectiveSet {
		return token.EffectiveMonthlyCostLimitMicroUSD
	}
	return token.MonthlyCostLimitMicroUSD
}

func microUSDPtr(micro int64) *float64 {
	if micro <= 0 {
		return nil
	}
	value := util.MicroUSDToUSD(micro)
	return &value
}

func remainingMicroUSDPtr(limitMicro, usedMicro int64) *float64 {
	if limitMicro <= 0 {
		return nil
	}
	remaining := limitMicro - usedMicro
	if remaining < 0 {
		remaining = 0
	}
	value := util.MicroUSDToUSD(remaining)
	return &value
}
