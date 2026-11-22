package widget

import (
	"github.com/ironpark/ggui"
	"testing"
	"time"
)

func TestPopupPaintsContentAboveAndClosesOnOutsidePress(t *testing.T) {
	t.Parallel()
	var contentRect ggui.Rect
	taps := 0
	content := Box(probe(80, 40, &contentRect)).Fill(nil)
	anchor := Tap(Box().Size(60, 20), func() { taps++ })
	p := Popup(anchor, content)
	tree := Column(Box().Size(10, 100), p)
	pr := ggui.NewProbe(tree, ggui.Sz(300, 300))
	pr.Frame()
	if contentRect != (ggui.Rect{}) {
		t.Fatal("content painted while closed")
	}
	p.Show()
	pr.Frame()
	// The anchor is at the window's left edge, which the content keeps
	// PopupCollisionPadding from.
	if contentRect != ggui.Rct(ggui.Pt(5, 124), ggui.Sz(80, 40)) {
		t.Fatalf("content at %+v, want below the anchor at (5,124) 80x40", contentRect)
	}
	// A press inside the content stays there; one outside closes and is
	// swallowed, so the anchor's tap does not fire.
	pr.Click(ggui.Pt(40, 140))
	if !p.IsOpen() {
		t.Fatal("a click inside the content closed the popup")
	}
	pr.Click(ggui.Pt(30, 110))
	if p.IsOpen() || taps != 0 {
		t.Fatalf("open %v taps %d after a click on the anchor under the scrim; want closed and 0", p.IsOpen(), taps)
	}
	pr.Click(ggui.Pt(30, 110))
	if taps != 1 {
		t.Fatalf("taps = %d after a click on the anchor with the popup closed, want 1", taps)
	}
}

func TestPopupFlipsAboveWhenNoRoomBelow(t *testing.T) {
	t.Parallel()
	var contentRect ggui.Rect
	p := Popup(Box().Size(60, 20), probe(80, 100, &contentRect))
	tree := Column(Box().Size(10, 250), p)
	pr := ggui.NewProbe(tree, ggui.Sz(300, 300))
	p.Show()
	pr.Frame()
	if contentRect.Origin.Y != 250-4-100 {
		t.Fatalf("content at %+v, want above the anchor", contentRect)
	}
	if got, _ := PopupOf(p.env); got != p {
		t.Fatal("PopupOf did not find the popup in the content's env")
	}
}

func TestPopupOpenDetachesItsBindingAndOnCloseFiresOnce(t *testing.T) {
	t.Parallel()
	closes := 0
	open := ggui.State(false)
	p := Popup(Box().Size(60, 20), Box().Size(80, 40)).BindOpen(open).OnClose(func() { closes++ })
	pr := ggui.NewProbe(p, ggui.Sz(300, 300))
	defer pr.Close()
	pr.Frame()
	open.Set(true)
	if !p.IsOpen() {
		t.Fatal("writing the bound state did not open the popup")
	}
	p.Hide()
	if ggui.Untrack(open.Get) || closes != 1 {
		t.Fatalf("Hide left the binding %v and fired OnClose %d times; want false and 1", ggui.Untrack(open.Get), closes)
	}
	p.Toggle()
	if !ggui.Untrack(open.Get) {
		t.Fatal("Toggle on a closed popup did not write the binding open")
	}
	// Open detaches: the popup closes, and the state it was bound to stays.
	p.Open(false)
	if p.IsOpen() || !ggui.Untrack(open.Get) || closes != 2 {
		t.Fatalf("Open(false): open %v, binding %v, closes %d; want closed, binding untouched, 2", p.IsOpen(), ggui.Untrack(open.Get), closes)
	}
	p.Open(false)
	open.Set(false)
	if closes != 2 {
		t.Fatalf("closing a closed popup, or writing a detached binding, fired OnClose: %d", closes)
	}
	p.Toggle()
	if !p.IsOpen() || ggui.Untrack(open.Get) {
		t.Fatalf("Toggle after Open: open %v, old binding %v; want open and the binding left alone", p.IsOpen(), ggui.Untrack(open.Get))
	}
}

