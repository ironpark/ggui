package ggui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func clickAt(in *inputState, p Point) {
	in.dispatch(frameInput{pos: p, down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	in.dispatch(frameInput{pos: p, up: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
}

func TestButtonTapsAndDisables(t *testing.T) {
	taps := 0
	b := Button("go", func() { taps++ })
	var in inputState
	paintFrame(&in, b, Sz(100, 40))
	clickAt(&in, Pt(10, 10))
	if taps != 1 {
		t.Fatalf("taps = %d, want 1", taps)
	}
	in.dispatch(frameInput{pos: Pt(10, 10)})
	if in.cursor != ebiten.CursorShapePointer {
		t.Fatalf("cursor = %v over a button, want pointer", in.cursor)
	}
	b.Disabled(true)
	paintFrame(&in, b, Sz(100, 40))
	clickAt(&in, Pt(10, 10))
	if taps != 1 {
		t.Fatalf("taps = %d after a click on a disabled button, want 1", taps)
	}
	if in.cursor != ebiten.CursorShapeDefault {
		t.Fatalf("cursor = %v over a disabled button, want default", in.cursor)
	}
}

func TestButtonSizesFromThemePadding(t *testing.T) {
	b := Button("go", nil)
	got := b.Layout(Loose(Sz(300, 300)), Env{})
	text := Text("go").Layout(Loose(Sz(300, 300)), Env{})
	th := DefaultTheme()
	if got.W != text.W+th.Space*4 || got.H != text.H+th.Space*1.5 {
		t.Fatalf("button %v around text %v, want theme padding", got, text)
	}
}

func TestCheckboxTogglesSignal(t *testing.T) {
	on := State(false)
	changed := []bool{}
	c := Checkbox(on, "label").OnChange(func(b bool) { changed = append(changed, b) })
	var in inputState
	paintFrame(&in, c, Sz(200, 30))
	clickAt(&in, Pt(5, 5))
	clickAt(&in, Pt(60, 5)) // on the label
	if len(changed) != 2 || !changed[0] || changed[1] || on.Peek() {
		t.Fatalf("changed = %v, on = %v", changed, on.Peek())
	}
	size := c.Layout(Loose(Sz(200, 30)), Env{})
	label := Text("label").Layout(Loose(Sz(200, 30)), Env{})
	if size.W != controlSize+controlGap+label.W {
		t.Fatalf("checkbox width %v, want glyph + gap + label %v", size.W, label.W)
	}
}

func TestRadioSelectsValue(t *testing.T) {
	choice := State("a")
	col := Column(Radio(choice, "a", "A"), Radio(choice, "b", "B")).Gap(4)
	var in inputState
	paintFrame(&in, col, Sz(100, 60))
	clickAt(&in, Pt(5, controlSize+4+5))
	if choice.Peek() != "b" {
		t.Fatalf("choice = %q, want b", choice.Peek())
	}
}

func TestSwitchToggles(t *testing.T) {
	on := State(false)
	s := Switch(on, "")
	var in inputState
	paintFrame(&in, s, Sz(100, 30))
	clickAt(&in, Pt(5, 5))
	if !on.Peek() {
		t.Fatal("switch did not turn on")
	}
	if got := s.Layout(Loose(Sz(100, 30)), Env{}); got != Sz(switchWidth, switchHeight) {
		t.Fatalf("switch size %v", got)
	}
}

func TestSliderDragsBeyondItsRect(t *testing.T) {
	v := State(0.0)
	s := Slider(v, 0, 100).Step(10)
	var in inputState
	paintFrame(&in, s, Sz(100+2*sliderKnob, 20))
	in.dispatch(frameInput{pos: Pt(sliderKnob+50, 10), down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	if v.Peek() != 50 {
		t.Fatalf("value = %v after pressing mid-track, want 50", v.Peek())
	}
	in.dispatch(frameInput{pos: Pt(sliderKnob+73, 10)})
	if v.Peek() != 70 {
		t.Fatalf("value = %v after dragging to 73%%, want 70 with step 10", v.Peek())
	}
	in.dispatch(frameInput{pos: Pt(500, 300)}) // dragged far outside
	if v.Peek() != 100 {
		t.Fatalf("value = %v after dragging past the end, want 100", v.Peek())
	}
	in.dispatch(frameInput{pos: Pt(500, 300), up: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	in.dispatch(frameInput{pos: Pt(500, 300)})
	if v.Peek() != 100 || s.pressed {
		t.Fatalf("value = %v pressed = %v after release", v.Peek(), s.pressed)
	}
}

func TestTextFieldFocusesFromItsPadding(t *testing.T) {
	useFakeIME(t)
	value := State("")
	f := TextField(value)
	var in inputState
	paintFrame(&in, f, Sz(200, 40))
	clickAt(&in, Pt(2, 2)) // in the padding, outside the editor's own rect
	if !f.Input().Focused() {
		t.Fatal("click in the padding did not focus the field")
	}
	if in.cursor != ebiten.CursorShapeText {
		t.Fatalf("cursor = %v over a text field, want text", in.cursor)
	}
}

func TestDividerFillsItsAxis(t *testing.T) {
	if got := Divider().Layout(Loose(Sz(120, 50)), Env{}); got != Sz(120, 1) {
		t.Fatalf("horizontal = %v", got)
	}
	if got := Divider().Vertical().Layout(Loose(Sz(120, 50)), Env{}); got != Sz(1, 50) {
		t.Fatalf("vertical = %v", got)
	}
}

func TestControlsPaintOnNilCanvas(t *testing.T) {
	useFakeIME(t)
	widgets := []Widget{
		Button("b", nil), Button("b", nil).Secondary(), ButtonOf(Box().Size(4, 4), nil),
		Checkbox(State(true), "c"), Radio(State(1), 1, "r"), Switch(State(true), "s"),
		Slider(State(0.5), 0, 1), TextField(State("x")), Divider(),
	}
	for _, w := range widgets {
		w.Paint(nil, Rct(Pt(0, 0), w.Layout(Loose(Sz(200, 50)), Env{})))
	}
}
