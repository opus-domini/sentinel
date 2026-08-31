package mcpserver

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/runbook"
	opsplane "github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/store"
)

func TestRunbookToolErrorReportsBusyTarget(t *testing.T) {
	t.Parallel()

	err := runbookToolError("run runbook", runbook.ErrTargetBusy)
	if err == nil || err.Error() != "target service already has an active execution" {
		t.Fatalf("error = %v", err)
	}
}

func assertExecutionReceipt(t *testing.T, definition *store.OpsRunbookExecutionSnapshot, runbookID string) {
	t.Helper()
	if definition == nil || definition.SchemaVersion != 1 || definition.RunbookID != runbookID {
		t.Fatalf("execution receipt = %#v", definition)
	}
}

type runbookTargetCatalog struct {
	services []opsplane.ServiceStatus
}

func (c runbookTargetCatalog) ListServices(context.Context) ([]opsplane.ServiceStatus, error) {
	return c.services, nil
}

func TestRunbookToolsLifecycle(t *testing.T) {
	t.Parallel()
	st, err := store.New(filepath.Join(t.TempDir(), "sentinel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	var eventMu sync.Mutex
	var runbookActions []string
	manager := runbook.NewManager(st, nil, func(eventType string, payload map[string]any) {
		if eventType != events.TypeOpsRunbooks {
			return
		}
		eventMu.Lock()
		defer eventMu.Unlock()
		action, _ := payload["action"].(string)
		runbookActions = append(runbookActions, action)
	}, 2, func(context.Context, string, ...string) (string, error) {
		return "0123456789", nil
	})
	t.Cleanup(func() { manager.Shutdown(context.Background()) })
	toolset := &tools{runbooks: manager}

	_, created, err := toolset.createRunbook(context.Background(), nil, runbookCreateInput{
		Name:  "MCP output",
		Steps: []store.OpsRunbookStep{{Type: "run", Title: "print", Command: "printf 0123456789"}},
	})
	if err != nil {
		t.Fatalf("createRunbook() error = %v", err)
	}
	if !created.Runbook.Enabled {
		t.Fatal("createRunbook() did not default enabled to true")
	}

	_, listed, err := toolset.listRunbooks(context.Background(), nil, runbookListInput{})
	if err != nil {
		t.Fatalf("listRunbooks() = %#v, error = %v", listed, err)
	}
	found := false
	for _, item := range listed.Runbooks {
		found = found || item.ID == created.Runbook.ID
	}
	if !found {
		t.Fatalf("created runbook missing from listRunbooks(): %#v", listed)
	}
	_, got, err := toolset.getRunbook(context.Background(), nil, runbookIDInput{RunbookID: created.Runbook.ID})
	if err != nil || got.Runbook.Name != "MCP output" {
		t.Fatalf("getRunbook() = %#v, error = %v", got, err)
	}

	_, started, err := toolset.runRunbook(context.Background(), nil, runbookRunInput{RunbookID: created.Runbook.ID})
	if err != nil {
		t.Fatalf("runRunbook() error = %v", err)
	}
	_, waited, err := toolset.waitRunbook(context.Background(), nil, runbookWaitInput{
		RunID:           started.Run.ID,
		TimeoutMS:       5_000,
		OutputTailChars: 4,
	})
	if err != nil {
		t.Fatalf("waitRunbook() error = %v", err)
	}
	if waited.TimedOut || waited.Run.Status != "succeeded" {
		t.Fatalf("waitRunbook() = %#v", waited)
	}
	if len(waited.Run.StepResults) != 1 || waited.Run.StepResults[0].Output != "6789" || !waited.Run.StepResults[0].OutputTruncated {
		t.Fatalf("bounded step output = %#v", waited.Run.StepResults)
	}
	assertExecutionReceipt(t, waited.Run.Definition, created.Runbook.ID)

	_, runs, err := toolset.listRunbookRuns(context.Background(), nil, runbookListRunsInput{Limit: 1, OutputTailChars: 4})
	if err != nil || len(runs.Runs) != 1 || runs.Runs[0].ID != started.Run.ID {
		t.Fatalf("listRunbookRuns() = %#v, error = %v", runs, err)
	}
	_, fetchedRun, err := toolset.getRunbookRun(context.Background(), nil, runbookRunLookupInput{RunID: started.Run.ID, OutputTailChars: 4})
	if err != nil || fetchedRun.Run.Status != "succeeded" {
		t.Fatalf("getRunbookRun() = %#v, error = %v", fetchedRun, err)
	}

	if _, _, err := toolset.deleteRunbook(context.Background(), nil, runbookDeleteInput{
		RunbookID: created.Runbook.ID, ConfirmName: "wrong",
	}); err == nil || !strings.Contains(err.Error(), "confirmName") {
		t.Fatalf("deleteRunbook() mismatch error = %v", err)
	}
	_, deleted, err := toolset.deleteRunbook(context.Background(), nil, runbookDeleteInput{
		RunbookID: created.Runbook.ID, ConfirmName: created.Runbook.Name,
	})
	if err != nil || !deleted.Deleted || deleted.RunbookID != created.Runbook.ID {
		t.Fatalf("deleteRunbook() = %#v, error = %v", deleted, err)
	}
	if _, err := manager.GetRun(context.Background(), started.Run.ID); err != nil {
		t.Fatalf("historical run was not preserved: %v", err)
	}
	eventMu.Lock()
	defer eventMu.Unlock()
	if got, want := strings.Join(runbookActions, ","), "create,delete"; got != want {
		t.Fatalf("runbook event actions = %q, want %q", got, want)
	}
}

func TestRunbookCreateToolUsesCanonicalServiceTargetValidation(t *testing.T) {
	t.Parallel()

	st, err := store.New(filepath.Join(t.TempDir(), "sentinel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	manager := runbook.NewManager(st, runbookTargetCatalog{services: []opsplane.ServiceStatus{
		{Name: "nginx"},
	}}, nil, 1, nil)
	t.Cleanup(func() { manager.Shutdown(context.Background()) })
	toolset := &tools{runbooks: manager}

	_, created, err := toolset.createRunbook(context.Background(), nil, runbookCreateInput{
		Name:          "Recover Nginx",
		TargetService: "nginx",
		Steps:         []store.OpsRunbookStep{{Type: "run", Title: "Check", Command: "true"}},
	})
	if err != nil {
		t.Fatalf("createRunbook: %v", err)
	}
	if created.Runbook.TargetService != "nginx" {
		t.Fatalf("targetService = %q, want nginx", created.Runbook.TargetService)
	}
	if _, _, err := toolset.createRunbook(context.Background(), nil, runbookCreateInput{
		Name:          "Duplicate",
		TargetService: "nginx",
		Steps:         []store.OpsRunbookStep{{Type: "run", Title: "Check", Command: "true"}},
	}); err == nil || !strings.Contains(err.Error(), "already associated") {
		t.Fatalf("duplicate target error = %v", err)
	}
}

func TestRunbookWaitStopsForHumanApproval(t *testing.T) {
	t.Parallel()
	st, err := store.New(filepath.Join(t.TempDir(), "sentinel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	manager := runbook.NewManager(st, nil, nil, 1, func(context.Context, string, ...string) (string, error) {
		return "", nil
	})
	t.Cleanup(func() { manager.Shutdown(context.Background()) })
	toolset := &tools{runbooks: manager}

	_, created, err := toolset.createRunbook(context.Background(), nil, runbookCreateInput{
		Name:  "Human gate",
		Steps: []store.OpsRunbookStep{{Type: "approval", Title: "Approve", Description: "Confirm deployment"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, started, err := toolset.runRunbook(context.Background(), nil, runbookRunInput{RunbookID: created.Runbook.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, waited, err := toolset.waitRunbook(context.Background(), nil, runbookWaitInput{RunID: started.Run.ID, TimeoutMS: 5_000})
	if err != nil {
		t.Fatal(err)
	}
	if waited.TimedOut || waited.Run.Status != store.OpsRunbookStatusWaitingApproval {
		t.Fatalf("waitRunbook() = %#v", waited)
	}
	if _, _, err := toolset.deleteRunbook(context.Background(), nil, runbookDeleteInput{
		RunbookID: created.Runbook.ID, ConfirmName: created.Runbook.Name,
	}); err == nil || !strings.Contains(err.Error(), "waiting-for-approval") {
		t.Fatalf("active delete error = %v", err)
	}
	if _, err := manager.Reject(context.Background(), started.Run.ID); err != nil {
		t.Fatal(err)
	}
}

// blockingRunRepo holds every run lookup until release is closed, standing in
// for the shared single-connection SQLite handle under load.
type blockingRunRepo struct {
	runbook.ManagerRepo
	release chan struct{}
	run     store.OpsRunbookRun
}

func (r *blockingRunRepo) GetOpsRunbookRun(ctx context.Context, _ string) (store.OpsRunbookRun, error) {
	select {
	case <-r.release:
		return r.run, nil
	case <-ctx.Done():
		return store.OpsRunbookRun{}, ctx.Err()
	}
}

func TestRunbookWaitReportsTimeoutWhenTheDeadlineCrossesAPoll(t *testing.T) {
	t.Parallel()
	repo := &blockingRunRepo{
		release: make(chan struct{}),
		run:     store.OpsRunbookRun{ID: "run_1", Status: store.OpsRunbookStatusRunning, TotalSteps: 2},
	}
	manager := runbook.NewManager(repo, nil, nil, 1, nil)
	t.Cleanup(func() { manager.Shutdown(context.Background()) })
	toolset := &tools{runbooks: manager}

	timer := time.AfterFunc(400*time.Millisecond, func() { close(repo.release) })
	t.Cleanup(func() { timer.Stop() })

	_, waited, err := toolset.waitRunbook(context.Background(), nil, runbookWaitInput{RunID: "run_1", TimeoutMS: 200})
	if err != nil {
		t.Fatalf("waitRunbook() error = %v, want a timedOut result", err)
	}
	if !waited.TimedOut || waited.Run.ID != "run_1" {
		t.Fatalf("waitRunbook() = %#v", waited)
	}
}

func TestRunbookWaitTimeoutAndCursor(t *testing.T) {
	t.Parallel()
	if got, truncated := trailingRunes("áβcdef", 3); got != "def" || !truncated {
		t.Fatalf("trailingRunes() = %q, %t", got, truncated)
	}
	if got := normalizeOutputLimit(maxRunOutputChars + 1); got != maxRunOutputChars {
		t.Fatalf("normalizeOutputLimit() = %d", got)
	}
	if defaultRunWait > maxToolWait || runPollInterval > time.Second {
		t.Fatal("invalid runbook wait constants")
	}
}
