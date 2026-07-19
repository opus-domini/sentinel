package runbook

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
)

const defaultMaxConcurrentRuns = 5

// ErrTooManyExecutions is returned when the shared manual-execution limit is
// full across HTTP and MCP callers.
var ErrTooManyExecutions = errors.New("too many concurrent runbook executions")

// ErrInvalidDefinition is returned when a runbook definition is not valid.
var ErrInvalidDefinition = errors.New("invalid runbook definition")

// ErrInvalidParameters is returned when supplied execution parameters do not
// satisfy the runbook definition.
var ErrInvalidParameters = errors.New("invalid runbook parameters")

// ErrInvalidRunState is returned when an approval transition is not valid for
// the current persisted run state.
var ErrInvalidRunState = errors.New("invalid runbook run state")

// ManagerRepo is the complete persistence contract used by Manager.
type ManagerRepo interface {
	Repo
	ListOpsRunbooks(ctx context.Context) ([]store.OpsRunbook, error)
	ListOpsRunbookRuns(ctx context.Context, limit int) ([]store.OpsRunbookRun, error)
	InsertOpsRunbook(ctx context.Context, write store.OpsRunbookWrite) (store.OpsRunbook, error)
	UpdateOpsRunbook(ctx context.Context, write store.OpsRunbookWrite) (store.OpsRunbook, error)
	CreateOpsRunbookRunWithParams(ctx context.Context, runbookID string, at time.Time, params map[string]string) (store.OpsRunbookRun, error)
	DeleteOpsRunbook(ctx context.Context, id, expectedName string) (store.OpsRunbookDeleteResult, error)
}

// Manager owns the shared manual runbook control plane used by HTTP and MCP.
type Manager struct {
	repo   ManagerRepo
	emit   EmitFunc
	ctx    context.Context
	cancel context.CancelFunc
	sem    chan struct{}
	wg     sync.WaitGroup
}

// NewManager creates a shared runbook manager.
func NewManager(repo ManagerRepo, emit EmitFunc, maxConcurrent int) *Manager {
	if maxConcurrent <= 0 {
		maxConcurrent = defaultMaxConcurrentRuns
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		repo:   repo,
		emit:   emit,
		ctx:    ctx,
		cancel: cancel,
		sem:    make(chan struct{}, maxConcurrent),
	}
}

// List returns every persisted runbook.
func (m *Manager) List(ctx context.Context) ([]store.OpsRunbook, error) {
	if m == nil || m.repo == nil {
		return nil, errors.New("runbook manager is unavailable")
	}
	return m.repo.ListOpsRunbooks(ctx)
}

// Get returns one persisted runbook.
func (m *Manager) Get(ctx context.Context, id string) (store.OpsRunbook, error) {
	if m == nil || m.repo == nil {
		return store.OpsRunbook{}, errors.New("runbook manager is unavailable")
	}
	return m.repo.GetOpsRunbook(ctx, id)
}

// Create validates and persists a runbook without executing it.
func (m *Manager) Create(ctx context.Context, write store.OpsRunbookWrite) (store.OpsRunbook, []ShellWarning, error) {
	if m == nil || m.repo == nil {
		return store.OpsRunbook{}, nil, errors.New("runbook manager is unavailable")
	}
	if err := ValidateDefinition(write); err != nil {
		return store.OpsRunbook{}, nil, fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
	}
	created, err := m.repo.InsertOpsRunbook(ctx, write)
	if err != nil {
		return store.OpsRunbook{}, nil, err
	}
	return created, ShellWarnings(write.Steps), nil
}

// Update validates and replaces one persisted runbook definition. It remains
// an HTTP-only capability; MCP deliberately does not register an update tool.
func (m *Manager) Update(ctx context.Context, write store.OpsRunbookWrite) (store.OpsRunbook, []ShellWarning, error) {
	if m == nil || m.repo == nil {
		return store.OpsRunbook{}, nil, errors.New("runbook manager is unavailable")
	}
	if err := ValidateDefinition(write); err != nil {
		return store.OpsRunbook{}, nil, fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
	}
	updated, err := m.repo.UpdateOpsRunbook(ctx, write)
	if err != nil {
		return store.OpsRunbook{}, nil, err
	}
	return updated, ShellWarnings(write.Steps), nil
}

