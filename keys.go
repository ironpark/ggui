package ggui

import (
	"slices"

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
// the zero value: what shows where no region asks for a shape, and what a
// region asks for to show the arrow over one beneath it that asks for
// another.
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

// The keys that are the runtime's before they are the focused widget's:
// Tab, App.OnKey hooks, App.Shortcut and Canvas.Shortcut chords, and,
// after the widget, the bare-key shortcuts and a focus trap's Escape. A
// ChordClaimer keeps its chords from all of them.

// consumeBindings takes the keys that belong to the runtime rather than to a
// widget -- Tab, registered shortcuts and the chords that fire before the
// focused widget sees them -- and returns what is left for the widget. The
// frame's slice belongs to the caller of dispatch, so a removal clones it.
func (in *inputState) consumeBindings(keys []KeyboardKey, mods Mods) []KeyboardKey {
	// A chord the focused handler claims is its own: none of the bindings
	// below sees it.
	claimed := func(k KeyboardKey) bool { return in.claimedBy(KeyEvent{Kind: KeyPress, Key: k, Mods: mods}) != nil }
	if i := slices.Index(keys, KeyTab); i >= 0 && !claimed(KeyTab) {
		keys = slices.Delete(slices.Clone(keys), i, i+1)
		in.moveFocus(pick(mods.Shift, -1, 1))
	}
	if len(in.shortcuts) > 0 {
		keys = in.withoutShortcuts(keys, mods, claimed)
	}
	if len(in.chords) > 0 || len(in.painted) > 0 {
		keys = slices.DeleteFunc(slices.Clone(keys), func(k KeyboardKey) bool {
			return !claimed(k) && in.runChords(k, mods, true)
		})
	}
	return keys
}

// claimedBy returns the focused handler when it claims ev, else nil.
func (in *inputState) claimedBy(ev KeyEvent) KeyHandler {
	if in.focused == nil {
		return nil
	}
	cur := in.findKeyRegion(in.focused)
	if cur == nil {
		return nil
	}
	if c, ok := cur.key.(ChordClaimer); ok && c.ClaimsChord(ev) {
		return cur.key
	}
	return nil
}

// sendClaimed delivers ev to the focused handler if it claims it, and
// reports whether it did: how a menu action's chord reaches a focused
// field rather than the action.
func (in *inputState) sendClaimed(ev KeyEvent) bool {
	h := in.claimedBy(ev)
	if h == nil {
		return false
	}
	h.HandleKey(ev)
	return true
}

// consumes reports whether h claims ev through KeyConsumer.
func consumes(h KeyHandler, ev KeyEvent) bool {
	if c, ok := h.(KeyConsumer); ok {
		return c.ConsumesKey(ev)
	}
	return false
}

// addShortcut registers a chord.
func (in *inputState) addShortcut(chord string, fn func()) *ShortcutHandle {
	h := &ShortcutHandle{chord: MustChord(chord), fn: fn}
	in.chords = append(in.chords, h)
	return h
}

// runChords runs the shortcuts that match k, either the ones that go
// before the focused widget (modified or exclusive) or the rest, and
// reports whether one ran.
func (in *inputState) runChords(k KeyboardKey, mods Mods, before bool) bool {
	ran := false
	ev := KeyEvent{Kind: KeyPress, Key: k, Mods: mods}
	for _, h := range in.chords {
		if h.removed || (h.chord.Modified() || h.exclusive) != before || !ev.Is(h.chord) {
			continue
		}
		h.fn()
		ran = true
	}
	in.chords = slices.DeleteFunc(in.chords, func(h *ShortcutHandle) bool { return h.removed })
	scope := in.activeScope()
	for _, p := range in.painted {
		if p.chord.Modified() != before || !ev.Is(p.chord) || (scope != nil && p.scope != scope) {
			continue
		}
		p.fn()
		ran = true
	}
	return ran
}

// withoutShortcuts returns keys less those a shortcut consumed, copying
// only once one has.
func (in *inputState) withoutShortcuts(keys []KeyboardKey, mods Mods, claimed func(KeyboardKey) bool) []KeyboardKey {
	out, copied := keys, false
	for i, k := range keys {
		switch taken := !claimed(k) && in.shortcut(k, mods); {
		case taken && !copied:
			out, copied = slices.Clone(keys[:i]), true
		case !taken && copied:
			out = append(out, k)
		}
	}
	return out
}

// shortcut offers a key press to the OnKey handlers and reports whether
// one consumed it.
func (in *inputState) shortcut(k KeyboardKey, mods Mods) bool {
	for _, fn := range in.shortcuts {
		if fn(KeyEvent{Kind: KeyPress, Key: k, Mods: mods}) {
			return true
		}
	}
	return false
}

// afterFocused runs what comes after the focused widget saw the keys: the
// bare-key shortcuts and the trap's Escape, for keys the widget did not
// consume.
func (in *inputState) afterFocused(f frameInput, h KeyHandler) {
	for _, k := range f.keys {
		ev := KeyEvent{Kind: KeyPress, Key: k, Mods: f.mods}
		if h != nil && consumes(h, ev) {
			continue
		}
		if k == KeyEscape && f.mods == (Mods{}) {
			if s := in.activeScope(); s != nil && s.escape != nil {
				s.escape()
				continue
			}
		}
		in.runChords(k, f.mods, false)
	}
}
