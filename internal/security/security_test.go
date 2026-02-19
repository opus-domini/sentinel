package security

import (
	"crypto/tls"
	"encoding/base64"
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := New("", tt.allowedOrigins, CookieSecureAuto)
			r := httptest.NewRequest("GET", "http://"+tt.host+"/", nil)
			r.Host = tt.host
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if tt.tls {
				r.TLS = &tls.ConnectionState{}
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

func TestRequireBearer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		token   string
		auth    string
		wantErr error
	}{
		{
			name:  "no token configured",
			token: "",
			auth:  "",
		},
		{
			name:  "correct token",
			token: "my-token",
			auth:  "Bearer my-token",
		},
		{
			name:    "wrong token",
			token:   "my-token",
			auth:    "Bearer wrong-token",
			wantErr: ErrUnauthorized,
		},
		{
			name:    "no authorization header",
			token:   "my-token",
			auth:    "",
			wantErr: ErrUnauthorized,
		},
		{
			name:    "wrong prefix basic",
			token:   "my-token",
			auth:    "Basic xxx",
			wantErr: ErrUnauthorized,
		},
		{
			name:  "token with extra spaces",
			token: "my-token",
			auth:  "Bearer  my-token ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := New(tt.token, nil, CookieSecureAuto)
			r := httptest.NewRequest("GET", "http://localhost/", nil)
			if tt.auth != "" {
				r.Header.Set("Authorization", tt.auth)
			}

			err := g.RequireBearer(r)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("RequireBearer() error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("RequireBearer() unexpected error: %v", err)
			}
		})
	}
}

func TestRequireWSToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		token    string
		auth     string
		subproto string
		wantErr  error
	}{
		{
			name:  "no token configured",
			token: "",
		},
		{
			name:  "correct token in authorization header",
			token: "my-token",
			auth:  "Bearer my-token",
		},
		{
			name:    "wrong token in authorization header",
			token:   "my-token",
			auth:    "Bearer wrong",
			wantErr: ErrUnauthorized,
		},
		{
			name:     "correct token in websocket subprotocol",
			token:    "my-token",
			subproto: "sentinel.v1, sentinel.auth.bXktdG9rZW4",
		},
		{
			name:     "invalid subprotocol token",
			token:    "my-token",
			subproto: "sentinel.v1, sentinel.auth.d3Jvbmc",
			wantErr:  ErrUnauthorized,
		},
		{
			name:     "authorization takes precedence over subprotocol",
			token:    "my-token",
			auth:     "Bearer my-token",
			subproto: "sentinel.v1, sentinel.auth.d3Jvbmc",
		},
		{
			name:    "no token anywhere",
			token:   "my-token",
			wantErr: ErrUnauthorized,
		},
		{
			name:     "only generic subprotocol no auth token",
			token:    "my-token",
			subproto: "sentinel.v1",
			wantErr:  ErrUnauthorized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest("GET", "http://localhost/", nil)
			if tt.auth != "" {
				r.Header.Set("Authorization", tt.auth)
			}
			if tt.subproto != "" {
				r.Header.Set("Sec-WebSocket-Protocol", tt.subproto)
			}
			g := New(tt.token, nil, CookieSecureAuto)

			err := g.RequireWSToken(r)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("RequireWSToken() error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("RequireWSToken() unexpected error: %v", err)
			}
		})
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
			r := httptest.NewRequest("GET", "http://localhost/", nil)
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

		req := httptest.NewRequest("GET", "http://localhost/", nil)
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
		if c.SameSite != http.SameSiteStrictMode {
			t.Fatalf("cookie SameSite = %v, want %v", c.SameSite, http.SameSiteStrictMode)
		}
	})

	t.Run("set cookie over forwarded https", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest("GET", "http://localhost/", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		rec := httptest.NewRecorder()
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

		req := httptest.NewRequest("GET", "https://localhost/", nil)
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
	})
}

func TestWSSubprotocolToken(t *testing.T) {
	t.Parallel()

	token := "tok.en-123"
	encoded := base64.RawURLEncoding.EncodeToString([]byte(token))
	r := httptest.NewRequest("GET", "http://localhost/", nil)
	r.Header.Set("Sec-WebSocket-Protocol", "sentinel.v1, sentinel.auth."+encoded)
	if got := wsSubprotocolToken(r); got != token {
		t.Fatalf("wsSubprotocolToken() = %q, want %q", got, token)
	}
}

func TestBearerToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		auth string
		want string
	}{
		{"valid bearer", "Bearer my-token", "my-token"},
		{"empty header", "", ""},
		{"wrong scheme", "Basic xyz", ""},
		{"with whitespace", "Bearer  spaced-token  ", "spaced-token"},
		{"missing value", "Bearer ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest("GET", "http://localhost/", nil)
			if tt.auth != "" {
				r.Header.Set("Authorization", tt.auth)
			}
			if got := bearerToken(r); got != tt.want {
				t.Errorf("bearerToken() = %q, want %q", got, tt.want)
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
			name:       "remote with token only is valid",
			listenAddr: "0.0.0.0:4040",
			token:      "secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateRemoteExposure(tt.listenAddr, tt.token)
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
