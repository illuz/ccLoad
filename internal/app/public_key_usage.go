package app

import (
	"context"
	"net/http"
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

// PublicKeyUsageResponse intentionally omits token metadata and channel data.
type PublicKeyUsageResponse struct {
	TodayStart int64                      `json:"today_start"`
	Today      PublicKeyTodayUsage        `json:"today"`
	CostQuota  PublicKeyCostQuota         `json:"cost_quota"`
	Trend      []PublicKeyUsageTrendPoint `json:"trend"`
	UpdatedAt  int64                      `json:"updated_at"`
}

// HandlePublicKeyUsage returns only today's live totals and half-hour trend.
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
	authTokenID := token.ID
	points, err := s.store.AggregateRangeWithFilter(ctx, todayStart, now, publicKeyUsageBucket, &model.LogFilter{
		AuthTokenID: &authTokenID,
	})
	if err != nil {
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	today, trend := buildPublicKeyTodayUsage(points)
	costQuota := s.buildPublicKeyCostQuota(ctx, token)

	RespondJSON(c, http.StatusOK, PublicKeyUsageResponse{
		TodayStart: todayStart.UnixMilli(),
		Today:      today,
		CostQuota:  costQuota,
		Trend:      trend,
		UpdatedAt:  now.UnixMilli(),
	})
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

func (s *Server) findPublicUsageToken(ctx context.Context, key string) (*model.AuthToken, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, false
	}

	token, err := s.store.GetAuthTokenByValue(ctx, model.HashToken(key))
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

func (s *Server) buildPublicKeyCostQuota(ctx context.Context, token *model.AuthToken) PublicKeyCostQuota {
	if token == nil {
		return PublicKeyCostQuota{}
	}
	token.NormalizeDailyCostForToday()

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
