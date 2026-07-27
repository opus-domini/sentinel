package api

import "net/http"

func (h *Handler) registerNowRoutes(mux *http.ServeMux) {
	h.registerRoutes(mux, []routeBinding{
		{pattern: "GET /api/now", handler: h.now},
		{pattern: "POST /api/now/services/{service}/runbook", handler: h.runNowServiceRunbook},
	})
}
