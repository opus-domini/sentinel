package api

import "net/http"

func (h *Handler) registerTimelineRoutes(mux *http.ServeMux) {
	h.registerRoutes(mux, []routeBinding{
		{pattern: "GET /api/ops/timeline", handler: h.opsTimeline},
	})
}
