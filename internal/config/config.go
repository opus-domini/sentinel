// Package config loads Sentinel configuration.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/opus-domini/sentinel/internal/humanize"
	"github.com/opus-domini/sentinel/internal/userswitch"
	"github.com/opus-domini/sentinel/internal/validate"
)

const (
	// CookieSecureAuto lets Sentinel decide the cookie Secure flag per request.
	CookieSecureAuto = "auto"
	// CookieSecureAlways always enables the cookie Secure flag.
	CookieSecureAlways = "always"
	// CookieSecureNever always disables the cookie Secure flag.
	CookieSecureNever = "never"

	// DefaultLogLevel is the fallback log level when config omits one.
	DefaultLogLevel = "info"

	configFileName = "config.toml"
	defaultHost    = "127.0.0.1"
	defaultPort    = 4040
)

// Config is the complete runtime configuration for Sentinel.
type Config struct {
	Version      int                `toml:"version" json:"version"`
	Server       ServerConfig       `toml:"server" json:"server"`
	Storage      StorageConfig      `toml:"storage" json:"storage"`
	Log          LogConfig          `toml:"log" json:"log"`
	HealthReport HealthReportConfig `toml:"health_report" json:"health_report"`
	Watchtower   WatchtowerConfig   `toml:"watchtower" json:"watchtower"`
	MCP          MCPConfig          `toml:"mcp" json:"mcp"`
	Runbooks     RunbooksConfig     `toml:"runbooks" json:"runbooks"`
	MultiUser    MultiUserConfig    `toml:"multi_user" json:"multi_user"`
	SystemUsers  []string           `toml:"-" json:"system_users"`
}

// ServerConfig controls the local HTTP API and web UI listener.
type ServerConfig struct {
	Host                string   `toml:"host" json:"host"`
	Port                int      `toml:"port" json:"port"`
	Token               string   `toml:"token" json:"token,omitempty"`
	AllowedOrigins      []string `toml:"allowed_origins" json:"allowed_origins"`
	TrustedProxies      []string `toml:"trusted_proxies" json:"trusted_proxies"`
	CookieSecure        string   `toml:"cookie_secure" json:"cookie_secure"`
	AllowInsecureCookie bool     `toml:"allow_insecure_cookie" json:"allow_insecure_cookie"`
	Timezone            string   `toml:"timezone" json:"timezone"`
	Locale              string   `toml:"locale" json:"locale"`
}

// StorageConfig controls the SQLite database location.
type StorageConfig struct {
	Path string `toml:"path" json:"path"`
}

// LogConfig controls daemon logging.
type LogConfig struct {
	Level string `toml:"level" json:"level"`
	Path  string `toml:"path" json:"path"`
}

// HealthReportConfig controls scheduled health report delivery.
type HealthReportConfig struct {
	WebhookURL string `toml:"webhook_url" json:"webhook_url"`
	Schedule   string `toml:"schedule" json:"schedule"`
}

// WatchtowerConfig represents watchtower config data.
type WatchtowerConfig struct {
	Enabled        bool          `toml:"enabled" json:"enabled"`
	TickInterval   time.Duration `toml:"tick_interval" json:"tick_interval"`
	CaptureLines   int           `toml:"capture_lines" json:"capture_lines"`
	CaptureTimeout time.Duration `toml:"capture_timeout" json:"capture_timeout"`
	JournalRows    int           `toml:"journal_rows" json:"journal_rows"`
}

// MCPConfig controls the HTTP Model Context Protocol endpoint.
type MCPConfig struct {
	Enabled bool `toml:"enabled" json:"enabled"`
}

// RunbooksConfig controls runbook execution behavior.
type RunbooksConfig struct {
	MaxConcurrent int `toml:"max_concurrent" json:"max_concurrent"`
}

// MultiUserConfig represents multi user config data.
type MultiUserConfig struct {
	AllowedUsers     []string `toml:"allowed_users" json:"allowed_users"`
	AllowRootTarget  bool     `toml:"allow_root_target" json:"allow_root_target"`
	UserSwitchMethod string   `toml:"user_switch_method" json:"user_switch_method"`
}

