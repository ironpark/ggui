package apitest_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/widget"
)

// sidePopup opens a 100x50 popup on a 60x20 anchor at at, in a 400x300
// window.
func sidePopup(t *testing.T, at ggui.Point, set func(*widget.PopupWidget)) *widget.PopupWidget {
	t.Helper()
	p := widget.Popup(widget.Box().Size(60, 20), widget.Box().Size(100, 50))
	set(p)
	pr := ggui.NewProbe(widget.Row(widget.Padding(p, at.Y, 0, 0, at.X)).Align(widget.AlignStart), ggui.Sz(400, 300))
	t.Cleanup(pr.Close)
	p.Show()
	pr.Frame()
	return p
}

func TestPopupSidePlacesContentAroundTheAnchor(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		side widget.PopupSide
		want ggui.Rect
		on   widget.PopupSide
	}{
		{"bottom", widget.PopupBottom, ggui.Rct(ggui.Pt(150, 124), ggui.Sz(100, 50)), widget.PopupBottom},
		{"top", widget.PopupTop, ggui.Rct(ggui.Pt(150, 46), ggui.Sz(100, 50)), widget.PopupTop},
		{"right", widget.PopupRight, ggui.Rct(ggui.Pt(214, 100), ggui.Sz(100, 50)), widget.PopupRight},
		{"left", widget.PopupLeft, ggui.Rct(ggui.Pt(46, 100), ggui.Sz(100, 50)), widget.PopupLeft},
		{"inline end", widget.PopupInlineEnd, ggui.Rct(ggui.Pt(214, 100), ggui.Sz(100, 50)), widget.PopupRight},
		{"inline start", widget.PopupInlineStart, ggui.Rct(ggui.Pt(46, 100), ggui.Sz(100, 50)), widget.PopupLeft},
	} {
		p := sidePopup(t, ggui.Pt(150, 100), func(p *widget.PopupWidget) { p.Side(tt.side) })
		if p.Rect() != tt.want || p.PlacedSide() != tt.on {
			t.Errorf("%s: content at %v on side %v, want %v on %v", tt.name, p.Rect(), p.PlacedSide(), tt.want, tt.on)
		}
	}
}

func TestPopupInlineSidesFollowTheWritingDirection(t *testing.T) {
	t.Parallel()
	// The anchor records the Rect it is painted in, which is the popup's.
	var anchor ggui.Rect
	p := widget.Popup(widget.Box(probe(60, 20, &anchor)).Size(60, 20), widget.Box().Size(100, 50)).Side(widget.PopupInlineEnd)
	pr := ggui.NewProbe(ggui.Provide(ggui.DirectionKey, ggui.DirRTL, widget.Center(p)), ggui.Sz(400, 300))
	defer pr.Close()
	p.Show()
	pr.Frame()
	if p.PlacedSide() != widget.PopupLeft || p.Rect().Origin.X+p.Rect().Size.W != anchor.Origin.X-4 {
		t.Fatalf("inline end under RTL: content at %v on side %v beside anchor %v", p.Rect(), p.PlacedSide(), anchor)
	}
}

