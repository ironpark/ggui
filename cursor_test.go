package ggui

import "testing"

// The topmost region under the pointer that asks for a cursor decides it,
// and asking for the default arrow is asking: it keeps the shape of a
// region beneath from showing through.
func TestARegionAskingForTheArrowHidesTheCursorBeneath(t *testing.T) {
	t.Parallel()
	w := Stack(
		Pointer(Box().Size(100, 100)).Cursor(CursorShapeText),
		Pointer(Box().Size(50, 50)).Cursor(CursorShapeDefault),
		// A region that asks for nothing lets the one beneath decide.
		Padding(Pointer(Box().Size(20, 20)).OnMove(func(PointerEvent) {}), 70, 0, 0, 70),
	)
	p := NewProbe(w, Sz(100, 100))
	defer p.Close()
	for _, c := range []struct {
		name string
		at   Point
		want CursorShape
	}{
		{"over the region asking for the arrow", Pt(25, 25), CursorShapeDefault},
		{"beside it", Pt(60, 25), CursorShapeText},
		{"over a region that asks for nothing", Pt(80, 80), CursorShapeText},
	} {
		p.Move(c.at)
		if got := p.Cursor(); got != c.want {
			t.Errorf("%s the cursor is %v, want %v", c.name, got, c.want)
		}
	}
}

// arrowControl is a control that asks for the arrow, as a button does.
type arrowControl struct {
	Interactive
	shape CursorShape
}

func (a *arrowControl) Layout(c Constraints, _ Env) Size   { return c.Constrain(Sz(40, 40)) }
func (a *arrowControl) Paint(dst *Canvas, r Rect)          { a.Hit(dst, r, a, a.shape) }
func (a *arrowControl) HandlePointer(ev PointerEvent) bool { return a.Pointer(ev, nil) }
func (a *arrowControl) HandleKey(KeyEvent)                 {}
func (a *arrowControl) Semantics() (Role, string)          { return RoleButton, "arrow" }

func TestAControlAskingForTheArrowHidesTheCursorBeneath(t *testing.T) {
	t.Parallel()
	w := Stack(Pointer(Box().Size(100, 100)).Cursor(CursorShapeText), &arrowControl{})
	p := NewProbe(w, Sz(100, 100))
	defer p.Close()
	p.Move(Pt(20, 20))
	if got := p.Cursor(); got != CursorShapeDefault {
		t.Fatalf("over a control asking for the arrow the cursor is %v", got)
	}
	p.Move(Pt(70, 70))
	if got := p.Cursor(); got != CursorShapeText {
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
		field := Pointer(Box().Size(300, 300)).Cursor(CursorShapeText)
		inner := Pointer(Box().Size(40, 20)).Cursor(CursorShapePointer)
		pop := Popup(Box().Size(60, 20), Column(inner, Box().Size(100, 60))).Modal(modal)
		p := NewProbe(Stack(field, Padding(pop, 50, 0, 0, 50)), Sz(300, 300))
		p.Move(Pt(200, 200))
		if got := p.Cursor(); got != CursorShapeText {
			t.Errorf("modal=%v: before the popup opens the cursor is %v, want text", modal, got)
		}
		pop.Show()
		p.Frame()
		r := pop.Rect()
		p.Move(Pt(r.Origin.X+r.Size.W-5, r.Origin.Y+r.Size.H-5))
		if got := p.Cursor(); got != CursorShapeDefault {
			t.Errorf("modal=%v: over the popup's content the cursor is %v, want the arrow", modal, got)
		}
		p.Move(r.Origin.Add(Pt(5, 5)))
		if got := p.Cursor(); got != CursorShapePointer {
			t.Errorf("modal=%v: over a link in the popup the cursor is %v, want its hand", modal, got)
		}
		p.Move(Pt(250, 250))
		if got, want := p.Cursor(), pick(modal, CursorShape(CursorShapeDefault), CursorShape(CursorShapeText)); got != want {
			t.Errorf("modal=%v: outside the open popup the cursor is %v, want %v", modal, got, want)
		}
		p.Close()
	}
}
