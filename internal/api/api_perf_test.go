package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/store"
)

func BenchmarkPerfRouteMeta(b *testing.B) {
	mux := newPerfMux(b)
	req := httptest.NewRequest(http.MethodGet, "/api/meta", nil)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("unexpected status: %d", rec.Code)
		}
	}
}

func BenchmarkPerfRouteOpsAlerts(b *testing.B) {
	mux := newPerfMux(b)
	req := httptest.NewRequest(http.MethodGet, "/api/ops/alerts?limit=20", nil)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			b.Fatalf("alerts route is not mounted")
		}
	}
}

func newPerfMux(tb testing.TB) *http.ServeMux {
	tb.Helper()
	st, err := store.New(filepath.Join(tb.TempDir(), "perf.db"))
	if err != nil {
		tb.Fatalf("store.New() error = %v", err)
	}
	tb.Cleanup(func() { _ = st.Close() })

	mux := http.NewServeMux()
	Register(
		mux,
		security.New("", nil, security.CookieSecureAuto),
		st,
		&mockOpsControlPlane{},
		&mockRecovery{},
		events.NewHub(),
		"test",
		"",
	)
	return mux
}
