package mcpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/opus-domini/sentinel/internal/runbook"
	"github.com/opus-domini/sentinel/internal/store"
)

const (
	defaultRunOutputChars = 4_000
	maxRunOutputChars     = 32_768
	defaultRunListLimit   = 20
	maxRunListLimit       = 50
	defaultRunWait        = 10 * time.Second
	runPollInterval       = 200 * time.Millisecond
)

type runbookListInput struct{}

type runbookSummary struct {
	ID               string                   `json:"id"`
	Name             string                   `json:"name"`
	Description      string                   `json:"description"`
	Enabled          bool                     `json:"enabled"`
	TotalSteps       int                      `json:"totalSteps"`
	Parameters       []store.RunbookParameter `json:"parameters"`
	RequiresApproval bool                     `json:"requiresApproval"`
}

type runbookListOutput struct {
	Runbooks []runbookSummary `json:"runbooks"`
}

type runbookIDInput struct {
	RunbookID string `json:"runbookId" jsonschema:"stable runbook ID"`
}

type runbookGetOutput struct {
	Runbook store.OpsRunbook `json:"runbook"`
}

type runbookCreateInput struct {
	Name        string                   `json:"name" jsonschema:"runbook name"`
	Description string                   `json:"description,omitempty" jsonschema:"purpose and operational context"`
	Steps       []store.OpsRunbookStep   `json:"steps" jsonschema:"ordered run, script, or approval steps"`
	Parameters  []store.RunbookParameter `json:"parameters,omitempty" jsonschema:"typed parameters accepted by this runbook"`
	Enabled     *bool                    `json:"enabled,omitempty" jsonschema:"whether the runbook can be executed; defaults to true"`
	WebhookURL  string                   `json:"webhookURL,omitempty" jsonschema:"optional HTTP or HTTPS completion webhook"`
}

type runbookCreateOutput struct {
	Runbook       store.OpsRunbook       `json:"runbook"`
	ShellWarnings []runbook.ShellWarning `json:"shellWarnings,omitempty"`
}

type runbookDeleteInput struct {
	RunbookID   string `json:"runbookId" jsonschema:"stable runbook ID"`
	ConfirmName string `json:"confirmName" jsonschema:"exact persisted runbook name required for destructive confirmation"`
}

type runbookDeleteOutput struct {
	Deleted          bool   `json:"deleted"`
	RunbookID        string `json:"runbookId"`
	Name             string `json:"name"`
	DeletedSchedules int64  `json:"deletedSchedules"`
}

type runbookRunInput struct {
	RunbookID  string            `json:"runbookId" jsonschema:"stable runbook ID"`
	Parameters map[string]string `json:"parameters,omitempty" jsonschema:"parameter values keyed by parameter name"`
}

type runbookRunLookupInput struct {
	RunID           string `json:"runId" jsonschema:"run ID returned by runbook_run"`
	OutputTailChars int    `json:"outputTailChars,omitempty" jsonschema:"maximum trailing characters kept per step output; defaults to 4000 and caps at 32768"`
}

type runbookListRunsInput struct {
	Limit           int `json:"limit,omitempty" jsonschema:"maximum recent runs; defaults to 20 and caps at 50"`
	OutputTailChars int `json:"outputTailChars,omitempty" jsonschema:"maximum trailing characters kept per step output; defaults to 4000 and caps at 32768"`
}

type runbookWaitInput struct {
	RunID               string `json:"runId" jsonschema:"run ID returned by runbook_run"`
	AfterCompletedSteps *int   `json:"afterCompletedSteps,omitempty" jsonschema:"return after completedSteps becomes greater than this value"`
	TimeoutMS           int    `json:"timeoutMs,omitempty" jsonschema:"long-poll timeout in milliseconds; defaults to 10000 and caps at 20000"`
	OutputTailChars     int    `json:"outputTailChars,omitempty" jsonschema:"maximum trailing characters kept per step output; defaults to 4000 and caps at 32768"`
}

