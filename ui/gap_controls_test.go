package ui_test

import (
	"slices"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestTogglePressedBindingAndDisabled(t *testing.T) {
	state := ggui.State(false)
	calls := 0
	toggle := ui.Toggle(state, "Bold").OnChange(func(bool) { calls++ })
	p := ggui.NewProbe(toggle, ggui.Sz(100, 40))
	defer p.Close()
	p.Tap("Bold")
	if !state.Get() || calls != 1 {
		t.Fatal("toggle did not update binding and callback")
	}
	if n := node(t, p.Semantics(), ggui.RoleButton, "Bold"); n.Checked != ggui.Tri(true) {
		t.Fatal("pressed state missing")
	}
	p.Type(ggui.Mods{}, ggui.KeySpace)
	if state.Get() || calls != 2 {
		t.Fatal("Space did not toggle")
	}
	at := node(t, p.Semantics(), ggui.RoleButton, "Bold").Rect.Center()
	toggle.Disabled(true)
	p.Click(at)
	if state.Get() || calls != 2 {
		t.Fatal("disabled toggle changed")
	}
}

func TestToggleGroupMultipleKeepsFocusSeparateFromSelection(t *testing.T) {
	state := ggui.State([]string{"bold"})
	var changed []string
	g := ui.ToggleGroupMultiple(state).Options([]string{"bold", "italic", "underline"}).Name("Formatting").OnValuesChange(func(v []string) { changed = v })
	p := ggui.NewProbe(g, ggui.Sz(360, 40))
	defer p.Close()
	p.Tap("italic")
	if !slices.Equal(state.Get(), []string{"bold", "italic"}) {
		t.Fatalf("selected=%v", state.Get())
	}
	changed[0] = "callback mutation"
	if state.Get()[0] != "bold" {
		t.Fatal("callback aliases binding")
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowRight)
	if len(state.Get()) != 2 {
		t.Fatal("arrow changed selection")
	}
	p.Type(ggui.Mods{}, ggui.KeySpace)
	if !slices.Equal(state.Get(), []string{"bold", "italic", "underline"}) {
		t.Fatalf("Space selected=%v", state.Get())
	}
	p.Tap("bold")
	if !slices.Equal(state.Get(), []string{"italic", "underline"}) {
		t.Fatalf("deselect=%v", state.Get())
	}
	at := node(t, p.Semantics(), ggui.RoleButton, "italic").Rect.Center()
	g.Disabled(true)
	g.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyArrowLeft})
	p.Click(at)
	if len(state.Get()) != 2 {
		t.Fatal("disabled group changed")
	}
}

func TestRTLControlsMirrorGeometryAndKeyboard(t *testing.T) {
	selected := ggui.State(0)
	tabs := ui.Tabs(selected, ui.Tab("First", ggui.Text("One")), ui.Tab("Second", ggui.Text("Two")))
	choice := ggui.State("A")
	group := ui.ToggleGroup(choice).Options([]string{"A", "B"})
	buttons := ui.ButtonGroup(ui.Button("Left logical", nil), ui.Button("Right logical", nil))
	p := ggui.NewProbe(ggui.Directionality(ggui.DirRTL, ggui.Column(tabs, group, buttons)), ggui.Sz(400, 200))
	defer p.Close()
	a, b := node(t, p.Semantics(), ggui.RoleTab, "First"), node(t, p.Semantics(), ggui.RoleTab, "Second")
	if a.Rect.Origin.X <= b.Rect.Origin.X {
		t.Fatal("RTL tabs did not reverse visual order")
	}
	a, b = node(t, p.Semantics(), ggui.RoleRadio, "A"), node(t, p.Semantics(), ggui.RoleRadio, "B")
	if a.Rect.Origin.X <= b.Rect.Origin.X {
		t.Fatal("RTL toggle group did not reverse visual order")
	}
	a, b = node(t, p.Semantics(), ggui.RoleButton, "Left logical"), node(t, p.Semantics(), ggui.RoleButton, "Right logical")
	if a.Rect.Origin.X <= b.Rect.Origin.X {
		t.Fatal("RTL button group did not reverse visual order")
	}
	tabs.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyArrowLeft})
	group.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyArrowLeft})
	tabs.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeySpace})
	if selected.Get() != 1 || choice.Get() != "B" {
		t.Fatal("RTL left arrow did not move to next logical item")
	}
}

