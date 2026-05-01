package api

import (
	"context"
	"testing"

	"github.com/opus-domini/sentinel/internal/store"
)

func TestPopulateSessionUsersFromPresets(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	ctx := context.Background()
	if err := st.SetSessionUser(ctx, "api", "deploy"); err != nil {
		t.Fatalf("SetSessionUser: %v", err)
	}
	if _, err := st.CreateSessionPreset(ctx, store.SessionPresetWrite{
		Name: "preset-api",
		Cwd:  "/srv/preset",
		Icon: "server",
		User: "postgres",
	}); err != nil {
		t.Fatalf("CreateSessionPreset: %v", err)
	}

	h.populateSessionUsersFromPresets(ctx)
	if got := h.SessionUser("api"); got != "deploy" {
		t.Fatalf("SessionUser(api) = %q, want deploy", got)
	}
	if got := h.SessionUser("preset-api"); got != "postgres" {
		t.Fatalf("SessionUser(preset-api) = %q, want postgres", got)
	}
}