var (
	osUserHomeDir = os.UserHomeDir
	osCurrentUser = user.Current
	osGeteuid     = os.Geteuid
	osTempDir     = os.TempDir
)

// ErrConfigExists is returned when Init refuses to overwrite an existing file.
var ErrConfigExists = errors.New("config file already exists")

// Default returns Sentinel's built-in configuration.
func Default() Config {
	return DefaultForDataDir(defaultDataDir())
}

// DefaultForDataDir returns built-in defaults rooted at an explicit deployment
// data directory instead of the caller's HOME.
func DefaultForDataDir(dataRoot string) Config {
	dataRoot = strings.TrimSpace(dataRoot)
	if dataRoot == "" {
		dataRoot = defaultDataDir()
	}
	return Config{
		Version: 1,
		Server: ServerConfig{
			Host:         defaultHost,
			Port:         defaultPort,
			CookieSecure: CookieSecureAuto,
			Timezone:     time.Now().Location().String(),
		},
		Storage: StorageConfig{Path: filepath.Join(dataRoot, "sentinel.db")},
		Log:     LogConfig{Level: DefaultLogLevel, Path: filepath.Join(dataRoot, "logs", "sentinel.log")},
		Watchtower: WatchtowerConfig{
			Enabled:        true,
			TickInterval:   1 * time.Second,
			CaptureLines:   80,
			CaptureTimeout: 150 * time.Millisecond,
			JournalRows:    5000,
		},
		Runbooks: RunbooksConfig{MaxConcurrent: 5},
		MultiUser: MultiUserConfig{
			UserSwitchMethod: defaultUserSwitchMethod(),
		},
	}
}

// Address returns the configured HTTP listen address.
func (c Config) Address() string {
	return c.Server.Address()
}

// DataDir returns the directory that owns local Sentinel runtime data.
func (c Config) DataDir() string {
	if strings.TrimSpace(c.Storage.Path) == "" {
		return ""
	}
	return filepath.Dir(c.Storage.Path)
}

// Address returns the configured HTTP listen address.
func (c ServerConfig) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

// Path returns the resolved config file path for the current environment.
func Path() string {
	path := strings.TrimSpace(os.Getenv("SENTINEL_CONFIG"))
	if path == "" {
		path = filepath.Join(defaultDataDir(), configFileName)
	}
	resolved, err := ExpandPath(path)
	if err != nil {
		return path
	}
	return resolved
}

// Load reads the configured TOML file and returns the resolved effective config.
func Load() (Config, string, error) {
	return LoadPath(Path())
}

// LoadPath reads a TOML file and resolves defaults without creating files.
func LoadPath(path string) (Config, string, error) {
	return loadPathWithDefaults(path, Default())
}

// LoadPathForDataDir loads a deployment config with defaults rooted at its
// explicit data directory.
func LoadPathForDataDir(path, dataDir string) (Config, string, error) {
	return loadPathWithDefaults(path, DefaultForDataDir(dataDir))
}

func loadPathWithDefaults(path string, defaults Config) (Config, string, error) {
	resolved, err := resolvePathOrDefault(path)
	if err != nil {
		cfg := defaults
		return cfg, path, err
	}
	if _, err := os.Stat(resolved); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			cfg := defaults
			return cfg, resolved, fmt.Errorf("stat config file: %w", err)
		}
		cfg := defaults
		applyEnv(&cfg)
		return cfg, resolved, cfg.Resolve()
	}
	return loadExistingWithDefaults(resolved, true, defaults)
}

func loadExisting(path string, applyEnvironment bool) (Config, string, error) {
	return loadExistingWithDefaults(path, applyEnvironment, Default())
}

func loadExistingWithDefaults(path string, applyEnvironment bool, defaults Config) (Config, string, error) {
	cfg := defaults
	meta, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return cfg, path, fmt.Errorf("decode config: %w", err)
	}
	var issues []string
	for _, key := range meta.Undecoded() {
		issues = append(issues, "unknown key: "+strings.Join(key, "."))
	}
	if err := cfg.Resolve(); err != nil {
		issues = append(issues, err.Error())
	}
	if applyEnvironment {
		applyEnv(&cfg)
		if err := cfg.Resolve(); err != nil {
			issues = append(issues, err.Error())
		}
	}
	if len(issues) > 0 {
		return cfg, path, configValidationError{Path: path, Issues: issues}
	}
	return cfg, path, nil
}

