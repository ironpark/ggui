package ggui

import (
	"image/color"
	"math"
	"strings"
	"testing"
)

// lineTexts returns what each line of a laid-out RichText draws.
func lineTexts(t *RichTextWidget) []string {
	var out []string
	for _, ln := range t.para.lines {
		out = append(out, ln.s)
	}
	return out
}

func TestRichTextWrapsAcrossSpans(t *testing.T) {
	t.Parallel()
	w := RichText(Span("aaa "), Span("bbb").Weight(WeightBold), Span(" ccc"))
	wide := w.Layout(Loose(Sz(1000, 1000)), Env{})
	if got := lineTexts(w); len(got) != 1 || got[0] != "aaa bbb ccc" {
		t.Fatalf("wide: lines = %q, want one", got)
	}
	if w.Text() != "aaa bbb ccc" {
		t.Fatalf("Text = %q", w.Text())
	}
	narrow := w.Layout(Loose(Sz(wide.W*0.7, 1000)), Env{})
	if got := lineTexts(w); len(got) != 2 || got[0] != "aaa bbb" || got[1] != "ccc" {
		t.Fatalf("narrow: lines = %q, want [aaa bbb, ccc]", got)
	}
	if narrow.W > wide.W*0.7 || narrow.H <= wide.H {
		t.Fatalf("narrow = %+v, want no wider than %v and taller than %v", narrow, wide.W*0.7, wide.H)
	}
	// The first line holds the regular and the bold span, the bold one
	// in a style of its own.
	if n := len(w.para.lines[0].styles); n != 2 {
		t.Fatalf("first line has %d styles, want 2", n)
	}
}

func TestRichTextOneSpanMeasuresAsText(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"hello world", "a\nb\nc", ""} {
		got := RichText(Span(s)).Layout(Loose(Sz(1000, 1000)), Env{})
		want := Text(s).Layout(Loose(Sz(1000, 1000)), Env{})
		if math.Abs(got.W-want.W) > 0.01 || math.Abs(got.H-want.H) > 0.01 {
			t.Fatalf("%q: RichText %v, Text %v", s, got, want)
		}
	}
}

func TestRichTextBaselineIsTheTallestSpans(t *testing.T) {
	t.Parallel()
	small := RichText(Span("x"))
	small.Layout(Loose(Sz(1000, 1000)), Env{})
	mixed := RichText(Span("x"), Span("Y").Size(40))
	mixed.Layout(Loose(Sz(1000, 1000)), Env{})
	sb, ok1 := small.Baseline()
	mb, ok2 := mixed.Baseline()
	if !ok1 || !ok2 || mb <= sb {
		t.Fatalf("baselines %v (%v) and %v (%v), want the large span's to sit lower", sb, ok1, mb, ok2)
	}
	text := Text("x")
	text.Layout(Loose(Sz(1000, 1000)), Env{})
	if tb, _ := text.Baseline(); math.Abs(tb-sb) > 0.01 {
		t.Fatalf("one span's baseline %v, Text's %v", sb, tb)
	}
}

func TestRichTextMaxLinesAndEllipsis(t *testing.T) {
	t.Parallel()
	w := RichText(Span("aaa bbb "), Span("ccc ddd").Weight(WeightBold)).MaxLines(1)
	full := RichText(Span("aaa bbb ccc ddd")).Layout(Loose(Sz(1000, 1000)), Env{})
	w.Layout(Loose(Sz(full.W/2, 1000)), Env{})
	got := lineTexts(w)
	if len(got) != 1 || !strings.HasSuffix(got[0], "…") || strings.HasSuffix(got[0], " …") {
		t.Fatalf("lines = %q, want one cut with an ellipsis", got)
	}
	if w.para.lines[0].width > full.W/2+0.01 {
		t.Fatalf("cut line is %v wide, want at most %v", w.para.lines[0].width, full.W/2)
	}

	e := RichText(Span("aaa bbb ccc ddd")).NoWrap().Ellipsis()
	e.Layout(Loose(Sz(full.W/2, 1000)), Env{})
	if got := lineTexts(e); len(got) != 1 || !strings.HasSuffix(got[0], "…") {
		t.Fatalf("NoWrap Ellipsis lines = %q", got)
	}
	fits := RichText(Span("aaa")).Ellipsis()
	fits.Layout(Loose(Sz(1000, 1000)), Env{})
	if got := lineTexts(fits); got[0] != "aaa" {
		t.Fatalf("text that fits was cut: %q", got)
	}
}

func TestRichTextAlignAndSpansSetter(t *testing.T) {
	t.Parallel()
	w := RichText(Span("ab")).Align(1)
	w.Layout(Tight(Sz(200, 20)), Env{})
	var box Rect
	w.para.rects(0, 2, 200, w.alignment(), func(r Rect) { box = r })
	if math.Abs(box.Origin.X+box.Size.W-200) > 0.01 {
		t.Fatalf("right-aligned text covers %v, want it to end at 200", box)
	}
	w.Spans(Span("one "), Span("two"))
	w.Layout(Loose(Sz(1000, 1000)), Env{})
	if w.Text() != "one two" {
		t.Fatalf("after Spans, Text = %q", w.Text())
	}
	spans := State([]TextSpan{Span("bound")})
	w.BindSpans(spans)
	w.Layout(Loose(Sz(1000, 1000)), Env{})
	if w.Text() != "bound" {
		t.Fatalf("after BindSpans, Text = %q", w.Text())
	}
}

func TestRichTextSpansInheritAndOverride(t *testing.T) {
	t.Parallel()
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	env := Env{}.WithText(TextStyle{Color: red, Size: 20})
	w := RichText(Span("a"), Span("b").Color(blue).Size(10))
	w.Layout(Loose(Sz(1000, 1000)), env)
	a, b := w.para.runs[0].style, w.para.runs[1].style
	if a.Color != red || a.Size != 20 {
		t.Fatalf("plain span drew in %v at %v, want the inherited red at 20", a.Color, a.Size)
	}
	if b.Color != blue || b.Size != 10 {
		t.Fatalf("styled span drew in %v at %v, want blue at 10", b.Color, b.Size)
	}
	if Span("x").Underline().Name("n").Text() != "x" {
		t.Fatal("Span setters lost the text")
	}
}

func TestRichTextLinkTapsWithPointerCursorAndA11y(t *testing.T) {
	t.Parallel()
	taps := 0
	p := NewProbe(RichText(Span("Read the "), Span("guide").Underline().OnTap(func() { taps++ }), Span(" first.")), Sz(400, 40))
	defer p.Close()
	link, ok := p.FindRole(RoleLink, "guide")
	if !ok {
		t.Fatal("the tappable span registered no link")
	}
	p.Move(link.Center())
	if c := p.Cursor(); c != CursorShapePointer {
		t.Fatalf("cursor over the link is %v, want the pointer", c)
	}
	p.Click(link.Center())
	if taps != 1 {
		t.Fatalf("%d taps after a click, want 1", taps)
	}
	tree := p.Semantics()
	if _, ok := tree.Find(RoleText, "Read the guide first."); !ok {
		t.Fatal("the paragraph is missing from the semantics tree")
	}
	node, ok := tree.Find(RoleLink, "guide")
	if !ok || node.Actions&ActionPress == 0 {
		t.Fatalf("link node %+v, want a pressable link", node)
	}
	if node.Parent < 0 || tree.At(node.Parent).Role != RoleText {
		t.Fatal("the link is not a child of its paragraph")
	}
	p.Perform(node.ID, Action{Kind: ActionPress})
	if taps != 2 {
		t.Fatalf("%d taps after ActionPress, want 2", taps)
	}
}
