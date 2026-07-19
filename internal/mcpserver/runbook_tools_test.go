package mcpserver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/runbook"
	"github.com/opus-domini/sentinel/internal/store"
)

func TestRunbookToolsLifecycle(t *testing.T) {
	t.Parallel()
	st, err := store.New(filepath.Join(t.TempDir(), "sentinel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	manager := runbook.NewManager(st, nil, 2)
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
}

func TestRunbookWaitStopsForHumanApproval(t *testing.T) {
	t.Parallel()
	st, err := store.New(filepath.Join(t.TempDir(), "sentinel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	manager := runbook.NewManager(st, nil, 1)
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
