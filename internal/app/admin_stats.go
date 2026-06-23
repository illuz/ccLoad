package app

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"ccLoad/internal/model"
	"ccLoad/internal/util"
	"ccLoad/internal/version"

	"github.com/gin-gonic/gin"
)

// ==================== 统计和监控 ====================
// 从admin.go拆分统计监控,遵循SRP原则

// HandleErrors 获取日志列表
// GET /admin/logs?range=today&limit=100&offset=0
func (s *Server) HandleErrors(c *gin.Context) {
	params := ParsePaginationParams(c)
	lf := BuildLogFilter(c)
	since, until := params.GetTimeRange()

	logs, total, err := s.store.ListLogsRangeWithCount(c.Request.Context(), since, until, params.Limit, params.Offset, &lf)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	RespondJSONWithCount(c, http.StatusOK, logs, total)
}

// HandleMetrics 获取聚合指标数据
// GET /admin/metrics?range=today&bucket_min=5&channel_type=anthropic&model=claude-3-5-sonnet-20241022&channel_id=1&channel_name_like=xxx
func (s *Server) HandleMetrics(c *gin.Context) {
	params := ParsePaginationParams(c)
	bucketMin, _ := strconv.Atoi(c.DefaultQuery("bucket_min", "5"))
	if bucketMin <= 0 {
		bucketMin = 5
	}

	// 使用统一的筛选参数构建器（支持 channel_type、channel_id、channel_name_like、model、auth_token_id）
	lf := BuildLogFilter(c)
	lf.LogSource = model.LogSourceProxy

	since, until := params.GetTimeRange()
	pts, err := s.store.AggregateRangeWithFilter(c.Request.Context(), since, until, time.Duration(bucketMin)*time.Minute, &lf)

	if err != nil {
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	RespondJSON(c, http.StatusOK, pts)
}

// HandleStats 获取渠道和模型统计
// GET /admin/stats?range=today&channel_name_like=xxx&model_like=xxx
func (s *Server) HandleStats(c *gin.Context) {
	params := ParsePaginationParams(c)
	lf := BuildLogFilter(c)
	lf.LogSource = model.LogSourceProxy

	startTime, endTime := params.GetTimeRange()

	// 判断是否为本日（本日才计算最近一分钟）
	isToday := params.Range == "today" || params.Range == ""

	stats, err := s.statsCache.GetStats(c.Request.Context(), startTime, endTime, &lf, isToday)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	// 计算时间跨度（秒），用于前端计算RPM和QPS
	durationSeconds := endTime.Sub(startTime).Seconds()
	if durationSeconds < 1 {
		durationSeconds = 1 // 防止除零
	}

	// 获取RPM统计（峰值、平均、最近一分钟）
	rpmStats, err := s.statsCache.GetRPMStats(c.Request.Context(), startTime, endTime, &lf, isToday)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	// 计算健康时间线（固定48个时间点，当日显示最近4小时）
	channelHealth := s.fillHealthTimeline(c.Request.Context(), stats, startTime, endTime, &lf, isToday)

	RespondJSON(c, http.StatusOK, gin.H{
		"stats":            stats,
		"channel_health":   channelHealth,
		"duration_seconds": durationSeconds,
		"rpm_stats":        rpmStats,
		"is_today":         isToday,
	})
}

// HandlePublicSummary 获取基础统计摘要（路由层要求管理员登录）
// GET /public/summary?range=today
// 按渠道类型分组统计，Claude和Codex类型包含Token和成本信息
func (s *Server) HandlePublicSummary(c *gin.Context) {
	params := ParsePaginationParams(c)
	startTime, endTime := params.GetTimeRange()

	// 判断是否为本日（本日才计算最近一分钟）
	isToday := params.Range == "today" || params.Range == ""
	ctx := c.Request.Context()

	// [OPT] P1: 并行执行独立查询
	// 并行查询结果变量
	var (
		stats         []model.StatsEntry
		rpmStats      *model.RPMStats
		channelMetas  map[int64]ChannelMeta
		tokenCostRows []model.CostByChannelTokenRow
		authTokens    []*model.AuthToken
		statsErr      error
		rpmErr        error
		typesErr      error
		tokenCostErr  error
		authTokensErr error
		wg            sync.WaitGroup
	)

	wg.Add(5)

	// 查询1: 基础统计（使用 Lite 版本跳过 fillStatsRPM）
	go func() {
		defer wg.Done()
		stats, statsErr = s.statsCache.GetStatsLite(ctx, startTime, endTime, nil)
	}()

	// 查询2: RPM统计
	go func() {
		defer wg.Done()
		rpmStats, rpmErr = s.statsCache.GetRPMStats(ctx, startTime, endTime, nil, isToday)
	}()

	// 查询3: 渠道元信息映射（带缓存，含名称+类型）
	go func() {
		defer wg.Done()
		channelMetas, typesErr = s.getChannelMetaMapCached(ctx)
	}()

	// 查询4: (channel_id, auth_token_id) 维度的成本聚合（用于按 API 令牌拆分的饼图）
	go func() {
		defer wg.Done()
		tokenCostRows, tokenCostErr = s.store.GetCostByChannelAndToken(ctx, startTime, endTime)
	}()

	// 查询5: auth token 列表（用于把 auth_token_id 解析为描述/明文展示名）
	go func() {
		defer wg.Done()
		authTokens, authTokensErr = s.store.ListAuthTokens(ctx)
	}()

	wg.Wait()

	// 错误处理
	if statsErr != nil {
		RespondError(c, http.StatusInternalServerError, statsErr)
		return
	}
	if rpmErr != nil {
		RespondError(c, http.StatusInternalServerError, rpmErr)
		return
	}
	if typesErr != nil {
		RespondError(c, http.StatusInternalServerError, typesErr)
		return
	}
	if tokenCostErr != nil {
		RespondError(c, http.StatusInternalServerError, tokenCostErr)
		return
	}
	if authTokensErr != nil {
		RespondError(c, http.StatusInternalServerError, authTokensErr)
		return
	}

	// 计算时间跨度（秒），用于前端计算RPM和QPS
	durationSeconds := endTime.Sub(startTime).Seconds()
	if durationSeconds < 1 {
		durationSeconds = 1 // 防止除零
	}

	// 按渠道类型分组统计
	typeStats := make(map[string]*TypeSummary)
	// 临时累加器：按类型聚合 by-channel 数据，避免在 TypeSummary 上暴露内部状态
	type channelAgg struct {
		cost  float64
		name  string
	}
	byChannelAgg := make(map[string]map[int64]*channelAgg)
	totalSuccess := 0
	totalError := 0

	for _, stat := range stats {
		// 获取渠道类型与名称，跳过无法确定类型的记录（已删除的渠道）
		var channelType string
		var channelName string
		if stat.ChannelID != nil {
			if meta, ok := channelMetas[int64(*stat.ChannelID)]; ok {
				channelType = meta.Type
				channelName = meta.Name
			}
		}
		if channelType == "" {
			// 渠道已删除或类型未知，不计入按类型统计（与 /admin/stats 保持一致）
			continue
		}

		totalSuccess += stat.Success
		totalError += stat.Error

		// 初始化类型统计
		if _, exists := typeStats[channelType]; !exists {
			typeStats[channelType] = &TypeSummary{
				ChannelType:     channelType,
				TotalRequests:   0,
				SuccessRequests: 0,
				ErrorRequests:   0,
			}
			byChannelAgg[channelType] = make(map[int64]*channelAgg)
		}

		ts := typeStats[channelType]
		ts.TotalRequests += stat.Success + stat.Error
		ts.SuccessRequests += stat.Success
		ts.ErrorRequests += stat.Error

		// 所有渠道类型都统计Token和成本
		if stat.TotalInputTokens != nil {
			ts.TotalInputTokens += *stat.TotalInputTokens
		}
		if stat.TotalOutputTokens != nil {
			ts.TotalOutputTokens += *stat.TotalOutputTokens
		}
		if stat.TotalCost != nil {
			ts.TotalCost += *stat.TotalCost
		}
		if stat.EffectiveCost != nil {
			if ts.EffectiveCost == nil {
				ts.EffectiveCost = new(float64)
			}
			*ts.EffectiveCost += *stat.EffectiveCost
		} else if stat.TotalCost != nil {
			if ts.EffectiveCost == nil {
				ts.EffectiveCost = new(float64)
			}
			*ts.EffectiveCost += *stat.TotalCost
		}

		// Claude和Codex类型额外统计缓存（其他类型不支持prompt caching）
		isCacheType := channelType == "anthropic" || channelType == "codex"
		if isCacheType {
			if stat.TotalCacheReadInputTokens != nil {
				ts.TotalCacheReadTokens += *stat.TotalCacheReadInputTokens
			}
			if stat.TotalCacheCreationInputTokens != nil {
				ts.TotalCacheCreationTokens += *stat.TotalCacheCreationInputTokens
			}
		}

		// 按 channel 聚合 effective_cost（缺省回退到 total_cost）
		if stat.ChannelID != nil {
			chID := int64(*stat.ChannelID)
			eff := 0.0
			if stat.EffectiveCost != nil {
				eff = *stat.EffectiveCost
			} else if stat.TotalCost != nil {
				eff = *stat.TotalCost
			}
			if eff > 0 {
				agg, exists := byChannelAgg[channelType][chID]
				if !exists {
					agg = &channelAgg{name: channelName}
					byChannelAgg[channelType][chID] = agg
				}
				agg.cost += eff
			}
		}
	}

	// 把 by-channel 聚合结果转为按成本降序的切片（截断到前 N 名避免响应体膨胀）
	const maxChannelEntries = 20
	for channelType, agg := range byChannelAgg {
		ts := typeStats[channelType]
		if ts == nil || len(agg) == 0 {
			continue
		}
		entries := make([]ChannelCostEntry, 0, len(agg))
		for chID, a := range agg {
			entries = append(entries, ChannelCostEntry{
				ChannelID:   chID,
				ChannelName: a.name,
				Cost:        a.cost,
			})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Cost > entries[j].Cost
		})
		if len(entries) > maxChannelEntries {
			entries = entries[:maxChannelEntries]
		}
		ts.ByChannel = entries
	}

	// 按 (channel_type, auth_token_id) 聚合 effective_cost，用于「今日 API 令牌消费对比」饼图
	// auth_token_id → 展示名（描述优先，缺失则用明文令牌前缀，再缺失则用 #id）
	tokenDisplayName := make(map[int64]string, len(authTokens))
	for _, t := range authTokens {
		name := t.Description
		if name == "" && t.PlainToken != "" {
			name = t.PlainToken
		}
		if name == "" {
			name = "Token #" + strconv.FormatInt(t.ID, 10)
		}
		tokenDisplayName[t.ID] = name
	}
	type tokenAgg struct {
		id   int64
		name string
		cost float64
	}
	byTokenAgg := make(map[string]map[int64]*tokenAgg)
	for _, row := range tokenCostRows {
		meta, ok := channelMetas[row.ChannelID]
		if !ok || meta.Type == "" {
			continue
		}
		channelType := meta.Type
		if byTokenAgg[channelType] == nil {
			byTokenAgg[channelType] = make(map[int64]*tokenAgg)
		}
		agg, exists := byTokenAgg[channelType][row.AuthTokenID]
		if !exists {
			agg = &tokenAgg{id: row.AuthTokenID, name: tokenDisplayName[row.AuthTokenID]}
			byTokenAgg[channelType][row.AuthTokenID] = agg
		}
		agg.cost += row.EffectiveCost
	}
	const maxTokenEntries = 20
	for channelType, agg := range byTokenAgg {
		ts := typeStats[channelType]
		if ts == nil || len(agg) == 0 {
			continue
		}
		entries := make([]AuthTokenCostEntry, 0, len(agg))
		for _, a := range agg {
			if a.cost > 0 {
				entries = append(entries, AuthTokenCostEntry{
					AuthTokenID: a.id,
					Name:        a.name,
					Cost:        a.cost,
				})
			}
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Cost > entries[j].Cost
		})
		if len(entries) > maxTokenEntries {
			entries = entries[:maxTokenEntries]
		}
		ts.ByToken = entries
	}

	response := gin.H{
		"total_requests":   totalSuccess + totalError,
		"success_requests": totalSuccess,
		"error_requests":   totalError,
		"range":            params.Range,
		"duration_seconds": durationSeconds,
		"rpm_stats":        rpmStats,
		"is_today":         isToday,
		"by_type":          typeStats, // 按渠道类型分组的统计
	}

	RespondJSON(c, http.StatusOK, response)
}

