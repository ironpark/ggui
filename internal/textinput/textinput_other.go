//go:build !darwin || ios

// Package textinput is ggui's copy of Ebitengine's exp/textinput. On macOS
// it is the package's source with one patch: a session that starts right
// after a commit keeps the marked text the IME queued in the same key
// press, so a Korean composition does not lose its first jamo (Ebitengine
// issue with discardMarkedText in start; PR on hold). Ebitengine's other
// platform backends need its internal packages, so everywhere else the
// package is the upstream one.
package textinput

import "github.com/hajimehoshi/ebiten/v2/exp/textinput"

type (
	Composer       = textinput.Composer
	SessionOptions = textinput.SessionOptions
	Composition    = textinput.Composition
	Commit         = textinput.Commit
	Field          = textinput.Field
)