// ValidateFile validates a Sentinel TOML config file.
func ValidateFile(path string) error {
	resolved, err := resolvePathOrDefault(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("config file not found: %s", resolved)
		}
		return fmt.Errorf("stat config file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("config path is a directory: %s", resolved)
	}
	_, _, err = loadExisting(resolved, false)
	return err
}

type configValidationError struct {
	Path   string
	Issues []string
}

func (e configValidationError) Error() string {
	return fmt.Sprintf("invalid config %s: %s", e.Path, strings.Join(e.Issues, "; "))
}

// Init creates the canonical config file. It refuses to overwrite existing
// files unless force is true.
func Init(force bool) (string, error) {
	return InitPath(Path(), defaultDataDir(), force)
}

// InitPath creates a config at an explicit deployment path and data root.
func InitPath(path, dataDir string, force bool) (string, error) {
	resolved, err := ExpandPath(path)
	if err != nil {
		return path, err
	}
	path = resolved
	if !force {
		if _, err := os.Stat(path); err == nil {
			return path, fmt.Errorf("%w: %s", ErrConfigExists, path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return path, fmt.Errorf("stat config file: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return path, fmt.Errorf("create config dir: %w", err)
	}

	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !force {
		flag |= os.O_EXCL
	}
	file, err := os.OpenFile(path, flag, 0o600) //nolint:gosec // canonical deployment config file.
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return path, fmt.Errorf("%w: %s", ErrConfigExists, path)
		}
		return path, fmt.Errorf("create config file: %w", err)
	}
	if _, err := file.Write(defaultConfigTOML(DefaultForDataDir(dataDir))); err != nil {
		_ = file.Close()
		return path, fmt.Errorf("write config file: %w", err)
	}
	if err := file.Close(); err != nil {
		return path, fmt.Errorf("close config file: %w", err)
	}
	return path, nil
}

// Ensure loads the config, creates it when missing, and creates local dirs.
func Ensure() (Config, string, error) {
	path := Path()
	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Config{}, path, fmt.Errorf("stat config file: %w", err)
		}
		if _, err := Init(false); err != nil && !errors.Is(err, ErrConfigExists) {
			return Config{}, path, err
		}
	}
	cfg, resolved, err := Load()
	if err != nil {
		return cfg, resolved, err
	}
	if err := EnsureDirs(cfg); err != nil {
		return cfg, resolved, err
	}
	return cfg, resolved, nil
}

// EnsureDirs creates local directories referenced by config paths.
func EnsureDirs(cfg Config) error {
	for label, path := range map[string]string{
		"storage": cfg.Storage.Path,
		"log":     cfg.Log.Path,
	} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("create %s dir: %w", label, err)
		}
	}
	return nil
}

