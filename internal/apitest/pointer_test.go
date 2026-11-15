package apitest_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/pkg/gem"
)

// reachButton is a control hit a little way outside its Rect.
type reachButton struct {
	ggui.Interactive
	taps, enters, exits int
	margin              float64
}

func (b *reachButton) Layout(c ggui.Constraints, _ ggui.Env) ggui.Size {
	return c.Constrain(ggui.Sz(16, 16))
}
func (b *reachButton) Paint(dst *ggui.Canvas, r ggui.Rect) {
	b.HitReach(dst, r, r.Grow(b.margin), b, ggui.CursorShapePointer)
}
func (b *reachButton) HandleKey(ev ggui.KeyEvent) { b.Keyboard(ev, nil) }
func (b *reachButton) HandlePointer(ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerEnter:
		b.enters++
	case ggui.PointerExit:
		b.exits++
	}
	return b.Pointer(ev, func() { b.taps++ })
}

// HitReach takes the pointer in the reach as in the Rect, as one region:
// crossing between them is no exit, a click in the reach is a tap and
// focuses, and the Rect a probe finds is still the control's own.
func TestHitReachExtendsThePointerTargetOnly(t *testing.T) {
	t.Parallel()
	b := &reachButton{margin: 8}
	b.Role = ggui.RoleButton
	b.SetName("Target")
	p := ggui.NewProbe(ggui.Box(b).Padding(ggui.Insets(20)), ggui.Sz(100, 100))
	defer p.Close()
	f, ok := p.FindRole(ggui.RoleButton, "Target")
	if !ok || f.Rect != ggui.Rct(ggui.Pt(20, 20), ggui.Sz(16, 16)) {
		t.Fatalf("found %v %v, want the control's own 16px Rect", f, ok)
	}
	outside, inside := ggui.Pt(14, 14), ggui.Pt(28, 28)
	p.Move(outside)
	if !b.Hovered || b.enters != 1 {
		t.Fatalf("hovered=%v enters=%d in the reach, want hovered once", b.Hovered, b.enters)
	}
	if p.Cursor() != ggui.CursorShapePointer {
		t.Fatal("the reach does not carry the control's cursor")
	}
	p.Move(inside)
	p.Move(outside)
	if b.enters != 1 || b.exits != 0 {
		t.Fatalf("enters=%d exits=%d crossing between the Rect and its reach, want 1 and 0", b.enters, b.exits)
	}
	p.Click(outside)
	if b.taps != 1 || !b.Focused {
		t.Fatalf("taps=%d focused=%v after a click in the reach, want 1 and focused", b.taps, b.Focused)
	}
	p.Move(ggui.Pt(10, 10))
	if b.Hovered || b.exits != 1 {
		t.Fatalf("hovered=%v exits=%d past the reach, want left once", b.Hovered, b.exits)
	}
	p.Click(ggui.Pt(10, 10))
	if b.taps != 1 {
		t.Fatal("a click past the reach tapped")
	}
}

// A reach is cut by the clip its control is painted in, and moves with a
// transform as the Rect does.
func TestHitReachFollowsClipAndTransform(t *testing.T) {
	t.Parallel()
	b := &reachButton{margin: 8}
	clipped := ggui.FromFuncs(func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(60, 60)) }, func(dst *ggui.Canvas, r ggui.Rect) {
		dst.Clip(ggui.Rct(ggui.Pt(0, 0), ggui.Sz(60, 24))).Paint(b, ggui.Rct(ggui.Pt(20, 4), ggui.Sz(16, 16)))
	})
	p := ggui.NewProbe(clipped, ggui.Sz(60, 60))
	p.Move(ggui.Pt(28, 22))
	if !b.Hovered {
		t.Fatal("the reach inside the clip is not hit")
	}
	p.Move(ggui.Pt(28, 26))
	if b.Hovered {
		t.Fatal("the reach is hit outside the clip")
	}
	p.Close()

	b = &reachButton{margin: 8}
	p = ggui.NewProbe(ggui.Box(ggui.Transform(b).Translate(30, 0)).Padding(ggui.Insets(20)), ggui.Sz(100, 100))
	defer p.Close()
	p.Move(ggui.Pt(44, 28))
	if !b.Hovered {
		t.Fatal("the reach did not move with its control")
	}
	p.Move(ggui.Pt(14, 28))
	if b.Hovered {
		t.Fatal("the reach stayed where the control was laid out")
	}
}

// The topmost region under the pointer that asks for a cursor decides it,
// and asking for the default arrow is asking: it keeps the shape of a
// region beneath from showing through.
func TestARegionAskingForTheArrowHidesTheCursorBeneath(t *testing.T) {
	t.Parallel()
	w := ggui.Stack(
		ggui.Pointer(ggui.Box().Size(100, 100)).Cursor(ggui.CursorShapeText),
		ggui.Pointer(ggui.Box().Size(50, 50)).Cursor(ggui.CursorShapeDefault),
		// A region that asks for nothing lets the one beneath decide.
		ggui.Padding(ggui.Pointer(ggui.Box().Size(20, 20)).OnMove(func(ggui.PointerEvent) {}), 70, 0, 0, 70),
	)
	p := ggui.NewProbe(w, ggui.Sz(100, 100))
	defer p.Close()
	for _, c := range []struct {
		name string
		at   ggui.Point
		want ggui.CursorShape
	}{
		{"over the region asking for the arrow", ggui.Pt(25, 25), ggui.CursorShapeDefault},
		{"beside it", ggui.Pt(60, 25), ggui.CursorShapeText},
		{"over a region that asks for nothing", ggui.Pt(80, 80), ggui.CursorShapeText},
	} {
		p.Move(c.at)
		if got := p.Cursor(); got != c.want {
			t.Errorf("%s the cursor is %v, want %v", c.name, got, c.want)
		}
	}
}

