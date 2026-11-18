//go:build !darwin && !(cgo && (amd64 || arm64) && (linux || (windows && ggui_rcodesign)))

package macos

import "context"

// Verify checks nothing in a build of ggui that cannot sign here: only
// what is signed is verified, and zapp's checks would link the rcodesign
// library this build leaves out; see backend.
func Verify(context.Context, string, bool) (string, []Failure) { return "", nil }
