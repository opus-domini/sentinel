package security

import (
	"errors"
	"testing"
)

func TestValidateTargetUserEmptyAllowedUsers(t *testing.T) {
	t.Parallel()

	g := NewWithMultiUser("token", nil, CookieSecureAuto, MultiUserConfig{
		SystemUsers: []string{"hugo", "postgres"},
	})
	if err := g.ValidateTargetUser(""); err != nil {
		t.Errorf("empty user should pass: %v", err)
	}
	// When AllowedUsers is empty but SystemUsers is set, any system user is allowed.
	if err := g.ValidateTargetUser("postgres"); err != nil {
		t.Errorf("system user should pass when allowlist is empty: %v", err)
	}
}

func TestValidateTargetUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		config     MultiUserConfig
		targetUser string
		wantErr    error
	}{
		{
			name: "empty user always passes",
			config: MultiUserConfig{
				AllowedUsers: []string{"postgres"},
				SystemUsers:  []string{"postgres"},
			},
			targetUser: "",
			wantErr:    nil,
		},
		{
			name: "allowed user passes",
			config: MultiUserConfig{
				AllowedUsers: []string{"postgres", "deploy"},
				SystemUsers:  []string{"postgres", "deploy"},
			},
			targetUser: "postgres",
			wantErr:    nil,
		},
		{
			name: "unknown user fails",
			config: MultiUserConfig{
				AllowedUsers: []string{"postgres"},
				SystemUsers:  []string{"postgres"},
			},
			targetUser: "unknown",
			wantErr:    ErrUserNotAllowlist,
		},
		{
			name: "root denied when not allowed",
			config: MultiUserConfig{
				AllowedUsers:    []string{"root"},
				AllowRootTarget: false,
				SystemUsers:     []string{"root"},
			},
			targetUser: "root",
			wantErr:    ErrRootNotAllowed,
		},
		{
			name: "root allowed when configured",
			config: MultiUserConfig{
				AllowedUsers:    []string{"root"},
				AllowRootTarget: true,
				SystemUsers:     []string{"root"},
			},
			targetUser: "root",
			wantErr:    nil,
		},
		{
			name: "system user allowed when allowlist is empty",
			config: MultiUserConfig{
				SystemUsers: []string{"deploy", "hugo"},
			},
			targetUser: "deploy",
			wantErr:    nil,
		},
		{
			name: "non-system user rejected when allowlist is empty",
			config: MultiUserConfig{
				SystemUsers: []string{"deploy", "hugo"},
			},
			targetUser: "ghost",
			wantErr:    ErrUserNotSystemUser,
		},
		{
			name:       "root denied when allowlist is empty and root not allowed",
			config:     MultiUserConfig{SystemUsers: []string{"hugo", "root"}},
			targetUser: "root",
			wantErr:    ErrRootNotAllowed,
		},
		{
			name: "root allowed when allowlist is empty and root allowed",
			config: MultiUserConfig{
				AllowRootTarget: true,
				SystemUsers:     []string{"hugo", "root"},
			},
			targetUser: "root",
			wantErr:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithMultiUser("token", nil, CookieSecureAuto, tt.config)
			err := g.ValidateTargetUser(tt.targetUser)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error %v, got nil", tt.wantErr)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTargetUserNilGuard(t *testing.T) {
	t.Parallel()

	var g *Guard
	if err := g.ValidateTargetUser("postgres"); err == nil {
		t.Error("nil guard should return error")
	}
}

func TestValidateTargetUserEmptySystemUsers(t *testing.T) {
	t.Parallel()

	g := NewWithMultiUser("token", nil, CookieSecureAuto, MultiUserConfig{
		SystemUsers: nil, // no system users loaded
	})

	// Empty target user should always pass.
	if err := g.ValidateTargetUser(""); err != nil {
		t.Errorf("empty user should pass: %v", err)
	}

	// Non-empty target user should fail when SystemUsers is empty.
	err := g.ValidateTargetUser("postgres")
	if err == nil {
		t.Fatal("expected error when SystemUsers is empty, got nil")
	}
	if !errors.Is(err, ErrNoSystemUsers) {
		t.Errorf("error = %v, want ErrNoSystemUsers", err)
	}
}

func TestAllowedUsers(t *testing.T) {
	t.Parallel()

	t.Run("returns nil when no config", func(t *testing.T) {
		t.Parallel()
		g := New("token", nil, CookieSecureAuto)
		if g.AllowedUsers() != nil {
			t.Errorf("AllowedUsers() = %v, want nil", g.AllowedUsers())
		}
	})

	t.Run("returns allowlist when configured", func(t *testing.T) {
		t.Parallel()
		g := NewWithMultiUser("token", nil, CookieSecureAuto, MultiUserConfig{
			AllowedUsers: []string{"postgres", "deploy"},
			SystemUsers:  []string{"postgres", "deploy", "hugo"},
		})
		users := g.AllowedUsers()
		if len(users) != 2 {
			t.Fatalf("AllowedUsers() = %v, want [postgres deploy]", users)
		}
	})

	t.Run("returns system users when no allowlist", func(t *testing.T) {
		t.Parallel()
		g := NewWithMultiUser("token", nil, CookieSecureAuto, MultiUserConfig{
			SystemUsers: []string{"deploy", "hugo", "root"},
		})
		users := g.AllowedUsers()
		// root should be filtered out (AllowRootTarget is false).
		if len(users) != 2 {
			t.Fatalf("AllowedUsers() = %v, want [deploy hugo]", users)
		}
	})

	t.Run("returns system users including root when allowed", func(t *testing.T) {
		t.Parallel()
		g := NewWithMultiUser("token", nil, CookieSecureAuto, MultiUserConfig{
			AllowRootTarget: true,
			SystemUsers:     []string{"deploy", "hugo", "root"},
		})
		users := g.AllowedUsers()
		if len(users) != 3 {
			t.Fatalf("AllowedUsers() = %v, want [deploy hugo root]", users)
		}
	})

	t.Run("nil guard returns nil", func(t *testing.T) {
		t.Parallel()
		var g *Guard
		if g.AllowedUsers() != nil {
			t.Errorf("nil guard AllowedUsers() should be nil")
		}
	})
}

func TestSystemUsers(t *testing.T) {
	t.Parallel()

	t.Run("returns system users list", func(t *testing.T) {
		t.Parallel()
		g := NewWithMultiUser("token", nil, CookieSecureAuto, MultiUserConfig{
			SystemUsers: []string{"deploy", "hugo"},
		})
		su := g.SystemUsers()
		if len(su) != 2 {
			t.Fatalf("SystemUsers() = %v, want [deploy hugo]", su)
		}
	})

	t.Run("nil guard returns nil", func(t *testing.T) {
		t.Parallel()
		var g *Guard
		if g.SystemUsers() != nil {
			t.Errorf("nil guard SystemUsers() should be nil")
		}
	})
}
