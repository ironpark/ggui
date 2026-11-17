package macos

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// backend signs through one toolchain. On macOS it is one of zapp's
// signing backends, which drive Apple's tools; elsewhere there is none,
// since zapp signs there with a Rust library that only a cgo build links,
// and linking it would ask every Linux or Windows build of ggui with a C
// compiler to link 30 MB it does not use, which on Windows fails outright
// with MinGW's GCC. ggui therefore signs with an identity on a Mac only.
type backend interface {
	Describe(ctx context.Context, path string) (string, error)
	Sign(ctx context.Context, path string) error
}

// Signing says how to sign, gathered from ggui.toml, the flags and the
// environment.
type Signing struct {
	// Identity names a keychain identity: all or part of its name, such
	// as "Developer ID Application", or its SHA-1 hash. "-" signs ad hoc,
	// or, with a certificate, picks its Developer ID Application identity.
	Identity     string
	Entitlements string

	// A PKCS#12 certificate and key to sign with instead of the keychain:
	// a file, or base64 text. P12Source says where it came from, a path
	// or an environment variable, for messages.
	P12File, P12Base64, P12Source string
	P12Password, P12PasswordFile  string

	// PKGIdentity signs the installer package; "" is the Developer ID
	// Installer identity.
	PKGIdentity string

	Notary Notary
}

// Notary is how to reach Apple's notary service.
type Notary struct {
	Profile    string
	APIKeyFile string
	AppleID    string
	TeamID     string
	Password   string
	// PasswordVar is the environment variable the password is read from,
	// for messages.
	PasswordVar string
	Staple      bool
	Timeout     time.Duration
}

// HasCertificate reports whether s signs with a certificate rather than
// the keychain.
func (s Signing) HasCertificate() bool { return s.P12File != "" || s.P12Base64 != "" }

// AdHoc reports whether s signs ad hoc: for running on this Mac only.
func (s Signing) AdHoc() bool { return s.Identity == "-" && !s.HasCertificate() }

// Describe says what s signs with, safe to print.
func (s Signing) Describe() string {
	switch {
	case s.HasCertificate():
		return "the certificate " + s.P12Source
	case s.AdHoc():
		return "ad hoc"
	}
	return fmt.Sprintf("the keychain identity %q", s.Identity)
}

// Describe says which credentials notarize, safe to print.
func (n Notary) Describe() string {
	switch {
	case n.Profile != "":
		return fmt.Sprintf("the notarytool profile %q", n.Profile)
	case n.APIKeyFile != "":
		return "the App Store Connect API key " + n.APIKeyFile
	case n.AppleID != "":
		return fmt.Sprintf("the Apple ID %s of team %s", n.AppleID, n.TeamID)
	}
	return "no credentials"
}

// check reports credentials that cannot notarize, before anything is
// built, saying what to give instead.
func (n Notary) check() error {
	switch {
	case n.Profile != "" || n.APIKeyFile != "":
		return nil
	case n.AppleID != "":
		if n.TeamID == "" {
			return errors.New("notarizing with an Apple ID needs its team ID too: macos.notary.team_id or --team-id")
		}
		if n.Password == "" {
			return fmt.Errorf("notarizing with the Apple ID %s needs its app-specific password in $%s; make one at https://account.apple.com under Sign-In and Security", n.AppleID, n.PasswordVar)
		}
		return nil
	}
	return errors.New("notarizing needs credentials: a notarytool keychain profile (xcrun notarytool store-credentials NAME, then macos.notary.profile or --notary-profile), " +
		"an App Store Connect API key (macos.notary.api_key_file or --api-key-file), or an Apple ID with macos.notary.apple_id, team_id and its app-specific password in $" + n.PasswordVar)
}

// Signer signs, notarizes and staples what a build makes. Make one with
// NewSigner before building, so that credentials that cannot work fail
// the build before it starts, and Close it after.
type Signer struct {
	s   Signing
	app backend // nil when signing ad hoc
	pkg backend // made when an installer is first signed
	// Identity is the identity the app is signed with, masked for
	// printing, once Check has found it.
	Identity string
}

// NewSigner checks s and, when it signs with an identity, sets up zapp's
// backend for it, which needs a Mac.
func NewSigner(s Signing, notarize bool) (*Signer, error) {
	sg := &Signer{s: s}
	if s.AdHoc() {
		if notarize {
			return nil, errors.New("notarization needs a Developer ID signature, not an ad hoc one: set macos.signing_identity or --identity to your " +
				`"Developer ID Application" identity, or sign with a certificate: macos.p12_file, --p12-file or $GGUI_P12_BASE64`)
		}
		return sg, nil
	}
	if notarize {
		if err := s.Notary.check(); err != nil {
			return nil, err
		}
	}
	var err error
	if sg.app, err = newBackend(s, s.Identity, s.Entitlements); err != nil {
		return nil, fmt.Errorf("signing with %s: %w", s.Describe(), err)
	}
	return sg, nil
}

