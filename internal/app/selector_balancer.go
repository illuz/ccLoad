package app

import (
	"context"
	"math"
	"sort"
	"time"

	modelpkg "ccLoad/internal/model"
)

const (
	// effPriorityPrecision 有效优先级分组精度（*10可区分0.1差异，如5.0 vs 5.1）
	// 设计考虑：优先级通常是整数（5, 10），成功率惩罚基于统计（精度有限），0.1精度已足够
	effPriorityPrecision = 10
)

func effectiveBasePriority(ch *modelpkg.Config, inputTokens int) float64 {
	return float64(effectivePriorityForSort(ch, inputTokens))
}

func effectivePriorityForSort(ch *modelpkg.Config, inputTokens int) int {
	if ch == nil {
		return 0
	}
	priority := ch.Priority
	if inputTokens > 0 && ch.InputPriorityBonusEnabled && ch.InputPriorityThreshold > 0 && inputTokens > ch.InputPriorityThreshold {
		priority += ch.InputPriorityBonus
	}
	return priority
}

type inputTokensContextKey struct{}

func contextWithEstimatedInputTokens(ctx context.Context, tokens int) context.Context {
	if tokens <= 0 {
		return ctx
	}
	return context.WithValue(ctx, inputTokensContextKey{}, tokens)
}

func estimatedInputTokensFromContext(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	if v, ok := ctx.Value(inputTokensContextKey{}).(int); ok && v > 0 {
		return v
	}
	return 0
}

func effPriorityBucket(p float64) int64 {
	scaled := p * float64(effPriorityPrecision)
	// 浮点误差修正：避免 5.1*10 得到 50.999999... 被截断到 50
	if scaled >= 0 {
		scaled += 1e-9
	} else {
		scaled -= 1e-9
	}
	return int64(math.Trunc(scaled))
}

// channelWithScore 带有效优先级的渠道
type channelWithScore struct {
	config      *modelpkg.Config
	effPriority float64
}

// sortChannelsByHealth 按健康度排序渠道（仅排序，不改变冷却过滤语义）
// keyCooldowns: Key级冷却状态，用于计算有效Key数量（排除冷却中的Key）
// now: 当前时间，用于判断Key是否处于冷却中
func (s *Server) sortChannelsByHealth(
	channels []*modelpkg.Config,
	keyCooldowns map[int64]map[int]time.Time,
	now time.Time,
) []*modelpkg.Config {
	return s.sortChannelsByHealthWithInputTokens(channels, keyCooldowns, now, 0)
}

func (s *Server) sortChannelsByHealthWithInputTokens(
	channels []*modelpkg.Config,
	keyCooldowns map[int64]map[int]time.Time,
	now time.Time,
	inputTokens int,
) []*modelpkg.Config {
	if len(channels) == 0 {
		return channels
	}

	if s == nil || s.healthCache == nil {
		return s.balanceSamePriorityChannelsWithInputTokens(channels, keyCooldowns, now, inputTokens)
	}

	cfg := s.healthCache.Config()

	scored := make([]channelWithScore, len(channels))
	for i, ch := range channels {
		stats := s.healthCache.GetHealthStats(ch.ID)
		scored[i] = channelWithScore{
			config:      ch,
			effPriority: s.calculateEffectivePriorityWithInputTokens(ch, stats, cfg, inputTokens),
		}
	}

	// 按有效优先级排序（越大越优先，与原有逻辑一致）
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].effPriority > scored[j].effPriority
	})

	// 同有效优先级内按 KeyCount 平滑加权轮询（负载均衡）
	// 说明：healthCache 开启后仍需按 Key 数量分流。
	// 这里仅把“本轮选中的渠道”移动到组首，确保首选渠道按权重分布；其余顺序保持稳定，便于失败回退时可预测。
	result := make([]*modelpkg.Config, len(scored))
	groupStart := 0
	for i := 1; i <= len(scored); i++ {
		if i == len(scored) || effPriorityBucket(scored[i].effPriority) != effPriorityBucket(scored[groupStart].effPriority) {
			if i-groupStart > 1 {
				s.balanceScoredChannelsInPlace(scored[groupStart:i], keyCooldowns, now)
			}
			groupStart = i
		}
	}

	for i, item := range scored {
		result[i] = item.config
	}
	return result
}

