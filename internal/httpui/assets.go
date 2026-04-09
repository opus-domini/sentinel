package httpui

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"

	clientassets "github.com/opus-domini/sentinel/client"
)

var (
	distFS     fs.FS
	distFSInit sync.Once
	distFSErr  error
)

const manifestAppName = "Sentinel"

func ensureDistFS() error {
	distFSInit.Do(func() {
		distFS, distFSErr = fs.Sub(clientassets.DistFS, "dist")
	})
	return distFSErr
}

func registerAssetRoutes(mux *http.ServeMux) error {
	if err := ensureDistFS(); err != nil {
		return fmt.Errorf("embed dist: %w", err)
	}

	var err error
	assetsFS, err := fs.Sub(distFS, "assets")
	if err == nil {
		mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assetsFS))))
	}
	return nil
}

func serveDistPath(w http.ResponseWriter, r *http.Request, filePath string) bool {
	if ensureDistFS() != nil {
		return false
	}

	clean := strings.TrimPrefix(path.Clean("/"+filePath), "/")
	if clean == "." || clean == "" {
		return false
	}

	info, err := fs.Stat(distFS, clean)
	if err != nil || info.IsDir() {
		return false
	}

	http.ServeFileFS(w, r, distFS, clean)
	return true
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

func (h *Handler) serveManifest(w http.ResponseWriter, r *http.Request) {
	if err := h.guard.CheckOrigin(r); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if ensureDistFS() != nil {
		http.Error(w, "frontend bundle missing", http.StatusInternalServerError)
		return
	}

	rawManifest, err := fs.ReadFile(distFS, "manifest.webmanifest")
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

func isReservedPath(urlPath string) bool {
	return urlPath == "api" ||
		urlPath == "ws" ||
		urlPath == "assets" ||
		strings.HasPrefix(urlPath, "api/") ||
		strings.HasPrefix(urlPath, "ws/") ||
		strings.HasPrefix(urlPath, "assets/")
}
