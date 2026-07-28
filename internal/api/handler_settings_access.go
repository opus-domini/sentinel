package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/daemon"
)

const (
	settingsSchemeHTTP       = "http"
	settingsSchemeHTTPS      = "https"
	settingsListenerWildcard = "wildcard"
	settingsListenerLoopback = "loopback"
)

type settingsAccess struct {
	Listener       settingsListener       `json:"listener"`
	Authentication settingsAuthentication `json:"authentication"`
	Origins        settingsOrigins        `json:"origins"`
	Proxies        settingsProxies        `json:"proxies"`
	Cookies        settingsCookies        `json:"cookies"`
	Recovery       settingsRecovery       `json:"recovery"`
}

type settingsListener struct {
	Host           stringSetting  `json:"host"`
	Port           integerSetting `json:"port"`
	Classification string         `json:"classification"`
	Address        string         `json:"address"`
}

type settingsAuthentication struct {
	Token                  sensitiveSetting `json:"token"`
	RuntimeTokenConfigured bool             `json:"runtimeTokenConfigured"`
}

type settingsOrigins struct {
	Allowed stringListSetting `json:"allowed"`
}

type settingsProxies struct {
	Trusted stringListSetting `json:"trusted"`
}

type settingsCookies struct {
	Secure        stringSetting `json:"secure"`
	AllowInsecure boolSetting   `json:"allowInsecure"`
}

type settingsRecovery struct {
	ConfigPath      string `json:"configPath"`
	BackupPath      string `json:"backupPath"`
	RestoreCommand  string `json:"restoreCommand"`
	ValidateCommand string `json:"validateCommand"`
	RestartCommand  string `json:"restartCommand,omitempty"`
	Instruction     string `json:"instruction"`
}

type patchAccess struct {
	ReconnectOrigin     string               `json:"reconnectOrigin"`
	Host                *string              `json:"host,omitempty"`
	Port                *int                 `json:"port,omitempty"`
	Token               *patchSecretMutation `json:"token,omitempty"`
	AllowedOrigins      *[]string            `json:"allowedOrigins,omitempty"`
	TrustedProxies      *[]string            `json:"trustedProxies,omitempty"`
	CookieSecure        *string              `json:"cookieSecure,omitempty"`
	AllowInsecureCookie *bool                `json:"allowInsecureCookie,omitempty"`
}

func accessSettingsKeys(req *patchAccess) []string {
	if req == nil {
		return nil
	}
	keys := make([]string, 0, 7)
	if req.Host != nil {
		keys = append(keys, config.FieldServerHost)
	}
	if req.Port != nil {
		keys = append(keys, config.FieldServerPort)
	}
	if secretMutationChangesValue(req.Token) {
		keys = append(keys, config.FieldServerToken)
	}
	if req.AllowedOrigins != nil {
		keys = append(keys, config.FieldServerAllowedOrigins)
	}
	if req.TrustedProxies != nil {
		keys = append(keys, config.FieldServerTrustedProxies)
	}
	if req.CookieSecure != nil {
		keys = append(keys, config.FieldServerCookieSecure)
	}
	if req.AllowInsecureCookie != nil {
		keys = append(keys, config.FieldServerAllowInsecureCookie)
	}
	return keys
}

func validateAccessSecretMutation(req *patchAccess) error {
	if req == nil {
		return nil
	}
	return validateSecretMutation("access.token", req.Token)
}

