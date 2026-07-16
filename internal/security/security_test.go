package security

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTokenRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{"token set", "secret", true},
		{"token empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := New(tt.token, nil, CookieSecureAuto)
			if got := g.TokenRequired(); got != tt.want {
				t.Errorf("TokenRequired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		allowedOrigins []string
		origin         string
		host           string
		tls            bool
		forwarded      string
		trustedProxies []string
		remoteAddr     string
		wantErr        error
	}{
		{
			name:   "no origin header",
			origin: "",
			host:   "localhost:4040",
		},
		{
			name:           "origin in allowed list",
			allowedOrigins: []string{"http://trusted.example.com"},
			origin:         "http://trusted.example.com",
			host:           "localhost:4040",
		},
		{
			name:   "same origin http",
			origin: "http://localhost:4040",
			host:   "localhost:4040",
		},
		{
			name:    "different host",
			origin:  "http://evil.example.com",
			host:    "localhost:4040",
			wantErr: ErrOriginDenied,
		},
		{
			name:    "different scheme https origin http request",
			origin:  "https://localhost:4040",
			host:    "localhost:4040",
			wantErr: ErrOriginDenied,
		},
		{
			name:    "invalid url as origin",
			origin:  "://bad",
			host:    "localhost:4040",
			wantErr: ErrOriginDenied,
		},
		{
			name:   "tls request with https origin",
			origin: "https://localhost:4040",
			host:   "localhost:4040",
			tls:    true,
		},
		{
			name:   "empty allowed origins matching same origin",
			origin: "http://myhost:8080",
			host:   "myhost:8080",
		},
		{
			name:           "reverse proxy forwarded https",
			origin:         "https://myhost.ts.net",
			host:           "myhost.ts.net",
			forwarded:      "https",
			trustedProxies: []string{"192.0.2.10"},
			remoteAddr:     "192.0.2.10:1234",
		},
		{
			name:       "untrusted forwarded https ignored",
			origin:     "https://myhost.ts.net",
			host:       "myhost.ts.net",
			forwarded:  "https",
			remoteAddr: "192.0.2.11:1234",
			wantErr:    ErrUntrustedProxy,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithOptions("", tt.allowedOrigins, CookieSecureAuto, MultiUserConfig{}, tt.trustedProxies)
			r := httptest.NewRequest(http.MethodGet, "http://"+tt.host+"/", nil)
			r.Host = tt.host
			if tt.remoteAddr != "" {
				r.RemoteAddr = tt.remoteAddr
			}
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if tt.tls {
				r.TLS = &tls.ConnectionState{}
			}
			if tt.forwarded != "" {
				r.Header.Set("X-Forwarded-Proto", tt.forwarded)
			}

			err := g.CheckOrigin(r)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("CheckOrigin() error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("CheckOrigin() unexpected error: %v", err)
			}
		})
	}
}

func TestCheckOriginDescribesUntrustedHTTPSProxy(t *testing.T) {
	t.Parallel()

	guard := NewWithOptions("", nil, CookieSecureAuto, MultiUserConfig{}, nil)
	req := httptest.NewRequest(http.MethodPost, "http://sentinel.example/api/connection/check", nil)
	req.Host = "sentinel.example"
	req.RemoteAddr = "127.0.0.1:43210"
	req.Header.Set("Origin", "https://sentinel.example")
	req.Header.Set("X-Forwarded-Proto", "https")

	err := guard.CheckOrigin(req)
	if !errors.Is(err, ErrUntrustedProxy) {
		t.Fatalf("CheckOrigin() error = %v, want ErrUntrustedProxy", err)
	}
	var originErr *OriginError
	if !errors.As(err, &originErr) {
		t.Fatalf("CheckOrigin() error type = %T, want *OriginError", err)
	}
	if originErr.Origin != "https://sentinel.example" || originErr.Proxy != "127.0.0.1" {
		t.Fatalf("origin error = %+v", originErr)
	}
	if originErr.Message != `HTTPS proxy "127.0.0.1" is not trusted; add it to server.trusted_proxies` {
		t.Fatalf("message = %q", originErr.Message)
	}
}

func TestRequireAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		token     string
		cookie    string
		rawCookie string
		forwarded string
		wantErr   error
	}{
		{
			name:  "no token configured",
			token: "",
		},
		{
			name:   "valid auth cookie",
			token:  "my-token",
			cookie: "my-token",
		},
		{
			name:    "wrong auth cookie value",
			token:   "my-token",
			cookie:  "wrong",
			wantErr: ErrUnauthorized,
		},
		{
			name:      "invalid cookie encoding",
			token:     "my-token",
			rawCookie: "%%%not-base64%%%",
			wantErr:   ErrUnauthorized,
		},
		{
			name:    "missing cookie",
			token:   "my-token",
			wantErr: ErrUnauthorized,
		},
		{
			name:   "cookie with whitespace after decode",
			token:  "my-token",
			cookie: "  my-token  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := New(tt.token, nil, CookieSecureAuto)
			r := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
			if tt.forwarded != "" {
				r.Header.Set("X-Forwarded-Proto", tt.forwarded)
			}
			if tt.cookie != "" {
				r.AddCookie(&http.Cookie{
					Name:  AuthCookieName,
					Value: encodeBase64URL(tt.cookie),
				})
			}
			if tt.rawCookie != "" {
				r.AddCookie(&http.Cookie{
					Name:  AuthCookieName,
					Value: tt.rawCookie,
				})
			}

			err := g.RequireAuth(r)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("RequireAuth() unexpected error = %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("RequireAuth() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAuthCookieLifecycle(t *testing.T) {
	t.Parallel()

	g := New("secret-token", nil, CookieSecureAuto)

	t.Run("set cookie over http", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
		rec := httptest.NewRecorder()
		g.SetAuthCookie(rec, req)

		res := rec.Result()
		defer func() { _ = res.Body.Close() }()

		cookies := res.Cookies()
		if len(cookies) != 1 {
			t.Fatalf("cookies len = %d, want 1", len(cookies))
		}
		c := cookies[0]
		if c.Name != AuthCookieName {
			t.Fatalf("cookie name = %q, want %q", c.Name, AuthCookieName)
		}
		if c.Value != encodeBase64URL("secret-token") {
			t.Fatalf("cookie value = %q, want encoded token", c.Value)
		}
		if c.Path != "/" {
			t.Fatalf("cookie path = %q, want /", c.Path)
		}
		if !c.HttpOnly {
			t.Fatal("cookie HttpOnly = false, want true")
		}
		if c.Secure {
			t.Fatal("cookie Secure = true, want false on plain http")
		}
		if c.SameSite != http.SameSiteLaxMode {
			t.Fatalf("cookie SameSite = %v, want %v", c.SameSite, http.SameSiteLaxMode)
		}
		wantMaxAge := 30 * 24 * 60 * 60
		if c.MaxAge != wantMaxAge {
			t.Fatalf("cookie MaxAge = %d, want %d", c.MaxAge, wantMaxAge)
		}
	})

	t.Run("set cookie over forwarded https", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		req.RemoteAddr = "192.0.2.10:1234"
		rec := httptest.NewRecorder()
		g := NewWithOptions("secret-token", nil, CookieSecureAuto, MultiUserConfig{}, []string{"192.0.2.10"})
		g.SetAuthCookie(rec, req)

		res := rec.Result()
		defer func() { _ = res.Body.Close() }()

		cookies := res.Cookies()
		if len(cookies) != 1 {
			t.Fatalf("cookies len = %d, want 1", len(cookies))
		}
		if !cookies[0].Secure {
			t.Fatal("cookie Secure = false, want true for https proxy")
		}
	})

	t.Run("clear cookie", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "https://localhost/", nil)
		req.TLS = &tls.ConnectionState{}
		rec := httptest.NewRecorder()
		g.ClearAuthCookie(rec, req)

		res := rec.Result()
		defer func() { _ = res.Body.Close() }()

		cookies := res.Cookies()
		if len(cookies) != 1 {
			t.Fatalf("cookies len = %d, want 1", len(cookies))
		}
		c := cookies[0]
		if c.Name != AuthCookieName {
			t.Fatalf("cookie name = %q, want %q", c.Name, AuthCookieName)
		}
		if c.Value != "" {
			t.Fatalf("cookie value = %q, want empty", c.Value)
		}
		if c.MaxAge >= 0 {
			t.Fatalf("cookie MaxAge = %d, want negative", c.MaxAge)
		}
		if c.Expires.After(time.Now().UTC()) {
			t.Fatalf("cookie Expires = %s, want in the past", c.Expires)
		}
		if !c.Secure {
			t.Fatal("cookie Secure = false, want true for tls request")
		}
		if c.SameSite != http.SameSiteLaxMode {
			t.Fatalf("cookie SameSite = %v, want %v", c.SameSite, http.SameSiteLaxMode)
		}
	})
}

func TestCookieSecurePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		policy     CookieSecurePolicy
		tls        bool
		forwarded  string
		trusted    bool
		wantSecure bool
	}{
		{"always over http", CookieSecureAlways, false, "", false, true},
		{"always over https", CookieSecureAlways, true, "", false, true},
		{"never over http", CookieSecureNever, false, "", false, false},
		{"never over https", CookieSecureNever, true, "", false, false},
		{"never over forwarded https", CookieSecureNever, false, "https", true, false},
		{"auto over http", CookieSecureAuto, false, "", false, false},
		{"auto over https", CookieSecureAuto, true, "", false, true},
		{"auto over forwarded https", CookieSecureAuto, false, "https", true, true},
		{"auto ignores untrusted forwarded https", CookieSecureAuto, false, "https", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			trusted := []string(nil)
			if tt.trusted {
				trusted = []string{"192.0.2.10"}
			}
			g := NewWithOptions("secret", nil, tt.policy, MultiUserConfig{}, trusted)

			req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
			req.RemoteAddr = "192.0.2.10:1234"
			if tt.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if tt.forwarded != "" {
				req.Header.Set("X-Forwarded-Proto", tt.forwarded)
			}
			rec := httptest.NewRecorder()
			g.SetAuthCookie(rec, req)

			res := rec.Result()
			defer func() { _ = res.Body.Close() }()

			cookies := res.Cookies()
			if len(cookies) != 1 {
				t.Fatalf("cookies len = %d, want 1", len(cookies))
			}
			if cookies[0].Secure != tt.wantSecure {
				t.Fatalf("cookie Secure = %v, want %v", cookies[0].Secure, tt.wantSecure)
			}
		})
	}
}

func TestParseCookieSecurePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  CookieSecurePolicy
	}{
		{"auto", CookieSecureAuto},
		{"always", CookieSecureAlways},
		{"never", CookieSecureNever},
		{"ALWAYS", CookieSecureAlways},
		{"  Never  ", CookieSecureNever},
		{"", CookieSecureAuto},
		{"invalid", CookieSecureAuto},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			if got := ParseCookieSecurePolicy(tt.input); got != tt.want {
				t.Errorf("ParseCookieSecurePolicy(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateRemoteExposure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		listenAddr string
		token      string
		origins    []string
		wantErr    error
	}{
		{
			name:       "localhost without token is allowed",
			listenAddr: "127.0.0.1:4040",
			token:      "",
		},
		{
			name:       "localhost hostname is allowed",
			listenAddr: "localhost:4040",
			token:      "",
		},
		{
			name:       "all interfaces requires token",
			listenAddr: ":4040",
			token:      "",
			wantErr:    ErrRemoteToken,
		},
		{
			name:       "remote ip requires token",
			listenAddr: "192.168.1.12:4040",
			token:      "",
			wantErr:    ErrRemoteToken,
		},
		{
			name:       "remote with token requires allowed origin",
			listenAddr: "0.0.0.0:4040",
			token:      "secret",
			wantErr:    ErrRemoteAllowedOrigin,
		},
		{
			name:       "remote with token and allowed origin is valid",
			listenAddr: "0.0.0.0:4040",
			token:      "secret",
			origins:    []string{"https://example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateRemoteExposure(tt.listenAddr, tt.token, tt.origins)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateRemoteExposure() unexpected error = %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateRemoteExposure() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRemoteExposureRequiresAllowedOriginWhenProvided(t *testing.T) {
	t.Parallel()
	if err := ValidateRemoteExposure("0.0.0.0:4040", "secret", nil); !errors.Is(err, ErrRemoteAllowedOrigin) {
		t.Fatalf("ValidateRemoteExposure() error = %v, want %v", err, ErrRemoteAllowedOrigin)
	}
	if err := ValidateRemoteExposure("0.0.0.0:4040", "secret", []string{"https://example.com"}); err != nil {
		t.Fatalf("ValidateRemoteExposure() unexpected error = %v", err)
	}
}

func TestNilGuardFailsClosed(t *testing.T) {
	t.Parallel()

	var g *Guard // nil receiver: every auth method must deny / not panic
	if g.TokenRequired() {
		t.Fatal("nil guard TokenRequired() = true, want false")
	}
	if g.TokenMatches("anything") {
		t.Fatal("nil guard TokenMatches() = true, want false (fail closed)")
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := g.RequireAuth(req); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("nil guard RequireAuth() = %v, want ErrUnauthorized", err)
	}
	req.Header.Set("Origin", "https://evil.example")
	if err := g.CheckOrigin(req); !errors.Is(err, ErrOriginDenied) {
		t.Fatalf("nil guard CheckOrigin() = %v, want ErrOriginDenied", err)
	}
}
