package main

import (
	"testing"

	"github.com/abhinavxd/libredesk/internal/setting/models"
)

func TestValidatePortalSettings(t *testing.T) {
	tests := []struct {
		name        string
		req         models.Portal
		wantErr     bool
		wantTTL     string
		wantSession string
		wantSecret  string
		wantRoot    string
	}{
		{
			name:        "empty lifetimes fall back to defaults",
			req:         models.Portal{},
			wantTTL:     "15m",
			wantSession: "720h",
		},
		{
			name:        "trims whitespace around lifetimes and secret",
			req:         models.Portal{MagicLinkTTL: " 20m ", SessionLifetime: " 48h ", JWTSecret: "  secret  "},
			wantTTL:     "20m",
			wantSession: "48h",
			wantSecret:  "secret",
		},
		{
			name:        "trims a trailing slash from the portal root URL",
			req:         models.Portal{RootURL: " https://support.example.com/ "},
			wantTTL:     "15m",
			wantSession: "720h",
			wantRoot:    "https://support.example.com",
		},
		{
			name:    "rejects a portal root URL without a scheme",
			req:     models.Portal{RootURL: "support.example.com"},
			wantErr: true,
		},
		{
			name:    "rejects a non-http portal root URL",
			req:     models.Portal{RootURL: "ftp://support.example.com"},
			wantErr: true,
		},
		{
			name:    "rejects a sign-in link lifetime below one minute",
			req:     models.Portal{MagicLinkTTL: "0s"},
			wantErr: true,
		},
		{
			name:    "rejects an invalid session lifetime",
			req:     models.Portal{SessionLifetime: "forever"},
			wantErr: true,
		},
		{
			name:    "rejects a session lifetime below one hour",
			req:     models.Portal{SessionLifetime: "30m"},
			wantErr: true,
		},
		{
			name:    "rejects a weak JWT secret when JWT sign-in is enabled",
			req:     models.Portal{JWTEnabled: true, JWTSecret: "short"},
			wantErr: true,
		},
		{
			name: "accepts a strong JWT secret when JWT sign-in is enabled",
			req: models.Portal{
				JWTEnabled: true,
				JWTSecret:  "0123456789abcdef0123456789abcdef",
			},
			wantTTL:     "15m",
			wantSession: "720h",
		},
		{
			name:        "allows a short secret when JWT sign-in is disabled",
			req:         models.Portal{JWTSecret: "short"},
			wantTTL:     "15m",
			wantSession: "720h",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := newValidatorTestApp(t)
			err := validatePortalSettings(app, &tc.req)
			assertValidation(t, err, tc.wantErr)
			if tc.wantErr {
				return
			}
			if tc.req.MagicLinkTTL != tc.wantTTL {
				t.Fatalf("magic link ttl = %q, want %q", tc.req.MagicLinkTTL, tc.wantTTL)
			}
			if tc.req.SessionLifetime != tc.wantSession {
				t.Fatalf("session lifetime = %q, want %q", tc.req.SessionLifetime, tc.wantSession)
			}
			if tc.wantSecret != "" && tc.req.JWTSecret != tc.wantSecret {
				t.Fatalf("jwt secret = %q, want %q", tc.req.JWTSecret, tc.wantSecret)
			}
			if tc.wantRoot != "" && tc.req.RootURL != tc.wantRoot {
				t.Fatalf("root url = %q, want %q", tc.req.RootURL, tc.wantRoot)
			}
		})
	}
}