type runbookStepResultOutput struct {
	StepIndex       int    `json:"stepIndex"`
	Title           string `json:"title"`
	Type            string `json:"type"`
	Output          string `json:"output"`
	OutputTruncated bool   `json:"outputTruncated"`
	Error           string `json:"error"`
	DurationMS      int64  `json:"durationMs"`
}

type runbookRunOutput struct {
	ID             string                    `json:"id"`
	RunbookID      string                    `json:"runbookId"`
	RunbookName    string                    `json:"runbookName"`
	Status         string                    `json:"status"`
	TotalSteps     int                       `json:"totalSteps"`
	CompletedSteps int                       `json:"completedSteps"`
	CurrentStep    string                    `json:"currentStep"`
	Error          string                    `json:"error"`
	StepResults    []runbookStepResultOutput `json:"stepResults"`
	ParametersUsed map[string]string         `json:"parametersUsed"`
	CreatedAt      string                    `json:"createdAt"`
	StartedAt      string                    `json:"startedAt,omitempty"`
	FinishedAt     string                    `json:"finishedAt,omitempty"`
}

type runbookRunResult struct {
	Run runbookRunOutput `json:"run"`
}

type runbookListRunsOutput struct {
	Runs []runbookRunOutput `json:"runs"`
}

type runbookWaitOutput struct {
	Run      runbookRunOutput `json:"run"`
	TimedOut bool             `json:"timedOut"`
}

func (t *tools) registerRunbookTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "runbook_list",
		Description: "List Sentinel runbooks with execution and approval metadata.",
		Annotations: closedWorldAnnotations(true, false, true),
	}, t.listRunbooks)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "runbook_get",
		Description: "Get the complete definition of one Sentinel runbook.",
		Annotations: closedWorldAnnotations(true, false, true),
	}, t.getRunbook)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "runbook_create",
		Description: "Validate and create a Sentinel runbook without executing it.",
		Annotations: closedWorldAnnotations(false, false, false),
	}, t.createRunbook)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "runbook_delete",
		Description: "Delete a Sentinel runbook and its schedules after exact-name confirmation; historical runs are preserved and active runbooks are refused.",
		Annotations: closedWorldAnnotations(false, true, false),
	}, t.deleteRunbook)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "runbook_run",
		Description: "Start a runbook with typed parameters. Approval steps pause for a human and cannot be approved through MCP.",
		Annotations: closedWorldAnnotations(false, true, false),
	}, t.runRunbook)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "runbook_get_run",
		Description: "Get one runbook execution with bounded trailing step output.",
		Annotations: closedWorldAnnotations(true, false, true),
	}, t.getRunbookRun)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "runbook_wait",
		Description: "Wait up to 20 seconds for a run to finish, reach human approval, or advance beyond a completed-step cursor.",
		Annotations: closedWorldAnnotations(true, false, true),
	}, t.waitRunbook)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "runbook_list_runs",
		Description: "List recent runbook executions with bounded trailing step output.",
		Annotations: closedWorldAnnotations(true, false, true),
	}, t.listRunbookRuns)
}

func (t *tools) listRunbooks(ctx context.Context, _ *mcp.CallToolRequest, _ runbookListInput) (*mcp.CallToolResult, runbookListOutput, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	items, err := t.runbooks.List(ctx)
	if err != nil {
		return nil, runbookListOutput{}, toolError("list runbooks", err)
	}
	output := runbookListOutput{Runbooks: make([]runbookSummary, 0, len(items))}
	for _, item := range items {
		output.Runbooks = append(output.Runbooks, summarizeRunbook(item))
	}
	return nil, output, nil
}

