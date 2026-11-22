package widget

import (
	"github.com/ironpark/ggui"
	"slices"
	"testing"
)

// The tests here are of Pointer, Tap and Focus: that each hands its
// callbacks what the kernel routes to it. How the kernel routes input,
// the topmost region first, captures and hover, is tested in input_test.go
// on regions of its own.

// Pointer calls each callback it has for what reaches it: a tap for a
// press and release inside, a hover as the pointer comes and goes, the
// drags of a press it took, inside or out, and their release, and the
// wheel, which a Tap inside it takes none of. Over it the cursor is the
// shape it asks for.
func TestPointerCallsItsCallbacks(t *testing.T) {
	t.Parallel()
	var taps, drags, ups int
	var hover []bool
	var scrolled ggui.Point
	w := Column(
		Pointer(Tap(Box().Size(50, 50), func() { taps++ })).OnScroll(func(d ggui.Point) { scrolled = d }),
		Pointer(Box().Size(50, 50)).
			OnHover(func(on bool) { hover = append(hover, on) }).
			OnDrag(func(ggui.PointerEvent) { drags++ }).
			OnUp(func(ggui.PointerEvent) { ups++ }).
			Cursor(ggui.CursorShapeText),
	)
	p := ggui.NewProbe(w, ggui.Sz(50, 100))
	defer p.Close()
	p.Click(ggui.Pt(10, 10))
	p.Press(ggui.Pt(10, 10))
	p.Release(ggui.Pt(10, 200))
	if taps != 1 {
		t.Fatalf("taps = %d after a click and a press let go outside, want 1", taps)
	}
	p.Scroll(ggui.Pt(10, 10), ggui.Pt(0, -3))
	if scrolled != ggui.Pt(0, -3) {
		t.Fatalf("the wheel reached OnScroll as %v through the Tap, want (0, -3)", scrolled)
	}
	p.Move(ggui.Pt(10, 60))
	if p.Cursor() != ggui.CursorShapeText {
		t.Fatalf("cursor %v over the Pointer, want its text cursor", p.Cursor())
	}
	p.Press(ggui.Pt(10, 60))
	p.Move(ggui.Pt(200, 200))
	p.Release(ggui.Pt(200, 200))
	p.Move(ggui.Pt(10, 10))
	if drags != 1 || ups != 1 {
		t.Fatalf("drags %d, releases %d; want the drag outside and its release", drags, ups)
	}
	if !slices.Equal(hover, []bool{true, false}) {
		t.Fatalf("hover %v, want it entered and left", hover)
	}
}

// Focus takes focus when clicked, hands the keys and the text typed while
// it holds it to OnKey and OnText, and says when it gains and loses it.
func TestFocusCallsItsCallbacks(t *testing.T) {
	t.Parallel()
	var keys []ggui.KeyboardKey
	var typed string
	var focus []bool
	w := Row(
		Focus(Box().Size(50, 50)).
			OnKey(func(k ggui.KeyboardKey) { keys = append(keys, k) }).
			OnText(func(s string) { typed += s }).
			OnFocus(func(on bool) { focus = append(focus, on) }),
		Box().Size(50, 50),
	)
	p := ggui.NewProbe(w, ggui.Sz(100, 50))
	defer p.Close()
	p.Type(ggui.Mods{}, ggui.KeyA)
	if len(keys) != 0 {
		t.Fatal("keys reached Focus before it was clicked")
	}
	p.Click(ggui.Pt(10, 10))
	p.TypeText(ggui.Mods{}, ggui.KeyA, "a")
	if !slices.Equal(keys, []ggui.KeyboardKey{ggui.KeyA}) || typed != "a" {
		t.Fatalf("keys %v, typed %q; want [A] and a", keys, typed)
	}
	p.Click(ggui.Pt(75, 10))
	p.Type(ggui.Mods{}, ggui.KeyB)
	if len(keys) != 1 || !slices.Equal(focus, []bool{true, false}) {
		t.Fatalf("keys %v, focus %v; want none after the click away, and focus gained then lost", keys, focus)
	}
}

func TestPointerOnHoverNilClearsWithoutPanic(t *testing.T) {
	t.Parallel()
	w := Pointer(Box().Size(50, 50)).OnHover(func(bool) {}).OnHover(nil)
	if w.onEnter != nil || w.onExit != nil {
		t.Fatal("OnHover(nil) did not clear onEnter/onExit")
	}
	p := ggui.NewProbe(w, ggui.Sz(50, 50))
	defer p.Close()
	p.Move(ggui.Pt(10, 10))
	p.Move(ggui.Pt(100, 100))
}