func (h *Handler) applyAccessSettings(candidate *config.Config, req *patchAccess) error {
	if req == nil {
		return nil
	}
	if req.Host != nil {
		host := strings.TrimSpace(*req.Host)
		if host == "" {
			return h.accessValidationError("server.host is required")
		}
		candidate.Server.Host = host
	}
	if req.Port != nil {
		candidate.Server.Port = *req.Port
	}
	applySecretMutation(&candidate.Server.Token, req.Token)
	if req.AllowedOrigins != nil {
		values, err := h.validateAccessList(config.FieldServerAllowedOrigins, *req.AllowedOrigins)
		if err != nil {
			return err
		}
		candidate.Server.AllowedOrigins = values
	}
	if req.TrustedProxies != nil {
		values, err := h.validateAccessList(config.FieldServerTrustedProxies, *req.TrustedProxies)
		if err != nil {
			return err
		}
		candidate.Server.TrustedProxies = values
	}
	if req.CookieSecure != nil {
		candidate.Server.CookieSecure = strings.TrimSpace(*req.CookieSecure)
	}
	if req.AllowInsecureCookie != nil {
		candidate.Server.AllowInsecureCookie = *req.AllowInsecureCookie
	}
	return nil
}

func (h *Handler) validateAccessList(field string, values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" || value != raw {
			return nil, h.accessValidationError(field + " entries must be non-empty and trimmed")
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, h.accessValidationError(field + " contains duplicate entry " + strconv.Quote(value))
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func (h *Handler) validateAccessRequest(req *patchAccess) error {
	if req == nil {
		return nil
	}
	if strings.TrimSpace(req.ReconnectOrigin) == "" {
		return h.accessValidationError("access.reconnectOrigin is required for every access update")
	}
	if _, err := parseCanonicalSettingsOrigin(req.ReconnectOrigin); err != nil {
		return h.accessValidationError("access.reconnectOrigin " + err.Error())
	}
	return nil
}

func (h *Handler) accessCandidatePreflight(
	r *http.Request,
	req *patchAccess,
) config.CandidatePreflight {
	if req == nil {
		return nil
	}
	return func(
		_ config.State,
		baseline config.Config,
		_ config.Config,
		effective config.Config,
		actualKeys []string,
	) error {
		if err := h.validateReconnectOrigin(r, req.ReconnectOrigin, effective); err != nil {
			return err
		}
		if !slices.Contains(actualKeys, config.FieldServerHost) &&
			!slices.Contains(actualKeys, config.FieldServerPort) {
			return nil
		}
		check := preflightSettingsBind
		if h != nil && h.settingsBindCheck != nil {
			check = h.settingsBindCheck
		}
		if err := check(baseline.Address(), effective.Address()); err != nil {
			return h.accessValidationError(err.Error())
		}
		return nil
	}
}

func (h *Handler) validateReconnectOrigin(
	r *http.Request,
	raw string,
	candidate config.Config,
) error {
	reconnect, err := parseCanonicalSettingsOrigin(raw)
	if err != nil {
		return h.accessValidationError("access.reconnectOrigin " + err.Error())
	}
	expected, err := expectedReconnectOrigin(r, candidate.Server)
	if err != nil {
		return h.accessValidationError("access.reconnectOrigin cannot be derived: " + err.Error())
	}
	if reconnect != expected {
		return h.accessValidationError(
			fmt.Sprintf("access.reconnectOrigin must be %q for the candidate listener", expected),
		)
	}
	parsed, _ := url.Parse(reconnect)
	if strings.TrimSpace(candidate.Server.Token) != "" &&
		candidate.Server.CookieSecure == config.CookieSecureAlways &&
		parsed.Scheme != settingsSchemeHTTPS {
		return h.accessValidationError(
			"server.cookie_secure=always requires an HTTPS reconnect origin when authentication is enabled",
		)
	}
	return nil
}

func parseCanonicalSettingsOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", errors.New("must be an absolute HTTP or HTTPS origin")
	}
	if parsed.Scheme != settingsSchemeHTTP && parsed.Scheme != settingsSchemeHTTPS {
		return "", errors.New("must use the http or https scheme")
	}
	if parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("must not contain credentials, a path, query parameters, or a fragment")
	}
	canonical := parsed.Scheme + "://" + parsed.Host
	if raw != canonical {
		return "", fmt.Errorf("must use canonical form %q", canonical)
	}
	return canonical, nil
}

