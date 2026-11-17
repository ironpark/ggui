package macos

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/ironpark/zapp/pkg/verify"
)

// Verify checks the app, disk image or installer at path as a release:
// its bundle, its signature and what Gatekeeper makes of it, with zapp's
// checks. It returns the report, as text, and the checks that failed.
//
// Gatekeeper rejects what is signed but not notarized, so when notarized
// is false that verdict is expected: the report shows it as a warning,
// and it is not counted as a failure.
func Verify(ctx context.Context, path string, notarized bool) (string, []Failure) {
	var r verify.Report
	switch strings.ToLower(filepath.Ext(path)) {
	case ".dmg":
		r = verify.DMG(ctx, path)
	case ".pkg":
		r = verify.PKG(ctx, path)
	default:
		r = verify.App(ctx, path)
	}
	var failed []Failure
	for i, c := range r.Checks {
		if c.Status != verify.Fail {
			continue
		}
		if !notarized && c.Name == "Gatekeeper" && strings.Contains(c.Detail, "Unnotarized") {
			r.Checks[i].Status = verify.Warn
			r.Checks[i].Detail += ", as expected before notarization"
			continue
		}
		failed = append(failed, Failure{c.Name, c.Detail})
	}
	return r.String(), failed
}
