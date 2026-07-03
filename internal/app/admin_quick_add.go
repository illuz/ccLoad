package app

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"ccLoad/internal/model"
	"ccLoad/internal/util"

	"github.com/gin-gonic/gin"
)

// quickAddResponse 快速添加渠道响应
type quickAddResponse struct {
	Channel      *model.Config         `json:"channel"`
	Group        *model.AuthTokenGroup `json:"group,omitempty"`
	ChannelGroup *model.ChannelGroup   `json:"channel_group,omitempty"`
}

// HandleQuickAddChannel 快速添加渠道:粘贴 URL + Key(s),复制模型/手填模型,可选追加到 auth token 分组。
// POST /admin/channels/quick-add
func (s *Server) HandleQuickAddChannel(c *gin.Context) {
	var req QuickAddChannelRequest
	if err := BindAndValidate(c, &req); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	// 解析模型来源:复制源渠道 或 手填模型
	var modelEntries []model.ModelEntry
	channelType := req.ChannelType
	protocolTransformMode := ""
	var protocolTransforms []string

	if req.ModelSourceChannelID != nil {
		src, err := s.store.GetConfig(ctx, *req.ModelSourceChannelID)
		if err != nil {
			RespondErrorMsg(c, http.StatusBadRequest, "model_source_channel_id not found")
			return
		}
		// 模型条目深拷贝(清掉 RedirectModel 相同的情况由 ToConfig 再处理)
		modelEntries = make([]model.ModelEntry, len(src.ModelEntries))
		for i, m := range src.ModelEntries {
			modelEntries[i] = m
		}
		// 类型默认继承源渠道
		if channelType == "" {
			channelType = src.ChannelType
		}
		protocolTransformMode = src.ProtocolTransformMode
		protocolTransforms = append([]string(nil), src.ProtocolTransforms...)
	} else {
		// 手填模型 -> ModelEntry(无 redirect,无 fixed_cost)
		modelEntries = make([]model.ModelEntry, 0, len(req.Models))
		for _, m := range req.Models {
			modelEntries = append(modelEntries, model.ModelEntry{Model: m})
		}
	}

	// channelType 仍为空 -> 快速添加默认 codex
	if channelType == "" {
		channelType = util.ChannelTypeCodex
	}

	channelGroupID := int64(0)
	if req.ChannelGroupID != nil {
		if *req.ChannelGroupID < 0 {
			RespondErrorMsg(c, http.StatusBadRequest, "channel_group_id must be >= 0")
			return
		}
		channelGroupID = *req.ChannelGroupID
	}

	// 优先级:nil 或负数 -> 默认 299
	priority := 299
	if req.Priority != nil && *req.Priority >= 0 {
		priority = *req.Priority
	}

	// 组装内部 ChannelRequest 并校验
	cr := ChannelRequest{
		Name:                  req.Name,
		APIKey:                strings.Join(req.APIKeys, ","),
		URL:                   req.URL,
		ChannelType:           channelType,
		ProtocolTransformMode: protocolTransformMode,
		ProtocolTransforms:    protocolTransforms,
		KeyStrategy:           model.KeyStrategySequential,
		Priority:              priority,
		GroupID:               channelGroupID,
		Models:                modelEntries,
		Enabled:               true,
		CostMultiplier:        1,
	}
	if err := cr.Validate(); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}

	created, err := s.createChannelFromRequest(ctx, cr)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	var channelGroup *model.ChannelGroup
	if channelGroupID > 0 {
		channelGroup, err = s.store.GetChannelGroup(ctx, channelGroupID)
		if err != nil {
			log.Printf("[WARN] quick-add: channel group %d not found after channel created (channel=%d)", channelGroupID, created.ID)
		}
	}

	// 可选:追加到 auth token 分组
	var group *model.AuthTokenGroup
	authTokenGroupID := req.AuthTokenGroupID
	if authTokenGroupID == nil {
		authTokenGroupID = req.GroupID
	}
	if authTokenGroupID != nil {
		group, err = s.store.GetAuthTokenGroup(ctx, *authTokenGroupID)
		if err != nil {
			log.Printf("[WARN] quick-add: auth token group %d not found after channel created (channel=%d)", *authTokenGroupID, created.ID)
			RespondJSON(c, http.StatusCreated, quickAddResponse{Channel: created, ChannelGroup: channelGroup})
			return
		}
		// 追加新渠道 ID(去重)
		alreadyExists := false
		for _, id := range group.AllowedChannelIDs {
			if id == created.ID {
				alreadyExists = true
				break
			}
		}
		if !alreadyExists {
			group.AllowedChannelIDs = append(group.AllowedChannelIDs, created.ID)
			if err := s.store.UpdateAuthTokenGroup(ctx, group); err != nil {
				log.Printf("[WARN] quick-add: failed to update auth token group %d (channel=%d): %v", *authTokenGroupID, created.ID, err)
				RespondJSON(c, http.StatusCreated, quickAddResponse{Channel: created, ChannelGroup: channelGroup})
				return
			}
			if err := s.authService.ReloadAuthTokens(); err != nil {
				log.Printf("[WARN] quick-add: ReloadAuthTokens failed: %v", err)
			}
		}
	}

	RespondJSON(c, http.StatusCreated, quickAddResponse{Channel: created, Group: group, ChannelGroup: channelGroup})
}
