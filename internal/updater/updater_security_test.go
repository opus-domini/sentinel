package updater

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRequireSecureURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{"https allowed", "https://api.github.com/repos/x/y", false},
		{"http loopback ip allowed", "http://127.0.0.1:8080/x", false},
		{"http localhost allowed", "http://localhost:9999/x", false},
		{"http ipv6 loopback allowed", "http://[::1]:8080/x", false},
		{"http public rejected", "http://example.com/x", true},
		{"ftp rejected", "ftp://example.com/x", true},
		{"garbage rejected", "://nope", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := requireSecureURL(tc.rawURL)
			if tc.wantErr && err == nil {
				t.Fatalf("requireSecureURL(%q) = nil, want error", tc.rawURL)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("requireSecureURL(%q) error = %v, want nil", tc.rawURL, err)
			}
		})
	}
}

func TestSecureRedirect(t *testing.T) {
	t.Parallel()

	t.Run("https target allowed", func(t *testing.T) {
		t.Parallel()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/next", nil)
		if err := secureRedirect(req, nil); err != nil {
			t.Fatalf("secureRedirect(https) error = %v, want nil", err)
		}
	})

	t.Run("insecure target rejected", func(t *testing.T) {
		t.Parallel()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/next", nil)
		if err := secureRedirect(req, nil); err == nil {
			t.Fatal("secureRedirect(http public) error = nil, want rejection")
		}
	})

	t.Run("too many redirects rejected", func(t *testing.T) {
		t.Parallel()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/next", nil)
		via := make([]*http.Request, 10)
		if err := secureRedirect(req, via); err == nil {
			t.Fatal("secureRedirect with 10 prior hops error = nil, want stop")
		}
	})
}

func TestWithSecureRedirect(t *testing.T) {
	t.Parallel()

	t.Run("nil client gets default with guard", func(t *testing.T) {
		t.Parallel()
		client := withSecureRedirect(nil, 7*time.Second)
		if client == nil {
			t.Fatal("withSecureRedirect(nil) = nil")
		}
		if client.Timeout != 7*time.Second {
			t.Fatalf("Timeout = %v, want 7s", client.Timeout)
		}
		if client.CheckRedirect == nil {
			t.Fatal("CheckRedirect not set on default client")
		}
	})

	t.Run("existing CheckRedirect is preserved", func(t *testing.T) {
		t.Parallel()
		sentinelErr := errors.New("custom")
		orig := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return sentinelErr }}
		got := withSecureRedirect(orig, time.Second)
		if got != orig {
			t.Fatal("withSecureRedirect replaced a client that already had CheckRedirect")
		}
	})

	t.Run("client without guard is cloned and guarded", func(t *testing.T) {
		t.Parallel()
		orig := &http.Client{Timeout: 3 * time.Second}
		got := withSecureRedirect(orig, time.Second)
		if got == orig {
			t.Fatal("withSecureRedirect mutated the original client instead of cloning")
		}
		if orig.CheckRedirect != nil {
			t.Fatal("withSecureRedirect mutated the original client's CheckRedirect")
		}
		if got.CheckRedirect == nil {
			t.Fatal("cloned client missing CheckRedirect")
		}
	})
}

// TestFetchJSONRejectsInsecureURL confirms the scheme gate fires before any
// request is dispatched, so a non-loopback http base URL is refused outright.
func TestFetchJSONRejectsInsecureURL(t *testing.T) {
	t.Parallel()

	_, err := Check(context.Background(), CheckOptions{
		CurrentVersion: "1.0.0",
		APIBaseURL:     "http://example.com",
		OS:             "linux",
		Arch:           "amd64",
	})
	if err == nil {
		t.Fatal("Check() with insecure base URL error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "https is required") {
		t.Fatalf("Check() error = %v, want https-required rejection", err)
	}
}

// TestDownloadToFileRejectsOversizedArchive confirms a body that exceeds the
// archive cap is reported explicitly rather than silently truncated.
func TestDownloadToFileRejectsOversizedArchive(t *testing.T) {
	t.Parallel()

	oversize := maxArchiveSize + 1024
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", oversize))
		_, _ = w.Write(make([]byte, oversize))
	}))
	defer ts.Close()

	dest := filepath.Join(t.TempDir(), "archive.tar.gz")
	err := downloadToFile(context.Background(), ts.Client(), ts.URL, dest)
	if err == nil {
		t.Fatal("downloadToFile() error = nil, want oversized rejection")
	}
	if !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Fatalf("downloadToFile() error = %v, want oversized rejection", err)
	}
}

// TestInstallBinaryRestoresOriginalOnChmodFailure confirms that a chmod failure
// leaves the previous binary in place instead of stranding a non-executable
// new binary at the exec path.
func TestInstallBinaryRestoresOriginalOnChmodFailure(t *testing.T) {
	origChmod := chmodFn
	t.Cleanup(func() { chmodFn = origChmod })
	chmodFn = func(_ string, _ os.FileMode) error {
		return errors.New("boom")
	}

	dir := t.TempDir()
	execPath := filepath.Join(dir, "sentinel")
	newPath := filepath.Join(dir, "sentinel.new")
	backupPath := filepath.Join(dir, "sentinel.bak")
	writeFile(t, execPath, "old")
	writeFile(t, newPath, "new")

	err := installBinary(execPath, newPath, backupPath)
	if err == nil {
		t.Fatal("installBinary() error = nil, want chmod failure")
	}
	if !strings.Contains(err.Error(), "chmod installed binary") {
		t.Fatalf("installBinary() error = %v, want chmod failure", err)
	}
	if got := readFile(t, execPath); got != "old" {
		t.Fatalf("after chmod failure exec path = %q, want original old binary restored", got)
	}
}