func TestLabelFocusesFieldAndActivatesCheckbox(t *testing.T) {
	value := ggui.State("")
	checked := ggui.State(false)
	input := ui.TextField(value)
	box := ui.Checkbox(checked, "")
	p := ggui.NewProbe(ggui.Column(ui.Field("Email address", input), ui.Label("Accept terms").For(box), box), ggui.Sz(300, 180))
	defer p.Close()
	p.Tap("Email address")
	p.Text("hello")
	if value.Get() != "hello" {
		t.Fatalf("label did not focus editor: %q", value.Get())
	}
	p.Tap("Accept terms")
	if !checked.Get() {
		t.Fatal("label did not toggle checkbox")
	}
	at := node(t, p.Semantics(), ggui.RoleText, "Accept terms").Rect.Center()
	box.Disabled(true)
	p.Click(at)
	if !checked.Get() {
		t.Fatal("disabled associated control changed")
	}
}

func TestTabsManualVerticalAndDisabledPages(t *testing.T) {
	selected := ggui.State(0)
	tabs := ui.Tabs(selected,
		ui.Tab("First", ui.Button("First action", nil)),
		ui.Tab("Unavailable", ggui.Text("Unavailable")).Disabled(true),
		ui.Tab("Last", ui.Button("Last action", nil)),
	).Vertical().Line()
	p := ggui.NewProbe(tabs, ggui.Sz(400, 200))
	defer p.Close()
	tree := p.Semantics()
	first, last := node(t, tree, ggui.RoleTab, "First"), node(t, tree, ggui.RoleTab, "Last")
	body := node(t, tree, ggui.RoleButton, "First action")
	if last.Rect.Origin.Y <= first.Rect.Origin.Y || body.Rect.Origin.X <= first.Rect.Origin.X {
		t.Fatal("vertical geometry incorrect")
	}
	tabs.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyArrowDown})
	if selected.Get() != 0 {
		t.Fatal("manual arrow activated a panel")
	}
	tabs.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyEnter})
	if selected.Get() != 2 {
		t.Fatal("manual activation failed to skip disabled page")
	}
	disabled := node(t, p.Semantics(), ggui.RoleTab, "Unavailable")
	if !disabled.Disabled {
		t.Fatal("disabled state missing")
	}
	p.Perform(disabled.ID, ggui.Action{Kind: ggui.ActionSelect})
	if selected.Get() != 2 {
		t.Fatal("disabled page activated")
	}
	tabs.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyHome})
	if selected.Get() != 2 {
		t.Fatal("Home changed page in manual mode")
	}
	tabs.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeySpace})
	if selected.Get() != 0 {
		t.Fatal("Home/Space failed")
	}
}

func TestRTLResizablePointerAndKeyboardAgree(t *testing.T) {
	fraction := ggui.State(.5)
	split := ui.Resizable(fraction, ui.Button("First pane", nil), ui.Button("Second pane", nil)).Name("Split")
	p := ggui.NewProbe(ggui.Directionality(ggui.DirRTL, split), ggui.Sz(400, 200))
	defer p.Close()
	first, second := node(t, p.Semantics(), ggui.RoleButton, "First pane"), node(t, p.Semantics(), ggui.RoleButton, "Second pane")
	if first.Rect.Origin.X <= second.Rect.Origin.X {
		t.Fatal("RTL panes not mirrored")
	}
	handle, ok := p.Find("Split")
	if !ok {
		t.Fatal("no divider")
	}
	p.Press(handle.Center())
	p.Move(handle.Center().Add(ggui.Pt(-40, 0)))
	p.Release(handle.Center().Add(ggui.Pt(-40, 0)))
	if fraction.Get() <= .5 {
		t.Fatal("left drag did not grow RTL first pane")
	}
	before := fraction.Get()
	split.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyArrowLeft})
	if fraction.Get() <= before {
		t.Fatal("left key disagrees with drag")
	}
}

