//go:build !darwin && cgo && (amd64 || arm64) && (linux || (windows && ggui_rcodesign))

package macos

import "github.com/ironpark/zapp/pkg/signing"

// Toolchain says what this build of ggui signs macOS apps with here.
func Toolchain() (string, error) {
	return "zapp's rcodesign library, linked into this build of ggui", nil
}

// newBackend sets up zapp's rcodesign backend for signing with s's
// certificate; it has no keychain, so identity names none.
func newBackend(s Signing, identity, entitlements string) (backend, error) {
	c, err := s.rcodesignCredentials(identity, entitlements)
	if err != nil {
		return nil, err
	}
	b, err := signing.Select(c)
	if err != nil {
		return nil, err
	}
	return b, nil
}
