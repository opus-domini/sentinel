package api

import "net/http"

func (h *Handler) registerServicesRoutes(mux *http.ServeMux) {
	h.registerRoutes(mux, []routeBinding{
		{pattern: "GET /api/ops/overview", handler: h.opsOverview},
		{pattern: "GET /api/ops/services", handler: h.opsServices},
		{pattern: "POST /api/ops/services", handler: h.registerOpsService},
		{pattern: "DELETE /api/ops/services/{service}", handler: h.unregisterOpsService},
		{pattern: "GET /api/ops/services/browse", handler: h.browseOpsServices},
		{pattern: "GET /api/ops/services/discover", handler: h.discoverOpsServices},
		{pattern: "GET /api/ops/services/{service}/status", handler: h.opsServiceStatus},
		{pattern: "POST /api/ops/services/{service}/action", handler: h.opsServiceAction},
		{pattern: "GET /api/ops/services/{service}/logs", handler: h.opsServiceLogs},
		{pattern: "POST /api/ops/services/unit/action", handler: h.opsUnitAction},
		{pattern: "GET /api/ops/services/unit/status", handler: h.opsUnitStatus},
		{pattern: "GET /api/ops/services/unit/logs", handler: h.opsUnitLogs},
	})
}
