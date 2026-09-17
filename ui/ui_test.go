package ui_test

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestButtonTapsAndDisables(t *testing.T) {
	taps := 0
	b := ui.Button("go", func() { taps++ })
	p := ggui.NewProbe(b, ggui.Sz(100, 40))
	p.Click(ggui.Pt(10, 10))
	if taps != 1 {
		t.Fatalf("taps = %d, want 1", taps)
	}
	p.Move(ggui.Pt(10, 10))
	if p.Cursor() != ebiten.CursorShapePointer {
		t.Fatalf("cursor = %v over a button, want pointer", p.Cursor())
	}
	b.Disabled(true)
	p.Click(ggui.Pt(10, 10))
	if taps != 1 {
		t.Fatalf("taps = %d after a click on a disabled button, want 1", taps)
	}
	if p.Cursor() != ebiten.CursorShapeDefault {
		t.Fatalf("cursor = %v over a disabled button, want default", p.Cursor())
	}
}

func TestButtonSizesFromThemePadding(t *testing.T) {
	got := ui.Button("go", nil).Layout(ggui.Loose(ggui.Sz(300, 300)), ggui.Env{})
	text := ggui.Text("go").Layout(ggui.Loose(ggui.Sz(300, 300)), ggui.Env{})
	th := ggui.DefaultTheme()
	if got.W != text.W+th.Space*4 || got.H != text.H+th.Space*1.5 {
		t.Fatalf("button %v around text %v, want theme padding", got, text)
	}
	padded := ui.Button("go", nil).Pad(1).Layout(ggui.Loose(ggui.Sz(300, 300)), ggui.Env{})
	if padded.W != text.W+2 {
		t.Fatalf("Pad(1) gave width %v, want text + 2", padded.W)
	}
}

func TestCheckboxTogglesSignal(t *testing.T) {
	on := ggui.State(false)
	changed := []bool{}
	c := ui.Checkbox(on, "label").OnChange(func(b bool) { changed = append(changed, b) })
	p := ggui.NewProbe(c, ggui.Sz(200, 30))
	p.Click(ggui.Pt(5, 5))
	p.Click(ggui.Pt(60, 5)) // on the label
	if len(changed) != 2 || !changed[0] || changed[1] || on.Peek() {
		t.Fatalf("changed = %v, on = %v", changed, on.Peek())
	}
	size := c.Layout(ggui.Loose(ggui.Sz(200, 30)), ggui.Env{})
	label := ggui.Text("label").Layout(ggui.Loose(ggui.Sz(200, 30)), ggui.Env{})
	if size.W != 18+8+label.W {
		t.Fatalf("checkbox width %v, want glyph + gap + label %v", size.W, label.W)
	}
}

func TestRadioSelectsValue(t *testing.T) {
	choice := ggui.State("a")
	col := ggui.Column(ui.Radio(choice, "a", "A"), ui.Radio(choice, "b", "B")).Gap(4)
	p := ggui.NewProbe(col, ggui.Sz(100, 60))
	p.Click(ggui.Pt(5, 18+4+5))
	if choice.Peek() != "b" {
		t.Fatalf("choice = %q, want b", choice.Peek())
	}
}

func TestSwitchToggles(t *testing.T) {
	on := ggui.State(false)
	s := ui.Switch(on, "")
	p := ggui.NewProbe(s, ggui.Sz(100, 30))
	p.Click(ggui.Pt(5, 5))
	if !on.Peek() {
		t.Fatal("switch did not turn on")
	}
	if got := s.Layout(ggui.Loose(ggui.Sz(100, 30)), ggui.Env{}); got != ggui.Sz(36, 20) {
		t.Fatalf("switch size %v", got)
	}
}

func TestSliderDragsBeyondItsRect(t *testing.T) {
	v := ggui.State(0.0)
	s := ui.Slider(v, 0, 100).Step(10)
	const knob = 8
	p := ggui.NewProbe(s, ggui.Sz(100+2*knob, 20))
	p.Press(ggui.Pt(knob+50, 10))
	if v.Peek() != 50 {
		t.Fatalf("value = %v after pressing mid-track, want 50", v.Peek())
	}
	p.Move(ggui.Pt(knob+73, 10))
	if v.Peek() != 70 {
		t.Fatalf("value = %v after dragging to 73%%, want 70 with step 10", v.Peek())
	}
	p.Move(ggui.Pt(500, 300)) // dragged far outside
	if v.Peek() != 100 {
		t.Fatalf("value = %v after dragging past the end, want 100", v.Peek())
	}
	p.Release(ggui.Pt(500, 300))
	p.Move(ggui.Pt(500, 300))
	if v.Peek() != 100 {
		t.Fatalf("value = %v after release", v.Peek())
	}
}

