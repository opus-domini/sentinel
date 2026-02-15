package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	latestReleasePath = "/repos/opus-domini/sentinel/releases/latest"
	checksumAssetPath = "/assets/checksums"
)

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		left  string
		right string
		want  int
	}{
		{left: "1.0.0", right: "1.0.0", want: 0},
		{left: "1.0.0", right: "1.0.1", want: -1},
		{left: "1.2.0", right: "1.1.9", want: 1},
		{left: "v1.2.3", right: "1.2.3", want: 0},
		{left: "1.2.3", right: "1.2.3-rc.1", want: 1},
		{left: "1.2.3-alpha.1", right: "1.2.3-alpha.2", want: -1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("%s_vs_%s", tc.left, tc.right), func(t *testing.T) {
			t.Parallel()
			got := compareVersions(tc.left, tc.right)
			switch {
			case tc.want < 0 && got >= 0:
				t.Fatalf("compareVersions(%q, %q) = %d, want < 0", tc.left, tc.right, got)
			case tc.want > 0 && got <= 0:
				t.Fatalf("compareVersions(%q, %q) = %d, want > 0", tc.left, tc.right, got)
			case tc.want == 0 && got != 0:
				t.Fatalf("compareVersions(%q, %q) = %d, want 0", tc.left, tc.right, got)
			}
		})
	}
}

func TestIsCurrentUpToDate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{name: "stable equal", current: "1.2.3", latest: "1.2.3", want: true},
		{name: "stable older", current: "1.2.2", latest: "1.2.3", want: false},
		{name: "stable newer", current: "1.2.4", latest: "1.2.3", want: true},
		{name: "dev current", current: "dev", latest: "1.2.3", want: false},
		{name: "empty current", current: "", latest: "1.2.3", want: false},
		{name: "non-semver equal", current: "custom-build", latest: "custom-build", want: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isCurrentUpToDate(tc.current, tc.latest)
			if got != tc.want {
				t.Fatalf("isCurrentUpToDate(%q, %q) = %t, want %t", tc.current, tc.latest, got, tc.want)
			}
		})
	}
}

func TestParseChecksums(t *testing.T) {
	t.Parallel()

	raw := `
# checksums
1111111111111111111111111111111111111111111111111111111111111111  sentinel-1.0.0-linux-amd64.tar.gz
2222222222222222222222222222222222222222222222222222222222222222 *sentinel-1.0.0-darwin-arm64.tar.gz
invalid line
`
	got := parseChecksums(raw)
	if len(got) != 2 {
		t.Fatalf("len(parseChecksums) = %d, want 2", len(got))
	}
	if got["sentinel-1.0.0-linux-amd64.tar.gz"] == "" {
		t.Fatal("missing linux checksum")
	}
	if got["sentinel-1.0.0-darwin-arm64.tar.gz"] == "" {
		t.Fatal("missing darwin checksum")
	}
}

