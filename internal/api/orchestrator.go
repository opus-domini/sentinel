package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/activity"
	"github.com/opus-domini/sentinel/internal/alerts"
	opsplane "github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/store"
)

type opsOrchestratorRepo interface {
	InsertActivityEvent(ctx context.Context, event activity.EventWrite) (activity.Event, error)
	UpsertAlert(ctx context.Context, alert alerts.AlertWrite) (alerts.Alert, error)
	AckAlert(ctx context.Context, id int64, ackAt time.Time) (alerts.Alert, error)
	InsertCustomService(ctx context.Context, svc store.CustomServiceWrite) (store.CustomService, error)
	DeleteCustomService(ctx context.Context, name string) error
}

type opsOrchestrator struct {
	repo opsOrchestratorRepo
}

// RecordServiceAction persists a timeline event for a service action and,
// if the service entered a failed state, upserts an alert.
func (o *opsOrchestrator) RecordServiceAction(ctx context.Context, serviceStatus opsplane.ServiceStatus, action string, at time.Time) (activity.Event, bool, []alerts.Alert, error) {
	if o == nil || o.repo == nil {
		return activity.Event{}, false, nil, nil
	}
	normalizedAction := strings.ToLower(strings.TrimSpace(action))
	state := strings.ToLower(strings.TrimSpace(serviceStatus.ActiveState))
	severity := "info"
	switch {
	case state == stateFailed:
		severity = "error"
	case normalizedAction == opsplane.ActionStop:
		severity = activity.SeverityWarn
	}

	event, err := o.repo.InsertActivityEvent(ctx, activity.EventWrite{
		Source:    "service",
		EventType: "service.action",
		Severity:  severity,
		Resource:  serviceStatus.Name,
		Message:   fmt.Sprintf("%s %s", serviceStatus.DisplayName, normalizedAction),
		Details:   fmt.Sprintf("unit=%s manager=%s scope=%s state=%s", serviceStatus.Unit, serviceStatus.Manager, serviceStatus.Scope, serviceStatus.ActiveState),
		Metadata:  marshalMetadata(map[string]string{"action": normalizedAction, "service": serviceStatus.Name, "manager": serviceStatus.Manager, "scope": serviceStatus.Scope, "state": serviceStatus.ActiveState}),
		CreatedAt: at,
	})
	if err != nil {
		return activity.Event{}, false, nil, err
	}

	firedAlerts := make([]alerts.Alert, 0, 1)
	if state == stateFailed {
		alert, alertErr := o.repo.UpsertAlert(ctx, alerts.AlertWrite{
			DedupeKey: fmt.Sprintf("service:%s:failed", serviceStatus.Name),
			Source:    "service",
			Resource:  serviceStatus.Name,
			Title:     fmt.Sprintf("%s entered failed state", serviceStatus.DisplayName),
			Message:   fmt.Sprintf("%s is failed after %s", serviceStatus.DisplayName, normalizedAction),
			Severity:  "error",
			Metadata:  marshalMetadata(map[string]string{"action": normalizedAction, "service": serviceStatus.Name, "unit": serviceStatus.Unit}),
			CreatedAt: at,
		})
		if alertErr != nil {
			return activity.Event{}, false, nil, alertErr
		}
		firedAlerts = append(firedAlerts, alert)
	}

	return event, true, firedAlerts, nil
}

// AckAlert acknowledges an alert and records a timeline event.
func (o *opsOrchestrator) AckAlert(ctx context.Context, alertID int64, at time.Time) (alerts.Alert, activity.Event, bool, error) {
	if o == nil || o.repo == nil {
		return alerts.Alert{}, activity.Event{}, false, nil
	}
	alert, err := o.repo.AckAlert(ctx, alertID, at)
	if err != nil {
		return alerts.Alert{}, activity.Event{}, false, err
	}
	event, err := o.repo.InsertActivityEvent(ctx, activity.EventWrite{
		Source:    "alert",
		EventType: "alert.acked",
		Severity:  "info",
		Resource:  alert.Resource,
		Message:   fmt.Sprintf("Alert acknowledged: %s", alert.Title),
		Details:   alert.Message,
		Metadata:  marshalMetadata(map[string]any{"alertId": alert.ID, "dedupeKey": alert.DedupeKey}),
		CreatedAt: at,
	})
	if err != nil {
		return alerts.Alert{}, activity.Event{}, false, err
	}
	return alert, event, true, nil
}

// RegisterService persists a custom service and records a timeline event.
func (o *opsOrchestrator) RegisterService(ctx context.Context, svc store.CustomServiceWrite, at time.Time) (activity.Event, error) {
	if o == nil || o.repo == nil {
		return activity.Event{}, nil
	}
	if _, err := o.repo.InsertCustomService(ctx, svc); err != nil {
		return activity.Event{}, err
	}
	te, _ := o.repo.InsertActivityEvent(ctx, activity.EventWrite{
		Source:    "service",
		EventType: "service.registered",
		Severity:  "info",
		Resource:  svc.Name,
		Message:   fmt.Sprintf("Custom service registered: %s", svc.DisplayName),
		Details:   fmt.Sprintf("unit=%s manager=%s scope=%s", svc.Unit, svc.Manager, svc.Scope),
		CreatedAt: at,
	})
	return te, nil
}

// UnregisterService removes a custom service and records a timeline event.
func (o *opsOrchestrator) UnregisterService(ctx context.Context, name string, at time.Time) (activity.Event, error) {
	if o == nil || o.repo == nil {
		return activity.Event{}, nil
	}
	if err := o.repo.DeleteCustomService(ctx, name); err != nil {
		return activity.Event{}, err
	}
	te, _ := o.repo.InsertActivityEvent(ctx, activity.EventWrite{
		Source:    "service",
		EventType: "service.unregistered",
		Severity:  "info",
		Resource:  name,
		Message:   fmt.Sprintf("Custom service removed: %s", name),
		CreatedAt: at,
	})
	return te, nil
}

// RecordRunbookStarted persists a timeline event for a runbook execution start.
func (o *opsOrchestrator) RecordRunbookStarted(ctx context.Context, job store.OpsRunbookRun, at time.Time) (activity.Event, error) {
	if o == nil || o.repo == nil {
		return activity.Event{}, nil
	}
	return o.repo.InsertActivityEvent(ctx, activity.EventWrite{
		Source:    "runbook",
		EventType: "runbook.started",
		Severity:  "info",
		Resource:  job.RunbookID,
		Message:   fmt.Sprintf("Runbook started: %s", job.RunbookName),
		Details:   fmt.Sprintf("job=%s steps=%d", job.ID, job.TotalSteps),
		Metadata:  marshalMetadata(map[string]string{"jobId": job.ID, "runbookId": job.RunbookID, "status": job.Status}),
		CreatedAt: at,
	})
}

// RecordConfigUpdated persists a timeline event for a configuration file update.
func (o *opsOrchestrator) RecordConfigUpdated(ctx context.Context, at time.Time) (activity.Event, error) {
	if o == nil || o.repo == nil {
		return activity.Event{}, nil
	}
	return o.repo.InsertActivityEvent(ctx, activity.EventWrite{
		Source:    "config",
		EventType: "config.updated",
		Severity:  "info",
		Resource:  "config.toml",
		Message:   "Configuration file updated via API",
		CreatedAt: at,
	})
}
