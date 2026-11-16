package widget

import (
	"github.com/ironpark/ggui"
	"testing"
	"time"
)

// Where a popup is placed on its side is tested through the public surface
// in internal/apitest; these read the entrance it plays, its reveal and a
// Transition's origin, which only the package sees.

func TestPopupEntranceGrowsFromTheAnchorSideWithoutMovingItsHits(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		side   PopupSide
		origin ggui.Point
		travel ggui.Point
	}{
		{PopupBottom, ggui.Pt(0, 0), ggui.Pt(0, -8)},
		{PopupTop, ggui.Pt(0, 1), ggui.Pt(0, 8)},
		{PopupRight, ggui.Pt(0, 0), ggui.Pt(-8, 0)},
		{PopupLeft, ggui.Pt(1, 0), ggui.Pt(8, 0)},
	} {
		var painted ggui.Rect
		p := Popup(Box().Size(60, 20), probe(100, 50, &painted)).Side(tt.side)
		pr := ggui.NewProbe(Center(p), ggui.Sz(400, 300))
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
	var painted ggui.Rect
	p := Popup(Box().Size(60, 20), probe(100, 50, &painted)).Animated(false)
	pr := ggui.NewProbe(Center(p), ggui.Sz(400, 300))
	defer pr.Close()
	p.Show()
	pr.Frame()
	if v := p.reveal.Value(ggui.FrameTime()); v != 1 {
		t.Fatalf("an unanimated popup is %v of the way in on its first frame, want 1", v)
	}
	p.Hide()
	pr.Frame()
	painted = ggui.Rect{}
	pr.Advance(time.Millisecond)
	if painted != (ggui.Rect{}) {
		t.Fatal("an unanimated popup still painted after it closed")
	}
}

func TestTransitionScaleOriginAnchorsTheScale(t *testing.T) {
	t.Parallel()
	tr := Transition(Box().Size(100, 50)).Scale(.5)
	if tr.origin != ggui.Pt(.5, .5) {
		t.Fatalf("a Transition scales about %v by default, want its center", tr.origin)
	}
	if tr.ScaleOrigin(0, 1).origin != ggui.Pt(0, 1) {
		t.Fatal("ScaleOrigin did not set the origin")
	}
}
