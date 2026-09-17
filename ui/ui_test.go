package ui_test

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// find returns the region with role and label, or fails the test.
func find(t *testing.T, p *ggui.Probe, role ggui.Role, label string) ggui.Found {
	t.Helper()
	f, ok := p.FindRole(role, label)
	if !ok {
		t.Fatalf("no %s labelled %q", role, label)
	}
	return f
}

func TestButtonTapsAndDisables(t *testing.T) {
	taps := 0
	b := ui.Button("go", func() { taps++ })
	p := ggui.NewProbe(b, ggui.Sz(100, 40))
	defer p.Close()
	p.Tap("go")
	if taps != 1 {
		t.Fatalf("taps = %d, want 1", taps)
	}
	at := find(t, p, ggui.RoleButton, "go").Center()
	p.Move(at)
	if p.Cursor() != ebiten.CursorShapePointer {
		t.Fatalf("cursor = %v over a button, want pointer", p.Cursor())
	}
	b.Disabled(true)
	p.Click(at)
	if taps != 1 {
		t.Fatalf("taps = %d after a click on a disabled button, want 1", taps)
	}
	if p.Cursor() != ebiten.CursorShapeDefault {
		t.Fatalf("cursor = %v over a disabled button, want default", p.Cursor())
	}
	if _, ok := p.Find("go"); ok {
		t.Fatal("a disabled button is still found: it registers no region")
	}
}

