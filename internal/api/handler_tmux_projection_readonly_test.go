package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

// upsertCountingRepo counts writes to the sessions table so a listing test can
// assert that reading sessions does not write on every request.
type upsertCountingRepo struct {
	handlerRepo
	upserts atomic.Int64
}

func (r *upsertCountingRepo) UpsertSession(ctx context.Context, name, hash, content string) error {
	r.upserts.Add(1)
	return r.handlerRepo.UpsertSession(ctx, name, hash, content)
}

func listSessionsOK(t *testing.T, h *Handler) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	h.listSessions(w, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	return jsonBody(t, w)
}

func TestListSessionsDoesNotRewriteStoredSessionMeta(t *testing.T) {
	t.Parallel()

	t.Run("projected session with stored hash never writes", func(t *testing.T) {
		t.Parallel()
		const sessionName = "dev"
		ctx := context.Background()
		now := time.Now().UTC().Truncate(time.Second)
		h, st := newTestHandler(t, &mockTmux{
			listSessionsFn: func(_ context.Context) ([]tmux.Session, error) {
				return nil, &tmux.Error{Kind: tmux.ErrKindServerNotRunning}
			},
		})
		if err := st.UpsertSession(ctx, sessionName, "h-fixed", "legacy"); err != nil {
			t.Fatalf("UpsertSession: %v", err)
		}
		if err := st.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{
			SessionName: sessionName,
			Windows:     1,
			Panes:       1,
			ActivityAt:  now,
			LastPreview: "tail from watchtower",
			Rev:         3,
			UpdatedAt:   now,
		}); err != nil {
			t.Fatalf("UpsertWatchtowerSession: %v", err)
		}

		counter := &upsertCountingRepo{handlerRepo: h.repo}
		h.repo = counter

		for range 3 {
			body := listSessionsOK(t, h)
			data, _ := body["data"].(map[string]any)
			sessions, _ := data["sessions"].([]any)
			if len(sessions) != 1 {
				t.Fatalf("sessions count = %d, want 1", len(sessions))
			}
			item, _ := sessions[0].(map[string]any)
			if item["hash"] != "h-fixed" {
				t.Fatalf("hash = %v, want h-fixed", item["hash"])
			}
			if item["lastContent"] != "tail from watchtower" {
				t.Fatalf("lastContent = %v, want tail from watchtower", item["lastContent"])
			}
		}

		if got := counter.upserts.Load(); got != 0 {
			t.Fatalf("UpsertSession calls = %d, want 0: listing sessions must not write", got)
		}
	})

	t.Run("live session seeds the hash once", func(t *testing.T) {
		t.Parallel()
		const sessionName = "api"
		now := time.Now().UTC().Truncate(time.Second)
		h, _ := newTestHandler(t, &mockTmux{
			listSessionsFn: func(_ context.Context) ([]tmux.Session, error) {
				return []tmux.Session{{
					Name:       sessionName,
					Windows:    1,
					CreatedAt:  now,
					ActivityAt: now,
				}}, nil
			},
		})

		counter := &upsertCountingRepo{handlerRepo: h.repo}
		h.repo = counter

		first := listSessionsOK(t, h)
		firstData, _ := first["data"].(map[string]any)
		firstSessions, _ := firstData["sessions"].([]any)
		if len(firstSessions) != 1 {
			t.Fatalf("sessions count = %d, want 1", len(firstSessions))
		}
		firstItem, _ := firstSessions[0].(map[string]any)
		seededHash, _ := firstItem["hash"].(string)
		if seededHash == "" {
			t.Fatal("hash was not computed for a session with no stored meta")
		}
		if got := counter.upserts.Load(); got != 1 {
			t.Fatalf("UpsertSession calls after first listing = %d, want 1", got)
		}

		for range 3 {
			body := listSessionsOK(t, h)
			data, _ := body["data"].(map[string]any)
			sessions, _ := data["sessions"].([]any)
			item, _ := sessions[0].(map[string]any)
			if item["hash"] != seededHash {
				t.Fatalf("hash = %v, want stable %s", item["hash"], seededHash)
			}
		}
		if got := counter.upserts.Load(); got != 1 {
			t.Fatalf("UpsertSession calls after repeated listings = %d, want 1", got)
		}
	})
}
