// Package scheduler runs periodic Sentinel jobs.
package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/runbook"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/validate"
)

const (
	keyAction   = "action"
	keyJobID    = "jobId"
	keySchedule = "schedule"
)

const (
	defaultTickInterval  = 5 * time.Second
	defaultMaxConcurrent = 5
	stepTimeout          = 30 * time.Second
	catchUpWindow        = 24 * time.Hour
)

type schedulerRepo interface {
	ListDueSchedules(ctx context.Context, now time.Time, limit int) ([]store.OpsSchedule, error)
	GetOpsRunbook(ctx context.Context, id string) (store.OpsRunbook, error)
	CreateOpsRunbookRun(ctx context.Context, write store.OpsRunbookRunWrite) (store.OpsRunbookRun, error)
	UpdateScheduleAfterRun(ctx context.Context, scheduleID, lastRunAt, lastRunStatus, nextRunAt string, enabled bool) error
	UpdateScheduleLastRun(ctx context.Context, scheduleID, lastRunAt, lastRunStatus string) error
}

// Options configures the scheduler service.
type Options struct {
	TickInterval  time.Duration
	MaxConcurrent int
	EventHub      *events.Hub
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
	rb, rbErr := s.repo.GetOpsRunbook(ctx, sched.RunbookID)
	if rbErr != nil {
		s.releaseSchedule(sched.ID)
		if errors.Is(rbErr, sql.ErrNoRows) {
			slog.Warn("scheduler auto-heal: disabling orphan schedule", keySchedule, sched.ID, "runbook", sched.RunbookID)
			s.disableSchedule(ctx, sched)
			return
		}
		slog.Warn("scheduler load runbook failed", keySchedule, sched.ID, "runbook", sched.RunbookID, "err", rbErr)
		return
	}
	if !rb.Enabled {
		// The operator disabled the runbook. Skip this occurrence and advance to
		// the next one; leaving next_run_at in the past would make the schedule
		// due on every tick forever.
		s.releaseSchedule(sched.ID)
		slog.Info("scheduler skipping disabled runbook", keySchedule, sched.ID, "runbook", sched.RunbookID)
		s.recomputeNextRun(ctx, sched)
		return
	}
	params := runbook.ResolveParams(rb.Parameters, nil)
	if err := runbook.ValidateParams(rb.Parameters, params); err != nil {
		// A required parameter has no default; running with placeholders would be
		// worse than skipping. The scheduler never supplies overrides, so every
		// future tick would fail identically: disable instead of warning forever.
		s.releaseSchedule(sched.ID)
		slog.Warn("scheduler disabling schedule: unmet required parameters", keySchedule, sched.ID, "runbook", sched.RunbookID, "err", err)
		s.disableSchedule(ctx, sched)
		return
	}

	// Advance next_run_at (and mark running) BEFORE creating the run so a crash
	// between the two can't leave the schedule still 'due' and re-fire a
	// duplicate run on restart. If the create below fails the schedule simply
	// skips this cycle, which is safer than a double run.
	nextRunAt, enabled := s.computeNextRun(sched)
	if err := s.repo.UpdateScheduleAfterRun(ctx, sched.ID, now.Format(time.RFC3339), "running", nextRunAt, enabled); err != nil {
		s.releaseSchedule(sched.ID)
		slog.Warn("scheduler advance schedule failed", keySchedule, sched.ID, "err", err)
		return
	}

	job, err := s.repo.CreateOpsRunbookRun(ctx, store.OpsRunbookRunWrite{
		Definition: rb,
		Source:     store.OpsRunbookRunSourceScheduler,
		Parameters: params,
		At:         now,
	})
	if err != nil {
		s.releaseSchedule(sched.ID)
		if errors.Is(err, store.ErrOpsRunbookTargetBusy) {
			if updateErr := s.repo.UpdateScheduleLastRun(
				ctx,
				sched.ID,
				now.Format(time.RFC3339),
				"target_busy",
			); updateErr != nil {
				slog.Warn("scheduler record busy target failed", keySchedule, sched.ID, "err", updateErr)
			}
			s.publish(events.TypeScheduleUpdated, map[string]any{
				keyAction:   "run_skipped",
				keySchedule: sched.ID,
				"status":    "target_busy",
			})
			return
		}
		slog.Warn("scheduler create run failed", keySchedule, sched.ID, "runbook", sched.RunbookID, "err", err)
		return
	}

	slog.Info("scheduler triggered run", keySchedule, sched.ID, "runbook", sched.RunbookID, "job", job.ID)

	s.publish(events.TypeScheduleUpdated, map[string]any{
		keyAction:   "triggered",
		keySchedule: sched.ID,
		keyJobID:    job.ID,
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
		s.executeRunbook(s.runCtx, job, sched.ID)
	}()
}

func (s *Service) executeRunbook(ctx context.Context, job store.OpsRunbookRun, scheduleID string) {
	runbook.Run(ctx, s.runbookRepo, s.emitEvent, runbook.RunParams{
		Job:         job,
		StepTimeout: stepTimeout,
		OnFinish: func(ctx context.Context, status string) {
			finished := time.Now().UTC()
			// Update only last_run_*; next_run_at/enabled were set at dispatch and
			// may have been edited during the run.
			if err := s.repo.UpdateScheduleLastRun(ctx, scheduleID, finished.Format(time.RFC3339), status); err != nil {
				slog.Warn("scheduler: update schedule after run", "err", err)
			}
			s.publish(events.TypeScheduleUpdated, map[string]any{
				keyAction:   "run_completed",
				keySchedule: scheduleID,
				keyJobID:    job.ID,
				"status":    status,
			})
		},
	})
}

func (s *Service) emitEvent(eventType string, payload map[string]any) {
	s.publish(eventType, payload)
}

// computeNextRun returns the schedule's next fire time. A false second return
// means the schedule cannot fire again and must be disabled: it is a one-time
// schedule, or its cron expression will never produce another occurrence.
func (s *Service) computeNextRun(sched store.OpsSchedule) (string, bool) {
	if sched.ScheduleType == "once" {
		return "", false
	}
	return nextCronRun(sched)
}

// nextCronRun computes the next occurrence of a cron schedule. It returns false
// when the expression cannot be parsed, or when it has no next occurrence at all
// (robfig/cron gives up after five years and returns the zero time, e.g. for
// "0 0 30 2 *"). Both are terminal: persisting the zero time would leave the
// schedule permanently overdue and rewritten on every tick.
func nextCronRun(sched store.OpsSchedule) (string, bool) {
	loc, err := time.LoadLocation(sched.Timezone)
	if err != nil {
		slog.Warn("scheduler invalid timezone, using UTC", keySchedule, sched.ID, "timezone", sched.Timezone)
		loc = time.UTC
	}
	cronSched, err := validate.ParseCron(sched.CronExpr)
	if err != nil {
		slog.Warn("scheduler invalid cron expression", keySchedule, sched.ID, "expr", sched.CronExpr, "err", err)
		return "", false
	}
	next := cronSched.Next(time.Now().In(loc))
	if next.IsZero() {
		slog.Warn("scheduler cron expression never fires", keySchedule, sched.ID, "expr", sched.CronExpr)
		return "", false
	}
	return next.UTC().Format(time.RFC3339), true
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

		slog.Info("scheduler catching up missed run", keySchedule, sched.ID, "missed_at", sched.NextRunAt)
		s.executeDueSchedule(ctx, sched, now)
	}
}

