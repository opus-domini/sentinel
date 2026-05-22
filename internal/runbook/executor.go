package runbook

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

// StepResult holds the outcome of a single executed step.
type StepResult struct {
	StepIndex     int
	Title         string
	Type          string // "run", "script", "approval"
	Output        string
	Error         string
	Duration      time.Duration
	NeedsApproval bool // true when an approval step pauses execution
	Retries       int  // number of retries attempted
}

// BeforeStepFunc is called before each step begins execution.
type BeforeStepFunc func(stepIndex int, step Step)

// ProgressFunc is called after each step completes with the count of
// completed steps, the title of the step just finished, and its result.
type ProgressFunc func(completedSteps int, currentStep string, result StepResult)

// CommandRunner executes an external command and returns its combined output.
type CommandRunner func(ctx context.Context, name string, args ...string) (string, error)

// Step describes a single runbook step to execute.
type Step struct {
	Type            string `json:"type"`
	Title           string `json:"title"`
	Command         string `json:"command,omitempty"`
	Script          string `json:"script,omitempty"`
	Description     string `json:"description,omitempty"`
	ContinueOnError bool   `json:"continueOnError,omitempty"`
	Timeout         int    `json:"timeout,omitempty"`
	Retries         int    `json:"retries,omitempty"`
	RetryDelay      int    `json:"retryDelay,omitempty"`
}

// ExecuteResult holds the outcome of an Execute call, including whether
// the execution was paused waiting for approval.
type ExecuteResult struct {
	Results       []StepResult
	NeedsApproval bool
	PausedAtStep  int   // index of the approval step that paused execution
	CtxErr        error // non-nil when execution was aborted by context cancellation/timeout
}

// Executor runs a sequence of runbook steps.
type Executor struct {
	runner      CommandRunner
	stepTimeout time.Duration
	params      map[string]string // substituted into commands before execution
}

const (
	stepTypeRun      = "run"
	stepTypeScript   = "script"
	stepTypeApproval = "approval"

	defaultStepTimeout = 30 * time.Second
	defaultRetryDelay  = 2 * time.Second
)

// NewExecutor creates an Executor. If runner is nil a default runner backed
// by exec.CommandContext is used. If stepTimeout is zero it defaults to 30s.
// The optional params map is used to substitute {{PARAM}} placeholders in
// step commands before execution.
func NewExecutor(runner CommandRunner, stepTimeout time.Duration, params ...map[string]string) *Executor {
	if runner == nil {
		runner = defaultRunner
	}
	if stepTimeout == 0 {
		stepTimeout = defaultStepTimeout
	}
	var p map[string]string
	if len(params) > 0 {
		p = params[0]
	}
	return &Executor{
		runner:      runner,
		stepTimeout: stepTimeout,
		params:      p,
	}
}

// Execute runs steps sequentially. It stops on the first command/script
// failure (unless ContinueOnError is set) and returns partial results
// together with an error. When an approval step is encountered, execution
// pauses and the result indicates approval is needed.
// The beforeStep callback, when non-nil, is invoked before each step begins.
// The progress callback, when non-nil, is invoked after every completed step.
func (e *Executor) Execute(ctx context.Context, steps []Step, beforeStep BeforeStepFunc, progress ProgressFunc) ([]StepResult, error) {
	res := e.ExecuteFrom(ctx, steps, 0, beforeStep, progress)
	return res.Results, res.Err()
}

