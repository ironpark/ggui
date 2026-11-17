package doctor

import (
	"context"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
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
		checkSigningElsewhere(r, m)
		return
	}
	checkIdentities(ctx, r, m)
	checkCertificate(r, m, proj)
	checkNotarization(ctx, r, m, proj)
}

// checkSigningElsewhere reports that away from a Mac ggui does not sign,
// which matters only when ggui.toml asks for a signature.
func checkSigningElsewhere(r *Report, m config.MacOS) {
	detail := "ggui signs macOS apps on a Mac only, with Apple's tools; here bundles, disk images and installers are left unsigned"
	if m.SigningIdentity != "-" || m.Notarize || m.P12File != "" {
		r.fail("macOS signing", detail+", and ggui.toml asks for a signature", "build the release on a Mac, as a macOS runner in CI does")
		return
	}
	r.ok("macOS signing", detail)
}
