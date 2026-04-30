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
	if _, err := st.CreateSessionPreset(ctx, store.SessionPresetWrite{
		Name: "api",
		Cwd:  "/srv/api",
		Icon: "server",
		User: "deploy",
	}); err != nil {
		t.Fatalf("CreateSessionPreset: %v", err)
	}

	h.populateSessionUsersFromPresets(ctx)
	if got := h.SessionUser("api"); got != "deploy" {
		t.Fatalf("SessionUser(api) = %q, want deploy", got)
	}
}
