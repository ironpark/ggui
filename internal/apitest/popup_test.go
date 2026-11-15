package apitest_test

import (
	"testing"

	"github.com/ironpark/ggui"
)

// sidePopup opens a 100x50 popup on a 60x20 anchor at at, in a 400x300
// window.
func sidePopup(t *testing.T, at ggui.Point, set func(*ggui.PopupWidget)) *ggui.PopupWidget {
	t.Helper()
	p := ggui.Popup(ggui.Box().Size(60, 20), ggui.Box().Size(100, 50))
	set(p)
	pr := ggui.NewProbe(ggui.Row(ggui.Padding(p, at.Y, 0, 0, at.X)).Align(ggui.AlignStart), ggui.Sz(400, 300))
	t.Cleanup(pr.Close)
	p.Show()
	pr.Frame()
	return p
}

func TestPopupSidePlacesContentAroundTheAnchor(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		side ggui.PopupSide
		want ggui.Rect
		on   ggui.PopupSide
	}{
		{"bottom", ggui.PopupBottom, ggui.Rct(ggui.Pt(150, 124), ggui.Sz(100, 50)), ggui.PopupBottom},
		{"top", ggui.PopupTop, ggui.Rct(ggui.Pt(150, 46), ggui.Sz(100, 50)), ggui.PopupTop},
		{"right", ggui.PopupRight, ggui.Rct(ggui.Pt(214, 100), ggui.Sz(100, 50)), ggui.PopupRight},
		{"left", ggui.PopupLeft, ggui.Rct(ggui.Pt(46, 100), ggui.Sz(100, 50)), ggui.PopupLeft},
		{"inline end", ggui.PopupInlineEnd, ggui.Rct(ggui.Pt(214, 100), ggui.Sz(100, 50)), ggui.PopupRight},
		{"inline start", ggui.PopupInlineStart, ggui.Rct(ggui.Pt(46, 100), ggui.Sz(100, 50)), ggui.PopupLeft},
	} {
		p := sidePopup(t, ggui.Pt(150, 100), func(p *ggui.PopupWidget) { p.Side(tt.side) })
		if p.Rect() != tt.want || p.PlacedSide() != tt.on {
			t.Errorf("%s: content at %v on side %v, want %v on %v", tt.name, p.Rect(), p.PlacedSide(), tt.want, tt.on)
		}
	}
}

func TestPopupInlineSidesFollowTheWritingDirection(t *testing.T) {
	t.Parallel()
	// The anchor records the Rect it is painted in, which is the popup's.
	var anchor ggui.Rect
	p := ggui.Popup(ggui.Box(probe(60, 20, &anchor)).Size(60, 20), ggui.Box().Size(100, 50)).Side(ggui.PopupInlineEnd)
	pr := ggui.NewProbe(ggui.Provide(ggui.DirectionKey, ggui.DirRTL, ggui.Center(p)), ggui.Sz(400, 300))
	defer pr.Close()
	p.Show()
	pr.Frame()
	if p.PlacedSide() != ggui.PopupLeft || p.Rect().Origin.X+p.Rect().Size.W != anchor.Origin.X-4 {
		t.Fatalf("inline end under RTL: content at %v on side %v beside anchor %v", p.Rect(), p.PlacedSide(), anchor)
	}
}

func TestPopupSideFlipsWhenOnlyTheOppositeSideHasRoom(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		at   ggui.Point
		side ggui.PopupSide
		on   ggui.PopupSide
	}{
		{"top at the top edge", ggui.Pt(150, 10), ggui.PopupTop, ggui.PopupBottom},
		{"bottom at the bottom edge", ggui.Pt(150, 270), ggui.PopupBottom, ggui.PopupTop},
		{"left at the left edge", ggui.Pt(10, 100), ggui.PopupLeft, ggui.PopupRight},
		{"right at the right edge", ggui.Pt(330, 100), ggui.PopupRight, ggui.PopupLeft},
	} {
		p := sidePopup(t, tt.at, func(p *ggui.PopupWidget) { p.Side(tt.side) })
		r := p.Rect()
		if p.PlacedSide() != tt.on || r.Origin.X < 0 || r.Origin.Y < 0 || r.Origin.X+r.Size.W > 400 || r.Origin.Y+r.Size.H > 300 {
			t.Errorf("%s: content at %v on side %v, want it whole on side %v", tt.name, r, p.PlacedSide(), tt.on)
		}
	}
}

