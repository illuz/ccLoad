package app

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"ccLoad/internal/model"

	"github.com/gin-gonic/gin"
)

func TestPublicKeyUsageRequiresAValidKey(t *testing.T) {
	server := newInMemoryServer(t)
	ctx := context.Background()
	key := "sk-public-usage-test"
	now := time.Now()
	token := &model.AuthToken{
		Token:                 model.HashToken(key),
		PlainToken:            key,
		Description:           "must not be returned",
		CreatedAt:             now.Add(-24 * time.Hour),
		IsActive:              true,
		SuccessCount:          7,
		FailureCount:          2,
		PromptTokensTotal:     700,
		CompletionTokensTotal: 300,
		TotalCostUSD:          1.25,
		EffectiveCostUSD:      1.5,
	}
	if err := server.store.CreateAuthToken(ctx, token); err != nil {
		t.Fatalf("CreateAuthToken failed: %v", err)
	}
	if err := server.store.BatchAddLogs(ctx, []*model.LogEntry{
		{
			Time:          model.JSONTime{Time: now},
			Model:         "gpt-test",
			ChannelID:     1,
			StatusCode:    http.StatusOK,
			IsStreaming:   true,
			FirstByteTime: 0.12,
			AuthTokenID:   token.ID,
			InputTokens:   12,
			OutputTokens:  34,
			Cost:          0.05,
		},
	}); err != nil {
		t.Fatalf("BatchAddLogs failed: %v", err)
	}

	router := gin.New()
	server.SetupRoutes(router)

	t.Run("valid key returns only usage data", func(t *testing.T) {
		w := serveHTTP(t, router, newRequest(http.MethodGet, "/public/key-usage?key="+key+"&range=today", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
		}
		if body := w.Body.String(); containsAny(body, key, "must not be returned") {
			t.Fatalf("public response leaked token metadata: %s", body)
		}

		var response struct {
			Success bool `json:"success"`
			Data    struct {
				Range   string                    `json:"range"`
				History model.AuthTokenRangeStats `json:"history"`
				Today   model.AuthTokenRangeStats `json:"today"`
				Total   model.AuthTokenRangeStats `json:"total"`
			} `json:"data"`
		}
		mustUnmarshalJSON(t, w.Body.Bytes(), &response)
		if !response.Success || response.Data.Range != "today" {
			t.Fatalf("unexpected response: %+v", response)
		}
		if response.Data.History.SuccessCount != 1 || response.Data.Today.PromptTokens != 12 {
			t.Fatalf("unexpected today stats: %+v / %+v", response.Data.History, response.Data.Today)
		}
		if response.Data.Total.SuccessCount != 7 || response.Data.Total.EffectiveCost != 1.5 {
			t.Fatalf("unexpected total stats: %+v", response.Data.Total)
		}
	})

	for _, target := range []string{
		"/public/key-usage",
		"/public/key-usage?key=unknown",
	} {
		t.Run(target, func(t *testing.T) {
			w := serveHTTP(t, router, newRequest(http.MethodGet, target, nil))
			if w.Code != http.StatusNotFound {
				t.Fatalf("status=%d, want %d, body=%s", w.Code, http.StatusNotFound, w.Body.String())
			}
		})
	}
}

func TestPublicKeyUsagePageRequiresAValidKey(t *testing.T) {
	originalFS := embedFS
	defer func() { embedFS = originalFS }()
	SetEmbedFS(os.DirFS("../.."), "web")

	server := newInMemoryServer(t)
	key := "sk-public-page-test"
	if err := server.store.CreateAuthToken(context.Background(), &model.AuthToken{
		Token:      model.HashToken(key),
		PlainToken: key,
		IsActive:   true,
	}); err != nil {
		t.Fatalf("CreateAuthToken failed: %v", err)
	}

	router := gin.New()
	server.SetupRoutes(router)

	w := serveHTTP(t, router, newRequest(http.MethodGet, "/key-usage?key="+key, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type=%q, want html", got)
	}

	for _, target := range []string{
		"/key-usage",
		"/key-usage?key=unknown",
		"/web/key-usage.html?key=" + key,
	} {
		w := serveHTTP(t, router, newRequest(http.MethodGet, target, nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d, want %d", target, w.Code, http.StatusNotFound)
		}
	}
}

func containsAny(value string, values ...string) bool {
	for _, candidate := range values {
		if candidate != "" && strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