// Delete atomically removes a runbook and its schedules while preserving
// historical runs.
func (m *Manager) Delete(ctx context.Context, id, expectedName string) (store.OpsRunbookDeleteResult, error) {
	if m == nil || m.repo == nil {
		return store.OpsRunbookDeleteResult{}, errors.New("runbook manager is unavailable")
	}
	return m.repo.DeleteOpsRunbook(ctx, id, expectedName)
}

// ListRuns returns recent runbook executions.
func (m *Manager) ListRuns(ctx context.Context, limit int) ([]store.OpsRunbookRun, error) {
	if m == nil || m.repo == nil {
		return nil, errors.New("runbook manager is unavailable")
	}
	return m.repo.ListOpsRunbookRuns(ctx, limit)
}

// GetRun returns one runbook execution.
func (m *Manager) GetRun(ctx context.Context, id string) (store.OpsRunbookRun, error) {
	if m == nil || m.repo == nil {
		return store.OpsRunbookRun{}, errors.New("runbook manager is unavailable")
	}
	return m.repo.GetOpsRunbookRun(ctx, id)
}

// Start validates parameters, persists a run, and launches it asynchronously.
func (m *Manager) Start(ctx context.Context, runbookID string, params map[string]string, source string) (store.OpsRunbookRun, error) {
	if m == nil || m.repo == nil {
		return store.OpsRunbookRun{}, errors.New("runbook manager is unavailable")
	}
	if !m.acquire() {
		return store.OpsRunbookRun{}, ErrTooManyExecutions
	}
	release := true
	defer func() {
		if release {
			m.release()
		}
	}()

	rb, err := m.repo.GetOpsRunbook(ctx, runbookID)
	if err != nil {
		return store.OpsRunbookRun{}, err
	}
	if err := ValidateInputParams(rb.Parameters, params); err != nil {
		return store.OpsRunbookRun{}, fmt.Errorf("%w: %w", ErrInvalidParameters, err)
	}
	resolved := ResolveParams(rb.Parameters, params)
	if err := ValidateParams(rb.Parameters, resolved); err != nil {
		return store.OpsRunbookRun{}, fmt.Errorf("%w: %w", ErrInvalidParameters, err)
	}
	now := time.Now().UTC()
	job, err := m.repo.CreateOpsRunbookRunWithParams(ctx, runbookID, now, resolved)
	if err != nil {
		return store.OpsRunbookRun{}, err
	}

	m.emitEvent("ops.job.updated", map[string]any{
		keyGlobalRev: now.UnixMilli(),
		keyJob:       job,
	})
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer m.release()
		Run(m.ctx, m.repo, m.emitEvent, RunParams{
			Job:         job,
			Source:      source,
			StepTimeout: 30 * time.Second,
			Parameters:  resolved,
		})
	}()
	release = false
	return job, nil
}

