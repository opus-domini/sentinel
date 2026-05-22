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
			{output: "script output\n"},
		},
	}

	steps := []Step{
		{Type: "run", Title: "Build", Command: "make build"},
		{Type: "script", Title: "Run script", Script: "#!/bin/sh\necho hello"},
		{Type: "approval", Title: "Review logs", Description: "Check the output looks correct"},
	}

	exec := NewExecutor(mock.run, time.Minute)
	res := exec.ExecuteFrom(context.Background(), steps, 0, nil, nil)

	// Approval step should pause execution.
	if !res.NeedsApproval {
		t.Fatal("expected NeedsApproval=true for approval step")
	}

	// We should have 3 results (run, script, approval).
	if len(res.Results) != 3 {
		t.Fatalf("got %d results, want 3", len(res.Results))
	}

	// run step
	if res.Results[0].Type != "run" || res.Results[0].Output != "built ok\n" || res.Results[0].Error != "" {
		t.Errorf("run step: got type=%q output=%q error=%q", res.Results[0].Type, res.Results[0].Output, res.Results[0].Error)
	}

	// script step
	if res.Results[1].Type != "script" || res.Results[1].Output != "script output\n" || res.Results[1].Error != "" {
		t.Errorf("script step: got type=%q output=%q error=%q", res.Results[1].Type, res.Results[1].Output, res.Results[1].Error)
	}

	// approval step (no runner call)
	if res.Results[2].Type != "approval" || res.Results[2].Output != "Check the output looks correct" {
		t.Errorf("approval step: got type=%q output=%q", res.Results[2].Type, res.Results[2].Output)
	}

	// Only run and script should invoke the runner.
	if got := mock.callCount(); got != 2 {
		t.Errorf("runner called %d times, want 2", got)
	}

	// Duration should be populated for all steps.
	for i, r := range res.Results {
		if r.Duration == 0 {
			t.Errorf("result[%d].Duration = 0, want > 0", i)
		}
	}
}

func TestRunStepFailureStopsExecution(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		results: []mockResult{
			{output: "ok"},
			{output: "FAIL", err: fmt.Errorf("exit status 1")},
			{output: "should not run"},
		},
	}

	steps := []Step{
		{Type: "run", Title: "Step 1", Command: "echo ok"},
		{Type: "run", Title: "Step 2", Command: "false"},
		{Type: "run", Title: "Step 3", Command: "echo done"},
	}

	exec := NewExecutor(mock.run, time.Minute)
	results, err := exec.Execute(context.Background(), steps, nil, nil)

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

func TestContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	mock := &mockRunner{}

	steps := []Step{
		{Type: "run", Title: "Should not run", Command: "echo hello"},
	}

	exec := NewExecutor(mock.run, time.Minute)
	results, err := exec.Execute(ctx, steps, nil, nil)

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
		{Type: "run", Title: "First", Command: "echo a"},
		{Type: "run", Title: "Second", Command: "echo b"},
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
	results, err := exec.Execute(context.Background(), steps, nil, progress)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	if len(events) != 2 {
		t.Fatalf("progress called %d times, want 2", len(events))
	}

	want := []progressEvent{
		{completed: 1, title: "First", stepType: "run"},
		{completed: 2, title: "Second", stepType: "run"},
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

	results, err := exec.Execute(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}

	results, err = exec.Execute(context.Background(), []Step{}, nil, nil)
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
		{Type: "run", Title: "Slow step", Command: "sleep 10"},
	}

	exec := NewExecutor(slowRunner, 50*time.Millisecond)
	start := time.Now()
	results, err := exec.Execute(context.Background(), steps, nil, nil)
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

func TestContextCancelledBetweenSteps(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	callCount := 0
	runner := func(_ context.Context, _ string, _ ...string) (string, error) {
		callCount++
		if callCount == 1 {
			cancel() // cancel after first step completes
		}
		return "ok", nil
	}

	steps := []Step{
		{Type: "run", Title: "Step 1", Command: "echo first"},
		{Type: "run", Title: "Step 2", Command: "echo second"},
	}

	exec := NewExecutor(runner, time.Minute)
	results, _ := exec.Execute(ctx, steps, nil, nil)

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (partial - only first step)", len(results))
	}
	if results[0].Title != "Step 1" {
		t.Errorf("result[0].Title = %q, want Step 1", results[0].Title)
	}
}