// arrowControl is a control that asks for the arrow, as a button does.
type arrowControl struct {
	ggui.Interactive
	shape ggui.CursorShape
}

func (a *arrowControl) Layout(c ggui.Constraints, _ ggui.Env) ggui.Size {
	return c.Constrain(ggui.Sz(40, 40))
}
func (a *arrowControl) Paint(dst *ggui.Canvas, r ggui.Rect)     { a.Hit(dst, r, a, a.shape) }
func (a *arrowControl) HandlePointer(ev ggui.PointerEvent) bool { return a.Pointer(ev, nil) }
func (a *arrowControl) HandleKey(ggui.KeyEvent)                 {}
func (a *arrowControl) Semantics() (ggui.Role, string)          { return ggui.RoleButton, "arrow" }

func TestAControlAskingForTheArrowHidesTheCursorBeneath(t *testing.T) {
	t.Parallel()
	w := ggui.Stack(ggui.Pointer(ggui.Box().Size(100, 100)).Cursor(ggui.CursorShapeText), &arrowControl{})
	p := ggui.NewProbe(w, ggui.Sz(100, 100))
	defer p.Close()
	p.Move(ggui.Pt(20, 20))
	if got := p.Cursor(); got != ggui.CursorShapeDefault {
		t.Fatalf("over a control asking for the arrow the cursor is %v", got)
	}
	p.Move(ggui.Pt(70, 70))
	if got := p.Cursor(); got != ggui.CursorShapeText {
		t.Fatalf("beside the control the cursor is %v, want text", got)
	}
}

// A popup's content is a surface of its own: the text field it floats over
// does not put its I-beam on it, and a modal one leaves nothing under its
// scrim to point at, while a non-modal one leaves the rest of the window
// as it was.
func TestAPopupShowsTheArrowOverWhatItCovers(t *testing.T) {
	t.Parallel()
	for _, modal := range []bool{true, false} {
		field := ggui.Pointer(ggui.Box().Size(300, 300)).Cursor(ggui.CursorShapeText)
		inner := ggui.Pointer(ggui.Box().Size(40, 20)).Cursor(ggui.CursorShapePointer)
		pop := ggui.Popup(ggui.Box().Size(60, 20), ggui.Column(inner, ggui.Box().Size(100, 60))).Modal(modal)
		p := ggui.NewProbe(ggui.Stack(field, ggui.Padding(pop, 50, 0, 0, 50)), ggui.Sz(300, 300))
		p.Move(ggui.Pt(200, 200))
		if got := p.Cursor(); got != ggui.CursorShapeText {
			t.Errorf("modal=%v: before the popup opens the cursor is %v, want text", modal, got)
		}
		pop.Show()
		p.Frame()
		r := pop.Rect()
		p.Move(ggui.Pt(r.Origin.X+r.Size.W-5, r.Origin.Y+r.Size.H-5))
		if got := p.Cursor(); got != ggui.CursorShapeDefault {
			t.Errorf("modal=%v: over the popup's content the cursor is %v, want the arrow", modal, got)
		}
		p.Move(r.Origin.Add(ggui.Pt(5, 5)))
		if got := p.Cursor(); got != ggui.CursorShapePointer {
			t.Errorf("modal=%v: over a link in the popup the cursor is %v, want its hand", modal, got)
		}
		p.Move(ggui.Pt(250, 250))
		if got, want := p.Cursor(), gem.Pick(modal, ggui.CursorShape(ggui.CursorShapeDefault), ggui.CursorShape(ggui.CursorShapeText)); got != want {
			t.Errorf("modal=%v: outside the open popup the cursor is %v, want %v", modal, got, want)
		}
		p.Close()
	}
}

// A region asking for the default cursor shows the arrow over whatever
// asked for another shape under it; one that asks for nothing lets that
// shape through.
func TestDefaultCursorIsARequest(t *testing.T) {
	t.Parallel()
	b := &reachButton{}
	root := ggui.FromFuncs(func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(100, 40)) }, func(dst *ggui.Canvas, r ggui.Rect) {
		dst.HitCursor(r, ggui.CursorShapeText)
		dst.HitCursor(ggui.Rct(ggui.Pt(0, 0), ggui.Sz(20, 40)), ggui.CursorShapeDefault)
		dst.HitPointer(ggui.Rct(ggui.Pt(40, 0), ggui.Sz(20, 40)), b)
		b.Hit(dst, ggui.Rct(ggui.Pt(80, 0), ggui.Sz(20, 40)), b, ggui.CursorShapeDefault)
	})
	p := ggui.NewProbe(root, ggui.Sz(100, 40))
	defer p.Close()
	for _, c := range []struct {
		x    float64
		want ggui.CursorShape
		what string
	}{
		{10, ggui.CursorShapeDefault, "a HitCursor for the default"},
		{30, ggui.CursorShapeText, "the region underneath"},
		{50, ggui.CursorShapeText, "a pointer region asking for no cursor"},
		{90, ggui.CursorShapeDefault, "a control hit with the default"},
	} {
		p.Move(ggui.Pt(c.x, 20))
		if got := p.Cursor(); got != c.want {
			t.Errorf("cursor = %v over %s, want %v", got, c.what, c.want)
		}
	}
}
