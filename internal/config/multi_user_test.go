package config

import (
	"os/user"
	"testing"
)

func TestApplyMultiUserConfigFromEnvVars(t *testing.T) {
	// Cannot be parallel because subtests use t.Setenv.

	tests := []struct {
		name       string
		envVars    map[string]string
		wantUsers  []string
		wantRoot   bool
		wantMethod string
	}{
		{
			name:       "defaults when nothing set",
			envVars:    nil,
			wantUsers:  nil,
			wantRoot:   false,
			wantMethod: "sudo",
		},
		{
			name:       "allowed users from env",
			envVars:    map[string]string{"SENTINEL_ALLOWED_USERS": "postgres,deploy"},
			wantUsers:  []string{"postgres", "deploy"},
			wantMethod: "sudo",
		},
		{
			name:       "allow root target",
			envVars:    map[string]string{"SENTINEL_ALLOW_ROOT_TARGET": "true"},
			wantRoot:   true,
			wantMethod: "sudo",
		},
		{
			name:       "direct switch method",
			envVars:    map[string]string{"SENTINEL_USER_SWITCH_METHOD": "direct"},
			wantMethod: "direct",
		},
		{
			name:       "invalid switch method defaults to sudo",
			envVars:    map[string]string{"SENTINEL_USER_SWITCH_METHOD": "runas"},
			wantMethod: "sudo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Cannot be parallel: uses t.Setenv.
			file := make(map[string]string)
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			cfg := &Config{}
			applyMultiUserConfig(cfg, file)

			if tt.wantUsers != nil {
				if len(cfg.MultiUser.AllowedUsers) != len(tt.wantUsers) {
					t.Fatalf("AllowedUsers = %v, want %v", cfg.MultiUser.AllowedUsers, tt.wantUsers)
				}
				for i, u := range tt.wantUsers {
					if cfg.MultiUser.AllowedUsers[i] != u {
						t.Errorf("AllowedUsers[%d] = %q, want %q", i, cfg.MultiUser.AllowedUsers[i], u)
					}
				}
			}
			if cfg.MultiUser.AllowRootTarget != tt.wantRoot {
				t.Errorf("AllowRootTarget = %v, want %v", cfg.MultiUser.AllowRootTarget, tt.wantRoot)
			}
			if cfg.MultiUser.UserSwitchMethod != tt.wantMethod {
				t.Errorf("UserSwitchMethod = %q, want %q", cfg.MultiUser.UserSwitchMethod, tt.wantMethod)
			}
		})
	}
}

func TestApplyMultiUserConfigFromTOML(t *testing.T) {
	t.Parallel()

	content := `[multi_user]
allowed_users = ["postgres", "deploy"]
allow_root_target = false
user_switch_method = "direct"
`
	file, err := decodeTOML(content)
	if err != nil {
		t.Fatalf("decodeTOML: %v", err)
	}

	cfg := &Config{}
	applyMultiUserConfig(cfg, file)

	if len(cfg.MultiUser.AllowedUsers) != 2 {
		t.Fatalf("AllowedUsers = %v, want [postgres deploy]", cfg.MultiUser.AllowedUsers)
	}
	if cfg.MultiUser.AllowedUsers[0] != "postgres" || cfg.MultiUser.AllowedUsers[1] != "deploy" {
		t.Errorf("AllowedUsers = %v, want [postgres deploy]", cfg.MultiUser.AllowedUsers)
	}
	if cfg.MultiUser.AllowRootTarget {
		t.Error("AllowRootTarget = true, want false")
	}
	if cfg.MultiUser.UserSwitchMethod != "direct" {
		t.Errorf("UserSwitchMethod = %q, want direct", cfg.MultiUser.UserSwitchMethod)
	}
}

func TestValidateMultiUserRemovesRootWhenNotAllowed(t *testing.T) {
	// Not parallel: mutates package-level userLookup.

	cfg := &Config{
		MultiUser: MultiUserConfig{
			AllowedUsers:    []string{"postgres", "root", "deploy"},
			AllowRootTarget: false,
		},
	}

	original := userLookup
	t.Cleanup(func() { userLookup = original })
	userLookup = func(name string) (*user.User, error) {
		return &user.User{Username: name}, nil
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
	// Not parallel: mutates package-level userLookup.

	cfg := &Config{
		MultiUser: MultiUserConfig{
			AllowedUsers:    []string{"root"},
			AllowRootTarget: true,
		},
	}

	original := userLookup
	t.Cleanup(func() { userLookup = original })
	userLookup = func(name string) (*user.User, error) {
		return &user.User{Username: name}, nil
	}

	ValidateMultiUser(cfg)

	if len(cfg.MultiUser.AllowedUsers) != 1 || cfg.MultiUser.AllowedUsers[0] != "root" {
		t.Fatalf("AllowedUsers = %v, want [root]", cfg.MultiUser.AllowedUsers)
	}
}

func TestValidateMultiUserWarnsForMissingUsers(t *testing.T) {
	// Not parallel: mutates package-level userLookup.

	original := userLookup
	t.Cleanup(func() { userLookup = original })
	userLookup = func(name string) (*user.User, error) {
		return nil, user.UnknownUserError(name)
	}

	cfg := &Config{
		MultiUser: MultiUserConfig{
			AllowedUsers: []string{"nonexistent"},
		},
	}
	// Should not panic — just logs a warning.
	ValidateMultiUser(cfg)
}

func TestValidateMultiUserNilConfig(t *testing.T) {
	t.Parallel()

	ValidateMultiUser(nil) // should not panic
}

func TestValidateMultiUserEmptyAllowedUsers(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		MultiUser: MultiUserConfig{
			AllowedUsers: nil,
		},
	}
	// Should not panic or log a warning — empty means "any user allowed".
	ValidateMultiUser(cfg)
}

func TestEnvVarsOverrideTOMLForMultiUser(t *testing.T) {
	// Cannot be parallel: uses t.Setenv.

	content := `[multi_user]
allowed_users = ["postgres"]
`
	file, err := decodeTOML(content)
	if err != nil {
		t.Fatalf("decodeTOML: %v", err)
	}

	t.Setenv("SENTINEL_ALLOWED_USERS", "deploy,www-data")

	cfg := &Config{}
	applyMultiUserConfig(cfg, file)

	if len(cfg.MultiUser.AllowedUsers) != 2 {
		t.Fatalf("AllowedUsers = %v, want [deploy www-data]", cfg.MultiUser.AllowedUsers)
	}
}
