package api

import (
	"context"
	"testing"

	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

func TestPopulateSessionUsersFromPresets(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil)
	h.guard = security.NewWithMultiUser("", nil, security.CookieSecureAuto, security.MultiUserConfig{
		SystemUsers: []string{"deploy", "postgres"},
	})
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

// Narrowing allowed_users (or turning off allow_root_target) and restarting is
// the flow the Settings UI prescribes, so rows persisted under the old policy
// must not be promoted back into executable capabilities at boot.
func TestPopulateSessionUsersDropsMappingsThePolicyRejects(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil)
	h.guard = security.NewWithMultiUser("", nil, security.CookieSecureAuto, security.MultiUserConfig{
		AllowedUsers: []string{"deploy"},
		SystemUsers:  []string{"deploy", "postgres", "root"},
	})
	ctx := context.Background()
	if err := st.SetSessionUser(ctx, "api", "deploy"); err != nil {
		t.Fatalf("SetSessionUser(api): %v", err)
	}
	if err := st.SetSessionUser(ctx, "db", "postgres"); err != nil {
		t.Fatalf("SetSessionUser(db): %v", err)
	}
	if _, err := st.CreateSessionPreset(ctx, store.SessionPresetWrite{
		Name: "rooted",
		Cwd:  "/root",
		Icon: "server",
		User: "root",
	}); err != nil {
		t.Fatalf("CreateSessionPreset: %v", err)
	}

	h.populateSessionUsersFromPresets(ctx)

	if got := h.SessionUser("api"); got != "deploy" {
		t.Fatalf("SessionUser(api) = %q, want deploy", got)
	}
	if got := h.SessionUser("db"); got != "" {
		t.Fatalf("SessionUser(db) = %q, want empty (postgres is off the allowlist)", got)
	}
	if got := h.SessionUser("rooted"); got != "" {
		t.Fatalf("SessionUser(rooted) = %q, want empty (root is off the allowlist)", got)
	}
	// A dropped mapping must not leak into the probe fallback either.
	for _, user := range h.knownSessionUsers() {
		if user != "deploy" {
			t.Fatalf("knownSessionUsers = %#v, want only [deploy]", h.knownSessionUsers())
		}
	}
}

// tmuxForSession is where a stored name becomes a command runner, so it is the
// point that must refuse a mapping the policy no longer permits.
func TestTmuxForSessionRefusesMappingThePolicyRejects(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, &mockTmux{})
	h.guard = security.NewWithMultiUser("", nil, security.CookieSecureAuto, security.MultiUserConfig{
		AllowedUsers: []string{"deploy"},
		SystemUsers:  []string{"deploy", "postgres"},
	})
	// Simulate a row that predates the allowlist being narrowed.
	h.sessionUsers.Store("db", "postgres")

	svc := h.tmuxForSession(context.Background(), "db")
	if wrapped, ok := svc.(tmux.Service); ok && wrapped.User != "" {
		t.Fatalf("tmuxForSession returned a service for %q, want the default service", wrapped.User)
	}
}
