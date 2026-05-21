package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/opus-domini/sentinel/internal/events"
	opsplane "github.com/opus-domini/sentinel/internal/services"
)

// ---------------------------------------------------------------------------
// ops control-plane error paths
// ---------------------------------------------------------------------------

func TestOpsServiceStatusErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("rejects empty service name", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/ops/services//status", nil)
		h.opsServiceStatus(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("returns 404 for unknown service", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		h.ops = &mockOpsControlPlane{
			inspectFn: func(context.Context, string) (opsplane.ServiceInspect, error) {
				return opsplane.ServiceInspect{}, opsplane.ErrServiceNotFound
			},
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/ops/services/missing/status", nil)
		r.SetPathValue("service", "missing")
		h.opsServiceStatus(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("returns 500 on inspect failure", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		h.ops = &mockOpsControlPlane{
			inspectFn: func(context.Context, string) (opsplane.ServiceInspect, error) {
				return opsplane.ServiceInspect{}, errors.New("boom")
			},
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/ops/services/api/status", nil)
		r.SetPathValue("service", "api")
		h.opsServiceStatus(w, r)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})
}

func TestOpsServiceLogsErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("rejects empty service name", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/ops/services//logs", nil)
		h.opsServiceLogs(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("returns 404 for unknown service", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		h.ops = &mockOpsControlPlane{
			logsFn: func(context.Context, string, int) (string, error) {
				return "", opsplane.ErrServiceNotFound
			},
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/ops/services/missing/logs", nil)
		r.SetPathValue("service", "missing")
		h.opsServiceLogs(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("returns 500 on logs failure", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		h.ops = &mockOpsControlPlane{
			logsFn: func(context.Context, string, int) (string, error) {
				return "", errors.New("boom")
			},
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/ops/services/api/logs", nil)
		r.SetPathValue("service", "api")
		h.opsServiceLogs(w, r)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})
}

func TestBrowseOpsServicesError(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{
		browseFn: func(context.Context) ([]opsplane.BrowsedService, error) {
			return nil, errors.New("boom")
		},
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/ops/services/browse", nil)
	h.browseOpsServices(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestOpsServiceActionErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/services/api/action", strings.NewReader(`bad`))
		r.SetPathValue("service", "api")
		h.opsServiceAction(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("rejects empty service name", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/services//action", strings.NewReader(`{"action":"start"}`))
		h.opsServiceAction(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("returns 500 on action failure", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		h.ops = &mockOpsControlPlane{
			actFn: func(context.Context, string, string) (opsplane.ServiceStatus, error) {
				return opsplane.ServiceStatus{}, errors.New("boom")
			},
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/services/api/action", strings.NewReader(`{"action":"restart"}`))
		r.SetPathValue("service", "api")
		h.opsServiceAction(w, r)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})
}

func TestRegisterOpsServiceErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/services", strings.NewReader(`bad`))
		h.registerOpsService(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("rejects missing name", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/services", strings.NewReader(`{"name":"","unit":"x.service"}`))
		h.registerOpsService(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("rejects missing unit", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/services", strings.NewReader(`{"name":"api","unit":""}`))
		h.registerOpsService(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("rejects duplicate registration", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		h.events = events.NewHub()
		body := `{"name":"api","unit":"api.service","manager":"systemd","scope":"system"}`

		first := httptest.NewRecorder()
		h.registerOpsService(first, httptest.NewRequest(http.MethodPost, "/api/ops/services", strings.NewReader(body)))
		if first.Code != http.StatusCreated {
			t.Fatalf("first register status = %d, want 201; body=%s", first.Code, first.Body.String())
		}

		second := httptest.NewRecorder()
		h.registerOpsService(second, httptest.NewRequest(http.MethodPost, "/api/ops/services", strings.NewReader(body)))
		if second.Code != http.StatusConflict {
			t.Fatalf("duplicate register status = %d, want 409; body=%s", second.Code, second.Body.String())
		}
	})
}

func TestUnregisterOpsServiceErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("rejects empty service name", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/ops/services/", nil)
		h.unregisterOpsService(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("returns 404 for unknown service", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/ops/services/missing", nil)
		r.SetPathValue("service", "missing")
		h.unregisterOpsService(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})
}

func TestOpsUnitActionErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/units/action", strings.NewReader(`bad`))
		h.opsUnitAction(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("rejects bad manager", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/units/action", strings.NewReader(`{"unit":"x.service","action":"start","manager":"upstart","scope":"system"}`))
		h.opsUnitAction(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("rejects bad scope", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/units/action", strings.NewReader(`{"unit":"x.service","action":"start","manager":"systemd","scope":"global"}`))
		h.opsUnitAction(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("returns 500 on action failure", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		h.ops = &mockOpsControlPlane{
			actByUnitFn: func(context.Context, string, string, string, string) error {
				return errors.New("boom")
			},
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/units/action", strings.NewReader(`{"unit":"x.service","action":"start","manager":"systemd","scope":"system"}`))
		h.opsUnitAction(w, r)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})

	t.Run("maps invalid action error to 400", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		h.ops = &mockOpsControlPlane{
			actByUnitFn: func(context.Context, string, string, string, string) error {
				return opsplane.ErrInvalidAction
			},
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/units/action", strings.NewReader(`{"unit":"x.service","action":"start","manager":"systemd","scope":"system"}`))
		h.opsUnitAction(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// nil ops control plane guards
// ---------------------------------------------------------------------------

func TestOpsHandlersRejectNilControlPlane(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		call func(h *Handler, w http.ResponseWriter, r *http.Request)
		req  func() *http.Request
	}{
		{
			name: "service status",
			call: (*Handler).opsServiceStatus,
			req:  func() *http.Request { return httptest.NewRequest(http.MethodGet, "/api/ops/services/api/status", nil) },
		},
		{
			name: "service logs",
			call: (*Handler).opsServiceLogs,
			req:  func() *http.Request { return httptest.NewRequest(http.MethodGet, "/api/ops/services/api/logs", nil) },
		},
		{
			name: "browse services",
			call: (*Handler).browseOpsServices,
			req:  func() *http.Request { return httptest.NewRequest(http.MethodGet, "/api/ops/services/browse", nil) },
		},
		{
			name: "service action",
			call: (*Handler).opsServiceAction,
			req: func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/api/ops/services/api/action", strings.NewReader(`{"action":"start"}`))
			},
		},
		{
			name: "unit action",
			call: (*Handler).opsUnitAction,
			req: func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/api/ops/units/action", strings.NewReader(`{}`))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, _ := newTestHandler(t, nil, nil)
			h.ops = nil
			w := httptest.NewRecorder()
			tc.call(h, w, tc.req())
			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", w.Code)
			}
		})
	}
}