func TestButtonSizesFromThemePadding(t *testing.T) {
	got := ui.Button("go", nil).Layout(ggui.Loose(ggui.Sz(300, 300)), ggui.Env{})
	text := ggui.Text("go").Layout(ggui.Loose(ggui.Sz(300, 300)), ggui.Env{})
	th := ggui.DefaultTheme()
	if got.W != text.W+th.ButtonPad.Left+th.ButtonPad.Right || got.H != text.H+th.ButtonPad.Top+th.ButtonPad.Bottom {
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
	defer p.Close()
	f := find(t, p, ggui.RoleCheckbox, "label")
	p.Click(f.Rect.Origin.Add(ggui.Pt(2, 2)))                                          // on the glyph
	p.Click(ggui.Pt(f.Rect.Origin.X+f.Rect.Size.W-2, f.Rect.Origin.Y+f.Rect.Size.H/2)) // on the label's end
	if len(changed) != 2 || !changed[0] || changed[1] || on.Peek() {
		t.Fatalf("changed = %v, on = %v", changed, on.Peek())
	}
	size := c.Layout(ggui.Loose(ggui.Sz(200, 30)), ggui.Env{})
	label := ggui.Text("label").Layout(ggui.Loose(ggui.Sz(200, 30)), ggui.Env{})
	th := ggui.DefaultTheme()
	if size.W != th.ControlSize+th.ControlGap+label.W {
		t.Fatalf("checkbox width %v, want glyph + gap + label %v", size.W, label.W)
	}
}

func TestRadioSelectsValue(t *testing.T) {
	choice := ggui.State("a")
	col := ggui.Column(ui.Radio(choice, "a", "A"), ui.Radio(choice, "b", "B")).Gap(4)
	p := ggui.NewProbe(col, ggui.Sz(100, 60))
	defer p.Close()
	p.Tap("B")
	if choice.Peek() != "b" {
		t.Fatalf("choice = %q, want b", choice.Peek())
	}
}

func TestSwitchToggles(t *testing.T) {
	on := ggui.State(false)
	s := ui.Switch(on, "")
	p := ggui.NewProbe(s, ggui.Sz(100, 30))
	defer p.Close()
	p.Click(find(t, p, ggui.RoleSwitch, "").Center())
	if !on.Peek() {
		t.Fatal("switch did not turn on")
	}
	if got := s.Layout(ggui.Loose(ggui.Sz(100, 30)), ggui.Env{}); got != ggui.Sz(36, 20) {
		t.Fatalf("switch size %v", got)
	}
}

// onTrack returns the point at fraction f along a slider's track.
func onTrack(s ggui.Found, f float64) ggui.Point {
	const knob = 8
	return ggui.Pt(s.Rect.Origin.X+knob+(s.Rect.Size.W-2*knob)*f, s.Center().Y)
}

func TestSliderDragsBeyondItsRect(t *testing.T) {
	v := ggui.State(0.0)
	s := ui.Slider(v, 0, 100).Step(10)
	p := ggui.NewProbe(s, ggui.Sz(100+2*8, 20))
	defer p.Close()
	track := find(t, p, ggui.RoleSlider, "")
	p.Press(onTrack(track, 0.5))
	if v.Peek() != 50 {
		t.Fatalf("value = %v after pressing mid-track, want 50", v.Peek())
	}
	p.Move(onTrack(track, 0.73))
	if v.Peek() != 70 {
		t.Fatalf("value = %v after dragging to 73%%, want 70 with step 10", v.Peek())
	}
	far := onTrack(track, 5).Add(ggui.Pt(0, 300)) // dragged far outside
	p.Move(far)
	if v.Peek() != 100 {
		t.Fatalf("value = %v after dragging past the end, want 100", v.Peek())
	}
	p.Release(far)
	p.Move(far)
	if v.Peek() != 100 {
		t.Fatalf("value = %v after release", v.Peek())
	}
}

func TestTextFieldFocusesFromItsPadding(t *testing.T) {
	value := ggui.State("")
	f := ui.TextField(value).Placeholder("name")
	p := ggui.NewProbe(f, ggui.Sz(200, 40))
	defer p.Close()
	box := find(t, p, ggui.RoleTextField, "name")
	p.Click(box.Rect.Origin.Add(ggui.Pt(2, 2))) // in the padding, outside the editor's own rect
	if !f.Input().Focused() || !p.Focused() {
		t.Fatal("click in the padding did not focus the field")
	}
	if p.Cursor() != ebiten.CursorShapeText {
		t.Fatalf("cursor = %v over a text field, want text", p.Cursor())
	}
	if got := f.Layout(ggui.Loose(ggui.Sz(300, 100)), ggui.Env{}); got.W != 300 || got.H <= 20 {
		t.Fatalf("TextField = %v, want full width and padded height", got)
	}
	f.Named("Name")
	if _, ok := p.FindRole(ggui.RoleTextField, "Name"); !ok {
		t.Fatal("Named did not rename the field")
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
		ui.Button("b", nil), ui.Button("b", nil).Outline(), ui.ButtonOf(ggui.Box().Size(4, 4), nil),
		ui.Checkbox(ggui.State(true), "c"), ui.Radio(ggui.State(1), 1, "r"), ui.Switch(ggui.State(true), "s"),
		ui.Slider(ggui.State(0.5), 0, 1), ui.TextField(ggui.State("x")), ui.Divider(),
		ui.Dialog(ggui.State(true), ggui.Text("d")).Title("t"),
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
		return ui.Switch(on, "Dark")
	})
	p := ggui.NewProbe(tree, ggui.Sz(100, 30))
	defer p.Close()
	p.Tap("Dark")
	p.Frame() // the rebuilt switch adopts the old knob mid-slide
	if !on.Peek() {
		t.Fatal("switch did not turn on")
	}
	// A second flip right away must start from wherever the knob was,
	// which is not yet the far end.
	p.Tap("Dark")
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
	defer p.Close()
	track := find(t, p, ggui.RoleSlider, "")
	p.Press(onTrack(track, 0.2))
	p.Move(onTrack(track, 0.6))
	p.Move(onTrack(track, 0.9))
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
	defer p.Close()
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
	defer p.Close()
	track := find(t, p, ggui.RoleSlider, "")
	p.Press(onTrack(track, 0.1))
	first := v.Peek()
	p.Move(onTrack(track, 0.5))
	p.Move(onTrack(track, 0.75))
	if v.Peek() <= first+40 {
		t.Fatalf("value %v after dragging to the right from %v: the moved slider lost the drag", v.Peek(), first)
	}
}

func TestSelectOpensPicksAndClosesWithPointerAndKeys(t *testing.T) {
	v := ggui.State("b")
	changes := 0
	sel := ui.Select(v, []string{"a", "b", "c"}).Named("letter").OnChange(func(string) { changes++ })
	tree := ggui.Column(ggui.Padding(sel, 10))
	p := ggui.NewProbe(tree, ggui.Sz(300, 300))
	defer p.Close()
	field := find(t, p, ggui.RoleSelect, "letter")
	p.Tap("letter")
	if !sel.Popup().IsOpen() {
		t.Fatal("a click did not open the list")
	}
	p.Frame()
	list := sel.Popup().Rect()
	if list.Origin.Y < field.Rect.Origin.Y+field.Rect.Size.H || list.Size.W < field.Rect.Size.W {
		t.Fatalf("list at %+v, want below the field %+v and at least as wide", list, field.Rect)
	}
	if opts := p.FindAll(ggui.RoleOption); len(opts) != 3 {
		t.Fatalf("%d options found, want 3", len(opts))
	}
	p.Tap("c")
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
	size := p.Frame()
	p.Click(ggui.Pt(size.W-10, size.H-10)) // the far corner, outside the list
	if sel.Popup().IsOpen() {
		t.Fatal("a click outside did not close the list")
	}
}

