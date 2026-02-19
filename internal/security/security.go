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
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrOriginDenied = errors.New("origin denied")
	ErrRemoteToken  = errors.New("token is required for non-loopback listen address")
	ErrRemoteOrigin = errors.New("allowed origins are required for non-loopback listen address")
)

type Guard struct {
	token          string
	allowedOrigins map[string]struct{}
}

func New(token string, allowedOrigins []string) *Guard {
	g := &Guard{
		token:          token,
		allowedOrigins: make(map[string]struct{}),
	}
	for _, origin := range allowedOrigins {
		g.allowedOrigins[origin] = struct{}{}
	}
	return g
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
	if r.TLS != nil {
		scheme = "https"
	}
	if parsed.Scheme != scheme || parsed.Host != r.Host {
		return fmt.Errorf("%w: expected %s://%s, got %s", ErrOriginDenied, scheme, r.Host, origin)
	}
	return nil
}

func (g *Guard) RequireBearer(r *http.Request) error {
	if !g.TokenRequired() {
		return nil
	}
	token := bearerToken(r)
	if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(g.token)) != 1 {
		return ErrUnauthorized
	}
	return nil
}

func (g *Guard) RequireWSToken(r *http.Request) error {
	if !g.TokenRequired() {
		return nil
	}
	token := bearerToken(r)
	if token == "" {
		token = wsSubprotocolToken(r)
	}
	if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(g.token)) != 1 {
		return ErrUnauthorized
	}
	return nil
}

func bearerToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" {
		return ""
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, prefix))
}

func wsSubprotocolToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Protocol"))
	if header == "" {
		return ""
	}
	for _, raw := range strings.Split(header, ",") {
		proto := strings.TrimSpace(raw)
		const prefix = "sentinel.auth."
		if !strings.HasPrefix(proto, prefix) {
			continue
		}
		encoded := strings.TrimSpace(strings.TrimPrefix(proto, prefix))
		if encoded == "" {
			continue
		}
		decoded, err := decodeBase64URL(encoded)
		if err != nil || strings.TrimSpace(decoded) == "" {
			continue
		}
		return decoded
	}
	return ""
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

// ValidateRemoteExposure enforces the minimum security baseline when Sentinel is
// configured to listen on a non-loopback address.
func ValidateRemoteExposure(listenAddr, token string, allowedOrigins []string) error {
	if !exposesBeyondLoopback(listenAddr) {
		return nil
	}

	var issues []error
	if strings.TrimSpace(token) == "" {
		issues = append(issues, ErrRemoteToken)
	}
	if !hasNonEmptyOrigin(allowedOrigins) {
		issues = append(issues, ErrRemoteOrigin)
	}
	if len(issues) == 0 {
		return nil
	}
	return errors.Join(issues...)
}

func hasNonEmptyOrigin(origins []string) bool {
	for _, origin := range origins {
		if strings.TrimSpace(origin) != "" {
			return true
		}
	}
	return false
}

func exposesBeyondLoopback(listenAddr string) bool {
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