func expectedReconnectOrigin(r *http.Request, candidate config.ServerConfig) (string, error) {
	scheme, currentHost := settingsRequestEndpoint(r)
	host := strings.TrimSpace(candidate.Host)
	if settingsListenerClassification(host) == settingsListenerWildcard {
		host = currentHost
	}
	if host == "" {
		return "", errors.New("current browser hostname is unavailable")
	}
	return scheme + "://" + net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(candidate.Port)), nil
}

func settingsRequestEndpoint(r *http.Request) (string, string) {
	scheme := settingsSchemeHTTP
	hostPort := ""
	if r != nil {
		if origin, err := url.Parse(strings.TrimSpace(r.Header.Get("Origin"))); err == nil &&
			(origin.Scheme == settingsSchemeHTTP || origin.Scheme == settingsSchemeHTTPS) &&
			origin.Host != "" {
			scheme = origin.Scheme
			hostPort = origin.Host
		} else {
			if r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), settingsSchemeHTTPS) {
				scheme = settingsSchemeHTTPS
			}
			hostPort = r.Host
		}
	}
	host, _, err := net.SplitHostPort(hostPort)
	if err == nil {
		return scheme, strings.Trim(host, "[]")
	}
	return scheme, strings.Trim(hostPort, "[]")
}

func settingsListenerClassification(host string) string {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" {
		return settingsListenerWildcard
	}
	if strings.EqualFold(host, "localhost") {
		return settingsListenerLoopback
	}
	if ip := net.ParseIP(host); ip != nil {
		switch {
		case ip.IsUnspecified():
			return settingsListenerWildcard
		case ip.IsLoopback():
			return settingsListenerLoopback
		}
	}
	return "specific"
}

func preflightSettingsBind(currentAddress, candidateAddress string) error {
	if listenerAddressesOverlap(currentAddress, candidateAddress) {
		return nil
	}
	var listenerConfig net.ListenConfig
	listener, err := listenerConfig.Listen(context.Background(), "tcp", candidateAddress)
	if err != nil {
		return fmt.Errorf("server listener preflight could not bind candidate address %q", candidateAddress)
	}
	return listener.Close()
}

func listenerAddressesOverlap(currentAddress, candidateAddress string) bool {
	currentHost, currentPort, currentErr := net.SplitHostPort(currentAddress)
	candidateHost, candidatePort, candidateErr := net.SplitHostPort(candidateAddress)
	if currentErr != nil || candidateErr != nil || currentPort != candidatePort {
		return false
	}
	if strings.EqualFold(currentHost, candidateHost) {
		return true
	}
	currentKind := settingsListenerClassification(currentHost)
	candidateKind := settingsListenerClassification(candidateHost)
	return currentKind == settingsListenerWildcard ||
		candidateKind == settingsListenerWildcard ||
		currentKind == settingsListenerLoopback && candidateKind == settingsListenerLoopback
}

func (h *Handler) accessValidationError(issue string) error {
	path := ""
	if h != nil && h.configService != nil {
		path = h.configService.Path()
	}
	return config.ValidationError{Path: path, Issues: []string{issue}}
}

func settingsRecoveryForDeployment(
	configPath string,
	backupPath string,
	deployment settingsDeployment,
	restart settingsRestart,
) settingsRecovery {
	prefix := ""
	if deployment.Scope == daemon.ScopeSystem {
		prefix = "sudo "
	}
	return settingsRecovery{
		ConfigPath:      configPath,
		BackupPath:      backupPath,
		RestoreCommand:  prefix + "cp -- " + settingsShellQuote(backupPath) + " " + settingsShellQuote(configPath),
		ValidateCommand: prefix + "sentinel --config " + settingsShellQuote(configPath) + " config validate --effective",
		RestartCommand:  restart.Command,
		Instruction:     "If the new listener cannot be reached after restart, restore the adjacent backup, validate the effective configuration, then restart the same deployment manually.",
	}
}

func settingsShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
