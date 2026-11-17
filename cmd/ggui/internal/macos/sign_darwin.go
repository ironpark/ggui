package macos

import (
	"context"
	"regexp"
	"strings"

	"github.com/ironpark/zapp/pkg/signing"
	zmacos "github.com/ironpark/zapp/pkg/signing/macos"
)

// fingerprint matches an identity's SHA-1 hash, as security
// find-identity prints it.
var fingerprint = regexp.MustCompile(`^[0-9A-Fa-f]{40}$`)

// newBackend sets up zapp's backend for signing with identity: Apple's
// tools, with the keychain or a certificate imported into a temporary one.
func newBackend(s Signing, identity, entitlements string) (backend, error) {
	c, err := s.credentials(identity, entitlements)
	if err != nil {
		return nil, err
	}
	b, err := signing.Select(c)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// credentials translates s for zapp, for signing with identity.
func (s Signing) credentials(identity, entitlements string) (signing.Credentials, error) {
	c := signing.Credentials{
		Entitlements: entitlements,
		P12File:      s.P12File, P12Base64: s.P12Base64,
		P12Password: s.P12Password, P12PasswordFile: s.P12PasswordFile,
		Profile: s.Notary.Profile, APIKeyFile: s.Notary.APIKeyFile,
		AppleID: s.Notary.AppleID, TeamID: s.Notary.TeamID, Password: s.Notary.Password,
		NotarizeTimeout: s.Notary.Timeout,
	}
	if s.HasCertificate() && identity == "-" {
		identity = "" // the certificate's Developer ID identity
	}
	c.Identity = identity
	if fingerprint.MatchString(identity) && !s.HasCertificate() {
		// zapp matches identities by name; a hash is turned into the
		// name of the identity it belongs to.
		ids, err := zmacos.ListIdentities(context.Background())
		if err != nil {
			return c, err
		}
		for _, id := range ids {
			if strings.EqualFold(id.Fingerprint, identity) {
				c.Identity = id.Description
			}
		}
	}
	return c, nil
}

// notarize submits path to Apple's notary service through b and, if
// staple, staples the ticket.
func notarize(ctx context.Context, b backend, path string, staple bool) error {
	return signing.Notarize(ctx, b.(signing.Backend), path, staple)
}

// closeBackend removes the temporary keychain a certificate was put in.
func closeBackend(b backend) error { return signing.Close(b.(signing.Backend)) }
