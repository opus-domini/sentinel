package scheduler

import (
	"context"
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
	stepTimeout         = 30 * time.Second
	catchUpWindow       = 24 * time.Hour
)

type schedulerRepo interface {
	ListDueSchedules(ctx context.Context, now time.Time) ([]store.OpsSchedule, error)
	ListOpsSchedules(ctx context.Context) ([]store.OpsSchedule, error)
	CreateOpsRunbookRun(ctx context.Context, runbookID string, now time.Time) (store.OpsRunbookRun, error)
	UpdateScheduleAfterRun(ctx context.Context, scheduleID, lastRunAt, lastRunStatus, nextRunAt string, enabled bool) error
}

// Options configures the scheduler service.
type Options struct {
	TickInterval time.Duration
	EventHub     *events.Hub
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
}

// New creates a scheduler service.
func New(r schedulerRepo, rr runbook.Repo, opts Options) *Service {
	if opts.TickInterval <= 0 {
		opts.TickInterval = defaultTickInterval
	}
	return &Service{
		repo:        r,
		runbookRepo: rr,
		opts:        opts,
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
	due, err := s.repo.ListDueSchedules(ctx, now)
	if err != nil {
		slog.Warn("scheduler list due schedules failed", "err", err)
		return
	}
	for _, sched := range due {
		s.executeDueSchedule(ctx, sched, now)
	}
}

func (s *Service) executeDueSchedule(ctx context.Context, sched store.OpsSchedule, now time.Time) {
	job, err := s.repo.CreateOpsRunbookRun(ctx, sched.RunbookID, now)
	if err != nil {
		slog.Warn("scheduler create run failed", "schedule", sched.ID, "runbook", sched.RunbookID, "err", err)
		return
	}

	slog.Info("scheduler triggered run", "schedule", sched.ID, "runbook", sched.RunbookID, "job", job.ID)

	// Compute next run and whether to disable.
	nextRunAt, enabled := s.computeNextRun(sched)

	if err := s.repo.UpdateScheduleAfterRun(ctx, sched.ID, now.Format(time.RFC3339), "running", nextRunAt, enabled); err != nil {
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
	runbook.Run(s.runbookRepo, s.emitEvent, runbook.RunParams{
		Job:         job,
		Source:      "scheduler",
		StepTimeout: stepTimeout,
		ExtraMetadata: map[string]string{
			"scheduleId": scheduleID,
		},
		OnFinish: func(ctx context.Context, status string) {
			finished := time.Now().UTC()
			if err := s.repo.UpdateScheduleAfterRun(ctx, scheduleID, finished.Format(time.RFC3339), status, "", true); err != nil {
				slog.Warn("scheduler: update schedule after run", "err", err)
			}
			s.publish(events.TypeScheduleUpdated, map[string]any{
				"action":   "run_completed",
				"schedule": scheduleID,
				"jobId":    job.ID,
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
	schedules, err := s.repo.ListOpsSchedules(ctx)
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
