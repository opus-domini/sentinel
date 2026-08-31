package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/opus-domini/sentinel/internal/tmux"
)

func errMessageValue(body map[string]any) string {
	e, ok := body["error"].(map[string]any)
	if !ok {
		return ""
	}
	m, _ := e[keyMessage].(string)
	return m
}

func TestSessionParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pathValue   string
		wantSession string
		wantOK      bool
	}{
		{"trims_surrounding_space", "  dev  ", "dev", true},
		{"rejects_name_with_space", "bad name", "", false},
		{"rejects_empty_name", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions/x/windows", nil)
			r.SetPathValue(keySession, tt.pathValue)

			session, ok := sessionParam(w, r)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if session != tt.wantSession {
				t.Fatalf("session = %q, want %q", session, tt.wantSession)
			}
			if tt.wantOK {
				if w.Body.Len() != 0 {
					t.Fatalf("valid session wrote a body: %s", w.Body.String())
				}
				return
			}
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", w.Code)
			}
			body := jsonBody(t, w)
			if errCode(body) != invalidRequestCode {
				t.Errorf("code = %q, want %s", errCode(body), invalidRequestCode)
			}
			if got := errMessageValue(body); got != "invalid session name" {
				t.Errorf("message = %q, want %q", got, "invalid session name")
			}
		})
	}
}

func TestRequirePaneInSession(t *testing.T) {
	t.Parallel()

	livePanes := func(_ context.Context, _ string) ([]tmux.Pane, error) {
		return []tmux.Pane{{Session: "dev", PaneID: "%5"}}, nil
	}

	t.Run("pane in session passes without writing", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{listPanesFn: livePanes})
		w := httptest.NewRecorder()

		if !h.requirePaneInSession(context.Background(), w, "dev", "%5") {
			t.Fatalf("requirePaneInSession() = false, want true")
		}
		if w.Body.Len() != 0 {
			t.Fatalf("wrote a body for an allowed pane: %s", w.Body.String())
		}
	})

	t.Run("pane outside session is rejected as client input", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{listPanesFn: livePanes})
		w := httptest.NewRecorder()

		if h.requirePaneInSession(context.Background(), w, "dev", "%9") {
			t.Fatalf("requirePaneInSession() = true, want false")
		}
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
		body := jsonBody(t, w)
		if errCode(body) != invalidRequestCode {
			t.Errorf("code = %q, want %s", errCode(body), invalidRequestCode)
		}
		if got := errMessageValue(body); got != "paneId does not belong to session" {
			t.Errorf("message = %q, want %q", got, "paneId does not belong to session")
		}
	})

	t.Run("missing session keeps the tmux status", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			listPanesFn: func(_ context.Context, _ string) ([]tmux.Pane, error) {
				return nil, &tmux.Error{Kind: tmux.ErrKindSessionNotFound}
			},
		}
		h, _ := newTestHandler(t, tm)
		w := httptest.NewRecorder()

		if h.requirePaneInSession(context.Background(), w, "dev", "%5") {
			t.Fatalf("requirePaneInSession() = true, want false")
		}
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
		if got := errCode(jsonBody(t, w)); got != string(tmux.ErrKindSessionNotFound) {
			t.Errorf("code = %q, want %q", got, string(tmux.ErrKindSessionNotFound))
		}
	})
}
