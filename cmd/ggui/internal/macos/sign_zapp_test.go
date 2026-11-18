//go:build darwin || (cgo && (amd64 || arm64) && (linux || (windows && ggui_rcodesign)))

package macos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/zapp/pkg/signing"
)

// rcodesign signs with the certificate alone and notarizes with an API
// key: what only the keychain or notarytool understand is left out, as
// zapp's Select refuses it away from a Mac.
func TestRcodesignCredentials(t *testing.T) {
	dir := t.TempDir()
	p12 := filepath.Join(dir, "cert.p12")
	pem := filepath.Join(dir, "cert.pem")
	passwordFile := filepath.Join(dir, "password")
	for path, data := range map[string]string{
		p12:          "\x30\x82\x0a\x00 binary PKCS#12",
		pem:          "\n-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n",
		passwordFile: "secret\n",
	} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	notary := Notary{
		Profile: "ggui", AppleID: "dev@example.com", TeamID: "ABCDE12345", Password: "app-specific",
		APIKeyFile: "/keys/api.json", Timeout: 30 * time.Minute, Staple: true,
	}

	tests := []struct {
		name     string
		s        Signing
		identity string
		want     signing.Credentials
	}{{
		name:     "PKCS#12 file",
		s:        Signing{P12File: p12, P12Password: "secret", Notary: notary},
		identity: "Developer ID Application",
		want: signing.Credentials{
			P12File: p12, P12Password: "secret", Entitlements: "app.entitlements",
			APIKeyFile: "/keys/api.json", NotarizeTimeout: 30 * time.Minute,
		},
	}, {
		name:     "PKCS#12 base64 with a password file",
		s:        Signing{P12Base64: "MIIK", P12PasswordFile: passwordFile},
		identity: "",
		want:     signing.Credentials{P12Base64: "MIIK", P12PasswordFile: passwordFile, Entitlements: "app.entitlements"},
	}, {
		name:     "PEM file",
		s:        Signing{P12File: pem, P12Password: "from the environment", Notary: Notary{APIKeyFile: "/keys/api.json"}},
		identity: "",
		want:     signing.Credentials{PEMFile: pem, Entitlements: "app.entitlements", APIKeyFile: "/keys/api.json"},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.s.rcodesignCredentials(tt.identity, "app.entitlements")
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}

	_, err := Signing{Identity: "Developer ID Application", Notary: notary}.rcodesignCredentials("Developer ID Application", "")
	if err == nil || !strings.Contains(err.Error(), "on a Mac only") || !strings.Contains(err.Error(), "--p12-file") {
		t.Errorf("a keychain identity with no certificate: %v", err)
	}
}

// On a Mac a PEM certificate file goes to zapp as one too, which
// repackages it for a temporary keychain.
func TestPEMOrP12(t *testing.T) {
	pem := filepath.Join(t.TempDir(), "cert.p12")
	if err := os.WriteFile(pem, []byte("-----BEGIN CERTIFICATE-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := signing.Credentials{P12File: pem, P12Password: "x"}
	pemOrP12(&c)
	if c.PEMFile != pem || c.P12File != "" || c.P12Password != "" {
		t.Errorf("a PEM file named as PKCS#12: %+v", c)
	}
	c = signing.Credentials{P12File: filepath.Join(t.TempDir(), "missing.p12")}
	pemOrP12(&c)
	if c.PEMFile != "" || c.P12File == "" {
		t.Errorf("a missing file is left to zapp to report: %+v", c)
	}
}
