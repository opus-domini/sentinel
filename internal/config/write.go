package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ErrConfigLocked reports that another process currently owns the config lock.
var ErrConfigLocked = errors.New("config file is locked by another writer")

type configFileLock struct {
	file *os.File
}

func lockConfig(path string) (*configFileLock, error) {
	lockPath := path + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // lock protects the configured path.
	if err != nil {
		return nil, fmt.Errorf("open config lock: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("set config lock mode: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("%w: %s", ErrConfigLocked, lockPath)
	}
	return &configFileLock{file: file}, nil
}

func (l *configFileLock) close() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	return errors.Join(unlockErr, closeErr)
}

type configReplacement struct {
	path     string
	previous []byte
	existed  bool
}

func replaceConfigFile(path string, content []byte) (*configReplacement, error) {
	replacement := &configReplacement{path: path}
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if info.IsDir() {
			return nil, fmt.Errorf("config path is a directory: %s", path)
		}
		replacement.existed = true
		replacement.previous, err = os.ReadFile(path) //nolint:gosec // configured local config path.
		if err != nil {
			return nil, fmt.Errorf("read existing config: %w", err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("inspect config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}
	if replacement.existed {
		if err := writeConfigFileAtomic(path+".bak", replacement.previous); err != nil {
			return nil, fmt.Errorf("write config backup: %w", err)
		}
	}
	if err := writeConfigFileAtomic(path, content); err != nil {
		return nil, err
	}
	return replacement, nil
}

func (r *configReplacement) rollback() error {
	if r == nil {
		return nil
	}
	if r.existed {
		if err := writeConfigFileAtomic(r.path, r.previous); err != nil {
			return fmt.Errorf("restore config: %w", err)
		}
		return nil
	}
	if err := os.Remove(r.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove newly created config: %w", err)
	}
	return syncConfigDirectory(filepath.Dir(r.path))
}

func writeConfigFileAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	file, err := os.CreateTemp(dir, ".sentinel-config-*")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	tmpPath := file.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("set temporary config mode: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync temporary config: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace config %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("set config mode: %w", err)
	}
	if err := syncConfigDirectory(dir); err != nil {
		return fmt.Errorf("sync config directory: %w", err)
	}
	return nil
}

func syncConfigDirectory(path string) error {
	dir, err := os.Open(path) //nolint:gosec // path is the parent of the configured config file.
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}
