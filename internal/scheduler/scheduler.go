// Package scheduler runs periodic Sentinel jobs.
package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/activity"
	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/runbook"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/validate"
)

const keyJobID = "jobId"

const (
	defaultTickInterval  = 5 * time.Second
	defaultMaxConcurrent = 5
	stepTimeout          = 30 * time.Second
	catchUpWindow        = 24 * time.Hour
)

type schedulerRepo interface {
	ListDueSchedules(ctx context.Context, now time.Time, limit int) ([]store.OpsSchedule, error)
	CreateOpsRunbookRun(ctx context.Context, runbookID string, now time.Time) (store.OpsRunbookRun, error)
	CreateOpsRunbookRunWithParams(ctx context.Context, runbookID string, now time.Time, params map[string]string) (store.OpsRunbookRun, error)
	UpdateScheduleAfterRun(ctx context.Context, scheduleID, lastRunAt, lastRunStatus, nextRunAt string, enabled bool) error
	UpdateScheduleLastRun(ctx context.Context, scheduleID, lastRunAt, lastRunStatus string) error
}

// Options configures the scheduler service.
type Options struct {
	TickInterval  time.Duration
	MaxConcurrent int
	EventHub      *events.Hub
	AlertRepo     runbook.AlertRepo
	// Guardrail, when non-nil, is evaluated against each scheduled runbook
	// command before execution (unattended runs are guarded like manual ones).
	Guardrail runbook.GuardrailFunc
}

// Service runs scheduled runbook executions on a tick loop.
type Service struct {
	repo        schedulerRepo
	runbookRepo runbook.Repo
	opts        Options
	startOnce   sync.Once
	stopOnce    sync.Once
	stopFn      context.CancelFunc
	doneCh      chan struct{}

	// runCtx is the parent context for all spawned runbook goroutines.
	// Cancelled on Stop to signal in-flight runs.
	runCtx    context.Context
	runCancel context.CancelFunc
	sem       chan struct{}
	wg        sync.WaitGroup

	// inFlight guards against overlapping runs of the same schedule: a schedule
	// stays claimed for the lifetime of its run, so a tick that sees it still
	// due (cron interval shorter than the run) skips it instead of double-firing.
	// stopping (under the same lock) makes wg.Add and Stop's wg.Wait mutually
	// exclusive, so a tick cannot register a new run after Stop began waiting.
	inFlightMu sync.Mutex
	inFlight   map[string]struct{}
	stopping   bool
}

// beginRun registers a run goroutine with the wait group unless the scheduler
// is stopping. It must wrap the matching wg.Done in the spawned goroutine.
func (s *Service) beginRun() bool {
	s.inFlightMu.Lock()
	defer s.inFlightMu.Unlock()
	if s.stopping {
		return false
	}
	s.wg.Add(1)
	return true
}

// New creates a scheduler service.
func New(r schedulerRepo, rr runbook.Repo, opts Options) *Service {
	if opts.TickInterval <= 0 {
		opts.TickInterval = defaultTickInterval
	}
	maxConc := opts.MaxConcurrent
	if maxConc <= 0 {
		maxConc = defaultMaxConcurrent
	}
	runCtx, runCancel := context.WithCancel(context.Background())
	return &Service{
		repo:        r,
		runbookRepo: rr,
		opts:        opts,
		sem:         make(chan struct{}, maxConc),
		runCtx:      runCtx,
		runCancel:   runCancel,
		inFlight:    make(map[string]struct{}),
	}
}

// claimSchedule marks a schedule as running. It returns false when a run for
// the same schedule is already in flight.
func (s *Service) claimSchedule(id string) bool {
	s.inFlightMu.Lock()
	defer s.inFlightMu.Unlock()
	if _, ok := s.inFlight[id]; ok {
		return false
	}
	s.inFlight[id] = struct{}{}
	return true
}

// releaseSchedule clears the in-flight marker for a schedule.
func (s *Service) releaseSchedule(id string) {
	s.inFlightMu.Lock()
	delete(s.inFlight, id)
	s.inFlightMu.Unlock()
}

// Start begins the scheduler tick loop in a background goroutine.
func (s *Service) Start(parent context.Context) {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(parent)
		s.stopFn = cancel
		s.doneCh = make(chan struct{})
		// Replace default runCtx with one derived from the parent so
		// that cancellation of the parent propagates to in-flight runs.
		s.runCancel()
		s.runCtx, s.runCancel = context.WithCancel(parent)

		go func() {
			defer close(s.doneCh)

			s.catchUpMissedRuns(ctx)

			ticker := time.NewTicker(s.opts.TickInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					s.tick(ctx)
				}
			}
		}()
	})
}