// TypeSummary 按渠道类型的统计摘要
type TypeSummary struct {
	ChannelType              string               `json:"channel_type"`
	TotalRequests            int                  `json:"total_requests"`
	SuccessRequests          int                  `json:"success_requests"`
	ErrorRequests            int                  `json:"error_requests"`
	TotalInputTokens         int64                `json:"total_input_tokens,omitempty"`          // 所有类型
	TotalOutputTokens        int64                `json:"total_output_tokens,omitempty"`         // 所有类型
	TotalCacheReadTokens     int64                `json:"total_cache_read_tokens,omitempty"`     // Claude/Codex专用（prompt caching）
	TotalCacheCreationTokens int64                `json:"total_cache_creation_tokens,omitempty"` // Claude/Codex专用（prompt caching）
	TotalCost                float64              `json:"total_cost,omitempty"`                  // 标准成本
	EffectiveCost            *float64             `json:"effective_cost,omitempty"`              // 倍率后成本
	ByToken                  []AuthTokenCostEntry `json:"by_token,omitempty"`                    // 该类型下各 API 令牌的 effective_cost 占用（用于饼图）
	ByChannel                []ChannelCostEntry   `json:"by_channel,omitempty"`                  // 该类型下各渠道的 effective_cost 占用（用于饼图）
}

