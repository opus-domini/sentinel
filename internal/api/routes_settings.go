package api

import "net/http"

func (h *Handler) registerSettingsRoutes(mux *http.ServeMux) {
	h.registerRoutes(mux, []routeBinding{
		{pattern: "GET /api/ops/config", handler: h.opsConfig},
		{pattern: "PATCH /api/ops/config", handler: h.patchOpsConfig},
		{pattern: "PATCH /api/ops/settings/timezone", handler: h.patchTimezone},
		{pattern: "PATCH /api/ops/settings/locale", handler: h.patchLocale},
		{pattern: "GET /api/ops/settings/webhook", handler: h.getWebhookSettings},
		{pattern: "PATCH /api/ops/settings/webhook", handler: h.patchWebhookSettings},
		{pattern: "POST /api/ops/webhook/test", handler: h.testWebhook},
		{pattern: "GET /api/ops/storage/stats", handler: h.storageStats},
		{pattern: "POST /api/ops/storage/flush", handler: h.flushStorage},
	})
}
