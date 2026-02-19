package runbook

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockCall records a single invocation of the mock runner.
type mockCall struct {
	Name string
	Args []string
}

// mockRunner returns a CommandRunner that records calls and returns
// pre-configured results keyed by invocation index.
type mockRunner struct {
	mu      sync.Mutex
	calls   []mockCall
	results []mockResult
}

type mockResult struct {
	output string
	err    error
}

func (m *mockRunner) run(_ context.Context, name string, args ...string) (string, error) {
	m.mu.Lock()
	idx := len(m.calls)
	m.calls = append(m.calls, mockCall{Name: name, Args: args})
	m.mu.Unlock()

	if idx < len(m.results) {
		r := m.results[idx]
		return r.output, r.err
	}
	return "", nil
}

func (m *mockRunner) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func TestExecuteAllStepTypes(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		results: []mockResult{
			{output: "built ok\n"},
			{output: ""},
		},
	}

	steps := []Step{
		{Type: "command", Title: "Build", Command: "make build"},
		{Type: "check", Title: "Verify binary", Check: "test -f ./app"},
		{Type: "manual", Title: "Review logs", Description: "Check the output looks correct"},
	}

	exec := NewExecutor(mock.run, time.Minute)
	results, err := exec.Execute(context.Background(), steps, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	// command step
	if results[0].Type != "command" || results[0].Output != "built ok\n" || results[0].Error != "" {
		t.Errorf("command step: got type=%q output=%q error=%q", results[0].Type, results[0].Output, results[0].Error)
	}

	// check step
	if results[1].Type != "check" || results[1].Error != "" {
		t.Errorf("check step: got type=%q error=%q", results[1].Type, results[1].Error)
	}

	// manual step (no runner call)
	if results[2].Type != "manual" || results[2].Output != "Check the output looks correct" {
		t.Errorf("manual step: got type=%q output=%q", results[2].Type, results[2].Output)
	}

	// Only command and check should invoke the runner.
	if got := mock.callCount(); got != 2 {
		t.Errorf("runner called %d times, want 2", got)
	}

	// Duration should be populated for all steps.
	for i, r := range results {
		if r.Duration == 0 {
			t.Errorf("result[%d].Duration = 0, want > 0", i)
		}
	}
}

func TestCommandStepFailureStopsExecution(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		results: []mockResult{
			{output: "ok"},
			{output: "FAIL", err: fmt.Errorf("exit status 1")},
			{output: "should not run"},
		},
	}

	steps := []Step{
		{Type: "command", Title: "Step 1", Command: "echo ok"},
		{Type: "command", Title: "Step 2", Command: "false"},
		{Type: "command", Title: "Step 3", Command: "echo done"},
	}

	exec := NewExecutor(mock.run, time.Minute)
	results, err := exec.Execute(context.Background(), steps, nil)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2 (partial)", len(results))
	}

	if results[1].Error == "" {
		t.Error("second step should have error set")
	}

	if got := mock.callCount(); got != 2 {
		t.Errorf("runner called %d times, want 2 (third step should not run)", got)
	}
}

func TestCheckStepFailureStopsExecution(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		results: []mockResult{
			{output: "", err: fmt.Errorf("exit status 1")},
		},
	}

	steps := []Step{
		{Type: "check", Title: "Health check", Check: "curl -f http://localhost/health"},
		{Type: "command", Title: "Deploy", Command: "deploy.sh"},
	}

	exec := NewExecutor(mock.run, time.Minute)
	results, err := exec.Execute(context.Background(), steps, nil)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	if results[0].Error == "" {
		t.Error("check step should have error set")
	}

	if got := mock.callCount(); got != 1 {
		t.Errorf("runner called %d times, want 1", got)
	}
}

func TestContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	mock := &mockRunner{}

	steps := []Step{
		{Type: "command", Title: "Should not run", Command: "echo hello"},
	}

	exec := NewExecutor(mock.run, time.Minute)
	results, err := exec.Execute(ctx, steps, nil)

	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}

	if len(results) != 0 {
		t.Errorf("got %d results, want 0 (nothing should execute)", len(results))
	}

	if got := mock.callCount(); got != 0 {
		t.Errorf("runner called %d times, want 0", got)
	}
}

func TestProgressCallbackCalledForEachStep(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		results: []mockResult{
			{output: "a"},
			{output: "b"},
		},
	}

	steps := []Step{
		{Type: "command", Title: "First", Command: "echo a"},
		{Type: "command", Title: "Second", Command: "echo b"},
		{Type: "manual", Title: "Third", Description: "review"},
	}

	type progressEvent struct {
		completed int
		title     string
		stepType  string
	}

	var events []progressEvent
	progress := func(completed int, current string, result StepResult) {
		events = append(events, progressEvent{
			completed: completed,
			title:     current,
			stepType:  result.Type,
		})
	}

	exec := NewExecutor(mock.run, time.Minute)
	results, err := exec.Execute(context.Background(), steps, progress)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	if len(events) != 3 {
		t.Fatalf("progress called %d times, want 3", len(events))
	}

	want := []progressEvent{
		{completed: 1, title: "First", stepType: "command"},
		{completed: 2, title: "Second", stepType: "command"},
		{completed: 3, title: "Third", stepType: "manual"},
	}

	for i, w := range want {
		got := events[i]
		if got.completed != w.completed || got.title != w.title || got.stepType != w.stepType {
			t.Errorf("event[%d] = %+v, want %+v", i, got, w)
		}
	}
}

func TestEmptyStepsList(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{}
	exec := NewExecutor(mock.run, time.Minute)

	results, err := exec.Execute(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}

	results, err = exec.Execute(context.Background(), []Step{}, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestStepTimeout(t *testing.T) {
	t.Parallel()

	slowRunner := func(ctx context.Context, _ string, _ ...string) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
			return "should not reach", nil
		}
	}

	steps := []Step{
		{Type: "command", Title: "Slow step", Command: "sleep 10"},
	}

	exec := NewExecutor(slowRunner, 50*time.Millisecond)
	start := time.Now()
	results, err := exec.Execute(context.Background(), steps, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	if results[0].Error == "" {
		t.Error("timed-out step should have error set")
	}

	if elapsed > 2*time.Second {
		t.Errorf("execution took %v, expected it to respect the 50ms timeout", elapsed)
	}
}

func TestDefaultTimeoutAndRunner(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(nil, 0)
	if exec.stepTimeout != defaultStepTimeout {
		t.Errorf("stepTimeout = %v, want %v", exec.stepTimeout, defaultStepTimeout)
	}
	if exec.runner == nil {
		t.Fatal("runner should not be nil after NewExecutor(nil, 0)")
	}
}

func TestUnknownStepType(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{}
	exec := NewExecutor(mock.run, time.Minute)

	steps := []Step{
		{Type: "unknown", Title: "Mystery step"},
	}

	results, err := exec.Execute(context.Background(), steps, nil)
	if err == nil {
		t.Fatal("expected error for unknown step type, got nil")
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	if results[0].Error == "" {
		t.Error("unknown step type should have error set")
	}

	if got := mock.callCount(); got != 0 {
		t.Errorf("runner called %d times, want 0 for unknown step type", got)
	}
}