// AuthTokenCostEntry 单个 API 令牌的成本占用
type AuthTokenCostEntry struct {
	AuthTokenID int64   `json:"auth_token_id"`
	Name        string  `json:"name"`
	Cost        float64 `json:"cost"`
}

// ChannelCostEntry 单个渠道的成本占用
type ChannelCostEntry struct {
	ChannelID   int64   `json:"channel_id"`
	ChannelName string  `json:"channel_name"`
	Cost        float64 `json:"cost"`
}

// ChannelMeta 渠道元信息（名称+类型）
type ChannelMeta struct {
	Name string
	Type string
}

// fetchChannelMetaMap 查询所有渠道的元信息映射（id -> {name, type}）
func (s *Server) fetchChannelMetaMap(ctx context.Context) (map[int64]ChannelMeta, error) {
	configs, err := s.store.ListConfigs(ctx)
	if err != nil {
		return nil, err
	}

	m := make(map[int64]ChannelMeta, len(configs))
	for _, cfg := range configs {
		m[cfg.ID] = ChannelMeta{Name: cfg.Name, Type: cfg.ChannelType}
	}
	return m, nil
}

// getChannelMetaMapCached 带 TTL 缓存的渠道元信息映射查询
// [OPT] P3: 渠道配置变化频率极低，使用 60 秒缓存减少数据库查询
const channelMetaCacheTTL = 60 * time.Second

