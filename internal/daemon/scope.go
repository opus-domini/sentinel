package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ErrNoServiceInstalled is returned by service operations when no Sentinel
// service unit is installed in any scope.
var ErrNoServiceInstalled = errors.New("no Sentinel service is installed in the user or system scope")

// InstalledScopes reports which scopes currently hold a Sentinel service unit.
type InstalledScopes struct {
	User   bool
	System bool
}

// None reports whether no service unit is installed in any scope.
func (s InstalledScopes) None() bool { return !s.User && !s.System }

// ScopedServiceStatus is a service status tagged with the scope it came from.
type ScopedServiceStatus struct {
	Deployment
	UserServiceStatus
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// userScopeUnitPath returns the user-scope systemd unit path, independent of
// the caller's euid (unlike UserServicePath, which resolves to the system path
// for root).
func userScopeUnitPath() (string, error) {
	home, err := userScopeHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user", userUnitName), nil
}

// DetectInstalledScopes inspects the filesystem for a Sentinel service unit in
// the user and system scopes. It does not depend on the caller's euid, so a
// system service is detected even when queried as an unprivileged user.
func DetectInstalledScopes() InstalledScopes {
	if runtime.GOOS == launchdSupportedOS {
		return detectLaunchdScopes()
	}
	var scopes InstalledScopes
	if path, err := userScopeUnitPath(); err == nil && fileExists(path) {
		scopes.User = true
	}
	if fileExists(systemUnitPath) {
		scopes.System = true
	}
	return scopes
}

func detectLaunchdScopes() InstalledScopes {
	var scopes InstalledScopes
	if path, err := userServicePathLaunchdForScope(managerScopeUser); err == nil && fileExists(path) {
		scopes.User = true
	}
	if path, err := userServicePathLaunchdForScope(managerScopeSystem); err == nil && fileExists(path) {
		scopes.System = true
	}
	return scopes
}

// resolveServiceScope returns the only installed service scope. Ambiguous
// user+system installations are rejected instead of silently selected by euid.
func resolveServiceScope() (string, error) {
	return resolveServiceScopeForEUID(os.Geteuid())
}

func resolveServiceScopeForEUID(euid int) (string, error) {
	deployment, err := resolveDeploymentForEUID(ScopeAuto, euid)
	return deployment.Scope, err
}

// requireScopePrivilege errors when acting on scope needs a privilege the
// caller lacks — modifying a system unit requires root.
func requireScopePrivilege(scope string) error {
	return RequireScopeAccess(scope)
}

// ServiceStatus reports the status of every scope where a Sentinel service
// unit is installed. The slice is empty when nothing is installed.
func ServiceStatus() ([]ScopedServiceStatus, error) {
	if runtime.GOOS == launchdSupportedOS {
		return serviceStatusLaunchd()
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return nil, err
	}
	scopes := DetectInstalledScopes()
	var report []ScopedServiceStatus
	if scopes.User {
		deployment, err := readDeployment(managerScopeUser)
		if err != nil {
			return nil, err
		}
		st, err := userStatusUserLinux()
		if err != nil {
			return nil, err
		}
		report = append(report, ScopedServiceStatus{Deployment: deployment, UserServiceStatus: st})
	}
	if scopes.System {
		deployment, err := readDeployment(managerScopeSystem)
		if err != nil {
			return nil, err
		}
		st := userStatusSystemLinux()
		report = append(report, ScopedServiceStatus{Deployment: deployment, UserServiceStatus: st})
	}
	return report, nil
}

func serviceStatusLaunchd() ([]ScopedServiceStatus, error) {
	scopes := detectLaunchdScopes()
	var report []ScopedServiceStatus
	if scopes.User {
		deployment, err := readDeployment(managerScopeUser)
		if err != nil {
			return nil, err
		}
		st, err := userStatusLaunchdForScope(managerScopeUser)
		if err != nil {
			return nil, err
		}
		report = append(report, ScopedServiceStatus{Deployment: deployment, UserServiceStatus: st})
	}
	if scopes.System {
		deployment, err := readDeployment(managerScopeSystem)
		if err != nil {
			return nil, err
		}
		st, err := userStatusLaunchdForScope(managerScopeSystem)
		if err != nil {
			return nil, err
		}
		report = append(report, ScopedServiceStatus{Deployment: deployment, UserServiceStatus: st})
	}
	return report, nil
}
