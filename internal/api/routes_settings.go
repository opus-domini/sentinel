package api

import "net/http"

func (h *Handler) registerSettingsRoutes(mux *http.ServeMux) {
	h.registerRoutes(mux, []routeBinding{
		{pattern: "GET /api/ops/settings", handler: h.getSettings},
		{pattern: "PATCH /api/ops/settings", handler: h.patchSettings},
		{pattern: "GET /api/ops/storage/stats", handler: h.storageStats},
		{pattern: "POST /api/ops/storage/flush", handler: h.flushStorage},
	})
}
