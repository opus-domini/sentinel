package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
)

type recordingLiveApplier struct {
	calls    int
	failOnce bool
}

func (a *recordingLiveApplier) ApplyConfig(_ context.Context, _, _ Config, _ []string) error {
	a.calls++
	if a.failOnce {
		a.failOnce = false
		return errors.New("live apply failed")
	}
	return nil
}

func TestServiceRevisionDistinguishesMissingAndEmpty(t *testing.T) {
	clearConfigEnv(t)
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	defaults := DefaultForDeployment(filepath.Join(root, "data"), filepath.Join(root, "sentinel.log"))
	service, err := NewService(path, defaults, nil)
	if err != nil {
		t.Fatal(err)
	}

	missing, err := service.Read()
	if err != nil {
		t.Fatal(err)
	}
	if missing.Exists {
		t.Fatal("missing config reported as existing")
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	empty, err := service.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !empty.Exists {
		t.Fatal("empty config reported as missing")
	}
	if missing.Revision == empty.Revision {
		t.Fatal("missing and empty config revisions are equal")
	}
}

func TestServiceTracksDefaultFileAndEnvironmentSources(t *testing.T) {
	clearConfigEnv(t)
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, []byte("[server]\ntimezone = \"UTC\"\nlocale = \"en-US\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SENTINEL_SERVER_LOCALE", "pt-BR")
	service, err := NewService(
		path,
		DefaultForDeployment(filepath.Join(root, "data"), filepath.Join(root, "sentinel.log")),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.Read()
	if err != nil {
		t.Fatal(err)
	}

	assertFieldSource(t, state, FieldServerTimezone, FieldSourceFile, true, true)
	assertFieldSource(t, state, FieldServerLocale, FieldSourceEnvironment, true, false)
	assertFieldSource(t, state, FieldServerPort, FieldSourceDefault, false, true)
	if state.Persisted.Server.Locale != "en-US" || state.Effective.Server.Locale != "pt-BR" {
		t.Fatalf("locale state = persisted:%q effective:%q", state.Persisted.Server.Locale, state.Effective.Server.Locale)
	}
}

func TestServiceUpdateCreatesCanonicalConfigAndBackup(t *testing.T) {
	clearConfigEnv(t)
	root := t.TempDir()
	path := filepath.Join(root, "nested", "config.toml")
	service, err := NewService(
		path,
		DefaultForDeployment(filepath.Join(root, "data"), filepath.Join(root, "sentinel.log")),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	before, err := service.Read()
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Update(context.Background(), before.Revision, []string{FieldServerLocale}, func(cfg *Config) error {
		cfg.Server.Locale = "pt-BR"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Exists || first.Effective.Server.Locale != "pt-BR" {
		t.Fatalf("first state = %+v", first)
	}
	assertFileMode(t, path, 0o600)
	if _, err := os.Stat(service.BackupPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup exists after first save: %v", err)
	}
	firstBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"version = 1", "[server]", `locale = "pt-BR"`, "[multi_user]"} {
		if !strings.Contains(string(firstBytes), fragment) {
			t.Fatalf("canonical config missing %q", fragment)
		}
	}

	second, err := service.Update(context.Background(), first.Revision, []string{FieldServerTimezone}, func(cfg *Config) error {
		cfg.Server.Timezone = "UTC"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Revision == first.Revision {
		t.Fatal("revision did not change")
	}
	backupBytes, err := os.ReadFile(service.BackupPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(backupBytes) != string(firstBytes) {
		t.Fatal("backup does not contain the previous canonical config")
	}
	assertFileMode(t, service.BackupPath(), 0o600)
	timezoneField, _ := second.Field(FieldServerTimezone)
	if timezoneField.ApplyMode != ApplyModePartial || !timezoneField.RestartPending {
		t.Fatalf("timezone field = %+v", timezoneField)
	}
}

func TestServiceUpdateRejectsEnvironmentOwnedField(t *testing.T) {
	clearConfigEnv(t)
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	t.Setenv("SENTINEL_SERVER_LOCALE", "pt-BR")
	service, err := NewService(
		path,
		DefaultForDeployment(filepath.Join(root, "data"), filepath.Join(root, "sentinel.log")),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.Read()
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Update(context.Background(), state.Revision, []string{FieldServerLocale}, func(cfg *Config) error {
		cfg.Server.Locale = "en-US"
		return nil
	})
	if !errors.Is(err, ErrEnvironmentOwned) {
		t.Fatalf("Update() error = %v, want ErrEnvironmentOwned", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("environment-owned update wrote config: %v", statErr)
	}
}

func TestServiceUpdateRejectsRevisionConflict(t *testing.T) {
	clearConfigEnv(t)
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	service, err := NewService(
		path,
		DefaultForDeployment(filepath.Join(root, "data"), filepath.Join(root, "sentinel.log")),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.Read()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[server]\nlocale = \"en-US\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = service.Update(context.Background(), state.Revision, []string{FieldServerLocale}, func(cfg *Config) error {
		cfg.Server.Locale = "pt-BR"
		return nil
	})
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("Update() error = %v, want ErrRevisionConflict", err)
	}
}

func TestServiceUpdatePreflightReceivesValidatedCandidateBeforeWrite(t *testing.T) {
	clearConfigEnv(t)
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	original := []byte("[server]\nport = 4040\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	applier := &recordingLiveApplier{}
	service, err := NewService(
		path,
		DefaultForDeployment(filepath.Join(root, "data"), filepath.Join(root, "sentinel.log")),
		applier,
	)
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.Read()
	if err != nil {
		t.Fatal(err)
	}

	preflightErr := errors.New("candidate endpoint is occupied")
	_, err = service.UpdateWithPreflight(
		context.Background(),
		state.Revision,
		[]string{FieldServerPort},
		func(cfg *Config) error {
			cfg.Server.Port = 5050
			return nil
		},
		func(
			current State,
			baseline Config,
			persisted Config,
			effective Config,
			actualKeys []string,
		) error {
			if current.Revision != state.Revision ||
				baseline.Server.Port != 4040 ||
				persisted.Server.Port != 5050 ||
				effective.Server.Port != 5050 ||
				!slices.Equal(actualKeys, []string{FieldServerPort}) {
				t.Fatalf(
					"preflight state = revision:%q baseline:%d persisted:%d effective:%d keys:%v",
					current.Revision,
					baseline.Server.Port,
					persisted.Server.Port,
					effective.Server.Port,
					actualKeys,
				)
			}
			return preflightErr
		},
	)
	if !errors.Is(err, preflightErr) {
		t.Fatalf("UpdateWithPreflight() error = %v, want preflight error", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(original) {
		t.Fatalf("preflight rejection changed config:\n%s", after)
	}
	if applier.calls != 0 {
		t.Fatalf("live applier calls = %d, want 0", applier.calls)
	}
	if _, statErr := os.Stat(service.BackupPath()); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("preflight rejection created backup: %v", statErr)
	}
}

func TestServiceUpdateRollsBackFileAndLiveState(t *testing.T) {
	clearConfigEnv(t)
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	original := []byte("[server]\nlocale = \"en-US\"\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	applier := &recordingLiveApplier{failOnce: true}
	service, err := NewService(
		path,
		DefaultForDeployment(filepath.Join(root, "data"), filepath.Join(root, "sentinel.log")),
		applier,
	)
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.Read()
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Update(context.Background(), state.Revision, []string{FieldServerLocale}, func(cfg *Config) error {
		cfg.Server.Locale = "pt-BR"
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "live apply failed") {
		t.Fatalf("Update() error = %v, want live apply failure", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("config was not rolled back byte for byte:\n%s", got)
	}
	if applier.calls != 2 {
		t.Fatalf("live applier calls = %d, want apply + compensation", applier.calls)
	}
}

func TestServiceUpdateRemovesFirstSaveWhenLiveApplyFails(t *testing.T) {
	clearConfigEnv(t)
	root := t.TempDir()
	path := filepath.Join(root, "nested", "config.toml")
	applier := &recordingLiveApplier{failOnce: true}
	service, err := NewService(
		path,
		DefaultForDeployment(filepath.Join(root, "data"), filepath.Join(root, "sentinel.log")),
		applier,
	)
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.Read()
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Update(context.Background(), state.Revision, []string{FieldServerLocale}, func(cfg *Config) error {
		cfg.Server.Locale = "pt-BR"
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "live apply failed") {
		t.Fatalf("Update() error = %v, want live apply failure", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("new config survived rollback: %v", statErr)
	}
	if applier.calls != 2 {
		t.Fatalf("live applier calls = %d, want apply + compensation", applier.calls)
	}
}

func TestServiceUpdateRejectsExternalLock(t *testing.T) {
	clearConfigEnv(t)
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	service, err := NewService(
		path,
		DefaultForDeployment(filepath.Join(root, "data"), filepath.Join(root, "sentinel.log")),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.Read()
	if err != nil {
		t.Fatal(err)
	}
	lockFile, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lockFile.Close() }()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) }()

	_, err = service.Update(context.Background(), state.Revision, []string{FieldServerLocale}, func(cfg *Config) error {
		cfg.Server.Locale = "pt-BR"
		return nil
	})
	if !errors.Is(err, ErrConfigLocked) {
		t.Fatalf("Update() error = %v, want ErrConfigLocked", err)
	}
}

func TestCanonicalValidationRules(t *testing.T) {
	clearConfigEnv(t)
	tests := []struct {
		name   string
		change func(*Config)
		want   string
	}{
		{name: "unsupported locale", change: func(cfg *Config) { cfg.Server.Locale = "xx-YY" }, want: "server.locale"},
		{name: "watchtower tick too low", change: func(cfg *Config) { cfg.Watchtower.TickInterval = 50_000_000 }, want: "watchtower.tick_interval"},
		{name: "capture lines too high", change: func(cfg *Config) { cfg.Watchtower.CaptureLines = 2001 }, want: "watchtower.capture_lines"},
		{name: "capture timeout too high", change: func(cfg *Config) { cfg.Watchtower.CaptureTimeout = 11_000_000_000 }, want: "watchtower.capture_timeout"},
		{name: "journal rows too low", change: func(cfg *Config) { cfg.Watchtower.JournalRows = 99 }, want: "watchtower.journal_rows"},
		{name: "runbooks too high", change: func(cfg *Config) { cfg.Runbooks.MaxConcurrent = 65 }, want: "runbooks.max_concurrent"},
		{name: "webhook userinfo", change: func(cfg *Config) { cfg.HealthReport.WebhookURL = "https://user@example.test/hook" }, want: "must not contain userinfo"},
		{name: "webhook fragment", change: func(cfg *Config) { cfg.HealthReport.WebhookURL = "https://example.test/hook#secret" }, want: "must not contain a fragment"},
		{name: "invalid cookie", change: func(cfg *Config) { cfg.Server.CookieSecure = "sometimes" }, want: "server.cookie_secure"},
		{name: "invalid switch method", change: func(cfg *Config) { cfg.MultiUser.UserSwitchMethod = "systemd" }, want: "multi_user.user_switch_method"},
		{name: "remote without token", change: func(cfg *Config) { cfg.Server.Host = "0.0.0.0" }, want: "token is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultForDeployment(t.TempDir(), filepath.Join(t.TempDir(), "sentinel.log"))
			tt.change(&cfg)
			err := cfg.Resolve()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Resolve() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func assertFieldSource(
	t *testing.T,
	state State,
	key string,
	source FieldSource,
	defined bool,
	editable bool,
) {
	t.Helper()
	field, ok := state.Field(key)
	if !ok {
		t.Fatalf("missing field %q", key)
	}
	if field.Source != source || field.Defined != defined || field.Editable != editable {
		t.Fatalf("field %q = %+v", key, field)
	}
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}
