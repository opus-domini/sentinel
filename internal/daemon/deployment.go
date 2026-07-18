package daemon

import (
	"errors"
	"fmt"
	"html"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// ScopeAuto selects the only installed deployment.
	ScopeAuto = managerScopeAuto
	// ScopeUser selects a per-user deployment.
	ScopeUser = managerScopeUser
	// ScopeSystem selects a machine-wide deployment.
	ScopeSystem = managerScopeSystem
)

// ErrAmbiguousDeployment reports simultaneous user and system installations.
var ErrAmbiguousDeployment = errors.New("sentinel is installed in both user and system scope")

// Deployment is the complete identity of one managed Sentinel installation.
// Service operations must keep these fields together instead of resolving
// scope, executable and configuration independently.
type Deployment struct {
	Scope             string
	UnitPath          string
	BinaryPath        string
	ConfigPath        string
	DataDir           string
	LegacyConfigPath  bool
	AutoUpdateService string
	AutoUpdateTimer   string
}

// InstalledDeployments returns every Sentinel deployment visible to the
// current user. When invoked through sudo, the user scope belongs to SUDO_USER.
func InstalledDeployments() ([]Deployment, error) {
	scopes := DetectInstalledScopes()
	deployments := make([]Deployment, 0, 2)
	if scopes.User {
		deployment, err := readDeployment(ScopeUser)
		if err != nil {
			return nil, err
		}
		deployments = append(deployments, deployment)
	}
	if scopes.System {
		deployment, err := readDeployment(ScopeSystem)
		if err != nil {
			return nil, err
		}
		deployments = append(deployments, deployment)
	}
	return deployments, nil
}

// ResolveDeployment selects an installed deployment. Auto succeeds only when
// exactly one deployment exists.
func ResolveDeployment(scopeRaw string) (Deployment, error) {
	scope, err := normalizeManagedScope(scopeRaw)
	if err != nil {
		return Deployment{}, err
	}
	deployments, err := InstalledDeployments()
	if err != nil {
		return Deployment{}, err
	}
	return selectDeployment(deployments, scope)
}

// ResolveInstallScope preserves an existing scope and refuses to create a
// second implicit deployment. A fresh install uses the caller privileges only
// when scope=auto.
func ResolveInstallScope(scopeRaw string) (string, error) {
	scope, err := normalizeManagedScope(scopeRaw)
	if err != nil {
		return "", err
	}
	deployments, err := InstalledDeployments()
	if err != nil {
		return "", err
	}
	return selectInstallScope(deployments, scope, os.Geteuid())
}

func selectDeployment(deployments []Deployment, scope string) (Deployment, error) {
	if scope == ScopeAuto {
		switch len(deployments) {
		case 0:
			return Deployment{}, ErrNoServiceInstalled
		case 1:
			return deployments[0], nil
		default:
			return Deployment{}, fmt.Errorf("%w; choose --scope user or --scope system", ErrAmbiguousDeployment)
		}
	}
	for _, deployment := range deployments {
		if deployment.Scope == scope {
			return deployment, nil
		}
	}
	return Deployment{}, fmt.Errorf("no Sentinel service is installed in %s scope", scope)
}

func selectInstallScope(deployments []Deployment, scope string, euid int) (string, error) {
	if len(deployments) > 1 {
		return "", fmt.Errorf("%w; remove one deployment before installing", ErrAmbiguousDeployment)
	}
	if len(deployments) == 1 {
		existing := deployments[0].Scope
		if scope == ScopeAuto || scope == existing {
			return existing, nil
		}
		return "", fmt.Errorf("sentinel is already installed in %s scope; uninstall it before installing in %s scope", existing, scope)
	}
	if scope != ScopeAuto {
		return scope, nil
	}
	if euid == 0 {
		return ScopeSystem, nil
	}
	return ScopeUser, nil
}

// RequireScopeAccess returns an actionable error before any config or network
// work happens under the wrong identity.
func RequireScopeAccess(scope string) error {
	switch scope {
	case ScopeSystem:
		if os.Geteuid() != 0 {
			return errors.New("the Sentinel deployment is system-wide; re-run with sudo and --scope system")
		}
	case ScopeUser:
		if os.Geteuid() == 0 {
			return errors.New("the Sentinel deployment belongs to a user; run the command as that user without sudo and use --scope user")
		}
	default:
		return fmt.Errorf("invalid deployment scope: %s", scope)
	}
	return nil
}

func normalizeManagedScope(raw string) (string, error) {
	scope := strings.ToLower(strings.TrimSpace(raw))
	switch scope {
	case "", ScopeAuto:
		return ScopeAuto, nil
	case ScopeUser, ScopeSystem:
		return scope, nil
	default:
		return "", fmt.Errorf("invalid scope %q (valid: auto, user, system)", raw)
	}
}

