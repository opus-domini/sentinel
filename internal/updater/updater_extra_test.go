package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRollbackBinary(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	execPath := filepath.Join(tmp, "sentinel")
	backupPath := execPath + ".bak"

	// Create the backup file.
	if err := os.WriteFile(backupPath, []byte("original-binary"), 0o600); err != nil {
		t.Fatalf("write backup: %v", err)
	}
	// Create a "corrupted" current binary.
	if err := os.WriteFile(execPath, []byte("bad-binary"), 0o600); err != nil {
		t.Fatalf("write exec: %v", err)
	}

	if err := rollbackBinary(execPath, backupPath); err != nil {
		t.Fatalf("rollbackBinary: %v", err)
	}

	data, err := os.ReadFile(execPath) //nolint:gosec // test file
	if err != nil {
		t.Fatalf("read after rollback: %v", err)
	}
	if string(data) != "original-binary" {
		t.Fatalf("binary = %q, want %q", string(data), "original-binary")
	}
}

func TestRollbackBinaryMissingBackup(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	execPath := filepath.Join(tmp, "sentinel")
	backupPath := execPath + ".bak"

	err := rollbackBinary(execPath, backupPath)
	if err == nil {
		t.Fatal("rollbackBinary with no backup = nil, want error")
	}
	if !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("error = %v, want 'rollback failed'", err)
	}
}

func TestRunCommandSuccess(t *testing.T) {
	original := execCommandContext
	t.Cleanup(func() { execCommandContext = original })

	execCommandContext = func(_ context.Context, _ string, _ ...string) command {
		return &fakeCommand{out: []byte("ok\n"), err: nil}
	}

	err := runCommand(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatalf("runCommand: %v", err)
	}
}

func TestRunCommandFailureWithOutput(t *testing.T) {
	original := execCommandContext
	t.Cleanup(func() { execCommandContext = original })

	execCommandContext = func(_ context.Context, _ string, _ ...string) command {
		return &fakeCommand{out: []byte("service not found"), err: errors.New("exit 1")}
	}

	err := runCommand(context.Background(), "systemctl", "restart", "nonexistent")
	if err == nil {
		t.Fatal("runCommand = nil, want error")
	}
	if !strings.Contains(err.Error(), "service not found") {
		t.Fatalf("error = %v, want to contain 'service not found'", err)
	}
}

func TestRunCommandFailureNoOutput(t *testing.T) {
	original := execCommandContext
	t.Cleanup(func() { execCommandContext = original })

	execCommandContext = func(_ context.Context, _ string, _ ...string) command {
		return &fakeCommand{out: nil, err: errors.New("exit 1")}
	}

	err := runCommand(context.Background(), "false")
	if err == nil {
		t.Fatal("runCommand = nil, want error")
	}
	if strings.Contains(err.Error(), ":") {
		// When output is empty, should just return the error directly.
		t.Fatalf("error should not contain output portion: %v", err)
	}
}

type fakeCommand struct {
	out []byte
	err error
}

func (c *fakeCommand) CombinedOutput() ([]byte, error) {
	return c.out, c.err
}

func TestValidateApplyEligibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     ApplyOptions
		check   CheckResult
		wantErr string
	}{
		{
			name: "ok_newer_release",
			cfg:  ApplyOptions{},
			check: CheckResult{
				CurrentVersion: "1.0.0",
				LatestVersion:  "1.1.0",
				ExpectedSHA256: strings.Repeat("a", 64),
			},
		},
		{
			name: "downgrade_blocked",
			cfg:  ApplyOptions{AllowDowngrade: false},
			check: CheckResult{
				CurrentVersion: "2.0.0",
				LatestVersion:  "1.0.0",
				ExpectedSHA256: strings.Repeat("a", 64),
			},
			wantErr: "newer than latest",
		},
		{
			name: "downgrade_allowed",
			cfg:  ApplyOptions{AllowDowngrade: true},
			check: CheckResult{
				CurrentVersion: "2.0.0",
				LatestVersion:  "1.0.0",
				ExpectedSHA256: strings.Repeat("a", 64),
			},
		},
		{
			name: "unverified_blocked",
			cfg:  ApplyOptions{AllowUnverified: false},
			check: CheckResult{
				CurrentVersion: "1.0.0",
				LatestVersion:  "1.1.0",
				ExpectedSHA256: "",
			},
			wantErr: "checksum is unavailable",
		},
		{
			name: "unverified_allowed",
			cfg:  ApplyOptions{AllowUnverified: true},
			check: CheckResult{
				CurrentVersion: "1.0.0",
				LatestVersion:  "1.1.0",
				ExpectedSHA256: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateApplyEligibility(tt.cfg, tt.check)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseSemver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantValid  bool
		wantMajor  int
		wantMinor  int
		wantPatch  int
		wantPrerel string
	}{
		{"simple", "1.2.3", true, 1, 2, 3, ""},
		{"with_v", "v1.2.3", true, 1, 2, 3, ""},
		{"prerelease", "1.2.3-rc.1", true, 1, 2, 3, "rc.1"},
		{"empty", "", false, 0, 0, 0, ""},
		{"two_parts", "1.2", false, 0, 0, 0, ""},
		{"non_numeric", "a.b.c", false, 0, 0, 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseSemver(tt.input)
			if got.valid != tt.wantValid {
				t.Fatalf("valid = %v, want %v", got.valid, tt.wantValid)
			}
			if !tt.wantValid {
				return
			}
			if got.major != tt.wantMajor || got.minor != tt.wantMinor || got.patch != tt.wantPatch {
				t.Fatalf("version = %d.%d.%d, want %d.%d.%d", got.major, got.minor, got.patch, tt.wantMajor, tt.wantMinor, tt.wantPatch)
			}
			if got.prerelease != tt.wantPrerel {
				t.Fatalf("prerelease = %q, want %q", got.prerelease, tt.wantPrerel)
			}
		})
	}
}

func TestLoadStateNonexistentDir(t *testing.T) {
	t.Parallel()

	st, err := loadState("/nonexistent/path/xxx")
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}
	// Should return empty state.
	if st.CurrentVersion != "" {
		t.Fatalf("CurrentVersion = %q, want empty", st.CurrentVersion)
	}
}

func TestBuildRestartCommandNoRestart(t *testing.T) {
	t.Parallel()

	cmd := (ApplyOptions{Restart: false}).buildRestartCommand()
	if len(cmd) != 0 {
		t.Fatalf("buildRestartCommand = %v, want empty", cmd)
	}
}

func TestBuildRestartCommandNoneScope(t *testing.T) {
	t.Parallel()

	cmd := (ApplyOptions{Restart: true, SystemdScope: "none"}).buildRestartCommand()
	if len(cmd) != 0 {
		t.Fatalf("buildRestartCommand = %v, want empty for scope=none", cmd)
	}
}

func TestNormalizeVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"v1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{"  v1.0.0  ", "1.0.0"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.input), func(t *testing.T) {
			t.Parallel()
			got := normalizeVersion(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeVersion(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCompareVersionsExtended(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		left  string
		right string
		want  int
	}{
		{"left_major_lower", "1.0.0", "2.0.0", -1},
		{"left_major_higher", "3.0.0", "2.0.0", 1},
		{"left_minor_lower", "1.2.0", "1.3.0", -1},
		{"left_minor_higher", "1.5.0", "1.3.0", 1},
		{"left_patch_lower", "1.2.3", "1.2.4", -1},
		{"left_patch_higher", "1.2.5", "1.2.4", 1},
		{"prerelease_compare", "1.2.3-alpha", "1.2.3-beta", -1},
		{"prerelease_vs_stable", "1.2.3-rc.1", "1.2.3", -1},
		{"equal_non_semver", "abc", "abc", 0},
		{"left_empty", "", "1.0.0", -1},
		{"right_empty", "1.0.0", "", 1},
		{"non_semver_lexicographic", "aaa", "bbb", -1},
		{"non_semver_lexicographic_higher", "bbb", "aaa", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := compareVersions(tt.left, tt.right)
			if got != tt.want {
				t.Fatalf("compareVersions(%q, %q) = %d, want %d", tt.left, tt.right, got, tt.want)
			}
		})
	}
}

