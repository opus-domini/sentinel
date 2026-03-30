package api

import "net/http"

func (h *Handler) registerTmuxRoutes(mux *http.ServeMux) {
	h.registerRoutes(mux, []routeBinding{
		{pattern: "GET /api/tmux/sessions", handler: h.listSessions},
		{pattern: "POST /api/tmux/sessions", handler: h.createSession},
		{pattern: "PATCH /api/tmux/sessions/order", handler: h.reorderSessions},
		{pattern: "GET /api/tmux/session-presets", handler: h.listSessionPresets},
		{pattern: "POST /api/tmux/session-presets", handler: h.createSessionPreset},
		{pattern: "PATCH /api/tmux/session-presets/order", handler: h.reorderSessionPresets},
		{pattern: "PATCH /api/tmux/session-presets/{preset}", handler: h.updateSessionPreset},
		{pattern: "DELETE /api/tmux/session-presets/{preset}", handler: h.deleteSessionPreset},
		{pattern: "POST /api/tmux/session-presets/{preset}/launch", handler: h.launchSessionPreset},
		{pattern: "PATCH /api/tmux/sessions/{session}", handler: h.renameSession},
		{pattern: "DELETE /api/tmux/sessions/{session}", handler: h.deleteSession},
		{pattern: "PATCH /api/tmux/sessions/{session}/icon", handler: h.setSessionIcon},
		{pattern: "POST /api/tmux/sessions/{session}/rename-window", handler: h.renameWindow},
		{pattern: "POST /api/tmux/sessions/{session}/rename-pane", handler: h.renamePane},
		{pattern: "POST /api/tmux/sessions/{session}/select-window", handler: h.selectWindow},
		{pattern: "POST /api/tmux/sessions/{session}/select-pane", handler: h.selectPane},
		{pattern: "POST /api/tmux/sessions/{session}/new-window", handler: h.newWindow},
		{pattern: "POST /api/tmux/sessions/{session}/kill-window", handler: h.killWindow},
		{pattern: "POST /api/tmux/sessions/{session}/kill-pane", handler: h.killPane},
		{pattern: "POST /api/tmux/sessions/{session}/split-pane", handler: h.splitPane},
		{pattern: "GET /api/tmux/sessions/{session}/windows", handler: h.listWindows},
		{pattern: "GET /api/tmux/sessions/{session}/panes", handler: h.listPanes},
		{pattern: "POST /api/tmux/sessions/{session}/seen", handler: h.markSessionSeen},
		{pattern: "PUT /api/tmux/presence", handler: h.setTmuxPresence},
		{pattern: "GET /api/tmux/frequent-dirs", handler: h.frequentDirectories},
		{pattern: "GET /api/tmux/activity/delta", handler: h.activityDelta},
		{pattern: "GET /api/tmux/activity/stats", handler: h.activityStats},
		{pattern: "GET /api/tmux/timeline", handler: h.timelineSearch},
	})
}
