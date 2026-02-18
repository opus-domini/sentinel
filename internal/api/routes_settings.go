package api

import "net/http"

func (h *Handler) registerSettingsRoutes(mux *http.ServeMux) {
	h.registerRoutes(mux, []routeBinding{
		{pattern: "GET /api/ops/config", handler: h.opsConfig},
		{pattern: "PATCH /api/ops/config", handler: h.patchOpsConfig},
		{pattern: "GET /api/ops/storage/stats", handler: h.storageStats},
		{pattern: "POST /api/ops/storage/flush", handler: h.flushStorage},
	})
}
