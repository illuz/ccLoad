package app

import (
	"context"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"ccLoad/internal/model"
	"ccLoad/internal/util"

	"github.com/gin-gonic/gin"
)

type billingGroupRequest struct {
	Name        string          `json:"name" binding:"required"`
	Slug        string          `json:"slug" binding:"required"`
	Description string          `json:"description"`
	Multiplier  *float64        `json:"multiplier"`
	Enabled     *bool           `json:"enabled"`
	ChannelIDs  []int64         `json:"channel_ids"`
	ChannelKeys map[int64][]int `json:"channel_keys"`
}

func (s *Server) HandleListBillingGroups(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	groups, err := s.store.ListBillingGroups(ctx)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, err)
		return
	}
	for _, group := range groups {
		billingGroupAvailability(s, ctx, group)
	}
	RespondJSON(c, http.StatusOK, gin.H{"groups": groups})
}

func (s *Server) HandleCreateBillingGroup(c *gin.Context) {
	var req billingGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	multiplier := 1.0
	if req.Multiplier != nil {
		multiplier = *req.Multiplier
	}
	channelIDs := req.ChannelIDs
	if req.ChannelKeys != nil {
		req.ChannelKeys = normalizeBillingGroupChannelKeys(req.ChannelKeys)
		channelIDs = billingGroupChannelIDsFromKeys(req.ChannelKeys)
	}
	group := &model.BillingGroup{Name: req.Name, Slug: req.Slug, Description: req.Description, Multiplier: multiplier, Enabled: enabled, ChannelIDs: channelIDs, ChannelKeys: req.ChannelKeys}
	if err := group.Validate(); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := s.store.CreateBillingGroup(ctx, group); err != nil {
		RespondError(c, http.StatusConflict, err)
		return
	}
	if err := s.store.SetBillingGroupChannels(ctx, group.ID, channelIDs); err != nil {
		RespondError(c, http.StatusInternalServerError, err)
		return
	}
	RespondJSON(c, http.StatusOK, group)
}

func (s *Server) HandleUpdateBillingGroup(c *gin.Context) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid billing group id")
		return
	}
	var req billingGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	group, err := s.store.GetBillingGroup(ctx, id)
	if err != nil {
		RespondErrorMsg(c, http.StatusNotFound, "billing group not found")
		return
	}
	group.Name, group.Slug, group.Description = req.Name, req.Slug, req.Description
	if req.ChannelKeys != nil {
		req.ChannelKeys = normalizeBillingGroupChannelKeys(req.ChannelKeys)
		group.ChannelIDs = billingGroupChannelIDsFromKeys(req.ChannelKeys)
		group.ChannelKeys = req.ChannelKeys
	} else {
		// Legacy channel_ids requests intentionally replace the old selection
		// and therefore clear any previously stored key-level restriction.
		group.ChannelKeys = nil
	}
	if req.Multiplier != nil {
		group.Multiplier = *req.Multiplier
	}
	if req.Enabled != nil {
		group.Enabled = *req.Enabled
	}
	if err := group.Validate(); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.UpdateBillingGroup(ctx, group); err != nil {
		RespondError(c, http.StatusInternalServerError, err)
		return
	}
	channelIDs := req.ChannelIDs
	if req.ChannelKeys != nil {
		req.ChannelKeys = normalizeBillingGroupChannelKeys(req.ChannelKeys)
		channelIDs = billingGroupChannelIDsFromKeys(req.ChannelKeys)
	}
	if err := s.store.SetBillingGroupChannels(ctx, id, channelIDs); err != nil {
		RespondError(c, http.StatusInternalServerError, err)
		return
	}
	group.ChannelIDs = channelIDs
	RespondJSON(c, http.StatusOK, group)
}

func billingGroupChannelIDsFromKeys(channelKeys map[int64][]int) []int64 {
	ids := make([]int64, 0, len(channelKeys))
	for channelID, keyIndexes := range channelKeys {
		if channelID <= 0 || len(keyIndexes) == 0 {
			continue
		}
		seen := make(map[int]struct{}, len(keyIndexes))
		valid := false
		for _, keyIndex := range keyIndexes {
			if keyIndex < 0 {
				continue
			}
			if _, ok := seen[keyIndex]; ok {
				continue
			}
			seen[keyIndex] = struct{}{}
			valid = true
		}
		if valid {
			ids = append(ids, channelID)
		}
	}
	slices.Sort(ids)
	return ids
}

func normalizeBillingGroupChannelKeys(channelKeys map[int64][]int) map[int64][]int {
	if channelKeys == nil {
		return nil
	}
	result := make(map[int64][]int, len(channelKeys))
	for channelID, indexes := range channelKeys {
		if channelID <= 0 {
			continue
		}
		seen := make(map[int]struct{}, len(indexes))
		for _, index := range indexes {
			if index < 0 {
				continue
			}
			if _, ok := seen[index]; ok {
				continue
			}
			seen[index] = struct{}{}
			result[channelID] = append(result[channelID], index)
		}
		if len(result[channelID]) > 1 {
			slices.Sort(result[channelID])
		}
	}
	return result
}

func (s *Server) HandleDeleteBillingGroup(c *gin.Context) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid billing group id")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := s.store.DeleteBillingGroup(ctx, id); err != nil {
		RespondError(c, http.StatusNotFound, err)
		return
	}
	RespondJSON(c, http.StatusOK, gin.H{"id": id})
}

func (s *Server) HandleAdjustAuthTokenBalance(c *gin.Context) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid token id")
		return
	}
	var req struct {
		AmountUSD float64 `json:"amount_usd"`
		Type      string  `json:"type"`
		Note      string  `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	delta := util.USDToMicroUSD(math.Abs(req.AmountUSD))
	if req.AmountUSD < 0 {
		delta = -delta
	}
	if delta == 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "amount_usd must be non-zero")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	entry, err := s.store.AdjustAuthTokenBalance(ctx, id, delta, req.Type, req.Note)
	if err != nil {
		RespondError(c, http.StatusBadRequest, err)
		return
	}
	RespondJSON(c, http.StatusOK, entry)
}

func (s *Server) HandleListAuthTokenBalanceTransactions(c *gin.Context) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid token id")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	entries, err := s.store.ListAuthTokenBalanceTransactions(ctx, id, 200, 0)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, err)
		return
	}
	RespondJSON(c, http.StatusOK, gin.H{"transactions": entries})
}