func TestPopupAlignAndOffsetAlongASide(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		side   ggui.PopupSide
		align  ggui.PopupAlignment
		offset float64
		want   ggui.Point
	}{
		{"below, start, overhanging", ggui.PopupBottom, ggui.PopupStart, -4, ggui.Pt(146, 124)},
		{"below, end, overhanging", ggui.PopupBottom, ggui.PopupEnd, -4, ggui.Pt(114, 124)},
		{"below, center, shifted", ggui.PopupBottom, ggui.PopupCenter, 10, ggui.Pt(140, 124)},
		{"right, start", ggui.PopupRight, ggui.PopupStart, 0, ggui.Pt(214, 100)},
		{"right, start, raised", ggui.PopupRight, ggui.PopupStart, -3, ggui.Pt(214, 97)},
		{"right, center", ggui.PopupRight, ggui.PopupCenter, 0, ggui.Pt(214, 85)},
		{"right, end", ggui.PopupRight, ggui.PopupEnd, 0, ggui.Pt(214, 70)},
		{"right, end, lowered", ggui.PopupRight, ggui.PopupEnd, -3, ggui.Pt(214, 73)},
	} {
		p := sidePopup(t, ggui.Pt(150, 100), func(p *ggui.PopupWidget) { p.Side(tt.side).Align(tt.align).AlignOffset(tt.offset) })
		if p.Rect().Origin != tt.want {
			t.Errorf("%s: content at %v, want %v", tt.name, p.Rect().Origin, tt.want)
		}
	}
}

// A popup keeps PopupCollisionPadding from the edges of the window, as a
// Base UI positioner keeps its collisionPadding of 5: it is moved along the
// anchor's side to stay that far inside. sidePopup's anchor is 60x20, its
// content 100x50 and its window 400x300.
func TestPopupKeepsTheCollisionPaddingFromEveryEdge(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		at    ggui.Point
		side  ggui.PopupSide
		align ggui.PopupAlignment
		want  ggui.Point
	}{
		{"below, at the left edge", ggui.Pt(0, 100), ggui.PopupBottom, ggui.PopupStart, ggui.Pt(5, 124)},
		{"below, its end at the right edge", ggui.Pt(340, 100), ggui.PopupBottom, ggui.PopupEnd, ggui.Pt(400-5-100, 124)},
		{"beside, at the top edge", ggui.Pt(100, 0), ggui.PopupRight, ggui.PopupStart, ggui.Pt(164, 5)},
		{"beside, at the bottom edge", ggui.Pt(100, 280), ggui.PopupRight, ggui.PopupStart, ggui.Pt(164, 300-5-50)},
	} {
		p := sidePopup(t, tt.at, func(p *ggui.PopupWidget) { p.Side(tt.side).Align(tt.align) })
		if got := p.Rect().Origin; got != tt.want {
			t.Errorf("%s: content at %v, want %v", tt.name, got, tt.want)
		}
	}
	// CollisionPadding(0) lets it sit flush against the edge.
	p := sidePopup(t, ggui.Pt(0, 100), func(p *ggui.PopupWidget) { p.CollisionPadding(0) })
	if got := p.Rect().Origin; got != ggui.Pt(0, 124) {
		t.Errorf("with no collision padding the content is at %v, want flush at (0,124)", got)
	}
}

// The room beside the anchor ends the collision padding short of the edge,
// so content that would only fit flush against it goes to the other side.
func TestPopupFlipsWhenItWouldComeWithinTheCollisionPadding(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		at   ggui.Point
		side ggui.PopupSide
		on   ggui.PopupSide
	}{
		// 300 - (224+20+4) leaves 52 below, 47 of them inside the padding.
		{"below", ggui.Pt(150, 224), ggui.PopupBottom, ggui.PopupTop},
		// 400 - (234+60+4) leaves 102 to the right, 97 inside the padding.
		{"right", ggui.Pt(234, 100), ggui.PopupRight, ggui.PopupLeft},
	} {
		p := sidePopup(t, tt.at, func(p *ggui.PopupWidget) { p.Side(tt.side) })
		if p.PlacedSide() != tt.on {
			t.Errorf("%s: content on side %v at %v, want it flipped to %v", tt.name, p.PlacedSide(), p.Rect(), tt.on)
		}
		flush := sidePopup(t, tt.at, func(p *ggui.PopupWidget) { p.Side(tt.side).CollisionPadding(0) })
		if flush.PlacedSide() != tt.side {
			t.Errorf("%s: with no collision padding the content is on side %v, want it kept on %v", tt.name, flush.PlacedSide(), tt.side)
		}
	}
}

