package watchtower

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

func TestDetectTimelineMarkerWithPatterns(t *testing.T) {
	t.Parallel()

	patterns := []store.MarkerPattern{
		{ID: "1", Pattern: "panic", Severity: "error", Enabled: true, Priority: 10},
		{ID: "2", Pattern: "timeout", Severity: "warn", Enabled: true, Priority: 50},
		{ID: "3", Pattern: "disabled-pattern", Severity: "error", Enabled: false, Priority: 5},
	}

	tests := []struct {
		name       string
		preview    string
		wantMatch  bool
		wantMarker string
		wantSev    string
	}{
		{
			name:       "matches error pattern",
			preview:    "goroutine 1 [panic]: runtime error",
			wantMatch:  true,
			wantMarker: "panic",
			wantSev:    "error",
		},
		{
			name:       "matches warn pattern",
			preview:    "connection timeout after 30s",
			wantMatch:  true,
			wantMarker: "timeout",
			wantSev:    "warn",
		},
		{
			name:       "case insensitive",
			preview:    "PANIC: fatal crash",
			wantMatch:  true,
			wantMarker: "panic",
			wantSev:    "error",
		},
		{
			name:       "disabled pattern not matched",
			preview:    "disabled-pattern found",
			wantMatch:  false,
			wantMarker: "",
			wantSev:    "",
		},
		{
			name:       "no match",
			preview:    "everything is fine",
			wantMatch:  false,
			wantMarker: "",
			wantSev:    "",
		},
		{
			name:       "empty preview",
			preview:    "",
			wantMatch:  false,
			wantMarker: "",
			wantSev:    "",
		},
		{
			name:       "priority ordering first match wins",
			preview:    "panic timeout both present",
			wantMatch:  true,
			wantMarker: "panic",
			wantSev:    "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			marker, sev, matched := detectTimelineMarker(tt.preview, patterns)
			if matched != tt.wantMatch {
				t.Fatalf("matched = %v, want %v", matched, tt.wantMatch)
			}
			if marker != tt.wantMarker {
				t.Fatalf("marker = %q, want %q", marker, tt.wantMarker)
			}
			if sev != tt.wantSev {
				t.Fatalf("severity = %q, want %q", sev, tt.wantSev)
			}
		})
	}
}

func TestDetectTimelineMarkerFallback(t *testing.T) {
	t.Parallel()

	// When patterns slice is empty, the function falls back to hardcoded markers.
	marker, sev, matched := detectTimelineMarker("panic: boom", nil)
	if !matched {
		t.Fatal("expected fallback match for 'panic'")
	}
	if marker != "panic" || sev != "error" {
		t.Fatalf("fallback: marker=%q sev=%q, want panic/error", marker, sev)
	}

	marker, sev, matched = detectTimelineMarker("deprecated function call", nil)
	if !matched {
		t.Fatal("expected fallback match for 'deprecated'")
	}
	if marker != "deprecated" || sev != "warn" {
		t.Fatalf("fallback: marker=%q sev=%q, want deprecated/warn", marker, sev)
	}
}

func TestDetectTimelineMarkerEmptyPatterns(t *testing.T) {
	t.Parallel()

	// Empty slice (not nil) means no patterns configured — should not match.
	_, _, matched := detectTimelineMarker("panic: boom", []store.MarkerPattern{})
	if matched {
		t.Fatal("expected no match with empty (non-nil) patterns slice")
	}
}

func TestMarkerCacheRefresh(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	svc := New(st, fakeTmux{}, Options{})

	// Initially, cache is empty.
	if patterns := svc.cachedMarkerPatterns(); len(patterns) != 0 {
		t.Fatalf("initial cache len = %d, want 0", len(patterns))
	}

	// Refresh should load patterns from the store (seeds).
	svc.refreshMarkerCache(context.Background())
	patterns := svc.cachedMarkerPatterns()
	if len(patterns) < 8 {
		t.Fatalf("after refresh cache len = %d, want >= 8", len(patterns))
	}

	// Cache should be valid; a second refresh with a short TTL should
	// still return cached data without re-querying.
	svc.markerMu.Lock()
	svc.markerCacheTTL = 10 * time.Second
	svc.markerMu.Unlock()

	svc.refreshMarkerCache(context.Background())
	if got := svc.cachedMarkerPatterns(); len(got) != len(patterns) {
		t.Fatalf("cache should be stable: got %d, want %d", len(got), len(patterns))
	}
}

func TestCollectUsesConfiguredMarkerPatterns(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()
	ctx := context.Background()

	// Add a custom pattern and disable the builtin ones.
	if err := st.UpsertMarkerPattern(ctx, store.MarkerPatternWrite{
		ID:       "custom.deploy-failed",
		Pattern:  "deploy failed",
		Severity: "error",
		Label:    "Deploy failure",
		Enabled:  true,
		Priority: 1,
	}); err != nil {
		t.Fatalf("UpsertMarkerPattern: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	var collectCount atomic.Int32
	fake := fakeTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return []tmux.Session{{
				Name:       "dev",
				Windows:    1,
				Attached:   1,
				CreatedAt:  now,
				ActivityAt: now,
			}}, nil
		},
		listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
			return []tmux.Window{{
				Session: "dev", Index: 0, Name: "main",
				Active: true, Panes: 1, Layout: "layout",
			}}, nil
		},
		listPanesFn: func(context.Context, string) ([]tmux.Pane, error) {
			return []tmux.Pane{{
				Session: "dev", WindowIndex: 0, PaneIndex: 0,
				PaneID: "%1", Title: "main", Active: true,
				CurrentPath: "/repo", StartCommand: "zsh",
				CurrentCommand: "zsh",
			}}, nil
		},
		capturePaneLinesFn: func(context.Context, string, int) (string, error) {
			if collectCount.Add(1) == 1 {
				return "line one", nil
			}
			return "deploy failed: exit code 1", nil
		},
	}

	svc := New(st, fake, Options{})

	// First collect: "line one" — no marker.
	if err := svc.collect(ctx); err != nil {
		t.Fatalf("collect #1: %v", err)
	}

	// Second collect: "deploy failed" — should trigger the custom marker.
	if err := svc.collect(ctx); err != nil {
		t.Fatalf("collect #2: %v", err)
	}

	timeline, err := st.SearchWatchtowerTimelineEvents(ctx, store.WatchtowerTimelineQuery{
		Session: "dev",
		Limit:   50,
	})
	if err != nil {
		t.Fatalf("SearchWatchtowerTimelineEvents: %v", err)
	}

	var foundCustomMarker bool
	for _, event := range timeline.Events {
		if event.EventType == "output.marker" && event.Marker == "deploy failed" {
			foundCustomMarker = true
			if event.Severity != "error" {
				t.Fatalf("custom marker severity = %q, want error", event.Severity)
			}
			break
		}
	}
	if !foundCustomMarker {
		t.Fatal("expected custom marker 'deploy failed' in timeline events")
	}
}

func TestNilServiceCachedMarkerPatterns(t *testing.T) {
	t.Parallel()

	var svc *Service
	patterns := svc.cachedMarkerPatterns()
	if patterns != nil {
		t.Fatalf("nil service cachedMarkerPatterns = %v, want nil", patterns)
	}
}

func TestNilServiceRefreshMarkerCache(t *testing.T) {
	t.Parallel()

	// Should not panic on nil service.
	var svc *Service
	svc.refreshMarkerCache(context.Background())
}