func (s *Server) getChannelMetaMapCached(ctx context.Context) (map[int64]ChannelMeta, error) {
	// 读锁检查缓存
	s.channelMetaCacheMu.RLock()
	if s.channelMetaCache != nil && time.Since(s.channelMetaCacheTime) < channelMetaCacheTTL {
		result := s.channelMetaCache
		s.channelMetaCacheMu.RUnlock()
		return result, nil
	}
	s.channelMetaCacheMu.RUnlock()

	// 写锁更新缓存
	s.channelMetaCacheMu.Lock()
	defer s.channelMetaCacheMu.Unlock()

	// 双重检查：可能其他 goroutine 已更新
	if s.channelMetaCache != nil && time.Since(s.channelMetaCacheTime) < channelMetaCacheTTL {
		return s.channelMetaCache, nil
	}

	channelMetas, err := s.fetchChannelMetaMap(ctx)
	if err != nil {
		return nil, err
	}

	s.channelMetaCache = channelMetas
	s.channelMetaCacheTime = time.Now()
	return channelMetas, nil
}

// HandleCooldownStats 获取当前冷却状态监控指标
// GET /admin/cooldown/stats
func (s *Server) HandleCooldownStats(c *gin.Context) {
	// 优先走缓存层，缓存不可用时自动降级到数据库查询
	channelCooldowns, _ := s.getAllChannelCooldowns(c.Request.Context())
	keyCooldowns, _ := s.getAllKeyCooldowns(c.Request.Context())

	var keyCount int
	for _, m := range keyCooldowns {
		keyCount += len(m)
	}

	response := gin.H{
		"channel_cooldowns": len(channelCooldowns),
		"key_cooldowns":     keyCount,
	}
	RespondJSON(c, http.StatusOK, response)
}

