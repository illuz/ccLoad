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
	IsActive             bool     `json:"is_active"`
	IsValid              bool     `json:"isValid"`
	Balance              any      `json:"balance"`
	Remaining            any      `json:"remaining"`
	Total                any      `json:"total"`
	Used                 float64  `json:"used"`
	Unit                 string   `json:"unit"`
	Extra                string   `json:"extra"`
	PlanName             string   `json:"plan_name,omitempty"`
	PlanNameCamel        string   `json:"planName,omitempty"`
	InvalidMessage       string   `json:"invalid_message,omitempty"`
	InvalidMessageCamel  string   `json:"invalidMessage,omitempty"`
	Error                string   `json:"error,omitempty"`
	LimitType            string   `json:"limit_type,omitempty"`
	LimitTypeCamel       string   `json:"limitType,omitempty"`
	DailyUsed            float64  `json:"daily_used"`
	DailyUsedCamel       float64  `json:"dailyUsed"`
	DailyLimit           *float64 `json:"daily_limit,omitempty"`
	DailyLimitCamel      *float64 `json:"dailyLimit,omitempty"`
	DailyRemaining       *float64 `json:"daily_remaining,omitempty"`
	DailyRemainingCamel  *float64 `json:"dailyRemaining,omitempty"`
	CostUsed             float64  `json:"cost_used"`
	CostUsedCamel        float64  `json:"costUsed"`
	CostLimit            *float64 `json:"cost_limit,omitempty"`
	CostLimitCamel       *float64 `json:"costLimit,omitempty"`
	CostRemaining        *float64 `json:"cost_remaining,omitempty"`
	CostRemainingCamel   *float64 `json:"costRemaining,omitempty"`
	UsagePercentage      *float64 `json:"usage_percentage,omitempty"`
	UsagePercentageCamel *float64 `json:"usagePercentage,omitempty"`
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
			Balance:             "-",
			Remaining:           "-",
			Total:               "-",
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
			Balance:             "-",
			Remaining:           "-",
			Total:               "-",
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

func (s *Server) buildAuthTokenUsageResponse(ctx context.Context, tokenHash string) (authTokenUsageResponse, error) {
	token, err := s.store.GetAuthTokenByValue(ctx, tokenHash)
	if err != nil {
		return authTokenUsageResponse{}, err
	}
	token.NormalizeDailyCostForToday()
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
	costUsedMicro := token.CostUsedMicroUSD
	effectiveDailyLimitMicro := effectiveDailyLimitMicro(token)
	effectiveCostLimitMicro := effectiveCostLimitMicro(token)

	if s.authService != nil {
		if used, limit, _ := s.authService.IsDailyCostLimitExceeded(tokenHash); limit > 0 {
			dailyUsedMicro = used
			effectiveDailyLimitMicro = limit
		}
		if used, limit, _ := s.authService.IsCostLimitExceeded(tokenHash); limit > 0 {
			costUsedMicro = used
			effectiveCostLimitMicro = limit
		}
	}

	dailyLimitPtr := microUSDPtr(effectiveDailyLimitMicro)
	dailyRemainingPtr := remainingMicroUSDPtr(effectiveDailyLimitMicro, dailyUsedMicro)
	costLimitPtr := microUSDPtr(effectiveCostLimitMicro)
	costRemainingPtr := remainingMicroUSDPtr(effectiveCostLimitMicro, costUsedMicro)

	resp := authTokenUsageResponse{
		IsActive:            token.IsValid(),
		IsValid:             token.IsValid(),
		Balance:             "-",
		Remaining:           "-",
		Total:               "-",
		Used:                util.MicroUSDToUSD(dailyUsedMicro),
		Unit:                "USD",
		Extra:               "无限制",
		PlanName:            planName,
		PlanNameCamel:       planName,
		DailyUsed:           util.MicroUSDToUSD(dailyUsedMicro),
		DailyUsedCamel:      util.MicroUSDToUSD(dailyUsedMicro),
		DailyLimit:          dailyLimitPtr,
		DailyLimitCamel:     dailyLimitPtr,
		DailyRemaining:      dailyRemainingPtr,
		DailyRemainingCamel: dailyRemainingPtr,
		CostUsed:            util.MicroUSDToUSD(costUsedMicro),
		CostUsedCamel:       util.MicroUSDToUSD(costUsedMicro),
		CostLimit:           costLimitPtr,
		CostLimitCamel:      costLimitPtr,
		CostRemaining:       costRemainingPtr,
		CostRemainingCamel:  costRemainingPtr,
	}

	displayUsedMicro := int64(0)
	displayLimitMicro := int64(0)
	switch {
	case effectiveDailyLimitMicro > 0:
		resp.LimitType = "daily"
		resp.LimitTypeCamel = "daily"
		resp.Total = dailyLimitPtr
		resp.Balance = dailyRemainingPtr
		resp.Remaining = dailyRemainingPtr
		displayUsedMicro = dailyUsedMicro
		displayLimitMicro = effectiveDailyLimitMicro
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
	} else if effectiveDailyLimitMicro > 0 && dailyUsedMicro >= effectiveDailyLimitMicro {
		resp.IsActive = false
		resp.IsValid = false
		resp.InvalidMessage = fmt.Sprintf("Daily cost limit exceeded: $%.2f used of $%.2f daily limit", util.MicroUSDToUSD(dailyUsedMicro), util.MicroUSDToUSD(effectiveDailyLimitMicro))
		resp.InvalidMessageCamel = resp.InvalidMessage
		resp.Error = resp.InvalidMessage
	} else if effectiveCostLimitMicro > 0 && costUsedMicro >= effectiveCostLimitMicro {
		resp.IsActive = false
		resp.IsValid = false
		resp.InvalidMessage = fmt.Sprintf("Cost limit exceeded: $%.2f used of $%.2f limit", util.MicroUSDToUSD(costUsedMicro), util.MicroUSDToUSD(effectiveCostLimitMicro))
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
