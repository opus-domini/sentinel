package server

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"time"
)

// requestIDKey is the context key for the request ID.
type requestIDKey struct{}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := generateRequestID()
		ctx := context.WithValue(r.Context(), requestIDKey{}, rid)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", rid)

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		defer func() {
			p := recover()
			if p == nil {
				return
			}
			// http.ErrAbortHandler is an intentional abort; net/http expects it
			// to propagate so it can suppress the connection without logging.
			if p == http.ErrAbortHandler { //nolint:errorlint // recover() value, sentinel identity compare matches net/http
				panic(p)
			}
			slog.Error("request panic recovered", "method", r.Method, "path", r.URL.Path, "request_id", rid, "panic", p, "stack", string(debug.Stack()))
			// Only emit a 500 if the response is still ours: after a Hijack
			// (WebSocket) or a written header, the connection is no longer
			// writable as an HTTP response.
			if !rec.wroteHeader && !rec.hijacked {
				rec.WriteHeader(http.StatusInternalServerError)
			}
		}()
		rec.ServeHTTP(next, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(start).Truncate(time.Millisecond), "request_id", rid)
	})
}

// statusRecorder wraps http.ResponseWriter to capture the status code.
// Unwrap returns the underlying ResponseWriter so net/http can discover
// http.Hijacker (needed for WebSocket upgrade) via interface assertion.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	hijacked    bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.wroteHeader = true
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap exposes the underlying ResponseWriter so http.ResponseController
// and net/http can discover interfaces like http.Hijacker.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// Hijack implements http.Hijacker so that direct type assertions
// (e.g. w.(http.Hijacker)) work through the wrapper. This is required
// for WebSocket upgrade in internal/ws which uses a direct assertion.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := r.ResponseWriter.(http.Hijacker); ok {
		conn, rw, err := hj.Hijack()
		if err == nil {
			r.hijacked = true
		}
		return conn, rw, err
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

func (r *statusRecorder) ServeHTTP(next http.Handler, req *http.Request) {
	next.ServeHTTP(r, req)
}

func generateRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}
