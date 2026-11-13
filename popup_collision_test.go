package ggui

import "testing"

// A popup keeps PopupCollisionPadding from the edges of the window, as a
// Base UI positioner keeps its collisionPadding of 5: it is moved along the
// anchor's side to stay that far inside. sidePopup's anchor is 60x20, its
// content 100x50 and its window 400x300.
func TestPopupKeepsTheCollisionPaddingFromEveryEdge(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		at    Point
		side  PopupSide
		align PopupAlignment
		want  Point
	}{
		{"below, at the left edge", Pt(0, 100), PopupBottom, PopupStart, Pt(5, 124)},
		{"below, its end at the right edge", Pt(340, 100), PopupBottom, PopupEnd, Pt(400-5-100, 124)},
		{"beside, at the top edge", Pt(100, 0), PopupRight, PopupStart, Pt(164, 5)},
		{"beside, at the bottom edge", Pt(100, 280), PopupRight, PopupStart, Pt(164, 300-5-50)},
	} {
		p := sidePopup(t, tt.at, func(p *PopupWidget) { p.Side(tt.side).Align(tt.align) })
		if got := p.Rect().Origin; got != tt.want {
			t.Errorf("%s: content at %v, want %v", tt.name, got, tt.want)
		}
	}
	// CollisionPadding(0) lets it sit flush against the edge.
	p := sidePopup(t, Pt(0, 100), func(p *PopupWidget) { p.CollisionPadding(0) })
	if got := p.Rect().Origin; got != Pt(0, 124) {
		t.Errorf("with no collision padding the content is at %v, want flush at (0,124)", got)
	}
}

// The room beside the anchor ends the collision padding short of the edge,
// so content that would only fit flush against it goes to the other side.
func TestPopupFlipsWhenItWouldComeWithinTheCollisionPadding(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		at   Point
		side PopupSide
		on   PopupSide
	}{
		// 300 - (224+20+4) leaves 52 below, 47 of them inside the padding.
		{"below", Pt(150, 224), PopupBottom, PopupTop},
		// 400 - (234+60+4) leaves 102 to the right, 97 inside the padding.
		{"right", Pt(234, 100), PopupRight, PopupLeft},
	} {
		p := sidePopup(t, tt.at, func(p *PopupWidget) { p.Side(tt.side) })
		if p.PlacedSide() != tt.on {
			t.Errorf("%s: content on side %v at %v, want it flipped to %v", tt.name, p.PlacedSide(), p.Rect(), tt.on)
		}
		flush := sidePopup(t, tt.at, func(p *PopupWidget) { p.Side(tt.side).CollisionPadding(0) })
		if flush.PlacedSide() != tt.side {
			t.Errorf("%s: with no collision padding the content is on side %v, want it kept on %v", tt.name, flush.PlacedSide(), tt.side)
		}
	}
}

// Content taller than the room on either side is held to the room that
// ends the collision padding inside the window.
func TestPopupIsHeldToTheRoomInsideTheCollisionPadding(t *testing.T) {
	t.Parallel()
	var content Rect
	p := Popup(Box().Size(60, 20), probe(80, 1000, &content))
	pr := NewProbe(Row(Padding(p, 100, 0, 0, 150)).Align(AlignStart), Sz(400, 300))
	defer pr.Close()
	p.Show()
	pr.Frame()
	if want := Rct(Pt(150, 124), Sz(80, 300-5-124)); content != want {
		t.Fatalf("content at %v, want %v: from the gap under the anchor to 5px above the window's bottom", content, want)
	}
}