func TestBeforeStepCallback(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		results: []mockResult{
			{output: "ok"},
		},
	}

	steps := []Step{
		{Type: "run", Title: "Build", Command: "make"},
	}

	type beforeCall struct {
		index int
		title string
	}
	var calls []beforeCall
	beforeStep := func(index int, step Step) {
		calls = append(calls, beforeCall{index: index, title: step.Title})
	}

	exec := NewExecutor(mock.run, time.Minute)
	results, err := exec.Execute(context.Background(), steps, beforeStep, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if len(calls) != 1 {
		t.Fatalf("beforeStep called %d times, want 1", len(calls))
	}
	if calls[0].index != 0 || calls[0].title != "Build" {
		t.Errorf("beforeStep call = %+v, want {index:0, title:Build}", calls[0])
	}
}

func TestUnknownStepType(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{}
	exec := NewExecutor(mock.run, time.Minute)

	steps := []Step{
		{Type: "unknown", Title: "Mystery step"},
	}

	results, err := exec.Execute(context.Background(), steps, nil, nil)
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

func TestExecuteWithParameterSubstitution(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		results: []mockResult{
			{output: "deployed"},
			{output: "healthy"},
		},
	}

	steps := []Step{
		{Type: "run", Title: "Deploy", Command: "deploy.sh --host={{HOST}} --env={{ENV}}"},
		{Type: "run", Title: "Health check", Command: "curl -f http://{{HOST}}:{{PORT}}/health"},
	}

	params := map[string]string{
		"HOST": "server.example.com",
		"ENV":  "production",
		"PORT": "8080",
	}

	exec := NewExecutor(mock.run, time.Minute, params)
	results, err := exec.Execute(context.Background(), steps, nil, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	// Verify the runner received substituted commands.
	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.calls) != 2 {
		t.Fatalf("runner called %d times, want 2", len(mock.calls))
	}

	// Run step should have substituted host and env.
	cmdArgs := mock.calls[0].Args
	if len(cmdArgs) < 2 {
		t.Fatalf("command args = %v, want at least 2 elements", cmdArgs)
	}
	cmd := cmdArgs[1] // args[0] is "-c", args[1] is the command
	wantCmd := "deploy.sh --host='server.example.com' --env='production'"
	if cmd != wantCmd {
		t.Errorf("command = %q, want %q", cmd, wantCmd)
	}

	// Second run step should have substituted host and port.
	checkArgs := mock.calls[1].Args
	checkCmd := checkArgs[1]
	wantCheck := "curl -f http://'server.example.com':'8080'/health"
	if checkCmd != wantCheck {
		t.Errorf("check = %q, want %q", checkCmd, wantCheck)
	}
}

func TestExecuteWithoutParams(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		results: []mockResult{
			{output: "ok"},
		},
	}

	steps := []Step{
		{Type: "run", Title: "Build", Command: "make build"},
	}

	// No params — backward compatible.
	exec := NewExecutor(mock.run, time.Minute)
	results, err := exec.Execute(context.Background(), steps, nil, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	cmd := mock.calls[0].Args[1]
	if cmd != "make build" {
		t.Errorf("command = %q, want %q (unchanged)", cmd, "make build")
	}
}

func TestExecuteShellEscapingInParams(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		results: []mockResult{
			{output: "ok"},
		},
	}

	steps := []Step{
		{Type: "run", Title: "Echo", Command: "echo {{MSG}}"},
	}

	// Value with shell metacharacters should be safely escaped.
	params := map[string]string{
		"MSG": "$(whoami); rm -rf /",
	}

	exec := NewExecutor(mock.run, time.Minute, params)
	_, err := exec.Execute(context.Background(), steps, nil, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	cmd := mock.calls[0].Args[1]
	// The value should be wrapped in single quotes, preventing shell expansion.
	want := "echo '$(whoami); rm -rf /'"
	if cmd != want {
		t.Errorf("command = %q, want %q", cmd, want)
	}
}

