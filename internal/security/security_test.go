package security

import (
	"crypto/tls"
	"errors"
	"net/http/httptest"
	"testing"
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
			g := New(tt.token, nil)
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
			g := New("", tt.allowedOrigins)
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
			g := New(tt.token, nil)
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
		name       string
		token      string
		queryToken string
		auth       string
		wantErr    error
	}{
		{
			name:  "no token configured",
			token: "",
		},
		{
			name:       "correct token in query",
			token:      "my-token",
			queryToken: "my-token",
		},
		{
			name:       "wrong token in query",
			token:      "my-token",
			queryToken: "wrong",
			wantErr:    ErrUnauthorized,
		},
		{
			name:  "correct token in header",
			token: "my-token",
			auth:  "Bearer my-token",
		},
		{
			name:       "query takes precedence over header",
			token:      "my-token",
			queryToken: "my-token",
			auth:       "Bearer wrong",
		},
		{
			name:    "no token anywhere",
			token:   "my-token",
			wantErr: ErrUnauthorized,
		},
		{
			name:       "wrong query only",
			token:      "my-token",
			queryToken: "wrong",
			wantErr:    ErrUnauthorized,
		},
		{
			name:  "empty query correct header",
			token: "my-token",
			auth:  "Bearer my-token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			url := "http://localhost/"
			if tt.queryToken != "" {
				url = "http://localhost/?token=" + tt.queryToken
			}
			r := httptest.NewRequest("GET", url, nil)
			if tt.auth != "" {
				r.Header.Set("Authorization", tt.auth)
			}
			g := New(tt.token, nil)

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