// HandleGetChannelTypes 获取渠道类型配置(公开端点,前端动态加载)
// GET /public/channel-types
// 编译时常量，浏览器缓存24小时减少HF Spaces等高延迟环境的网络往返
func (s *Server) HandleGetChannelTypes(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=86400")
	RespondJSON(c, http.StatusOK, util.ChannelTypes)
}

// HandlePublicVersion 获取当前版本信息(公开端点,前端显示版本)
// GET /public/version
// 版本信息变化频率极低（后台每4小时检查一次），缓存5分钟
func (s *Server) HandlePublicVersion(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=300")
	hasUpdate, latestVersion, releaseURL := version.GetUpdateInfo()
	RespondJSON(c, http.StatusOK, gin.H{
		"version":        version.Version,
		"has_update":     hasUpdate,
		"latest_version": latestVersion,
		"release_url":    releaseURL,
	})
}

// ModelsChannelsResponse 模型和渠道列表响应
type ModelsChannelsResponse struct {
	Models   []string              `json:"models"`
	Channels []model.ChannelNameID `json:"channels"`
}

// HandleGetModels 获取数据库中有日志的模型和渠道列表（去重）
// GET /admin/models
// 支持参数：range（时间范围）、channel_type（渠道类型筛选）
func (s *Server) HandleGetModels(c *gin.Context) {
	rangeParam := c.DefaultQuery("range", "this_month")
	params := ParsePaginationParams(c)
	params.Range = rangeParam
	since, until := params.GetTimeRange()

	channelType := c.Query("channel_type")
	logFilter := &model.LogFilter{LogSource: model.LogSourceProxy}

	models, err := s.store.GetDistinctModels(c.Request.Context(), since, until, channelType, logFilter)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	channels, err := s.store.GetDistinctChannels(c.Request.Context(), since, until, channelType, logFilter)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	RespondJSON(c, http.StatusOK, ModelsChannelsResponse{Models: models, Channels: channels})
}

// HandleHealth 健康检查端点(公开访问,无需认证)
// GET /health
// 仅检查数据库连接是否活跃（适用于K8s liveness/readiness probe）
func (s *Server) HandleHealth(c *gin.Context) {
	// 设置100ms超时，避免慢查询阻塞healthcheck
	ctx, cancel := context.WithTimeout(c.Request.Context(), 100*time.Millisecond)
	defer cancel()

	if err := s.store.Ping(ctx); err != nil {
		RespondError(c, http.StatusServiceUnavailable, err)
		return
	}

	RespondJSON(c, http.StatusOK, gin.H{"status": "ok"})
}

