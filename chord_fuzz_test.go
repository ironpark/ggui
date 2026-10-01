package ggui

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// modifierAliases are the words ParseChord reads as each modifier.
var modifierAliases = map[string][]string{
	"cmd":   {"cmd", "command", "super"},
	"ctrl":  {"ctrl", "control"},
	"alt":   {"alt", "option", "opt"},
	"shift": {"shift"},
	"meta":  {"meta", "win"},
}

// chordParts splits s the way a chord is written: lower-cased and trimmed,
// on "+", with a trailing "++" naming the plus key.
func chordParts(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	parts := strings.Split(s, "+")
	if strings.HasSuffix(s, "++") {
		parts = append(parts[:len(parts)-2], "plus")
	}
	return parts
}

func FuzzParseChord(f *testing.F) {
	for _, s := range []string{
		"", "+", "++", "+++", "a", "A", "cmd+a", "cmd+s", "Ctrl+Shift+Z", "ctrl++", "cmd+plus",
		"cmd+minus", "alt+enter", "option+return", "opt+esc", "super+up", "command+down",
		"control+left", "win+right", "meta+f12", "shift+tab", "f1", "F24", "numpadadd",
		"ctrl+alt+meta+shift+cmd+delete", "shift", "cmd", "s+ctrl", "ctrl+", "ctrl+s+",
		" cmd+s ", "\tctrl+k\n", "ctrl+bogus", "ctrl+ s", "CMD+SHIFT+PGUP", "bksp", "del",
		"pgdn", "cmd+shift+backquote", "ctrl+shift+equal", "cmd+K", "cmd+cmd+s",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		c, err := ParseChord(s)
		if err != nil {
			return
		}
		if c.Key > KeyMax || c.Key.String() == "" {
			t.Fatalf("ParseChord(%q) = key %d, which has no name", s, c.Key)
		}

		// Each modifier is set exactly when one of its words was written.
		parts := chordParts(s)
		has := func(mod string) bool {
			return slices.ContainsFunc(parts, func(p string) bool { return slices.Contains(modifierAliases[mod], p) })
		}
		for mod, set := range map[string]bool{
			"cmd": c.Cmd, "ctrl": c.Mods.Ctrl, "alt": c.Mods.Alt, "shift": c.Mods.Shift, "meta": c.Mods.Meta,
		} {
			if set != has(mod) {
				t.Fatalf("ParseChord(%q): %s is %v, but the chord's words are %q", s, mod, set, parts)
			}
		}
		if want := c.Cmd || c.Mods.Ctrl || c.Mods.Alt || c.Mods.Meta; c.Modified() != want {
			t.Fatalf("ParseChord(%q).Modified() = %v, want %v", s, c.Modified(), want)
		}

		// A press of the chord, with the modifiers the platform holds for
		// it, is the chord, and cmd becomes the platform's command key.
		held := c.held()
		if !(KeyEvent{Kind: KeyPress, Key: c.Key, Mods: held}).Is(c) {
			t.Fatalf("a press of %q with %+v held is not the chord", s, held)
		}
		if c.Cmd && !held.Ctrl && !held.Meta {
			t.Fatalf("%q holds cmd, but a press of it holds neither Ctrl nor Meta", s)
		}

		// String writes back what ParseChord reads.
		back, err := ParseChord(c.String())
		if err != nil || back != c {
			t.Fatalf("ParseChord(%q) = %+v, but its String %q parses to %+v, %v", s, c, c.String(), back, err)
		}
		if c.Label() == "" {
			t.Fatalf("chord %q has an empty label", s)
		}

		// Modifiers may come in any order before the key.
		words := strings.Split(c.String(), "+")
		mods := words[:len(words)-1]
		slices.Reverse(mods)
		reordered := strings.Join(append(mods, words[len(words)-1]), "+")
		if got, err := ParseChord(reordered); err != nil || got != c {
			t.Fatalf("ParseChord(%q) = %+v, %v; want %+v, as %q", reordered, got, err, c, s)
		}

		// Case does not matter.
		if isASCII(s) {
			if got, err := ParseChord(strings.ToUpper(s)); err != nil || got != c {
				t.Fatalf("ParseChord(%q) = %+v, %v; want %+v, as %q", strings.ToUpper(s), got, err, c, s)
			}
		}

		// Nor does space around the chord. A chord ending in "++" is left
		// out: see TestParseChordIgnoresSpaceAroundThePlusKey.
		if !strings.HasSuffix(strings.TrimSpace(s), "++") {
			spaced := " \t" + s + "\n "
			if got, err := ParseChord(spaced); err != nil || got != c {
				t.Fatalf("ParseChord(%q) = %+v, %v; want %+v, as %q", spaced, got, err, c, s)
			}
		}
	})
}

