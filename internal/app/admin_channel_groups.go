package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"ccLoad/internal/model"

	"github.com/gin-gonic/gin"
)

// HandleListChannelGroups 列出渠道分组。
// GET /admin/channel-groups
func (s *Server) HandleListChannelGroups(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	groups, err := s.store.ListChannelGroups(ctx)
	if err != nil {
		log.Print("[ERROR] 列出渠道分组失败: " + err.Error())
		RespondError(c, http.StatusInternalServerError, err)
		return
	}
	if groups == nil {
		groups = []*model.ChannelGroup{}
	}
	RespondJSON(c, http.StatusOK, gin.H{"groups": groups})
}

type channelGroupRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Color       *string `json:"color"`
}

func buildChannelGroupFromRequest(req channelGroupRequest, existing *model.ChannelGroup) (*model.ChannelGroup, error) {
	group := &model.ChannelGroup{}
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
	if req.Color != nil {
		color := model.NormalizeAuthTokenGroupColor(*req.Color)
		if color == "" {
			group.Color = model.DefaultAuthTokenGroupColor
		} else {
			if !model.IsSupportedAuthTokenGroupColor(color) {
				return nil, errors.New("color must be one of the preset values")
			}
			group.Color = color
		}
	}
	if group.Color == "" {
		group.Color = model.DefaultAuthTokenGroupColor
	}
	if strings.TrimSpace(group.Name) == "" {
		return nil, errors.New("name is required")
	}
	return group, nil
}

// HandleCreateChannelGroup 创建渠道分组。
// POST /admin/channel-groups
func (s *Server) HandleCreateChannelGroup(c *gin.Context) {
	var req channelGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == nil || strings.TrimSpace(*req.Name) == "" {
		RespondErrorMsg(c, http.StatusBadRequest, "name is required")
		return
	}
	group, err := buildChannelGroupFromRequest(req, nil)
	if err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := s.store.CreateChannelGroup(ctx, group); err != nil {
		log.Print("[ERROR] 创建渠道分组失败: " + err.Error())
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	RespondJSON(c, http.StatusOK, group)
}

// HandleUpdateChannelGroup 更新渠道分组。
// PUT /admin/channel-groups/:id
func (s *Server) HandleUpdateChannelGroup(c *gin.Context) {
	id, err := ParseInt64Param(c, "id")
	if err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid group id")
		return
	}

	var req channelGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		RespondErrorMsg(c, http.StatusBadRequest, "name is required")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	existing, err := s.store.GetChannelGroup(ctx, id)
	if err != nil {
		RespondErrorMsg(c, http.StatusNotFound, "channel group not found")
		return
	}
	group, err := buildChannelGroupFromRequest(req, existing)
	if err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	group.ID = id

	if err := s.store.UpdateChannelGroup(ctx, group); err != nil {
		log.Print("[ERROR] 更新渠道分组失败: " + err.Error())
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	RespondJSON(c, http.StatusOK, group)
}

// HandleDeleteChannelGroup 删除空渠道分组。
// DELETE /admin/channel-groups/:id
func (s *Server) HandleDeleteChannelGroup(c *gin.Context) {
	id, err := ParseInt64Param(c, "id")
	if err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid group id")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := s.store.DeleteChannelGroup(ctx, id); err != nil {
		if strings.Contains(err.Error(), "not empty") {
			RespondErrorMsg(c, http.StatusBadRequest, "channel group is not empty")
			return
		}
		log.Print("[ERROR] 删除渠道分组失败: " + err.Error())
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	RespondJSON(c, http.StatusOK, gin.H{"id": id})
}
