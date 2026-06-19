package app

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"ccLoad/internal/model"

	"github.com/gin-gonic/gin"
)

// HandleListAuthTokenGroups 列出 API 令牌分组。
// GET /admin/auth-token-groups
func (s *Server) HandleListAuthTokenGroups(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	groups, err := s.store.ListAuthTokenGroups(ctx)
	if err != nil {
		log.Print("[ERROR] 列出令牌分组失败: " + err.Error())
		RespondError(c, http.StatusInternalServerError, err)
		return
	}
	if groups == nil {
		groups = []*model.AuthTokenGroup{}
	}
	RespondJSON(c, http.StatusOK, gin.H{"groups": groups})
}

type authTokenGroupRequest struct {
	Name              *string  `json:"name"`
	Description       *string  `json:"description"`
	AllowedModels     []string `json:"allowed_models"`
	AllowedChannelIDs []int64  `json:"allowed_channel_ids"`
	CostLimitUSD      *float64 `json:"cost_limit_usd"`
	MaxConcurrency    *int     `json:"max_concurrency"`
}

func buildAuthTokenGroupFromRequest(req authTokenGroupRequest, existing *model.AuthTokenGroup) (*model.AuthTokenGroup, error) {
	group := &model.AuthTokenGroup{}
	if existing != nil {
		clone := *existing
		group = &clone
	}
	if req.Name != nil {
		group.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		group.Description = strings.TrimSpace(*req.Description)
	}
	if req.AllowedModels != nil {
		group.AllowedModels = req.AllowedModels
	}
	if req.AllowedChannelIDs != nil {
		group.AllowedChannelIDs = req.AllowedChannelIDs
	}
	if req.CostLimitUSD != nil {
		group.SetCostLimitUSD(*req.CostLimitUSD)
	}
	if req.MaxConcurrency != nil {
		group.MaxConcurrency = *req.MaxConcurrency
	}
	return group, group.ValidateUsageLimits()
}

// HandleCreateAuthTokenGroup 创建 API 令牌分组。
// POST /admin/auth-token-groups
func (s *Server) HandleCreateAuthTokenGroup(c *gin.Context) {
	var req authTokenGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == nil || strings.TrimSpace(*req.Name) == "" {
		RespondErrorMsg(c, http.StatusBadRequest, "name is required")
		return
	}
	if req.CostLimitUSD != nil && *req.CostLimitUSD < 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "cost_limit_usd must be >= 0")
		return
	}
	if req.MaxConcurrency != nil && *req.MaxConcurrency < 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "max_concurrency must be >= 0")
		return
	}

	group, err := buildAuthTokenGroupFromRequest(req, nil)
	if err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := s.store.CreateAuthTokenGroup(ctx, group); err != nil {
		log.Print("[ERROR] 创建令牌分组失败: " + err.Error())
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	if err := s.authService.ReloadAuthTokens(); err != nil {
		log.Print("[WARN]  热更新失败: " + err.Error())
	}

	RespondJSON(c, http.StatusOK, group)
}

// HandleUpdateAuthTokenGroup 更新 API 令牌分组。
// PUT /admin/auth-token-groups/:id
func (s *Server) HandleUpdateAuthTokenGroup(c *gin.Context) {
	id, err := ParseInt64Param(c, "id")
	if err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid group id")
		return
	}

	var req authTokenGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		RespondErrorMsg(c, http.StatusBadRequest, "name is required")
		return
	}
	if req.CostLimitUSD != nil && *req.CostLimitUSD < 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "cost_limit_usd must be >= 0")
		return
	}
	if req.MaxConcurrency != nil && *req.MaxConcurrency < 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "max_concurrency must be >= 0")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	existing, err := s.store.GetAuthTokenGroup(ctx, id)
	if err != nil {
		RespondErrorMsg(c, http.StatusNotFound, "auth token group not found")
		return
	}
	group, err := buildAuthTokenGroupFromRequest(req, existing)
	if err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	group.ID = id

	if err := s.store.UpdateAuthTokenGroup(ctx, group); err != nil {
		log.Print("[ERROR] 更新令牌分组失败: " + err.Error())
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	if err := s.authService.ReloadAuthTokens(); err != nil {
		log.Print("[WARN]  热更新失败: " + err.Error())
	}

	RespondJSON(c, http.StatusOK, group)
}

// HandleDeleteAuthTokenGroup 删除空 API 令牌分组。
// DELETE /admin/auth-token-groups/:id
func (s *Server) HandleDeleteAuthTokenGroup(c *gin.Context) {
	id, err := ParseInt64Param(c, "id")
	if err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid group id")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := s.store.DeleteAuthTokenGroup(ctx, id); err != nil {
		if strings.Contains(err.Error(), "not empty") {
			RespondErrorMsg(c, http.StatusBadRequest, "auth token group is not empty")
			return
		}
		log.Print("[ERROR] 删除令牌分组失败: " + err.Error())
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	if err := s.authService.ReloadAuthTokens(); err != nil {
		log.Print("[WARN]  热更新失败: " + err.Error())
	}

	RespondJSON(c, http.StatusOK, gin.H{"id": id})
}
