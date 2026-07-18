package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestManagedFileReplacementRestoresExistingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sentinel.service")
	if err := os.WriteFile(path, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	replacement, err := replaceManagedFile(path, []byte("new"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := rollbackManagedFiles(errors.New("activation failed"), replacement); err == nil || !strings.Contains(err.Error(), "activation failed") {
		t.Fatalf("rollback error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "old" {
		t.Fatalf("content = %q, want old", content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("mode = %o, want 640", got)
	}
}

func TestRollbackSystemdInstallCleansFreshUnit(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sentinel.service")
	replacement, err := replaceManagedFile(path, []byte("new"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	runFn := func(args ...string) error {
		calls = append(calls, slices.Clone(args))
		return nil
	}
	_ = rollbackSystemdInstall(errors.New("start failed"), runFn, "sentinel", false, replacement)
	want := [][]string{{"disable", "--now", "sentinel"}, {"daemon-reload"}}
	if !slices.EqualFunc(calls, want, slices.Equal[[]string]) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fresh unit remains after rollback: %v", err)
	}
}

func TestRollbackSystemdInstallRestartsPreviousActiveUnit(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sentinel.service")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	replacement, err := replaceManagedFile(path, []byte("new"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	runFn := func(args ...string) error {
		calls = append(calls, slices.Clone(args))
		return nil
	}
	_ = rollbackSystemdInstall(errors.New("start failed"), runFn, "sentinel", true, replacement)
	want := [][]string{{"daemon-reload"}, {"restart", "sentinel"}}
	if !slices.EqualFunc(calls, want, slices.Equal[[]string]) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestManagedFileReplacementRemovesFreshFileOnRollback(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sentinel.service")
	replacement, err := replaceManagedFile(path, []byte("new"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_ = rollbackManagedFiles(errors.New("activation failed"), replacement)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fresh managed file remains after rollback: %v", err)
	}
}
