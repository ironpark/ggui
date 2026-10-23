package ggui

import "testing"

func TestSelectableRichTextLinkStillTaps(t *testing.T) {
	t.Parallel()
	taps := 0
	p := NewProbe(RichText(Span("Read the "), Span("guide").OnTap(func() { taps++ }), Span(" first.")).Selectable(), Sz(400, 40))
	defer p.Close()
	link, ok := p.FindRole(RoleLink, "guide")
	if !ok {
		t.Fatal("no link")
	}
	p.Move(link.Center())
	if c := p.Cursor(); c != CursorShapePointer {
		t.Errorf("cursor %v", c)
	}
	p.Click(link.Center())
	if taps != 1 {
		t.Fatalf("taps %d", taps)
	}
}

func TestRichTextRelayoutsWhenASpanResizes(t *testing.T) {
	t.Parallel()
	w := RichText(Span("abc").Size(10))
	a := w.Layout(Loose(Sz(1000, 1000)), Env{})
	w.Spans(Span("abc").Size(30))
	b := w.Layout(Loose(Sz(1000, 1000)), Env{})
	if b.W <= a.W || b.H <= a.H {
		t.Fatalf("size 10 %v, size 30 %v", a, b)
	}
}

func TestSelectableRTLWordsHitWhereTheyAreDrawn(t *testing.T) {
	t.Parallel()
	w := RichText(Span("שלום עולם")).Selectable()
	p := NewProbe(Directionality(DirRTL, w), Sz(300, 30))
	defer p.Close()
	p.Frame()
	node, ok := p.Semantics().Find(RoleText, "שלום עולם")
	if !ok {
		t.Fatal("no node")
	}
	r := node.Rect
	at := Pt(r.Origin.X+r.Size.W-3, r.Origin.Y+r.Size.H/2)
	p.Click(at)
	p.Click(at)
	if got := w.SelectedText(); got != "שלום" {
		t.Fatalf("double click at the right selected %q", got)
	}
	left := Pt(r.Origin.X+r.Size.W-w.para.lines[0].width+3, r.Origin.Y+r.Size.H/2)
	p.Frame()
	p.Click(left)
	p.Click(left)
	if got := w.SelectedText(); got != "עולם" {
		t.Fatalf("double click at the left selected %q", got)
	}
}
