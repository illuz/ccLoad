package app

import (
	"encoding/base64"
	"net/http"
	"testing"

	"ccLoad/internal/model"

	"github.com/gin-gonic/gin"
)

func TestHandleGetDebugLog_NotFoundIncludesRelevantSettings(t *testing.T) {
	srv := newInMemoryServer(t)

	if err := srv.store.UpdateSetting(t.Context(), "debug_log_enabled", "false"); err != nil {
		t.Fatalf("update debug_log_enabled: %v", err)
	}
	if err := srv.store.UpdateSetting(t.Context(), "debug_log_retention_minutes", "15"); err != nil {
		t.Fatalf("update debug_log_retention_minutes: %v", err)
	}
	if err := srv.store.UpdateSetting(t.Context(), "debug_log_preserve_auth_token_id", "9"); err != nil {
		t.Fatalf("update debug_log_preserve_auth_token_id: %v", err)
	}

	c, w := newTestContext(t, newRequest(http.MethodGet, "/admin/debug-logs/123", nil))
	c.Params = gin.Params{{Key: "log_id", Value: "123"}}

	srv.HandleGetDebugLog(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusNotFound)
	}

	type unavailableData struct {
		Reason                   string               `json:"reason"`
		DebugLogEnabled          *model.SystemSetting `json:"debug_log_enabled"`
		DebugLogRetentionMinutes *model.SystemSetting `json:"debug_log_retention_minutes"`
		DebugLogPreserveToken    *model.SystemSetting `json:"debug_log_preserve_auth_token_id"`
	}

	resp := mustParseAPIResponse[unavailableData](t, w.Body.Bytes())
	if resp.Success {
		t.Fatalf("success=%v, want false", resp.Success)
	}
	if resp.Error != "debug log unavailable" {
		t.Fatalf("error=%q, want %q", resp.Error, "debug log unavailable")
	}
	if resp.Data.Reason != "debug_log_not_found" {
		t.Fatalf("reason=%q, want %q", resp.Data.Reason, "debug_log_not_found")
	}
	if resp.Data.DebugLogEnabled == nil {
		t.Fatal("debug_log_enabled should be returned")
	}
	if resp.Data.DebugLogEnabled.Key != "debug_log_enabled" || resp.Data.DebugLogEnabled.Value != "false" {
		t.Fatalf("debug_log_enabled=%+v, want key/value debug_log_enabled/false", resp.Data.DebugLogEnabled)
	}
	if resp.Data.DebugLogRetentionMinutes == nil {
		t.Fatal("debug_log_retention_minutes should be returned")
	}
	if resp.Data.DebugLogRetentionMinutes.Key != "debug_log_retention_minutes" || resp.Data.DebugLogRetentionMinutes.Value != "15" {
		t.Fatalf("debug_log_retention_minutes=%+v, want key/value debug_log_retention_minutes/15", resp.Data.DebugLogRetentionMinutes)
	}
	if resp.Data.DebugLogPreserveToken == nil || resp.Data.DebugLogPreserveToken.Value != "9" {
		t.Fatalf("debug_log_preserve_auth_token_id=%+v, want value 9", resp.Data.DebugLogPreserveToken)
	}
}

func TestHandleGetDebugLogReadsFileAndMasksHeaders(t *testing.T) {
	srv := newInMemoryServer(t)
	entry := &model.DebugLogEntry{
		LogID:       321,
		CreatedAt:   123456,
		ReqMethod:   http.MethodPost,
		ReqURL:      "https://api.example.com/v1/messages",
		ReqHeaders:  `{"Authorization":"Bearer top-secret","Content-Type":"application/json"}`,
		ReqBody:     []byte(`{"hello":"world"}`),
		RespStatus:  http.StatusOK,
		RespHeaders: `{"Set-Cookie":"session=secret","Content-Type":"application/octet-stream"}`,
		RespBody:    []byte{0xff, 0x00, 0x01},
	}
	if err := srv.debugLogs.Put(t.Context(), entry); err != nil {
		t.Fatalf("write debug log file: %v", err)
	}

	c, w := newTestContext(t, newRequest(http.MethodGet, "/admin/debug-logs/321", nil))
	c.Params = gin.Params{{Key: "log_id", Value: "321"}}
	srv.HandleGetDebugLog(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", w.Code, w.Body.String())
	}
	resp := mustParseAPIResponse[map[string]any](t, w.Body.Bytes())
	if !resp.Success {
		t.Fatalf("response=%+v", resp)
	}
	if got := resp.Data["req_body"]; got != `{"hello":"world"}` {
		t.Fatalf("req_body=%v", got)
	}
	if got := resp.Data["resp_body"]; got != base64.StdEncoding.EncodeToString(entry.RespBody) {
		t.Fatalf("resp_body=%v", got)
	}
	if got := resp.Data["resp_body_encoding"]; got != "base64" {
		t.Fatalf("resp_body_encoding=%v", got)
	}
	if got := resp.Data["req_headers"].(string); got == entry.ReqHeaders || got == "" {
		t.Fatalf("request headers were not masked: %s", got)
	}
	if got := resp.Data["resp_headers"].(string); got == entry.RespHeaders || got == "" {
		t.Fatalf("response headers were not masked: %s", got)
	}
}
