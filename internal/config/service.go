package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

var (
	// ErrRevisionConflict reports that the config changed after it was read.
	ErrRevisionConflict = errors.New("config revision conflict")
	// ErrEnvironmentOwned reports a file mutation shadowed by an environment override.
	ErrEnvironmentOwned = errors.New("config field is owned by environment")
)

// LiveApplier applies and compensates config fields supported by the process.
type LiveApplier interface {
	ApplyConfig(context.Context, Config, Config, []string) error
}

// Service serializes canonical config reads and writes for one deployment.
type Service struct {
	path        string
	defaults    Config
	liveApplier LiveApplier

	mu          sync.Mutex
	baseline    Config
	baselineSet bool
}

// NewService creates a canonical config service for one resolved deployment.
func NewService(path string, defaults Config, liveApplier LiveApplier) (*Service, error) {
	resolved, err := resolvePathOrDefault(path)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(defaults.Storage.Path) == "" {
		defaults = Default()
	}
	if err := defaults.normalizeWithDefaults(defaults); err != nil {
		return nil, fmt.Errorf("resolve config defaults: %w", err)
	}
	if err := validateConfigValues(defaults); err != nil {
		return nil, fmt.Errorf("validate config defaults: %w", err)
	}
	return &Service{
		path:        resolved,
		defaults:    defaults,
		liveApplier: liveApplier,
	}, nil
}

