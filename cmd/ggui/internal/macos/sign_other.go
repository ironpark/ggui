//go:build !darwin && !(cgo && (amd64 || arm64) && (linux || (windows && ggui_rcodesign)))

package macos

import (
	"context"
	"fmt"
	"runtime"
)

// Toolchain reports that this build of ggui cannot sign macOS apps, and
// how to get one that can; see backend.
func Toolchain() (string, error) { return "", errSignElsewhere() }

// newBackend refuses: this build of ggui has no rcodesign; see backend.
func newBackend(Signing, string, string) (backend, error) {
	return nil, errSignElsewhere()
}

func notarize(context.Context, backend, string, bool) error { return errSignElsewhere() }

func closeBackend(backend) error { return nil }

// errSignElsewhere says why this build cannot sign, and what can: a
// build of ggui that links zapp's rcodesign library, or a Mac.
func errSignElsewhere() error {
	const fallback = "; or build the release on a Mac, or leave macos.signing_identity at \"-\" to build it unsigned"
	const install = "github.com/ironpark/ggui/cmd/ggui@latest"
	switch platform := runtime.GOOS + "/" + runtime.GOARCH; platform {
	case "linux/amd64", "linux/arm64":
		return fmt.Errorf("this build of ggui cannot sign or notarize macOS apps: on %s it signs with zapp's rcodesign library, which only a build with cgo links. "+
			"Install a release binary of ggui, which has it, or build ggui with cgo and a C compiler (gcc or clang) on PATH: CGO_ENABLED=1 go install %s%s",
			platform, install, fallback)
	case "windows/amd64", "windows/arm64":
		cc := "x86_64-w64-mingw32-clang"
		if runtime.GOARCH == "arm64" {
			cc = "aarch64-w64-mingw32-clang"
		}
		return fmt.Errorf("this build of ggui cannot sign or notarize macOS apps: on %s it signs with zapp's rcodesign library, which only a build with cgo and the tag ggui_rcodesign links. "+
			"Install a release binary of ggui, which has it, or build ggui with LLVM MinGW (https://github.com/mstorsjo/llvm-mingw) on PATH, whose toolchain the library is built with, not MinGW's GCC: "+
			"set CGO_ENABLED=1 and CC=%s, then go install -tags ggui_rcodesign %s%s",
			platform, cc, install, fallback)
	}
	return fmt.Errorf("ggui cannot sign or notarize macOS apps on %s/%s: away from a Mac it signs with zapp's rcodesign library, which is built for Linux and Windows on amd64 and arm64 only. "+
		"Build the release on a Mac or on one of those, or leave macos.signing_identity at \"-\" to build it unsigned", runtime.GOOS, runtime.GOARCH)
}
