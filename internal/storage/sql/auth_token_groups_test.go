package sql_test

import (
	"context"
	"testing"

	"ccLoad/internal/model"
)

func TestAuthTokenGroup_CreateAndUpdateColor(t *testing.T) {
	t.Parallel()

	store := newTestStore(t, "auth_token_groups.db")
	ctx := context.Background()

	group := &model.AuthTokenGroup{
		Name:                   "Premium",
		Description:            "primary",
		Color:                  "#3b82f6",
		AllowedChannelIDs:      []int64{4},
		ChannelRestrictionMode: model.ChannelRestrictionModeDeny,
	}
	if err := store.CreateAuthTokenGroup(ctx, group); err != nil {
		t.Fatalf("create auth token group: %v", err)
	}

	got, err := store.GetAuthTokenGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("get auth token group: %v", err)
	}
	if got.Color != "#3b82f6" {
		t.Fatalf("color=%q, want %q", got.Color, "#3b82f6")
	}
	restriction, err := got.ChannelRestriction()
	if err != nil {
		t.Fatalf("ChannelRestriction failed: %v", err)
	}
	if got.ChannelRestrictionMode != model.ChannelRestrictionModeDeny || restriction.Allows(4) {
		t.Fatalf("stored channel restriction mode/list not preserved: %+v", got)
	}

	group.Color = "#ef4444"
	group.Description = "updated"
	group.ChannelRestrictionMode = model.ChannelRestrictionModeAllow
	if err := store.UpdateAuthTokenGroup(ctx, group); err != nil {
		t.Fatalf("update auth token group: %v", err)
	}

	got, err = store.GetAuthTokenGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("get updated auth token group: %v", err)
	}
	if got.Color != "#ef4444" {
		t.Fatalf("updated color=%q, want %q", got.Color, "#ef4444")
	}
	if got.ChannelRestrictionMode != model.ChannelRestrictionModeAllow {
		t.Fatalf("updated channel_restriction_mode=%q, want allow", got.ChannelRestrictionMode)
	}
}

func TestAuthTokenGroup_InvalidChannelRestrictionModeWriteReturnsError(t *testing.T) {
	t.Parallel()

	store := newTestStore(t, "auth_token_groups_invalid_mode.db")
	err := store.CreateAuthTokenGroup(context.Background(), &model.AuthTokenGroup{
		Name:                   "Invalid",
		ChannelRestrictionMode: "denyy",
	})
	if err == nil {
		t.Fatal("expected invalid group channel_restriction_mode to be rejected")
	}
}