func TestPopupSideFlipsWhenOnlyTheOppositeSideHasRoom(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		at   ggui.Point
		side widget.PopupSide
		on   widget.PopupSide
	}{
		{"top at the top edge", ggui.Pt(150, 10), widget.PopupTop, widget.PopupBottom},
		{"bottom at the bottom edge", ggui.Pt(150, 270), widget.PopupBottom, widget.PopupTop},
		{"left at the left edge", ggui.Pt(10, 100), widget.PopupLeft, widget.PopupRight},
		{"right at the right edge", ggui.Pt(330, 100), widget.PopupRight, widget.PopupLeft},
	} {
		p := sidePopup(t, tt.at, func(p *widget.PopupWidget) { p.Side(tt.side) })
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
		side   widget.PopupSide
		align  widget.PopupAlignment
		offset float64
		want   ggui.Point
	}{
		{"below, start, overhanging", widget.PopupBottom, widget.PopupStart, -4, ggui.Pt(146, 124)},
		{"below, end, overhanging", widget.PopupBottom, widget.PopupEnd, -4, ggui.Pt(114, 124)},
		{"below, center, shifted", widget.PopupBottom, widget.PopupCenter, 10, ggui.Pt(140, 124)},
		{"right, start", widget.PopupRight, widget.PopupStart, 0, ggui.Pt(214, 100)},
		{"right, start, raised", widget.PopupRight, widget.PopupStart, -3, ggui.Pt(214, 97)},
		{"right, center", widget.PopupRight, widget.PopupCenter, 0, ggui.Pt(214, 85)},
		{"right, end", widget.PopupRight, widget.PopupEnd, 0, ggui.Pt(214, 70)},
		{"right, end, lowered", widget.PopupRight, widget.PopupEnd, -3, ggui.Pt(214, 73)},
	} {
		p := sidePopup(t, ggui.Pt(150, 100), func(p *widget.PopupWidget) { p.Side(tt.side).Align(tt.align).AlignOffset(tt.offset) })
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
		side  widget.PopupSide
		align widget.PopupAlignment
		want  ggui.Point
	}{
		{"below, at the left edge", ggui.Pt(0, 100), widget.PopupBottom, widget.PopupStart, ggui.Pt(5, 124)},
		{"below, start at the right edge", ggui.Pt(340, 100), widget.PopupBottom, widget.PopupStart, ggui.Pt(400-5-100, 124)},
		{"below, its end at the right edge", ggui.Pt(340, 100), widget.PopupBottom, widget.PopupEnd, ggui.Pt(400-5-100, 124)},
		{"beside, at the top edge", ggui.Pt(100, 0), widget.PopupRight, widget.PopupStart, ggui.Pt(164, 5)},
		{"beside, at the bottom edge", ggui.Pt(100, 280), widget.PopupRight, widget.PopupStart, ggui.Pt(164, 300-5-50)},
	} {
		p := sidePopup(t, tt.at, func(p *widget.PopupWidget) { p.Side(tt.side).Align(tt.align) })
		if got := p.Rect(); got != ggui.Rct(tt.want, ggui.Sz(100, 50)) {
			t.Errorf("%s: content at %v, want %v", tt.name, got, ggui.Rct(tt.want, ggui.Sz(100, 50)))
		}
	}
	// CollisionPadding(0) lets it sit flush against the edge.
	p := sidePopup(t, ggui.Pt(0, 100), func(p *widget.PopupWidget) { p.CollisionPadding(0) })
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
		side widget.PopupSide
		on   widget.PopupSide
	}{
		// 300 - (224+20+4) leaves 52 below, 47 of them inside the padding.
		{"below", ggui.Pt(150, 224), widget.PopupBottom, widget.PopupTop},
		// 400 - (234+60+4) leaves 102 to the right, 97 inside the padding.
		{"right", ggui.Pt(234, 100), widget.PopupRight, widget.PopupLeft},
	} {
		p := sidePopup(t, tt.at, func(p *widget.PopupWidget) { p.Side(tt.side) })
		if p.PlacedSide() != tt.on {
			t.Errorf("%s: content on side %v at %v, want it flipped to %v", tt.name, p.PlacedSide(), p.Rect(), tt.on)
		}
		flush := sidePopup(t, tt.at, func(p *widget.PopupWidget) { p.Side(tt.side).CollisionPadding(0) })
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
	p := widget.Popup(widget.Box().Size(60, 20), probe(80, 1000, &content))
	pr := ggui.NewProbe(widget.Row(widget.Padding(p, 100, 0, 0, 150)).Align(widget.AlignStart), ggui.Sz(400, 300))
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
		align widget.PopupAlignment
		dir   ggui.TextDirection
		x     float64
	}{
		{widget.PopupStart, ggui.DirLTR, 0}, {widget.PopupCenter, ggui.DirLTR, -20}, {widget.PopupEnd, ggui.DirLTR, -40},
		{widget.PopupStart, ggui.DirRTL, -40}, {widget.PopupCenter, ggui.DirRTL, -20}, {widget.PopupEnd, ggui.DirRTL, 0},
	} {
		var anchor ggui.Rect
		p := widget.Popup(widget.Box(probe(60, 20, &anchor)).Size(60, 20), widget.Box().Size(100, 50)).Align(tt.align)
		pr := ggui.NewProbe(ggui.Provide(ggui.DirectionKey, tt.dir, widget.Column(widget.Padding(p, 0, 100))), ggui.Sz(400, 250))
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
	p := widget.Popup(widget.Box().Size(50, 20), widget.Box().Size(100, 100)).Modal(false).BindOpen(ggui.Controlled(true, func(bool) { requests++ })).OnClose(func() { closes++ })
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
		input := widget.TextInput(ggui.State("")).Name("Query")
		item := widget.TextInput(ggui.State("")).Name("Item")
		p := widget.Popup(input, widget.Box(item).Size(120, 40)).Modal(false).FocusOnOpen(!keep)
		pr := ggui.NewProbe(widget.Column(p), ggui.Sz(300, 250))
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

func TestPopupKeysKeepsFocusOnTheOpener(t *testing.T) {
	t.Parallel()
	useQuietIME(t)
	for _, keys := range []bool{true, false} {
		field := widget.TextInput(ggui.State("x"))
		p := widget.Popup(field, widget.Box().Size(80, 40))
		if keys {
			p.Keys(field)
		}
		pr := ggui.NewProbe(widget.Column(p), ggui.Sz(200, 200))
		pr.Click(ggui.Pt(100, 5))
		p.Show()
		pr.Click(ggui.Pt(20, 40)) // inside the content
		if field.Focused() != keys {
			t.Errorf("Keys %v: the field is focused %v after a click in the content, want %v", keys, field.Focused(), keys)
		}
		pr.Close()
	}
}

func TestPopupOwnerNestsTheContentUnderTheOpener(t *testing.T) {
	t.Parallel()
	useQuietIME(t)
	for _, owned := range []bool{true, false} {
		field := widget.TextInput(ggui.State("")).Name("combo")
		option := ggui.FromFuncs(
			func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(80, 20)) },
			func(dst *ggui.Canvas, r ggui.Rect) { dst.Leaf(r, ggui.Node{Role: ggui.RoleImage, Name: "option"}) },
		)
		p := widget.Popup(field, option)
		if owned {
			p.Owner(field)
		}
		pr := ggui.NewProbe(widget.Column(p), ggui.Sz(200, 200))
		p.Show()
		tree := pr.Semantics()
		under := false
		for i, n := range tree.Nodes(ggui.RoleImage) {
			for _, a := range tree.Ancestors(i) {
				under = under || (a.Role == ggui.RoleTextField && a.Name == "combo")
			}
			if n.Name != "option" {
				t.Fatalf("unexpected image %q", n.Name)
			}
		}
		if under != owned {
			t.Errorf("Owner set %v: the option is under the field %v", owned, under)
		}
		pr.Close()
	}
}
