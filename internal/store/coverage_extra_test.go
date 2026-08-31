// Package store persists Sentinel state in SQLite.
package store

import (
	"testing"
	"time"
)

func TestBuildWatchtowerPatchHelpers(t *testing.T) {
	t.Parallel()

	const patchSession = "dev"
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	windows := []WatchtowerWindow{{
		SessionName:      patchSession,
		WindowIndex:      1,
		TmuxWindowID:     "@2",
		Name:             "logs",
		Active:           true,
		Layout:           "layout",
		UnreadPanes:      1,
		HasUnread:        true,
		Rev:              42,
		WindowActivityAt: now,
	}}
	panes := []WatchtowerPane{{
		SessionName:  patchSession,
		WindowIndex:  1,
		PaneID:       "%3",
		Title:        "tail",
		Revision:     4,
		SeenRevision: 2,
		ChangedAt:    now,
	}}

	inspector := BuildWatchtowerInspectorPatch(patchSession, windows, panes)
	if inspector["session"] != patchSession {
		t.Fatalf("inspector session = %#v, want %s", inspector["session"], patchSession)
	}
	windowPatches, _ := inspector["windows"].([]map[string]any)
	if len(windowPatches) != 1 {
		t.Fatalf("window patches len = %d, want 1", len(windowPatches))
	}
	if windowPatches[0]["panes"] != 1 {
		t.Fatalf("window pane count = %#v, want 1", windowPatches[0]["panes"])
	}

	managed := map[string]ManagedTmuxWindow{
		"@2": {ID: "mw-1", WindowName: "Managed logs", Icon: "terminal", LauncherID: "launcher-1"},
	}
	managedPatches := BuildWatchtowerWindowPatchesWithManaged(windows, panes, managed)
	if len(managedPatches) != 1 {
		t.Fatalf("managed patches len = %d, want 1", len(managedPatches))
	}
	if managedPatches[0]["displayName"] != "Managed logs" || managedPatches[0]["managed"] != true {
		t.Fatalf("managed patch = %#v, want managed display", managedPatches[0])
	}
}
