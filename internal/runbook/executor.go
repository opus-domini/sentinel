package runbook

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// StepResult holds the outcome of a single executed step.
type StepResult struct {
	StepIndex int
	Title     string
	Type      string // "command", "check", "manual"
	Output    string
	Error     string
	Duration  time.Duration
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
	Type        string `json:"type"`
	Title       string `json:"title"`
	Command     string `json:"command,omitempty"`
	Check       string `json:"check,omitempty"`
	Description string `json:"description,omitempty"`
}

// Executor runs a sequence of runbook steps.
type Executor struct {
	runner      CommandRunner
	stepTimeout time.Duration
}

const defaultStepTimeout = 30 * time.Second

// NewExecutor creates an Executor. If runner is nil a default runner backed
// by exec.CommandContext is used. If stepTimeout is zero it defaults to 30s.
func NewExecutor(runner CommandRunner, stepTimeout time.Duration) *Executor {
	if runner == nil {
		runner = defaultRunner
	}
	if stepTimeout == 0 {
		stepTimeout = defaultStepTimeout
	}
	return &Executor{
		runner:      runner,
		stepTimeout: stepTimeout,
	}
}

// Execute runs steps sequentially. It stops on the first command/check
// failure and returns partial results together with an error. The beforeStep
// callback, when non-nil, is invoked before each step begins. The progress
// callback, when non-nil, is invoked after every completed step.
func (e *Executor) Execute(ctx context.Context, steps []Step, beforeStep BeforeStepFunc, progress ProgressFunc) ([]StepResult, error) {
	results := make([]StepResult, 0, len(steps))

	for i, step := range steps {
		if err := ctx.Err(); err != nil {
			return results, fmt.Errorf("step %d %q: %w", i, step.Title, err)
		}

		if beforeStep != nil {
			beforeStep(i, step)
		}

		stepCtx, cancel := context.WithTimeout(ctx, e.stepTimeout)
		start := time.Now()
		result := e.executeStep(stepCtx, i, step)
		result.Duration = time.Since(start)
		cancel()

		results = append(results, result)

		if progress != nil {
			progress(len(results), step.Title, result)
		}

		if result.Error != "" {
			return results, fmt.Errorf("step %d %q failed: %s", i, step.Title, result.Error)
		}
	}

	return results, nil
}

func (e *Executor) executeStep(ctx context.Context, index int, step Step) StepResult {
	result := StepResult{
		StepIndex: index,
		Title:     step.Title,
		Type:      step.Type,
	}

	switch step.Type {
	case "command":
		output, err := e.runner(ctx, "sh", "-c", step.Command)
		result.Output = output
		if err != nil {
			result.Error = err.Error()
		}
	case "check":
		output, err := e.runner(ctx, "sh", "-c", step.Check)
		result.Output = output
		if err != nil {
			result.Error = err.Error()
		}
	case "manual":
		result.Output = step.Description
	default:
		result.Error = fmt.Sprintf("unknown step type: %q", step.Type)
	}

	return result
}

func defaultRunner(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}
