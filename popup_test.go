package ggui

import "testing"

func TestPopupPaintsContentAboveAndClosesOnOutsidePress(t *testing.T) {
	var contentRect Rect
	taps := 0
	content := Box(probe(80, 40, &contentRect)).Fill(nil)
	anchor := Tap(Box().Size(60, 20), func() { taps++ })
	p := Popup(anchor, content)
	tree := Column(Box().Size(10, 100), p)
	pr := NewProbe(tree, Sz(300, 300))
	pr.Frame()
	if contentRect != (Rect{}) {
		t.Fatal("content painted while closed")
	}
	p.Show()
	pr.Frame()
	if contentRect != Rct(Pt(0, 124), Sz(80, 40)) {
		t.Fatalf("content at %+v, want below the anchor at (0,124) 80x40", contentRect)
	}
	// A press inside the content stays there; one outside closes and is
	// swallowed, so the anchor's tap does not fire.
	pr.Click(Pt(40, 140))
	if !p.IsOpen() {
		t.Fatal("a click inside the content closed the popup")
	}
	pr.Click(Pt(30, 110))
	if p.IsOpen() || taps != 0 {
		t.Fatalf("open %v taps %d after a click on the anchor under the scrim; want closed and 0", p.IsOpen(), taps)
	}
	pr.Click(Pt(30, 110))
	if taps != 1 {
		t.Fatalf("taps = %d after a click on the anchor with the popup closed, want 1", taps)
	}
}

func TestPopupFlipsAboveWhenNoRoomBelow(t *testing.T) {
	var contentRect Rect
	p := Popup(Box().Size(60, 20), probe(80, 100, &contentRect))
	tree := Column(Box().Size(10, 250), p)
	pr := NewProbe(tree, Sz(300, 300))
	p.Show()
	pr.Frame()
	if contentRect.Origin.Y != 250-4-100 {
		t.Fatalf("content at %+v, want above the anchor", contentRect)
	}
	if got, _ := PopupOf(p.env); got != p {
		t.Fatal("PopupOf did not find the popup in the content's env")
	}
}
