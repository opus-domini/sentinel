package security

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrUnauthorized      = errors.New("unauthorized")
	ErrOriginDenied      = errors.New("origin denied")
	ErrRemoteToken       = errors.New("token is required for non-loopback listen address")
	ErrRootNotAllowed    = errors.New("root user is not allowed as a target")
	ErrUserNotAllowlist  = errors.New("user not in allowlist")
	ErrNoSystemUsers     = errors.New("no system users loaded; multi-user switching unavailable")
	ErrUserNotSystemUser = errors.New("user not found in system users")
)

const AuthCookieName = "sentinel_auth"

// CookieSecurePolicy controls the Secure flag on auth cookies.
type CookieSecurePolicy int

const (
	// CookieSecureAuto sets Secure based on per-request TLS detection.
	CookieSecureAuto CookieSecurePolicy = iota
	// CookieSecureAlways forces the Secure flag regardless of transport.
	CookieSecureAlways
	// CookieSecureNever omits the Secure flag regardless of transport.
	CookieSecureNever
)

// ParseCookieSecurePolicy converts a config string to a CookieSecurePolicy.
func ParseCookieSecurePolicy(s string) CookieSecurePolicy {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "always":
		return CookieSecureAlways
	case "never":
		return CookieSecureNever
	default:
		return CookieSecureAuto
	}
}

// MultiUserConfig holds the multi-user session settings consumed by Guard.
type MultiUserConfig struct {
	AllowedUsers    []string
	AllowRootTarget bool
	SystemUsers     []string
}

type Guard struct {
	token          string
	allowedOrigins map[string]struct{}
	cookieSecure   CookieSecurePolicy
	multiUser      MultiUserConfig
}

func New(token string, allowedOrigins []string, cookieSecure CookieSecurePolicy) *Guard {
	return NewWithMultiUser(token, allowedOrigins, cookieSecure, MultiUserConfig{})
}

func NewWithMultiUser(token string, allowedOrigins []string, cookieSecure CookieSecurePolicy, mu MultiUserConfig) *Guard {
	g := &Guard{
		token:          strings.TrimSpace(token),
		allowedOrigins: make(map[string]struct{}),
		cookieSecure:   cookieSecure,
		multiUser:      mu,
	}
	for _, origin := range allowedOrigins {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			continue
		}
		g.allowedOrigins[trimmed] = struct{}{}
	}
	return g
}

// AllowedUsers returns the effective user list for the frontend.
// When an explicit allowlist is configured, it is returned.
// Otherwise, SystemUsers is returned (filtered by AllowRootTarget).
func (g *Guard) AllowedUsers() []string {
	if g == nil {
		return nil
	}
	if len(g.multiUser.AllowedUsers) > 0 {
		return g.multiUser.AllowedUsers
	}
	if len(g.multiUser.SystemUsers) == 0 {
		return nil
	}
	if g.multiUser.AllowRootTarget {
		return g.multiUser.SystemUsers
	}
	filtered := make([]string, 0, len(g.multiUser.SystemUsers))
	for _, u := range g.multiUser.SystemUsers {
		if u != "root" {
			filtered = append(filtered, u)
		}
	}
	return filtered
}

// SystemUsers returns the in-memory system user list loaded at startup.
func (g *Guard) SystemUsers() []string {
	if g == nil {
		return nil
	}
	return g.multiUser.SystemUsers
}