// fillHealthTimeline 为每个统计条目填充健康时间线
// isToday=true: 显示最近4小时，每5分钟一个状态（48个）
// isToday=false: 按总时间跨度/48计算时间桶
func (s *Server) fillHealthTimeline(ctx context.Context, stats []model.StatsEntry, startTime, endTime time.Time, filter *model.LogFilter, isToday bool) map[int][]model.HealthPoint {
	if len(stats) == 0 {
		return nil
	}

	const numBuckets = 48

	// 计算健康指示器的时间范围和桶大小
	var healthStart time.Time
	var bucketSeconds int64

	if isToday {
		// 当日：最近4小时，每5分钟一个桶
		bucketSeconds = 5 * 60 // 5分钟
		healthStart = endTime.Add(-4 * time.Hour)
		// 确保不早于查询开始时间
		if healthStart.Before(startTime) {
			healthStart = startTime
		}
	} else {
		// 其他时间范围：按总时长/48计算
		duration := endTime.Sub(startTime)
		bucketSeconds = int64(duration.Seconds() / numBuckets)
		if bucketSeconds < 1 {
			bucketSeconds = 1
		}
		healthStart = startTime
	}

	// 转换为毫秒，直接与 logs.time 比较，避免索引失效
	sinceMs := healthStart.UnixMilli()
	untilMs := endTime.UnixMilli()
	bucketMs := bucketSeconds * 1000

	// 构建结构化查询参数（SQL 构建已下沉到存储层）
	params := model.HealthTimelineParams{
		SinceMs:  sinceMs,
		UntilMs:  untilMs,
		BucketMs: bucketMs,
		Filter:   filter,
	}

	rows, err := s.store.GetHealthTimeline(ctx, params)
	if err != nil {
		// 静默失败，不影响主流程
		return nil
	}

	// 构建映射：(channel_id, model) -> StatsEntry索引
	type channelModelKey struct {
		channelID int
		model     string
	}
	statsMap := make(map[channelModelKey]int)
	for i := range stats {
		if stats[i].ChannelID != nil {
			key := channelModelKey{
				channelID: *stats[i].ChannelID,
				model:     stats[i].Model,
			}
			statsMap[key] = i
		}
	}

	// 解析查询结果 - 按时间桶索引位置填充
	timeline := make(map[channelModelKey][]model.HealthPoint)

	sinceUnix := healthStart.Unix()

	// 为每个渠道初始化48个空时间点
	for key := range statsMap {
		points := make([]model.HealthPoint, numBuckets)
		for i := 0; i < numBuckets; i++ {
			points[i] = model.HealthPoint{
				Ts:          time.Unix(sinceUnix+int64(i)*bucketSeconds, 0),
				SuccessRate: -1, // -1 表示无数据
			}
		}
		timeline[key] = points
	}

	for _, row := range rows {
		key := channelModelKey{channelID: row.ChannelID, model: row.Model}

		// 只处理 stats 中存在的组合
		if _, exists := statsMap[key]; !exists {
			continue
		}

		// 计算该时间桶对应的索引位置（BucketTs 是毫秒，需转换为秒再计算）
		bucketIndex := int((row.BucketTs/1000 - sinceUnix) / bucketSeconds)
		if bucketIndex < 0 || bucketIndex >= numBuckets {
			continue
		}

		total := row.Success + row.ErrorCount
		successRate := 0.0
		if total > 0 {
			successRate = float64(row.Success) / float64(total)
		}

		// 更新数据字段，保留初始化时的 Ts（Go 计算的桶起始时间）
		// 不能用 SQL 的 FLOOR 桶边界覆盖 Ts，否则同一桶索引在不同模型间
		// 产生不同时间戳，导致前端按 ts 合并时出现幽灵条目
		p := &timeline[key][bucketIndex]
		p.SuccessRate = successRate
		p.SuccessCount = row.Success
		p.ErrorCount = row.ErrorCount
		p.RateLimitedCount = row.RateLimitedCount
		p.AvgFirstByteTime = row.AvgFirstByteTime
		p.AvgDuration = row.AvgDuration
		p.TotalInputTokens = row.InputTokens
		p.TotalOutputTokens = row.OutputTokens
		p.TotalCacheReadTokens = row.CacheReadTokens
		p.TotalCacheCreationTokens = row.CacheCreationTokens
		p.TotalCost = row.TotalCost
		p.EffectiveCost = row.EffectiveCost
	}

	// 填充到 stats 中（per-model，供 stats 页面使用）
	for key, idx := range statsMap {
		if points, exists := timeline[key]; exists {
			stats[idx].HealthTimeline = points
		}
	}

	// 按渠道聚合健康时间线（供渠道管理页面使用）
	// 用桶索引合并，不依赖时间戳字符串，彻底避免前端 merge 的对齐问题
	channelHealth := make(map[int][]model.HealthPoint)
	for key, points := range timeline {
		ch, exists := channelHealth[key.channelID]
		if !exists {
			ch = make([]model.HealthPoint, numBuckets)
			for i := range ch {
				ch[i] = model.HealthPoint{
					Ts:          points[i].Ts,
					SuccessRate: -1,
				}
			}
			channelHealth[key.channelID] = ch
		}
		for i, pt := range points {
			if pt.SuccessRate < 0 {
				continue
			}
			if ch[i].SuccessRate < 0 {
				ch[i] = pt
				continue
			}
			// 加权合并平均值（用 SuccessCount 做权重，比前端用 total 更准确）
			oldSucc := ch[i].SuccessCount
			newSucc := pt.SuccessCount
			if totalSucc := oldSucc + newSucc; totalSucc > 0 {
				w := float64(totalSucc)
				ch[i].AvgFirstByteTime = (ch[i].AvgFirstByteTime*float64(oldSucc) + pt.AvgFirstByteTime*float64(newSucc)) / w
				ch[i].AvgDuration = (ch[i].AvgDuration*float64(oldSucc) + pt.AvgDuration*float64(newSucc)) / w
			}
			ch[i].SuccessCount += pt.SuccessCount
			ch[i].ErrorCount += pt.ErrorCount
			ch[i].RateLimitedCount += pt.RateLimitedCount
			if total := ch[i].SuccessCount + ch[i].ErrorCount; total > 0 {
				ch[i].SuccessRate = float64(ch[i].SuccessCount) / float64(total)
			}
			ch[i].TotalInputTokens += pt.TotalInputTokens
			ch[i].TotalOutputTokens += pt.TotalOutputTokens
			ch[i].TotalCacheReadTokens += pt.TotalCacheReadTokens
			ch[i].TotalCacheCreationTokens += pt.TotalCacheCreationTokens
			ch[i].TotalCost += pt.TotalCost
			ch[i].EffectiveCost += pt.EffectiveCost
		}
	}
	return channelHealth
}

// HandleStatsFilterOptions 返回统计页筛选下拉的全集（渠道名/模型），
// 从指定时间范围内的日志记录中提取，与表格数据解耦。
// GET /admin/stats/filter-options?range=today&channel_type=
func (s *Server) HandleStatsFilterOptions(c *gin.Context) {
	params := ParsePaginationParams(c)
	startTime, endTime := params.GetTimeRange()

	lf := BuildLogFilter(c)
	lf.LogSource = model.LogSourceProxy

	channelType := c.Query("channel_type")
	if channelType == "all" {
		channelType = ""
	}

	channels, err := s.store.GetDistinctChannels(c.Request.Context(), startTime, endTime, channelType, &lf)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	models, err := s.store.GetDistinctModels(c.Request.Context(), startTime, endTime, channelType, &lf)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	channelNames := make([]string, 0, len(channels))
	for _, ch := range channels {
		if ch.Name != "" {
			channelNames = append(channelNames, ch.Name)
		}
	}

	RespondJSON(c, http.StatusOK, gin.H{
		"channel_names": channelNames,
		"models":        models,
	})
}
