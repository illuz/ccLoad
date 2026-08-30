package app

import (
	"net/http"
	"testing"
	"time"

	"ccLoad/internal/model"
	"ccLoad/internal/protocol"
)

func TestProtocolCapabilityCachePendingSummariesAndExpiry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	first := protocolCapabilityKey{
		channelID:      7,
		baseURL:        "https://one.example.com",
		clientProtocol: protocol.OpenAI,
		requestFamily:  protocol.RequestFamilyChatCompletions,
	}
	second := first
	second.baseURL = "https://two.example.com"

	var cache protocolCapabilityCache
	cache.markUnsupportedAt(first, now)
	cache.markUnsupportedAt(second, now.Add(time.Minute))

	retryAt, pending := cache.pending(first, now.Add(time.Second))
	if !pending || !retryAt.Equal(now.Add(unsupportedProtocolCapabilityTTL)) {
		t.Fatalf("pending()=(%v,%v), want retry at %v", retryAt, pending, now.Add(unsupportedProtocolCapabilityTTL))
	}
	summary := cache.unsupportedRetrySummaries(now)[7]
	if summary.count != 2 || !summary.retryAt.Equal(now.Add(unsupportedProtocolCapabilityTTL)) {
		t.Fatalf("summary=%+v, want count 2 and earliest retry", summary)
	}

	if _, pending := cache.pending(first, now.Add(unsupportedProtocolCapabilityTTL)); pending {
		t.Fatal("entry remained pending at its retry deadline")
	}
	if summaries := cache.unsupportedRetrySummaries(now.Add(unsupportedProtocolCapabilityTTL + time.Minute)); len(summaries) != 0 {
		t.Fatalf("expired summaries were not removed: %+v", summaries)
	}
}

func TestProtocolCapabilityKeyOnlyTracksExplicitUpstreamTransforms(t *testing.T) {
	t.Parallel()

	reqCtx := &proxyRequestContext{
		clientProtocol: protocol.OpenAI,
		requestPath:    "/v1/chat/completions",
	}
	cfg := &model.Config{
		ID:                    9,
		ChannelType:           "anthropic",
		ProtocolTransformMode: model.ProtocolTransformModeUpstream,
		ProtocolTransforms:    []string{"openai"},
	}
	key, ok := protocolCapabilityKeyForRequest(cfg, reqCtx, "https://api.example.com")
	if !ok || key.channelID != 9 || key.clientProtocol != protocol.OpenAI || key.requestFamily != protocol.RequestFamilyChatCompletions {
		t.Fatalf("protocolCapabilityKeyForRequest()=(%+v,%v)", key, ok)
	}

	cfg.ProtocolTransformMode = model.ProtocolTransformModeLocal
	if _, ok := protocolCapabilityKeyForRequest(cfg, reqCtx, "https://api.example.com"); ok {
		t.Fatal("local translation must not create a protocol re-probe entry")
	}
	cfg.ProtocolTransformMode = model.ProtocolTransformModeUpstream
	cfg.ChannelType = "openai"
	if _, ok := protocolCapabilityKeyForRequest(cfg, reqCtx, "https://api.example.com"); ok {
		t.Fatal("native protocol must not create a protocol re-probe entry")
	}

	if !shouldCacheProtocolUnsupported(http.StatusNotFound, false) ||
		!shouldCacheProtocolUnsupported(http.StatusMethodNotAllowed, false) ||
		shouldCacheProtocolUnsupported(http.StatusNotFound, true) ||
		shouldCacheProtocolUnsupported(http.StatusBadRequest, false) {
		t.Fatal("stable endpoint failure classification is incorrect")
	}
}