// Approve atomically claims and resumes an approval-paused run.
func (m *Manager) Approve(ctx context.Context, runID, source string) (store.OpsRunbookRun, error) {
	if m == nil || m.repo == nil {
		return store.OpsRunbookRun{}, errors.New("runbook manager is unavailable")
	}
	job, err := m.repo.GetOpsRunbookRun(ctx, runID)
	if err != nil {
		return store.OpsRunbookRun{}, err
	}
	if job.Status != store.OpsRunbookStatusWaitingApproval {
		return store.OpsRunbookRun{}, fmt.Errorf("%w: run status is %q, not waiting_approval", ErrInvalidRunState, job.Status)
	}
	approvalStep := approvalStepIndex(job)
	if approvalStep < 0 {
		return store.OpsRunbookRun{}, errors.New("could not find approval step in results")
	}
	if !m.acquire() {
		return store.OpsRunbookRun{}, ErrTooManyExecutions
	}
	release := true
	defer func() {
		if release {
			m.release()
		}
	}()

	now := time.Now().UTC()
	running, err := m.repo.UpdateOpsRunbookRun(ctx, store.OpsRunbookRunUpdate{
		RunID:          job.ID,
		Status:         runnerStatusRunning,
		CompletedSteps: approvalStep + 1,
		CurrentStep:    job.CurrentStep,
		StartedAt:      now.Format(time.RFC3339),
		FromStatus:     store.OpsRunbookStatusWaitingApproval,
	})
	if err != nil {
		if errors.Is(err, store.ErrOpsRunbookRunConflict) {
			return store.OpsRunbookRun{}, fmt.Errorf("%w: run is no longer waiting for approval", ErrInvalidRunState)
		}
		return store.OpsRunbookRun{}, err
	}

	m.emitEvent("ops.job.updated", map[string]any{
		keyGlobalRev: now.UnixMilli(),
		keyJob:       running,
	})
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer m.release()
		ResumeRun(m.ctx, m.repo, m.emitEvent, RunParams{
			Job:         running,
			Source:      source,
			StepTimeout: 30 * time.Second,
			Parameters:  job.ParametersUsed,
		}, approvalStep)
	}()
	release = false
	return running, nil
}

// Reject atomically fails an approval-paused run.
func (m *Manager) Reject(ctx context.Context, runID string) (store.OpsRunbookRun, error) {
	if m == nil || m.repo == nil {
		return store.OpsRunbookRun{}, errors.New("runbook manager is unavailable")
	}
	job, err := m.repo.GetOpsRunbookRun(ctx, runID)
	if err != nil {
		return store.OpsRunbookRun{}, err
	}
	if job.Status != store.OpsRunbookStatusWaitingApproval {
		return store.OpsRunbookRun{}, fmt.Errorf("%w: run status is %q, not waiting_approval", ErrInvalidRunState, job.Status)
	}
	now := time.Now().UTC()
	updated, err := m.repo.UpdateOpsRunbookRun(ctx, store.OpsRunbookRunUpdate{
		RunID:          runID,
		Status:         runnerStatusFailed,
		CompletedSteps: job.CompletedSteps,
		CurrentStep:    job.CurrentStep,
		Error:          "approval rejected",
		FinishedAt:     now.Format(time.RFC3339),
		FromStatus:     store.OpsRunbookStatusWaitingApproval,
	})
	if err != nil {
		if errors.Is(err, store.ErrOpsRunbookRunConflict) {
			return store.OpsRunbookRun{}, fmt.Errorf("%w: run is no longer waiting for approval", ErrInvalidRunState)
		}
		return store.OpsRunbookRun{}, err
	}
	m.emitEvent("ops.job.updated", map[string]any{
		keyGlobalRev: now.UnixMilli(),
		keyJob:       updated,
	})
	return updated, nil
}

func approvalStepIndex(job store.OpsRunbookRun) int {
	index := -1
	for _, result := range job.StepResults {
		if result.Type == stepTypeApproval {
			index = result.StepIndex
		}
	}
	return index
}

func (m *Manager) acquire() bool {
	select {
	case m.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

func (m *Manager) release() {
	<-m.sem
}

func (m *Manager) emitEvent(eventType string, payload map[string]any) {
	if m.emit != nil {
		m.emit(eventType, payload)
	}
}

// WaitIdle waits until all manager-owned executions finish.
func (m *Manager) WaitIdle() {
	if m != nil {
		m.wg.Wait()
	}
}

// Shutdown cancels manager-owned executions and waits for them to finish.
func (m *Manager) Shutdown(ctx context.Context) {
	if m == nil {
		return
	}
	m.cancel()
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// IsTerminalStatus reports whether a run no longer executes or waits for
// approval.
func IsTerminalStatus(status string) bool {
	return status == runnerStatusSucceeded || status == runnerStatusFailed
}

// IsWaitingApproval reports whether a run is paused for human approval.
func IsWaitingApproval(status string) bool {
	return status == runnerStatusWaitingApproval
}

var _ ManagerRepo = (*store.Store)(nil)