func (s *Service) recomputeNextRun(ctx context.Context, sched store.OpsSchedule) {
	if sched.ScheduleType == "once" {
		// One-time schedule that's past due and beyond catch-up: disable it.
		s.disableSchedule(ctx, sched)
		return
	}

	nextRun, ok := nextCronRun(sched)
	if !ok {
		// The expression can never fire again. Leaving next_run_at in the past
		// keeps the schedule due on every tick forever, so disable it instead.
		slog.Warn("scheduler disabling schedule: cron expression has no next run", keySchedule, sched.ID, "expr", sched.CronExpr)
		s.disableSchedule(ctx, sched)
		return
	}
	if err := s.repo.UpdateScheduleAfterRun(ctx, sched.ID, sched.LastRunAt, sched.LastRunStatus, nextRun, true); err != nil {
		slog.Warn("scheduler: recompute next run", keySchedule, sched.ID, "err", err)
	}
}

// disableSchedule turns a schedule off after a terminal condition, clearing
// next_run_at so it stops coming back as due. The last-run fields are kept so
// the UI still shows what the schedule did before it stopped; callers log why.
func (s *Service) disableSchedule(ctx context.Context, sched store.OpsSchedule) {
	if err := s.repo.UpdateScheduleAfterRun(ctx, sched.ID, sched.LastRunAt, sched.LastRunStatus, "", false); err != nil {
		slog.Warn("scheduler: disable schedule", keySchedule, sched.ID, "err", err)
	}
}

func (s *Service) publish(eventType string, payload map[string]any) {
	if s == nil || s.opts.EventHub == nil {
		return
	}
	s.opts.EventHub.Publish(events.NewEvent(eventType, payload))
}