// Path returns the canonical config path owned by the service.
func (s *Service) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Read returns one coherent persisted/effective snapshot.
func (s *Service) Read() (State, error) {
	if s == nil {
		return State{}, errors.New("config service is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readLocked()
}

// ValidatePersisted validates the file without applying environment overrides.
func (s *Service) ValidatePersisted() error {
	if s == nil {
		return errors.New("config service is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, exists, err := readConfigBytes(s.path)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("config file not found: %s", s.path)
	}
	_, _, err = decodeConfigBytes(s.path, raw, s.defaults, true)
	return err
}

// Update validates, persists and applies a typed mutation.
func (s *Service) Update(
	ctx context.Context,
	expectedRevision string,
	keys []string,
	mutate func(*Config) error,
) (State, error) {
	if s == nil {
		return State{}, errors.New("config service is nil")
	}
	if mutate == nil {
		return State{}, errors.New("config mutation is required")
	}
	keys = slices.Clone(keys)
	slices.Sort(keys)
	keys = slices.Compact(keys)
	for _, key := range keys {
		if !slices.Contains(configFieldKeys, key) {
			return State{}, fmt.Errorf("unknown config field %q", key)
		}
		if !fieldIsWritable(key) {
			return State{}, fmt.Errorf("config field %q is read-only", key)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	lock, err := lockConfig(s.path)
	if err != nil {
		return State{}, err
	}
	defer func() { _ = lock.close() }()

	current, err := s.readLocked()
	if err != nil {
		return State{}, err
	}
	if expectedRevision != "" && expectedRevision != current.Revision {
		return State{}, fmt.Errorf("%w: expected %s, current %s", ErrRevisionConflict, expectedRevision, current.Revision)
	}
	for _, key := range keys {
		if current.envFields[key] {
			return State{}, fmt.Errorf("%w: %s", ErrEnvironmentOwned, key)
		}
	}

	candidate := current.Persisted
	if err := mutate(&candidate); err != nil {
		return State{}, err
	}
	if err := candidate.normalizeWithDefaults(s.defaults); err != nil {
		return State{}, err
	}
	if err := validateConfigValues(candidate); err != nil {
		return State{}, err
	}
	effective := candidate
	_, envIssues := applyEnvironment(&effective)
	if err := effective.normalizeWithDefaults(s.defaults); err != nil {
		return State{}, err
	}
	if len(envIssues) > 0 {
		return State{}, errors.New(strings.Join(envIssues, "; "))
	}
	if err := validateEffectiveConfig(effective); err != nil {
		return State{}, err
	}

	actualKeys := diffConfigKeys(current.Persisted, candidate)
	for _, key := range actualKeys {
		if !slices.Contains(keys, key) {
			return State{}, fmt.Errorf("config mutation changed unexpected field %q", key)
		}
	}
	if len(actualKeys) == 0 {
		return current, nil
	}

	replacement, err := replaceConfigFile(s.path, RenderTOML(candidate))
	if err != nil {
		return State{}, err
	}
	if s.liveApplier != nil {
		if err := s.liveApplier.ApplyConfig(ctx, current.Effective, effective, actualKeys); err != nil {
			rollbackErr := replacement.rollback()
			compensateErr := s.liveApplier.ApplyConfig(ctx, effective, current.Effective, actualKeys)
			return State{}, errors.Join(err, rollbackErr, compensateErr)
		}
	}

	next, err := s.readLocked()
	if err != nil {
		rollbackErr := replacement.rollback()
		if s.liveApplier != nil {
			return State{}, errors.Join(err, rollbackErr, s.liveApplier.ApplyConfig(ctx, effective, current.Effective, actualKeys))
		}
		return State{}, errors.Join(err, rollbackErr)
	}
	return next, nil
}

func (s *Service) readLocked() (State, error) {
	raw, exists, err := readConfigBytes(s.path)
	if err != nil {
		return State{}, err
	}
	persisted, fileFields, err := decodeConfigBytes(s.path, raw, s.defaults, false)
	if err != nil {
		return State{}, err
	}
	effective := persisted
	envFields, envIssues := applyEnvironment(&effective)
	if err := effective.normalizeWithDefaults(s.defaults); err != nil {
		return State{}, err
	}
	if len(envIssues) > 0 {
		return State{}, configValidationError{Path: s.path, Issues: envIssues}
	}
	if err := validateEffectiveConfig(effective); err != nil {
		return State{}, configValidationError{Path: s.path, Issues: []string{err.Error()}}
	}
	if !s.baselineSet {
		s.baseline = effective
		s.baselineSet = true
	}
	state := State{
		Path:       s.path,
		Exists:     exists,
		Revision:   configRevision(exists, raw),
		Default:    s.defaults,
		Persisted:  persisted,
		Effective:  effective,
		fileFields: fileFields,
		envFields:  envFields,
	}
	state.Fields = buildFieldStates(state, s.baseline)
	return state, nil
}

func decodeConfigBytes(path string, raw []byte, defaults Config, standalone bool) (Config, map[string]bool, error) {
	cfg := defaults
	var meta toml.MetaData
	var err error
	if len(raw) > 0 {
		meta, err = toml.Decode(string(raw), &cfg)
		if err != nil {
			return cfg, nil, fmt.Errorf("decode config: %w", err)
		}
	}
	var issues []string
	for _, key := range meta.Undecoded() {
		issues = append(issues, "unknown key: "+strings.Join(key, "."))
	}
	if err := cfg.normalizeWithDefaults(defaults); err != nil {
		issues = append(issues, err.Error())
	}
	if standalone {
		if err := validateEffectiveConfig(cfg); err != nil {
			issues = append(issues, err.Error())
		}
	} else if err := validateConfigValues(cfg); err != nil {
		issues = append(issues, err.Error())
	}
	if len(issues) > 0 {
		return cfg, nil, configValidationError{Path: path, Issues: issues}
	}
	return cfg, fileFieldSet(meta), nil
}

func readConfigBytes(path string) ([]byte, bool, error) {
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if info.IsDir() {
			return nil, false, fmt.Errorf("config path is a directory: %s", path)
		}
		raw, readErr := os.ReadFile(path) //nolint:gosec // configured local config path.
		if readErr != nil {
			return nil, false, fmt.Errorf("read config file: %w", readErr)
		}
		return raw, true, nil
	case errors.Is(err, os.ErrNotExist):
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("stat config file: %w", err)
	}
}

// BackupPath returns the adjacent recoverable config backup path.
func (s *Service) BackupPath() string {
	if s == nil || s.path == "" {
		return ""
	}
	return filepath.Clean(s.path + ".bak")
}
