package security

import (
	"errors"
	"testing"
)

func TestValidateTargetUserEmptyAllowedUsers(t *testing.T) {
	t.Parallel()

	g := NewWithMultiUser("token", nil, CookieSecureAuto, MultiUserConfig{})
	if err := g.ValidateTargetUser(""); err != nil {
		t.Errorf("empty user should pass: %v", err)
	}
	// When AllowedUsers is empty, any non-root user is allowed.
	if err := g.ValidateTargetUser("postgres"); err != nil {
		t.Errorf("any user should pass when allowlist is empty: %v", err)
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
			},
			targetUser: "",
			wantErr:    nil,
		},
		{
			name: "allowed user passes",
			config: MultiUserConfig{
				AllowedUsers: []string{"postgres", "deploy"},
			},
			targetUser: "postgres",
			wantErr:    nil,
		},
		{
			name: "unknown user fails",
			config: MultiUserConfig{
				AllowedUsers: []string{"postgres"},
			},
			targetUser: "unknown",
			wantErr:    ErrUserNotAllowlist,
		},
		{
			name: "root denied when not allowed",
			config: MultiUserConfig{
				AllowedUsers:    []string{"root"},
				AllowRootTarget: false,
			},
			targetUser: "root",
			wantErr:    ErrRootNotAllowed,
		},
		{
			name: "root allowed when configured",
			config: MultiUserConfig{
				AllowedUsers:    []string{"root"},
				AllowRootTarget: true,
			},
			targetUser: "root",
			wantErr:    nil,
		},
		{
			name:       "any user allowed when allowlist is empty",
			config:     MultiUserConfig{},
			targetUser: "deploy",
			wantErr:    nil,
		},
		{
			name:       "root denied when allowlist is empty and root not allowed",
			config:     MultiUserConfig{},
			targetUser: "root",
			wantErr:    ErrRootNotAllowed,
		},
		{
			name: "root allowed when allowlist is empty and root allowed",
			config: MultiUserConfig{
				AllowRootTarget: true,
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

func TestAllowedUsers(t *testing.T) {
	t.Parallel()

	t.Run("returns nil when no config", func(t *testing.T) {
		t.Parallel()
		g := New("token", nil, CookieSecureAuto)
		if g.AllowedUsers() != nil {
			t.Errorf("AllowedUsers() = %v, want nil", g.AllowedUsers())
		}
	})

	t.Run("returns list when configured", func(t *testing.T) {
		t.Parallel()
		g := NewWithMultiUser("token", nil, CookieSecureAuto, MultiUserConfig{
			AllowedUsers: []string{"postgres", "deploy"},
		})
		users := g.AllowedUsers()
		if len(users) != 2 {
			t.Fatalf("AllowedUsers() = %v, want [postgres deploy]", users)
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
