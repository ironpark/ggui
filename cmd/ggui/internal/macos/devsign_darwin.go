//go:build darwin

package macos

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/machosign"
	zmacos "github.com/ironpark/zapp/pkg/signing/macos"
)

// listKeychainIdentities returns valid signing identities from the macOS
// keychain using zapp's security find-identity wrapper.
func listKeychainIdentities(ctx context.Context) ([]KeychainIdentity, error) {
	ids, err := zmacos.ListIdentities(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]KeychainIdentity, len(ids))
	for i, id := range ids {
		out[i] = KeychainIdentity{
			Fingerprint: id.Fingerprint,
			Description: id.Description,
			Type:        id.Type,
		}
	}
	return out, nil
}

// signDevPath signs a dev bundle or standalone binary on macOS.
// Ad-hoc bundles use pure-Go machosign; keychain identities invoke codesign
// with --timestamp=none (offline, fast, no hardened runtime) so TCC
// permissions persist across rebuilds without slowing down the dev loop.
func signDevPath(ctx context.Context, ds DevSigning, path string, isBundle bool, identifier string) error {
	if ds.AdHoc() {
		if isBundle {
			return signAdHoc(path, ds.Entitlements)
		}
		if ds.Entitlements == "" && identifier == "" {
			return nil
		}
		ents, err := readEntitlements(ds.Entitlements)
		if err != nil {
			return err
		}
		return machosign.SignFile(path, machosign.Options{
			Identifier:   identifier,
			Entitlements: ents,
		})
	}
	args := []string{"--force", "--sign", ds.Target, "--timestamp=none"}
	if !isBundle && identifier != "" {
		args = append(args, "--identifier", identifier)
	}
	if ds.Entitlements != "" {
		args = append(args, "--entitlements", ds.Entitlements)
	}
	args = append(args, path)
	cmd := exec.CommandContext(ctx, "codesign", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		hint := ""
		if strings.Contains(msg, "errSecInternalComponent") {
			hint = "; keychain may be locked (unlock it with `security unlock-keychain` or pass --sign - to sign ad hoc)"
		}
		if msg != "" {
			return fmt.Errorf("codesign %s: %s%s", ds.Display, msg, hint)
		}
		return fmt.Errorf("codesign %s: %w%s", ds.Display, err, hint)
	}
	return nil
}
