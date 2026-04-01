package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
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

func TestReorderWindowsHandler(t *testing.T) {
	t.Parallel()

	t.Run("reorders live windows and syncs managed launcher order", func(t *testing.T) {
		t.Parallel()

		const sessionName = "dev"

		h, st := newTestHandler(t, &mockTmux{}, nil)
		ctx := context.Background()

		firstManaged, err := st.CreateManagedTmuxWindow(ctx, store.ManagedTmuxWindowWrite{
			SessionName:     sessionName,
			LauncherID:      "launcher-claude",
			LauncherName:    "Claude Code",
			Icon:            "bot",
			Command:         "claude",
			CwdMode:         "session",
			WindowName:      "Claude Code",
			TmuxWindowID:    "@1",
			LastWindowIndex: 0,
		})
		if err != nil {
			t.Fatalf("CreateManagedTmuxWindow(first): %v", err)
		}
		secondManaged, err := st.CreateManagedTmuxWindow(ctx, store.ManagedTmuxWindowWrite{
			SessionName:     sessionName,
			LauncherID:      "launcher-runner",
			LauncherName:    "Runner",
			Icon:            "terminal",
			Command:         "",
			CwdMode:         "session",
			WindowName:      "Runner",
			TmuxWindowID:    "@3",
			LastWindowIndex: 2,
		})
		if err != nil {
			t.Fatalf("CreateManagedTmuxWindow(second): %v", err)
		}

		listCalls := 0
		var reordered []string
		var restoredIndex int
		restoredSelected := false
		h.tmux = &mockTmux{
			listWindowsFn: func(_ context.Context, session string) ([]tmux.Window, error) {
				if session != sessionName {
					t.Fatalf("ListWindows session = %q, want %s", session, sessionName)
				}
				listCalls++
				if listCalls == 1 {
					return []tmux.Window{
						{Session: sessionName, ID: "@1", Index: 0, Name: "claude", Active: true, Panes: 1},
						{Session: sessionName, ID: "@2", Index: 1, Name: "shell", Active: false, Panes: 1},
						{Session: sessionName, ID: "@3", Index: 2, Name: "runner", Active: false, Panes: 1},
					}, nil
				}
				return []tmux.Window{
					{Session: sessionName, ID: "@2", Index: 0, Name: "shell", Active: true, Panes: 1},
					{Session: sessionName, ID: "@3", Index: 1, Name: "runner", Active: false, Panes: 1},
					{Session: sessionName, ID: "@1", Index: 2, Name: "claude", Active: false, Panes: 1},
				}, nil
			},
			reorderWindowsFn: func(_ context.Context, session string, orderedWindowIDs []string) error {
				if session != sessionName {
					t.Fatalf("ReorderWindows session = %q, want %s", session, sessionName)
				}
				reordered = append([]string{}, orderedWindowIDs...)
				return nil
			},
			selectWindowFn: func(_ context.Context, session string, index int) error {
				if session != sessionName {
					t.Fatalf("SelectWindow session = %q, want %s", session, sessionName)
				}
				restoredIndex = index
				restoredSelected = true
				return nil
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/"+sessionName+"/windows/order", strings.NewReader(`{"windowIds":["@2","@3","@1"]}`))
		r.SetPathValue("session", sessionName)

		h.reorderWindows(w, r)

		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204 body=%s", w.Code, w.Body.String())
		}
		if !reflect.DeepEqual(reordered, []string{"@2", "@3", "@1"}) {
			t.Fatalf("reordered ids = %#v", reordered)
		}
		if !restoredSelected || restoredIndex != 2 {
			t.Fatalf("restored active window = %v index=%d, want index 2", restoredSelected, restoredIndex)
		}

		rows, err := st.ListManagedTmuxWindowsBySession(ctx, sessionName)
		if err != nil {
			t.Fatalf("ListManagedTmuxWindowsBySession(): %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("managed rows = %d, want 2", len(rows))
		}

		var gotFirst, gotSecond store.ManagedTmuxWindow
		for _, row := range rows {
			switch row.ID {
			case firstManaged.ID:
				gotFirst = row
			case secondManaged.ID:
				gotSecond = row
			}
		}
		if gotFirst.LastWindowIndex != 2 || gotFirst.SortOrder != 3 {
			t.Fatalf("first managed = %+v, want index=2 sort=3", gotFirst)
		}
		if gotSecond.LastWindowIndex != 1 || gotSecond.SortOrder != 2 {
			t.Fatalf("second managed = %+v, want index=1 sort=2", gotSecond)
		}
	})

	t.Run("rejects stale client order", func(t *testing.T) {
		t.Parallel()

		reorderCalled := false
		h, _ := newTestHandler(t, &mockTmux{
			listWindowsFn: func(_ context.Context, session string) ([]tmux.Window, error) {
				if session != "dev" {
					t.Fatalf("ListWindows session = %q, want dev", session)
				}
				return []tmux.Window{
					{Session: "dev", ID: "@1", Index: 0, Name: "claude", Active: true, Panes: 1},
					{Session: "dev", ID: "@2", Index: 1, Name: "shell", Active: false, Panes: 1},
				}, nil
			},
			reorderWindowsFn: func(_ context.Context, _ string, _ []string) error {
				reorderCalled = true
				return nil
			},
		}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/dev/windows/order", strings.NewReader(`{"windowIds":["@1","@3"]}`))
		r.SetPathValue("session", "dev")

		h.reorderWindows(w, r)

		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409 body=%s", w.Code, w.Body.String())
		}
		if reorderCalled {
			t.Fatal("expected reorder not to be called for stale order")
		}

		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("json decode error: %v", err)
		}
		if got := errCode(body); got != "WINDOW_ORDER_STALE" {
			t.Fatalf("error code = %q, want WINDOW_ORDER_STALE", got)
		}
	})
}
