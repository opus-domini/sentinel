// Package ui serves the embedded Sentinel frontend.
package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

const manifestAppName = "Sentinel"

// errBundleMissing is returned when the SPA bundle was not built into the
// binary (only the committed .gitkeep is present). It maps to a 503 so the
// frontend not-built state is reported as a transient service condition.
var errBundleMissing = errors.New("frontend bundle not built")

// spa serves the compiled single-page app from a parameterized file system.
// It is constructed from the embedded dist tree in production, but accepts any
// fs.FS so the asset, manifest and SPA-routing logic is unit-testable against
// an in-memory file system.
type spa struct {
	dist fs.FS
}

// newSPA roots a spa at the "dist" subtree of the provided file system. It
// returns errBundleMissing when the subtree is absent — i.e. the frontend was
// never built — so callers can surface a 503 not-built state.
func newSPA(embedded fs.FS) (*spa, error) {
	dist, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil, fmt.Errorf("embed dist: %w", err)
	}
	// A built bundle always ships index.html; its absence means only the
	// committed .gitkeep is present.
	if _, statErr := fs.Stat(dist, "index.html"); statErr != nil {
		return &spa{dist: dist}, errBundleMissing
	}
	return &spa{dist: dist}, nil
}

// built reports whether the SPA bundle was compiled into the binary.
func (s *spa) built() bool {
	if s == nil || s.dist == nil {
		return false
	}
	_, err := fs.Stat(s.dist, "index.html")
	return err == nil
}

// setSecurityHeaders applies baseline security headers to SPA/asset responses.
// It intentionally avoids a restrictive resource CSP (the terminal renderer
// relies on inline styles); frame-ancestors/X-Frame-Options stop framing and
// nosniff stops MIME sniffing.
func setSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "same-origin")
	h.Set("Content-Security-Policy", "frame-ancestors 'none'")
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w)
		next.ServeHTTP(w, r)
	})
}

// registerAssets wires the /assets/ static file route into the mux.
func (s *spa) registerAssets(mux *http.ServeMux) {
	if s == nil || s.dist == nil {
		return
	}
	if assetsFS, err := fs.Sub(s.dist, "assets"); err == nil {
		mux.Handle("GET /assets/", securityHeaders(http.StripPrefix("/assets/", http.FileServer(http.FS(assetsFS)))))
	}
}

// servePath serves a single file from the dist tree, returning false (without
// writing a response) when the path is empty, traverses out of the tree, names
// a directory, or does not exist.
func (s *spa) servePath(w http.ResponseWriter, r *http.Request, filePath string) bool {
	if s == nil || s.dist == nil {
		return false
	}

	clean := strings.TrimPrefix(path.Clean("/"+filePath), "/")
	if clean == "." || clean == "" {
		return false
	}

	info, err := fs.Stat(s.dist, clean)
	if err != nil || info.IsDir() {
		return false
	}

	setSecurityHeaders(w)
	http.ServeFileFS(w, r, s.dist, clean)
	return true
}

// serveManifest writes the web app manifest, branding name/short_name with the
// host name. It is a method seam so the Handler can delegate after its origin
// check.
func (s *spa) serveManifest(w http.ResponseWriter, r *http.Request) {
	if !s.built() {
		http.Error(w, "frontend bundle not built", http.StatusServiceUnavailable)
		return
	}

	rawManifest, err := readManifestFile(s.dist)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	var manifest map[string]any
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		http.Error(w, "invalid manifest", http.StatusInternalServerError)
		return
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
	}

	manifest["name"] = formatManifestAppName(hostname)
	manifest["short_name"] = formatManifestAppShortName(hostname)

	encodedManifest, err := json.Marshal(manifest)
	if err != nil {
		http.Error(w, "invalid manifest", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	_, _ = w.Write(encodedManifest)
}

func formatManifestAppName(hostname string) string {
	trimmedHostname := strings.TrimSpace(hostname)
	if trimmedHostname == "" {
		return manifestAppName
	}
	return trimmedHostname + " - " + manifestAppName
}

func formatManifestAppShortName(hostname string) string {
	trimmedHostname := strings.TrimSpace(hostname)
	if trimmedHostname == "" {
		return manifestAppName
	}
	return trimmedHostname
}

func readManifestFile(dist fs.FS) ([]byte, error) {
	if dist == nil {
		return nil, fs.ErrNotExist
	}
	return fs.ReadFile(dist, "manifest.webmanifest")
}

func isReservedPath(urlPath string) bool {
	return urlPath == "api" ||
		urlPath == "ws" ||
		urlPath == "assets" ||
		strings.HasPrefix(urlPath, "api/") ||
		strings.HasPrefix(urlPath, "ws/") ||
		strings.HasPrefix(urlPath, "assets/")
}
