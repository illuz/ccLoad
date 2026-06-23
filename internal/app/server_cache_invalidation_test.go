package app

import (
	"testing"
	"time"
)

// TestInvalidateChannelListCache_ClearsChannelMetaCache 验证 P1-5：
// 渠道 CRUD 调用 InvalidateChannelListCache 时一并清 channelMetaCache，
// 避免 admin 改动后 60s TTL 内的脏读（read-after-write 一致性）。
func TestInvalidateChannelListCache_ClearsChannelMetaCache(t *testing.T) {
	t.Parallel()

	s := &Server{}
	// 预置缓存（模拟已缓存的渠道元信息映射）
	s.channelMetaCacheMu.Lock()
	s.channelMetaCache = map[int64]ChannelMeta{1: {Name: "a", Type: "anthropic"}, 2: {Name: "b", Type: "openai"}}
	s.channelMetaCacheTime = time.Now()
	s.channelMetaCacheMu.Unlock()

	// 空 Server 下 getChannelCache 返回 nil、channelBalancer 为 nil，调用安全
	s.InvalidateChannelListCache()

	s.channelMetaCacheMu.RLock()
	defer s.channelMetaCacheMu.RUnlock()
	if s.channelMetaCache != nil {
		t.Fatalf("InvalidateChannelListCache 后 channelMetaCache 应被清空, got %v", s.channelMetaCache)
	}
}
