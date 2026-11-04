package ggui

import (
	"math"
	"testing"

	"github.com/ironpark/ggfx"
)

func TestDirectionalityMirrorsRowsAndColumns(t *testing.T) {
	t.Parallel()
	rtl := Env{}.With(DirectionKey, DirRTL)
	if rtl.Direction() != DirRTL || (Env{}).Direction() != DirLTR || DirRTL.String() != "rtl" {
		t.Fatal("Env.Direction does not follow DirectionKey")
	}
	row := Row(Box().Size(10, 10), Box().Size(20, 10))
	size := row.Layout(Loose(Sz(100, 100)), rtl)
	if row.rects[0].Origin.X != size.W-10 || row.rects[1].Origin.X != 0 {
		t.Fatalf("RTL row offsets %v in %v, want the first child rightmost", row.rects, size)
	}
	col := Column(Box().Size(10, 10), Box().Size(30, 10))
	col.Layout(Loose(Sz(100, 100)), rtl)
	if col.rects[0].Origin.X != 20 || col.rects[1].Origin.X != 0 {
		t.Fatalf("RTL column offsets %v, want AlignStart on the right", col.rects)
	}
	ltr := Row(Box().Size(10, 10), Box().Size(20, 10))
	ltr.Layout(Loose(Sz(100, 100)), Env{})
	if ltr.rects[0].Origin.X != 0 {
		t.Fatalf("LTR row moved: %v", ltr.rects)
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
	if ps[0].RTL || w.lines[0][ps[0].Start:ps[0].End] != "world" {
		t.Fatalf("pieces %+v, want world on the left", ps)
	}

	r := RichText(Span("ab"))
	r.Layout(Tight(Sz(200, 20)), rtl)
	var box Rect
	r.para.Rects(0, 2, 200, r.alignment(), func(b Rect) { box = b })
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