func readDeployment(scope string) (Deployment, error) {
	servicePath, err := servicePathForScope(scope)
	if err != nil {
		return Deployment{}, err
	}
	raw, err := os.ReadFile(servicePath) //nolint:gosec // fixed managed unit path.
	if err != nil {
		return Deployment{}, fmt.Errorf("read %s service definition: %w", scope, err)
	}

	var binaryPath, configPath, dataDir string
	if runtime.GOOS == launchdSupportedOS {
		binaryPath, configPath = parseLaunchdProgramArguments(string(raw))
		dataDir = parseLaunchdEnvironment(string(raw), "SENTINEL_DATA_DIR")
	} else {
		binaryPath, configPath = parseSystemdExecStart(string(raw))
		dataDir = parseSystemdEnvironment(string(raw), "SENTINEL_DATA_DIR")
	}
	if strings.TrimSpace(binaryPath) == "" {
		return Deployment{}, fmt.Errorf("%s service definition does not contain a Sentinel executable", scope)
	}

	legacy := false
	if strings.TrimSpace(configPath) == "" {
		configPath, dataDir, err = legacyScopePaths(scope)
		if err != nil {
			return Deployment{}, err
		}
		legacy = scope == ScopeSystem
	}
	if strings.TrimSpace(dataDir) == "" {
		dataDir = filepath.Dir(configPath)
	}
	autoService, err := UserAutoUpdateServicePathForScope(scope)
	if err != nil {
		return Deployment{}, err
	}
	autoTimer, err := UserAutoUpdateTimerPathForScope(scope)
	if err != nil {
		return Deployment{}, err
	}
	return Deployment{
		Scope:             scope,
		UnitPath:          servicePath,
		BinaryPath:        binaryPath,
		ConfigPath:        configPath,
		DataDir:           dataDir,
		LegacyConfigPath:  legacy,
		AutoUpdateService: autoService,
		AutoUpdateTimer:   autoTimer,
	}, nil
}

// ScopePaths returns the canonical config and data paths for a fresh scope.
func ScopePaths(scope string) (configPath, dataDir string, err error) {
	switch scope {
	case ScopeUser:
		home, homeErr := userScopeHomeDir()
		if homeErr != nil {
			return "", "", homeErr
		}
		dataDir = filepath.Join(home, ".sentinel")
		return filepath.Join(dataDir, "config.toml"), dataDir, nil
	case ScopeSystem:
		if runtime.GOOS == launchdSupportedOS {
			dataDir = "/Library/Application Support/Sentinel"
			return filepath.Join(dataDir, "config.toml"), dataDir, nil
		}
		return "/etc/sentinel/config.toml", "/var/lib/sentinel", nil
	default:
		return "", "", fmt.Errorf("invalid deployment scope: %s", scope)
	}
}

func legacyScopePaths(scope string) (configPath, dataDir string, err error) {
	if scope == ScopeUser {
		return ScopePaths(scope)
	}
	rootHome := "/root"
	if runtime.GOOS == launchdSupportedOS {
		rootHome = "/var/root"
	}
	dataDir = filepath.Join(rootHome, ".sentinel")
	return filepath.Join(dataDir, "config.toml"), dataDir, nil
}

func servicePathForScope(scope string) (string, error) {
	if runtime.GOOS == launchdSupportedOS {
		return userServicePathLaunchdForScope(scope)
	}
	if scope == ScopeSystem {
		return systemUnitPath, nil
	}
	if scope == ScopeUser {
		return userScopeUnitPath()
	}
	return "", fmt.Errorf("invalid deployment scope: %s", scope)
}

func userScopeHomeDir() (string, error) {
	if os.Geteuid() == 0 {
		if sudoUser := strings.TrimSpace(os.Getenv("SUDO_USER")); sudoUser != "" && sudoUser != "root" {
			account, err := user.Lookup(sudoUser)
			if err != nil {
				return "", fmt.Errorf("resolve sudo user %q: %w", sudoUser, err)
			}
			return account.HomeDir, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return home, nil
}

func parseSystemdExecStart(raw string) (binaryPath, configPath string) {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ExecStart=") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "ExecStart="))
		if len(fields) == 0 {
			return "", ""
		}
		binaryPath = unescapeSystemdValue(fields[0])
		for _, field := range fields[1:] {
			field = unescapeSystemdValue(field)
			if strings.HasPrefix(field, "--config=") {
				configPath = strings.TrimPrefix(field, "--config=")
			}
		}
		return binaryPath, configPath
	}
	return "", ""
}

func parseSystemdEnvironment(raw, key string) string {
	prefixes := []string{"Environment=" + key + "=", `Environment="` + key + "="}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		for _, prefix := range prefixes {
			if strings.HasPrefix(line, prefix) {
				value := strings.TrimPrefix(line, prefix)
				value = strings.TrimSuffix(value, `"`)
				return unescapeSystemdValue(value)
			}
		}
	}
	return ""
}

func unescapeSystemdValue(value string) string {
	value = strings.ReplaceAll(value, `\x20`, " ")
	return strings.ReplaceAll(value, `\\`, `\`)
}

func parseLaunchdProgramArguments(raw string) (binaryPath, configPath string) {
	start := strings.Index(raw, "<key>ProgramArguments</key>")
	if start < 0 {
		return "", ""
	}
	section := raw[start:]
	end := strings.Index(section, "</array>")
	if end < 0 {
		return "", ""
	}
	values := plistStringValues(section[:end])
	if len(values) == 0 {
		return "", ""
	}
	binaryPath = values[0]
	for _, value := range values[1:] {
		if strings.HasPrefix(value, "--config=") {
			configPath = strings.TrimPrefix(value, "--config=")
		}
	}
	return binaryPath, configPath
}

func parseLaunchdEnvironment(raw, key string) string {
	marker := "<key>" + key + "</key>"
	index := strings.Index(raw, marker)
	if index < 0 {
		return ""
	}
	values := plistStringValues(raw[index+len(marker):])
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func plistStringValues(raw string) []string {
	values := []string{}
	for {
		start := strings.Index(raw, "<string>")
		if start < 0 {
			return values
		}
		raw = raw[start+len("<string>"):]
		end := strings.Index(raw, "</string>")
		if end < 0 {
			return values
		}
		values = append(values, html.UnescapeString(raw[:end]))
		raw = raw[end+len("</string>"):]
	}
}
