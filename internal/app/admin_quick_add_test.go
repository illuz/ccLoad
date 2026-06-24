package app

import (
	"context"
	"net/http"
	"testing"

	"ccLoad/internal/model"
)

func TestHandleQuickAddChannel_ManualModels(t *testing.T) {
	server := newInMemoryServer(t)

	c, w := newTestContext(t, newJSONRequest(t, http.MethodPost, "/admin/channels/quick-add", map[string]any{
		"url":         "https://codex.hiyo.top",
		"api_keys":    []string{"sk-aaa", "sk-bbb", "sk-aaa"},
		"channel_type": "codex",
		"models":      []string{"gpt-5-codex", "gpt-5"},
	}))
	server.HandleQuickAddChannel(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, want %d, body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	resp := mustParseAPIResponse[quickAddResponse](t, w.Body.Bytes())
	if resp.Data.Channel == nil {
		t.Fatalf("channel is nil")
	}
	if resp.Data.Channel.ChannelType != "codex" {
		t.Fatalf("channel_type=%q, want codex", resp.Data.Channel.ChannelType)
	}
	if resp.Data.Channel.URL != "https://codex.hiyo.top" {
		t.Fatalf("url=%q, want https://codex.hiyo.top", resp.Data.Channel.URL)
	}
	if resp.Data.Channel.Name != "codex.hiyo.top" {
		t.Fatalf("name=%q, want codex.hiyo.top", resp.Data.Channel.Name)
	}
	if resp.Data.Channel.Priority != 299 {
		t.Fatalf("priority=%d, want 299 (default)", resp.Data.Channel.Priority)
	}
	if len(resp.Data.Channel.ModelEntries) != 2 {
		t.Fatalf("expected 2 models, got %d", len(resp.Data.Channel.ModelEntries))
	}

	// 验证 Key 实际写入了(去重后 2 个)
	keys, err := server.store.GetAPIKeys(context.Background(), resp.Data.Channel.ID)
	if err != nil {
		t.Fatalf("GetAPIKeys failed: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys (after dedup), got %d", len(keys))
	}
}

func TestHandleQuickAddChannel_CopyFromSourceChannel(t *testing.T) {
	server := newInMemoryServer(t)
	ctx := context.Background()

	// 先建一个源渠道,带模型 + 协议转换配置
	src, err := server.store.CreateConfig(ctx, &model.Config{
		Name:                  "src-codex",
		ChannelType:           "codex",
		URL:                   "https://upstream.codex.example",
		ProtocolTransformMode: "upstream",
		ProtocolTransforms:    []string{"anthropic"},
		ModelEntries: []model.ModelEntry{
			{Model: "gpt-5-codex"},
			{Model: "gpt-5"},
		},
		Enabled:        true,
		CostMultiplier: 1,
	})
	if err != nil {
		t.Fatalf("CreateConfig src failed: %v", err)
	}

	c, w := newTestContext(t, newJSONRequest(t, http.MethodPost, "/admin/channels/quick-add", map[string]any{
		"url":                     "https://new.codex.example",
		"api_keys":                []string{"sk-xxx"},
		"model_source_channel_id": src.ID,
	}))
	server.HandleQuickAddChannel(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, want %d, body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	resp := mustParseAPIResponse[quickAddResponse](t, w.Body.Bytes())
	ch := resp.Data.Channel
	if ch == nil {
		t.Fatalf("channel is nil")
	}
	if ch.ChannelType != "codex" {
		t.Fatalf("channel_type=%q, want codex (inherited from source)", ch.ChannelType)
	}
	if len(ch.ModelEntries) != 2 {
		t.Fatalf("expected 2 models copied from source, got %d", len(ch.ModelEntries))
	}
	if ch.ModelEntries[0].Model != "gpt-5-codex" {
		t.Fatalf("first model=%q, want gpt-5-codex", ch.ModelEntries[0].Model)
	}
	if ch.ProtocolTransformMode != "upstream" {
		t.Fatalf("protocol_transform_mode=%q, want upstream (copied from source)", ch.ProtocolTransformMode)
	}
}

func TestHandleQuickAddChannel_WithGroup(t *testing.T) {
	server := newInMemoryServer(t)
	ctx := context.Background()

	// 建一个 auth token 分组,里面已有 3 个渠道
	group := &model.AuthTokenGroup{
		Name:              "GPT",
		Color:             model.DefaultAuthTokenGroupColor,
		AllowedChannelIDs: []int64{101, 102, 103},
	}
	if err := server.store.CreateAuthTokenGroup(ctx, group); err != nil {
		t.Fatalf("CreateAuthTokenGroup failed: %v", err)
	}

	// 第一次 quick-add
	c, w := newTestContext(t, newJSONRequest(t, http.MethodPost, "/admin/channels/quick-add", map[string]any{
		"url":          "https://codex.hiyo.top",
		"api_keys":     []string{"sk-aaa"},
		"channel_type": "codex",
		"models":       []string{"gpt-5-codex"},
		"group_id":     group.ID,
	}))
	server.HandleQuickAddChannel(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("first quick-add: status=%d, body=%s", w.Code, w.Body.String())
	}
	firstNewID := mustParseAPIResponse[quickAddResponse](t, w.Body.Bytes()).Data.Channel.ID

	// 第二次 quick-add,模拟用户连续加两个渠道
	c2, w2 := newTestContext(t, newJSONRequest(t, http.MethodPost, "/admin/channels/quick-add", map[string]any{
		"url":          "https://codex2.hiyo.top",
		"api_keys":     []string{"sk-bbb"},
		"channel_type": "codex",
		"models":       []string{"gpt-5-codex"},
		"group_id":     group.ID,
	}))
	server.HandleQuickAddChannel(c2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("second quick-add: status=%d, body=%s", w2.Code, w2.Body.String())
	}
	secondNewID := mustParseAPIResponse[quickAddResponse](t, w2.Body.Bytes()).Data.Channel.ID

	// 重新从数据库读,确认所有 ID 都保留:原有 3 个 + 新加 2 个
	reloaded, err := server.store.GetAuthTokenGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("GetAuthTokenGroup reload failed: %v", err)
	}
	want := map[int64]bool{101: true, 102: true, 103: true, firstNewID: true, secondNewID: true}
	if len(reloaded.AllowedChannelIDs) != len(want) {
		t.Fatalf("AllowedChannelIDs=%v, want %d entries (existing 3 + new 2), got %d",
			reloaded.AllowedChannelIDs, len(want), len(reloaded.AllowedChannelIDs))
	}
	for id := range want {
		found := false
		for _, got := range reloaded.AllowedChannelIDs {
			if got == id {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("channel ID %d missing from group.AllowedChannelIDs=%v", id, reloaded.AllowedChannelIDs)
		}
	}
}

func TestHandleQuickAddChannel_ValidationErrors(t *testing.T) {
	server := newInMemoryServer(t)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"empty url", map[string]any{
			"api_keys": []string{"sk-aaa"},
			"models":   []string{"gpt-5"},
		}},
		{"empty keys", map[string]any{
			"url":      "https://x.example",
			"api_keys": []string{},
			"models":   []string{"gpt-5"},
		}},
		{"no model source and no models", map[string]any{
			"url":      "https://x.example",
			"api_keys": []string{"sk-aaa"},
		}},
		{"both model source and models", map[string]any{
			"url":                     "https://x.example",
			"api_keys":                []string{"sk-aaa"},
			"models":                  []string{"gpt-5"},
			"model_source_channel_id": 1,
		}},
		{"invalid channel_type", map[string]any{
			"url":          "https://x.example",
			"api_keys":     []string{"sk-aaa"},
			"channel_type": "xxx",
			"models":       []string{"gpt-5"},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, w := newTestContext(t, newJSONRequest(t, http.MethodPost, "/admin/channels/quick-add", tc.body))
			server.HandleQuickAddChannel(c)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status=%d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
			}
		})
	}
}
