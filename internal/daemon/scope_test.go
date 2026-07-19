package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInstalledScopesNone(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		scopes InstalledScopes
		want   bool
	}{
		{name: "empty", scopes: InstalledScopes{}, want: true},
		{name: "user only", scopes: InstalledScopes{User: true}, want: false},
		{name: "system only", scopes: InstalledScopes{System: true}, want: false},
		{name: "both", scopes: InstalledScopes{User: true, System: true}, want: false},
	}
	for _, tc := range cases {
		if got := tc.scopes.None(); got != tc.want {
			t.Errorf("%s: None() = %t, want %t", tc.name, got, tc.want)
		}
	}
}

func TestRequireScopePrivilege(t *testing.T) {
	t.Parallel()

	if err := requireScopePrivilege(managerScopeUser); err != nil {
		t.Fatalf("user scope should never require a privilege: %v", err)
	}
	err := requireScopePrivilege(managerScopeSystem)
	if os.Geteuid() == 0 {
		if err != nil {
			t.Fatalf("root acting on the system scope should be allowed: %v", err)
		}
	} else if err == nil {
		t.Fatal("a non-root caller acting on the system scope must be rejected")
	}
}

// writeUserUnit creates a user-scope unit file under a temporary HOME and
// returns once HOME is pointed at it.
func writeUserUnit(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	unit := renderUserUnit("/tmp/sentinel", filepath.Join(home, ".sentinel", "config.toml"), filepath.Join(home, ".sentinel"), filepath.Join(home, ".sentinel", "logs", "sentinel.log"))
	if err := os.WriteFile(filepath.Join(dir, userUnitName), []byte(unit), 0o600); err != nil {
		t.Fatalf("write unit: %v", err)
	}
}

func TestDetectInstalledScopesUser(t *testing.T) {
	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}

	t.Setenv("HOME", t.TempDir())
	if DetectInstalledScopes().User {
		t.Fatal("User = true with no unit file installed")
	}

	writeUserUnit(t)
	if !DetectInstalledScopes().User {
		t.Fatal("User = false after writing the user unit file")
	}
}

func TestResolveServiceScopeNoneInstalled(t *testing.T) {
	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	t.Setenv("HOME", t.TempDir())
	if DetectInstalledScopes().System {
		t.Skip("a system-scope sentinel unit exists on this host")
	}

	if _, err := resolveServiceScope(); !errors.Is(err, ErrNoServiceInstalled) {
		t.Fatalf("resolveServiceScope() error = %v, want ErrNoServiceInstalled", err)
	}
}

func TestResolveServiceScopeUser(t *testing.T) {
	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if DetectInstalledScopes().System {
		t.Skip("a system-scope sentinel unit exists on this host")
	}

	writeUserUnit(t)
	scope, err := resolveServiceScope()
	if err != nil {
		t.Fatalf("resolveServiceScope() error = %v", err)
	}
	if scope != managerScopeUser {
		t.Fatalf("scope = %q, want %q", scope, managerScopeUser)
	}
}

func TestServiceStatusReportsUserScope(t *testing.T) {
	if runtime.GOOS != systemdSupportedOS {
		t.Skip("test requires Linux")
	}
	if DetectInstalledScopes().System {
		t.Skip("a system-scope sentinel unit exists on this host")
	}

	writeUserUnit(t)
	report, err := ServiceStatus()
	if err != nil {
		t.Fatalf("ServiceStatus() error = %v", err)
	}
	if len(report) != 1 || report[0].Scope != managerScopeUser {
		t.Fatalf("report = %+v, want a single user-scope entry", report)
	}
	if !report[0].UnitFileExists {
		t.Fatal("user entry UnitFileExists = false, want true")
	}
}
