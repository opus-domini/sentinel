package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFrequentDirectories(t *testing.T) {
	t.Parallel()

	const apiDir = "/srv/api"

	h, st := newTestHandler(t, nil)
	ctx := context.Background()
	for _, dir := range []string{apiDir, "/srv/web", apiDir} {
		if err := st.RecordSessionDirectory(ctx, dir); err != nil {
			t.Fatalf("RecordSessionDirectory(%s): %v", dir, err)
		}
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/tmux/frequent-directories?limit=1", nil)
	h.frequentDirectories(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("frequentDirectories status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	dirs, _ := data["dirs"].([]any)
	if len(dirs) != 1 || dirs[0] != apiDir {
		t.Fatalf("dirs = %#v, want [/srv/api]", dirs)
	}
}
