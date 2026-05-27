package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMultiUserConfigFromEnvVars(t *testing.T) {
	tests := []struct {
		name       string
		envVars    map[string]string
		wantUsers  []string
		wantRoot   bool
		wantMethod string
	}{
		{
			name:       "defaults when nothing set",
			wantMethod: defaultUserSwitchMethod(),
		},
		{
			name:       "allowed users from env",
			envVars:    map[string]string{"SENTINEL_ALLOWED_USERS": "postgres,deploy"},
			wantUsers:  []string{"postgres", "deploy"},
			wantMethod: defaultUserSwitchMethod(),
		},
		{
			name:       "allow root target",
			envVars:    map[string]string{"SENTINEL_ALLOW_ROOT_TARGET": "true"},
			wantRoot:   true,
			wantMethod: defaultUserSwitchMethod(),
		},
		{
			name:       "systemd-run switch method",
			envVars:    map[string]string{"SENTINEL_USER_SWITCH_METHOD": "systemd-run"},
			wantMethod: "systemd-run",
		},
		{
			name:       "sudo switch method",
			envVars:    map[string]string{"SENTINEL_USER_SWITCH_METHOD": "sudo"},
			wantMethod: "sudo",
		},
		{
			name:       "invalid switch method keeps default",
			envVars:    map[string]string{"SENTINEL_USER_SWITCH_METHOD": "runas"},
			wantMethod: defaultUserSwitchMethod(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SENTINEL_ALLOWED_USERS", "")
			t.Setenv("SENTINEL_ALLOW_ROOT_TARGET", "")
			t.Setenv("SENTINEL_USER_SWITCH_METHOD", "")
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			cfg := Default()
			applyEnv(&cfg)
			if err := cfg.Resolve(); err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}

			assertStrings(t, cfg.MultiUser.AllowedUsers, tt.wantUsers)
			if cfg.MultiUser.AllowRootTarget != tt.wantRoot {
				t.Errorf("AllowRootTarget = %v, want %v", cfg.MultiUser.AllowRootTarget, tt.wantRoot)
			}
			if cfg.MultiUser.UserSwitchMethod != tt.wantMethod {
				t.Errorf("UserSwitchMethod = %q, want %q", cfg.MultiUser.UserSwitchMethod, tt.wantMethod)
			}
		})
	}
}

func TestMultiUserConfigFromTOML(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`[multi_user]
allowed_users = ["postgres", "deploy"]
allow_root_target = false
user_switch_method = "sudo"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := LoadPath(path)
	if err != nil {
		t.Fatalf("LoadPath() error = %v", err)
	}
	assertStrings(t, cfg.MultiUser.AllowedUsers, []string{"postgres", "deploy"})
	if cfg.MultiUser.AllowRootTarget {
		t.Error("AllowRootTarget = true, want false")
	}
	if cfg.MultiUser.UserSwitchMethod != "sudo" {
		t.Errorf("UserSwitchMethod = %q, want sudo", cfg.MultiUser.UserSwitchMethod)
	}
}

func TestEnvVarsOverrideTOMLForMultiUser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`[multi_user]
allowed_users = ["postgres"]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SENTINEL_ALLOWED_USERS", "deploy,www-data")

	cfg, _, err := LoadPath(path)
	if err != nil {
		t.Fatalf("LoadPath() error = %v", err)
	}
	assertStrings(t, cfg.MultiUser.AllowedUsers, []string{"deploy", "www-data"})
}

func TestValidateMultiUserRemovesRootWhenNotAllowed(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		SystemUsers: []string{"deploy", "postgres", "root"},
		MultiUser: MultiUserConfig{
			AllowedUsers:    []string{"postgres", "root", "deploy"},
			AllowRootTarget: false,
		},
	}

	ValidateMultiUser(cfg)

	const rootUser = "root"
	for _, u := range cfg.MultiUser.AllowedUsers {
		if u == rootUser {
			t.Fatal("root should have been removed from AllowedUsers")
		}
	}
	if len(cfg.MultiUser.AllowedUsers) != 2 {
		t.Fatalf("AllowedUsers = %v, want [postgres deploy]", cfg.MultiUser.AllowedUsers)
	}
}

func TestValidateMultiUserKeepsRootWhenAllowed(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		SystemUsers: []string{"root"},
		MultiUser: MultiUserConfig{
			AllowedUsers:    []string{"root"},
			AllowRootTarget: true,
		},
	}

	ValidateMultiUser(cfg)

	if len(cfg.MultiUser.AllowedUsers) != 1 || cfg.MultiUser.AllowedUsers[0] != "root" {
		t.Fatalf("AllowedUsers = %v, want [root]", cfg.MultiUser.AllowedUsers)
	}
}

func TestValidateMultiUserWarnsForMissingUsers(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		SystemUsers: []string{"hugo"},
		MultiUser: MultiUserConfig{
			AllowedUsers: []string{"nonexistent"},
		},
	}
	ValidateMultiUser(cfg)
}

func TestValidateMultiUserNilConfig(t *testing.T) {
	t.Parallel()

	ValidateMultiUser(nil)
}

func TestValidateMultiUserEmptyAllowedUsers(t *testing.T) {
	t.Parallel()

	cfg := &Config{MultiUser: MultiUserConfig{AllowedUsers: nil}}
	ValidateMultiUser(cfg)
}

func TestValidateMultiUserCrossReferencesSystemUsers(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		SystemUsers: []string{"deploy", "hugo"},
		MultiUser: MultiUserConfig{
			AllowedUsers: []string{"deploy", "ghost"},
		},
	}

	ValidateMultiUser(cfg)

	if len(cfg.MultiUser.AllowedUsers) != 2 {
		t.Fatalf("AllowedUsers = %v, want [deploy ghost]", cfg.MultiUser.AllowedUsers)
	}
}

func TestValidateMultiUserNoSystemUsersSkipsCrossReference(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		SystemUsers: nil,
		MultiUser: MultiUserConfig{
			AllowedUsers: []string{"deploy"},
		},
	}
	ValidateMultiUser(cfg)
}

func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("strings = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("strings[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
