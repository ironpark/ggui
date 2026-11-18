package doctor

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
	zmacos "github.com/ironpark/zapp/pkg/signing/macos"
)

// checkIdentities lists the keychain's signing identities, and checks the
// one ggui.toml names is among them.
func checkIdentities(ctx context.Context, r *Report, m config.MacOS) {
	ids, err := zmacos.ListIdentities(ctx)
	if err != nil {
		r.warn("signing identities", "security find-identity failed: "+err.Error(), "")
		return
	}
	var names []string
	developerID := false
	for _, id := range ids {
		names = append(names, id.Description)
		developerID = developerID || strings.HasPrefix(id.Description, "Developer ID Application")
	}
	switch {
	case len(ids) == 0:
		r.warn("signing identities", "none in the keychain: builds are signed ad hoc, which runs on this Mac only",
			"to ship to other Macs, create a Developer ID Application certificate (Apple Developer Program) at https://developer.apple.com/account/resources/certificates, or sign in CI with a .p12")
	case !developerID:
		r.warn("signing identities", strings.Join(names, "\n")+"\nnone is a Developer ID Application identity, which apps shipped outside the App Store are signed with",
			"create one at https://developer.apple.com/account/resources/certificates")
	default:
		r.ok("signing identities", strings.Join(names, "\n"))
	}

	id := m.SigningIdentity
	if id == "-" || os.Getenv(m.P12Base64Env) != "" || m.P12File != "" {
		return
	}
	for _, have := range ids {
		if strings.Contains(have.Description, id) || strings.EqualFold(have.Fingerprint, id) {
			r.ok("signing identity", fmt.Sprintf("macos.signing_identity %q is %s", id, have.Description))
			return
		}
	}
	r.fail("signing identity", fmt.Sprintf("macos.signing_identity %q is not in the keychain, so ggui build --target macos will fail", id),
		"name one of the identities above, or \"-\" to sign ad hoc")
}

// checkNotarization reports the tools notarizing takes and the
// credentials ggui.toml and the environment give it.
func checkNotarization(ctx context.Context, r *Report, m config.MacOS, proj *project.Project) {
	var missing []string
	for _, tool := range []string{"notarytool", "stapler"} {
		if run(ctx, "xcrun", "--find", tool) == "" {
			missing = append(missing, tool)
		}
	}
	if len(missing) > 0 {
		r.warn("notarytool", strings.Join(missing, " and ")+" not found: notarizing needs the Xcode command line tools", "xcode-select --install")
	} else {
		r.ok("notarytool", "found, with stapler")
	}

	n := m.Notary
	var detail, fix string
	ok := true
	switch {
	case n.Profile != "":
		detail = fmt.Sprintf("the notarytool profile %q (macos.notary.profile)", n.Profile)
	case n.APIKeyFile != "":
		path := n.APIKeyFile
		if proj != nil {
			path = proj.Path(path)
		}
		detail = "the App Store Connect API key " + path
		if _, err := os.Stat(path); err != nil {
			ok, fix = false, "macos.notary.api_key_file: "+err.Error()
		}
	case n.AppleID != "":
		detail = fmt.Sprintf("the Apple ID %s of team %s", n.AppleID, n.TeamID)
		if os.Getenv(n.PasswordEnv) == "" {
			ok, fix = false, "set $"+n.PasswordEnv+" to the Apple ID's app-specific password when notarizing"
		}
	default:
		detail = "no credentials in ggui.toml; ggui build --notarize takes --notary-profile, --api-key-file, or --apple-id and --team-id"
		if !m.Notarize {
			r.ok("notarization", detail)
			return
		}
		ok, fix = false, "xcrun notarytool store-credentials NAME, then set macos.notary.profile = \"NAME\""
	}
	switch {
	case ok:
		r.ok("notarization", detail)
	case m.Notarize:
		r.fail("notarization", detail+"; macos.notarize is on", fix)
	default:
		r.warn("notarization", detail, fix)
	}
}
