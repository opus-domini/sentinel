//go:build integration

package security

import (
	"errors"
	"testing"
)

func TestIntegrationRemoteExposureBaseline(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		listenAddr     string
		token          string
		allowedOrigins []string
		wantErr        error
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
			name:       "public bind fails without origins",
			listenAddr: "0.0.0.0:4040",
			token:      "secret",
			wantErr:    ErrRemoteOrigin,
		},
		{
			name:           "public bind passes with complete baseline",
			listenAddr:     "0.0.0.0:4040",
			token:          "secret",
			allowedOrigins: []string{"https://ops.example.com"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateRemoteExposure(tc.listenAddr, tc.token, tc.allowedOrigins)
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
