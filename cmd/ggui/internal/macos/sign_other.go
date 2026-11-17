//go:build !darwin

package macos

import (
	"context"
	"fmt"
	"runtime"
)

// newBackend refuses: ggui signs with an identity on a Mac only; see
// backend.
func newBackend(Signing, string, string) (backend, error) {
	return nil, errSignElsewhere()
}

func notarize(context.Context, backend, string, bool) error { return errSignElsewhere() }

func closeBackend(backend) error { return nil }

func errSignElsewhere() error {
	return fmt.Errorf("ggui signs and notarizes macOS apps on a Mac, with Apple's tools; on %s it builds the bundle, disk image and installer unsigned. "+
		"Build the release on a Mac, as a macOS runner in CI does, or leave macos.signing_identity at \"-\"", runtime.GOOS)
}
