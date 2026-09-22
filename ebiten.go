package ggui

import "github.com/ironpark/ggfx"

// The types ggui hands a widget that come from Ebitengine, under ggui's own
// names, so that a widget package imports ggui alone. They are aliases, not
// definitions: ggui.KeyEnter and ggfx.KeyEnter are the same value of the
// same type, so code written either way keeps working and the two mix
// freely. What they do not do is hide Ebitengine; ggui renders with it, and
// Canvas.Image is still an *ggfx.Image.

//go:generate go run ./internal/tools/keygen/cmd -o keys_gen.go

// KeyboardKey is a physical key on the keyboard, named for what it is on a US
// layout. The constants are in keys_gen.go; Chord and KeyEvent carry them,
// and ParseChord reads them from a string.
type KeyboardKey = ggfx.Key

// KeyMax is the largest KeyboardKey, for ranging over all of them.
const KeyMax = ggfx.KeyMax

// CursorShape is what the pointer looks like over a region; a widget picks
// one when it registers the region with Hit or HitCursor.
type CursorShape = ggfx.CursorShapeType

// The cursor shapes a platform offers. CursorShapeDefault is the arrow, and
// the zero value, which means a region does not ask for a shape at all.
const (
	CursorShapeDefault    = ggfx.CursorShapeDefault
	CursorShapeText       = ggfx.CursorShapeText
	CursorShapeCrosshair  = ggfx.CursorShapeCrosshair
	CursorShapePointer    = ggfx.CursorShapePointer
	CursorShapeEWResize   = ggfx.CursorShapeEWResize
	CursorShapeNSResize   = ggfx.CursorShapeNSResize
	CursorShapeNESWResize = ggfx.CursorShapeNESWResize
	CursorShapeNWSEResize = ggfx.CursorShapeNWSEResize
	CursorShapeMove       = ggfx.CursorShapeMove
	CursorShapeNotAllowed = ggfx.CursorShapeNotAllowed
)

// MouseButton is which button a PointerEvent came from.
type MouseButton = ggfx.MouseButton

// The buttons ggui routes. A press of anything else is ignored.
const (
	MouseButtonLeft   = ggfx.MouseButtonLeft
	MouseButtonMiddle = ggfx.MouseButtonMiddle
	MouseButtonRight  = ggfx.MouseButtonRight
)
