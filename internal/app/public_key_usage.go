package app

import (
	"context"
	"net/http"
	"strings"
	"time"

	"ccLoad/internal/model"

	"github.com/gin-gonic/gin"
)

// PublicKeyUsageResponse contains only usage figures. It intentionally omits
// the token value, hash, description, and quota configuration.
type PublicKeyUsageResponse struct {
	Range            string                     `json:"range"`
	RangeStart       int64                      `json:"range_start"`
	RangeEnd         int64                      `json:"range_end"`
	History          *model.AuthTokenRangeStats `json:"history"`
	Today            *model.AuthTokenRangeStats `json:"today"`
	Total            model.AuthTokenRangeStats  `json:"total"`
	UpdatedAt        int64                      `json:"updated_at"`
	HistoryIsCurrent bool                       `json:"history_is_current"`
}

// HandlePublicKeyUsage returns the requested key's historical range, live
// statistics for today, and cumulative totals.
// GET /public/key-usage?key=...&range=today|yesterday|this_week|this_month|custom
func (s *Server) HandlePublicKeyUsage(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	token, ok := s.findPublicUsageToken(ctx, c.Query("key"))
	if !ok {
		respondPublicUsageNotFound(c)
		return
	}

	params := ParsePaginationParams(c)
	now := time.Now()
	todayStart := beginningOfDay(now)
	historyStart, historyEnd := params.GetTimeRangeAt(now)
	total := publicUsageTotal(token)

	var (
		history *model.AuthTokenRangeStats
		err     error
	)
	if params.Range == "all" {
		history = &total
		historyStart = token.CreatedAt
		historyEnd = now
	} else {
		history, err = s.store.GetAuthTokenStatsByIDInRange(ctx, token.ID, historyStart, historyEnd, params.Range == "today")
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err)
			return
		}
	}

	today := history
	if !sameTimeRange(historyStart, historyEnd, todayStart, now) || params.Range == "all" {
		today, err = s.store.GetAuthTokenStatsByIDInRange(ctx, token.ID, todayStart, now, true)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err)
			return
		}
	}

	RespondJSON(c, http.StatusOK, PublicKeyUsageResponse{
		Range:            params.Range,
		RangeStart:       historyStart.UnixMilli(),
		RangeEnd:         historyEnd.UnixMilli(),
		History:          history,
		Today:            today,
		Total:            total,
		UpdatedAt:        now.UnixMilli(),
		HistoryIsCurrent: params.Range == "today",
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

func publicUsageTotal(token *model.AuthToken) model.AuthTokenRangeStats {
	if token == nil {
		return model.AuthTokenRangeStats{}
	}
	return model.AuthTokenRangeStats{
		SuccessCount:        token.SuccessCount,
		FailureCount:        token.FailureCount,
		PromptTokens:        token.PromptTokensTotal,
		CompletionTokens:    token.CompletionTokensTotal,
		CacheReadTokens:     token.CacheReadTokensTotal,
		CacheCreationTokens: token.CacheCreationTokensTotal,
		TotalCost:           token.TotalCostUSD,
		EffectiveCost:       token.EffectiveCostUSD,
		StreamAvgTTFB:       token.StreamAvgTTFB,
		NonStreamAvgRT:      token.NonStreamAvgRT,
		StreamCount:         token.StreamCount,
		NonStreamCount:      token.NonStreamCount,
	}
}

func sameTimeRange(firstStart, firstEnd, secondStart, secondEnd time.Time) bool {
	return firstStart.Equal(secondStart) && firstEnd.Equal(secondEnd)
}
