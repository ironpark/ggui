package ggui

import (
	"math"
	"os"
	"slices"
	"testing"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/typesetting/bidi"
	"golang.org/x/image/font/gofont/goregular"
)

func TestVisualOrderAppliesRuleL2(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		levels []bidi.Level
		want   []int
	}{
		{[]bidi.Level{0, 1, 1, 0}, []int{0, 2, 1, 3}},
		{[]bidi.Level{1, 1, 1}, []int{2, 1, 0}},
		{[]bidi.Level{1, 2, 1}, []int{2, 1, 0}},
		{[]bidi.Level{1, 2, 2, 1}, []int{3, 1, 2, 0}},
		{[]bidi.Level{0, 0}, []int{0, 1}},
	} {
		if got := visualOrder(tc.levels); !slices.Equal(got, tc.want) {
			t.Errorf("visualOrder(%v) = %v, want %v", tc.levels, got, tc.want)
		}
	}
}

func oneStyle(s string) []lineStyle {
	return []lineStyle{{start: 0, end: len(s), font: DefaultFont(), size: DefaultTextSize}}
}

func TestLayoutLineOrdersRunsByDirection(t *testing.T) {
	t.Parallel()
	s := "abc שלום"
	ltr := layoutLine(s, oneStyle(s), DirLTR, "")
	if len(ltr) != 2 || ltr[0].rtl || !ltr[1].rtl || s[ltr[0].start:ltr[0].end] != "abc " {
		t.Fatalf("LTR pieces %+v, want \"abc \" then the Hebrew", ltr)
	}
	rtl := layoutLine(s, oneStyle(s), DirRTL, "")
	if len(rtl) != 2 || !rtl[0].rtl || rtl[1].rtl || s[rtl[1].start:rtl[1].end] != "abc" {
		t.Fatalf("RTL pieces %+v, want the Hebrew on the left and \"abc\" on the right", rtl)
	}
	if rtl[0].x != 0 || math.Abs(rtl[1].x-rtl[0].w) > 1e-9 {
		t.Fatalf("pieces do not abut: %+v", rtl)
	}
	// Inside a right-to-left piece the text runs leftwards.
	p := &rtl[0]
	if a, b := p.offsetX(s, oneStyle(s), p.start), p.offsetX(s, oneStyle(s), p.end); a <= b {
		t.Fatalf("Hebrew starts at x %v and ends at %v, want it to run leftwards", a, b)
	}
	// The Hebrew's first letter is at its right edge, under the RTL caret.
	if got := caretAt(s, rtl, oneStyle(s), piecesWidth(rtl)+5); got != 3 {
		t.Fatalf("past the right edge the caret is at %d, want the end of abc (3)", got)
	}
}

func TestLatinTextNeedsNoReordering(t *testing.T) {
	t.Parallel()
	if needsBidi("hello, world 123", DirLTR) {
		t.Fatal("plain Latin text in a left-to-right paragraph was reordered")
	}
	if !needsBidi("hello", DirRTL) || !needsBidi("x مرحبا", DirLTR) {
		t.Fatal("text that needs reordering was left alone")
	}
}

func TestShapingLanguage(t *testing.T) {
	t.Parallel()
	if got := shapingLanguage("", "مرحبا").String(); got != "und-Arab" {
		t.Fatalf("Arabic sample gave %q", got)
	}
	if got := shapingLanguage("he-IL", "x").String(); got != "he-IL" {
		t.Fatalf("explicit language gave %q", got)
	}
}

func TestDirectionalityMirrorsRowsAndColumns(t *testing.T) {
	t.Parallel()
	rtl := Env{}.With(DirectionKey, DirRTL)
	if rtl.Direction() != DirRTL || (Env{}).Direction() != DirLTR || DirRTL.String() != "rtl" {
		t.Fatal("Env.Direction does not follow DirectionKey")
	}
	row := Row(Box().Size(10, 10), Box().Size(20, 10))
	size := row.Layout(Loose(Sz(100, 100)), rtl)
	if row.offsets[0].X != size.W-10 || row.offsets[1].X != 0 {
		t.Fatalf("RTL row offsets %v in %v, want the first child rightmost", row.offsets, size)
	}
	col := Column(Box().Size(10, 10), Box().Size(30, 10))
	col.Layout(Loose(Sz(100, 100)), rtl)
	if col.offsets[0].X != 20 || col.offsets[1].X != 0 {
		t.Fatalf("RTL column offsets %v, want AlignStart on the right", col.offsets)
	}
	ltr := Row(Box().Size(10, 10), Box().Size(20, 10))
	ltr.Layout(Loose(Sz(100, 100)), Env{})
	if ltr.offsets[0].X != 0 {
		t.Fatalf("LTR row moved: %v", ltr.offsets)
	}
}