func TestAssociatedLabelOpensSelect(t *testing.T) {
	selected := ggui.State("A")
	selectBox := ui.Select(selected).Options([]string{"A", "B"})
	p := ggui.NewProbe(ui.Field("Choice", selectBox), ggui.Sz(300, 200))
	defer p.Close()
	p.Tap("Choice")
	if !selectBox.Popup().IsOpen() {
		t.Fatal("label did not activate select")
	}
	p.Tap("B")
	if selected.Get() != "B" {
		t.Fatal("label-opened select did not commit")
	}
}

func TestCompositeRovingFocusIsAnnouncedAndDoesNotTrapTab(t *testing.T) {
	selected := ggui.State(0)
	tabs := ui.Tabs(selected, ui.Tab("First", ui.Button("Page action", nil)), ui.Tab("Second", ggui.Text("Second page")))
	p := ggui.NewProbe(ggui.Column(tabs, ui.Button("After tabs", nil)), ggui.Sz(400, 160))
	defer p.Close()
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeyArrowRight)
	focused, ok := p.Semantics().Focused()
	if !ok || focused.Role != ggui.RoleTab || focused.Name != "Second" || selected.Get() != 0 {
		t.Fatalf("manual focus=%+v selected=%v", focused, selected.Get())
	}
	p.Type(ggui.Mods{}, ggui.KeyTab)
	if focused, ok = p.Semantics().Focused(); !ok || focused.Name != "Page action" {
		t.Fatalf("Tab did not leave list: %+v", focused)
	}
	second := node(t, p.Semantics(), ggui.RoleTab, "Second")
	p.Perform(second.ID, ggui.Action{Kind: ggui.ActionFocus})
	p.Type(ggui.Mods{}, ggui.KeySpace)
	if selected.Get() != 1 {
		t.Fatal("accessibility-focused tab did not activate")
	}
	values := ggui.State([]string{"A"})
	q := ggui.NewProbe(ggui.Column(ui.ToggleGroupMultiple(values).Options([]string{"A", "B"}), ui.Button("After group", nil)), ggui.Sz(300, 100))
	defer q.Close()
	q.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeyArrowRight)
	focused, ok = q.Semantics().Focused()
	if !ok || focused.Role != ggui.RoleButton || focused.Name != "B" || len(values.Get()) != 1 {
		t.Fatalf("multiple focus=%+v", focused)
	}
	q.Type(ggui.Mods{}, ggui.KeyTab)
	if focused, ok = q.Semantics().Focused(); !ok || focused.Name != "After group" {
		t.Fatalf("Tab did not leave group: %+v", focused)
	}
}

func TestLabelFocusesNumberEditor(t *testing.T) {
	p := ggui.NewProbe(ui.Field("Quantity", ui.NumberInput(ggui.State(2.0))), ggui.Sz(240, 100))
	defer p.Close()
	p.Tap("Quantity")
	focused, ok := p.Semantics().Focused()
	if !ok || focused.Role != ggui.RoleSpinButton || focused.Name != "Quantity" {
		t.Fatalf("number label focus=%+v", focused)
	}
}

func TestModalRejectedCloseDoesNotReportCompletion(t *testing.T) {
	for _, drawer := range []bool{false, true} {
		opened, accept := true, false
		requests, closes := 0, 0
		binding := ggui.Bind(func() bool { return opened }, func(v bool) {
			requests++
			if accept {
				opened = v
			}
		})
		var close func()
		if drawer {
			close = ui.Drawer(binding, ggui.Text("Body")).OnClose(func() { closes++ }).Close
		} else {
			close = ui.Dialog(binding, ggui.Text("Body")).OnClose(func() { closes++ }).Close
		}
		close()
		if !opened || requests != 1 || closes != 0 {
			t.Fatalf("rejected close: open=%v requests=%d closes=%d", opened, requests, closes)
		}
		accept = true
		close()
		close()
		if opened || requests != 2 || closes != 1 {
			t.Fatalf("accepted close: open=%v requests=%d closes=%d", opened, requests, closes)
		}
	}
}