// Content taller than the room on either side is held to the room that
// ends the collision padding inside the window.
func TestPopupIsHeldToTheRoomInsideTheCollisionPadding(t *testing.T) {
	t.Parallel()
	var content ggui.Rect
	p := ggui.Popup(ggui.Box().Size(60, 20), probe(80, 1000, &content))
	pr := ggui.NewProbe(ggui.Row(ggui.Padding(p, 100, 0, 0, 150)).Align(ggui.AlignStart), ggui.Sz(400, 300))
	defer pr.Close()
	p.Show()
	pr.Frame()
	if want := ggui.Rct(ggui.Pt(150, 124), ggui.Sz(80, 300-5-124)); content != want {
		t.Fatalf("content at %v, want %v: from the gap under the anchor to 5px above the window's bottom", content, want)
	}
}

func TestPopupLogicalAlignment(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		align ggui.PopupAlignment
		dir   ggui.TextDirection
		x     float64
	}{
		{ggui.PopupStart, ggui.DirLTR, 0}, {ggui.PopupCenter, ggui.DirLTR, -20}, {ggui.PopupEnd, ggui.DirLTR, -40},
		{ggui.PopupStart, ggui.DirRTL, -40}, {ggui.PopupCenter, ggui.DirRTL, -20}, {ggui.PopupEnd, ggui.DirRTL, 0},
	} {
		var anchor ggui.Rect
		p := ggui.Popup(ggui.Box(probe(60, 20, &anchor)).Size(60, 20), ggui.Box().Size(100, 50)).Align(tt.align)
		pr := ggui.NewProbe(ggui.Provide(ggui.DirectionKey, tt.dir, ggui.Column(ggui.Padding(p, 0, 100))), ggui.Sz(400, 250))
		p.Show()
		pr.Frame()
		want := anchor.Origin.X + tt.x
		if p.Rect().Origin.X != want {
			t.Errorf("align=%v dir=%v rect=%v want x=%v", tt.align, tt.dir, p.Rect(), want)
		}
		pr.Close()
	}
}

func TestPopupRejectedControlledCloseDoesNotNotify(t *testing.T) {
	t.Parallel()
	requests, closes := 0, 0
	p := ggui.Popup(ggui.Box().Size(50, 20), ggui.Box().Size(100, 100)).Modal(false).BindOpen(ggui.Controlled(true, func(bool) { requests++ })).OnClose(func() { closes++ })
	pr := ggui.NewProbe(p, ggui.Sz(300, 250))
	defer pr.Close()
	pr.Frame()
	p.Hide()
	if !p.IsOpen() || requests != 1 || closes != 0 {
		t.Fatalf("rejected close: open=%v requests=%d closes=%d", p.IsOpen(), requests, closes)
	}
}

// A nonmodal popup that does not focus on open leaves the keyboard in the
// editor that opened it, as a combobox's list leaves it in its input.
func TestNonmodalPopupFocusOnOpenFalseKeepsTheOpenersFocus(t *testing.T) {
	t.Parallel()
	for _, keep := range []bool{false, true} {
		input := ggui.TextInput(ggui.State("")).Name("Query")
		item := ggui.TextInput(ggui.State("")).Name("Item")
		p := ggui.Popup(input, ggui.Box(item).Size(120, 40)).Modal(false).FocusOnOpen(!keep)
		pr := ggui.NewProbe(ggui.Column(p), ggui.Sz(300, 250))
		pr.Tap("Query")
		p.Show()
		pr.Frame()
		pr.Frame()
		focused, _ := pr.Semantics().Focused()
		want := "Item"
		if keep {
			want = "Query"
		}
		if focused.Name != want {
			t.Errorf("FocusOnOpen(%v): %q has focus, want %q", !keep, focused.Name, want)
		}
		pr.Close()
	}
}
