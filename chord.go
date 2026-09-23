package ggui

import (
	"errors"
	"maps"
	"strings"
)

// Chord is a key with modifiers, as a shortcut names it: "cmd+s",
// "ctrl+shift+z", "escape". Parse one with ParseChord.
type Chord struct {
	Key  KeyboardKey
	Mods Mods
	// Cmd stands for the platform's command modifier: Meta on macOS, Ctrl
	// elsewhere. A chord written with "cmd" matches through Mods.Cmd.
	Cmd bool
}

// Modified reports whether the chord holds a modifier other than Shift,
// which is what makes it run before the focused widget.
func (c Chord) Modified() bool { return c.Cmd || c.Mods.Ctrl || c.Mods.Alt || c.Mods.Meta }

// String writes the chord back the way ParseChord reads it.
func (c Chord) String() string {
	var parts []string
	if c.Cmd {
		parts = append(parts, "cmd")
	}
	if c.Mods.Ctrl {
		parts = append(parts, "ctrl")
	}
	if c.Mods.Alt {
		parts = append(parts, "alt")
	}
	if c.Mods.Meta {
		parts = append(parts, "meta")
	}
	if c.Mods.Shift {
		parts = append(parts, "shift")
	}
	return strings.Join(append(parts, strings.ToLower(c.Key.String())), "+")
}

// Is reports whether ev is a press of c: the same key with the same
// modifiers, no more and no fewer.
func (ev KeyEvent) Is(c Chord) bool {
	if ev.Kind != KeyPress || ev.Key != c.Key {
		return false
	}
	want := c.Mods
	if c.Cmd {
		if runtimeIsDarwin() {
			want.Meta = true
		} else {
			want.Ctrl = true
		}
	}
	return ev.Mods == want
}

// ParseChord reads a chord such as "cmd+s", "ctrl+shift+z", "alt+enter"
// or "f1". Modifiers are cmd (the platform's command key), ctrl, alt (or
// option), shift and meta; the key is any ggfx key name, ignoring
// case, with esc, return, up, down, left, right, plus and minus accepted.
func ParseChord(s string) (Chord, error) {
	var c Chord
	parts := strings.Split(strings.ToLower(strings.TrimSpace(s)), "+")
	if s == "" {
		return c, errors.New("ggui: empty chord")
	}
	// "ctrl++" names the plus key.
	if strings.HasSuffix(s, "++") {
		parts = append(parts[:len(parts)-2], "plus")
	}
	for i, p := range parts {
		last := i == len(parts)-1
		switch p {
		case "cmd", "command", "super":
			c.Cmd = true
		case "ctrl", "control":
			c.Mods.Ctrl = true
		case "alt", "option", "opt":
			c.Mods.Alt = true
		case "shift":
			c.Mods.Shift = true
		case "meta", "win":
			c.Mods.Meta = true
		default:
			k, ok := keyNames()[p]
			if !ok || !last {
				return c, errors.New("ggui: unknown key " + strings.TrimSpace(s))
			}
			c.Key = k
		}
		if last && c.Key == 0 && p != "a" {
			return c, errors.New("ggui: chord " + strings.TrimSpace(s) + " names no key")
		}
	}
	return c, nil
}

// MustChord is ParseChord for a chord written in the source; it panics on
// an error.
func MustChord(s string) Chord {
	c, err := ParseChord(s)
	if err != nil {
		panic(err)
	}
	return c
}

var keyTable map[string]KeyboardKey

// keyNames maps every key's lower-cased ggfx name and a few aliases
// to the key, built on first use.
func keyNames() map[string]KeyboardKey {
	if keyTable != nil {
		return keyTable
	}
	keyTable = map[string]KeyboardKey{}
	for k := KeyboardKey(0); k <= KeyMax; k++ {
		if name := k.String(); name != "" {
			keyTable[strings.ToLower(name)] = k
		}
	}
	maps.Copy(keyTable, map[string]KeyboardKey{
		"esc": KeyEscape, "return": KeyEnter, "up": KeyArrowUp,
		"down": KeyArrowDown, "left": KeyArrowLeft, "right": KeyArrowRight,
		"plus": KeyEqual, "minus": KeyMinus, "del": KeyDelete,
		"pgup": KeyPageUp, "pgdn": KeyPageDown, "bksp": KeyBackspace,
	})
	return keyTable
}

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