func TestComparePrerelease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		left  string
		right string
		want  int
	}{
		{"both_empty", "", "", 0},
		{"left_empty_stable_wins", "", "rc.1", 1},
		{"right_empty_stable_wins", "rc.1", "", -1},
		{"equal", "alpha.1", "alpha.1", 0},
		{"both_numeric", "1.2", "1.3", -1},
		{"both_numeric_higher", "2.1", "1.5", 1},
		{"numeric_vs_string", "1", "alpha", -1},
		{"string_vs_numeric", "alpha", "1", 1},
		{"string_compare_lower", "alpha", "beta", -1},
		{"string_compare_higher", "beta", "alpha", 1},
		{"left_shorter", "rc", "rc.1", -1},
		{"right_shorter", "rc.1", "rc", 1},
		{"whitespace_trimmed", "  rc.1  ", "rc.1", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := comparePrerelease(tt.left, tt.right)
			if got != tt.want {
				t.Fatalf("comparePrerelease(%q, %q) = %d, want %d", tt.left, tt.right, got, tt.want)
			}
		})
	}
}

func TestIsCurrentUpToDateExtended(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{"newer", "2.0.0", "1.0.0", true},
		{"both_empty", "", "", false},
		{"current_empty", "", "1.0.0", false},
		{"latest_empty", "1.0.0", "", false},
		{"non_semver_equal", "abc", "abc", true},
		{"non_semver_different", "abc", "def", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isCurrentUpToDate(tt.current, tt.latest)
			if got != tt.want {
				t.Fatalf("isCurrentUpToDate(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestBuildRestartCommand(t *testing.T) {
	// Tests that swap package-level vars must NOT use t.Parallel().

	t.Run("system_scope_linux", func(t *testing.T) {
		origGOOS := updaterRuntimeGOOS
		t.Cleanup(func() { updaterRuntimeGOOS = origGOOS })
		updaterRuntimeGOOS = hostOSLinux

		cmd := (ApplyOptions{Restart: true, SystemdScope: "system"}).buildRestartCommand()
		if len(cmd) != 3 || cmd[0] != "systemctl" || cmd[1] != "restart" {
			t.Fatalf("buildRestartCommand = %v, want systemctl restart", cmd)
		}
	})

	t.Run("user_scope_linux", func(t *testing.T) {
		origGOOS := updaterRuntimeGOOS
		t.Cleanup(func() { updaterRuntimeGOOS = origGOOS })
		updaterRuntimeGOOS = hostOSLinux

		cmd := (ApplyOptions{Restart: true, SystemdScope: "user"}).buildRestartCommand()
		if len(cmd) != 4 || cmd[0] != "systemctl" || cmd[1] != "--user" {
			t.Fatalf("buildRestartCommand = %v, want systemctl --user restart", cmd)
		}
	})

	t.Run("system_scope_darwin", func(t *testing.T) {
		origGOOS := updaterRuntimeGOOS
		t.Cleanup(func() { updaterRuntimeGOOS = origGOOS })
		updaterRuntimeGOOS = hostOSDarwin

		cmd := (ApplyOptions{Restart: true, SystemdScope: "system"}).buildRestartCommand()
		if len(cmd) == 0 || cmd[0] != "launchctl" {
			t.Fatalf("buildRestartCommand on darwin = %v, want launchctl", cmd)
		}
	})

	t.Run("user_scope_darwin", func(t *testing.T) {
		origGOOS := updaterRuntimeGOOS
		t.Cleanup(func() { updaterRuntimeGOOS = origGOOS })
		updaterRuntimeGOOS = hostOSDarwin

		cmd := (ApplyOptions{Restart: true, SystemdScope: "user"}).buildRestartCommand()
		if len(cmd) == 0 || cmd[0] != "launchctl" {
			t.Fatalf("buildRestartCommand on darwin user = %v, want launchctl", cmd)
		}
	})

	t.Run("launchd_scope_root", func(t *testing.T) {
		origGOOS := updaterRuntimeGOOS
		origEUID := updaterGeteuid
		t.Cleanup(func() { updaterRuntimeGOOS = origGOOS; updaterGeteuid = origEUID })
		updaterRuntimeGOOS = hostOSLinux
		updaterGeteuid = func() int { return 0 }

		cmd := (ApplyOptions{Restart: true, SystemdScope: "launchd"}).buildRestartCommand()
		if len(cmd) == 0 || cmd[0] != "launchctl" {
			t.Fatalf("buildRestartCommand launchd root = %v, want launchctl", cmd)
		}
	})

	t.Run("launchd_scope_non_root", func(t *testing.T) {
		origGOOS := updaterRuntimeGOOS
		origEUID := updaterGeteuid
		t.Cleanup(func() { updaterRuntimeGOOS = origGOOS; updaterGeteuid = origEUID })
		updaterRuntimeGOOS = hostOSLinux
		updaterGeteuid = func() int { return 1000 }

		cmd := (ApplyOptions{Restart: true, SystemdScope: "launchd"}).buildRestartCommand()
		if len(cmd) == 0 || cmd[0] != "launchctl" {
			t.Fatalf("buildRestartCommand launchd non-root = %v, want launchctl", cmd)
		}
	})

	t.Run("default_scope_falls_through", func(t *testing.T) {
		origGOOS := updaterRuntimeGOOS
		t.Cleanup(func() { updaterRuntimeGOOS = origGOOS })
		updaterRuntimeGOOS = hostOSLinux

		cmd := (ApplyOptions{Restart: true, SystemdScope: "unknown"}).buildRestartCommand()
		if len(cmd) != 4 || cmd[0] != "systemctl" || cmd[1] != "--user" {
			t.Fatalf("buildRestartCommand unknown scope = %v, want systemctl --user restart", cmd)
		}
	})
}

func TestDefaultRestartScope(t *testing.T) {
	// Tests that swap package-level vars must NOT use t.Parallel().

	t.Run("linux_root", func(t *testing.T) {
		origGOOS := updaterRuntimeGOOS
		origEUID := updaterGeteuid
		t.Cleanup(func() { updaterRuntimeGOOS = origGOOS; updaterGeteuid = origEUID })
		updaterRuntimeGOOS = hostOSLinux
		updaterGeteuid = func() int { return 0 }

		if got := defaultRestartScope(); got != restartScopeSystem {
			t.Fatalf("defaultRestartScope() = %q, want %q", got, restartScopeSystem)
		}
	})

	t.Run("linux_user", func(t *testing.T) {
		origGOOS := updaterRuntimeGOOS
		origEUID := updaterGeteuid
		t.Cleanup(func() { updaterRuntimeGOOS = origGOOS; updaterGeteuid = origEUID })
		updaterRuntimeGOOS = hostOSLinux
		updaterGeteuid = func() int { return 1000 }

		if got := defaultRestartScope(); got != restartScopeUser {
			t.Fatalf("defaultRestartScope() = %q, want %q", got, restartScopeUser)
		}
	})

	t.Run("darwin_root", func(t *testing.T) {
		origGOOS := updaterRuntimeGOOS
		origEUID := updaterGeteuid
		t.Cleanup(func() { updaterRuntimeGOOS = origGOOS; updaterGeteuid = origEUID })
		updaterRuntimeGOOS = hostOSDarwin
		updaterGeteuid = func() int { return 0 }

		if got := defaultRestartScope(); got != restartScopeSystem {
			t.Fatalf("defaultRestartScope() = %q, want %q", got, restartScopeSystem)
		}
	})

	t.Run("darwin_user", func(t *testing.T) {
		origGOOS := updaterRuntimeGOOS
		origEUID := updaterGeteuid
		t.Cleanup(func() { updaterRuntimeGOOS = origGOOS; updaterGeteuid = origEUID })
		updaterRuntimeGOOS = hostOSDarwin
		updaterGeteuid = func() int { return 1000 }

		if got := defaultRestartScope(); got != restartScopeLaunchd {
			t.Fatalf("defaultRestartScope() = %q, want %q", got, restartScopeLaunchd)
		}
	})

	t.Run("unknown_os", func(t *testing.T) {
		origGOOS := updaterRuntimeGOOS
		t.Cleanup(func() { updaterRuntimeGOOS = origGOOS })
		updaterRuntimeGOOS = "freebsd"

		if got := defaultRestartScope(); got != restartScopeNone {
			t.Fatalf("defaultRestartScope() = %q, want %q", got, restartScopeNone)
		}
	})
}

func TestFindAssetByName(t *testing.T) {
	t.Parallel()

	assets := []asset{
		{Name: "sentinel-linux-amd64.tar.gz"},
		{Name: "sentinel-checksums.txt"},
	}

	t.Run("found", func(t *testing.T) {
		t.Parallel()
		got, ok := findAssetByName(assets, "sentinel-checksums.txt")
		if !ok {
			t.Fatal("expected to find asset")
		}
		if got.Name != "sentinel-checksums.txt" {
			t.Fatalf("got %q", got.Name)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()
		_, ok := findAssetByName(assets, "nonexistent")
		if ok {
			t.Fatal("expected not found")
		}
	})
}

func TestFindChecksumAsset(t *testing.T) {
	t.Parallel()

	t.Run("version_specific", func(t *testing.T) {
		t.Parallel()
		assets := []asset{
			{Name: "sentinel-1.2.3-checksums.txt"},
			{Name: "sentinel-checksums.txt"},
		}
		got, ok := findChecksumAsset(assets, "1.2.3")
		if !ok {
			t.Fatal("expected to find checksum asset")
		}
		if got.Name != "sentinel-1.2.3-checksums.txt" {
			t.Fatalf("got %q", got.Name)
		}
	})

	t.Run("generic_fallback", func(t *testing.T) {
		t.Parallel()
		assets := []asset{
			{Name: "sentinel-checksums.txt"},
		}
		got, ok := findChecksumAsset(assets, "1.2.3")
		if !ok {
			t.Fatal("expected to find checksum asset")
		}
		if got.Name != "sentinel-checksums.txt" {
			t.Fatalf("got %q", got.Name)
		}
	})

	t.Run("checksums_txt_fallback", func(t *testing.T) {
		t.Parallel()
		assets := []asset{
			{Name: "checksums.txt"},
		}
		got, ok := findChecksumAsset(assets, "1.2.3")
		if !ok {
			t.Fatal("expected to find checksum asset")
		}
		if got.Name != "checksums.txt" {
			t.Fatalf("got %q", got.Name)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()
		assets := []asset{
			{Name: "something-else.txt"},
		}
		_, ok := findChecksumAsset(assets, "1.2.3")
		if ok {
			t.Fatal("expected not found")
		}
	})
}

func TestParseSHA256Digest(t *testing.T) {
	t.Parallel()

	valid := strings.Repeat("abcdef01", 8)

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"valid_64_hex", valid, valid},
		{"sha256_prefix", "sha256:" + valid, valid},
		{"SHA256_prefix", "SHA256:" + valid, valid},
		{"with_whitespace", "  " + valid + "  ", valid},
		{"empty", "", ""},
		{"too_short", "abcdef", ""},
		{"non_hex", strings.Repeat("g", 64), ""},
		{"mixed_case", strings.ToUpper(valid), valid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseSHA256Digest(tt.raw)
			if got != tt.want {
				t.Fatalf("parseSHA256Digest(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseChecksumsExtended(t *testing.T) {
	t.Parallel()

	hash := strings.Repeat("a", 64)

	tests := []struct {
		name string
		raw  string
		want map[string]string
	}{
		{
			name: "bsd_style_with_star",
			raw:  hash + " *sentinel.tar.gz\n",
			want: map[string]string{"sentinel.tar.gz": hash},
		},
		{
			name: "comments_and_blanks",
			raw:  "# checksums\n\n" + hash + "  file.tar.gz\n\n",
			want: map[string]string{"file.tar.gz": hash},
		},
		{
			name: "short_hash_skipped",
			raw:  "abc  file.tar.gz\n",
			want: map[string]string{},
		},
		{
			name: "single_field_skipped",
			raw:  "onlyfield\n",
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseChecksums(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("parseChecksums: got %d entries, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseChecksums[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestWithUpdateLock(t *testing.T) {
	t.Parallel()

	t.Run("empty_datadir_skips_lock", func(t *testing.T) {
		t.Parallel()
		called := false
		err := withUpdateLock("", func() error {
			called = true
			return nil
		})
		if err != nil {
			t.Fatalf("withUpdateLock: %v", err)
		}
		if !called {
			t.Fatal("fn was not called")
		}
	})

	t.Run("whitespace_datadir_skips_lock", func(t *testing.T) {
		t.Parallel()
		called := false
		err := withUpdateLock("   ", func() error {
			called = true
			return nil
		})
		if err != nil {
			t.Fatalf("withUpdateLock: %v", err)
		}
		if !called {
			t.Fatal("fn was not called")
		}
	})

	t.Run("creates_lock_and_runs_fn", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		called := false
		err := withUpdateLock(tmp, func() error {
			called = true
			return nil
		})
		if err != nil {
			t.Fatalf("withUpdateLock: %v", err)
		}
		if !called {
			t.Fatal("fn was not called")
		}
		// Lock file should have been created.
		lockPath := filepath.Join(tmp, "updater", "update.lock")
		if _, err := os.Stat(lockPath); err != nil {
			t.Fatalf("lock file not found: %v", err)
		}
	})

	t.Run("fn_error_propagates", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		wantErr := errors.New("fn failed")
		err := withUpdateLock(tmp, func() error {
			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("err = %v, want %v", err, wantErr)
		}
	})
}

func TestWriteStateAndLoadState(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	writeState(tmp, func(st *State) {
		st.CurrentVersion = "1.2.3"
		st.LatestVersion = "1.3.0"
	})

	loaded, err := loadState(tmp)
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}
	if loaded.CurrentVersion != "1.2.3" {
		t.Fatalf("CurrentVersion = %q, want %q", loaded.CurrentVersion, "1.2.3")
	}
	if loaded.LatestVersion != "1.3.0" {
		t.Fatalf("LatestVersion = %q, want %q", loaded.LatestVersion, "1.3.0")
	}
}

func TestBuildLaunchdRestartCommand(t *testing.T) {
	t.Parallel()

	t.Run("default_unit_label", func(t *testing.T) {
		t.Parallel()
		cmd := buildLaunchdRestartCommand("user", defaultServiceUnit)
		if len(cmd) == 0 {
			t.Fatal("expected non-empty command")
		}
		if cmd[0] != "launchctl" || cmd[1] != "kickstart" {
			t.Fatalf("cmd = %v, want launchctl kickstart ...", cmd)
		}
	})

	t.Run("system_scope", func(t *testing.T) {
		t.Parallel()
		cmd := buildLaunchdRestartCommand("system", "my.service")
		if len(cmd) != 4 {
			t.Fatalf("cmd len = %d, want 4", len(cmd))
		}
		if !strings.Contains(cmd[3], "system/my.service") {
			t.Fatalf("cmd[3] = %q, want system/my.service", cmd[3])
		}
	})
}
