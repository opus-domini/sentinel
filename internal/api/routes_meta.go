package api

import "net/http"

func (h *Handler) registerMetaRoutes(mux *http.ServeMux) {
	h.registerRoutes(mux, []routeBinding{
		{pattern: "GET /api/meta", handler: h.meta},
		{pattern: "GET /api/fs/dirs", handler: h.listDirectories},
	})
}