func (t *tools) getRunbook(ctx context.Context, _ *mcp.CallToolRequest, input runbookIDInput) (*mcp.CallToolResult, runbookGetOutput, error) {
	id, err := requiredID(input.RunbookID, "runbookId")
	if err != nil {
		return nil, runbookGetOutput{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := t.runbooks.Get(ctx, id)
	if err != nil {
		return nil, runbookGetOutput{}, runbookToolError("get runbook", err)
	}
	return nil, runbookGetOutput{Runbook: item}, nil
}

func (t *tools) createRunbook(ctx context.Context, _ *mcp.CallToolRequest, input runbookCreateInput) (*mcp.CallToolResult, runbookCreateOutput, error) {
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	created, warnings, err := t.runbooks.Create(ctx, store.OpsRunbookWrite{
		Name:        input.Name,
		Description: input.Description,
		Steps:       input.Steps,
		Parameters:  input.Parameters,
		Enabled:     enabled,
		WebhookURL:  input.WebhookURL,
	})
	if err != nil {
		return nil, runbookCreateOutput{}, runbookToolError("create runbook", err)
	}
	return nil, runbookCreateOutput{Runbook: created, ShellWarnings: warnings}, nil
}

func (t *tools) deleteRunbook(ctx context.Context, _ *mcp.CallToolRequest, input runbookDeleteInput) (*mcp.CallToolResult, runbookDeleteOutput, error) {
	id, err := requiredID(input.RunbookID, "runbookId")
	if err != nil {
		return nil, runbookDeleteOutput{}, err
	}
	if input.ConfirmName == "" {
		return nil, runbookDeleteOutput{}, errors.New("confirmName is required and must exactly match the persisted runbook name")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	deleted, err := t.runbooks.Delete(ctx, id, input.ConfirmName)
	if err != nil {
		return nil, runbookDeleteOutput{}, runbookToolError("delete runbook", err)
	}
	return nil, runbookDeleteOutput{
		Deleted:          true,
		RunbookID:        deleted.ID,
		Name:             deleted.Name,
		DeletedSchedules: deleted.DeletedSchedules,
	}, nil
}

func (t *tools) runRunbook(ctx context.Context, _ *mcp.CallToolRequest, input runbookRunInput) (*mcp.CallToolResult, runbookRunResult, error) {
	id, err := requiredID(input.RunbookID, "runbookId")
	if err != nil {
		return nil, runbookRunResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	run, err := t.runbooks.Start(ctx, id, input.Parameters, "mcp")
	if err != nil {
		return nil, runbookRunResult{}, runbookToolError("run runbook", err)
	}
	return nil, runbookRunResult{Run: projectRun(run, defaultRunOutputChars)}, nil
}

func (t *tools) getRunbookRun(ctx context.Context, _ *mcp.CallToolRequest, input runbookRunLookupInput) (*mcp.CallToolResult, runbookRunResult, error) {
	id, err := requiredID(input.RunID, "runId")
	if err != nil {
		return nil, runbookRunResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	run, err := t.runbooks.GetRun(ctx, id)
	if err != nil {
		return nil, runbookRunResult{}, runbookToolError("get runbook run", err)
	}
	return nil, runbookRunResult{Run: projectRun(run, normalizeOutputLimit(input.OutputTailChars))}, nil
}

func (t *tools) listRunbookRuns(ctx context.Context, _ *mcp.CallToolRequest, input runbookListRunsInput) (*mcp.CallToolResult, runbookListRunsOutput, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = defaultRunListLimit
	}
	if limit > maxRunListLimit {
		limit = maxRunListLimit
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	runs, err := t.runbooks.ListRuns(ctx, limit)
	if err != nil {
		return nil, runbookListRunsOutput{}, toolError("list runbook runs", err)
	}
	output := runbookListRunsOutput{Runs: make([]runbookRunOutput, 0, len(runs))}
	outputLimit := normalizeOutputLimit(input.OutputTailChars)
	for _, item := range runs {
		output.Runs = append(output.Runs, projectRun(item, outputLimit))
	}
	return nil, output, nil
}

func (t *tools) waitRunbook(ctx context.Context, _ *mcp.CallToolRequest, input runbookWaitInput) (*mcp.CallToolResult, runbookWaitOutput, error) {
	id, err := requiredID(input.RunID, "runId")
	if err != nil {
		return nil, runbookWaitOutput{}, err
	}
	if input.AfterCompletedSteps != nil && *input.AfterCompletedSteps < 0 {
		return nil, runbookWaitOutput{}, errors.New("afterCompletedSteps must not be negative")
	}
	timeout := defaultRunWait
	if input.TimeoutMS > 0 {
		timeout = time.Duration(input.TimeoutMS) * time.Millisecond
	}
	if timeout > maxToolWait {
		timeout = maxToolWait
	}
	if timeout < runPollInterval {
		timeout = runPollInterval
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(runPollInterval)
	defer ticker.Stop()

	var latest store.OpsRunbookRun
	for {
		item, getErr := t.runbooks.GetRun(waitCtx, id)
		if getErr != nil {
			return nil, runbookWaitOutput{}, runbookToolError("wait for runbook run", getErr)
		}
		latest = item
		if runbook.IsTerminalStatus(item.Status) || runbook.IsWaitingApproval(item.Status) ||
			(input.AfterCompletedSteps != nil && item.CompletedSteps > *input.AfterCompletedSteps) {
			return nil, runbookWaitOutput{Run: projectRun(item, normalizeOutputLimit(input.OutputTailChars))}, nil
		}
		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return nil, runbookWaitOutput{Run: projectRun(latest, normalizeOutputLimit(input.OutputTailChars)), TimedOut: true}, nil
			}
			return nil, runbookWaitOutput{}, waitCtx.Err()
		case <-ticker.C:
		}
	}
}

func summarizeRunbook(item store.OpsRunbook) runbookSummary {
	requiresApproval := false
	for _, step := range item.Steps {
		if step.Type == "approval" {
			requiresApproval = true
			break
		}
	}
	return runbookSummary{
		ID:               item.ID,
		Name:             item.Name,
		Description:      item.Description,
		Enabled:          item.Enabled,
		TotalSteps:       len(item.Steps),
		Parameters:       item.Parameters,
		RequiresApproval: requiresApproval,
	}
}

func projectRun(item store.OpsRunbookRun, outputLimit int) runbookRunOutput {
	stepResults := make([]runbookStepResultOutput, 0, len(item.StepResults))
	for _, result := range item.StepResults {
		output, truncated := trailingRunes(result.Output, outputLimit)
		stepResults = append(stepResults, runbookStepResultOutput{
			StepIndex:       result.StepIndex,
			Title:           result.Title,
			Type:            result.Type,
			Output:          output,
			OutputTruncated: truncated,
			Error:           result.Error,
			DurationMS:      result.DurationMs,
		})
	}
	return runbookRunOutput{
		ID:             item.ID,
		RunbookID:      item.RunbookID,
		RunbookName:    item.RunbookName,
		Status:         item.Status,
		TotalSteps:     item.TotalSteps,
		CompletedSteps: item.CompletedSteps,
		CurrentStep:    item.CurrentStep,
		Error:          item.Error,
		StepResults:    stepResults,
		ParametersUsed: item.ParametersUsed,
		CreatedAt:      item.CreatedAt,
		StartedAt:      item.StartedAt,
		FinishedAt:     item.FinishedAt,
	}
}

func requiredID(raw, field string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	return value, nil
}

func normalizeOutputLimit(value int) int {
	if value <= 0 {
		return defaultRunOutputChars
	}
	if value > maxRunOutputChars {
		return maxRunOutputChars
	}
	return value
}

func trailingRunes(value string, limit int) (string, bool) {
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}
	return string(runes[len(runes)-limit:]), true
}

func runbookToolError(action string, err error) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("%s: not found", action)
	case errors.Is(err, store.ErrOpsRunbookNameMismatch):
		return errors.New("delete runbook: confirmName does not exactly match the persisted runbook name")
	case errors.Is(err, store.ErrOpsRunbookActive):
		return errors.New("delete runbook: runbook has a queued, running, or waiting-for-approval execution")
	default:
		return toolError(action, err)
	}
}
