package ggui

import "github.com/ironpark/ggwd"

// The types ggui hands a widget that come from ggwd, under ggui's own names,
// so that a widget package imports ggui alone. They are aliases, not
// definitions: ggui.KeyEnter and ggwd.KeyEnter are the same value of the same
// type, so code written either way keeps working and the two mix freely. The
// input vocabulary is ggwd's, and its events carry it as they are. What the
// aliases do not do is hide ggfx; ggui renders with it, and Canvas.Image is
// still a *ggfx.Image.

//go:generate go run ./internal/tools/keygen/cmd -o keys_gen.go

// KeyboardKey is a physical key on the keyboard, named for what it is on a US
// layout. The constants are in keys_gen.go; Chord and KeyEvent carry them,
// and ParseChord reads them from a string.
type KeyboardKey = ggwd.Key

// KeyMax is the largest KeyboardKey, for ranging over all of them.
const KeyMax = ggwd.KeyMax

// CursorShape is what the pointer looks like over a region; a widget picks
// one when it registers the region with Hit or HitCursor.
type CursorShape = ggwd.CursorShape

// The cursor shapes a platform offers. CursorShapeDefault is the arrow, and
// the zero value, which means a region does not ask for a shape at all.
const (
	CursorShapeDefault    = ggwd.CursorShapeDefault
	CursorShapeText       = ggwd.CursorShapeText
	CursorShapeCrosshair  = ggwd.CursorShapeCrosshair
	CursorShapePointer    = ggwd.CursorShapePointer
	CursorShapeEWResize   = ggwd.CursorShapeEWResize
	CursorShapeNSResize   = ggwd.CursorShapeNSResize
	CursorShapeNESWResize = ggwd.CursorShapeNESWResize
	CursorShapeNWSEResize = ggwd.CursorShapeNWSEResize
	CursorShapeMove       = ggwd.CursorShapeMove
	CursorShapeNotAllowed = ggwd.CursorShapeNotAllowed
)

// MouseButton is which button a PointerEvent came from.
type MouseButton = ggwd.MouseButton

// The buttons ggui routes. A press of anything else is ignored.
const (
	MouseButtonLeft   = ggwd.MouseButtonLeft
	MouseButtonMiddle = ggwd.MouseButtonMiddle
	MouseButtonRight  = ggwd.MouseButtonRight
)

// ScrollPhase is where a trackpad's scroll is in the gesture making it.
type ScrollPhase = ggwd.ScrollPhase

const (
	ScrollPhaseNone          = ggwd.ScrollPhaseNone          // a wheel, or no gesture reported
	ScrollPhaseBegan         = ggwd.ScrollPhaseBegan         // the fingers start to move
	ScrollPhaseChanged       = ggwd.ScrollPhaseChanged       // the fingers move on
	ScrollPhaseEnded         = ggwd.ScrollPhaseEnded         // the fingers lift
	ScrollPhaseMomentum      = ggwd.ScrollPhaseMomentum      // momentum after they lift
	ScrollPhaseMomentumEnded = ggwd.ScrollPhaseMomentumEnded // the momentum stops
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