func TestTextUnderRTLStartsAtTheRight(t *testing.T) {
	t.Parallel()
	rtl := Env{}.With(DirectionKey, DirRTL)
	w := Text("שלום world")
	w.Layout(Loose(Sz(300, 100)), rtl)
	if len(w.bidi.lines) != 1 || w.bidi.lines[0] == nil {
		t.Fatal("RTL text was not reordered")
	}
	if a := pick(w.alignSet, w.align, startAlign(w.bidi.dir)); a != 1 {
		t.Fatalf("default alignment under RTL is %v, want 1", a)
	}
	w.Align(0)
	if a := pick(w.alignSet, w.align, startAlign(w.bidi.dir)); a != 0 {
		t.Fatalf("explicit Align(0) gave %v", a)
	}
	// The Latin word comes first in the reordered line: it is on the left.
	ps := w.bidi.lines[0]
	if ps[0].rtl || w.lines[0][ps[0].start:ps[0].end] != "world" {
		t.Fatalf("pieces %+v, want world on the left", ps)
	}

	r := RichText(Span("ab"))
	r.Layout(Tight(Sz(200, 20)), rtl)
	var box Rect
	r.para.rects(0, 2, 200, r.alignment(), func(b Rect) { box = b })
	if math.Abs(box.Origin.X+box.Size.W-200) > 0.01 {
		t.Fatalf("RTL rich text covers %v, want it at the right edge", box)
	}
	// The reordered lines, a selection, an underline and a rotated line
	// all paint into an image.
	img := ggfx.NewImage(300, 120)
	defer img.Deallocate()
	sel := RichText(Span("שלום "), Span("bold").Weight(WeightBold).Underline().OnTap(func() {})).Selectable()
	rot := Text("مرحبا 😀 abc")
	p := NewProbe(Directionality(DirRTL, Column(Text("مرحبا بالعالم 123 😀"), sel, FromFuncs(
		func(c Constraints, env Env) Size { return rot.Layout(c, env) },
		func(dst *Canvas, r Rect) { rot.PaintRotated(dst, r, 0.3) },
	))), Sz(300, 120))
	defer p.Close()
	p.RenderTo(img)
	p.Frame()
	node, ok := p.Semantics().Find(RoleText, "שלום bold")
	if !ok || node.Rect.Origin.X+node.Rect.Size.W < 299 {
		t.Fatalf("selectable RTL text at %v, want it at the right of the column", node.Rect)
	}
	p.Click(Pt(node.Rect.Origin.X+node.Rect.Size.W-2, node.Rect.Origin.Y+node.Rect.Size.H/2))
	p.Key("cmd+a")
	p.Frame()
	if sel.SelectedText() != "שלום bold" {
		t.Fatalf("select all in RTL selected %q", sel.SelectedText())
	}
}

// TestRightToLeftFallbackRunsAreReversed checks a Hebrew phrase set in a
// font without Hebrew: each word comes from the fallback, the space from
// the font itself, and the words must still run right to left.
func TestRightToLeftFallbackRunsAreReversed(t *testing.T) {
	const path = "/System/Library/Fonts/SFHebrew.ttf"
	if _, err := os.Stat(path); err != nil {
		t.Skip("no Hebrew system font")
	}
	he, err := LoadFontFile(path)
	if err != nil {
		t.Skip(err)
	}
	f := MustFont(goregular.TTF).Fallback(he.NoFallback())
	s := "שלום עולם"
	ps := layoutLine(s, []lineStyle{{start: 0, end: len(s), font: f, size: 14}}, DirRTL, "he")
	if len(ps) != 1 || !ps[0].rtl {
		t.Fatalf("pieces %+v, want one right-to-left piece", ps)
	}
	segs := ps[0].segs
	if len(segs) != 3 || segs[0].face != 1 || segs[1].face != 0 || segs[2].face != 1 {
		t.Fatalf("segs %+v, want the fallback, the space in the font, the fallback", segs)
	}
	first := ps[0].offsetX(s, []lineStyle{{start: 0, end: len(s), font: f, size: 14}}, 0)
	if math.Abs(first-ps[0].w) > 1e-6 {
		t.Fatalf("the first word starts at %v, want the right edge %v", first, ps[0].w)
	}
}
