//go:build darwin || (cgo && (amd64 || arm64) && (linux || (windows && ggui_rcodesign)))

package macos

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/ironpark/zapp/pkg/signing"
)

// rcodesignCredentials translates s for zapp's rcodesign backend, which
// signs away from a Mac: with a certificate and its private key alone,
// since there is no keychain, and notarizing with an App Store Connect
// API key, since there is no notarytool. The certificate holds the one
// identity it signs as, so identity, a keychain's name that ggui.toml
// may give for signing on a Mac, is not passed on; nor are a notarytool
// profile or an Apple ID, which Notary.check refuses here when notarizing.
// It is built on a Mac too, for its tests.
func (s Signing) rcodesignCredentials(identity, entitlements string) (signing.Credentials, error) {
	if !s.HasCertificate() {
		return signing.Credentials{}, errKeychainElsewhere(identity)
	}
	c := signing.Credentials{
		Entitlements: entitlements,
		P12File:      s.P12File, P12Base64: s.P12Base64,
		P12Password: s.P12Password, P12PasswordFile: s.P12PasswordFile,
		APIKeyFile:      s.Notary.APIKeyFile,
		NotarizeTimeout: s.Notary.Timeout,
	}
	pemOrP12(&c)
	return c, nil
}

// errKeychainElsewhere refuses to sign with a keychain identity away
// from a Mac, where there is no keychain.
func errKeychainElsewhere(identity string) error {
	return fmt.Errorf("keychain identities, such as %q, are on a Mac only: away from one ggui signs with a certificate and its private key, "+
		"a PKCS#12 (.p12) or PEM file, given with macos.p12_file or --p12-file, or as base64 text in the variable macos.p12_base64_env names, $GGUI_P12_BASE64 by default", identity)
}

// pemOrP12 passes a certificate file that is PEM, the private key and
// certificate as text, on as one: macos.p12_file and --p12-file take
// either.
func pemOrP12(c *signing.Credentials) {
	if c.P12File != "" && isPEM(c.P12File) {
		c.PEMFile = c.P12File
		c.P12File, c.P12Password, c.P12PasswordFile = "", "", ""
	}
}

// isPEM reports whether the file at path starts as PEM does. A file that
// cannot be read is left to zapp to report.
func isPEM(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	return bytes.HasPrefix(bytes.TrimLeft(head[:n], "\ufeff \t\r\n"), []byte("-----BEGIN "))
}

// notarize submits path to Apple's notary service through b and, if
// staple, staples the ticket.
func notarize(ctx context.Context, b backend, path string, staple bool) error {
	return signing.Notarize(ctx, b.(signing.Backend), path, staple)
}

// closeBackend removes the temporary keychain a certificate was put in,
// or elsewhere the certificate file rcodesign read.
func closeBackend(b backend) error { return signing.Close(b.(signing.Backend)) }
