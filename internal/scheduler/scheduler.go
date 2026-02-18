package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/runbook"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/validate"
)

const (
	defaultTickInterval = 5 * time.Second
	executionTimeout    = 5 * time.Minute
	stepTimeout         = 30 * time.Second
	catchUpWindow       = 24 * time.Hour
	stateFailed         = "failed"
)

// Options configures the scheduler service.
type Options struct {
	TickInterval time.Duration
	EventHub     *events.Hub
}

// Service runs scheduled runbook executions on a tick loop.
type Service struct {
	store     *store.Store
	opts      Options
	startOnce sync.Once
	stopOnce  sync.Once
	stopFn    context.CancelFunc
	doneCh    chan struct{}
}

// New creates a scheduler service.
func New(st *store.Store, opts Options) *Service {
	if opts.TickInterval <= 0 {
		opts.TickInterval = defaultTickInterval
	}
	return &Service{
		store: st,
		opts:  opts,
	}
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

// Stop gracefully stops the scheduler service.
func (s *Service) Stop(ctx context.Context) {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.stopFn != nil {
			s.stopFn()
		}
		if s.doneCh == nil {
			return
		}
		select {
		case <-s.doneCh:
		case <-ctx.Done():
		}
	})
}

func (s *Service) tick(ctx context.Context) {
	now := time.Now().UTC()
	due, err := s.store.ListDueSchedules(ctx, now)
	if err != nil {
		slog.Warn("scheduler list due schedules failed", "err", err)
		return
	}
	for _, sched := range due {
		s.executeDueSchedule(ctx, sched, now)
	}
}

func (s *Service) executeDueSchedule(ctx context.Context, sched store.OpsSchedule, now time.Time) {
	job, err := s.store.CreateOpsRunbookRun(ctx, sched.RunbookID, now)
	if err != nil {
		slog.Warn("scheduler create run failed", "schedule", sched.ID, "runbook", sched.RunbookID, "err", err)
		return
	}

	slog.Info("scheduler triggered run", "schedule", sched.ID, "runbook", sched.RunbookID, "job", job.ID)

	// Compute next run and whether to disable.
	nextRunAt, enabled := s.computeNextRun(sched)

	if err := s.store.UpdateScheduleAfterRun(ctx, sched.ID, now.Format(time.RFC3339), "running", nextRunAt, enabled); err != nil {
		slog.Warn("scheduler update after run failed", "schedule", sched.ID, "err", err)
	}

	s.publish(events.TypeScheduleUpdated, map[string]any{
		"action":   "triggered",
		"schedule": sched.ID,
		"jobId":    job.ID,
	})

	go s.executeRunbook(job, sched.ID)
}

func (s *Service) executeRunbook(job store.OpsRunbookRun, scheduleID string) {
	ctx, cancel := context.WithTimeout(context.Background(), executionTimeout)
	defer cancel()

	now := time.Now().UTC()

	// Mark as running.
	runningJob, _ := s.store.UpdateOpsRunbookRun(ctx, store.OpsRunbookRunUpdate{
		RunID:          job.ID,
		Status:         "running",
		CompletedSteps: 0,
		CurrentStep:    job.CurrentStep,
		StartedAt:      now.Format(time.RFC3339),
	})
	s.publish(events.TypeOpsJob, map[string]any{
		"globalRev": now.UnixMilli(),
		"job":       runningJob,
	})

	// Fetch runbook steps.
	runbooks, err := s.store.ListOpsRunbooks(ctx)
	if err != nil {
		s.finishRunbookRun(ctx, job.ID, scheduleID, stateFailed, 0, "", err.Error(), "[]")
		return
	}
	var steps []runbook.Step
	for _, rb := range runbooks {
		if rb.ID == job.RunbookID {
			for _, st := range rb.Steps {
				steps = append(steps, runbook.Step{
					Type:        st.Type,
					Title:       st.Title,
					Command:     st.Command,
					Check:       st.Check,
					Description: st.Description,
				})
			}
			break
		}
	}

	executor := runbook.NewExecutor(nil, stepTimeout)
	var accumulated []store.OpsRunbookStepResult
	progress := func(completed int, stepTitle string, result runbook.StepResult) {
		sr := store.OpsRunbookStepResult{
			StepIndex:  result.StepIndex,
			Title:      result.Title,
			Type:       result.Type,
			Output:     result.Output,
			Error:      result.Error,
			DurationMs: result.Duration.Milliseconds(),
		}
		accumulated = append(accumulated, sr)
		stepResultsJSON, _ := json.Marshal(accumulated)
		updated, _ := s.store.UpdateOpsRunbookRun(ctx, store.OpsRunbookRunUpdate{
			RunID:          job.ID,
			Status:         "running",
			CompletedSteps: completed,
			CurrentStep:    stepTitle,
			StepResults:    string(stepResultsJSON),
			StartedAt:      now.Format(time.RFC3339),
		})
		s.publish(events.TypeOpsJob, map[string]any{
			"globalRev": time.Now().UTC().UnixMilli(),
			"job":       updated,
		})
	}

	results, execErr := executor.Execute(ctx, steps, progress)

	finalStatus := "succeeded"
	errMsg := ""
	if execErr != nil {
		finalStatus = stateFailed
		errMsg = execErr.Error()
	}
	lastStep := ""
	if len(results) > 0 {
		lastStep = results[len(results)-1].Title
	}
	stepResultsJSON, _ := json.Marshal(accumulated)

	s.finishRunbookRun(ctx, job.ID, scheduleID, finalStatus, len(results), lastStep, errMsg, string(stepResultsJSON))
}

