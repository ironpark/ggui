package ggui

import (
	"testing"
	"time"
)

// These tests check that the parity tracks compose: text under paint
// effects, animation values driving paint effects, and selectable text
// inside the scroll track's sticky headers.

func TestRichTextLinkTapsUnderATransform(t *testing.T) {
	t.Parallel()
	taps := 0
	text := RichText(Span("Read the "), Span("guide").OnTap(func() { taps++ }), Span(" first.")).Selectable()
	p := NewProbe(topLeft(Transform(text).Translate(120, 40)), Sz(500, 200))
	defer p.Close()
	p.Frame()
	link, ok := p.FindRole(RoleLink, "guide")
	if !ok {
		t.Fatal("no link")
	}
	if link.Center().X < 120 || link.Center().Y < 40 {
		t.Fatalf("link reported at %v; want its moved on-screen box", link)
	}
	p.Click(link.Center())
	if taps != 1 {
		t.Fatalf("taps %d; want the moved link to take the click", taps)
	}
	if got := text.SelectedText(); got != "" {
		t.Fatalf("a link tap selected %q", got)
	}
}

func TestSelectableTextSelectsUnderAScale(t *testing.T) {
	t.Parallel()
	w := Text("hello world").Selectable()
	p := NewProbe(topLeft(Transform(w).Scale(2).Origin(0, 0)), Sz(600, 100))
	defer p.Close()
	p.Frame()
	local := textAt(w.rich, Rct(Pt(0, 0), Sz(300, 50)), 4) // late in "hello", where an unmapped 2x point lands in "world"
	at := Pt(local.X*2, local.Y*2)
	p.Click(at)
	p.Click(at)
	if got := w.rich.SelectedText(); got != "hello" {
		t.Fatalf("double click under a 2x scale selected %q, want hello", got)
	}
}

func TestTimelineDrivesATransformOffset(t *testing.T) {
	t.Parallel()
	activeWorld().anims.list = nil
	tl := Timeline(time.Second)
	tg := &target{size: Sz(20, 20)}
	w := Transform(tg).BindOffset(Interpolate(tl, Pt(0, 0), Pt(100, 0), LerpPoint)).Origin(0, 0)
	p := NewProbe(topLeft(w), Sz(300, 100))
	defer p.Close()
	p.Click(Pt(5, 5))
	if len(tg.taps()) != 1 {
		t.Fatal("the child at rest missed its click")
	}
	tl.Seek(0.5)
	p.Frame()
	p.Click(Pt(55, 5))
	if taps := tg.taps(); len(taps) != 2 || !nearPt(taps[1], Pt(5, 5)) {
		t.Fatalf("taps %v; want the second at (5, 5) after the timeline moved the child 50", taps)
	}
}

func TestSelectableTextInAPinnedStickyHeader(t *testing.T) {
	t.Parallel()
	head := Text("alpha beta").Selectable()
	var body Rect
	a := Sticky(head, probe(200, 300, &body))
	offset := State(0.0)
	p := NewProbe(Scroll(Column(a)).BindOffset(offset), Sz(200, 100))
	defer p.Close()
	p.Frame()
	offset.Set(120)
	p.Frame()
	if !a.Pinned() {
		t.Fatal("header did not pin")
	}
	at := textAt(head.rich, Rct(Pt(0, 0), Sz(200, 30)), 7) // inside "beta"
	p.Click(at)
	p.Click(at)
	if got := head.rich.SelectedText(); got != "beta" {
		t.Fatalf("double click on the pinned header selected %q, want beta", got)
	}
}
