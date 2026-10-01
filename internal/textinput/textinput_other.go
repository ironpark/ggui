//go:build js

// Package textinput adapts ggfx's event-based desktop IME. The browser
// keeps exp/textinput's backend, which composes in a hidden text area.
package textinput

import "github.com/ironpark/ggfx/exp/textinput"

type (
	Composer       = textinput.Composer
	SessionOptions = textinput.SessionOptions
	Composition    = textinput.Composition
	Commit         = textinput.Commit
)