func TestCheckUsesDigestWhenAvailable(t *testing.T) {
	t.Parallel()

	archiveName := "sentinel-1.2.3-linux-amd64.tar.gz"
	digest := strings.Repeat("a", 64)
	var serverURL string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case latestReleasePath:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v1.2.3",
				"html_url": "https://github.com/opus-domini/sentinel/releases/tag/v1.2.3",
				"assets": []map[string]any{{
					"name":                 archiveName,
					"browser_download_url": serverURL + "/assets/archive",
					"digest":               "sha256:" + digest,
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	serverURL = ts.URL

	res, err := Check(context.Background(), CheckOptions{
		CurrentVersion: "1.2.0",
		APIBaseURL:     ts.URL,
		OS:             "linux",
		Arch:           "amd64",
	})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.UpToDate {
		t.Fatal("Check() returned UpToDate=true, want false")
	}
	if res.ExpectedSHA256 != digest {
		t.Fatalf("ExpectedSHA256 = %q, want %q", res.ExpectedSHA256, digest)
	}
}

func TestCheckUsesChecksumAssetWhenDigestMissing(t *testing.T) {
	t.Parallel()

	archiveName := "sentinel-1.3.0-linux-amd64.tar.gz"
	sum := strings.Repeat("b", 64)
	var serverURL string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case latestReleasePath:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v1.3.0",
				"assets": []map[string]any{
					{
						"name":                 archiveName,
						"browser_download_url": serverURL + "/assets/archive",
					},
					{
						"name":                 "sentinel-1.3.0-checksums.txt",
						"browser_download_url": serverURL + checksumAssetPath,
					},
				},
			})
		case checksumAssetPath:
			_, _ = fmt.Fprintf(w, "%s  %s\n", sum, archiveName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	serverURL = ts.URL

	res, err := Check(context.Background(), CheckOptions{
		CurrentVersion: "1.2.0",
		APIBaseURL:     ts.URL,
		OS:             "linux",
		Arch:           "amd64",
	})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.ExpectedSHA256 != sum {
		t.Fatalf("ExpectedSHA256 = %q, want %q", res.ExpectedSHA256, sum)
	}
}

func TestApplyReplacesBinaryAndWritesState(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	execPath := filepath.Join(tmp, "sentinel")
	if err := os.WriteFile(execPath, []byte("old-binary"), 0o600); err != nil {
		t.Fatalf("write current binary: %v", err)
	}

	archiveName := "sentinel-1.4.0-linux-amd64.tar.gz"
	archivePath := filepath.Join(tmp, archiveName)
	if err := writeArchive(archivePath, []byte("new-binary")); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	sum, err := fileSHA256(archivePath)
	if err != nil {
		t.Fatalf("fileSHA256: %v", err)
	}
	var serverURL string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case latestReleasePath:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v1.4.0",
				"assets": []map[string]any{
					{
						"name":                 archiveName,
						"browser_download_url": serverURL + "/assets/archive",
					},
					{
						"name":                 "sentinel-1.4.0-checksums.txt",
						"browser_download_url": serverURL + checksumAssetPath,
					},
				},
			})
		case "/assets/archive":
			data, _ := os.ReadFile(archivePath) //nolint:gosec // test file
			_, _ = w.Write(data)
		case checksumAssetPath:
			_, _ = fmt.Fprintf(w, "%s  %s\n", sum, archiveName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	serverURL = ts.URL

	result, err := Apply(context.Background(), ApplyOptions{
		CurrentVersion: "1.3.0",
		APIBaseURL:     ts.URL,
		OS:             "linux",
		Arch:           "amd64",
		DataDir:        tmp,
		ExecPath:       execPath,
		Restart:        false,
		SystemdScope:   "none",
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !result.Applied {
		t.Fatal("Apply() reported Applied=false")
	}

	gotBinary, err := os.ReadFile(execPath) //nolint:gosec // test file
	if err != nil {
		t.Fatalf("read installed binary: %v", err)
	}
	if string(gotBinary) != "new-binary" {
		t.Fatalf("installed binary = %q, want %q", string(gotBinary), "new-binary")
	}

	backup, err := os.ReadFile(execPath + ".bak") //nolint:gosec // test file
	if err != nil {
		t.Fatalf("read backup binary: %v", err)
	}
	if string(backup) != "old-binary" {
		t.Fatalf("backup binary = %q, want %q", string(backup), "old-binary")
	}

	st, err := Status(tmp)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if st.LastAppliedAt.IsZero() {
		t.Fatal("state.LastAppliedAt is zero")
	}
	if st.LastError != "" {
		t.Fatalf("state.LastError = %q, want empty", st.LastError)
	}
	if st.CurrentVersion != "1.4.0" {
		t.Fatalf("state.CurrentVersion = %q, want 1.4.0", st.CurrentVersion)
	}
}

func TestApplyFailsOnChecksumMismatch(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	execPath := filepath.Join(tmp, "sentinel")
	if err := os.WriteFile(execPath, []byte("stable-binary"), 0o600); err != nil {
		t.Fatalf("write current binary: %v", err)
	}

	archiveName := "sentinel-1.5.0-linux-amd64.tar.gz"
	archivePath := filepath.Join(tmp, archiveName)
	if err := writeArchive(archivePath, []byte("bad-new-binary")); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	var serverURL string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case latestReleasePath:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v1.5.0",
				"assets": []map[string]any{
					{
						"name":                 archiveName,
						"browser_download_url": serverURL + "/assets/archive",
					},
					{
						"name":                 "sentinel-1.5.0-checksums.txt",
						"browser_download_url": serverURL + checksumAssetPath,
					},
				},
			})
		case "/assets/archive":
			data, _ := os.ReadFile(archivePath) //nolint:gosec // test file
			_, _ = w.Write(data)
		case checksumAssetPath:
			// Intentionally wrong checksum.
			_, _ = fmt.Fprintf(w, "%s  %s\n", strings.Repeat("c", 64), archiveName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	serverURL = ts.URL

	_, err := Apply(context.Background(), ApplyOptions{
		CurrentVersion: "1.4.0",
		APIBaseURL:     ts.URL,
		OS:             "linux",
		Arch:           "amd64",
		DataDir:        tmp,
		ExecPath:       execPath,
		Restart:        false,
		SystemdScope:   "none",
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Apply() error = %v, want checksum mismatch", err)
	}

	current, readErr := os.ReadFile(execPath) //nolint:gosec // test file
	if readErr != nil {
		t.Fatalf("read current binary: %v", readErr)
	}
	if string(current) != "stable-binary" {
		t.Fatalf("current binary = %q, want %q", string(current), "stable-binary")
	}
}

func writeArchive(path string, binary []byte) error {
	out, err := os.Create(path) //nolint:gosec // test helper controls path.
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)
	header := &tar.Header{
		Name:    "sentinel",
		Mode:    0o755,
		Size:    int64(len(binary)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(header); err != nil {
		_ = tw.Close()
		_ = gz.Close()
		return err
	}
	if _, err := tw.Write(binary); err != nil {
		_ = tw.Close()
		_ = gz.Close()
		return err
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return err
	}
	return gz.Close()
}