// ValidateTargetUser checks whether targetUser is a permitted multi-user
// session target. Returns nil when targetUser is empty (use default user).
func (g *Guard) ValidateTargetUser(targetUser string) error {
	if g == nil {
		return ErrRootNotAllowed
	}
	targetUser = strings.TrimSpace(targetUser)
	if targetUser == "" {
		return nil
	}
	if targetUser == "root" && !g.multiUser.AllowRootTarget {
		return ErrRootNotAllowed
	}

	// When SystemUsers is empty, we cannot verify users -- block switching.
	if len(g.multiUser.SystemUsers) == 0 {
		return ErrNoSystemUsers
	}

	// When AllowedUsers is set, validate against the allowlist.
	if len(g.multiUser.AllowedUsers) > 0 {
		for _, allowed := range g.multiUser.AllowedUsers {
			if allowed == targetUser {
				return nil
			}
		}
		return fmt.Errorf("%w: %s", ErrUserNotAllowlist, targetUser)
	}

	// No allowlist: validate against system users.
	for _, su := range g.multiUser.SystemUsers {
		if su == targetUser {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrUserNotSystemUser, targetUser)
}

func (g *Guard) TokenRequired() bool {
	return g.token != ""
}

func (g *Guard) CheckOrigin(r *http.Request) error {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return nil
	}
	if _, ok := g.allowedOrigins[origin]; ok {
		return nil
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return fmt.Errorf("%w: invalid origin", ErrOriginDenied)
	}

	scheme := "http"
	if requestUsesTLS(r) {
		scheme = "https"
	}
	if parsed.Scheme != scheme || parsed.Host != r.Host {
		return fmt.Errorf("%w: expected %s://%s, got %s", ErrOriginDenied, scheme, r.Host, origin)
	}
	return nil
}

func (g *Guard) RequireAuth(r *http.Request) error {
	if !g.TokenMatches(cookieToken(r)) {
		return ErrUnauthorized
	}
	return nil
}

func (g *Guard) SetAuthCookie(w http.ResponseWriter, r *http.Request) {
	if !g.TokenRequired() {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     AuthCookieName,
		Value:    encodeBase64URL(g.token),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   g.resolveSecure(r),
	})
}

func (g *Guard) ClearAuthCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     AuthCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   g.resolveSecure(r),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
	})
}

func (g *Guard) resolveSecure(r *http.Request) bool {
	switch g.cookieSecure {
	case CookieSecureAlways:
		return true
	case CookieSecureNever:
		return false
	case CookieSecureAuto:
		return requestUsesTLS(r)
	}
	return requestUsesTLS(r)
}

func (g *Guard) TokenMatches(token string) bool {
	if !g.TokenRequired() {
		return true
	}
	token = strings.TrimSpace(token)
	return token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(g.token)) == 1
}

func cookieToken(r *http.Request) string {
	cookie, err := r.Cookie(AuthCookieName)
	if err != nil {
		return ""
	}
	decoded, err := decodeBase64URL(strings.TrimSpace(cookie.Value))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(decoded)
}

func decodeBase64URL(s string) (string, error) {
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	decoded, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func encodeBase64URL(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

func requestUsesTLS(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

// ValidateRemoteExposure enforces the minimum security baseline when Sentinel is
// configured to listen on a non-loopback address.
func ValidateRemoteExposure(listenAddr, token string) error {
	if !ExposesBeyondLoopback(listenAddr) {
		return nil
	}
	if strings.TrimSpace(token) == "" {
		return ErrRemoteToken
	}
	return nil
}

// HasAllowedOrigins reports whether at least one non-empty origin is configured.
func HasAllowedOrigins(origins []string) bool {
	for _, origin := range origins {
		if strings.TrimSpace(origin) != "" {
			return true
		}
	}
	return false
}

// ExposesBeyondLoopback reports whether listenAddr is reachable from outside the host.
func ExposesBeyondLoopback(listenAddr string) bool {
	host := listenHost(listenAddr)
	if host == "" {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return false
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return !ip.IsLoopback()
	}
	// Any named host other than localhost may resolve to a routable address.
	return true
}

func listenHost(listenAddr string) string {
	addr := strings.TrimSpace(listenAddr)
	if addr == "" {
		return ""
	}
	if strings.HasPrefix(addr, ":") {
		return ""
	}
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return strings.Trim(strings.TrimSpace(host), "[]")
	}
	// Best effort fallback for host-only values.
	return strings.Trim(addr, "[]")
}
