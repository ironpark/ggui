package doctor

import (
	"context"
	"os"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/macos"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
)

// checkSigning reports what a macOS build can be signed and notarized
// with here, and whether what ggui.toml asks for is at hand. proj is nil
// when ggui.toml is invalid, which is reported already.
func checkSigning(ctx context.Context, r *Report, proj *project.Project) {
	m := config.Defaults().MacOS
	if proj != nil {
		m = proj.Config.MacOS
	}
	if !hostIsDarwin() {
		_, cannot := macos.Toolchain()
		checkSigningElsewhere(r, m, proj, cannot)
		return
	}
	checkIdentities(ctx, r, m)
	checkCertificate(r, m, proj)
	checkNotarization(ctx, r, m, proj)
}

// checkSigningElsewhere reports whether this build of ggui signs macOS
// apps away from a Mac, which only one that links zapp's rcodesign
// library does; cannot says why it does not, nil when it does. Then it
// checks that what ggui.toml asks for can be done here: a certificate,
// not a keychain identity, and an App Store Connect API key to notarize.
func checkSigningElsewhere(r *Report, m config.MacOS, proj *project.Project, cannot error) {
	wantsSignature := m.SigningIdentity != "-" || m.Notarize || hasCertificate(m)
	if cannot != nil {
		detail := "bundles, disk images and installers are built unsigned here: " + cannot.Error()
		if wantsSignature {
			r.fail("macOS signing", detail+"; and ggui.toml asks for a signature", "")
			return
		}
		r.ok("macOS signing", detail)
		return
	}
	r.ok("macOS signing", "this build of ggui signs and notarizes with zapp's rcodesign library: with a PKCS#12 or PEM certificate, and an App Store Connect API key; "+
		"keychain identities, notarytool profiles, Apple IDs and signing installers are on a Mac only")
	checkCertificate(r, m, proj)
	if m.SigningIdentity != "-" && !hasCertificate(m) {
		r.fail("signing identity", "macos.signing_identity names a keychain identity, which is on a Mac only",
			"sign with a certificate exported with its private key: macos.p12_file, or base64 text in $"+m.P12Base64Env)
	}

	n := m.Notary
	switch {
	case n.APIKeyFile != "":
		path := n.APIKeyFile
		if proj != nil {
			path = proj.Path(path)
		}
		if _, err := os.Stat(path); err != nil {
			r.fail("notarization", "macos.notary.api_key_file: "+err.Error(), "rcodesign encode-app-store-connect-api-key writes the key's JSON")
			return
		}
		r.ok("notarization", "the App Store Connect API key "+path)
	case m.Notarize:
		r.fail("notarization", "macos.notarize is on, and away from a Mac notarizing takes an App Store Connect API key",
			"set macos.notary.api_key_file to the JSON rcodesign encode-app-store-connect-api-key writes, or give --api-key-file")
	}
}

// hasCertificate reports whether ggui.toml or the environment gives a
// certificate to sign with.
func hasCertificate(m config.MacOS) bool {
	return m.P12File != "" || (m.P12Base64Env != "" && os.Getenv(m.P12Base64Env) != "")
}

// checkCertificate reports a certificate ggui.toml or the environment
// gives for signing in place of the keychain.
func checkCertificate(r *Report, m config.MacOS, proj *project.Project) {
	var from string
	switch {
	case m.P12File != "":
		path := m.P12File
		if proj != nil {
			path = proj.Path(path)
		}
		if _, err := os.Stat(path); err != nil {
			r.fail("certificate", "macos.p12_file: "+err.Error(), "export the Developer ID certificate with its private key as .p12, or remove the key")
			return
		}
		from = "macos.p12_file " + path
	case m.P12Base64Env != "" && os.Getenv(m.P12Base64Env) != "":
		from = "$" + m.P12Base64Env
	default:
		return
	}
	detail := "signs with the certificate in " + from + " instead of the keychain"
	if m.P12PasswordFile == "" && (m.P12PasswordEnv == "" || os.Getenv(m.P12PasswordEnv) == "") {
		r.warn("certificate", detail+", with no password", "set $"+m.P12PasswordEnv+" or macos.p12_password_file if the certificate has one")
		return
	}
	r.ok("certificate", detail)
}
