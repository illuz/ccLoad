package app

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"ccLoad/internal/model"
	"ccLoad/internal/protocol"
)

const unsupportedProtocolCapabilityTTL = 10 * time.Minute

type protocolCapabilityKey struct {
	channelID      int64
	baseURL        string
	clientProtocol protocol.Protocol
	requestFamily  protocol.RequestFamily
}

type protocolCapabilityEntry struct {
	retryAfter time.Time
}

type protocolProbeRetrySummary struct {
	count   int
	retryAt time.Time
}

// protocolCapabilityCache only tracks temporarily unsupported explicitly
// configured upstream protocol surfaces. It does not discover protocols.
type protocolCapabilityCache struct {
	mu      sync.Mutex
	entries map[protocolCapabilityKey]protocolCapabilityEntry
}

func (c *protocolCapabilityCache) pending(key protocolCapabilityKey, now time.Time) (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return time.Time{}, false
	}
	if !entry.retryAfter.After(now) {
		delete(c.entries, key)
		return time.Time{}, false
	}
	return entry.retryAfter, true
}

func (c *protocolCapabilityCache) markUnsupported(key protocolCapabilityKey) {
	c.markUnsupportedAt(key, time.Now())
}

func (c *protocolCapabilityCache) markUnsupportedAt(key protocolCapabilityKey, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[protocolCapabilityKey]protocolCapabilityEntry)
	}
	for existingKey, entry := range c.entries {
		if !entry.retryAfter.After(now) {
			delete(c.entries, existingKey)
		}
	}
	c.entries[key] = protocolCapabilityEntry{retryAfter: now.Add(unsupportedProtocolCapabilityTTL)}
}

func (c *protocolCapabilityCache) clearKey(key protocolCapabilityKey) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// unsupportedRetrySummaries returns each channel's pending re-probe count and
// earliest retry. Expired entries are removed while building the snapshot.
func (c *protocolCapabilityCache) unsupportedRetrySummaries(now time.Time) map[int64]protocolProbeRetrySummary {
	c.mu.Lock()
	defer c.mu.Unlock()

	summaries := make(map[int64]protocolProbeRetrySummary)
	for key, entry := range c.entries {
		if !entry.retryAfter.After(now) {
			delete(c.entries, key)
			continue
		}
		summary := summaries[key.channelID]
		summary.count++
		if summary.retryAt.IsZero() || entry.retryAfter.Before(summary.retryAt) {
			summary.retryAt = entry.retryAfter
		}
		summaries[key.channelID] = summary
	}
	return summaries
}

func (c *protocolCapabilityCache) clear() {
	c.mu.Lock()
	clear(c.entries)
	c.mu.Unlock()
}

func protocolCapabilityKeyForRequest(cfg *model.Config, reqCtx *proxyRequestContext, baseURL string) (protocolCapabilityKey, bool) {
	if cfg == nil || reqCtx == nil || cfg.GetProtocolTransformMode() != model.ProtocolTransformModeUpstream {
		return protocolCapabilityKey{}, false
	}
	clientProtocol := reqCtx.clientProtocol
	baseProtocol := protocol.Protocol(strings.ToLower(strings.TrimSpace(cfg.GetChannelType())))
	if clientProtocol == "" || clientProtocol == baseProtocol || !cfg.SupportsProtocol(string(clientProtocol)) {
		return protocolCapabilityKey{}, false
	}
	requestFamily := protocol.DetectRequestFamily(reqCtx.requestPath)
	baseURL = strings.TrimSpace(baseURL)
	if requestFamily == protocol.RequestFamilyUnknown || baseURL == "" {
		return protocolCapabilityKey{}, false
	}
	return protocolCapabilityKey{
		channelID:      cfg.ID,
		baseURL:        baseURL,
		clientProtocol: clientProtocol,
		requestFamily:  requestFamily,
	}, true
}

func shouldCacheProtocolUnsupported(statusCode int, modelScoped bool) bool {
	if modelScoped {
		return false
	}
	return statusCode == http.StatusNotFound || statusCode == http.StatusMethodNotAllowed
}