// Resolve expands paths, normalizes defaults, and validates configured values.
func (c *Config) Resolve() error {
	if c == nil {
		return nil
	}
	defaults := Default()
	if c.Version == 0 {
		c.Version = defaults.Version
	}
	if strings.TrimSpace(c.Server.Host) == "" {
		c.Server.Host = defaults.Server.Host
	}
	if c.Server.Port == 0 {
		c.Server.Port = defaults.Server.Port
	}
	c.Server.CookieSecure = normalizeCookieSecure(c.Server.CookieSecure, defaults.Server.CookieSecure)
	c.Server.AllowedOrigins = cleanStrings(c.Server.AllowedOrigins)
	c.Server.TrustedProxies = cleanStrings(c.Server.TrustedProxies)
	c.Server.Locale = strings.TrimSpace(c.Server.Locale)
	c.Server.Timezone = strings.TrimSpace(c.Server.Timezone)
	if c.Server.Timezone == "" {
		c.Server.Timezone = defaults.Server.Timezone
	}
	if strings.TrimSpace(c.Storage.Path) == "" {
		c.Storage.Path = defaults.Storage.Path
	}
	if strings.TrimSpace(c.Log.Level) == "" {
		c.Log.Level = defaults.Log.Level
	}
	c.Log.Level = strings.ToLower(strings.TrimSpace(c.Log.Level))
	if strings.TrimSpace(c.Log.Path) == "" {
		c.Log.Path = defaults.Log.Path
	}
	if c.Runbooks.MaxConcurrent == 0 {
		c.Runbooks.MaxConcurrent = defaults.Runbooks.MaxConcurrent
	}
	if c.Watchtower.TickInterval == 0 {
		c.Watchtower.TickInterval = defaults.Watchtower.TickInterval
	}
	if c.Watchtower.CaptureLines == 0 {
		c.Watchtower.CaptureLines = defaults.Watchtower.CaptureLines
	}
	if c.Watchtower.CaptureTimeout == 0 {
		c.Watchtower.CaptureTimeout = defaults.Watchtower.CaptureTimeout
	}
	if c.Watchtower.JournalRows == 0 {
		c.Watchtower.JournalRows = defaults.Watchtower.JournalRows
	}
	c.MultiUser.AllowedUsers = cleanStrings(c.MultiUser.AllowedUsers)
	if strings.TrimSpace(c.MultiUser.UserSwitchMethod) == "" {
		c.MultiUser.UserSwitchMethod = defaults.MultiUser.UserSwitchMethod
	}
	c.MultiUser.UserSwitchMethod = userswitch.NormalizeMethod(c.MultiUser.UserSwitchMethod, defaults.MultiUser.UserSwitchMethod)

	var err error
	c.Storage.Path, err = ExpandPath(c.Storage.Path)
	if err != nil {
		return err
	}
	c.Log.Path, err = ExpandPath(c.Log.Path)
	if err != nil {
		return err
	}
	return validateConfig(*c)
}

func validateConfig(cfg Config) error {
	var issues []string
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		issues = append(issues, "server.port must be between 1 and 65535")
	}
	if err := validateListenAddress(cfg.Address()); err != nil {
		issues = append(issues, "server host and port must form a valid listen address: "+err.Error())
	}
	switch cfg.Server.CookieSecure {
	case CookieSecureAuto, CookieSecureAlways, CookieSecureNever:
	default:
		issues = append(issues, `server.cookie_secure must be one of "auto", "always", or "never"`)
	}
	for _, origin := range cfg.Server.AllowedOrigins {
		if err := validateAllowedOrigin(origin); err != nil {
			issues = append(issues, fmt.Sprintf("server.allowed_origins entry %q %v", origin, err))
		}
	}
	for _, proxy := range cfg.Server.TrustedProxies {
		if net.ParseIP(proxy) == nil {
			if _, _, err := net.ParseCIDR(proxy); err != nil {
				issues = append(issues, fmt.Sprintf(
					"server.trusted_proxies entry %q must be an IP address or CIDR",
					proxy,
				))
			}
		}
	}
	if cfg.Server.CookieSecure == CookieSecureAuto && len(cfg.Server.TrustedProxies) == 0 {
		for _, origin := range cfg.Server.AllowedOrigins {
			parsed, err := url.Parse(origin)
			if err == nil && parsed.Scheme == "https" {
				issues = append(issues,
					"server.trusted_proxies must contain the HTTPS proxy address when server.cookie_secure is \"auto\" and server.allowed_origins contains an HTTPS origin",
				)
				break
			}
		}
	}
	switch cfg.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		issues = append(issues, `log.level must be one of "debug", "info", "warn", or "error"`)
	}
	if err := validate.Timezone(cfg.Server.Timezone); err != nil {
		issues = append(issues, "server.timezone "+err.Error())
	}
	if cfg.Runbooks.MaxConcurrent <= 0 {
		issues = append(issues, "runbooks.max_concurrent must be a positive integer")
	}
	if cfg.Watchtower.TickInterval <= 0 {
		issues = append(issues, "watchtower.tick_interval must be a positive duration")
	}
	if cfg.Watchtower.CaptureLines <= 0 {
		issues = append(issues, "watchtower.capture_lines must be a positive integer")
	}
	if cfg.Watchtower.CaptureTimeout <= 0 {
		issues = append(issues, "watchtower.capture_timeout must be a positive duration")
	}
	if cfg.Watchtower.JournalRows <= 0 {
		issues = append(issues, "watchtower.journal_rows must be a positive integer")
	}
	if cfg.MCP.Enabled && strings.TrimSpace(cfg.Server.Token) == "" {
		issues = append(issues, "mcp.enabled requires server.token")
	}
	if strings.TrimSpace(cfg.HealthReport.Schedule) != "" {
		if err := validate.CronExpression(cfg.HealthReport.Schedule); err != nil {
			issues = append(issues, "health_report.schedule "+err.Error())
		}
	}
	if len(issues) > 0 {
		return errors.New(strings.Join(issues, "; "))
	}
	return nil
}

