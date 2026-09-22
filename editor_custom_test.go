package ggui

import (
	"strings"
	"testing"
	"unicode"
)

func TestTextInputFilterNativeCommitsAndCaret(t *testing.T) {
	v := State("")
	e := TextInput(v).Filter(func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsDigit(r) {
				return r
			}
			return -1
		}, s)
	})
	e.imeCommit("1a2-3")
	if v.Get() != "123" || e.ed.caret != 3 {
		t.Fatalf("filtered commit %q caret %d", v.Get(), e.ed.caret)
	}
	e.Select(1, 2)
	e.imeCommit("x9")
	if v.Get() != "193" || e.ed.caret != 2 {
		t.Fatal("replacement")
	}
	e.imeStart, e.imeEnd = 0, 3
	e.imeReplace("1", "z8", "3")
	if v.Get() != "183" || e.ed.caret != 2 {
		t.Fatal("IME replace")
	}
	e.Layout(Loose(Sz(200, 40)), Env{})
	e.PaintCustom(nil, Rct(Point{}, Sz(200, 40)), func(_ *Canvas, _ Rect, s TextInputState) Rect {
		if s.Text != "183" {
			t.Fatal(s)
		}
		return Rct(Pt(32, 8), Sz(1, 16))
	})
	if e.caretPx.Min.X != 32 {
		t.Fatal("custom IME caret")
	}
}
