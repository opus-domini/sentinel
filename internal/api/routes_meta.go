package api

import "net/http"

func (h *Handler) registerMetaRoutes(mux *http.ServeMux) {
	h.registerPublicRoutes(mux, []routeBinding{
		{pattern: "PUT /api/auth/token", handler: h.setAuthToken},
		{pattern: "DELETE /api/auth/token", handler: h.clearAuthToken},
	})

	h.registerRoutes(mux, []routeBinding{
		{pattern: "GET /api/meta", handler: h.meta},
		{pattern: "GET /api/fs/dirs", handler: h.listDirectories},
	})
}
