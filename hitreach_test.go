package ggui

import "testing"

// reachButton is a control hit a little way outside its Rect.
type reachButton struct {
	Interactive
	taps, enters, exits int
	margin              float64
}

func (b *reachButton) Layout(c Constraints, _ Env) Size { return c.Constrain(Sz(16, 16)) }
func (b *reachButton) Paint(dst *Canvas, r Rect) {
	b.HitReach(dst, r, r.Grow(b.margin), b, CursorShapePointer)
}
func (b *reachButton) HandleKey(ev KeyEvent) { b.Keyboard(ev, nil) }
func (b *reachButton) HandlePointer(ev PointerEvent) bool {
	switch ev.Kind {
	case PointerEnter:
		b.enters++
	case PointerExit:
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
	b.Role = RoleButton
	b.SetName("Target")
	p := NewProbe(Box(b).Padding(Insets(20)), Sz(100, 100))
	defer p.Close()
	f, ok := p.FindRole(RoleButton, "Target")
	if !ok || f.Rect != Rct(Pt(20, 20), Sz(16, 16)) {
		t.Fatalf("found %v %v, want the control's own 16px Rect", f, ok)
	}
	outside, inside := Pt(14, 14), Pt(28, 28)
	p.Move(outside)
	if !b.Hovered || b.enters != 1 {
		t.Fatalf("hovered=%v enters=%d in the reach, want hovered once", b.Hovered, b.enters)
	}
	if p.Cursor() != CursorShapePointer {
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
	p.Move(Pt(10, 10))
	if b.Hovered || b.exits != 1 {
		t.Fatalf("hovered=%v exits=%d past the reach, want left once", b.Hovered, b.exits)
	}
	p.Click(Pt(10, 10))
	if b.taps != 1 {
		t.Fatal("a click past the reach tapped")
	}
}

// A reach is cut by the clip its control is painted in, and moves with a
// transform as the Rect does.
func TestHitReachFollowsClipAndTransform(t *testing.T) {
	t.Parallel()
	b := &reachButton{margin: 8}
	clipped := FromFuncs(func(c Constraints, _ Env) Size { return c.Constrain(Sz(60, 60)) }, func(dst *Canvas, r Rect) {
		dst.Clip(Rct(Pt(0, 0), Sz(60, 24))).Paint(b, Rct(Pt(20, 4), Sz(16, 16)))
	})
	p := NewProbe(clipped, Sz(60, 60))
	p.Move(Pt(28, 22))
	if !b.Hovered {
		t.Fatal("the reach inside the clip is not hit")
	}
	p.Move(Pt(28, 26))
	if b.Hovered {
		t.Fatal("the reach is hit outside the clip")
	}
	p.Close()

	b = &reachButton{margin: 8}
	p = NewProbe(Box(Transform(b).Translate(30, 0)).Padding(Insets(20)), Sz(100, 100))
	defer p.Close()
	p.Move(Pt(44, 28))
	if !b.Hovered {
		t.Fatal("the reach did not move with its control")
	}
	p.Move(Pt(14, 28))
	if b.Hovered {
		t.Fatal("the reach stayed where the control was laid out")
	}
}
