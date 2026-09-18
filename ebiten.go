package ggui

import "github.com/hajimehoshi/ebiten/v2"

// The types ggui hands a widget that come from Ebitengine, under ggui's own
// names, so that a widget package imports ggui alone. They are aliases, not
// definitions: ggui.KeyEnter and ebiten.KeyEnter are the same value of the
// same type, so code written either way keeps working and the two mix
// freely. What they do not do is hide Ebitengine; ggui renders with it, and
// Canvas.Image is still an *ebiten.Image.

//go:generate go run ./internal/keygen/cmd -o keys_gen.go

// KeyboardKey is a physical key on the keyboard, named for what it is on a US
// layout. The constants are in keys_gen.go; Chord and KeyEvent carry them,
// and ParseChord reads them from a string.
type KeyboardKey = ebiten.Key

// KeyMax is the largest KeyboardKey, for ranging over all of them.
const KeyMax = ebiten.KeyMax

// CursorShape is what the pointer looks like over a region; a widget picks
// one when it registers the region with Hit or HitCursor.
type CursorShape = ebiten.CursorShapeType

// The cursor shapes a platform offers. CursorShapeDefault is the arrow, and
// the zero value, which means a region does not ask for a shape at all.
const (
	CursorShapeDefault    = ebiten.CursorShapeDefault
	CursorShapeText       = ebiten.CursorShapeText
	CursorShapeCrosshair  = ebiten.CursorShapeCrosshair
	CursorShapePointer    = ebiten.CursorShapePointer
	CursorShapeEWResize   = ebiten.CursorShapeEWResize
	CursorShapeNSResize   = ebiten.CursorShapeNSResize
	CursorShapeNESWResize = ebiten.CursorShapeNESWResize
	CursorShapeNWSEResize = ebiten.CursorShapeNWSEResize
	CursorShapeMove       = ebiten.CursorShapeMove
	CursorShapeNotAllowed = ebiten.CursorShapeNotAllowed
)

// MouseButton is which button a PointerEvent came from.
type MouseButton = ebiten.MouseButton

// The buttons ggui routes. A press of anything else is ignored.
const (
	MouseButtonLeft   = ebiten.MouseButtonLeft
	MouseButtonMiddle = ebiten.MouseButtonMiddle
	MouseButtonRight  = ebiten.MouseButtonRight
)
