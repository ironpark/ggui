package macos

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestResolveDevIdentity(t *testing.T) {
	devCert := KeychainIdentity{
		Fingerprint: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Description: "Apple Development: Alice (TEAM123456)",
		Type:        "Apple Development",
	}
	releaseCert := KeychainIdentity{
		Fingerprint: "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
		Description: "Developer ID Application: Example Inc (TEAM123456)",
		Type:        "Developer ID Application",
	}
	selfSigned := KeychainIdentity{
		Fingerprint: "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC",
		Description: "Local Dev Cert",
	}
	list := func(ids ...KeychainIdentity) func(context.Context) ([]KeychainIdentity, error) {
		return func(context.Context) ([]KeychainIdentity, error) { return ids, nil }
	}
	listErr := func(context.Context) ([]KeychainIdentity, error) {
		return nil, errors.New("security unavailable")
	}

	t.Run("flag overrides everything", func(t *testing.T) {
		got, err := resolveDevIdentity(t.Context(), DevSigningRequest{
			FlagIdentity:  "Local Dev Cert",
			FlagSet:       true,
			DevIdentity:   "Apple Development",
			MacOSIdentity: "Developer ID Application",
		}, list(devCert, releaseCert, selfSigned))
		if err != nil {
			t.Fatal(err)
		}
		if got.Target != selfSigned.Fingerprint || got.Source != SourceFlag || got.AdHoc() {
			t.Errorf("got %+v, want selfSigned from flag", got)
		}
	})

	t.Run("flag dash forces ad-hoc", func(t *testing.T) {
		got, err := resolveDevIdentity(t.Context(), DevSigningRequest{
			FlagIdentity:  "-",
			FlagSet:       true,
			DevIdentity:   "Apple Development",
			MacOSIdentity: "Developer ID Application",
		}, list(devCert, releaseCert))
		if err != nil {
			t.Fatal(err)
		}
		if !got.AdHoc() || got.Source != SourceFlag {
			t.Errorf("got %+v, want ad-hoc from flag", got)
		}
	})

	t.Run("flag empty errors", func(t *testing.T) {
		_, err := resolveDevIdentity(t.Context(), DevSigningRequest{FlagIdentity: "", FlagSet: true}, list(devCert))
		if err == nil || !strings.Contains(err.Error(), "--sign") {
			t.Errorf("err = %v, want --sign error", err)
		}
	})

	t.Run("flag missing identity errors", func(t *testing.T) {
		_, err := resolveDevIdentity(t.Context(), DevSigningRequest{FlagIdentity: "Missing", FlagSet: true}, list(devCert))
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("err = %v, want not found error", err)
		}
	})

	t.Run("flag matches fingerprint case-insensitively", func(t *testing.T) {
		got, err := resolveDevIdentity(t.Context(), DevSigningRequest{
			FlagIdentity: strings.ToLower(devCert.Fingerprint),
			FlagSet:      true,
		}, list(releaseCert, devCert))
		if err != nil {
			t.Fatal(err)
		}
		if got.Target != devCert.Fingerprint || got.Display != devCert.Description {
			t.Errorf("got %+v, want devCert", got)
		}
	})

	t.Run("dev.identity overrides macos.signing_identity", func(t *testing.T) {
		got, err := resolveDevIdentity(t.Context(), DevSigningRequest{
			DevIdentity:   "Local Dev Cert",
			MacOSIdentity: "Developer ID Application",
		}, list(devCert, releaseCert, selfSigned))
		if err != nil {
			t.Fatal(err)
		}
		if got.Target != selfSigned.Fingerprint || got.Source != SourceDevConfig {
			t.Errorf("got %+v, want selfSigned from dev.identity", got)
		}
	})

	t.Run("dev.identity dash forces ad-hoc", func(t *testing.T) {
		got, err := resolveDevIdentity(t.Context(), DevSigningRequest{
			DevIdentity:   "-",
			MacOSIdentity: "Developer ID Application",
		}, list(devCert, releaseCert))
		if err != nil {
			t.Fatal(err)
		}
		if !got.AdHoc() || got.Source != SourceDevConfig {
			t.Errorf("got %+v, want ad-hoc from dev.identity", got)
		}
	})

	t.Run("dev.identity missing errors", func(t *testing.T) {
		_, err := resolveDevIdentity(t.Context(), DevSigningRequest{DevIdentity: "Nope"}, list(devCert))
		if err == nil || !strings.Contains(err.Error(), "dev.identity") {
			t.Errorf("err = %v, want dev.identity error", err)
		}
	})

	t.Run("macos.signing_identity used when present", func(t *testing.T) {
		got, err := resolveDevIdentity(t.Context(), DevSigningRequest{
			MacOSIdentity: "Developer ID Application",
		}, list(devCert, releaseCert))
		if err != nil {
			t.Fatal(err)
		}
		if got.Target != releaseCert.Fingerprint || got.Source != SourceMacOSConfig || len(got.Warnings) != 0 {
			t.Errorf("got %+v, want releaseCert from macos.signing_identity", got)
		}
	})

	t.Run("macos.signing_identity missing warns and falls back to Apple Development", func(t *testing.T) {
		got, err := resolveDevIdentity(t.Context(), DevSigningRequest{
			MacOSIdentity: "Developer ID Application: CI Only",
		}, list(devCert))
		if err != nil {
			t.Fatal(err)
		}
		if got.Target != devCert.Fingerprint || got.Source != SourceAutoDetected || len(got.Warnings) != 1 {
			t.Errorf("got %+v, want fallback to Apple Development with warning", got)
		}
	})

	t.Run("auto-detects Apple Development when macos.signing_identity is dash", func(t *testing.T) {
		got, err := resolveDevIdentity(t.Context(), DevSigningRequest{
			MacOSIdentity: "-",
		}, list(releaseCert, devCert))
		if err != nil {
			t.Fatal(err)
		}
		if got.Target != devCert.Fingerprint || got.Source != SourceAutoDetected {
			t.Errorf("got %+v, want auto-detected Apple Development", got)
		}
	})

	t.Run("falls back to ad-hoc when no Apple Development in keychain", func(t *testing.T) {
		got, err := resolveDevIdentity(t.Context(), DevSigningRequest{
			MacOSIdentity: "-",
		}, list(releaseCert))
		if err != nil {
			t.Fatal(err)
		}
		if !got.AdHoc() || got.Source != SourceAdHoc {
			t.Errorf("got %+v, want ad-hoc fallback", got)
		}
	})

	t.Run("falls back to ad-hoc when keychain query fails", func(t *testing.T) {
		got, err := resolveDevIdentity(t.Context(), DevSigningRequest{
			MacOSIdentity: "-",
		}, listErr)
		if err != nil {
			t.Fatal(err)
		}
		if !got.AdHoc() || got.Source != SourceAdHoc {
			t.Errorf("got %+v, want ad-hoc fallback on list error", got)
		}
	})
}
