package api

import "net/http"

func (h *Handler) registerAlertsRoutes(mux *http.ServeMux) {
	h.registerRoutes(mux, []routeBinding{
		{pattern: "GET /api/ops/alerts", handler: h.opsAlerts},
		{pattern: "POST /api/ops/alerts/{alert}/ack", handler: h.ackOpsAlert},
	})
}
