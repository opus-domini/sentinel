package api

import "net/http"

func (h *Handler) registerMetricsRoutes(mux *http.ServeMux) {
	h.registerRoutes(mux, []routeBinding{
		{pattern: "GET /api/ops/metrics", handler: h.opsMetrics},
	})
}