// ExecuteFrom runs steps starting from the given index. This allows
// resuming execution after an approval step has been approved.
func (e *Executor) ExecuteFrom(ctx context.Context, steps []Step, startFrom int, beforeStep BeforeStepFunc, progress ProgressFunc) ExecuteResult {
	results := make([]StepResult, 0, len(steps))

	for i := startFrom; i < len(steps); i++ {
		step := steps[i]
		if err := ctx.Err(); err != nil {
			return ExecuteResult{
				Results: results,
				CtxErr:  fmt.Errorf("step %d %q: %w", i, step.Title, err),
			}
		}

		if beforeStep != nil {
			beforeStep(i, step)
		}

		timeout := e.stepTimeout
		if step.Timeout > 0 {
			timeout = time.Duration(step.Timeout) * time.Second
		}

		stepCtx, cancel := context.WithTimeout(ctx, timeout)
		start := time.Now()
		result := e.executeStepWithRetries(stepCtx, i, step)
		result.Duration = time.Since(start)
		cancel()

		results = append(results, result)

		if progress != nil {
			progress(len(results), step.Title, result)
		}

		// Approval step pauses execution.
		if result.NeedsApproval {
			return ExecuteResult{
				Results:       results,
				NeedsApproval: true,
				PausedAtStep:  i,
			}
		}

		if result.Error != "" && !step.ContinueOnError {
			return ExecuteResult{
				Results: results,
			}
		}
	}

	return ExecuteResult{
		Results: results,
	}
}

// Err returns an error if the last result has an error and was not a
// continue-on-error step. Returns nil for successful or approval-paused runs.
func (r ExecuteResult) Err() error {
	if r.NeedsApproval {
		return nil
	}
	if r.CtxErr != nil {
		return r.CtxErr
	}
	if len(r.Results) == 0 {
		return nil
	}
	last := r.Results[len(r.Results)-1]
	if last.Error != "" {
		return fmt.Errorf("step %d %q failed: %s", last.StepIndex, last.Title, last.Error)
	}
	return nil
}

func (e *Executor) executeStepWithRetries(ctx context.Context, index int, step Step) StepResult {
	result := e.executeStep(ctx, index, step)

	retries := step.Retries
	if retries <= 0 || result.Error == "" || step.Type == stepTypeApproval {
		return result
	}

	delay := defaultRetryDelay
	if step.RetryDelay > 0 {
		delay = time.Duration(step.RetryDelay) * time.Second
	}

	for attempt := 1; attempt <= retries; attempt++ {
		slog.Info("retrying step", "step", index, "title", step.Title, "attempt", attempt, "maxRetries", retries)

		select {
		case <-ctx.Done():
			result.Retries = attempt
			return result
		case <-time.After(delay):
		}

		result = e.executeStep(ctx, index, step)
		result.Retries = attempt

		if result.Error == "" {
			return result
		}
	}

	return result
}

func (e *Executor) executeStep(ctx context.Context, index int, step Step) StepResult {
	result := StepResult{
		StepIndex: index,
		Title:     step.Title,
		Type:      step.Type,
	}

	switch step.Type {
	case stepTypeRun:
		cmd := SubstituteParams(step.Command, e.params)
		output, err := e.runner(ctx, "sh", "-c", cmd)
		result.Output = output
		if err != nil {
			result.Error = err.Error()
		}
	case stepTypeScript:
		output, err := e.executeScript(ctx, step)
		result.Output = output
		if err != nil {
			result.Error = err.Error()
		}
	case stepTypeApproval:
		result.Output = step.Description
		result.NeedsApproval = true
	default:
		result.Error = fmt.Sprintf("unknown step type: %q", step.Type)
	}

	return result
}

func (e *Executor) executeScript(ctx context.Context, step Step) (string, error) {
	script := SubstituteParams(step.Script, e.params)

	tmpFile, err := os.CreateTemp("", "sentinel-step-*.sh")
	if err != nil {
		return "", fmt.Errorf("create temp script: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.WriteString(script); err != nil {
		_ = tmpFile.Close()
		return "", fmt.Errorf("write temp script: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close temp script: %w", err)
	}
	if err := os.Chmod(tmpFile.Name(), 0700); err != nil { //nolint:gosec // G302: script needs execute permission
		return "", fmt.Errorf("chmod temp script: %w", err)
	}

	return e.runner(ctx, "sh", tmpFile.Name())
}

func defaultRunner(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}
