//go:build contract

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/security"
)

type contractRoute struct {
	name   string
	method string
	path   string
	body   string
}

func TestContractRoutesAreMountedByFeature(t *testing.T) {
	t.Parallel()

	mux := newContractMux(t)
	routes := []contractRoute{
		{name: "meta", method: http.MethodGet, path: "/api/meta"},
		{name: "dirs", method: http.MethodGet, path: "/api/fs/dirs?prefix=/tmp"},

		{name: "tmux-sessions", method: http.MethodGet, path: "/api/tmux/sessions"},
		{name: "tmux-create", method: http.MethodPost, path: "/api/tmux/sessions", body: `{"name":"dev","cwd":"/tmp"}`},
		{name: "tmux-rename", method: http.MethodPatch, path: "/api/tmux/sessions/dev", body: `{"newName":"dev2"}`},
		{name: "tmux-delete", method: http.MethodDelete, path: "/api/tmux/sessions/dev"},
		{name: "tmux-windows", method: http.MethodGet, path: "/api/tmux/sessions/dev/windows"},
		{name: "tmux-panes", method: http.MethodGet, path: "/api/tmux/sessions/dev/panes"},
		{name: "tmux-activity-delta", method: http.MethodGet, path: "/api/tmux/activity/delta"},
		{name: "tmux-activity-stats", method: http.MethodGet, path: "/api/tmux/activity/stats"},
		{name: "tmux-timeline", method: http.MethodGet, path: "/api/tmux/timeline"},
		{name: "tmux-mark-seen", method: http.MethodPost, path: "/api/tmux/sessions/dev/seen", body: `{"scope":"session"}`},

		{name: "ops-overview", method: http.MethodGet, path: "/api/ops/overview"},
		{name: "ops-services", method: http.MethodGet, path: "/api/ops/services"},
		{name: "ops-service-status", method: http.MethodGet, path: "/api/ops/services/sentinel/status"},
		{name: "ops-service-action", method: http.MethodPost, path: "/api/ops/services/sentinel/action", body: `{"action":"restart"}`},
		{name: "ops-services-browse", method: http.MethodGet, path: "/api/ops/services/browse"},
		{name: "ops-services-discover", method: http.MethodGet, path: "/api/ops/services/discover"},
		{name: "ops-unit-status", method: http.MethodGet, path: "/api/ops/services/unit/status?unit=ssh.service&scope=system&manager=systemd"},
		{name: "ops-unit-logs", method: http.MethodGet, path: "/api/ops/services/unit/logs?unit=ssh.service&scope=system&manager=systemd"},
		{name: "ops-unit-action", method: http.MethodPost, path: "/api/ops/services/unit/action", body: `{"unit":"ssh.service","scope":"system","manager":"systemd","action":"restart"}`},

		{name: "alerts-list", method: http.MethodGet, path: "/api/ops/alerts"},
		{name: "alerts-ack", method: http.MethodPost, path: "/api/ops/alerts/1/ack"},
		{name: "alerts-delete", method: http.MethodDelete, path: "/api/ops/alerts/1"},
		{name: "activity", method: http.MethodGet, path: "/api/ops/activity"},
		{name: "metrics", method: http.MethodGet, path: "/api/ops/metrics"},

		{name: "runbooks-list", method: http.MethodGet, path: "/api/ops/runbooks"},
		{name: "runbooks-create", method: http.MethodPost, path: "/api/ops/runbooks", body: `{"id":"noop","name":"Noop","description":"noop","steps":[{"type":"command","title":"echo","command":"echo ok"}]}`},
		{name: "runbooks-update", method: http.MethodPut, path: "/api/ops/runbooks/noop", body: `{"name":"Noop","description":"noop","steps":[{"type":"command","title":"echo","command":"echo ok"}]}`},
		{name: "runbooks-delete", method: http.MethodDelete, path: "/api/ops/runbooks/noop"},
		{name: "runbooks-run", method: http.MethodPost, path: "/api/ops/runbooks/noop/run", body: `{"trigger":"manual"}`},
		{name: "runbooks-job", method: http.MethodGet, path: "/api/ops/jobs/noop"},
		{name: "runbooks-job-delete", method: http.MethodDelete, path: "/api/ops/jobs/noop"},
		{name: "schedules-list", method: http.MethodGet, path: "/api/ops/schedules"},
		{name: "schedules-create", method: http.MethodPost, path: "/api/ops/schedules", body: `{"runbookID":"noop","scheduleType":"once","timezone":"UTC","runAt":"2030-01-01T00:00:00Z","enabled":true}`},
		{name: "schedules-update", method: http.MethodPut, path: "/api/ops/schedules/noop", body: `{"runbookID":"noop","scheduleType":"once","timezone":"UTC","runAt":"2030-01-01T00:00:00Z","enabled":true}`},
		{name: "schedules-delete", method: http.MethodDelete, path: "/api/ops/schedules/noop"},
		{name: "schedules-trigger", method: http.MethodPost, path: "/api/ops/schedules/noop/trigger"},

		{name: "config-get", method: http.MethodGet, path: "/api/ops/config"},
		{name: "config-patch", method: http.MethodPatch, path: "/api/ops/config", body: `{"logLevel":"info"}`},
		{name: "storage-stats", method: http.MethodGet, path: "/api/ops/storage/stats"},
		{name: "storage-flush", method: http.MethodPost, path: "/api/ops/storage/flush", body: `{"resource":"timeline"}`},

		{name: "guardrails-rules", method: http.MethodGet, path: "/api/ops/guardrails/rules"},
		{name: "guardrails-create", method: http.MethodPost, path: "/api/ops/guardrails/rules", body: `{"pattern":"^test$","enabled":true}`},
		{name: "guardrails-update", method: http.MethodPatch, path: "/api/ops/guardrails/rules/delete-session", body: `{"enabled":true}`},
		{name: "guardrails-delete", method: http.MethodDelete, path: "/api/ops/guardrails/rules/delete-session"},
		{name: "guardrails-audit", method: http.MethodGet, path: "/api/ops/guardrails/audit"},
		{name: "guardrails-evaluate", method: http.MethodPost, path: "/api/ops/guardrails/evaluate", body: `{"action":"kill-session","args":{"session":"dev"}}`},

		{name: "recovery-overview", method: http.MethodGet, path: "/api/recovery/overview"},
		{name: "recovery-sessions", method: http.MethodGet, path: "/api/recovery/sessions"},
		{name: "recovery-archive", method: http.MethodPost, path: "/api/recovery/sessions/dev/archive"},
		{name: "recovery-snapshots", method: http.MethodGet, path: "/api/recovery/sessions/dev/snapshots"},
		{name: "recovery-snapshot", method: http.MethodGet, path: "/api/recovery/snapshots/1"},
		{name: "recovery-restore", method: http.MethodPost, path: "/api/recovery/snapshots/1/restore", body: `{}`},
		{name: "recovery-job", method: http.MethodGet, path: "/api/recovery/jobs/noop"},
	}

	for _, tc := range routes {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var bodyReader *strings.Reader
			if tc.body == "" {
				bodyReader = strings.NewReader("")
			} else {
				bodyReader = strings.NewReader(tc.body)
			}
			req := httptest.NewRequest(tc.method, tc.path, bodyReader)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			notFoundByMux := rec.Code == http.StatusNotFound &&
				strings.Contains(rec.Body.String(), "404 page not found") &&
				!strings.Contains(rec.Header().Get("Content-Type"), "application/json")
			if notFoundByMux {
				t.Fatalf("route %s %s is not mounted", tc.method, tc.path)
			}
		})
	}
}

func newContractMux(t *testing.T) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	Register(
		mux,
		security.New("", nil, security.CookieSecureAuto),
		newTestStore(t),
		&mockOpsControlPlane{},
		&mockRecovery{},
		events.NewHub(),
		"test",
		"",
	)
	return mux
}
