package api

import "net/http"

func (h *Handler) registerActivityRoutes(mux *http.ServeMux) {
	h.registerRoutes(mux, []routeBinding{
		{pattern: "GET /api/ops/activity", handler: h.opsActivity},
	})
}
