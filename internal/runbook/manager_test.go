package runbook

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/opus-domini/sentinel/internal/events"
	opsplane "github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/store"
)

type targetCatalogStub struct {
	services []opsplane.ServiceStatus
	err      error
}

func (s targetCatalogStub) ListServices(context.Context) ([]opsplane.ServiceStatus, error) {
	return s.services, s.err
}

func TestManagerEmitsRunbookDefinitionChanges(t *testing.T) {
	t.Parallel()

	st, err := store.New(filepath.Join(t.TempDir(), "sentinel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	var (
		types   []string
		actions []string
	)
	manager := NewManager(st, nil, func(eventType string, payload map[string]any) {
		types = append(types, eventType)
		action, _ := payload["action"].(string)
		actions = append(actions, action)
	}, 1, nil)
	t.Cleanup(func() { manager.Shutdown(context.Background()) })

	created, _, err := manager.Create(context.Background(), store.OpsRunbookWrite{
		Name:  "Definition events",
		Steps: []store.OpsRunbookStep{{Type: "run", Title: "check", Command: "true"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Update(context.Background(), store.OpsRunbookWrite{
		ID:      created.ID,
		Name:    "Definition events updated",
		Enabled: true,
		Steps:   []store.OpsRunbookStep{{Type: "run", Title: "check", Command: "true"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Delete(context.Background(), created.ID, "Definition events updated"); err != nil {
		t.Fatal(err)
	}

	if got := strings.Join(types, ","); got != strings.Repeat(events.TypeOpsRunbooks+",", 2)+events.TypeOpsRunbooks {
		t.Fatalf("event types = %q", got)
	}
	if got := strings.Join(actions, ","); got != "create,update,delete" {
		t.Fatalf("event actions = %q", got)
	}
}

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
	manager := NewManager(st, targetCatalogStub{services: []opsplane.ServiceStatus{{Name: "nginx"}}}, func(_ string, payload map[string]any) {
		job, ok := payload[keyJob].(store.OpsRunbookRun)
		if !ok {
			return
		}
		mu.Lock()
		statuses = append(statuses, job.Status)
		mu.Unlock()
	}, 1, func(context.Context, string, ...string) (string, error) {
		return "", nil
	})
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
		Enabled:       true,
		TargetService: "nginx",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(context.Background(), rb.ID, map[string]string{"ENVV": "staging"}); !errors.Is(err, ErrInvalidParameters) {
		t.Fatalf("Start(unknown parameter) error = %v, want ErrInvalidParameters", err)
	}
	run, err := manager.Start(context.Background(), rb.ID, map[string]string{"ENV": "production"})
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
	if finished.Source != store.OpsRunbookRunSourceRunbooks ||
		finished.TargetKind != store.OpsRunbookRunTargetService ||
		finished.TargetName != "nginx" {
		t.Fatalf("run context = (%q, %q, %q)", finished.Source, finished.TargetKind, finished.TargetName)
	}
	nowRun, err := manager.StartFromNow(context.Background(), rb.ID, map[string]string{"ENV": "staging"})
	if err != nil {
		t.Fatal(err)
	}
	manager.WaitIdle()
	nowFinished, err := manager.GetRun(context.Background(), nowRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nowFinished.Source != store.OpsRunbookRunSourceNow {
		t.Fatalf("Now run source = %q, want %q", nowFinished.Source, store.OpsRunbookRunSourceNow)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(statuses) < 2 || statuses[0] != "queued" || statuses[len(statuses)-1] != runnerStatusSucceeded {
		t.Fatalf("event statuses = %q", statuses)
	}
}

func TestManagerValidatesUniqueTrackedServiceTargets(t *testing.T) {
	t.Parallel()

	st, err := store.New(filepath.Join(t.TempDir(), "sentinel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	manager := NewManager(st, targetCatalogStub{services: []opsplane.ServiceStatus{
		{Name: "nginx"},
		{Name: "redis"},
	}}, nil, 1, nil)
	t.Cleanup(func() { manager.Shutdown(context.Background()) })

	first, _, err := manager.Create(context.Background(), store.OpsRunbookWrite{
		Name:          "Nginx recovery",
		TargetService: "nginx",
		Steps:         []store.OpsRunbookStep{{Type: "run", Title: "Check", Command: "true"}},
	})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if first.TargetService != "nginx" {
		t.Fatalf("targetService = %q, want nginx", first.TargetService)
	}
	if _, _, err := manager.Create(context.Background(), store.OpsRunbookWrite{
		Name:          "Duplicate target",
		TargetService: "nginx",
		Steps:         []store.OpsRunbookStep{{Type: "run", Title: "Check", Command: "true"}},
	}); !errors.Is(err, ErrTargetServiceConflict) {
		t.Fatalf("duplicate target error = %v, want ErrTargetServiceConflict", err)
	}
	if _, _, err := manager.Create(context.Background(), store.OpsRunbookWrite{
		Name:          "Missing target",
		TargetService: "postgres",
		Steps:         []store.OpsRunbookStep{{Type: "run", Title: "Check", Command: "true"}},
	}); !errors.Is(err, ErrTargetServiceNotFound) {
		t.Fatalf("missing target error = %v, want ErrTargetServiceNotFound", err)
	}
}

func TestManagerRejectsSecondActiveExecutionForSameTarget(t *testing.T) {
	t.Parallel()

	st, err := store.New(filepath.Join(t.TempDir(), "sentinel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	manager := NewManager(
		st,
		targetCatalogStub{services: []opsplane.ServiceStatus{{Name: "nginx"}}},
		nil,
		2,
		func(ctx context.Context, _ string, _ ...string) (string, error) {
			once.Do(func() { close(started) })
			select {
			case <-release:
				return "ok", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
	)
	t.Cleanup(func() {
		close(release)
		manager.Shutdown(context.Background())
	})
	rb, _, err := manager.Create(context.Background(), store.OpsRunbookWrite{
		Name:          "Nginx recovery",
		TargetService: "nginx",
		Steps:         []store.OpsRunbookStep{{Type: "run", Title: "Run", Command: "true"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(context.Background(), rb.ID, nil); err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := manager.StartFromNow(context.Background(), rb.ID, nil); !errors.Is(err, ErrTargetBusy) {
		t.Fatalf("second start error = %v, want ErrTargetBusy", err)
	}
	runs, err := manager.ListRuns(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
}
