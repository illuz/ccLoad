package app

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"ccLoad/internal/model"

	"github.com/gin-gonic/gin"
)

func TestHandleChannelEditorAggregatesInitialState(t *testing.T) {
	server, store, cleanup := setupAdminTestServer(t)
	defer cleanup()

	ctx := context.Background()
	server.urlSelector = NewURLSelector()
	server.configService = NewConfigService(store)
	if err := server.configService.LoadDefaults(ctx); err != nil {
		t.Fatalf("加载系统设置失败: %v", err)
	}

	created, err := store.CreateConfig(ctx, &model.Config{
		Name:                "channel-editor-bootstrap",
		URL:                 "https://api.example.com",
		RequestDelaySeconds: 4,
		ModelEntries:        []model.ModelEntry{{Model: "external-model"}},
		Enabled:             true,
	})
	if err != nil {
		t.Fatalf("创建渠道失败: %v", err)
	}
	if err := store.CreateAPIKeysBatch(ctx, []*model.APIKey{{
		ChannelID: created.ID,
		KeyIndex:  0,
		APIKey:    "sk-editor-test",
		Note:      "primary",
	}}); err != nil {
		t.Fatalf("创建 API Key 失败: %v", err)
	}
	if err := store.AddLog(ctx, &model.LogEntry{
		Time:       model.JSONTime{Time: time.Now()},
		LogSource:  model.LogSourceProxy,
		ChannelID:  created.ID,
		Model:      "external-model",
		StatusCode: http.StatusOK,
		BaseURL:    created.URL,
	}); err != nil {
		t.Fatalf("写入模型统计日志失败: %v", err)
	}
	server.urlSelector.RecordLatency(created.ID, created.URL, 50*time.Millisecond)
	server.urlSelector.RecordRequestResult(created.ID, created.URL, http.StatusOK)

	path := "/admin/channels/" + strconv.FormatInt(created.ID, 10) + "/editor"
	c, w := newTestContext(t, newRequest(http.MethodGet, path, nil))
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(created.ID, 10)}}
	server.HandleChannelEditor(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	resp := mustParseAPIResponse[channelEditorData](t, w.Body.Bytes())
	data := resp.Data
	if data.Channel.ID != created.ID || data.Channel.Name != created.Name {
		t.Fatalf("channel=%+v, want id=%d name=%q", data.Channel, created.ID, created.Name)
	}
	if data.Channel.RequestDelaySeconds != 4 {
		t.Fatalf("request_delay_seconds=%d, want 4", data.Channel.RequestDelaySeconds)
	}
	if len(data.Keys) != 1 || data.Keys[0].APIKey != "sk-editor-test" {
		t.Fatalf("keys=%+v, want configured key", data.Keys)
	}
	if !data.ModelStats.Available || len(data.ModelStats.Items) != 1 {
		t.Fatalf("model_stats=%+v, want available stats", data.ModelStats)
	}
	if !data.URLStats.Available || len(data.URLStats.Items) != 1 || data.URLStats.Items[0].Requests != 1 {
		t.Fatalf("url_stats=%+v, want available runtime stats", data.URLStats)
	}
}

func TestHandleChannelEditorMarksOptionalStatsUnavailable(t *testing.T) {
	server, store, cleanup := setupAdminTestServer(t)
	defer cleanup()

	created, err := store.CreateConfig(context.Background(), &model.Config{
		Name:         "channel-editor-degraded",
		URL:          "https://api.example.com",
		ModelEntries: []model.ModelEntry{{Model: "external-model"}},
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("创建渠道失败: %v", err)
	}
	server.statsCache = nil
	server.urlSelector = nil

	path := "/admin/channels/" + strconv.FormatInt(created.ID, 10) + "/editor"
	c, w := newTestContext(t, newRequest(http.MethodGet, path, nil))
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(created.ID, 10)}}
	server.HandleChannelEditor(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	data := mustParseAPIResponse[channelEditorData](t, w.Body.Bytes()).Data
	if data.ModelStats.Available || data.ModelStats.Items == nil || len(data.ModelStats.Items) != 0 {
		t.Fatalf("model_stats=%+v, want unavailable empty stats", data.ModelStats)
	}
	if data.URLStats.Available || data.URLStats.Items == nil || len(data.URLStats.Items) != 0 {
		t.Fatalf("url_stats=%+v, want unavailable empty stats", data.URLStats)
	}
}
