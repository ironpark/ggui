package ui_test

import (
	"testing"
	"time"

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

func TestSliderKeepsDraggingWhenItMoves(t *testing.T) {
	// The value it writes changes a widget above it, so every drag frame
	// moves the slider itself; capture must follow the widget, not its Rect.
	v := ggui.State(0.0)
	tree := ggui.Column(
		ggui.Reactive(func() ggui.Widget { return ggui.Box().Size(10, 10+v.Get()) }),
		ui.Slider(v, 0, 100),
	)
	p := ggui.NewProbe(tree, ggui.Sz(216, 300))
	p.Press(ggui.Pt(8+20, 20))
	first := v.Peek()
	p.Move(ggui.Pt(8+100, 20))
	p.Move(ggui.Pt(8+150, 20))
	if v.Peek() <= first+40 {
		t.Fatalf("value %v after dragging to the right from %v: the moved slider lost the drag", v.Peek(), first)
	}
}

func TestSelectOpensPicksAndClosesWithPointerAndKeys(t *testing.T) {
	v := ggui.State("b")
	changes := 0
	sel := ui.SelectStrings(v, "a", "b", "c").OnChange(func(string) { changes++ })
	tree := ggui.Column(ggui.Padding(sel, 10))
	p := ggui.NewProbe(tree, ggui.Sz(300, 300))
	field := p.Frame()
	_ = field
	p.Click(ggui.Pt(50, 20))
	if !sel.Popup().IsOpen() {
		t.Fatal("a click did not open the list")
	}
	p.Frame()
	list := sel.Popup().Rect()
	if list.Origin.Y < 20 || list.Size.W < 160 {
		t.Fatalf("list at %+v, want below the field and at least as wide", list)
	}
	// The third row is the last third of the list.
	rowH := list.Size.H / 3
	p.Click(ggui.Pt(list.Origin.X+20, list.Origin.Y+rowH*2.5))
	if v.Peek() != "c" || changes != 1 || sel.Popup().IsOpen() {
		t.Fatalf("value %q changes %d open %v after clicking the third row", v.Peek(), changes, sel.Popup().IsOpen())
	}
	// Keyboard: focus is on the field; Down while closed steps the value.
	p.Type(ggui.Mods{}, ebiten.KeyArrowDown)
	if v.Peek() != "a" {
		t.Fatalf("Down wrapped to %q, want a", v.Peek())
	}
	p.Type(ggui.Mods{}, ebiten.KeySpace, ebiten.KeyArrowDown, ebiten.KeyEnter)
	if v.Peek() != "b" || sel.Popup().IsOpen() {
		t.Fatalf("Space, Down, Enter gave %q open %v; want b and closed", v.Peek(), sel.Popup().IsOpen())
	}
	p.Type(ggui.Mods{}, ebiten.KeySpace)
	p.Click(ggui.Pt(250, 250))
	if sel.Popup().IsOpen() {
		t.Fatal("a click outside did not close the list")
	}
}

func TestMenuRunsItemsAndClosesOnEscape(t *testing.T) {
	ran := ""
	m := ui.Menu("File", ui.MenuItem("New", func() { ran = "new" }), ui.MenuDivider(), ui.MenuItem("Quit", func() { ran = "quit" }))
	p := ggui.NewProbe(ggui.Column(m), ggui.Sz(300, 300))
	p.Frame()
	p.Click(ggui.Pt(10, 10))
	if !m.Popup().IsOpen() {
		t.Fatal("click did not open the menu")
	}
	p.Frame()
	panel := m.Popup().Rect()
	if panel.Origin.Y < 20 {
		t.Fatalf("panel at %+v overlaps the button", panel)
	}
	p.Click(ggui.Pt(panel.Origin.X+10, panel.Origin.Y+12))
	if ran != "new" || m.Popup().IsOpen() {
		t.Fatalf("ran %q open %v after clicking the first item", ran, m.Popup().IsOpen())
	}
	p.Type(ggui.Mods{}, ebiten.KeyArrowDown, ebiten.KeyArrowDown, ebiten.KeyEnter)
	if ran != "quit" {
		t.Fatalf("Down, Down, Enter ran %q, want quit", ran)
	}
	p.Type(ggui.Mods{}, ebiten.KeyArrowDown, ebiten.KeyEscape)
	if m.Popup().IsOpen() {
		t.Fatal("Escape did not close the menu")
	}
}

func TestTabsSwitchByClickAndKeys(t *testing.T) {
	sel := ggui.State(0)
	var a, b ggui.Rect
	tabs := ui.Tabs(sel, ui.Tab("One", probe(80, 30, &a)), ui.Tab("Two", probe(80, 30, &b)))
	p := ggui.NewProbe(ggui.Column(tabs), ggui.Sz(300, 200))
	p.Frame()
	if a == (ggui.Rect{}) || b != (ggui.Rect{}) {
		t.Fatalf("first page %+v second %+v; want only the first painted", a, b)
	}
	// The second label sits right of the first; click near its middle.
	p.Click(ggui.Pt(80, 12))
	if sel.Peek() != 1 {
		t.Fatalf("selected %d after clicking the second label", sel.Peek())
	}
	a = ggui.Rect{}
	p.Frame()
	if b == (ggui.Rect{}) || a != (ggui.Rect{}) {
		t.Fatal("second page not shown alone after the switch")
	}
	p.Type(ggui.Mods{}, ebiten.KeyArrowRight)
	if sel.Peek() != 0 {
		t.Fatalf("Right wrapped to %d, want 0", sel.Peek())
	}
	p.Type(ggui.Mods{}, ebiten.KeyEnd)
	if sel.Peek() != 1 {
		t.Fatalf("End went to %d, want 1", sel.Peek())
	}
}

func TestCollapsibleTogglesAndHidesContent(t *testing.T) {
	open := ggui.State(false)
	var body ggui.Rect
	c := ui.Collapsible(open, "Details", probe(80, 40, &body))
	p := ggui.NewProbe(ggui.Column(c), ggui.Sz(300, 200))
	p.Frame()
	closed := c.Layout(ggui.Loose(ggui.Sz(300, 200)), ggui.Env{})
	if body != (ggui.Rect{}) {
		t.Fatal("content painted while closed")
	}
	p.Click(ggui.Pt(20, 10))
	if !open.Peek() {
		t.Fatal("click on the header did not open")
	}
	p.Frame()
	opened := c.Layout(ggui.Loose(ggui.Sz(300, 200)), ggui.Env{})
	if body == (ggui.Rect{}) || opened.H < closed.H+40 {
		t.Fatalf("content %+v, height %v -> %v; want content shown below the header", body, closed.H, opened.H)
	}
	p.Type(ggui.Mods{}, ebiten.KeySpace)
	if open.Peek() {
		t.Fatal("Space did not close")
	}
}

func TestCardBadgeProgressLayout(t *testing.T) {
	v := ggui.State(0.5)
	tree := ggui.Column(ui.Card(ggui.Text("x")), ui.Badge("new").Accent(), ui.Progress(v))
	p := ggui.NewProbe(tree, ggui.Sz(200, 200))
	if s := p.Frame(); s.W != 200 {
		t.Fatalf("size %v", s)
	}
	if got := ui.Progress(v).Layout(ggui.Loose(ggui.Sz(120, 100)), ggui.Env{}); got != ggui.Sz(120, 6) {
		t.Fatalf("progress %v, want 120x6", got)
	}
}

// probe is a fixed-size leaf that records where it was painted.
func probe(w, h float64, got *ggui.Rect) ggui.Widget {
	return ggui.FromFuncs(
		func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(w, h)) },
		func(_ *ggui.Canvas, r ggui.Rect) { *got = r },
	)
}

func TestSwitchKnobEasesOnTheClock(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	defer ggui.SetClock(func() time.Time { return now })()
	on := ggui.State(false)
	// Rebuilt on every flip: the knob's motion must live in the Canvas,
	// not the widget.
	tree := ggui.Reactive(func() ggui.Widget { on.Get(); return ui.Switch(on, "") })
	p := ggui.NewProbe(tree, ggui.Sz(100, 30))
	p.Frame()
	p.Click(ggui.Pt(5, 5))
	if !on.Peek() {
		t.Fatal("switch did not turn on")
	}
	p.Frame()
	now = now.Add(50 * time.Millisecond)
	p.Frame()
	p.Click(ggui.Pt(5, 5)) // flips back from mid-slide
	if on.Peek() {
		t.Fatal("switch did not turn off")
	}
}
