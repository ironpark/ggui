//go:build js

// Package textinput adapts ggfx's event-based desktop IME. The browser
// keeps exp/textinput's backend, which composes in a hidden text area.
package textinput

import "github.com/ironpark/ggwd/exp/textinput"

// Rebase is a no-op in the browser, whose text area holds the text itself.
func Rebase(*Composer, string, string) {}

type (
	Composer       = textinput.Composer
	SessionOptions = textinput.SessionOptions
	Composition    = textinput.Composition
	Commit         = textinput.Commit
)