func TestMenuRunsItemsAndClosesOnEscape(t *testing.T) {
	ran := ""
	m := ui.Menu("File", ui.MenuItem("New", func() { ran = "new" }), ui.MenuDivider(), ui.MenuItem("Quit", func() { ran = "quit" }))
	p := ggui.NewProbe(ggui.Column(m), ggui.Sz(300, 300))
	defer p.Close()
	button := find(t, p, ggui.RoleMenu, "File")
	p.Tap("File")
	if !m.Popup().IsOpen() {
		t.Fatal("click did not open the menu")
	}
	item := find(t, p, ggui.RoleMenuItem, "New")
	if item.Rect.Origin.Y < button.Rect.Origin.Y+button.Rect.Size.H {
		t.Fatalf("item at %+v overlaps the button %+v", item.Rect, button.Rect)
	}
	p.Tap("New")
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
	defer p.Close()
	p.Frame()
	if a == (ggui.Rect{}) || b != (ggui.Rect{}) {
		t.Fatalf("first page %+v second %+v; want only the first painted", a, b)
	}
	p.Tap("Two")
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
	defer p.Close()
	p.Frame()
	closed := c.Layout(ggui.Loose(ggui.Sz(300, 200)), ggui.Env{})
	if body != (ggui.Rect{}) {
		t.Fatal("content painted while closed")
	}
	p.Tap("Details")
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
	defer p.Close()
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
	on := ggui.State(false)
	// Rebuilt on every flip: the knob's motion must live in the Canvas,
	// not the widget.
	tree := ggui.Reactive(func() ggui.Widget { on.Get(); return ui.Switch(on, "Dark") })
	p := ggui.NewProbe(tree, ggui.Sz(100, 30))
	defer p.Close()
	p.Tap("Dark")
	if !on.Peek() {
		t.Fatal("switch did not turn on")
	}
	p.Advance(50 * time.Millisecond)
	p.Tap("Dark") // flips back from mid-slide
	if on.Peek() {
		t.Fatal("switch did not turn off")
	}
}

func TestMenuKeysSkipDisabledItems(t *testing.T) {
	ran := ""
	m := ui.Menu("File",
		ui.MenuItem("New", func() { ran = "new" }).Disabled(true),
		ui.MenuItem("Open", func() { ran = "open" }),
		ui.MenuItem("Quit", func() { ran = "quit" }).Disabled(true),
	)
	p := ggui.NewProbe(ggui.Column(m), ggui.Sz(300, 300))
	defer p.Close()
	p.Tap("File") // focus and open
	p.Type(ggui.Mods{}, ebiten.KeyArrowDown, ebiten.KeyEnter)
	if ran != "open" {
		t.Fatalf("Down, Enter ran %q, want open (the first enabled item)", ran)
	}
	p.Type(ggui.Mods{}, ebiten.KeyArrowUp, ebiten.KeyEnter)
	if ran != "open" {
		t.Fatalf("Up, Enter ran %q, want open (the only enabled item, wrapping)", ran)
	}
}

func TestDisabledTextFieldTakesNoInput(t *testing.T) {
	v := ggui.State("")
	f := ui.TextField(v).Disabled(true)
	p := ggui.NewProbe(f, ggui.Sz(200, 40))
	defer p.Close()
	size := p.Frame()
	p.Click(ggui.Pt(size.W/2, size.H/2))
	p.Text("x")
	if v.Peek() != "" || p.Focused() {
		t.Fatalf("value %q focused %v after clicking and typing into a disabled field", v.Peek(), p.Focused())
	}
	if p.Cursor() != ebiten.CursorShapeDefault {
		t.Fatalf("cursor = %v over a disabled field, want default", p.Cursor())
	}
}