func validateAllowedOrigin(origin string) error {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return errors.New("must be an absolute HTTP or HTTPS origin")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("must use the http or https scheme")
	}
	if parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("must not contain credentials, a path, query parameters, or a fragment")
	}
	canonical := parsed.Scheme + "://" + parsed.Host
	if origin != canonical {
		return fmt.Errorf("must use canonical form %q", canonical)
	}
	return nil
}

func applyEnv(cfg *Config) {
	if cfg == nil {
		return
	}
	applyServerEnv(cfg)
	applyStorageEnv(cfg)
	applyLogEnv(cfg)
	applyHealthReportEnv(cfg)
	applyWatchtowerEnv(cfg)
	applyMCPEnv(cfg)
	applyRunbooksEnv(cfg)
	applyMultiUserEnv(cfg)
}

func applyServerEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("SENTINEL_SERVER_HOST")); v != "" {
		cfg.Server.Host = v
	}
	if raw, ok := os.LookupEnv("SENTINEL_SERVER_PORT"); ok {
		v := strings.TrimSpace(raw)
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 1 && parsed <= 65535 {
			cfg.Server.Port = parsed
		} else if v != "" {
			slog.Warn("ignoring invalid SENTINEL_SERVER_PORT", "value", raw)
		}
	}
	if v := strings.TrimSpace(os.Getenv("SENTINEL_SERVER_TOKEN")); v != "" {
		cfg.Server.Token = v
	}
	if v := strings.TrimSpace(os.Getenv("SENTINEL_SERVER_ALLOWED_ORIGINS")); v != "" {
		cfg.Server.AllowedOrigins = splitCSV(v)
	}
	if v := strings.TrimSpace(os.Getenv("SENTINEL_SERVER_TRUSTED_PROXIES")); v != "" {
		cfg.Server.TrustedProxies = splitCSV(v)
	}
	if v := strings.TrimSpace(os.Getenv("SENTINEL_SERVER_COOKIE_SECURE")); v != "" {
		cfg.Server.CookieSecure = v
	}
	if v := strings.TrimSpace(os.Getenv("SENTINEL_SERVER_ALLOW_INSECURE_COOKIE")); v != "" {
		if parsed, ok := parseBool(v); ok {
			cfg.Server.AllowInsecureCookie = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("SENTINEL_SERVER_TIMEZONE")); v != "" {
		cfg.Server.Timezone = v
	}
	if v := strings.TrimSpace(os.Getenv("SENTINEL_SERVER_LOCALE")); v != "" {
		cfg.Server.Locale = v
	}
}

func applyStorageEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("SENTINEL_STORAGE_PATH")); v != "" {
		cfg.Storage.Path = v
	}
}

func applyLogEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("SENTINEL_LOG_LEVEL")); v != "" {
		cfg.Log.Level = v
	}
	if v := strings.TrimSpace(os.Getenv("SENTINEL_LOG_PATH")); v != "" {
		cfg.Log.Path = v
	}
}

func applyHealthReportEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("SENTINEL_HEALTH_REPORT_WEBHOOK_URL")); v != "" {
		cfg.HealthReport.WebhookURL = v
	}
	if v := strings.TrimSpace(os.Getenv("SENTINEL_HEALTH_REPORT_SCHEDULE")); v != "" {
		cfg.HealthReport.Schedule = v
	}
}

func applyWatchtowerEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("SENTINEL_WATCHTOWER_ENABLED")); v != "" {
		if parsed, ok := parseBool(v); ok {
			cfg.Watchtower.Enabled = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("SENTINEL_WATCHTOWER_TICK_INTERVAL")); v != "" {
		if parsed, ok := parseDuration(v); ok {
			cfg.Watchtower.TickInterval = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("SENTINEL_WATCHTOWER_CAPTURE_LINES")); v != "" {
		if parsed, ok := parsePositiveInt(v); ok {
			cfg.Watchtower.CaptureLines = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("SENTINEL_WATCHTOWER_CAPTURE_TIMEOUT")); v != "" {
		if parsed, ok := parseDuration(v); ok {
			cfg.Watchtower.CaptureTimeout = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("SENTINEL_WATCHTOWER_JOURNAL_ROWS")); v != "" {
		if parsed, ok := parsePositiveInt(v); ok {
			cfg.Watchtower.JournalRows = parsed
		}
	}
}

func applyMCPEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("SENTINEL_MCP_ENABLED")); v != "" {
		if parsed, ok := parseBool(v); ok {
			cfg.MCP.Enabled = parsed
		}
	}
}

func applyRunbooksEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("SENTINEL_RUNBOOK_MAX_CONCURRENT")); v != "" {
		if parsed, ok := parsePositiveInt(v); ok {
			cfg.Runbooks.MaxConcurrent = parsed
		}
	}
}

func applyMultiUserEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("SENTINEL_ALLOWED_USERS")); v != "" {
		cfg.MultiUser.AllowedUsers = splitCSV(v)
	}
	if v := strings.TrimSpace(os.Getenv("SENTINEL_ALLOW_ROOT_TARGET")); v != "" {
		if parsed, ok := parseBool(v); ok {
			cfg.MultiUser.AllowRootTarget = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("SENTINEL_USER_SWITCH_METHOD")); v != "" {
		cfg.MultiUser.UserSwitchMethod = v
	}
}

