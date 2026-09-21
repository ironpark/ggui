package ggui

// The accessibility tests that need a running app: a real widget tree, a
// Probe driving it, and the bridge reading what the frame published. The
// bridge's own unit tests live beside it, in the a11y package.

import (
	"testing"

	"github.com/ironpark/ggui/a11y"
)

// axBell counts presses, and says so, which is what the round trip through
// the bridge has to arrive at.
type axBell struct {
	Interactive
	rung int
}

func (b *axBell) Layout(c Constraints, _ Env) Size { return c.Constrain(Sz(40, 20)) }
func (b *axBell) Paint(dst *Canvas, r Rect)        { b.Hit(dst, r, b, 0) }
func (b *axBell) HandlePointer(PointerEvent) bool  { return true }
func (b *axBell) HandleKey(ev KeyEvent)            { b.Keyboard(ev, func() { b.rung++ }) }

// axDetailed runs a frame with the detail a text field only freezes while
// something is reading it, and puts the flag back.
func axDetailed(t *testing.T, p *Probe) *SemTree {
	t.Helper()
	a11y.SetWantsDetail(true)
	t.Cleanup(func() { a11y.SetWantsDetail(false) })
	return p.Semantics()
}

func TestAXTextFieldSaysWhereItsCharactersAre(t *testing.T) {
	value := State("hello world")
	w := TextInput(value)
	p := NewProbe(w, Sz(300, 40))
	defer p.Close()

	field, ok := axDetailed(t, p).Find(RoleTextField, "")
	if !ok {
		t.Fatal("no text field in the tree")
	}
	if got := a11y.CharCount(field.Node); got != 11 {
		t.Errorf("character count = %d, want 11", got)
	}
	if got := a11y.StringForRange(field.Node, 6, 5); got != "world" {
		t.Errorf("string for range = %q, want %q", got, "world")
	}
	if len(field.Runs) != 1 {
		t.Fatalf("%d runs, want one line", len(field.Runs))
	}
	if n := len(field.Runs[0].Stops); n != 12 {
		t.Errorf("%d stops, want one per character boundary (12)", n)
	}
	// The caret sits at the end after TextInput loaded the value, and the
	// field is one line, so that is where the insertion point is.
	if got := a11y.InsertionLine(field.Node); got != 0 {
		t.Errorf("insertion line = %d, want 0", got)
	}
	if loc, length, has := a11y.RangeForLine(field.Node, 0); !has || loc != 0 || length != 11 {
		t.Errorf("line 0 = %d+%d, %v; want the whole field", loc, length, has)
	}
	if _, _, has := a11y.RangeForLine(field.Node, 3); has {
		t.Error("a field with one line answered for line 3")
	}
	// A character's box is inside the field, is not empty, and moves right
	// as the offset does.
	first := a11y.RectForRange(field, 0, 1)
	later := a11y.RectForRange(field, 6, 1)
	if first.Empty() || later.Empty() {
		t.Fatalf("character boxes = %v, %v; want real rectangles", first, later)
	}
	if later.Origin.X <= first.Origin.X {
		t.Errorf("character 6 at x=%g is not right of character 0 at x=%g", later.Origin.X, first.Origin.X)
	}
	if first.Origin.X < field.Full.Origin.X || later.Origin.X > field.Full.Origin.X+field.Full.Size.W {
		t.Errorf("character boxes fell outside the field %v", field.Full)
	}
}

func TestAXTextDetailIsOnlyFrozenWhenSomethingIsReading(t *testing.T) {
	p := NewProbe(TextInput(State("hello")), Sz(300, 40))
	defer p.Close()
	field, ok := p.Semantics().Find(RoleTextField, "")
	if !ok {
		t.Fatal("no text field")
	}
	if field.Runs != nil {
		t.Error("the layout was frozen with nobody reading it; that is a measurement per character per frame")
	}
	// The selection is cheap and always there, so that the caret is known
	// the moment a bridge attaches.
	if field.SelEnd != len("hello") {
		t.Errorf("caret = %d, want the end of the text", field.SelEnd)
	}
	// Without a layout the field is still one line covering everything.
	if loc, length, has := a11y.RangeForLine(field.Node, 0); !has || loc != 0 || length != 5 {
		t.Errorf("line 0 = %d+%d, %v; want the whole field", loc, length, has)
	}
	if got := a11y.RectForRange(field, 0, 1); got != field.Full {
		t.Errorf("rect for range = %v, want the whole field %v", got, field.Full)
	}
}

