//go:build (!darwin || ios) && !windows

// Package textinput adapts ggfx's event-based desktop IME. Other platforms
// retain exp/textinput's platform backend.
package textinput

import "github.com/ironpark/ggfx/exp/textinput"

type (
	Composer       = textinput.Composer
	SessionOptions = textinput.SessionOptions
	Composition    = textinput.Composition
	Commit         = textinput.Commit
	Field          = textinput.Field
)
