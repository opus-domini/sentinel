package testenv

import (
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureCreatesIsolatedEnvironmentAndCommandBlockers(t *testing.T) {
	root := t.TempDir()
	emptyPathRoot := t.TempDir()
	commandLog := filepath.Join(root, "blocked.log")
	t.Setenv("SENTINEL_CONFIG", "/host/config.toml")
	t.Setenv("SENTINEL_SERVER_TOKEN", "host-token")

	if err := configure(root, commandLog, options{}); err != nil {
		t.Fatalf("configure() error = %v", err)
	}

	for _, key := range []string{"HOME", "TMPDIR", "XDG_CONFIG_HOME", "SENTINEL_DATA_DIR", "TMUX_TMPDIR"} {
		if value := os.Getenv(key); !strings.HasPrefix(value, root+string(os.PathSeparator)) {
			t.Fatalf("%s = %q, want path below %s", key, value, root)
		}
	}
	if value := os.Getenv("SENTINEL_CONFIG"); value != "" {
		t.Fatalf("SENTINEL_CONFIG = %q, want empty", value)
	}
	if value := os.Getenv("SENTINEL_SERVER_TOKEN"); value != "" {
		t.Fatalf("SENTINEL_SERVER_TOKEN = %q, want empty", value)
	}
	if value := os.Getenv("TMUX"); value != "" {
		t.Fatalf("TMUX = %q, want empty", value)
	}

	cmd := exec.Command("systemctl", "--user", "restart", "sentinel")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != blockedCommandExitCode {
		t.Fatalf("blocked systemctl error = %v, want exit code %d", err, blockedCommandExitCode)
	}
	attempts, err := os.ReadFile(commandLog)
	if err != nil {
		t.Fatalf("read command log: %v", err)
	}
	if !strings.Contains(string(attempts), "systemctl --user restart sentinel") {
		t.Fatalf("command log = %q", attempts)
	}
	if _, err := exec.LookPath("sh"); err == nil {
		t.Fatal("host sh unexpectedly available in isolated PATH")
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/test", nil)
	if err != nil {
		t.Fatalf("create external request: %v", err)
	}
	response, err := http.DefaultTransport.RoundTrip(req)
	if response != nil {
		_ = response.Body.Close()
	}
	if err == nil ||
		!strings.Contains(err.Error(), "outbound HTTP request blocked during tests") {
		t.Fatalf("external HTTP request error = %v, want host-isolation rejection", err)
	}

	called := false
	transport := loopbackOnlyTransport{base: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})}
	loopbackReq, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:4040/test", nil)
	if err != nil {
		t.Fatalf("create loopback request: %v", err)
	}
	response, err = transport.RoundTrip(loopbackReq)
	if err != nil {
		t.Fatalf("loopback HTTP request error = %v", err)
	}
	_ = response.Body.Close()
	if !called {
		t.Fatal("loopback HTTP request did not reach the underlying transport")
	}

	opts := options{}
	WithEmptyPath()(&opts)
	if err := configure(emptyPathRoot, filepath.Join(emptyPathRoot, "blocked.log"), opts); err != nil {
		t.Fatalf("configure(empty PATH) error = %v", err)
	}

	path := os.Getenv("PATH")
	if path != filepath.Join(emptyPathRoot, "empty-path") {
		t.Fatalf("PATH = %q, want isolated empty path", path)
	}
	if _, err := exec.LookPath("systemctl"); err == nil {
		t.Fatal("systemctl unexpectedly available in empty PATH")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
