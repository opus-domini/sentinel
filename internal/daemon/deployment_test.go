package daemon

import (
	"errors"
	"strings"
	"testing"
)

func TestSelectDeploymentMatrix(t *testing.T) {
	t.Parallel()

	user := Deployment{Scope: ScopeUser, BinaryPath: "/user/sentinel"}
	system := Deployment{Scope: ScopeSystem, BinaryPath: "/system/sentinel"}
	tests := []struct {
		name        string
		installed   []Deployment
		scope       string
		wantScope   string
		wantErr     error
		wantMessage string
	}{
		{name: "none auto", scope: ScopeAuto, wantErr: ErrNoServiceInstalled},
		{name: "user auto", installed: []Deployment{user}, scope: ScopeAuto, wantScope: ScopeUser},
		{name: "system auto", installed: []Deployment{system}, scope: ScopeAuto, wantScope: ScopeSystem},
		{name: "both auto", installed: []Deployment{user, system}, scope: ScopeAuto, wantErr: ErrAmbiguousDeployment},
		{name: "both explicit user", installed: []Deployment{user, system}, scope: ScopeUser, wantScope: ScopeUser},
		{name: "both explicit system", installed: []Deployment{user, system}, scope: ScopeSystem, wantScope: ScopeSystem},
		{name: "missing explicit scope", installed: []Deployment{user}, scope: ScopeSystem, wantMessage: "no Sentinel service is installed in system scope"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectDeployment(tc.installed, tc.scope)
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantMessage != "" && (err == nil || !strings.Contains(err.Error(), tc.wantMessage)) {
				t.Fatalf("error = %v, want message %q", err, tc.wantMessage)
			}
			if tc.wantErr == nil && tc.wantMessage == "" {
				if err != nil {
					t.Fatalf("selectDeployment() error = %v", err)
				}
				if got.Scope != tc.wantScope {
					t.Fatalf("scope = %q, want %q", got.Scope, tc.wantScope)
				}
			}
		})
	}
}

func TestSelectInstallScopeMatrix(t *testing.T) {
	t.Parallel()

	user := Deployment{Scope: ScopeUser}
	system := Deployment{Scope: ScopeSystem}
	tests := []struct {
		name      string
		installed []Deployment
		scope     string
		euid      int
		want      string
		wantErr   error
	}{
		{name: "fresh user", scope: ScopeAuto, euid: 1000, want: ScopeUser},
		{name: "fresh system", scope: ScopeAuto, euid: 0, want: ScopeSystem},
		{name: "fresh explicit user as root", scope: ScopeUser, euid: 0, want: ScopeUser},
		{name: "preserve user", installed: []Deployment{user}, scope: ScopeAuto, euid: 0, want: ScopeUser},
		{name: "preserve system", installed: []Deployment{system}, scope: ScopeAuto, euid: 1000, want: ScopeSystem},
		{name: "reject user to system", installed: []Deployment{user}, scope: ScopeSystem, euid: 0, wantErr: ErrAmbiguousDeployment},
		{name: "reject system to user", installed: []Deployment{system}, scope: ScopeUser, euid: 1000, wantErr: ErrAmbiguousDeployment},
		{name: "reject both", installed: []Deployment{user, system}, scope: ScopeAuto, euid: 1000, wantErr: ErrAmbiguousDeployment},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectInstallScope(tc.installed, tc.scope, tc.euid)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatal("selectInstallScope() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("selectInstallScope() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("scope = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseSystemdDeploymentFields(t *testing.T) {
	t.Parallel()

	unit := renderUserUnit("/opt/Sentinel App/sentinel", "/etc/sentinel/config.toml", "/var/lib/sentinel")
	binary, configPath := parseSystemdExecStart(unit)
	if binary != "/opt/Sentinel App/sentinel" {
		t.Fatalf("binary = %q", binary)
	}
	if configPath != "/etc/sentinel/config.toml" {
		t.Fatalf("config = %q", configPath)
	}
	if got := parseSystemdEnvironment(unit, "SENTINEL_DATA_DIR"); got != "/var/lib/sentinel" {
		t.Fatalf("data dir = %q", got)
	}
}

func TestParseLaunchdDeploymentFields(t *testing.T) {
	t.Parallel()

	plist := renderLaunchdUserServicePlist("/opt/sentinel", "/Library/Application Support/Sentinel/config.toml", "/Library/Application Support/Sentinel", "/tmp/out", "/tmp/err")
	binary, configPath := parseLaunchdProgramArguments(plist)
	if binary != "/opt/sentinel" || configPath != "/Library/Application Support/Sentinel/config.toml" {
		t.Fatalf("binary=%q config=%q", binary, configPath)
	}
	if got := parseLaunchdEnvironment(plist, "SENTINEL_DATA_DIR"); got != "/Library/Application Support/Sentinel" {
		t.Fatalf("data dir = %q", got)
	}
}
