package watchtower

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

func BenchmarkCollectFiftyPanes(b *testing.B) {
	dbPath := filepath.Join(b.TempDir(), "watchtower-bench.db")
	st, err := store.New(dbPath)
	if err != nil {
		b.Fatalf("store.New(%s): %v", dbPath, err)
	}
	defer func() { _ = st.Close() }()

	now := time.Now().UTC().Truncate(time.Second)
	windows := make([]tmux.Window, 0, 5)
	panes := make([]tmux.Pane, 0, 50)
	paneCounter := 0
	for windowIdx := 0; windowIdx < 5; windowIdx++ {
		windows = append(windows, tmux.Window{
			Session: "bench",
			Index:   windowIdx,
			Name:    "w",
			Active:  windowIdx == 0,
			Panes:   10,
			Layout:  "layout",
		})
		for paneIdx := 0; paneIdx < 10; paneIdx++ {
			paneCounter++
			panes = append(panes, tmux.Pane{
				Session:        "bench",
				WindowIndex:    windowIdx,
				PaneIndex:      paneIdx,
				PaneID:         "%" + strconv.Itoa(paneCounter),
				Title:          "pane",
				Active:         paneCounter == 1,
				TTY:            "/dev/pts/1",
				CurrentPath:    "/tmp",
				StartCommand:   "zsh",
				CurrentCommand: "zsh",
			})
		}
	}

	fake := fakeTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return []tmux.Session{{
				Name:       "bench",
				Windows:    len(windows),
				Attached:   1,
				CreatedAt:  now,
				ActivityAt: now,
			}}, nil
		},
		listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
			return windows, nil
		},
		listPanesFn: func(context.Context, string) ([]tmux.Pane, error) {
			return panes, nil
		},
		capturePaneLinesFn: func(context.Context, string, int) (string, error) {
			return "benchmark output line", nil
		},
	}
	svc := New(st, fake, Options{
		TickInterval:   time.Second,
		CaptureLines:   80,
		CaptureTimeout: 200 * time.Millisecond,
		JournalRows:    10000,
	})

	// Warmup initializes projections.
	if err := svc.collect(context.Background()); err != nil {
		b.Fatalf("warmup collect: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := svc.collect(context.Background()); err != nil {
			b.Fatalf("collect #%d: %v", i, err)
		}
	}
}
