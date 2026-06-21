package app

import (
	"context"
	"net/http"
	"testing"

	"ccLoad/internal/model"

	"github.com/gin-gonic/gin"
)

func TestAdminAPI_CreateAuthTokenGroup_WithColor(t *testing.T) {
	server := newInMemoryServer(t)

	c, w := newTestContext(t, newJSONRequest(t, http.MethodPost, "/admin/auth-token-groups", map[string]any{
		"name":  "Premium",
		"color": "#3b82f6",
	}))

	server.HandleCreateAuthTokenGroup(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	resp := mustParseAPIResponse[model.AuthTokenGroup](t, w.Body.Bytes())
	if resp.Data.Color != "#3b82f6" {
		t.Fatalf("color=%q, want %q", resp.Data.Color, "#3b82f6")
	}
}

func TestAdminAPI_UpdateAuthTokenGroup_InvalidColor(t *testing.T) {
	server := newInMemoryServer(t)
	ctx := context.Background()

	group := &model.AuthTokenGroup{
		Name:  "Premium",
		Color: model.DefaultAuthTokenGroupColor,
	}
	if err := server.store.CreateAuthTokenGroup(ctx, group); err != nil {
		t.Fatalf("CreateAuthTokenGroup failed: %v", err)
	}

	c, w := newTestContext(t, newJSONRequest(t, http.MethodPut, "/admin/auth-token-groups/1", map[string]any{
		"color": "#123123",
	}))
	c.Params = gin.Params{{Key: "id", Value: "1"}}

	server.HandleUpdateAuthTokenGroup(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}
