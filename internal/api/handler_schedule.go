package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/validate"
)

func (h *Handler) listSchedules(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	schedules, err := h.repo.ListOpsSchedules(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load schedules", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"schedules": schedules,
	})
}

func (h *Handler) createSchedule(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}

	var req struct {
		RunbookID    string `json:"runbookId"`
		Name         string `json:"name"`
		ScheduleType string `json:"scheduleType"`
		CronExpr     string `json:"cronExpr"`
		Timezone     string `json:"timezone"`
		RunAt        string `json:"runAt"`
		Enabled      bool   `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	if strings.TrimSpace(req.RunbookID) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "runbookId is required", nil)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "name is required", nil)
		return
	}
	if req.ScheduleType != scheduleTypeCron && req.ScheduleType != scheduleTypeOnce {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "scheduleType must be \"cron\" or \"once\"", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	nextRunAt, err := validateScheduleRequest(ctx, h.repo, req.RunbookID, req.ScheduleType, req.CronExpr, req.Timezone, req.RunAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if req.Timezone == "" && req.ScheduleType == scheduleTypeCron {
		req.Timezone = "UTC" //nolint:goconst // UTC is clearer inline than a constant
	}

	schedule, err := h.repo.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    req.RunbookID,
		Name:         req.Name,
		ScheduleType: req.ScheduleType,
		CronExpr:     req.CronExpr,
		Timezone:     req.Timezone,
		RunAt:        req.RunAt,
		Enabled:      req.Enabled,
		NextRunAt:    nextRunAt,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to create schedule", nil)
		return
	}

	h.emit(events.TypeScheduleUpdated, map[string]any{
		"action":   "created",
		"schedule": schedule,
	})

	writeData(w, http.StatusCreated, map[string]any{
		"schedule": schedule,
	})
}

func (h *Handler) updateSchedule(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	scheduleID := strings.TrimSpace(r.PathValue("schedule"))
	if scheduleID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "schedule id is required", nil)
		return
	}

	var req struct {
		RunbookID    string `json:"runbookId"`
		Name         string `json:"name"`
		ScheduleType string `json:"scheduleType"`
		CronExpr     string `json:"cronExpr"`
		Timezone     string `json:"timezone"`
		RunAt        string `json:"runAt"`
		Enabled      bool   `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	if strings.TrimSpace(req.RunbookID) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "runbookId is required", nil)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "name is required", nil)
		return
	}
	if req.ScheduleType != scheduleTypeCron && req.ScheduleType != scheduleTypeOnce {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "scheduleType must be \"cron\" or \"once\"", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	nextRunAt, err := validateScheduleRequest(ctx, h.repo, req.RunbookID, req.ScheduleType, req.CronExpr, req.Timezone, req.RunAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if req.Timezone == "" && req.ScheduleType == scheduleTypeCron {
		req.Timezone = "UTC"
	}

	schedule, err := h.repo.UpdateOpsSchedule(ctx, store.OpsScheduleWrite{
		ID:           scheduleID,
		RunbookID:    req.RunbookID,
		Name:         req.Name,
		ScheduleType: req.ScheduleType,
		CronExpr:     req.CronExpr,
		Timezone:     req.Timezone,
		RunAt:        req.RunAt,
		Enabled:      req.Enabled,
		NextRunAt:    nextRunAt,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "schedule not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to update schedule", nil)
		return
	}

	h.emit(events.TypeScheduleUpdated, map[string]any{
		"action":   "updated",
		"schedule": schedule,
	})

	writeData(w, http.StatusOK, map[string]any{
		"schedule": schedule,
	})
}

func (h *Handler) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	scheduleID := strings.TrimSpace(r.PathValue("schedule"))
	if scheduleID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "schedule id is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.repo.DeleteOpsSchedule(ctx, scheduleID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "schedule not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to delete schedule", nil)
		return
	}

	h.emit(events.TypeScheduleUpdated, map[string]any{
		"action":  "deleted",
		"removed": scheduleID,
	})

	writeData(w, http.StatusOK, map[string]any{
		"removed": scheduleID,
	})
}

func (h *Handler) triggerSchedule(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	scheduleID := strings.TrimSpace(r.PathValue("schedule"))
	if scheduleID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "schedule id is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	// Fetch the schedule to get the runbook ID.
	schedules, err := h.repo.ListOpsSchedules(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load schedules", nil)
		return
	}
	var sched *store.OpsSchedule
	for i := range schedules {
		if schedules[i].ID == scheduleID {
			sched = &schedules[i]
			break
		}
	}
	if sched == nil {
		writeError(w, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "schedule not found", nil)
		return
	}

	now := time.Now().UTC()
	job, err := h.repo.CreateOpsRunbookRun(ctx, sched.RunbookID, now)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "OPS_RUNBOOK_NOT_FOUND", "runbook not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to trigger schedule run", nil)
		return
	}

	// Update schedule last run info.
	if err := h.repo.UpdateScheduleAfterRun(ctx, scheduleID, now.Format(time.RFC3339), "running", sched.NextRunAt, sched.Enabled); err != nil {
		slog.Warn("trigger schedule: update after run failed", "schedule", scheduleID, "err", err)
	}

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		h.executeRunbookAsync(h.runCtx, job)
	}()

	globalRev := now.UnixMilli()
	h.emit(events.TypeOpsJob, map[string]any{
		"globalRev": globalRev,
		"job":       job,
	})
	h.emit(events.TypeScheduleUpdated, map[string]any{
		"action":   "triggered",
		"schedule": scheduleID,
		"jobId":    job.ID,
	})

	writeData(w, http.StatusAccepted, map[string]any{
		"job": job,
	})
}

// validateScheduleRequest checks runbook existence, parses cron/once fields,
// and returns the computed nextRunAt. It returns a user-facing error message
// on any validation failure.
type runbookLookup interface {
	GetOpsRunbook(ctx context.Context, id string) (store.OpsRunbook, error)
}

func validateScheduleRequest(ctx context.Context, repo runbookLookup, runbookID, scheduleType, cronExpr, timezone, runAt string) (string, error) {
	if _, err := repo.GetOpsRunbook(ctx, runbookID); err != nil {
		return "", fmt.Errorf("runbook not found")
	}

	switch scheduleType {
	case scheduleTypeCron:
		if err := validate.CronExpression(cronExpr); err != nil {
			return "", fmt.Errorf("invalid cron expression")
		}
		tz := timezone
		if tz == "" {
			tz = "UTC"
		}
		if err := validate.Timezone(tz); err != nil {
			return "", fmt.Errorf("invalid timezone")
		}
		loc, locErr := time.LoadLocation(tz)
		if locErr != nil {
			return "", fmt.Errorf("invalid timezone")
		}
		sched, cronErr := validate.ParseCron(cronExpr)
		if cronErr != nil {
			return "", fmt.Errorf("invalid cron expression")
		}
		return sched.Next(time.Now().In(loc)).UTC().Format(time.RFC3339), nil
	case scheduleTypeOnce:
		parsed, parseErr := time.Parse(time.RFC3339, runAt)
		if parseErr != nil {
			return "", fmt.Errorf("runAt must be a valid RFC3339 timestamp")
		}
		if !parsed.After(time.Now().UTC()) {
			return "", fmt.Errorf("runAt must be in the future")
		}
		return parsed.UTC().Format(time.RFC3339), nil
	default:
		return "", fmt.Errorf("scheduleType must be \"cron\" or \"once\"")
	}
}
