package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"ccLoad/internal/model"

	"github.com/gin-gonic/gin"
)

// ============================================================================
// API访问令牌管理 (Admin API)
// ============================================================================

type optionalInt64JSON struct {
	set   bool
	value *int64
}

func firstFloat64(values ...*float64) *float64 {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func (v *optionalInt64JSON) UnmarshalJSON(data []byte) error {
	v.set = true
	if string(data) == "null" {
		v.value = nil
		return nil
	}

	var n int64
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	v.value = &n
	return nil
}

// HandleListAuthTokens 列出 API 访问令牌（支持名称搜索、分页、时间范围统计）
// GET /admin/auth-tokens?range=today&search=foo&limit=200&offset=0
func (s *Server) HandleListAuthTokens(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	tokens, err := s.store.ListAuthTokens(ctx)
	if err != nil {
		log.Print("[ERROR] 列出令牌失败: " + err.Error())
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	if tokens == nil {
		tokens = make([]*model.AuthToken, 0)
	}
	groups, groupByID, err := s.loadAuthTokenGroupsForAdmin(ctx)
	if err != nil {
		log.Printf("[WARN]  查询令牌分组失败: %v", err)
		groups = []*model.AuthTokenGroup{}
		groupByID = map[int64]*model.AuthTokenGroup{}
	}
	applyAuthTokenGroupEffective(tokens, groupByID)
	tokens = filterAuthTokenList(tokens, c)

	params := ParsePaginationParams(c)
	totalCount := len(tokens)
	hasPagination := c.Query("limit") != "" || c.Query("offset") != ""

	type AuthTokenListResponse struct {
		Tokens          []*model.AuthToken      `json:"tokens"`
		Groups          []*model.AuthTokenGroup `json:"groups"`
		TotalCount      int                     `json:"total_count"`
		Limit           int                     `json:"limit"`
		Offset          int                     `json:"offset"`
		DurationSeconds float64                 `json:"duration_seconds,omitempty"`
		RPMStats        *model.RPMStats         `json:"rpm_stats,omitempty"`
		IsToday         bool                    `json:"is_today"`
	}

	resp := AuthTokenListResponse{
		Tokens:     tokens,
		Groups:     groups,
		TotalCount: totalCount,
		Limit:      params.Limit,
		Offset:     params.Offset,
		IsToday:    false,
	}

	// 如果请求中包含range参数，则叠加时间范围统计（用于tokens.html页面）
	timeRange := strings.TrimSpace(c.Query("range"))
	if timeRange != "" && timeRange != "all" {
		startTime, endTime := params.GetTimeRange()

		// 计算时间跨度（秒），用于前端计算RPM和QPS
		resp.DurationSeconds = endTime.Sub(startTime).Seconds()
		if resp.DurationSeconds < 1 {
			resp.DurationSeconds = 1 // 防止除零
		}

		// 判断是否为本日（本日才计算最近一分钟）
		isToday := timeRange == "today"
		resp.IsToday = isToday

		// 获取全局RPM统计（峰值、平均、最近一分钟）
		rpmStats, err := s.store.GetRPMStats(ctx, startTime, endTime, nil, isToday)
		if err != nil {
			log.Printf("[WARN]  查询RPM统计失败: %v", err)
			// 降级处理
		}
		resp.RPMStats = rpmStats

		// 从logs表聚合时间范围内的统计
		rangeStats, err := s.store.GetAuthTokenStatsInRange(ctx, startTime, endTime)
		if err != nil {
			log.Printf("[WARN]  查询时间范围统计失败: %v", err)
			// 降级处理：统计查询失败不影响token列表返回，仅记录警告
		} else {
			// 计算每个token的RPM统计（峰值、平均、最近）
			if err := s.store.FillAuthTokenRPMStats(ctx, rangeStats, startTime, endTime, isToday); err != nil {
				log.Printf("[WARN]  计算token RPM统计失败: %v", err)
			}

			// 将时间范围统计叠加到每个token的响应中
			for _, t := range tokens {
				if stat, ok := rangeStats[t.ID]; ok {
					// 用时间范围统计覆盖累计统计字段（前端透明）
					t.SuccessCount = stat.SuccessCount
					t.FailureCount = stat.FailureCount
					t.PromptTokensTotal = stat.PromptTokens
					t.CompletionTokensTotal = stat.CompletionTokens
					t.CacheReadTokensTotal = stat.CacheReadTokens
					t.CacheCreationTokensTotal = stat.CacheCreationTokens
					t.TotalCostUSD = stat.TotalCost
					t.EffectiveCostUSD = stat.EffectiveCost
					t.StreamAvgTTFB = stat.StreamAvgTTFB
					t.NonStreamAvgRT = stat.NonStreamAvgRT
					t.StreamCount = stat.StreamCount
					t.NonStreamCount = stat.NonStreamCount
					// RPM统计
					t.PeakRPM = stat.PeakRPM
					t.AvgRPM = stat.AvgRPM
					t.RecentRPM = stat.RecentRPM
				} else {
					// 该token在此时间范围内无数据，清零统计字段
					t.SuccessCount = 0
					t.FailureCount = 0
					t.PromptTokensTotal = 0
					t.CompletionTokensTotal = 0
					t.CacheReadTokensTotal = 0
					t.CacheCreationTokensTotal = 0
					t.TotalCostUSD = 0
					t.EffectiveCostUSD = 0
					t.StreamAvgTTFB = 0
					t.NonStreamAvgRT = 0
					t.StreamCount = 0
					t.NonStreamCount = 0
					t.PeakRPM = 0
					t.AvgRPM = 0
					t.RecentRPM = 0
				}
			}
		}

	}

	if hasPagination {
		resp.Tokens = paginateAuthTokens(tokens, params)
	} else {
		resp.Tokens = tokens
		resp.Limit = totalCount
		resp.Offset = 0
	}

	RespondJSON(c, http.StatusOK, resp)
}

func filterAuthTokenList(tokens []*model.AuthToken, c *gin.Context) []*model.AuthToken {
	exactName := strings.TrimSpace(c.Query("token_name"))
	if exactName == "" {
		exactName = strings.TrimSpace(c.Query("description"))
	}
	search := strings.TrimSpace(c.Query("search"))
	searchLower := strings.ToLower(search)

	if exactName == "" && search == "" {
		return tokens
	}

	filtered := make([]*model.AuthToken, 0, len(tokens))
	if exactName != "" {
		for _, token := range tokens {
			if token == nil {
				continue
			}
			if strings.TrimSpace(token.Description) == exactName ||
				strings.TrimSpace(token.GroupName) == exactName {
				filtered = append(filtered, token)
			}
		}
		return filtered
	}

	for _, token := range tokens {
		if token == nil {
			continue
		}
		if strings.Contains(strings.ToLower(strings.TrimSpace(token.Description)), searchLower) ||
			strings.Contains(strings.ToLower(strings.TrimSpace(token.GroupName)), searchLower) ||
			strings.Contains(strings.ToLower(strings.TrimSpace(token.PlainToken)), searchLower) {
			filtered = append(filtered, token)
		}
	}
	return filtered
}

func paginateAuthTokens(tokens []*model.AuthToken, params *PaginationParams) []*model.AuthToken {
	if params == nil {
		params = &PaginationParams{}
		params.SetDefaults()
	}
	limit := params.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= len(tokens) {
		return []*model.AuthToken{}
	}
	end := min(offset+limit, len(tokens))
	return tokens[offset:end]
}

func (s *Server) loadAuthTokenGroupsForAdmin(ctx context.Context) ([]*model.AuthTokenGroup, map[int64]*model.AuthTokenGroup, error) {
	groups, err := s.store.ListAuthTokenGroups(ctx)
	if err != nil {
		return nil, nil, err
	}
	groupByID := make(map[int64]*model.AuthTokenGroup, len(groups))
	for _, group := range groups {
		if group != nil {
			groupByID[group.ID] = group
		}
	}
	return groups, groupByID, nil
}

func applyAuthTokenGroupEffective(tokens []*model.AuthToken, groupByID map[int64]*model.AuthTokenGroup) {
	for _, token := range tokens {
		if token == nil {
			continue
		}
		token.ApplyGroupEffective(groupByID[token.GroupID])
	}
}

func generateRandomAuthTokenPlainText() (string, error) {
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	return "sk-" + hex.EncodeToString(tokenBytes), nil
}

func isDuplicateAuthTokenError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate entry") ||
		strings.Contains(msg, "unique constraint failed") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicated")
}

func applyDailyLimitAdjustmentRequest(token *model.AuthToken, doubleEnabled, tripleEnabled *bool, overrideUSD *float64) error {
	activeAdjustments := 0
	if doubleEnabled != nil && *doubleEnabled {
		activeAdjustments++
	}
	if tripleEnabled != nil && *tripleEnabled {
		activeAdjustments++
	}
	if overrideUSD != nil && *overrideUSD > 0 {
		activeAdjustments++
	}
	if activeAdjustments > 1 {
		return errors.New("daily_limit_double_enabled, daily_limit_triple_enabled, and daily_limit_override_usd are mutually exclusive")
	}
	if overrideUSD != nil && *overrideUSD > 0 {
		token.SetDailyLimitOverrideUSDForToday(*overrideUSD)
		return nil
	}
	if tripleEnabled != nil && *tripleEnabled {
		token.SetDailyLimitMultiplierForToday(3)
		return nil
	}
	if doubleEnabled != nil && *doubleEnabled {
		token.SetDailyLimitMultiplierForToday(2)
		return nil
	}
	if doubleEnabled != nil {
		token.DailyLimitDoubleDayKey = 0
	}
	if tripleEnabled != nil {
		token.DailyLimitTripleDayKey = 0
	}
	if overrideUSD != nil {
		token.ClearDailyLimitOverride()
	}
	return nil
}

// HandleCreateAuthToken 创建新的API访问令牌
// POST /admin/auth-tokens
func (s *Server) HandleCreateAuthToken(c *gin.Context) {
	var req struct {
		Description            string   `json:"description" binding:"required"`
		PlainToken             *string  `json:"plain_token"`
		ExpiresAt              *int64   `json:"expires_at"`          // Unix毫秒时间戳，nil表示永不过期
		IsActive               *bool    `json:"is_active"`           // nil表示默认启用
		AllowedModels          []string `json:"allowed_models"`      // 允许的模型列表，空表示无限制
		AllowedChannelIDs      []int64  `json:"allowed_channel_ids"` // 渠道限制列表，空表示无限制
		ChannelRestrictionMode string   `json:"channel_restriction_mode"`
		CostLimitUSD           *float64 `json:"cost_limit_usd"`       // 费用上限（0=无限制）
		DailyCostLimitUSD      *float64 `json:"daily_cost_limit_usd"` // 当日费用上限（0=无限制）
		MonthlyCostLimitUSD    *float64 `json:"monthly_cost_limit_usd"`
		CostMonthlyLimitUSD    *float64 `json:"cost_monthly_limit_usd"` // 上游命名兼容
		DailyLimitDouble       *bool    `json:"daily_limit_double_enabled"`
		DailyLimitTriple       *bool    `json:"daily_limit_triple_enabled"`
		DailyLimitOverrideUSD  *float64 `json:"daily_limit_override_usd"`
		MaxConcurrency         *int     `json:"max_concurrency"` // 最大并发请求数（0=无限制）
		CodexGuardEnabled      *bool    `json:"codex_guard_enabled"`
		GroupID                *int64   `json:"group_id"` // 分组ID，0/空表示未分组
		InheritQuota           *bool    `json:"inherit_quota"`
		InheritChannels        *bool    `json:"inherit_channels"`
		InheritModels          *bool    `json:"inherit_models"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.CostLimitUSD != nil && *req.CostLimitUSD < 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "cost_limit_usd must be >= 0")
		return
	}
	monthlyCostLimitUSD := firstFloat64(req.MonthlyCostLimitUSD, req.CostMonthlyLimitUSD)
	if req.DailyCostLimitUSD != nil && *req.DailyCostLimitUSD < 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "daily_cost_limit_usd must be >= 0")
		return
	}
	if monthlyCostLimitUSD != nil && *monthlyCostLimitUSD < 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "monthly_cost_limit_usd must be >= 0")
		return
	}
	if req.DailyLimitOverrideUSD != nil && *req.DailyLimitOverrideUSD < 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "daily_limit_override_usd must be >= 0")
		return
	}
	if req.MaxConcurrency != nil && *req.MaxConcurrency < 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "max_concurrency must be >= 0")
		return
	}
	if req.GroupID != nil && *req.GroupID < 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "group_id must be >= 0")
		return
	}
	channelRestrictionMode, err := model.NormalizeChannelRestrictionMode(req.ChannelRestrictionMode)
	if err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}

	tokenPlain := ""
	if req.PlainToken != nil {
		tokenPlain = strings.TrimSpace(*req.PlainToken)
	}
	if tokenPlain == "" {
		var err error
		tokenPlain, err = generateRandomAuthTokenPlainText()
		if err != nil {
			log.Print("[ERROR] 生成令牌失败: " + err.Error())
			RespondError(c, http.StatusInternalServerError, err)
			return
		}
	}

	// 计算SHA256哈希用于存储
	tokenHash := model.HashToken(tokenPlain)

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	authToken := &model.AuthToken{
		Token:                  tokenHash,
		PlainToken:             tokenPlain,
		Description:            req.Description,
		ExpiresAt:              req.ExpiresAt,
		IsActive:               isActive,
		CodexGuardEnabled:      req.CodexGuardEnabled != nil && *req.CodexGuardEnabled,
		AllowedModels:          req.AllowedModels,
		AllowedChannelIDs:      req.AllowedChannelIDs,
		ChannelRestrictionMode: channelRestrictionMode,
	}
	if req.GroupID != nil {
		authToken.GroupID = *req.GroupID
	}
	if authToken.GroupID > 0 {
		authToken.InheritQuota = true
		authToken.InheritChannels = true
		authToken.InheritModels = true
	}
	if req.InheritQuota != nil {
		authToken.InheritQuota = *req.InheritQuota
	}
	if req.InheritChannels != nil {
		authToken.InheritChannels = *req.InheritChannels
	}
	if req.InheritModels != nil {
		authToken.InheritModels = *req.InheritModels
	}
	if authToken.GroupID == 0 {
		authToken.InheritQuota = false
		authToken.InheritChannels = false
		authToken.InheritModels = false
	}
	if req.CostLimitUSD != nil {
		authToken.SetCostLimitUSD(*req.CostLimitUSD)
	}
	if req.DailyCostLimitUSD != nil {
		authToken.SetDailyCostLimitUSD(*req.DailyCostLimitUSD)
	}
	if monthlyCostLimitUSD != nil {
		authToken.SetMonthlyCostLimitUSD(*monthlyCostLimitUSD)
	}
	if err := applyDailyLimitAdjustmentRequest(authToken, req.DailyLimitDouble, req.DailyLimitTriple, req.DailyLimitOverrideUSD); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.MaxConcurrency != nil {
		authToken.MaxConcurrency = *req.MaxConcurrency
	}
	if err := authToken.ValidateUsageLimits(); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	if authToken.GroupID > 0 {
		if _, err := s.store.GetAuthTokenGroup(ctx, authToken.GroupID); err != nil {
			RespondErrorMsg(c, http.StatusBadRequest, "auth token group not found")
			return
		}
	}

	if err := s.store.CreateAuthToken(ctx, authToken); err != nil {
		if isDuplicateAuthTokenError(err) {
			RespondErrorMsg(c, http.StatusConflict, "auth token already exists")
			return
		}
		log.Print("[ERROR] 创建令牌失败: " + err.Error())
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	// 触发热更新（立即生效）
	if err := s.authService.ReloadAuthTokens(); err != nil {
		log.Print("[WARN]  热更新失败: " + err.Error())
	}

	log.Printf("[INFO] 创建API令牌: ID=%d, 描述=%s", authToken.ID, authToken.Description)

	// 返回明文令牌
	RespondJSON(c, http.StatusOK, gin.H{
		"id":                         authToken.ID,
		"token":                      tokenPlain, // 明文令牌，仅创建时返回
		"plain_token":                tokenPlain,
		"description":                authToken.Description,
		"created_at":                 authToken.CreatedAt,
		"expires_at":                 authToken.ExpiresAt,
		"is_active":                  authToken.IsActive,
		"codex_guard_enabled":        authToken.CodexGuardEnabled,
		"allowed_models":             authToken.AllowedModels,
		"allowed_channel_ids":        authToken.AllowedChannelIDs,
		"channel_restriction_mode":   authToken.ChannelRestrictionMode,
		"daily_cost_limit_usd":       authToken.DailyCostLimitUSD(),
		"monthly_cost_limit_usd":     authToken.MonthlyCostLimitUSD(),
		"cost_monthly_limit_usd":     authToken.MonthlyCostLimitUSD(),
		"daily_limit_double_enabled": authToken.IsDailyLimitDoubledToday(),
		"daily_limit_triple_enabled": authToken.IsDailyLimitTripledToday(),
		"daily_limit_override_usd":   authToken.DailyLimitOverrideUSDForToday(),
		"max_concurrency":            authToken.MaxConcurrency,
		"group_id":                   authToken.GroupID,
		"inherit_quota":              authToken.InheritQuota,
		"inherit_channels":           authToken.InheritChannels,
		"inherit_models":             authToken.InheritModels,
	})
}

// HandleUpdateAuthToken 更新令牌信息
// PUT /admin/auth-tokens/:id
func (s *Server) HandleUpdateAuthToken(c *gin.Context) {
	id, err := ParseInt64Param(c, "id")
	if err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid token id")
		return
	}

	var req struct {
		PlainToken             *string           `json:"plain_token"`
		Description            *string           `json:"description"`
		IsActive               *bool             `json:"is_active"`
		ExpiresAt              optionalInt64JSON `json:"expires_at"`
		AllowedModels          *[]string         `json:"allowed_models"`      // nil=不更新，空数组=清除限制
		AllowedChannelIDs      *[]int64          `json:"allowed_channel_ids"` // nil=不更新，空数组=清除限制
		ChannelRestrictionMode *string           `json:"channel_restriction_mode"`
		CostLimitUSD           *float64          `json:"cost_limit_usd"`       // 费用上限（0=无限制）
		DailyCostLimitUSD      *float64          `json:"daily_cost_limit_usd"` // 当日费用上限（0=无限制）
		MonthlyCostLimitUSD    *float64          `json:"monthly_cost_limit_usd"`
		CostMonthlyLimitUSD    *float64          `json:"cost_monthly_limit_usd"` // 上游命名兼容
		DailyLimitDouble       *bool             `json:"daily_limit_double_enabled"`
		DailyLimitTriple       *bool             `json:"daily_limit_triple_enabled"`
		DailyLimitOverrideUSD  *float64          `json:"daily_limit_override_usd"`
		MaxConcurrency         *int              `json:"max_concurrency"` // 最大并发请求数（0=无限制）
		CodexGuardEnabled      *bool             `json:"codex_guard_enabled"`
		GroupID                *int64            `json:"group_id"` // 分组ID，0表示未分组
		InheritQuota           *bool             `json:"inherit_quota"`
		InheritChannels        *bool             `json:"inherit_channels"`
		InheritModels          *bool             `json:"inherit_models"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.CostLimitUSD != nil && *req.CostLimitUSD < 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "cost_limit_usd must be >= 0")
		return
	}
	monthlyCostLimitUSD := firstFloat64(req.MonthlyCostLimitUSD, req.CostMonthlyLimitUSD)
	if req.DailyCostLimitUSD != nil && *req.DailyCostLimitUSD < 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "daily_cost_limit_usd must be >= 0")
		return
	}
	if monthlyCostLimitUSD != nil && *monthlyCostLimitUSD < 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "monthly_cost_limit_usd must be >= 0")
		return
	}
	if req.DailyLimitOverrideUSD != nil && *req.DailyLimitOverrideUSD < 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "daily_limit_override_usd must be >= 0")
		return
	}
	if req.MaxConcurrency != nil && *req.MaxConcurrency < 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "max_concurrency must be >= 0")
		return
	}
	if req.GroupID != nil && *req.GroupID < 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "group_id must be >= 0")
		return
	}
	var channelRestrictionMode string
	if req.ChannelRestrictionMode != nil {
		channelRestrictionMode, err = model.NormalizeChannelRestrictionMode(*req.ChannelRestrictionMode)
		if err != nil {
			RespondErrorMsg(c, http.StatusBadRequest, err.Error())
			return
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// 获取现有令牌
	token, err := s.store.GetAuthToken(ctx, id)
	if err != nil {
		RespondErrorMsg(c, http.StatusNotFound, "token not found")
		return
	}

	// 更新字段
	if req.PlainToken != nil {
		plainToken := strings.TrimSpace(*req.PlainToken)
		token.PlainToken = plainToken
		if plainToken != "" {
			token.Token = model.HashToken(plainToken)
		}
	}
	if req.Description != nil {
		token.Description = *req.Description
	}
	if req.IsActive != nil {
		token.IsActive = *req.IsActive
	}
	if req.CodexGuardEnabled != nil {
		token.CodexGuardEnabled = *req.CodexGuardEnabled
	}
	if req.ExpiresAt.set {
		token.ExpiresAt = req.ExpiresAt.value
	}
	if req.AllowedModels != nil {
		token.AllowedModels = *req.AllowedModels
	}
	if req.AllowedChannelIDs != nil {
		token.AllowedChannelIDs = *req.AllowedChannelIDs
	}
	if req.ChannelRestrictionMode != nil {
		token.ChannelRestrictionMode = channelRestrictionMode
	}
	if req.GroupID != nil {
		token.GroupID = *req.GroupID
	}
	if req.InheritQuota != nil {
		token.InheritQuota = *req.InheritQuota
	}
	if req.InheritChannels != nil {
		token.InheritChannels = *req.InheritChannels
	}
	if req.InheritModels != nil {
		token.InheritModels = *req.InheritModels
	}
	if token.GroupID == 0 {
		token.InheritQuota = false
		token.InheritChannels = false
		token.InheritModels = false
	}
	// cost_limit_usd 只有传入时才更新
	if req.CostLimitUSD != nil {
		token.SetCostLimitUSD(*req.CostLimitUSD)
	}
	if req.DailyCostLimitUSD != nil {
		token.SetDailyCostLimitUSD(*req.DailyCostLimitUSD)
	}
	if monthlyCostLimitUSD != nil {
		token.SetMonthlyCostLimitUSD(*monthlyCostLimitUSD)
	}
	if err := applyDailyLimitAdjustmentRequest(token, req.DailyLimitDouble, req.DailyLimitTriple, req.DailyLimitOverrideUSD); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.MaxConcurrency != nil {
		token.MaxConcurrency = *req.MaxConcurrency
	}
	if err := token.ValidateUsageLimits(); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	if token.GroupID > 0 {
		if _, err := s.store.GetAuthTokenGroup(ctx, token.GroupID); err != nil {
			RespondErrorMsg(c, http.StatusBadRequest, "auth token group not found")
			return
		}
	}

	if err := s.store.UpdateAuthToken(ctx, token); err != nil {
		if isDuplicateAuthTokenError(err) {
			RespondErrorMsg(c, http.StatusConflict, "auth token already exists")
			return
		}
		log.Print("[ERROR] 更新令牌失败: " + err.Error())
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	// 触发热更新
	if err := s.authService.ReloadAuthTokens(); err != nil {
		log.Print("[WARN]  热更新失败: " + err.Error())
	}

	if token.GroupID > 0 {
		if group, err := s.store.GetAuthTokenGroup(ctx, token.GroupID); err == nil {
			token.ApplyGroupEffective(group)
		}
	} else {
		token.ApplyGroupEffective(nil)
	}

	RespondJSON(c, http.StatusOK, token)
}

// HandleDeleteAuthToken 删除令牌
// DELETE /admin/auth-tokens/:id
func (s *Server) HandleDeleteAuthToken(c *gin.Context) {
	id, err := ParseInt64Param(c, "id")
	if err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid token id")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	if err := s.store.DeleteAuthToken(ctx, id); err != nil {
		log.Print("[ERROR] 删除令牌失败: " + err.Error())
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	// 触发热更新
	if err := s.authService.ReloadAuthTokens(); err != nil {
		log.Print("[WARN]  热更新失败: " + err.Error())
	}

	log.Printf("[INFO] 删除API令牌: ID=%d", id)

	RespondJSON(c, http.StatusOK, gin.H{"id": id})
}
