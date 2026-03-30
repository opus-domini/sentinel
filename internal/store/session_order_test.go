package store

import (
	"context"
	"testing"
)

func TestMoveSessionToFront(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	if err := s.UpsertSession(ctx, "api", "h1", "ready"); err != nil {
		t.Fatalf("UpsertSession(api): %v", err)
	}
	if err := s.UpsertSession(ctx, "web", "h2", "ready"); err != nil {
		t.Fatalf("UpsertSession(web): %v", err)
	}
	if err := s.MoveSessionToFront(ctx, "web"); err != nil {
		t.Fatalf("MoveSessionToFront(web): %v", err)
	}

	meta, err := s.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll(): %v", err)
	}
	if meta["web"].SortOrder >= meta["api"].SortOrder {
		t.Fatalf("web sort_order = %d, api sort_order = %d, want web before api", meta["web"].SortOrder, meta["api"].SortOrder)
	}
}

func TestReorderSessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	for _, name := range []string{"api", "web", "docs"} {
		if err := s.UpsertSession(ctx, name, "hash-"+name, "ready"); err != nil {
			t.Fatalf("UpsertSession(%s): %v", name, err)
		}
	}

	if err := s.ReorderSessions(ctx, []string{"docs", "api", "web"}); err != nil {
		t.Fatalf("ReorderSessions(): %v", err)
	}

	meta, err := s.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll(): %v", err)
	}
	if meta["docs"].SortOrder != 1 || meta["api"].SortOrder != 2 || meta["web"].SortOrder != 3 {
		t.Fatalf("unexpected session order: docs=%d api=%d web=%d", meta["docs"].SortOrder, meta["api"].SortOrder, meta["web"].SortOrder)
	}
}

func TestReorderSessionPresets(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	for _, name := range []string{"api", "web", "docs"} {
		if _, err := s.CreateSessionPreset(ctx, SessionPresetWrite{
			Name: name,
			Cwd:  "/srv/" + name,
			Icon: "terminal",
		}); err != nil {
			t.Fatalf("CreateSessionPreset(%s): %v", name, err)
		}
	}

	if err := s.ReorderSessionPresets(ctx, []string{"docs", "api", "web"}); err != nil {
		t.Fatalf("ReorderSessionPresets(): %v", err)
	}

	presets, err := s.ListSessionPresets(ctx)
	if err != nil {
		t.Fatalf("ListSessionPresets(): %v", err)
	}
	if len(presets) != 3 {
		t.Fatalf("got %d presets, want 3", len(presets))
	}
	if presets[0].Name != "docs" || presets[1].Name != "api" || presets[2].Name != "web" {
		t.Fatalf("unexpected preset order: %#v", presets)
	}
}
