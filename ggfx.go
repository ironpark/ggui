package ggui

import (
	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggip"
)

// The types ggui hands a widget that come from ggfx and ggip, under ggui's
// own names, so that a widget package imports ggui alone. They are aliases,
// not definitions: ggui.KeyEnter and ggip.KeyEnter are the same value of the
// same type, so code written either way keeps working and the two mix
// freely. The input vocabulary is ggip's, and ggfx's events are converted to
// it as they arrive; ggfx's own keys are a different type. What the aliases
// do not do is hide ggfx; ggui renders with it, and Canvas.Image is still a
// *ggfx.Image.

//go:generate go run ./internal/tools/keygen/cmd -o keys_gen.go

// KeyboardKey is a physical key on the keyboard, named for what it is on a US
// layout. The constants are in keys_gen.go; Chord and KeyEvent carry them,
// and ParseChord reads them from a string.
type KeyboardKey = ggip.Key

// KeyMax is the largest KeyboardKey, for ranging over all of them.
const KeyMax = ggip.KeyMax

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
type MouseButton = ggip.MouseButton

// The buttons ggui routes. A press of anything else is ignored.
const (
	MouseButtonLeft   = ggip.MouseButtonLeft
	MouseButtonMiddle = ggip.MouseButtonMiddle
	MouseButtonRight  = ggip.MouseButtonRight
)

// ScrollPhase is where a trackpad's scroll is in the gesture making it.
type ScrollPhase = ggfx.ScrollPhase

const (
	ScrollPhaseNone          = ggfx.ScrollPhaseNone          // a wheel, or no gesture reported
	ScrollPhaseBegan         = ggfx.ScrollPhaseBegan         // the fingers start to move
	ScrollPhaseChanged       = ggfx.ScrollPhaseChanged       // the fingers move on
	ScrollPhaseEnded         = ggfx.ScrollPhaseEnded         // the fingers lift
	ScrollPhaseMomentum      = ggfx.ScrollPhaseMomentum      // momentum after they lift
	ScrollPhaseMomentumEnded = ggfx.ScrollPhaseMomentumEnded // the momentum stops
)

// laterPhase is the phase a frame's scrolls report, of what it had and
// the next: the fingers lifting stays reported over all after it in the
// frame, momentum too, so a gesture is not seen still on; the momentum
// stopping, over more momentum.
func laterPhase(had, next ScrollPhase) ScrollPhase {
	switch {
	case had == ScrollPhaseEnded:
		return had
	case had == ScrollPhaseMomentumEnded && (next == ScrollPhaseMomentum || next == ScrollPhaseNone):
		return had
	}
	return next
}
