package runbook

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/opus-domini/sentinel/internal/store"
)

func TestManagerSharesValidationExecutionAndEventOrdering(t *testing.T) {
	t.Parallel()
	st, err := store.New(filepath.Join(t.TempDir(), "sentinel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	var (
		mu       sync.Mutex
		statuses []string
	)
	manager := NewManager(st, func(_ string, payload map[string]any) {
		job, ok := payload[keyJob].(store.OpsRunbookRun)
		if !ok {
			return
		}
		mu.Lock()
		statuses = append(statuses, job.Status)
		mu.Unlock()
	}, 1)
	t.Cleanup(func() { manager.Shutdown(context.Background()) })

	if _, _, err := manager.Create(context.Background(), store.OpsRunbookWrite{Name: "invalid"}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("Create() error = %v, want ErrInvalidDefinition", err)
	}
	rb, _, err := manager.Create(context.Background(), store.OpsRunbookWrite{
		Name:  "manager",
		Steps: []store.OpsRunbookStep{{Type: "run", Title: "run", Command: "true"}},
		Parameters: []store.RunbookParameter{{
			Name: "ENV", Type: "select", Options: []string{"staging", "production"}, Required: true,
		}},
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(context.Background(), rb.ID, map[string]string{"ENVV": "staging"}, "test"); !errors.Is(err, ErrInvalidParameters) {
		t.Fatalf("Start(unknown parameter) error = %v, want ErrInvalidParameters", err)
	}
	run, err := manager.Start(context.Background(), rb.ID, map[string]string{"ENV": "production"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	manager.WaitIdle()
	finished, err := manager.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != runnerStatusSucceeded {
		t.Fatalf("run status = %q, want succeeded", finished.Status)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(statuses) < 2 || statuses[0] != "queued" || statuses[len(statuses)-1] != runnerStatusSucceeded {
		t.Fatalf("event statuses = %q", statuses)
	}
}