// Check finds the signing identity, importing the certificate when there
// is one, so a build does not run only to fail at signing. It sets
// Identity.
func (sg *Signer) Check(ctx context.Context) error {
	if sg.app == nil {
		return nil
	}
	id, err := sg.app.Describe(ctx, "App.app")
	if err != nil {
		return sg.explain(err)
	}
	sg.Identity = id
	return nil
}

// explain adds what to do to zapp's errors about identities.
func (sg *Signer) explain(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "no identity matching") || strings.Contains(msg, "holds no signing identities"):
		return fmt.Errorf("%w; security find-identity -v -p codesigning lists this Mac's identities, and a Developer ID Application certificate comes from https://developer.apple.com/account/resources/certificates", err)
	case strings.Contains(msg, "holds no valid signing identity"):
		return fmt.Errorf("signing with %s: %w; the certificate must be a Developer ID Application certificate from Apple, exported with its private key", sg.s.Describe(), err)
	}
	return fmt.Errorf("signing with %s: %w", sg.s.Describe(), err)
}

// AdHoc reports whether the signer signs ad hoc.
func (sg *Signer) AdHoc() bool { return sg.app == nil }

// Sign signs the app, disk image or installer at path. It reports false
// when it left path unsigned: ad hoc signing away from macOS, where
// codesign is not, or an installer when the app is signed ad hoc, since
// an installer has no ad hoc signature.
func (sg *Signer) Sign(ctx context.Context, path string) (bool, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if sg.app == nil {
		if runtime.GOOS != "darwin" || ext == ".pkg" {
			return false, nil
		}
		return true, signAdHoc(ctx, path, sg.s.Entitlements)
	}
	b := sg.app
	if ext == ".pkg" {
		var err error
		if b, err = sg.installer(); err != nil {
			return false, err
		}
	}
	if err := b.Sign(ctx, path); err != nil {
		if ext == ".pkg" {
			return false, explainInstaller(err)
		}
		return false, sg.explain(err)
	}
	return true, nil
}

// installer returns the backend that signs installer packages, which
// take an identity of their own.
func (sg *Signer) installer() (backend, error) {
	if sg.pkg == nil {
		var err error
		if sg.pkg, err = newBackend(sg.s, sg.s.PKGIdentity, ""); err != nil {
			return nil, err
		}
	}
	return sg.pkg, nil
}

// CheckInstaller finds the identity an installer package is signed with,
// for a build that makes one, before it starts. Signing ad hoc, the
// package is left unsigned and there is nothing to find.
func (sg *Signer) CheckInstaller(ctx context.Context) error {
	if sg.app == nil {
		return nil
	}
	b, err := sg.installer()
	if err != nil {
		return err
	}
	if _, err := b.Describe(ctx, "App.pkg"); err != nil {
		return explainInstaller(err)
	}
	return nil
}

func explainInstaller(err error) error {
	return fmt.Errorf("signing the installer: %w; an installer is signed with a Developer ID Installer certificate, not the app's Developer ID Application one: "+
		"make one at https://developer.apple.com/account/resources/certificates, and name it in macos.pkg.signing_identity if the default does not find it", err)
}

// Notarize submits path to Apple's notary service, waits for the
// verdict and, unless told not to, staples the ticket to path.
func (sg *Signer) Notarize(ctx context.Context, path string) error {
	if sg.app == nil {
		return errors.New("notarization needs a Developer ID signature")
	}
	if err := notarize(ctx, sg.app, path, sg.s.Notary.Staple); err != nil {
		return fmt.Errorf("notarizing %s with %s: %w", filepath.Base(path), sg.s.Notary.Describe(), err)
	}
	return nil
}

// Staples reports whether Notarize staples the ticket.
func (sg *Signer) Staples() bool { return sg.s.Notary.Staple }

// Close removes the temporary keychain or certificate file a certificate
// was put in for the run.
func (sg *Signer) Close() error {
	var errs []error
	for _, b := range []backend{sg.app, sg.pkg} {
		if b != nil {
			errs = append(errs, closeBackend(b))
		}
	}
	return errors.Join(errs...)
}

// signAdHoc signs path ad hoc, which zapp leaves out: it signs for
// distribution, with the hardened runtime and a secure timestamp, which
// need an identity. Ad hoc signing is for running on this Mac.
func signAdHoc(ctx context.Context, path, entitlements string) error {
	args := []string{"--force", "--sign", "-"}
	if entitlements != "" {
		args = append(args, "--entitlements", entitlements)
	}
	args = append(args, path)
	if out, err := exec.CommandContext(ctx, "codesign", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("codesign: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}
