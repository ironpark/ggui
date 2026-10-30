package ggui

import (
	"image"
	"strings"
	"testing"
	"unicode"
)

func TestTextInputFilterNativeCommitsAndCaret(t *testing.T) {
	t.Parallel()
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
	if v.Get() != "123" || e.ed.Caret != 3 {
		t.Fatalf("filtered commit %q caret %d", v.Get(), e.ed.Caret)
	}
	e.Select(1, 2)
	e.imeCommit("x9")
	if v.Get() != "193" || e.ed.Caret != 2 {
		t.Fatal("replacement")
	}
	e.imeStart, e.imeEnd = 0, 3
	e.imeReplace("1", "z8", "3")
	if v.Get() != "183" || e.ed.Caret != 2 {
		t.Fatal("IME replace")
	}
	e.Layout(Loose(Sz(200, 40)), Env{})
	e.PaintCustom(nil, Rct(Point{}, Sz(200, 40)), func(_ *Canvas, _ Rect, s TextInputState) Rect {
		if s.Text != "183" {
			t.Fatal(s)
		}
		return Rct(Pt(32, 8), Sz(1, 16))
	})
	if e.caretBounds.Min.X != 32 {
		t.Fatal("custom IME caret")
	}
}

// Native window APIs consume logical coordinates even on HiDPI displays.
func TestTextInputIMECaretUsesLogicalCoordinates(t *testing.T) {
	t.Parallel()
	for _, scale := range []float64{1, 1.5, 2} {
		for _, multiline := range []bool{false, true} {
			e := TextInput(State(""))
			e.multiline = multiline
			e.Layout(Loose(Sz(200, 80)), Env{})
			c := &Canvas{scale: scale}
			e.Paint(c, Rct(Pt(32, 8), Sz(200, 80)))
			want := image.Rect(32, 8, 33, int(8+e.height()))
			if got := e.imeSession().CaretBounds; got != want {
				t.Fatalf("scale %g multiline %v: got %v, want %v", scale, multiline, got, want)
			}
			e.PaintCustom(c, Rct(Point{}, Sz(200, 80)), func(*Canvas, Rect, TextInputState) Rect {
				return Rct(Pt(32, 8), Sz(1, 16))
			})
			if got := e.imeSession().CaretBounds; got != image.Rect(32, 8, 33, 24) {
				t.Fatalf("scale %g custom: got %v", scale, got)
			}
		}
	}
}