func TestTextFieldFocusesFromItsPadding(t *testing.T) {
	value := ggui.State("")
	f := ui.TextField(value)
	p := ggui.NewProbe(f, ggui.Sz(200, 40))
	p.Click(ggui.Pt(2, 2)) // in the padding, outside the editor's own rect
	if !f.Input().Focused() || !p.Focused() {
		t.Fatal("click in the padding did not focus the field")
	}
	if p.Cursor() != ebiten.CursorShapeText {
		t.Fatalf("cursor = %v over a text field, want text", p.Cursor())
	}
	if got := f.Layout(ggui.Loose(ggui.Sz(300, 100)), ggui.Env{}); got.W != 300 || got.H <= 20 {
		t.Fatalf("TextField = %v, want full width and padded height", got)
	}
}

func TestDividerFillsItsAxis(t *testing.T) {
	if got := ui.Divider().Layout(ggui.Loose(ggui.Sz(120, 50)), ggui.Env{}); got != ggui.Sz(120, 1) {
		t.Fatalf("horizontal = %v", got)
	}
	if got := ui.Divider().Vertical().Layout(ggui.Loose(ggui.Sz(120, 50)), ggui.Env{}); got != ggui.Sz(1, 50) {
		t.Fatalf("vertical = %v", got)
	}
}

func TestControlsPaintOnNilCanvas(t *testing.T) {
	widgets := []ggui.Widget{
		ui.Button("b", nil), ui.Button("b", nil).Secondary(), ui.ButtonOf(ggui.Box().Size(4, 4), nil),
		ui.Checkbox(ggui.State(true), "c"), ui.Radio(ggui.State(1), 1, "r"), ui.Switch(ggui.State(true), "s"),
		ui.Slider(ggui.State(0.5), 0, 1), ui.TextField(ggui.State("x")), ui.Divider(),
	}
	for _, w := range widgets {
		w.Paint(nil, ggui.Rct(ggui.Pt(0, 0), w.Layout(ggui.Loose(ggui.Sz(200, 50)), ggui.Env{})))
	}
}

func TestSwitchKeepsSlidingAcrossRebuild(t *testing.T) {
	on := ggui.State(false)
	// The switch sits in a subtree that rebuilds when it is flipped, as a
	// theme toggle does.
	tree := ggui.Reactive(func() ggui.Widget {
		on.Get()
		return ui.Switch(on, "")
	})
	p := ggui.NewProbe(tree, ggui.Sz(100, 30))
	p.Frame()
	p.Click(ggui.Pt(5, 5))
	p.Frame() // the rebuilt switch adopts the old knob mid-slide
	p.Move(ggui.Pt(5, 5))
	if !on.Peek() {
		t.Fatal("switch did not turn on")
	}
	// A second flip right away must start from wherever the knob was,
	// which is not yet the far end.
	p.Click(ggui.Pt(5, 5))
	p.Frame()
	if on.Peek() {
		t.Fatal("switch did not turn off")
	}
}

func TestSliderDragSurvivesRebuild(t *testing.T) {
	v := ggui.State(0.0)
	tree := ggui.Reactive(func() ggui.Widget {
		v.Get() // every change rebuilds the slider
		return ui.Slider(v, 0, 100)
	})
	p := ggui.NewProbe(tree, ggui.Sz(116, 20))
	p.Press(ggui.Pt(8+20, 10))
	p.Move(ggui.Pt(8+60, 10))
	p.Move(ggui.Pt(8+90, 10))
	if v.Peek() != 90 {
		t.Fatalf("value = %v after dragging through rebuilds, want 90", v.Peek())
	}
}

func TestControlsWorkFromTheKeyboard(t *testing.T) {
	taps := 0
	on := ggui.State(false)
	v := ggui.State(50.0)
	tree := ggui.Column(
		ui.Button("go", func() { taps++ }),
		ui.Checkbox(on, "c"),
		ui.Slider(v, 0, 100).Step(5),
	).Gap(4)
	p := ggui.NewProbe(tree, ggui.Sz(200, 120))
	p.Type(ggui.Mods{}, ebiten.KeyTab, ebiten.KeyEnter)
	if taps != 1 {
		t.Fatalf("taps = %d after Tab, Enter; want 1", taps)
	}
	p.Type(ggui.Mods{}, ebiten.KeyTab, ebiten.KeySpace)
	if !on.Peek() {
		t.Fatal("Space did not toggle the focused checkbox")
	}
	p.Type(ggui.Mods{}, ebiten.KeyTab, ebiten.KeyArrowRight, ebiten.KeyArrowRight, ebiten.KeyArrowLeft)
	if v.Peek() != 55 {
		t.Fatalf("slider = %v after right, right, left; want 55", v.Peek())
	}
}
