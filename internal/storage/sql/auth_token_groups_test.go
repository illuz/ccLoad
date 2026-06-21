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
		Name:        "Premium",
		Description: "primary",
		Color:       "#3b82f6",
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

	group.Color = "#ef4444"
	group.Description = "updated"
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
}