func defaultConfigTOML(cfg Config) []byte {
	var b strings.Builder
	writeConfigLine(&b, "# Sentinel configuration")
	writeConfigLine(&b, "#")
	writeConfigLine(&b, "# This file is created by `sentinel config init`. Fields you remove fall back to")
	writeConfigLine(&b, "# built-in defaults. Paths support ~ and $ENV expansion.")
	writeConfigLine(&b, "")
	writeConfigLine(&b, "# Config schema version.")
	writeConfigLine(&b, "version = %d", cfg.Version)
	writeConfigLine(&b, "")
	writeConfigLine(&b, "# Local HTTP API and embedded web UI.")
	writeConfigLine(&b, "[server]")
	writeConfigLine(&b, "  # Keep localhost unless you also set server.token.")
	writeConfigLine(&b, "  # Environment variable: SENTINEL_SERVER_HOST")
	writeConfigLine(&b, "  host = %q", cfg.Server.Host)
	writeConfigLine(&b, "  # Environment variable: SENTINEL_SERVER_PORT")
	writeConfigLine(&b, "  port = %d", cfg.Server.Port)
	writeConfigLine(&b, "  # Environment variable: SENTINEL_SERVER_TOKEN")
	writeConfigLine(&b, "  token = %q", cfg.Server.Token)
	writeConfigLine(&b, "  # Environment variable: SENTINEL_SERVER_ALLOWED_ORIGINS")
	writeConfigLine(&b, "  allowed_origins = [%s]", quoteStringList(cfg.Server.AllowedOrigins))
	writeConfigLine(&b, "  # Trusted proxy IPs/CIDRs allowed to set X-Forwarded-Proto.")
	writeConfigLine(&b, "  # Environment variable: SENTINEL_SERVER_TRUSTED_PROXIES")
	writeConfigLine(&b, "  trusted_proxies = [%s]", quoteStringList(cfg.Server.TrustedProxies))
	writeConfigLine(&b, "  # Cookie Secure flag: auto, always, or never.")
	writeConfigLine(&b, "  # Environment variable: SENTINEL_SERVER_COOKIE_SECURE")
	writeConfigLine(&b, "  cookie_secure = %q", cfg.Server.CookieSecure)
	writeConfigLine(&b, "  # Environment variable: SENTINEL_SERVER_ALLOW_INSECURE_COOKIE")
	writeConfigLine(&b, "  allow_insecure_cookie = %t", cfg.Server.AllowInsecureCookie)
	writeConfigLine(&b, "  # IANA timezone for all displayed timestamps.")
	writeConfigLine(&b, "  # Environment variable: SENTINEL_SERVER_TIMEZONE")
	writeConfigLine(&b, "  timezone = %q", cfg.Server.Timezone)
	writeConfigLine(&b, "  # BCP 47 locale for date/number formatting. Empty uses browser default.")
	writeConfigLine(&b, "  # Environment variable: SENTINEL_SERVER_LOCALE")
	writeConfigLine(&b, "  locale = %q", cfg.Server.Locale)
	writeConfigLine(&b, "")
	writeConfigLine(&b, "# Local SQLite database.")
	writeConfigLine(&b, "[storage]")
	writeConfigLine(&b, "  # Environment variable: SENTINEL_STORAGE_PATH")
	writeConfigLine(&b, "  path = %q", cfg.Storage.Path)
	writeConfigLine(&b, "")
	writeConfigLine(&b, "# Daemon logging.")
	writeConfigLine(&b, "[log]")
	writeConfigLine(&b, "  # Environment variable: SENTINEL_LOG_LEVEL")
	writeConfigLine(&b, "  level = %q", cfg.Log.Level)
	writeConfigLine(&b, "  # Environment variable: SENTINEL_LOG_PATH")
	writeConfigLine(&b, "  path = %q", cfg.Log.Path)
	writeConfigLine(&b, "")
	writeConfigLine(&b, "# Scheduled health report delivery.")
	writeConfigLine(&b, "[health_report]")
	writeConfigLine(&b, "  # Environment variable: SENTINEL_HEALTH_REPORT_WEBHOOK_URL")
	writeConfigLine(&b, "  webhook_url = %q", cfg.HealthReport.WebhookURL)
	writeConfigLine(&b, "  # Environment variable: SENTINEL_HEALTH_REPORT_SCHEDULE")
	writeConfigLine(&b, "  schedule = %q", cfg.HealthReport.Schedule)
	writeConfigLine(&b, "")
	writeConfigLine(&b, "# Background activity projection and unread journal.")
	writeConfigLine(&b, "[watchtower]")
	writeConfigLine(&b, "  # Environment variable: SENTINEL_WATCHTOWER_ENABLED")
	writeConfigLine(&b, "  enabled = %t", cfg.Watchtower.Enabled)
	writeConfigLine(&b, "  # Environment variable: SENTINEL_WATCHTOWER_TICK_INTERVAL")
	writeConfigLine(&b, "  tick_interval = %q", humanize.Duration(cfg.Watchtower.TickInterval))
	writeConfigLine(&b, "  # Environment variable: SENTINEL_WATCHTOWER_CAPTURE_LINES")
	writeConfigLine(&b, "  capture_lines = %d", cfg.Watchtower.CaptureLines)
	writeConfigLine(&b, "  # Environment variable: SENTINEL_WATCHTOWER_CAPTURE_TIMEOUT")
	writeConfigLine(&b, "  capture_timeout = %q", humanize.Duration(cfg.Watchtower.CaptureTimeout))
	writeConfigLine(&b, "  # Environment variable: SENTINEL_WATCHTOWER_JOURNAL_ROWS")
	writeConfigLine(&b, "  journal_rows = %d", cfg.Watchtower.JournalRows)
	writeConfigLine(&b, "")
	writeConfigLine(&b, "# Model Context Protocol endpoint at /mcp.")
	writeConfigLine(&b, "[mcp]")
	writeConfigLine(&b, "  # Requires server.token and uses it as the Bearer token.")
	writeConfigLine(&b, "  # Environment variable: SENTINEL_MCP_ENABLED")
	writeConfigLine(&b, "  enabled = %t", cfg.MCP.Enabled)
	writeConfigLine(&b, "")
	writeConfigLine(&b, "# Manual runbook execution.")
	writeConfigLine(&b, "[runbooks]")
	writeConfigLine(&b, "  # Environment variable: SENTINEL_RUNBOOK_MAX_CONCURRENT")
	writeConfigLine(&b, "  max_concurrent = %d", cfg.Runbooks.MaxConcurrent)
	writeConfigLine(&b, "")
	writeConfigLine(&b, "# OS-user session targeting.")
	writeConfigLine(&b, "[multi_user]")
	writeConfigLine(&b, "  # Environment variable: SENTINEL_ALLOWED_USERS")
	writeConfigLine(&b, "  allowed_users = [%s]", quoteStringList(cfg.MultiUser.AllowedUsers))
	writeConfigLine(&b, "  # Environment variable: SENTINEL_ALLOW_ROOT_TARGET")
	writeConfigLine(&b, "  allow_root_target = %t", cfg.MultiUser.AllowRootTarget)
	writeConfigLine(&b, "  # Environment variable: SENTINEL_USER_SWITCH_METHOD")
	writeConfigLine(&b, "  user_switch_method = %q", cfg.MultiUser.UserSwitchMethod)
	return []byte(b.String())
}

