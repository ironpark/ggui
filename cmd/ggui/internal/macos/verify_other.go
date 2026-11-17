//go:build !darwin

package macos

import "context"

// Verify checks nothing away from a Mac: only what is signed is
// verified, and ggui signs on a Mac only. zapp's checks there would link
// its Rust signing library; see backend.
func Verify(context.Context, string, bool) (string, []Failure) { return "", nil }
