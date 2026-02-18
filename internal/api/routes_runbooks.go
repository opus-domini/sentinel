package api

import "net/http"

func (h *Handler) registerRunbooksRoutes(mux *http.ServeMux) {
	h.registerRoutes(mux, []routeBinding{
		{pattern: "GET /api/ops/runbooks", handler: h.opsRunbooks},
		{pattern: "POST /api/ops/runbooks", handler: h.createOpsRunbook},
		{pattern: "PUT /api/ops/runbooks/{runbook}", handler: h.updateOpsRunbook},
		{pattern: "DELETE /api/ops/runbooks/{runbook}", handler: h.deleteOpsRunbook},
		{pattern: "POST /api/ops/runbooks/{runbook}/run", handler: h.runOpsRunbook},
		{pattern: "GET /api/ops/jobs/{job}", handler: h.opsJob},
		{pattern: "DELETE /api/ops/jobs/{job}", handler: h.deleteOpsJob},
		{pattern: "GET /api/ops/schedules", handler: h.listSchedules},
		{pattern: "POST /api/ops/schedules", handler: h.createSchedule},
		{pattern: "PUT /api/ops/schedules/{schedule}", handler: h.updateSchedule},
		{pattern: "DELETE /api/ops/schedules/{schedule}", handler: h.deleteSchedule},
		{pattern: "POST /api/ops/schedules/{schedule}/trigger", handler: h.triggerSchedule},
	})
}