func resolvePathOrDefault(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = Path()
	}
	return ExpandPath(path)
}

func defaultDataDir() string {
	if v := strings.TrimSpace(os.Getenv("SENTINEL_DATA_DIR")); v != "" {
		return v
	}
	if home, err := resolveHomeDir(); err == nil {
		return filepath.Join(home, ".sentinel")
	}
	// Last-resort fallback for restricted service environments.
	return filepath.Join(osTempDir(), "sentinel")
}

// ExpandPath expands ~ and environment variables, then returns an absolute path.
func ExpandPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("path is required")
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := resolveHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	path = os.ExpandEnv(path)
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func validateListenAddress(addr string) error {
	_, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return err
	}
	value, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid port %q", port)
	}
	if value < 1 || value > 65535 {
		return fmt.Errorf("port %d out of range", value)
	}
	return nil
}

func normalizeCookieSecure(value, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case CookieSecureAuto, CookieSecureAlways, CookieSecureNever:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return fallback
	}
}

func splitCSV(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func parseBool(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func parseDuration(raw string) (time.Duration, bool) {
	v, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

func parsePositiveInt(raw string) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}

func defaultUserSwitchMethod() string {
	return userswitch.DefaultMethod(runtime.GOOS)
}

// ValidateMultiUser checks multi-user config consistency and logs warnings
// for recoverable issues. It modifies the config in place when corrections
// are needed (e.g. removing root from allowlist).
func ValidateMultiUser(cfg *Config) {
	if cfg == nil {
		return
	}

	// Remove root from allowlist if allow_root_target is false.
	if !cfg.MultiUser.AllowRootTarget {
		filtered := cfg.MultiUser.AllowedUsers[:0]
		for _, u := range cfg.MultiUser.AllowedUsers {
			if u == "root" {
				slog.Warn("removing root from allowed_users because allow_root_target is false")
				continue
			}
			filtered = append(filtered, u)
		}
		cfg.MultiUser.AllowedUsers = filtered
	}

	// Cross-reference allowed_users against system users.
	if len(cfg.SystemUsers) > 0 {
		systemSet := make(map[string]struct{}, len(cfg.SystemUsers))
		for _, u := range cfg.SystemUsers {
			systemSet[u] = struct{}{}
		}
		for _, u := range cfg.MultiUser.AllowedUsers {
			if _, ok := systemSet[u]; !ok {
				slog.Warn("allowed_users entry not found in system users", "user", u)
			}
		}
	}
}

func quoteStringList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	return strings.Join(quoted, ", ")
}

func writeConfigLine(b *strings.Builder, format string, args ...any) {
	if len(args) == 0 {
		b.WriteString(format)
	} else {
		_, _ = fmt.Fprintf(b, format, args...)
	}
	b.WriteByte('\n')
}

func resolveHomeDir() (string, error) {
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return home, nil
	}
	if home, err := osUserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return strings.TrimSpace(home), nil
	}
	if current, err := osCurrentUser(); err == nil && current != nil {
		if home := strings.TrimSpace(current.HomeDir); home != "" {
			return home, nil
		}
	}
	if osGeteuid() == 0 {
		// System services may run without HOME set.
		if runtime.GOOS == "darwin" {
			return "/var/root", nil
		}
		return "/root", nil
	}
	return "", errors.New("home directory not found")
}
