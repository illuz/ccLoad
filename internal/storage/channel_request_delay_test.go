//go:build sonic

package storage_test

import (
	"context"
	"path/filepath"
	"testing"

	"ccLoad/internal/model"
	"ccLoad/internal/storage"
)

func TestChannelRequestDelayPersistence(t *testing.T) {
	ctx := context.Background()
	store, err := storage.CreateSQLiteStore(filepath.Join(t.TempDir(), "request-delay.db"))
	if err != nil {
		t.Fatalf("CreateSQLiteStore() error = %v", err)
	}
	defer func() { _ = store.Close() }()

	created, err := store.CreateConfig(ctx, &model.Config{
		Name:                "delayed-channel",
		URL:                 "https://api.example.com",
		RequestDelaySeconds: 3,
		ModelEntries:        []model.ModelEntry{{Model: "model-1"}},
		Enabled:             true,
	})
	if err != nil {
		t.Fatalf("CreateConfig() error = %v", err)
	}
	if created.RequestDelaySeconds != 3 {
		t.Fatalf("created delay = %d, want 3", created.RequestDelaySeconds)
	}

	created.RequestDelaySeconds = 7
	updated, err := store.UpdateConfig(ctx, created.ID, created)
	if err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}
	if updated.RequestDelaySeconds != 7 {
		t.Fatalf("updated delay = %d, want 7", updated.RequestDelaySeconds)
	}

	channels, err := store.GetEnabledChannelsByModel(ctx, "model-1")
	if err != nil {
		t.Fatalf("GetEnabledChannelsByModel() error = %v", err)
	}
	if len(channels) != 1 || channels[0].RequestDelaySeconds != 7 {
		t.Fatalf("channels = %+v, want one channel with delay 7", channels)
	}
}
