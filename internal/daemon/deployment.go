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

// ErrInstallScopeRequired reports a fresh installation where user intent is
// required. Install entrypoints may prompt interactively or require an
// explicit user/system scope in automation.
var ErrInstallScopeRequired = errors.New("no existing Sentinel installation was found; choose user or system scope")

// Deployment is the complete identity of one managed Sentinel installation.
// Service operations must keep these fields together instead of resolving
// scope, executable and configuration independently.
type Deployment struct {
	Scope             string
	UnitPath          string
	BinaryPath        string
	ConfigPath        string
	DataDir           string
	AutoUpdateService string
	AutoUpdateTimer   string
}

// ScopeLayout is the canonical filesystem layout for one managed deployment.
// Managed commands must resolve these paths together so configuration, mutable
// state and logs cannot drift into different installation scopes.
type ScopeLayout struct {
	ConfigPath string
	DataDir    string
	LogPath    string
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
	selected, err := selectAccessibleDeployment(detectedDeployments(), scope, os.Geteuid())
	if err != nil {
		return Deployment{}, err
	}
	return readDeployment(selected.Scope)
}

// ResolveInstallScope preserves an existing service or canonical standalone
// binary scope and refuses to create a second implicit installation. A fresh
// install with scope=auto requires an explicit user choice.
func ResolveInstallScope(scopeRaw string) (string, error) {
	scope, err := normalizeManagedScope(scopeRaw)
	if err != nil {
		return "", err
	}
	return selectInstallScope(detectedInstallations(), scope)
}

func detectedInstallations() []Deployment {
	managed := detectedDeployments()
	userBinary, userErr := CanonicalBinaryPath(ScopeUser)
	systemBinary, systemErr := CanonicalBinaryPath(ScopeSystem)
	return installationCandidates(
		managed,
		userErr == nil && regularFileExists(userBinary),
		systemErr == nil && regularFileExists(systemBinary),
		userBinary,
		systemBinary,
	)
}

func installationCandidates(managed []Deployment, hasUserBinary, hasSystemBinary bool, userBinary, systemBinary string) []Deployment {
	if len(managed) > 0 {
		return managed
	}
	installations := make([]Deployment, 0, 2)
	if hasUserBinary {
		installations = append(installations, Deployment{Scope: ScopeUser, BinaryPath: userBinary})
	}
	if hasSystemBinary {
		installations = append(installations, Deployment{Scope: ScopeSystem, BinaryPath: systemBinary})
	}
	return installations
}

