package ggui

import "github.com/ironpark/ggwd"

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
