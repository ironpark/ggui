package ui

import (
	"fmt"
	"testing"

	"github.com/ironpark/ggui"
)

// A list with more options than fit shows an arrow at the edge there is
// more beyond, shadcn's SelectScrollDownButton and SelectScrollUpButton,
// and scrolls a row every 40ms while the pointer moved onto one stays.
func TestSelectScrollArrowsScrollWhileHovered(t *testing.T) {
	t.Parallel()
	options := make([]string, 40)
	for i := range options {
		options[i] = fmt.Sprintf("Option %02d", i)
	}
	value := ggui.State(options[0])
	s := Select(value).Options(options).Name("Pick").AlignItemWithTrigger(false)
	p := ggui.NewProbe(ggui.Column(s).Align(ggui.AlignStart), ggui.Sz(320, 300))
	defer p.Close()
	p.Tap("Pick")
	p.Advance(0)
	shown := p.FindAll(ggui.RoleOption)
	if len(shown) == 0 || len(shown) >= len(options) {
		t.Fatalf("%d options shown of %d, want a list that scrolls", len(shown), len(options))
	}
	top, bottom := shown[0].Rect, shown[len(shown)-1].Rect
	offset := func() float64 { return ggui.Untrack(s.viewport.offset.Get) }
	if s.viewport.up.shown || !s.viewport.down.shown {
		t.Fatalf("up=%v down=%v at the top of the list, want only the down arrow", s.viewport.up.shown, s.viewport.down.shown)
	}
	// The pointer at rest where the arrow appears scrolls nothing.
	on := ggui.Pt(bottom.Center().X, bottom.Origin.Y+bottom.Size.H-4)
	p.Move(on)
	p.Advance(selectArrowPace * 3)
	if offset() != 0 {
		t.Fatalf("the list scrolled to %v under a pointer that only arrived", offset())
	}
	p.Move(on.Add(ggui.Pt(1, 0)))
	if s.highlight != -1 {
		t.Fatalf("highlight = %d under the arrow, want none", s.highlight)
	}
	p.Advance(selectArrowPace)
	p.Advance(0)
	first := offset()
	if first <= 0 {
		t.Fatal("the list did not scroll a pace after the pointer moved on the down arrow")
	}
	if !s.viewport.up.shown {
		t.Fatal("no up arrow once the list has scrolled")
	}
	p.Advance(selectArrowPace)
	p.Advance(0)
	if got := offset(); got <= first {
		t.Fatalf("the list stayed at %v a pace later, want it further", got)
	}
	// Leaving the arrow stops it.
	p.Move(ggui.Pt(300, 290))
	at := offset()
	p.Advance(selectArrowPace * 4)
	if offset() != at {
		t.Fatalf("the list went on to %v after the pointer left the arrow at %v", offset(), at)
	}
	// The up arrow takes it back to the start, and goes.
	up := ggui.Pt(top.Center().X, top.Origin.Y+2)
	p.Move(up)
	p.Move(up.Add(ggui.Pt(1, 0)))
	for range 60 {
		p.Advance(selectArrowPace)
	}
	if offset() != 0 || s.viewport.up.shown {
		t.Fatalf("offset=%v up=%v after resting on the up arrow, want the start and no arrow", offset(), s.viewport.up.shown)
	}
}

// The keyboard's row is shown clear of the arrows.
func TestSelectKeyboardRowClearsTheScrollArrow(t *testing.T) {
	t.Parallel()
	options := make([]string, 40)
	for i := range options {
		options[i] = fmt.Sprintf("Option %02d", i)
	}
	s := Select(ggui.State(options[0])).Options(options).Name("Pick").AlignItemWithTrigger(false)
	p := ggui.NewProbe(ggui.Column(s).Align(ggui.AlignStart), ggui.Sz(320, 300))
	defer p.Close()
	p.Key("tab")
	p.Key("enter")
	p.Advance(0)
	shown := len(p.FindAll(ggui.RoleOption))
	for range shown + 2 {
		p.Key("down")
	}
	p.Advance(0)
	var row, last ggui.Rect
	for _, f := range p.FindAll(ggui.RoleOption) {
		if f.Label == options[s.highlight] {
			row = f.Rect
		}
		last = f.Rect
	}
	th := s.theme
	edge := last.Origin.Y + last.Size.H + th.PanelPad.Bottom
	if row.Empty() || row.Origin.Y+row.Size.H > edge-selectArrowHeight(th)+.5 {
		t.Fatalf("the highlighted row %v runs under the down arrow, which starts at %v", row, edge-selectArrowHeight(th))
	}
}