func TestAXPasswordKeepsItsShapeToItself(t *testing.T) {
	p := NewProbe(TextInput(State("hunter2")).Password(), Sz(300, 40))
	defer p.Close()
	field, _ := axDetailed(t, p).Find(RoleTextField, "")
	if field.Runs != nil {
		t.Error("a password field froze its character positions")
	}
}

func TestAXSetSelectionMovesTheCaret(t *testing.T) {
	w := TextInput(State("hello world"))
	p := NewProbe(w, Sz(300, 40))
	defer p.Close()
	field, ok := p.Semantics().Find(RoleTextField, "")
	if !ok {
		t.Fatal("no text field")
	}
	if !field.Actions.Has(ActionSetSelection) {
		t.Fatal("a text field does not offer its selection")
	}
	start, end := a11y.ByteRange(field.Node, 6, 5)
	p.Perform(field.ID, Action{Kind: ActionSetSelection, SelStart: start, SelEnd: end})
	after, _ := p.Semantics().Find(RoleTextField, "")
	if after.SelStart != 6 || after.SelEnd != 11 {
		t.Errorf("selection = %d..%d, want 6..11", after.SelStart, after.SelEnd)
	}
	if got := a11y.Selected(after.Node); got != "world" {
		t.Errorf("selected = %q, want %q", got, "world")
	}
}

func TestAXMultilineFieldReportsItsLines(t *testing.T) {
	w := TextInput(State("one\ntwo\nthree")).Multiline()
	p := NewProbe(w, Sz(300, 100))
	defer p.Close()
	field, ok := axDetailed(t, p).Find(RoleTextField, "")
	if !ok {
		t.Fatal("no text field")
	}
	if len(field.Runs) != 3 {
		t.Fatalf("%d runs, want three lines:\n%v", len(field.Runs), field.Runs)
	}
	for i, want := range []string{"one", "two", "three"} {
		loc, length, has := a11y.RangeForLine(field.Node, i)
		if !has {
			t.Fatalf("no line %d", i)
		}
		if got := a11y.StringForRange(field.Node, loc, length); got != want {
			t.Errorf("line %d = %q, want %q", i, got, want)
		}
	}
	// A byte on the second line is on line 1, and the lines go down the
	// screen in order.
	if got := a11y.LineForIndex(field.Node, 5); got != 1 {
		t.Errorf("line for offset 5 = %d, want 1", got)
	}
	if field.Runs[1].Rect.Origin.Y <= field.Runs[0].Rect.Origin.Y {
		t.Error("the second line is not below the first")
	}
	// The caret loaded at the end of the text, which is the last line.
	if got := a11y.InsertionLine(field.Node); got != 2 {
		t.Errorf("insertion line = %d, want 2", got)
	}
}

// TestAXPressRingsTheWidget is the other half of the round trip: the app
// carries out the press an assistive technology asked for, and the widget
// hears it. The bridge's half, where the element finds the node and asks,
// is TestAXPressReachesTheApp in the a11y package.
func TestAXPressRingsTheWidget(t *testing.T) {
	w := &axBell{}
	w.Role = RoleButton
	w.SetName("ring")
	p := NewProbe(w, Sz(100, 100))
	defer p.Close()

	node, ok := p.Semantics().Find(RoleButton, "ring")
	if !ok {
		t.Fatal("the button is not in the published tree")
	}
	if !node.Actions.Has(ActionPress) {
		t.Fatal("the button does not offer a press")
	}
	p.Perform(node.ID, Action{Kind: ActionPress})
	if w.rung != 1 {
		t.Errorf("rung %d times, want 1", w.rung)
	}
}
