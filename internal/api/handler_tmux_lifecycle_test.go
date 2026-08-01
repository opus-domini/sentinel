package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
	"github.com/opus-domini/sentinel/internal/tmuxlifecycle"
)

func TestSessionHandlersKeepLifecycleCoherent(t *testing.T) {
	t.Parallel()

	t.Run("SPA create remains persistent", func(t *testing.T) {
		t.Parallel()

		runtime := &apiLifecycleRuntime{}
		h, st := newTestHandler(t, &mockTmux{})
		h.lifecycle = tmuxlifecycle.New(st, tmuxlifecycle.Options{
			RuntimeForUser: func(string) tmuxlifecycle.Runtime { return runtime },
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", strings.NewReader(`{"name":"human"}`))
		h.createSession(w, r)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201", w.Code)
		}
		if snapshots := h.lifecycle.Snapshot(); len(snapshots) != 0 {
			t.Fatalf("SPA create registered lifecycle lease: %#v", snapshots)
		}
	})

	t.Run("rename follows stable runtime ID", func(t *testing.T) {
		t.Parallel()

		const (
			leaseID   = "lease_api_rename"
			sessionID = "$42"
		)
		runtime := &apiLifecycleRuntime{session: tmux.Session{ID: sessionID, Name: "old"}}
		tm := &mockTmux{
			getSessionFn: func(context.Context, string) (tmux.Session, error) {
				return runtime.session, nil
			},
			renameSessionFn: func(_ context.Context, _, newName string) error {
				runtime.session.Name = newName
				return nil
			},
		}
		h, st := newTestHandler(t, tm)
		manager := seedAPILifecycleManager(t, st, runtime, leaseID, sessionID, "old")
		h.lifecycle = manager
		if err := st.UpsertSession(context.Background(), "old", "hash", "content"); err != nil {
			t.Fatal(err)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/old", strings.NewReader(`{"newName":"new"}`))
		r.SetPathValue("session", "old")
		h.renameSession(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		lease, err := st.GetTmuxSessionLease(context.Background(), leaseID)
		if err != nil {
			t.Fatal(err)
		}
		if lease.SessionID != sessionID || lease.SessionName != "new" {
			t.Fatalf("persisted lifecycle = %#v", lease)
		}
		snapshot, ok := manager.SnapshotByID(leaseID)
		if !ok || snapshot.SessionID != sessionID || snapshot.SessionName != "new" {
			t.Fatalf("runtime lifecycle = %#v, found=%t", snapshot, ok)
		}
	})

	t.Run("human delete forgets lease and runtime projections", func(t *testing.T) {
		t.Parallel()

		const (
			leaseID   = "lease_api_delete"
			sessionID = "$43"
		)
		runtime := &apiLifecycleRuntime{session: tmux.Session{ID: sessionID, Name: "agent"}}
		tm := &mockTmux{
			getSessionFn: func(context.Context, string) (tmux.Session, error) {
				return runtime.session, nil
			},
			killSessionFn: func(context.Context, string) error {
				runtime.session = tmux.Session{}
				return nil
			},
		}
		h, st := newTestHandler(t, tm)
		manager := seedAPILifecycleManager(t, st, runtime, leaseID, sessionID, "agent")
		h.lifecycle = manager
		if err := st.UpsertSession(context.Background(), "agent", "hash", "content"); err != nil {
			t.Fatal(err)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/agent", nil)
		r.SetPathValue("session", "agent")
		h.deleteSession(w, r)

		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", w.Code)
		}
		if _, err := st.GetTmuxSessionLease(context.Background(), leaseID); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("GetTmuxSessionLease() after delete error = %v", err)
		}
		if snapshots := manager.Snapshot(); len(snapshots) != 0 {
			t.Fatalf("manager retained deleted lifecycle: %#v", snapshots)
		}
		metadata, err := st.GetAll(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := metadata["agent"]; ok {
			t.Fatalf("runtime session metadata was not removed: %#v", metadata)
		}
	})
}

func TestListSessionsProjectsLifecycleWithoutLeaseIdentity(t *testing.T) {
	t.Parallel()

	const (
		leaseID   = "lease_api_projection"
		sessionID = "$52"
	)
	now := time.Now().UTC().Truncate(time.Second)
	runtime := &apiLifecycleRuntime{session: tmux.Session{
		ID: sessionID, Name: "agent", Windows: 1, CreatedAt: now, ActivityAt: now,
	}}
	tm := &mockTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return []tmux.Session{
				runtime.session,
				{ID: "$58", Name: "human", Windows: 1, CreatedAt: now, ActivityAt: now},
			}, nil
		},
	}
	h, st := newTestHandler(t, tm)
	h.lifecycle = seedAPILifecycleManager(t, st, runtime, leaseID, sessionID, "agent")

	w := httptest.NewRecorder()
	h.listSessions(w, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "leaseId") || strings.Contains(w.Body.String(), leaseID) {
		t.Fatalf("HTTP projection exposed lifecycle lease identity: %s", w.Body.String())
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	sessions, _ := data["sessions"].([]any)
	if len(sessions) != 2 {
		t.Fatalf("sessions len = %d, want 2", len(sessions))
	}
	rows := make(map[string]map[string]any, len(sessions))
	for _, raw := range sessions {
		row, _ := raw.(map[string]any)
		name, _ := row["name"].(string)
		rows[name] = row
	}
	row := rows["agent"]
	lifecycle, _ := row["lifecycle"].(map[string]any)
	if lifecycle["mode"] != "ephemeral" || lifecycle["source"] != "mcp" || lifecycle["cleanupState"] != "active" {
		t.Fatalf("lifecycle = %#v", lifecycle)
	}
	if lifecycle["expiresAt"] == "" {
		t.Fatalf("lifecycle expiresAt missing: %#v", lifecycle)
	}
	if _, exists := rows["human"]["lifecycle"]; exists {
		t.Fatalf("persistent human session received lifecycle: %#v", rows["human"])
	}
}

func TestSessionLifecycleProjectionRequiresLiveStableIdentity(t *testing.T) {
	t.Parallel()

	const (
		leaseID   = "lease_api_reused_name"
		sessionID = "$53"
	)
	now := time.Now().UTC().Truncate(time.Second)
	runtime := &apiLifecycleRuntime{session: tmux.Session{ID: sessionID, Name: "agent"}}
	tm := &mockTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return []tmux.Session{{
				ID: "$99", Name: "agent", Windows: 1, CreatedAt: now, ActivityAt: now,
			}}, nil
		},
	}
	h, st := newTestHandler(t, tm)
	h.lifecycle = seedAPILifecycleManager(t, st, runtime, leaseID, sessionID, "agent")

	snapshot := h.loadEnrichedSessions(context.Background())
	if len(snapshot.Sessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(snapshot.Sessions))
	}
	if snapshot.Sessions[0].Lifecycle != nil {
		t.Fatalf("reused session name inherited stale lifecycle: %#v", snapshot.Sessions[0].Lifecycle)
	}
}