// Stop gracefully stops the scheduler service. It cancels the tick loop,
// signals in-flight runbook goroutines to stop, and waits for them.
func (s *Service) Stop(ctx context.Context) {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.stopFn != nil {
			s.stopFn()
		}
		if s.runCancel != nil {
			s.runCancel()
		}
		// Reject any further run registrations so wg.Add cannot race wg.Wait.
		s.inFlightMu.Lock()
		s.stopping = true
		s.inFlightMu.Unlock()
		if s.doneCh == nil {
			return
		}
		// Wait for tick loop.
		select {
		case <-s.doneCh:
		case <-ctx.Done():
		}
		// Wait for in-flight runbook goroutines.
		done := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
		}
	})
}

func (s *Service) tick(ctx context.Context) {
	now := time.Now().UTC()
	maxConc := cap(s.sem)
	due, err := s.repo.ListDueSchedules(ctx, now, maxConc)
	if err != nil {
		slog.Warn("scheduler list due schedules failed", "err", err)
		return
	}
	for _, sched := range due {
		nextRun, parseErr := time.Parse(time.RFC3339, sched.NextRunAt)
		if parseErr == nil && now.Sub(nextRun) > catchUpWindow {
			s.recomputeNextRun(ctx, sched)
			continue
		}
		s.executeDueSchedule(ctx, sched, now)
	}
}

func (s *Service) executeDueSchedule(ctx context.Context, sched store.OpsSchedule, now time.Time) {
	if !s.claimSchedule(sched.ID) {
		// A previous run for this schedule is still in flight; skip to avoid
		// overlapping runs of a non-idempotent runbook (restart/deploy/cleanup).
		return
	}

	// Resolve the runbook's parameter defaults so scheduled runs substitute
	// {{PARAM}} placeholders just like manual runs (which were running with the
	// raw placeholders before).
	rb, rbErr := s.runbookRepo.GetOpsRunbook(ctx, sched.RunbookID)
	if rbErr != nil {
		s.releaseSchedule(sched.ID)
		if errors.Is(rbErr, sql.ErrNoRows) {
			slog.Warn("scheduler auto-heal: disabling orphan schedule", "schedule", sched.ID, "runbook", sched.RunbookID)
			if healErr := s.repo.UpdateScheduleAfterRun(ctx, sched.ID, "", "", "", false); healErr != nil {
				slog.Warn("scheduler auto-heal: update failed", "schedule", sched.ID, "err", healErr)
			}
			return
		}
		slog.Warn("scheduler load runbook failed", "schedule", sched.ID, "runbook", sched.RunbookID, "err", rbErr)
		return
	}
	params := runbook.ResolveParams(rb.Parameters, nil)
	if err := runbook.ValidateParams(rb.Parameters, params); err != nil {
		// A required parameter has no default; running with placeholders would be
		// worse than skipping. Surface it instead of executing.
		s.releaseSchedule(sched.ID)
		slog.Warn("scheduler skipping run: unmet required parameters", "schedule", sched.ID, "runbook", sched.RunbookID, "err", err)
		return
	}

	// Advance next_run_at (and mark running) BEFORE creating the run so a crash
	// between the two can't leave the schedule still 'due' and re-fire a
	// duplicate run on restart. If the create below fails the schedule simply
	// skips this cycle, which is safer than a double run.
	nextRunAt, enabled := s.computeNextRun(sched)
	if err := s.repo.UpdateScheduleAfterRun(ctx, sched.ID, now.Format(time.RFC3339), "running", nextRunAt, enabled); err != nil {
		s.releaseSchedule(sched.ID)
		slog.Warn("scheduler advance schedule failed", "schedule", sched.ID, "err", err)
		return
	}

	job, err := s.repo.CreateOpsRunbookRunWithParams(ctx, sched.RunbookID, now, params)
	if err != nil {
		s.releaseSchedule(sched.ID)
		slog.Warn("scheduler create run failed", "schedule", sched.ID, "runbook", sched.RunbookID, "err", err)
		return
	}

	slog.Info("scheduler triggered run", "schedule", sched.ID, "runbook", sched.RunbookID, "job", job.ID)

	// Record the "runbook.started" activity event (same as manual runs).
	if _, err := s.runbookRepo.InsertActivityEvent(ctx, activity.EventWrite{
		Source:    "runbook",
		EventType: "runbook.started",
		Severity:  "info",
		Resource:  job.RunbookID,
		Message:   fmt.Sprintf("Runbook started: %s", job.RunbookName),
		Details:   fmt.Sprintf("job=%s steps=%d source=scheduler schedule=%s", job.ID, job.TotalSteps, sched.ID),
		Metadata:  marshalStartedMetadata(job.ID, job.RunbookID, sched.ID),
		CreatedAt: now,
	}); err != nil {
		slog.Warn("scheduler: record runbook.started event", "job", job.ID, "err", err)
	}

	s.publish(events.TypeScheduleUpdated, map[string]any{
		"action":   "triggered",
		"schedule": sched.ID,
		keyJobID:   job.ID,
	})

	if !s.beginRun() {
		s.releaseSchedule(sched.ID)
		return
	}
	go func() {
		defer s.wg.Done()
		defer s.releaseSchedule(sched.ID)
		// Acquire semaphore (backpressure).
		select {
		case s.sem <- struct{}{}:
			defer func() { <-s.sem }()
		case <-s.runCtx.Done():
			return
		}
		s.executeRunbook(s.runCtx, job, sched.ID, params)
	}()
}

