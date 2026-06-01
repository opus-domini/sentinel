//go:build integration

package security

import (
	"errors"
	"testing"
)

func TestIntegrationRemoteExposureBaseline(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		listenAddr string
		token      string
		origins    []string
		wantErr    error
	}{
		{
			name:       "local bind can run without auth baseline",
			listenAddr: "127.0.0.1:4040",
		},
		{
			name:       "public bind fails without token",
			listenAddr: "0.0.0.0:4040",
			wantErr:    ErrRemoteToken,
		},
		{
			name:       "public bind with token only fails without allowed origin",
			listenAddr: "0.0.0.0:4040",
			token:      "secret",
			wantErr:    ErrRemoteAllowedOrigin,
		},
		{
			name:       "public bind with token and allowed origin is valid",
			listenAddr: "0.0.0.0:4040",
			token:      "secret",
			origins:    []string{"https://example.com"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateRemoteExposure(tc.listenAddr, tc.token, tc.origins)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateRemoteExposure(%q) unexpected error: %v", tc.listenAddr, err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ValidateRemoteExposure(%q) error = %v, want %v", tc.listenAddr, err, tc.wantErr)
			}
		})
	}
}