func TestScriptStepExecution(t *testing.T) {
	t.Parallel()

	// Track that the runner receives a temp file path.
	mock := &mockRunner{
		results: []mockResult{
			{output: "script ran"},
		},
	}

	steps := []Step{
		{Type: "script", Title: "Run script", Script: "#!/bin/sh\necho hello"},
	}

	exec := NewExecutor(mock.run, time.Minute)
	results, err := exec.Execute(context.Background(), steps, nil, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	if results[0].Type != "script" {
		t.Errorf("type = %q, want script", results[0].Type)
	}

	if results[0].Output != "script ran" {
		t.Errorf("output = %q, want 'script ran'", results[0].Output)
	}

	// Verify the runner was called with "sh" and a temp file path.
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.calls) != 1 {
		t.Fatalf("runner called %d times, want 1", len(mock.calls))
	}
	if mock.calls[0].Name != "sh" {
		t.Errorf("runner name = %q, want sh", mock.calls[0].Name)
	}
	// The args should contain the temp file path (not "-c").
	if len(mock.calls[0].Args) != 1 {
		t.Fatalf("runner args = %v, want 1 element (temp file path)", mock.calls[0].Args)
	}
}

func TestScriptStepParameterSubstitution(t *testing.T) {
	t.Parallel()

	var capturedArgs []string
	runner := func(_ context.Context, name string, args ...string) (string, error) {
		capturedArgs = args
		return "ok", nil
	}

	steps := []Step{
		{Type: "script", Title: "Deploy", Script: "#!/bin/sh\ndeploy.sh {{HOST}}"},
	}

	params := map[string]string{"HOST": "prod.example.com"}
	exec := NewExecutor(runner, time.Minute, params)
	_, err := exec.Execute(context.Background(), steps, nil, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// The temp file should exist; it's cleaned up by the executor.
	if len(capturedArgs) != 1 {
		t.Fatalf("runner args = %v, want 1 element", capturedArgs)
	}
}

func TestApprovalStepPausesExecution(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		results: []mockResult{
			{output: "ok"},
		},
	}

	steps := []Step{
		{Type: "run", Title: "Build", Command: "make"},
		{Type: "approval", Title: "Approve deploy", Description: "Please review"},
		{Type: "run", Title: "Deploy", Command: "deploy.sh"},
	}

	exec := NewExecutor(mock.run, time.Minute)
	res := exec.ExecuteFrom(context.Background(), steps, 0, nil, nil)

	if !res.NeedsApproval {
		t.Fatal("expected NeedsApproval=true")
	}
	if res.PausedAtStep != 1 {
		t.Errorf("PausedAtStep = %d, want 1", res.PausedAtStep)
	}
	if len(res.Results) != 2 {
		t.Fatalf("got %d results, want 2 (build + approval)", len(res.Results))
	}

	// Verify approval step has the right output.
	if res.Results[1].Output != "Please review" {
		t.Errorf("approval output = %q, want 'Please review'", res.Results[1].Output)
	}
	if !res.Results[1].NeedsApproval {
		t.Error("approval step result should have NeedsApproval=true")
	}

	// Deploy step should not have run.
	if got := mock.callCount(); got != 1 {
		t.Errorf("runner called %d times, want 1 (deploy should not run)", got)
	}

	// Error should be nil (approval is not a failure).
	if res.Err() != nil {
		t.Errorf("Err() = %v, want nil", res.Err())
	}
}

func TestApprovalResumeExecution(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		results: []mockResult{
			{output: "deployed"},
		},
	}

	steps := []Step{
		{Type: "run", Title: "Build", Command: "make"},
		{Type: "approval", Title: "Approve deploy", Description: "Please review"},
		{Type: "run", Title: "Deploy", Command: "deploy.sh"},
	}

	exec := NewExecutor(mock.run, time.Minute)

	// Resume from step after approval (step 2).
	res := exec.ExecuteFrom(context.Background(), steps, 2, nil, nil)

	if res.NeedsApproval {
		t.Fatal("expected NeedsApproval=false after resume")
	}
	if len(res.Results) != 1 {
		t.Fatalf("got %d results, want 1 (deploy only)", len(res.Results))
	}
	if res.Results[0].Title != "Deploy" {
		t.Errorf("result[0].Title = %q, want Deploy", res.Results[0].Title)
	}
	if res.Err() != nil {
		t.Errorf("Err() = %v, want nil", res.Err())
	}
}