func detectedDeployments() []Deployment {
	scopes := DetectInstalledScopes()
	deployments := make([]Deployment, 0, 2)
	if scopes.User {
		deployments = append(deployments, Deployment{Scope: ScopeUser})
	}
	if scopes.System {
		deployments = append(deployments, Deployment{Scope: ScopeSystem})
	}
	return deployments
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

func selectAccessibleDeployment(deployments []Deployment, scope string, euid int) (Deployment, error) {
	selected, err := selectDeployment(deployments, scope)
	if err != nil {
		return Deployment{}, err
	}
	// Diagnose a mismatched caller identity before reading a root-owned unit.
	// System unit files are intentionally private, so attempting to parse one as
	// an unprivileged user would otherwise hide the useful scope diagnosis behind
	// a generic permission-denied error.
	if err := requireScopeAccess(selected.Scope, euid); err != nil {
		return Deployment{}, err
	}
	return selected, nil
}

func selectInstallScope(deployments []Deployment, scope string) (string, error) {
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
	return "", ErrInstallScopeRequired
}

// CanonicalBinaryPath returns the default executable path for an installation
// scope. Existing managed services may retain a different explicit path.
func CanonicalBinaryPath(scope string) (string, error) {
	switch scope {
	case ScopeUser:
		home, err := userScopeHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "bin", "sentinel"), nil
	case ScopeSystem:
		return filepath.Join(string(filepath.Separator), "usr", "local", "bin", "sentinel"), nil
	default:
		return "", fmt.Errorf("invalid deployment scope: %s", scope)
	}
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// RequireScopeAccess returns an actionable error before any config or network
// work happens under the wrong identity.
func RequireScopeAccess(scope string) error {
	return requireScopeAccess(scope, os.Geteuid())
}

func requireScopeAccess(scope string, euid int) error {
	switch scope {
	case ScopeSystem:
		if euid != 0 {
			return errors.New("the Sentinel deployment is system-wide; re-run with sudo and --scope system")
		}
	case ScopeUser:
		if euid == 0 {
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

func normalizeExplicitScope(raw string) (string, error) {
	scope, err := normalizeManagedScope(raw)
	if err != nil {
		return "", err
	}
	if scope == ScopeAuto {
		return "", errors.New("deployment scope must be user or system")
	}
	return scope, nil
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

	if strings.TrimSpace(configPath) == "" {
		configPath, dataDir, err = legacyScopePaths(scope)
		if err != nil {
			return Deployment{}, err
		}
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
		AutoUpdateService: autoService,
		AutoUpdateTimer:   autoTimer,
	}, nil
}

// LayoutForScope returns the canonical filesystem layout for a managed scope.
func LayoutForScope(scope string) (ScopeLayout, error) {
	switch scope {
	case ScopeUser:
		home, homeErr := userScopeHomeDir()
		if homeErr != nil {
			return ScopeLayout{}, homeErr
		}
		dataDir := filepath.Join(home, ".sentinel")
		return ScopeLayout{
			ConfigPath: filepath.Join(dataDir, "config.toml"),
			DataDir:    dataDir,
			LogPath:    filepath.Join(dataDir, "logs", "sentinel.log"),
		}, nil
	case ScopeSystem:
		if runtime.GOOS == launchdSupportedOS {
			return ScopeLayout{
				ConfigPath: "/Library/Preferences/io.opusdomini.sentinel.toml",
				DataDir:    "/Library/Application Support/Sentinel",
				LogPath:    "/Library/Logs/Sentinel/sentinel.log",
			}, nil
		}
		return ScopeLayout{
			ConfigPath: "/etc/sentinel/config.toml",
			DataDir:    "/var/lib/sentinel",
			LogPath:    "/var/log/sentinel/sentinel.log",
		}, nil
	default:
		return ScopeLayout{}, fmt.Errorf("invalid deployment scope: %s", scope)
	}
}

// ScopePaths returns the canonical config and data paths for a fresh scope.
func ScopePaths(scope string) (configPath, dataDir string, err error) {
	layout, err := LayoutForScope(scope)
	if err != nil {
		return "", "", err
	}
	return layout.ConfigPath, layout.DataDir, nil
}

// HasCanonicalPaths reports whether a deployment unit points exclusively at
// the canonical config and data locations for its scope.
func HasCanonicalPaths(deployment Deployment) (bool, error) {
	layout, err := LayoutForScope(deployment.Scope)
	if err != nil {
		return false, err
	}
	return sameCleanPath(deployment.ConfigPath, layout.ConfigPath) &&
		sameCleanPath(deployment.DataDir, layout.DataDir), nil
}

func sameCleanPath(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	return left != "" && right != "" && filepath.Clean(left) == filepath.Clean(right)
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

// LegacyLayoutForScope returns the only historical managed layout accepted by
// the migration command. It is intentionally not used for fresh installs.
func LegacyLayoutForScope(scope string) (ScopeLayout, error) {
	configPath, dataDir, err := legacyScopePaths(scope)
	if err != nil {
		return ScopeLayout{}, err
	}
	return ScopeLayout{
		ConfigPath: configPath,
		DataDir:    dataDir,
		LogPath:    filepath.Join(dataDir, "logs", "sentinel.log"),
	}, nil
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
