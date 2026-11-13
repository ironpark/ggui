package ggui

import (
	"testing"
	"time"
)

// sidePopup opens a 100x50 popup on a 60x20 anchor at at, in a 400x300
// window.
func sidePopup(t *testing.T, at Point, set func(*PopupWidget)) *PopupWidget {
	t.Helper()
	p := Popup(Box().Size(60, 20), Box().Size(100, 50))
	set(p)
	pr := NewProbe(Row(Padding(p, at.Y, 0, 0, at.X)).Align(AlignStart), Sz(400, 300))
	t.Cleanup(pr.Close)
	p.Show()
	pr.Frame()
	return p
}

func TestPopupSidePlacesContentAroundTheAnchor(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		side PopupSide
		want Rect
		on   PopupSide
	}{
		{"bottom", PopupBottom, Rct(Pt(150, 124), Sz(100, 50)), PopupBottom},
		{"top", PopupTop, Rct(Pt(150, 46), Sz(100, 50)), PopupTop},
		{"right", PopupRight, Rct(Pt(214, 100), Sz(100, 50)), PopupRight},
		{"left", PopupLeft, Rct(Pt(46, 100), Sz(100, 50)), PopupLeft},
		{"inline end", PopupInlineEnd, Rct(Pt(214, 100), Sz(100, 50)), PopupRight},
		{"inline start", PopupInlineStart, Rct(Pt(46, 100), Sz(100, 50)), PopupLeft},
	} {
		p := sidePopup(t, Pt(150, 100), func(p *PopupWidget) { p.Side(tt.side) })
		if p.Rect() != tt.want || p.PlacedSide() != tt.on {
			t.Errorf("%s: content at %v on side %v, want %v on %v", tt.name, p.Rect(), p.PlacedSide(), tt.want, tt.on)
		}
	}
}

func TestPopupInlineSidesFollowTheWritingDirection(t *testing.T) {
	t.Parallel()
	p := Popup(Box().Size(60, 20), Box().Size(100, 50)).Side(PopupInlineEnd)
	pr := NewProbe(Provide(DirectionKey, DirRTL, Center(p)), Sz(400, 300))
	defer pr.Close()
	p.Show()
	pr.Frame()
	if p.PlacedSide() != PopupLeft || p.Rect().Origin.X+p.Rect().Size.W != p.anchorRect.Origin.X-4 {
		t.Fatalf("inline end under RTL: content at %v on side %v beside anchor %v", p.Rect(), p.PlacedSide(), p.anchorRect)
	}
}

func TestPopupSideFlipsWhenOnlyTheOppositeSideHasRoom(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		at   Point
		side PopupSide
		on   PopupSide
	}{
		{"top at the top edge", Pt(150, 10), PopupTop, PopupBottom},
		{"bottom at the bottom edge", Pt(150, 270), PopupBottom, PopupTop},
		{"left at the left edge", Pt(10, 100), PopupLeft, PopupRight},
		{"right at the right edge", Pt(330, 100), PopupRight, PopupLeft},
	} {
		p := sidePopup(t, tt.at, func(p *PopupWidget) { p.Side(tt.side) })
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
		side   PopupSide
		align  PopupAlignment
		offset float64
		want   Point
	}{
		{"below, start, overhanging", PopupBottom, PopupStart, -4, Pt(146, 124)},
		{"below, end, overhanging", PopupBottom, PopupEnd, -4, Pt(114, 124)},
		{"below, center, shifted", PopupBottom, PopupCenter, 10, Pt(140, 124)},
		{"right, start", PopupRight, PopupStart, 0, Pt(214, 100)},
		{"right, start, raised", PopupRight, PopupStart, -3, Pt(214, 97)},
		{"right, center", PopupRight, PopupCenter, 0, Pt(214, 85)},
		{"right, end", PopupRight, PopupEnd, 0, Pt(214, 70)},
		{"right, end, lowered", PopupRight, PopupEnd, -3, Pt(214, 73)},
	} {
		p := sidePopup(t, Pt(150, 100), func(p *PopupWidget) { p.Side(tt.side).Align(tt.align).AlignOffset(tt.offset) })
		if p.Rect().Origin != tt.want {
			t.Errorf("%s: content at %v, want %v", tt.name, p.Rect().Origin, tt.want)
		}
	}
}

func TestPopupEntranceGrowsFromTheAnchorSideWithoutMovingItsHits(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		side   PopupSide
		origin Point
		travel Point
	}{
		{PopupBottom, Pt(0, 0), Pt(0, -8)},
		{PopupTop, Pt(0, 1), Pt(0, 8)},
		{PopupRight, Pt(0, 0), Pt(-8, 0)},
		{PopupLeft, Pt(1, 0), Pt(8, 0)},
	} {
		var painted Rect
		p := Popup(Box().Size(60, 20), probe(100, 50, &painted)).Side(tt.side)
		pr := NewProbe(Center(p), Sz(400, 300))
		p.Show()
		pr.Frame()
		if p.effect.origin != tt.origin || p.effect.travel != tt.travel {
			t.Errorf("side %v: entrance from origin %v travelling %v, want %v and %v", tt.side, p.effect.origin, p.effect.travel, tt.origin, tt.travel)
		}
		// On its first frame, far from in, the content is already painted,
		// and so pressed, where it lands.
		if painted != p.Rect() {
			t.Errorf("side %v: content painted at %v while entering, want its place %v", tt.side, painted, p.Rect())
		}
		pr.Close()
	}
}

func TestPopupAnimatedFalseOpensAndClosesAtOnce(t *testing.T) {
	t.Parallel()
	var painted Rect
	p := Popup(Box().Size(60, 20), probe(100, 50, &painted)).Animated(false)
	pr := NewProbe(Center(p), Sz(400, 300))
	defer pr.Close()
	p.Show()
	pr.Frame()
	if v := p.reveal.Value(FrameTime()); v != 1 {
		t.Fatalf("an unanimated popup is %v of the way in on its first frame, want 1", v)
	}
	p.Hide()
	pr.Frame()
	painted = Rect{}
	pr.Advance(time.Millisecond)
	if painted != (Rect{}) {
		t.Fatal("an unanimated popup still painted after it closed")
	}
}

func TestTransitionScaleOriginAnchorsTheScale(t *testing.T) {
	t.Parallel()
	tr := Transition(Box().Size(100, 50)).Scale(.5)
	if tr.origin != Pt(.5, .5) {
		t.Fatalf("a Transition scales about %v by default, want its center", tr.origin)
	}
	if tr.ScaleOrigin(0, 1).origin != Pt(0, 1) {
		t.Fatal("ScaleOrigin did not set the origin")
	}
}