func TestContinueOnError(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		results: []mockResult{
			{output: "FAIL", err: fmt.Errorf("exit status 1")},
			{output: "ok"},
		},
	}

	steps := []Step{
		{Type: "run", Title: "Flaky Step", Command: "maybe-fail", ContinueOnError: true},
		{Type: "run", Title: "Next Step", Command: "echo ok"},
	}

	exec := NewExecutor(mock.run, time.Minute)
	results, err := exec.Execute(context.Background(), steps, nil, nil)

	if err != nil {
		t.Fatalf("Execute returned error: %v (expected nil due to continueOnError)", err)
	}

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	// First step should have error set but execution continued.
	if results[0].Error == "" {
		t.Error("first step should have error set")
	}

	// Second step should have succeeded.
	if results[1].Error != "" {
		t.Errorf("second step error = %q, want empty", results[1].Error)
	}
}

func TestContinueOnErrorFollowedByFailure(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		results: []mockResult{
			{output: "FAIL", err: fmt.Errorf("exit status 1")},
			{output: "FAIL2", err: fmt.Errorf("exit status 2")},
		},
	}

	steps := []Step{
		{Type: "run", Title: "Soft fail", Command: "maybe-fail", ContinueOnError: true},
		{Type: "run", Title: "Hard fail", Command: "must-fail"},
	}

	exec := NewExecutor(mock.run, time.Minute)
	results, err := exec.Execute(context.Background(), steps, nil, nil)

	if err == nil {
		t.Fatal("expected error from hard fail step")
	}

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestPerStepTimeout(t *testing.T) {
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
		{Type: "run", Title: "Slow step", Command: "sleep 10", Timeout: 1}, // 1 second per-step timeout
	}

	// Executor default is 10 minutes but per-step overrides to 1s.
	exec := NewExecutor(slowRunner, 10*time.Minute)
	start := time.Now()
	results, err := exec.Execute(context.Background(), steps, nil, nil)
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

	if elapsed > 5*time.Second {
		t.Errorf("execution took %v, expected it to respect the 1s per-step timeout", elapsed)
	}
}

func TestRetryOnFailure(t *testing.T) {
	t.Parallel()

	callIdx := 0
	runner := func(_ context.Context, _ string, _ ...string) (string, error) {
		callIdx++
		if callIdx < 3 {
			return "fail", fmt.Errorf("transient error")
		}
		return "ok", nil
	}

	steps := []Step{
		{Type: "run", Title: "Flaky", Command: "flaky.sh", Retries: 3, RetryDelay: 0},
	}

	exec := NewExecutor(runner, time.Minute)
	results, err := exec.Execute(context.Background(), steps, nil, nil)

	if err != nil {
		t.Fatalf("Execute returned error: %v (expected nil after successful retry)", err)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	if results[0].Error != "" {
		t.Errorf("result error = %q, want empty (retry succeeded)", results[0].Error)
	}

	if results[0].Retries != 2 {
		t.Errorf("retries = %d, want 2", results[0].Retries)
	}
}

func TestRetryExhausted(t *testing.T) {
	t.Parallel()

	runner := func(_ context.Context, _ string, _ ...string) (string, error) {
		return "fail", fmt.Errorf("persistent error")
	}

	steps := []Step{
		{Type: "run", Title: "Always fails", Command: "fail.sh", Retries: 2, RetryDelay: 0},
	}

	exec := NewExecutor(runner, time.Minute)
	results, err := exec.Execute(context.Background(), steps, nil, nil)

	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	if results[0].Error == "" {
		t.Error("result should have error after all retries exhausted")
	}

	if results[0].Retries != 2 {
		t.Errorf("retries = %d, want 2", results[0].Retries)
	}
}

func TestRetryWithDelay(t *testing.T) {
	t.Parallel()

	callIdx := 0
	runner := func(_ context.Context, _ string, _ ...string) (string, error) {
		callIdx++
		if callIdx < 2 {
			return "fail", fmt.Errorf("error")
		}
		return "ok", nil
	}

	steps := []Step{
		{Type: "run", Title: "Retry with delay", Command: "test.sh", Retries: 2, RetryDelay: 1},
	}

	exec := NewExecutor(runner, time.Minute)
	start := time.Now()
	results, err := exec.Execute(context.Background(), steps, nil, nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	// Should have waited at least ~1 second for the retry delay.
	if elapsed < 500*time.Millisecond {
		t.Errorf("execution took %v, expected at least ~1s delay for retry", elapsed)
	}
}
