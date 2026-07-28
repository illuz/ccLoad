package debuglog

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ccLoad/internal/model"
)

func TestFileStoreRoundTrip(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := NewFileStore(root)
	tokenKey := "sk-DirectKey_123"
	entry := &model.DebugLogEntry{
		LogID:        42,
		AuthTokenID:  7,
		AuthTokenKey: tokenKey,
		CreatedAt:    time.Now().Unix(),
		ReqMethod:    "POST",
		ReqURL:       "https://api.example.com/v1/messages",
		ReqHeaders:   `{"Authorization":"Bearer secret"}`,
		ReqBody:      []byte{'{', 0xff, '}'},
		RespStatus:   200,
		RespHeaders:  `{"Content-Type":"application/octet-stream"}`,
		RespBody:     []byte{0x00, 0x01, 0xfe, 0xff},
	}
	if err := store.Put(context.Background(), entry); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := store.Get(context.Background(), entry.LogID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.LogID != entry.LogID || got.ReqURL != entry.ReqURL || got.RespStatus != entry.RespStatus {
		t.Fatalf("metadata mismatch: got=%+v want=%+v", got, entry)
	}
	if !bytes.Equal(got.ReqBody, entry.ReqBody) || !bytes.Equal(got.RespBody, entry.RespBody) {
		t.Fatalf("body mismatch: got req=%v resp=%v", got.ReqBody, got.RespBody)
	}

	entryDir := filepath.Join(root, tokenKey, "42")
	meta, err := os.ReadFile(filepath.Join(entryDir, metadataFileName))
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if len(meta) < 2 || meta[0] != 0x1f || meta[1] != 0x8b {
		t.Fatalf("metadata is not gzip data: %x", meta[:min(4, len(meta))])
	}
	for _, name := range []string{metadataFileName, requestFileName, responseFileName} {
		info, err := os.Stat(filepath.Join(entryDir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode=%o, want 600", name, info.Mode().Perm())
		}
	}
}

func TestFileStoreCompressesBodies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := NewFileStore(root)
	tokenKey := strings.Repeat("f", 64)
	body := bytes.Repeat([]byte(`{"type":"response.output_text.delta","delta":"hello world"}`+"\n"), 20_000)
	if err := store.Put(t.Context(), &model.DebugLogEntry{
		LogID: 8, AuthTokenKey: tokenKey, ReqBody: body, RespBody: body,
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	for _, name := range []string{requestFileName, responseFileName} {
		info, err := os.Stat(filepath.Join(root, tokenKey, "8", name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() >= int64(len(body))/10 {
			t.Fatalf("%s size=%d, raw=%d; expected gzip compression", name, info.Size(), len(body))
		}
	}
}

func TestFileStoreListAndCleanup(t *testing.T) {
	t.Parallel()

	store := NewFileStore(t.TempDir())
	now := time.Now().Truncate(time.Second)
	for _, tc := range []struct {
		id       int64
		tokenKey string
		created  time.Time
	}{{30, strings.Repeat("a", 64), now}, {2, strings.Repeat("b", 64), now.Add(-2 * time.Hour)}, {11, strings.Repeat("a", 64), now.Add(-time.Hour)}} {
		if err := store.Put(context.Background(), &model.DebugLogEntry{LogID: tc.id, AuthTokenKey: tc.tokenKey, CreatedAt: tc.created.Unix()}); err != nil {
			t.Fatalf("Put(%d): %v", tc.id, err)
		}
	}

	entries, err := store.List(context.Background(), 2, 0, 2)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 || entries[0].LogID != 11 || entries[1].LogID != 30 {
		t.Fatalf("List=%+v, want IDs 11,30", entries)
	}

	removed, err := store.Cleanup(context.Background(), CleanupPolicy{Cutoff: now.Add(-30 * time.Minute), MaxDelete: 1})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed=%d, want 1", removed)
	}
	remaining, err := store.List(context.Background(), 0, 0, 0)
	if err != nil {
		t.Fatalf("List remaining: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("remaining=%+v, want 2 entries", remaining)
	}
}

func TestFileStoreCleanupDoesNotAssumeIDsFollowCreationTime(t *testing.T) {
	t.Parallel()

	store := NewFileStore(t.TempDir())
	now := time.Now().Truncate(time.Second)
	for _, tc := range []struct {
		id      int64
		created time.Time
	}{
		{1, now},
		{2, now.Add(-2 * time.Hour)},
	} {
		if err := store.Put(t.Context(), &model.DebugLogEntry{LogID: tc.id, AuthTokenKey: strings.Repeat("c", 64), CreatedAt: tc.created.Unix()}); err != nil {
			t.Fatalf("Put(%d): %v", tc.id, err)
		}
	}

	removed, err := store.Cleanup(t.Context(), CleanupPolicy{Cutoff: now.Add(-time.Hour)})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed=%d, want 1", removed)
	}
	if exists, err := store.Exists(1); err != nil || !exists {
		t.Fatalf("newer entry exists=%v, err=%v", exists, err)
	}
	if exists, err := store.Exists(2); err != nil || exists {
		t.Fatalf("older entry exists=%v, err=%v", exists, err)
	}
}

func TestFileStoreCleanupPreservesConfiguredAuthToken(t *testing.T) {
	t.Parallel()

	store := NewFileStore(t.TempDir())
	now := time.Now().Truncate(time.Second)
	preservedKey := strings.Repeat("d", 64)
	entries := []*model.DebugLogEntry{
		{LogID: 1, AuthTokenID: 10, AuthTokenKey: preservedKey, CreatedAt: now.Add(-2 * time.Hour).Unix()},
		{LogID: 2, AuthTokenID: 20, AuthTokenKey: strings.Repeat("e", 64), CreatedAt: now.Add(-2 * time.Hour).Unix()},
	}
	for _, entry := range entries {
		if err := store.Put(t.Context(), entry); err != nil {
			t.Fatalf("Put(%d): %v", entry.LogID, err)
		}
	}

	removed, err := store.Cleanup(t.Context(), CleanupPolicy{
		Cutoff: now.Add(-time.Hour), PreserveAuthTokenID: 10, PreserveTokenKey: preservedKey,
	})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed=%d, want 1", removed)
	}
	if exists, _ := store.Exists(1); !exists {
		t.Fatal("configured token log was removed")
	}
	if exists, _ := store.Exists(2); exists {
		t.Fatal("unprotected expired log was retained")
	}
}

func TestFileStoreCleanupRemovesLegacyFlatLayout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	legacyDir := filepath.Join(root, "99")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "meta.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewFileStore(root)
	removed, err := store.Cleanup(t.Context(), CleanupPolicy{Cutoff: time.Now()})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed=%d, want 1", removed)
	}
	if _, err := os.Stat(legacyDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy directory still exists: %v", err)
	}
}

func TestFileStoreRejectsInvalidID(t *testing.T) {
	t.Parallel()

	store := NewFileStore(t.TempDir())
	if err := store.Put(context.Background(), &model.DebugLogEntry{}); err == nil {
		t.Fatal("Put should reject log ID zero")
	}
	if _, err := store.Get(context.Background(), -1); err == nil {
		t.Fatal("Get should reject negative log ID")
	}
}

func TestFileStoreRejectsUnsafeTokenDirectoryKey(t *testing.T) {
	t.Parallel()

	store := NewFileStore(t.TempDir())
	err := store.Put(t.Context(), &model.DebugLogEntry{LogID: 1, AuthTokenKey: "../outside"})
	if err == nil {
		t.Fatal("unsafe token key should be rejected")
	}
}