func (s *Service) executeRunbook(ctx context.Context, job store.OpsRunbookRun, scheduleID string, params map[string]string) {
	runbook.Run(ctx, s.runbookRepo, s.emitEvent, runbook.RunParams{
		Job:         job,
		Source:      "scheduler",
		StepTimeout: stepTimeout,
		Parameters:  params,
		AlertRepo:   s.opts.AlertRepo,
		Guardrail:   s.opts.Guardrail,
		ExtraMetadata: map[string]string{
			"scheduleId": scheduleID,
		},
		OnFinish: func(ctx context.Context, status string) {
			finished := time.Now().UTC()
			// Update only last_run_*; next_run_at/enabled were set at dispatch and
			// may have been edited during the run.
			if err := s.repo.UpdateScheduleLastRun(ctx, scheduleID, finished.Format(time.RFC3339), status); err != nil {
				slog.Warn("scheduler: update schedule after run", "err", err)
			}
			s.publish(events.TypeScheduleUpdated, map[string]any{
				"action":   "run_completed",
				"schedule": scheduleID,
				keyJobID:   job.ID,
				"status":   status,
			})
		},
	})
}

func (s *Service) emitEvent(eventType string, payload map[string]any) {
	s.publish(eventType, payload)
}

func (s *Service) computeNextRun(sched store.OpsSchedule) (string, bool) {
	if sched.ScheduleType == "once" {
		return "", false
	}

	// type="cron": compute next run time.
	loc, err := time.LoadLocation(sched.Timezone)
	if err != nil {
		slog.Warn("scheduler invalid timezone, using UTC", "schedule", sched.ID, "timezone", sched.Timezone)
		loc = time.UTC
	}
	cronSched, err := validate.ParseCron(sched.CronExpr)
	if err != nil {
		slog.Warn("scheduler invalid cron expression", "schedule", sched.ID, "expr", sched.CronExpr, "err", err)
		return "", false
	}
	nextRun := cronSched.Next(time.Now().In(loc)).UTC().Format(time.RFC3339)
	return nextRun, true
}

func (s *Service) catchUpMissedRuns(ctx context.Context) {
	now := time.Now().UTC()
	maxConc := cap(s.sem)
	due, err := s.repo.ListDueSchedules(ctx, now, maxConc)
	if err != nil {
		slog.Warn("scheduler catch-up list failed", "err", err)
		return
	}

	for _, sched := range due {
		nextRun, parseErr := time.Parse(time.RFC3339, sched.NextRunAt)
		if parseErr != nil {
			continue
		}
		// Too old; just recompute to the future.
		if now.Sub(nextRun) > catchUpWindow {
			s.recomputeNextRun(ctx, sched)
			continue
		}

		slog.Info("scheduler catching up missed run", "schedule", sched.ID, "missed_at", sched.NextRunAt)
		s.executeDueSchedule(ctx, sched, now)
	}
}

func (s *Service) recomputeNextRun(ctx context.Context, sched store.OpsSchedule) {
	if sched.ScheduleType == "once" {
		// One-time schedule that's past due and beyond catch-up: disable it.
		if err := s.repo.UpdateScheduleAfterRun(ctx, sched.ID, "", "", "", false); err != nil {
			slog.Warn("scheduler: disable one-time schedule", "schedule", sched.ID, "err", err)
		}
		return
	}

	loc, err := time.LoadLocation(sched.Timezone)
	if err != nil {
		loc = time.UTC
	}
	cronSched, err := validate.ParseCron(sched.CronExpr)
	if err != nil {
		slog.Warn("scheduler recompute failed", "schedule", sched.ID, "err", err)
		return
	}
	nextRun := cronSched.Next(time.Now().In(loc)).UTC().Format(time.RFC3339)
	if err := s.repo.UpdateScheduleAfterRun(ctx, sched.ID, sched.LastRunAt, sched.LastRunStatus, nextRun, true); err != nil {
		slog.Warn("scheduler: recompute next run", "schedule", sched.ID, "err", err)
	}
}

func (s *Service) publish(eventType string, payload map[string]any) {
	if s == nil || s.opts.EventHub == nil {
		return
	}
	s.opts.EventHub.Publish(events.NewEvent(eventType, payload))
}

func marshalStartedMetadata(jobID, runbookID, scheduleID string) string {
	b, err := json.Marshal(map[string]string{
		keyJobID:     jobID,
		"runbookId":  runbookID,
		"scheduleId": scheduleID,
		"status":     "queued",
	})
	if err != nil {
		return "{}"
	}
	return string(b)
}
