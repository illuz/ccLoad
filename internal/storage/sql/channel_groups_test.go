package sql_test

import (
	"context"
	"testing"

	"ccLoad/internal/model"
)

func TestChannelGroup_CreateListAndConfigFields(t *testing.T) {
	t.Parallel()

	store := newTestStore(t, "channel_groups.db")
	ctx := context.Background()

	group := &model.ChannelGroup{
		Name:        "Premium",
		Description: "paid channels",
		Color:       "#3b82f6",
	}
	if err := store.CreateChannelGroup(ctx, group); err != nil {
		t.Fatalf("create channel group: %v", err)
	}

	created, err := store.CreateConfig(ctx, &model.Config{
		Name:        "grouped-channel",
		GroupID:     group.ID,
		URL:         "https://api.example.com",
		Priority:    10,
		Enabled:     true,
		ChannelType: "openai",
		ModelEntries: []model.ModelEntry{
			{Model: "gpt-4.1"},
		},
	})
	if err != nil {
		t.Fatalf("create grouped channel: %v", err)
	}

	got, err := store.GetConfig(ctx, created.ID)
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if got.GroupID != group.ID {
		t.Fatalf("group_id=%d, want %d", got.GroupID, group.ID)
	}
	if got.GroupName != group.Name {
		t.Fatalf("group_name=%q, want %q", got.GroupName, group.Name)
	}
	if got.GroupColor != group.Color {
		t.Fatalf("group_color=%q, want %q", got.GroupColor, group.Color)
	}

	groups, err := store.ListChannelGroups(ctx)
	if err != nil {
		t.Fatalf("list channel groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("group count=%d, want 1", len(groups))
	}
	if groups[0].ChannelCount != 1 {
		t.Fatalf("channel_count=%d, want 1", groups[0].ChannelCount)
	}
}

func TestChannelGroup_DeleteRejectsNonEmptyAndBatchUpdate(t *testing.T) {
	t.Parallel()

	store := newTestStore(t, "channel_groups_batch.db")
	ctx := context.Background()

	groupA := &model.ChannelGroup{Name: "Group A", Color: "#64748b"}
	groupB := &model.ChannelGroup{Name: "Group B", Color: "#22c55e"}
	if err := store.CreateChannelGroup(ctx, groupA); err != nil {
		t.Fatalf("create group A: %v", err)
	}
	if err := store.CreateChannelGroup(ctx, groupB); err != nil {
		t.Fatalf("create group B: %v", err)
	}

	ch1 := createTestChannel(t, ctx, store, "channel-a")
	ch2 := createTestChannel(t, ctx, store, "channel-b")

	if _, err := store.UpdateConfig(ctx, ch1, &model.Config{
		Name:        "channel-a",
		GroupID:     groupA.ID,
		URL:         "https://api.example.com",
		Priority:    1,
		Enabled:     true,
		ChannelType: "openai",
		ModelEntries: []model.ModelEntry{
			{Model: "gpt-4"},
		},
	}); err != nil {
		t.Fatalf("assign group A to channel-a: %v", err)
	}
	if _, err := store.UpdateConfig(ctx, ch2, &model.Config{
		Name:        "channel-b",
		GroupID:     0,
		URL:         "https://api.example.com",
		Priority:    1,
		Enabled:     true,
		ChannelType: "openai",
		ModelEntries: []model.ModelEntry{
			{Model: "gpt-4"},
		},
	}); err != nil {
		t.Fatalf("set channel-b ungrouped: %v", err)
	}

	if err := store.DeleteChannelGroup(ctx, groupA.ID); err == nil {
		t.Fatalf("expected delete non-empty group to fail")
	}

	updated, err := store.BatchUpdateChannelGroup(ctx, []int64{ch1, ch2}, groupB.ID)
	if err != nil {
		t.Fatalf("batch update channel group: %v", err)
	}
	if updated != 2 {
		t.Fatalf("updated=%d, want 2", updated)
	}

	got1, err := store.GetConfig(ctx, ch1)
	if err != nil {
		t.Fatalf("get channel-a: %v", err)
	}
	got2, err := store.GetConfig(ctx, ch2)
	if err != nil {
		t.Fatalf("get channel-b: %v", err)
	}
	if got1.GroupID != groupB.ID || got2.GroupID != groupB.ID {
		t.Fatalf("group ids after batch update = (%d,%d), want both %d", got1.GroupID, got2.GroupID, groupB.ID)
	}

	updated, err = store.BatchUpdateChannelGroup(ctx, []int64{ch1}, 0)
	if err != nil {
		t.Fatalf("batch clear channel group: %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated when clear=%d, want 1", updated)
	}
	got1, err = store.GetConfig(ctx, ch1)
	if err != nil {
		t.Fatalf("get channel-a after clear: %v", err)
	}
	if got1.GroupID != 0 {
		t.Fatalf("channel-a group_id=%d, want 0", got1.GroupID)
	}
}