// calculateEffectivePriority 计算渠道的有效优先级
// 有效优先级 = 基础优先级 - 成功率惩罚 × 置信度（越大越优先）
// 置信度 = min(1.0, 样本量 / 置信阈值)，样本量越小惩罚越轻
func (s *Server) calculateEffectivePriority(
	ch *modelpkg.Config,
	stats modelpkg.ChannelHealthStats,
	cfg modelpkg.HealthScoreConfig,
) float64 {
	return s.calculateEffectivePriorityWithInputTokens(ch, stats, cfg, 0)
}

func (s *Server) calculateEffectivePriorityWithInputTokens(
	ch *modelpkg.Config,
	stats modelpkg.ChannelHealthStats,
	cfg modelpkg.HealthScoreConfig,
	inputTokens int,
) float64 {
	basePriority := effectiveBasePriority(ch, inputTokens)

	successRate := stats.SuccessRate
	if successRate < 0 {
		successRate = 0
	} else if successRate > 1 {
		successRate = 1
	}
	failureRate := 1.0 - successRate

	// 置信度：样本量越小，惩罚打折越多
	confidence := 1.0
	if cfg.MinConfidentSample > 0 {
		confidence = min(1.0, float64(stats.SampleCount)/float64(cfg.MinConfidentSample))
	}

	// 惩罚 = 失败率 × 权重 × 置信度
	penalty := failureRate * float64(cfg.SuccessRatePenaltyWeight) * confidence

	return basePriority - penalty
}

// balanceSamePriorityChannels 按优先级分组，组内使用平滑加权轮询
// 用于 healthCache 关闭时的场景，确保确定性分流
func (s *Server) balanceSamePriorityChannels(
	channels []*modelpkg.Config,
	keyCooldowns map[int64]map[int]time.Time,
	now time.Time,
) []*modelpkg.Config {
	return s.balanceSamePriorityChannelsWithInputTokens(channels, keyCooldowns, now, 0)
}

func (s *Server) balanceSamePriorityChannelsWithInputTokens(
	channels []*modelpkg.Config,
	keyCooldowns map[int64]map[int]time.Time,
	now time.Time,
	inputTokens int,
) []*modelpkg.Config {
	n := len(channels)
	if n <= 1 {
		return channels
	}

	if s == nil || s.channelBalancer == nil {
		result := make([]*modelpkg.Config, n)
		copy(result, channels)
		sort.SliceStable(result, func(i, j int) bool {
			return effectivePriorityForSort(result[i], inputTokens) > effectivePriorityForSort(result[j], inputTokens)
		})
		return result
	}

	// 按优先级降序排序（优先级大的排前面），确保相同优先级渠道连续
	result := make([]*modelpkg.Config, n)
	copy(result, channels)
	sort.SliceStable(result, func(i, j int) bool {
		return effectivePriorityForSort(result[i], inputTokens) > effectivePriorityForSort(result[j], inputTokens)
	})

	// 按优先级分组，组内使用平滑加权轮询
	groupStart := 0
	for i := 1; i <= n; i++ {
		if i == n || effectivePriorityForSort(result[i], inputTokens) != effectivePriorityForSort(result[groupStart], inputTokens) {
			if i-groupStart > 1 {
				group := result[groupStart:i]
				balanced := s.channelBalancer.SelectWithCooldown(group, keyCooldowns, now)
				copy(result[groupStart:i], balanced)
			}
			groupStart = i
		}
	}

	return result
}

// balanceScoredChannelsInPlace 对带分数的渠道列表进行平滑加权轮询
// 用于 healthCache 开启时的同有效优先级组内负载均衡（仅决定组内“首选”渠道）
func (s *Server) balanceScoredChannelsInPlace(
	items []channelWithScore,
	keyCooldowns map[int64]map[int]time.Time,
	now time.Time,
) {
	n := len(items)
	if n <= 1 {
		return
	}

	if s == nil || s.channelBalancer == nil {
		return
	}

	// 提取 Config 列表用于轮询选择
	configs := make([]*modelpkg.Config, n)
	for i, item := range items {
		configs[i] = item.config
	}

	// 使用平滑加权轮询获取排序后的结果
	balanced := s.channelBalancer.SelectWithCooldown(configs, keyCooldowns, now)

	// 按轮询结果重排 items（O(n) 交换）
	// balanced[0] 是选中的渠道，需要把它移到 items[0]
	selectedID := balanced[0].ID
	for i, item := range items {
		if item.config.ID == selectedID && i != 0 {
			items[0], items[i] = items[i], items[0]
			break
		}
	}
}
