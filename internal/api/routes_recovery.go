package api

import "net/http"

func (h *Handler) registerRecoveryRoutes(mux *http.ServeMux) {
	h.registerRoutes(mux, []routeBinding{
		{pattern: "GET /api/recovery/overview", handler: h.recoveryOverview},
		{pattern: "GET /api/recovery/sessions", handler: h.listRecoverySessions},
		{pattern: "POST /api/recovery/sessions/{session}/archive", handler: h.archiveRecoverySession},
		{pattern: "GET /api/recovery/sessions/{session}/snapshots", handler: h.listRecoverySnapshots},
		{pattern: "GET /api/recovery/snapshots/{snapshot}", handler: h.getRecoverySnapshot},
		{pattern: "POST /api/recovery/snapshots/{snapshot}/restore", handler: h.restoreRecoverySnapshot},
		{pattern: "GET /api/recovery/jobs/{job}", handler: h.getRecoveryJob},
	})
}