func TestSliderReportsChanges(t *testing.T) {
	v := ggui.State(0.0)
	var got []float64
	s := ui.Slider(v, 0, 100).Step(10).Named("volume").OnChange(func(x float64) { got = append(got, x) })
	p := ggui.NewProbe(s, ggui.Sz(116, 20))
	defer p.Close()
	p.Click(onTrack(find(t, p, ggui.RoleSlider, "volume"), 0.5))
	p.Type(ggui.Mods{}, ebiten.KeyArrowRight, ebiten.KeyArrowRight)
	if len(got) != 3 || got[0] != 50 || got[2] != 70 {
		t.Fatalf("OnChange saw %v, want [50 60 70]", got)
	}
}

func TestRadiosSelectAndReport(t *testing.T) {
	sel := ggui.State("a")
	var got string
	g := ui.Radios(sel, []string{"a", "b", "c"}).Vertical().OnChange(func(s string) { got = s })
	p := ggui.NewProbe(g, ggui.Sz(100, 100))
	defer p.Close()
	p.Tap("c")
	if sel.Peek() != "c" || got != "c" {
		t.Fatalf("selected %q reported %q after clicking the last radio, want c", sel.Peek(), got)
	}
	if radios := p.FindAll(ggui.RoleRadio); len(radios) != 3 {
		t.Fatalf("%d radios found, want 3", len(radios))
	}
}

func TestDialogKeyboardFlow(t *testing.T) {
	open := ggui.State(false)
	deleted := false
	tree := ggui.Column(
		ui.Button("Open", func() { open.Set(true) }),
		ui.Button("Other", nil),
		ui.Dialog(open, ggui.Row(
			ui.Button("Delete", func() { deleted = true; open.Set(false) }),
			ui.Button("Cancel", func() { open.Set(false) }).Outline(),
		)).Title("Confirm"),
	)
	p := ggui.NewProbe(tree, ggui.Sz(400, 300))
	defer p.Close()
	p.Type(ggui.Mods{}, ebiten.KeyTab, ebiten.KeyEnter) // focus Open, press it
	if !open.Peek() {
		t.Fatal("Enter on the focused button did not open the dialog")
	}
	dialog := find(t, p, ggui.RoleDialog, "Confirm")
	if size := p.Frame(); dialog.Center().X != size.W/2 {
		t.Fatalf("dialog at %+v is not centered in %v", dialog.Rect, size)
	}
	// Focus moved to the first button inside; Tab cycles inside the dialog
	// and never reaches Other.
	p.Type(ggui.Mods{}, ebiten.KeyTab, ebiten.KeyTab, ebiten.KeyEnter)
	if !deleted || open.Peek() {
		t.Fatalf("deleted %v open %v after Tab, Tab, Enter; want Delete pressed", deleted, open.Peek())
	}
	// Focus returned to the opener.
	p.Type(ggui.Mods{}, ebiten.KeyEnter)
	if !open.Peek() {
		t.Fatal("focus did not return to the opener when the dialog closed")
	}
	p.Type(ggui.Mods{}, ebiten.KeyEscape)
	if open.Peek() {
		t.Fatal("Escape did not close the dialog")
	}
	open.Set(true)
	p.Frame()
	size := p.Frame()
	p.Click(ggui.Pt(size.W-5, size.H-5)) // on the scrim
	if open.Peek() {
		t.Fatal("a click on the scrim did not close the dialog")
	}
}

func TestFieldNamesAndReportsErrors(t *testing.T) {
	email := ggui.State("")
	errText := ggui.State("")
	f := ui.Field("Email", ui.TextField(email)).Help("Work address").Error(errText)
	p := ggui.NewProbe(f, ggui.Sz(300, 100))
	defer p.Close()
	if _, ok := p.FindRole(ggui.RoleTextField, "Email"); !ok {
		t.Fatal("the field's label did not name the input")
	}
	loose := ggui.Loose(ggui.Sz(300, 300))
	plain := f.Layout(loose, ggui.Env{})
	errText.Set("Required")
	withErr := f.Layout(loose, ggui.Env{})
	if withErr.H != plain.H {
		t.Fatalf("height %v with an error, %v with help; the error takes the help's line", withErr.H, plain.H)
	}
	if bare := ui.Field("Name", ui.TextField(email)).Layout(loose, ggui.Env{}); bare.H >= plain.H {
		t.Fatalf("a field without help is %v tall, with help %v", bare.H, plain.H)
	}
}
