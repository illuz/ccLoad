package app

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"ccLoad/internal/model"

	"github.com/gin-gonic/gin"
)

func TestAdminAPI_ChannelGroupsCRUD(t *testing.T) {
	server := newInMemoryServer(t)

	c, w := newTestContext(t, newJSONRequest(t, http.MethodPost, "/admin/channel-groups", map[string]any{
		"name":        "Premium",
		"description": "paid channels",
		"color":       "#3b82f6",
	}))
	server.HandleCreateChannelGroup(c)
	if w.Code != http.StatusOK {
		t.Fatalf("create status=%d, body=%s", w.Code, w.Body.String())
	}

	created := mustParseAPIResponse[model.ChannelGroup](t, w.Body.Bytes()).Data
	if created.ID <= 0 {
		t.Fatalf("expected created group id > 0")
	}
	if created.Color != "#3b82f6" {
		t.Fatalf("color=%q, want #3b82f6", created.Color)
	}

	c, w = newTestContext(t, newRequest(http.MethodGet, "/admin/channel-groups", nil))
	server.HandleListChannelGroups(c)
	if w.Code != http.StatusOK {
		t.Fatalf("list status=%d, body=%s", w.Code, w.Body.String())
	}
	resp := mustParseAPIResponse[struct {
		Groups []*model.ChannelGroup `json:"groups"`
	}](t, w.Body.Bytes())
	if len(resp.Data.Groups) != 1 {
		t.Fatalf("group count=%d, want 1", len(resp.Data.Groups))
	}

	c, w = newTestContext(t, newJSONRequest(t, http.MethodPut, fmt.Sprintf("/admin/channel-groups/%d", created.ID), map[string]any{
		"name":        "Premium+",
		"description": "updated",
		"color":       "#22c55e",
	}))
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", created.ID)}}
	server.HandleUpdateChannelGroup(c)
	if w.Code != http.StatusOK {
		t.Fatalf("update status=%d, body=%s", w.Code, w.Body.String())
	}
	updated := mustParseAPIResponse[model.ChannelGroup](t, w.Body.Bytes()).Data
	if updated.Name != "Premium+" || updated.Color != "#22c55e" {
		t.Fatalf("updated group=%+v", updated)
	}
}

func TestAdminAPI_ChannelGroupDeleteRejectsNonEmptyAndBatchMove(t *testing.T) {
	server := newInMemoryServer(t)
	ctx := context.Background()

	groupA := &model.ChannelGroup{Name: "A", Color: model.DefaultAuthTokenGroupColor}
	groupB := &model.ChannelGroup{Name: "B", Color: "#22c55e"}
	if err := server.store.CreateChannelGroup(ctx, groupA); err != nil {
		t.Fatalf("create group A: %v", err)
	}
	if err := server.store.CreateChannelGroup(ctx, groupB); err != nil {
		t.Fatalf("create group B: %v", err)
	}

	created, err := server.store.CreateConfig(ctx, (&ChannelRequest{
		Name:            "channel-a",
		APIKey:          "sk-a",
		URL:             "https://api.example.com",
		ChannelType:     "openai",
		ProtocolTransforms: []string{},
		ProtocolTransformMode: model.ProtocolTransformModeLocal,
		KeyStrategy:     model.KeyStrategySequential,
		GroupID:         groupA.ID,
		Models:          []model.ModelEntry{{Model: "gpt-4.1"}},
		Enabled:         true,
		CostMultiplier:  1,
	}).ToConfig())
	if err != nil {
		t.Fatalf("create config: %v", err)
	}

	c, w := newTestContext(t, newRequest(http.MethodDelete, fmt.Sprintf("/admin/channel-groups/%d", groupA.ID), nil))
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", groupA.ID)}}
	server.HandleDeleteChannelGroup(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("delete non-empty status=%d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}

	c, w = newTestContext(t, newJSONRequest(t, http.MethodPost, "/admin/channels/batch-group", map[string]any{
		"channel_ids": []int64{created.ID},
		"group_id":    groupB.ID,
	}))
	server.HandleBatchUpdateChannelGroup(c)
	if w.Code != http.StatusOK {
		t.Fatalf("batch-group status=%d, body=%s", w.Code, w.Body.String())
	}

	got, err := server.store.GetConfig(ctx, created.ID)
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if got.GroupID != groupB.ID {
		t.Fatalf("group_id=%d, want %d", got.GroupID, groupB.ID)
	}

	c, w = newTestContext(t, newJSONRequest(t, http.MethodPost, "/admin/channels/batch-group", map[string]any{
		"channel_ids": []int64{created.ID},
		"group_id":    int64(0),
	}))
	server.HandleBatchUpdateChannelGroup(c)
	if w.Code != http.StatusOK {
		t.Fatalf("clear batch-group status=%d, body=%s", w.Code, w.Body.String())
	}
	got, err = server.store.GetConfig(ctx, created.ID)
	if err != nil {
		t.Fatalf("get config after clear: %v", err)
	}
	if got.GroupID != 0 {
		t.Fatalf("group_id after clear=%d, want 0", got.GroupID)
	}
}

func TestHandleQuickAddChannel_WithChannelGroup(t *testing.T) {
	server := newInMemoryServer(t)
	ctx := context.Background()

	channelGroup := &model.ChannelGroup{
		Name:  "Fast Lane",
		Color: "#8b5cf6",
	}
	if err := server.store.CreateChannelGroup(ctx, channelGroup); err != nil {
		t.Fatalf("CreateChannelGroup failed: %v", err)
	}

	c, w := newTestContext(t, newJSONRequest(t, http.MethodPost, "/admin/channels/quick-add", map[string]any{
		"url":              "https://grouped.example.com",
		"api_keys":         []string{"sk-quick"},
		"channel_type":     "codex",
		"models":           []string{"gpt-5-codex"},
		"channel_group_id": channelGroup.ID,
	}))
	server.HandleQuickAddChannel(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, want %d, body=%s", w.Code, http.StatusCreated, w.Body.String())
	}

	resp := mustParseAPIResponse[quickAddResponse](t, w.Body.Bytes())
	if resp.Data.Channel == nil {
		t.Fatalf("channel is nil")
	}
	if resp.Data.Channel.GroupID != channelGroup.ID {
		t.Fatalf("channel.group_id=%d, want %d", resp.Data.Channel.GroupID, channelGroup.ID)
	}
	if resp.Data.ChannelGroup == nil {
		t.Fatalf("channel_group is nil")
	}
	if resp.Data.ChannelGroup.ID != channelGroup.ID {
		t.Fatalf("channel_group.id=%d, want %d", resp.Data.ChannelGroup.ID, channelGroup.ID)
	}
}
