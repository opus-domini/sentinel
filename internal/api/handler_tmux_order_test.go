package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/opus-domini/sentinel/internal/store"
)

func TestReorderSessionsHandler(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, &mockTmux{}, nil)
	ctx := context.Background()
	for _, name := range []string{"api", "web", "docs"} {
		if err := st.UpsertSession(ctx, name, "hash-"+name, "ready"); err != nil {
			t.Fatalf("UpsertSession(%s): %v", name, err)
		}
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/order", strings.NewReader(`{"names":["docs","api","web"]}`))
	h.reorderSessions(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}

	meta, err := st.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll(): %v", err)
	}
	if meta["docs"].SortOrder != 1 || meta["api"].SortOrder != 2 || meta["web"].SortOrder != 3 {
		t.Fatalf("unexpected order: docs=%d api=%d web=%d", meta["docs"].SortOrder, meta["api"].SortOrder, meta["web"].SortOrder)
	}
}

func TestReorderSessionPresetsHandler(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, &mockTmux{}, nil)
	ctx := context.Background()
	for _, name := range []string{"api", "web", "docs"} {
		if _, err := st.CreateSessionPreset(ctx, store.SessionPresetWrite{
			Name: name,
			Cwd:  "/srv/" + name,
			Icon: "terminal",
		}); err != nil {
			t.Fatalf("CreateSessionPreset(%s): %v", name, err)
		}
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPatch, "/api/tmux/session-presets/order", strings.NewReader(`{"names":["docs","api","web"]}`))
	h.reorderSessionPresets(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}

	presets, err := st.ListSessionPresets(ctx)
	if err != nil {
		t.Fatalf("ListSessionPresets(): %v", err)
	}
	if presets[0].Name != "docs" || presets[1].Name != "api" || presets[2].Name != "web" {
		t.Fatalf("unexpected preset order: %#v", presets)
	}
}
