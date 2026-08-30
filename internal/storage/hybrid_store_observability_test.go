package storage

import (
	"errors"
	"testing"
)

// TestSyncToSQLite_CountsFailures 验证 P1-4：SQLite 缓存同步失败递增计数器，成功不计。
// 不依赖真实 DB——fn 直接注入成功/失败。
func TestSyncToSQLite_CountsFailures(t *testing.T) {
	t.Parallel()

	h := &HybridStore{}

	h.syncToSQLite("test_op", func() error { return errors.New("disk full") })
	if got := h.sqliteSyncFailCount.Load(); got != 1 {
		t.Fatalf("失败 1 次后 sqliteSyncFailCount=%d, want 1", got)
	}

	h.syncToSQLite("test_op", func() error { return errors.New("disk full") })
	if got := h.sqliteSyncFailCount.Load(); got != 2 {
		t.Fatalf("失败 2 次后 sqliteSyncFailCount=%d, want 2", got)
	}

	// 成功不计数
	h.syncToSQLite("test_op", func() error { return nil })
	if got := h.sqliteSyncFailCount.Load(); got != 2 {
		t.Fatalf("成功后 sqliteSyncFailCount=%d, want still 2", got)
	}
}

// TestEnqueueLogSync_CountsDrops 验证 P1-4：队列满时丢弃任务并递增计数器。
// 不启动 worker，手动填满 channel 以确定性触发丢弃路径。
func TestEnqueueLogSync_CountsDrops(t *testing.T) {
	t.Parallel()

	h := &HybridStore{
		syncCh: make(chan *syncTask, 2),
		stopCh: make(chan struct{}),
	}
	h.syncCh <- &syncTask{operation: "log"}
	h.syncCh <- &syncTask{operation: "log"} // 队列已满

	h.enqueueLogSync(&syncTask{operation: "log"})
	if got := h.syncQueueDropCount.Load(); got != 1 {
		t.Fatalf("队列满丢弃 1 次后 syncQueueDropCount=%d, want 1", got)
	}

	h.enqueueLogSync(&syncTask{operation: "log_batch"})
	if got := h.syncQueueDropCount.Load(); got != 2 {
		t.Fatalf("队列满丢弃 2 次后 syncQueueDropCount=%d, want 2", got)
	}

	// 队列有空位时不丢弃、不计数
	<-h.syncCh // 腾出 1 个空位
	h.enqueueLogSync(&syncTask{operation: "log"})
	if got := h.syncQueueDropCount.Load(); got != 2 {
		t.Fatalf("入队成功后 syncQueueDropCount=%d, want still 2", got)
	}
}

func TestHybridStoreRuntimeMetrics(t *testing.T) {
	t.Parallel()

	h := &HybridStore{syncCh: make(chan *syncTask, 3)}
	h.sqliteSyncFailCount.Store(2)
	h.mysqlSyncFailCount.Store(4)
	h.syncQueueDropCount.Store(5)
	h.mysqlSyncSuccessAt.Store(123456)
	h.syncCh <- &syncTask{operation: "log"}

	got := h.RuntimeMetrics()
	if got.SQLiteSyncFailures != 2 || got.MySQLSyncFailures != 4 || got.MySQLSyncDropped != 5 {
		t.Fatalf("failure counters=%+v", got)
	}
	if got.MySQLSyncPending != 1 || got.MySQLSyncQueueCapacity != 3 {
		t.Fatalf("queue metrics=%+v", got)
	}
	if got.MySQLSyncLastSuccess != 123456 {
		t.Fatalf("last success=%d, want 123456", got.MySQLSyncLastSuccess)
	}
}