func TestPopupGapSetsTheDistanceFromTheAnchor(t *testing.T) {
	t.Parallel()
	var content ggui.Rect
	p := Popup(Box().Size(60, 20), probe(80, 40, &content)).Gap(10)
	pr := ggui.NewProbe(Column(Box().Size(10, 30), p), ggui.Sz(300, 300))
	defer pr.Close()
	p.Show()
	pr.Frame()
	if content.Origin.Y != 30+20+10 || p.Rect() != content {
		t.Fatalf("content at %+v, Rect %+v; want it 10 below the anchor's bottom at 50", content, p.Rect())
	}
}

func TestPopupShowAtPlacesContentAtAPointUntilAnchored(t *testing.T) {
	t.Parallel()
	var content ggui.Rect
	p := Popup(Box().Size(60, 20), probe(80, 40, &content))
	pr := ggui.NewProbe(Column(p), ggui.Sz(300, 200))
	defer pr.Close()
	p.ShowAt(ggui.Pt(100, 50))
	pr.Frame()
	if content != ggui.Rct(ggui.Pt(100, 54), ggui.Sz(80, 40)) {
		t.Fatalf("ShowAt(100, 50) painted the content at %+v, want the gap below the point at (100,54)", content)
	}
	// A point near the edge keeps the content on screen, the collision
	// padding inside it.
	p.ShowAt(ggui.Pt(290, 50))
	pr.Frame()
	if content.Origin.X != 300-80-PopupCollisionPadding {
		t.Fatalf("ShowAt near the right edge put the content at x=%v, want 215", content.Origin.X)
	}
	p.AnchorPosition()
	pr.Frame()
	if content.Origin != ggui.Pt(PopupCollisionPadding, 24) {
		t.Fatalf("after AnchorPosition the content is at %v, want below the anchor at (5,24)", content.Origin)
	}
}

func TestPopupEscapeClosesAndReturnsFocus(t *testing.T) {
	t.Parallel()
	var openerFocused, insideFocused bool
	opener := Focus(Box().Size(60, 20)).OnFocus(func(v bool) { openerFocused = v })
	inside := Focus(Box().Size(80, 20)).OnFocus(func(v bool) { insideFocused = v })
	p := Popup(opener, Column(inside, Focus(Box().Size(80, 20))))
	pr := ggui.NewProbe(Column(p), ggui.Sz(300, 300))
	defer pr.Close()
	pr.Click(ggui.Pt(30, 10))
	if !openerFocused {
		t.Fatal("a click did not focus the opener")
	}
	p.Show()
	pr.Move(ggui.Pt(290, 290)) // the trap is noticed with the next input
	if !insideFocused || openerFocused {
		t.Fatalf("opening the popup: inside focused %v, opener %v; want focus moved into the popup", insideFocused, openerFocused)
	}
	pr.Key("tab", "tab")
	if !insideFocused {
		t.Fatal("Tab left the popup's two controls")
	}
	pr.Key("escape")
	pr.Move(ggui.Pt(290, 290))
	if p.IsOpen() || !openerFocused {
		t.Fatalf("after Escape: open %v, opener focused %v; want closed with focus back on the opener", p.IsOpen(), openerFocused)
	}
}

func TestPopupKeyKeepsItOpenWhenMoved(t *testing.T) {
	t.Parallel()
	moved := ggui.State(false)
	var p *PopupWidget
	pr := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Reactive(func() ggui.Widget {
			p = Popup(Box().Size(60, 20), Box().Size(80, 40)).Key("menu")
			if moved.Get() {
				return Column(Box().Size(10, 50), p)
			}
			return Column(p)
		})
	}, ggui.Sz(300, 300))
	defer pr.Close()
	pr.Frame()
	p.Show()
	pr.Frame()
	moved.Set(true)
	pr.Frame()
	if !p.IsOpen() {
		t.Fatal("a rebuilt popup with the same key at another place closed")
	}
}