// isASCII reports whether s is ASCII only, so upper-casing it cannot turn
// one key name into another.
func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func TestParseChordReadsEveryAlias(t *testing.T) {
	t.Parallel()
	cases := map[string]Chord{
		"command+s": {Key: KeyS, Cmd: true},
		"super+s":   {Key: KeyS, Cmd: true},
		"control+s": {Key: KeyS, Mods: Mods{Ctrl: true}},
		"option+s":  {Key: KeyS, Mods: Mods{Alt: true}},
		"opt+s":     {Key: KeyS, Mods: Mods{Alt: true}},
		"win+s":     {Key: KeyS, Mods: Mods{Meta: true}},
		"meta+s":    {Key: KeyS, Mods: Mods{Meta: true}},
		"esc":       {Key: KeyEscape},
		"return":    {Key: KeyEnter},
		"down":      {Key: KeyArrowDown},
		"left":      {Key: KeyArrowLeft},
		"right":     {Key: KeyArrowRight},
		"cmd+plus":  {Key: KeyEqual, Cmd: true},
		"ctrl++":    {Key: KeyEqual, Mods: Mods{Ctrl: true}},
		"minus":     {Key: KeyMinus},
		"del":       {Key: KeyDelete},
		"pgup":      {Key: KeyPageUp},
		"pgdn":      {Key: KeyPageDown},
		"bksp":      {Key: KeyBackspace},
		"a":         {Key: KeyA},
		"shift+a":   {Key: KeyA, Mods: Mods{Shift: true}},
		" cmd+a\t":  {Key: KeyA, Cmd: true},
	}
	for in, want := range cases {
		if got, err := ParseChord(in); err != nil || got != want {
			t.Errorf("ParseChord(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, bad := range []string{" ", "shift", "cmd+ctrl", "++", "a+b", "ctrl+s+", "cmd + s", "ctrl+bogus"} {
		if c, err := ParseChord(bad); err == nil {
			t.Errorf("ParseChord(%q) = %+v, want an error", bad, c)
		}
	}
	mustPanic(t, "MustChord of a chord with no key", func() { MustChord("cmd+shift") })
}

func TestParseChordIgnoresSpaceAroundThePlusKey(t *testing.T) {
	t.Parallel()
	want := Chord{Key: KeyEqual, Mods: Mods{Ctrl: true}}
	if got, err := ParseChord("ctrl++ "); err != nil || got != want {
		t.Fatalf("ParseChord(%q) = %+v, %v; want %+v, as ParseChord(\"ctrl++\")", "ctrl++ ", got, err, want)
	}
}

func TestChordLabelNamesEveryModifierAndKey(t *testing.T) {
	t.Parallel()
	mac := runtimeIsDarwin()
	cases := []struct {
		chord      string
		mac, other string
	}{
		{"ctrl+alt+shift+meta+a", "⌃⌥⇧⌘A", "Ctrl+Alt+Shift+Win+A"},
		{"cmd+digit7", "⌘7", "Ctrl+7"},
		{"enter", "↩", "Enter"},
		{"shift+tab", "⇧⇥", "Shift+Tab"},
		{"alt+left", "⌥←", "Alt+Left"},
		{"cmd+plus", "⌘=", "Ctrl+="},
		{"f5", "F5", "F5"},
	}
	for _, tc := range cases {
		want := tc.other
		if mac {
			want = tc.mac
		}
		if got := MustChord(tc.chord).Label(); got != want {
			t.Errorf("MustChord(%q).Label() = %q, want %q", tc.chord, got, want)
		}
	}
}
