package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type managedFileReplacement struct {
	path     string
	previous []byte
	mode     os.FileMode
	existed  bool
}

func replaceManagedFile(path string, content []byte, mode os.FileMode) (*managedFileReplacement, error) {
	replacement := &managedFileReplacement{path: path, mode: mode}
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if info.IsDir() {
			return nil, fmt.Errorf("managed file path is a directory: %s", path)
		}
		replacement.existed = true
		replacement.mode = info.Mode().Perm()
		replacement.previous, err = os.ReadFile(path) //nolint:gosec // path is a fixed managed unit path.
		if err != nil {
			return nil, fmt.Errorf("read existing managed file %s: %w", path, err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("inspect managed file %s: %w", path, err)
	}

	if err := writeManagedFileAtomic(path, content, mode); err != nil {
		return nil, err
	}
	return replacement, nil
}

func writeManagedFileAtomic(path string, content []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sentinel-unit-*")
	if err != nil {
		return fmt.Errorf("create temporary managed file for %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set temporary managed file mode: %w", err)
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary managed file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temporary managed file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary managed file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace managed file %s: %w", path, err)
	}
	return nil
}

func (replacement *managedFileReplacement) rollback() error {
	if replacement == nil {
		return nil
	}
	if !replacement.existed {
		if err := os.Remove(replacement.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove new managed file %s: %w", replacement.path, err)
		}
		return nil
	}
	if err := writeManagedFileAtomic(replacement.path, replacement.previous, replacement.mode); err != nil {
		return fmt.Errorf("restore managed file %s: %w", replacement.path, err)
	}
	return nil
}

func rollbackManagedFiles(cause error, replacements ...*managedFileReplacement) error {
	errs := []error{cause}
	for i := len(replacements) - 1; i >= 0; i-- {
		if err := replacements[i].rollback(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
