package httpui

import (
	"fmt"
	"io/fs"
	"net/http"
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

func isReservedPath(urlPath string) bool {
	return urlPath == "api" ||
		urlPath == "ws" ||
		urlPath == "assets" ||
		strings.HasPrefix(urlPath, "api/") ||
		strings.HasPrefix(urlPath, "ws/") ||
		strings.HasPrefix(urlPath, "assets/")
}
