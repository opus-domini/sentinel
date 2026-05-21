package updater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil { //nolint:gosec // test fixture
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test reads its own fixture
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestResolveExecPath(t *testing.T) {
	t.Parallel()

	t.Run("empty resolves current executable", func(t *testing.T) {
		t.Parallel()

		path, err := resolveExecPath("")
		if err != nil {
			t.Fatalf("resolveExecPath(\"\") error = %v", err)
		}
		if path == "" {
			t.Fatal("resolveExecPath(\"\") returned an empty path")
		}
	})

	t.Run("explicit path is returned", func(t *testing.T) {
		t.Parallel()

		bin := filepath.Join(t.TempDir(), "sentinel")
		writeFile(t, bin, "binary")
		path, err := resolveExecPath(bin)
		if err != nil {
			t.Fatalf("resolveExecPath() error = %v", err)
		}
		if !strings.HasSuffix(path, "sentinel") {
			t.Fatalf("resolveExecPath() = %q, want a path ending in sentinel", path)
		}
	})

	t.Run("newline in path is rejected", func(t *testing.T) {
		t.Parallel()

		if _, err := resolveExecPath("/bin/sentinel\nrm -rf /"); err == nil {
			t.Fatal("resolveExecPath() error = nil, want rejection for embedded newline")
		}
	})
}

func TestInstallBinary(t *testing.T) {
	t.Parallel()

	t.Run("replaces binary and backs up the old one", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		execPath := filepath.Join(dir, "sentinel")
		newPath := filepath.Join(dir, "sentinel.new")
		backupPath := filepath.Join(dir, "sentinel.bak")
		writeFile(t, execPath, "old")
		writeFile(t, newPath, "new")
		// A stale backup must be removed before the new backup is written.
		writeFile(t, backupPath, "stale")

		if err := installBinary(execPath, newPath, backupPath); err != nil {
			t.Fatalf("installBinary() error = %v", err)
		}
		if got := readFile(t, execPath); got != "new" {
			t.Fatalf("installed binary = %q, want new", got)
		}
		if got := readFile(t, backupPath); got != "old" {
			t.Fatalf("backup = %q, want old", got)
		}
	})

	t.Run("missing current binary fails", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		err := installBinary(
			filepath.Join(dir, "absent"),
			filepath.Join(dir, "sentinel.new"),
			filepath.Join(dir, "sentinel.bak"),
		)
		if err == nil {
			t.Fatal("installBinary() error = nil, want error for missing current binary")
		}
	})

	t.Run("missing new binary rolls back", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		execPath := filepath.Join(dir, "sentinel")
		writeFile(t, execPath, "old")

		err := installBinary(execPath, filepath.Join(dir, "absent.new"), filepath.Join(dir, "sentinel.bak"))
		if err == nil {
			t.Fatal("installBinary() error = nil, want error for missing new binary")
		}
		if got := readFile(t, execPath); got != "old" {
			t.Fatalf("after rollback the binary = %q, want the original old content", got)
		}
	})
}
