package api

import "net/http"

func (h *Handler) registerGuardrailsRoutes(mux *http.ServeMux) {
	h.registerRoutes(mux, []routeBinding{
		{pattern: "GET /api/ops/guardrails/rules", handler: h.listGuardrailRules},
		{pattern: "POST /api/ops/guardrails/rules", handler: h.createGuardrailRule},
		{pattern: "PATCH /api/ops/guardrails/rules/{rule}", handler: h.updateGuardrailRule},
		{pattern: "DELETE /api/ops/guardrails/rules/{rule}", handler: h.deleteGuardrailRule},
		{pattern: "GET /api/ops/guardrails/audit", handler: h.listGuardrailAudit},
		{pattern: "POST /api/ops/guardrails/evaluate", handler: h.evaluateGuardrail},
	})
}