func TestSessionLifecycleProjectionFallsBackToPersistedNameWhenRuntimeUnavailable(t *testing.T) {
	t.Parallel()

	const (
		leaseID   = "lease_api_fallback"
		sessionID = "$54"
	)
	runtime := &apiLifecycleRuntime{session: tmux.Session{ID: sessionID, Name: "agent"}}
	h, st := newTestHandler(t, &mockTmux{})
	h.lifecycle = seedAPILifecycleManager(t, st, runtime, leaseID, sessionID, "agent")
	sessions := []enrichedSession{{Name: "agent"}}

	h.overlaySessionLifecycles(sessions, map[string]string{}, map[string]bool{})

	if sessions[0].Lifecycle == nil || sessions[0].Lifecycle.Mode != "ephemeral" {
		t.Fatalf("persisted lifecycle fallback = %#v", sessions[0].Lifecycle)
	}
}

func seedAPILifecycleManager(
	t *testing.T,
	st *store.Store,
	runtime *apiLifecycleRuntime,
	leaseID, sessionID, sessionName string,
) *tmuxlifecycle.Manager {
	t.Helper()
	now := time.Now().UTC()
	if err := st.CreateTmuxSessionLease(context.Background(), store.TmuxSessionLease{
		LeaseID:       leaseID,
		SessionID:     sessionID,
		SessionName:   sessionName,
		Source:        store.TmuxSessionLeaseSourceMCP,
		State:         store.TmuxSessionLeaseActive,
		CreatedAt:     now,
		LastRenewedAt: now,
		ExpiresAt:     now.Add(time.Hour),
		UpdatedAt:     now,
	}); err != nil {
		t.Fatal(err)
	}
	manager := tmuxlifecycle.New(st, tmuxlifecycle.Options{
		RuntimeForUser: func(string) tmuxlifecycle.Runtime { return runtime },
	})
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	return manager
}

type apiLifecycleRuntime struct {
	session tmux.Session
}

func (r *apiLifecycleRuntime) CreateSessionWithID(context.Context, string, string) (tmux.Session, error) {
	return tmux.Session{}, errors.New("unexpected lifecycle create")
}

func (r *apiLifecycleRuntime) GetSession(_ context.Context, name string) (tmux.Session, error) {
	if r.session.ID == "" || r.session.Name != name {
		return tmux.Session{}, &tmux.Error{Kind: tmux.ErrKindSessionNotFound}
	}
	return r.session, nil
}

func (r *apiLifecycleRuntime) ListSessions(context.Context) ([]tmux.Session, error) {
	if r.session.ID == "" {
		return nil, nil
	}
	return []tmux.Session{r.session}, nil
}

func (r *apiLifecycleRuntime) KillSessionByID(_ context.Context, sessionID string) error {
	if r.session.ID != sessionID {
		return &tmux.Error{Kind: tmux.ErrKindSessionNotFound}
	}
	r.session = tmux.Session{}
	return nil
}

var _ tmuxlifecycle.Runtime = (*apiLifecycleRuntime)(nil)
