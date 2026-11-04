package ggui

import (
	"github.com/ironpark/ggwd"
)

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

// Chord is a key with modifiers, as a shortcut names it: "cmd+s",
// "ctrl+shift+z", "escape". Parse one with ParseChord. It is a ggwd.Chord;
// ggwd parses, matches and labels it.
type Chord ggwd.Chord

// Modified reports whether the chord holds a modifier other than Shift,
// which is what makes it run before the focused widget.
func (c Chord) Modified() bool { return ggwd.Chord(c).Modified() }

// String writes the chord back the way ParseChord reads it.
func (c Chord) String() string { return ggwd.Chord(c).String() }

// Label writes the chord the way the platform shows it to a user, in a
// menu or a tooltip: "⇧⌘S" on macOS, "Ctrl+Shift+S" elsewhere.
func (c Chord) Label() string { return ggwd.Chord(c).Label() }

// Is reports whether ev is a press of c: the same key with the same
// modifiers, no more and no fewer.
func (ev KeyEvent) Is(c Chord) bool {
	return ev.Kind == KeyPress && ggwd.Chord(c).Matches(ev.Key, ev.Mods)
}

// held is the modifiers a press of c holds on this platform, cmd turned
// into Meta on macOS and Ctrl elsewhere.
func (c Chord) held() Mods { return ggwd.Chord(c).Held() }

// ParseChord reads a chord such as "cmd+s", "ctrl+shift+z", "alt+enter"
// or "f1". See ggwd.ParseChord for the names it reads.
func ParseChord(s string) (Chord, error) {
	c, err := ggwd.ParseChord(s)
	return Chord(c), err
}

// MustChord is ParseChord for a chord written in the source; it panics on
// an error.
func MustChord(s string) Chord { return Chord(ggwd.MustChord(s)) }

// ShortcutHandle is a registered shortcut, for making it exclusive or
// removing it.
type ShortcutHandle struct {
	chord     Chord
	fn        func()
	exclusive bool
	removed   bool
}

// Exclusive makes a bare-key shortcut run before the focused widget and
// take the key from it, as a chord with a modifier always does. Without
// it, the widget sees the key first and the shortcut runs only when the
// widget did not consume it.
func (h *ShortcutHandle) Exclusive() *ShortcutHandle { h.exclusive = true; return h }

// Remove unregisters the shortcut.
func (h *ShortcutHandle) Remove() { h.removed = true }

// Chord returns what the shortcut listens for.
func (h *ShortcutHandle) Chord() Chord { return h.chord }