func TestPopupFadesOutInertThenStopsPainting(t *testing.T) {
	t.Parallel()
	paints, taps := 0, 0
	content := ggui.FromFuncs(
		func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(80, 40)) },
		func(*ggui.Canvas, ggui.Rect) { paints++ },
	)
	p := Popup(Box().Size(60, 20), Tap(content, func() { taps++ }))
	pr := ggui.NewProbe(p, ggui.Sz(300, 300))
	defer pr.Close()
	pr.Advance(0)
	p.Show()
	pr.Advance(time.Second)
	p.Hide()
	paints = 0
	pr.Advance(50 * time.Millisecond) // of the default 150ms
	if paints == 0 {
		t.Fatal("a closing popup stopped painting before its animation ended")
	}
	pr.Click(ggui.Pt(20, 40))
	if taps != 0 {
		t.Fatalf("a click on a closing popup's content tapped it %d times; it is inert", taps)
	}
	pr.Advance(time.Second)
	paints = 0
	pr.Frame()
	if paints != 0 {
		t.Fatal("a closed popup still paints after its animation")
	}
}

func TestPopupTrapsTabAndRestoresFocus(t *testing.T) {
	t.Parallel()
	focused := ""
	track := func(name string, size float64) *FocusWidget {
		return Focus(Box().Size(size, size)).OnFocus(func(on bool) {
			if on {
				focused = name
			} else if focused == name {
				focused = ""
			}
		})
	}
	a, b := track("a", 20), track("b", 20)
	c1, c2 := track("c1", 20), track("c2", 20)
	pop := Popup(a, Column(c1, c2))
	p := ggui.NewProbe(Column(pop, b), ggui.Sz(100, 200))
	defer p.Close()
	p.Click(ggui.Pt(10, 10))
	if focused != "a" {
		t.Fatalf("focused %q after clicking a", focused)
	}
	pop.Show()
	p.Frame()
	p.Type(ggui.Mods{}, ggui.KeyTab)
	if focused != "c2" {
		t.Fatalf("focused %q; the scope should take focus to c1 and Tab move it to c2", focused)
	}
	p.Type(ggui.Mods{}, ggui.KeyTab)
	if focused != "c1" {
		t.Fatalf("focused %q; Tab must wrap inside the popup, not reach b", focused)
	}
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	if pop.IsOpen() {
		t.Fatal("Escape did not close the popup")
	}
	p.Move(ggui.Pt(90, 190)) // the next frame's dispatch restores focus
	if focused != "a" {
		t.Fatalf("focused %q after closing, want a", focused)
	}
}

func TestPopupFocusContentAdoptPointAndNonmodalScrim(t *testing.T) {
	t.Parallel()
	// 1. FocusContent must not reset paintedOpen or drop opener.
	p := Popup(Box().Size(40, 20), Box().Size(40, 20))
	p.paintedOpen = true
	p.FocusContent()
	if !p.paintedOpen || !p.focusContent {
		t.Fatalf("FocusContent: paintedOpen=%v focusContent=%v, want true, true", p.paintedOpen, p.focusContent)
	}

	// 2. Adopt copies point even when BindOpen is set.
	open := ggui.State(true)
	pt := ggui.Pt(42, 77)
	prev := Popup(Box().Size(40, 20), Box().Size(40, 20)).BindOpen(open)
	prev.ShowAt(pt)
	next := Popup(Box().Size(40, 20), Box().Size(40, 20)).BindOpen(open)
	next.Adopt(prev)
	if next.point == nil || *next.point != pt {
		t.Fatalf("Adopt lost ShowAt point with BindOpen: %v", next.point)
	}

	// 3. popupScrim passes through PointerUp/PointerTap when nonmodal, blocks when modal.
	modal := popupScrim{Popup(Box(), Box())}
	nonmodal := popupScrim{Popup(Box(), Box()).Modal(false)}
	if !modal.HandlePointer(ggui.PointerEvent{Kind: ggui.PointerUp}) || !modal.HandlePointer(ggui.PointerEvent{Kind: ggui.PointerTap}) {
		t.Fatal("modal scrim failed to consume PointerUp/PointerTap")
	}
	if nonmodal.HandlePointer(ggui.PointerEvent{Kind: ggui.PointerUp}) || nonmodal.HandlePointer(ggui.PointerEvent{Kind: ggui.PointerTap}) {
		t.Fatal("nonmodal scrim consumed PointerUp/PointerTap")
	}
}