func (s *Service) finishRunbookRun(ctx context.Context, runID, scheduleID, status string, completed int, lastStep, errMsg, stepResultsJSON string) {
	finished := time.Now().UTC()
	_, _ = s.store.UpdateOpsRunbookRun(ctx, store.OpsRunbookRunUpdate{
		RunID:          runID,
		Status:         status,
		CompletedSteps: completed,
		CurrentStep:    lastStep,
		Error:          errMsg,
		StepResults:    stepResultsJSON,
		FinishedAt:     finished.Format(time.RFC3339),
	})

	globalRev := finished.UnixMilli()
	updatedJob, _ := s.store.GetOpsRunbookRun(ctx, runID)
	s.publish(events.TypeOpsJob, map[string]any{
		"globalRev": globalRev,
		"job":       updatedJob,
	})

	severity := "info"
	if status == stateFailed {
		severity = "error"
	}
	te, _ := s.store.InsertOpsTimelineEvent(ctx, store.OpsTimelineEventWrite{
		Source:    "scheduler",
		EventType: "runbook." + status,
		Severity:  severity,
		Resource:  runID,
		Message:   fmt.Sprintf("Scheduled runbook run %s", status),
		Details:   errMsg,
		Metadata:  fmt.Sprintf(`{"jobId":"%s","scheduleId":"%s","status":"%s"}`, runID, scheduleID, status),
		CreatedAt: finished,
	})
	if te.ID > 0 {
		s.publish(events.TypeOpsTimeline, map[string]any{
			"globalRev": globalRev,
			"event":     te,
		})
	}

	// Update schedule with final run status.
	_ = s.store.UpdateScheduleAfterRun(ctx, scheduleID, finished.Format(time.RFC3339), status, "", true)

	s.publish(events.TypeScheduleUpdated, map[string]any{
		"action":   "run_completed",
		"schedule": scheduleID,
		"jobId":    runID,
		"status":   status,
	})
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
	schedules, err := s.store.ListOpsSchedules(ctx)
	if err != nil {
		slog.Warn("scheduler catch-up list failed", "err", err)
		return
	}

	for _, sched := range schedules {
		if !sched.Enabled || sched.NextRunAt == "" {
			continue
		}
		nextRun, parseErr := time.Parse(time.RFC3339, sched.NextRunAt)
		if parseErr != nil {
			continue
		}
		if nextRun.After(now) {
			continue
		}
		// Only catch up if within the 24-hour window.
		if now.Sub(nextRun) > catchUpWindow {
			// Too old; just recompute to the future.
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
		_ = s.store.UpdateScheduleAfterRun(ctx, sched.ID, "", "", "", false)
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
	_ = s.store.UpdateScheduleAfterRun(ctx, sched.ID, sched.LastRunAt, sched.LastRunStatus, nextRun, true)
}

func (s *Service) publish(eventType string, payload map[string]any) {
	if s == nil || s.opts.EventHub == nil {
		return
	}
	s.opts.EventHub.Publish(events.NewEvent(eventType, payload))
}
