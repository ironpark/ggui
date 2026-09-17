package ggui

import (
	"strings"
	"testing"
)

func TestTextHeightFollowsLines(t *testing.T) {
	one := Text("a").Layout(Loose(Sz(1000, 1000)), Env{})
	two := Text("a\nb").Layout(Loose(Sz(1000, 1000)), Env{})
	if one.H <= 0 || two.H <= one.H {
		t.Fatalf("heights %v and %v, want one line shorter than two", one.H, two.H)
	}
	if d := two.H - one.H - DefaultTextSize*1.2; d > 1e-9 || d < -1e-9 {
		t.Fatalf("second line added %v, want the line spacing %v", two.H-one.H, DefaultTextSize*1.2)
	}
}

func TestTextWrapsAtSpaces(t *testing.T) {
	w := Text("aaa bbb ccc")
	wide := w.Layout(Loose(Sz(1000, 1000)), Env{})
	if len(w.lines) != 1 {
		t.Fatalf("wide: %d lines, want 1", len(w.lines))
	}
	narrow := w.Layout(Loose(Sz(wide.W*0.7, 1000)), Env{})
	if len(w.lines) != 2 || w.lines[0] != "aaa bbb" || w.lines[1] != "ccc" {
		t.Fatalf("narrow: lines = %q, want [aaa bbb, ccc]", w.lines)
	}
	if narrow.W > wide.W*0.7 || narrow.H <= wide.H {
		t.Fatalf("narrow = %+v, want no wider than %v and taller than %v", narrow, wide.W*0.7, wide.H)
	}
}

func TestTextBreaksLongWordsBetweenRunes(t *testing.T) {
	w := Text(strings.Repeat("x", 40))
	full := w.Layout(Loose(Sz(1000, 1000)), Env{})
	w.Layout(Loose(Sz(full.W/4, 1000)), Env{})
	if len(w.lines) < 4 {
		t.Fatalf("%d lines, want at least 4", len(w.lines))
	}
	for _, l := range w.lines {
		if lineWidth(l, w.faceAt(1)) > full.W/4 {
			t.Fatalf("line %q is wider than the limit", l)
		}
	}
}

func TestTextNoWrapKeepsOneLine(t *testing.T) {
	w := Text("aaa bbb ccc").NoWrap()
	w.Layout(Loose(Sz(5, 1000)), Env{})
	if len(w.lines) != 1 {
		t.Fatalf("%d lines, want 1", len(w.lines))
	}
}

func TestTextSizeScalesLayout(t *testing.T) {
	small := Text("hello").Size(10).Layout(Loose(Sz(1000, 1000)), Env{})
	big := Text("hello").Size(20).Layout(Loose(Sz(1000, 1000)), Env{})
	if big.W <= small.W || big.H <= small.H {
		t.Fatalf("size 20 = %+v not larger than size 10 = %+v", big, small)
	}
}

func TestLoadFontRejectsGarbage(t *testing.T) {
	if _, err := LoadFont([]byte("not a font")); err == nil {
		t.Fatal("LoadFont(garbage) = nil error")
	}
}
