package ggui

import (
	"fmt"
	"strings"
	"testing"
)

func TestTextHeightFollowsLines(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	w := Text("aaa bbb ccc").NoWrap()
	w.Layout(Loose(Sz(5, 1000)), Env{})
	if len(w.lines) != 1 {
		t.Fatalf("%d lines, want 1", len(w.lines))
	}
}

func TestTextSizeScalesLayout(t *testing.T) {
	t.Parallel()
	small := Text("hello").Size(10).Layout(Loose(Sz(1000, 1000)), Env{})
	big := Text("hello").Size(20).Layout(Loose(Sz(1000, 1000)), Env{})
	if big.W <= small.W || big.H <= small.H {
		t.Fatalf("size 20 = %+v not larger than size 10 = %+v", big, small)
	}
}

func TestTextHeightFollowsInheritedLineHeight(t *testing.T) {
	t.Parallel()
	w := Text("first\nsecond\nthird")
	c := Loose(Sz(1000, 1000))
	env := Env{}.WithText(TextStyle{Size: 20, LineHeight: 1})
	before := w.Layout(c, env)
	after := w.Layout(c, env.WithText(TextStyle{LineHeight: 2}))
	if after.W != before.W || after.H-before.H != 40 {
		t.Fatalf("line spacing: before=%v after=%v; want unchanged width and 40 more height", before, after)
	}
}

func TestLoadFontRejectsGarbage(t *testing.T) {
	t.Parallel()
	if _, err := LoadFont([]byte("not a font")); err == nil {
		t.Fatal("LoadFont(garbage) = nil error")
	}
}

func TestTextEllipsisCutsWhatDoesNotFit(t *testing.T) {
	t.Parallel()
	w := Text("aaa bbb ccc\nshort").NoWrap().Ellipsis()
	full := w.Layout(Loose(Sz(1000, 1000)), Env{})
	if fmt.Sprint(w.lines) != "[aaa bbb ccc short]" {
		t.Fatalf("lines that fit were changed: %q", w.lines)
	}
	got := w.Layout(Loose(Sz(full.W*0.6, 1000)), Env{})
	if len(w.lines) != 2 || !strings.HasSuffix(w.lines[0], "…") || strings.HasSuffix(w.lines[0], " …") || w.lines[1] != "short" {
		t.Fatalf("lines = %q, want the first cut with an ellipsis and the second whole", w.lines)
	}
	if got.W > full.W*0.6 {
		t.Fatalf("width %v, want no more than %v", got.W, full.W*0.6)
	}
	w.Layout(Loose(Sz(1, 1000)), Env{})
	if w.lines[0] != "…" {
		t.Fatalf("with no room the line is %q, want the ellipsis alone", w.lines[0])
	}
}

func TestTextMaxLinesEndsTheLastShownLine(t *testing.T) {
	t.Parallel()
	w := Text("aaa bbb ccc ddd").MaxLines(2)
	wide := w.Layout(Loose(Sz(1000, 1000)), Env{})
	if len(w.lines) != 1 || w.lines[0] != "aaa bbb ccc ddd" {
		t.Fatalf("text that fits was cut: %q", w.lines)
	}
	narrow := w.Layout(Loose(Sz(wide.W*0.3, 1000)), Env{})
	if len(w.lines) != 2 || !strings.HasSuffix(w.lines[1], "…") {
		t.Fatalf("lines = %q, want two, the second ending in an ellipsis", w.lines)
	}
	for _, l := range w.lines {
		if lineWidth(l, w.faceAt(1)) > wide.W*0.3 {
			t.Fatalf("line %q is wider than the limit", l)
		}
	}
	two := Text("a\nb").Layout(Loose(Sz(1000, 1000)), Env{})
	if narrow.H != two.H {
		t.Fatalf("height %v, want two lines' %v", narrow.H, two.H)
	}
	p := NewProbe(w, Sz(wide.W*0.3, 100))
	defer p.Close()
	if _, ok := p.Semantics().Find(RoleText, "aaa bbb ccc ddd"); !ok {
		t.Fatal("the semantics tree lost the text that was cut")
	}
}

func TestCanvasFitTextCutsToTheWidth(t *testing.T) {
	t.Parallel()
	c := &Canvas{scale: 2}
	s := "a fairly long label"
	full := c.TextWidth(s, nil, 14)
	if got := c.FitText(s, nil, 14, full); got != s {
		t.Fatalf("text that fits came back %q", got)
	}
	got := c.FitText(s, nil, 14, full/2)
	if !strings.HasSuffix(got, "…") || c.TextWidth(got, nil, 14) > full/2 {
		t.Fatalf("FitText = %q, %v wide; want an ellipsis within %v", got, c.TextWidth(got, nil, 14), full/2)
	}
}
