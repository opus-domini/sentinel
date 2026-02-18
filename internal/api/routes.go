package api

import "net/http"

type routeBinding struct {
	pattern string
	handler http.HandlerFunc
}

func (h *Handler) registerRoutes(mux *http.ServeMux, routes []routeBinding) {
	for _, route := range routes {
		mux.HandleFunc(route.pattern, h.wrap(route.handler))
	}
}
