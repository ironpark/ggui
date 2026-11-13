package ui_test

import (
	"slices"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestComboboxMultipleSearchToggleChipsAndKeyboard(t *testing.T) {
	t.Parallel()
	initial := []string{"Alpha"}
	values := ggui.State(initial)
	changes := 0
	combo := ui.ComboboxMultiple(values).Options([]string{"Alpha", "Beta", "Gamma"}).Name("Languages").OnChange(func(v []string) {
		changes++
		if len(v) > 0 {
			v[0] = "mutated callback"
		}
	})
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(combo)), ggui.Sz(420, 350))
	defer p.Close()
	if _, ok := p.Find("Remove Alpha"); !ok {
		t.Fatal("initial chip missing")
	}
	p.Tap("Languages")
	p.Move(ggui.Pt(410, 340))
	p.Text("bet")
	p.Frame()
	if _, ok := p.FindRole(ggui.RoleCheckbox, "Alpha"); ok {
		t.Fatal("search retained nonmatch")
	}
	p.Key("enter")
	if !slices.Equal(values.Get(), []string{"Alpha", "Beta"}) || !combo.Popup().IsOpen() {
		t.Fatalf("toggle=%v open=%v", values.Get(), combo.Popup().IsOpen())
	}
	if !slices.Equal(initial, []string{"Alpha"}) {
		t.Fatal("caller slice mutated")
	}
	p.Key("enter")
	if !slices.Equal(values.Get(), []string{"Alpha"}) {
		t.Fatalf("second Enter did not deselect: %v", values.Get())
	}
	p.Key("escape")
	p.Move(ggui.Pt(410, 340))
	p.Tap("Remove Alpha")
	if len(values.Get()) != 0 || changes != 3 {
		t.Fatalf("chip removal: values=%v changes=%d", values.Get(), changes)
	}
	p.Tap("Languages")
	p.Move(ggui.Pt(410, 340))
	p.Key("down", "enter")
	if !slices.Equal(values.Get(), []string{"Beta"}) {
		t.Fatalf("keyboard down selection=%v", values.Get())
	}
	p.Key("backspace")
	if len(values.Get()) != 0 {
		t.Fatal("empty-search Backspace did not remove final chip")
	}
}

func TestComboboxMultipleOptionsBindingsAndDisabled(t *testing.T) {
	t.Parallel()
	values := ggui.State([]string{"Outside"})
	options := ggui.State([]string{"Alpha", "Beta"})
	disabled := ggui.State(false)
	combo := ui.ComboboxMultiple(values).BindOptions(options).BindDisabled(disabled).Name("Pick")
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(combo)), ggui.Sz(350, 300))
	defer p.Close()
	p.Tap("Pick")
	p.Tap("Beta")
	if !slices.Equal(values.Get(), []string{"Outside", "Beta"}) {
		t.Fatalf("pointer selection=%v", values.Get())
	}
	options.Set([]string{"Gamma"})
	p.Frame()
	if _, ok := p.Find("Gamma"); !ok {
		t.Fatal("bound options failed to update open popup")
	}
	if !slices.Equal(values.Get(), []string{"Outside", "Beta"}) {
		t.Fatal("options changed selection")
	}
	disabled.Set(true)
	p.Frame()
	if combo.Popup().IsOpen() {
		t.Fatal("disabled popup remained open")
	}
	tree := p.Semantics()
	chip, _ := tree.Find(ggui.RoleButton, "Remove Beta")
	if !chip.Disabled {
		t.Fatal("disabled state did not reach chips")
	}
	if combo.Act(ggui.Action{Kind: ggui.ActionExpand}) {
		t.Fatal("disabled expansion accepted")
	}
}

func TestComboboxMultipleControlledSelectionDoesNotInventState(t *testing.T) {
	t.Parallel()
	var requested []string
	combo := ui.ComboboxMultiple(ggui.Controlled([]string{"Alpha"}, func(v []string) { requested = v })).Options([]string{"Alpha", "Beta"}).Name("Controlled")
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(combo)), ggui.Sz(350, 300))
	defer p.Close()
	p.Tap("Controlled")
	p.Tap("Beta")
	if !slices.Equal(requested, []string{"Alpha", "Beta"}) {
		t.Fatalf("request=%v", requested)
	}
	if _, ok := p.Find("Remove Beta"); ok {
		t.Fatal("unaccepted controlled selection became a chip")
	}
}

func TestComboboxMultipleChipsSitInsideItsField(t *testing.T) {
	t.Parallel()
	values := ggui.State([]string{"Alpha", "Beta"})
	combo := ui.ComboboxMultiple(values).Options([]string{"Alpha", "Beta", "Gamma"}).Name("Pick")
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(ggui.Box(combo).Width(300))), ggui.Sz(400, 300))
	defer p.Close()
	trigger, _ := p.Find("Pick")
	remove, _ := p.Find("Remove Beta")
	if remove.Rect.Origin.Y > trigger.Rect.Origin.Y+trigger.Rect.Size.H || remove.Rect.Origin.X > 300 {
		t.Fatalf("the chip at %v is outside the field's line at %v", remove.Rect, trigger.Rect)
	}
	p.Tap("Pick")
	tree := p.Semantics()
	if n, ok := tree.Find(ggui.RoleOption, "Beta"); !ok || !n.Selected || !(n.Checked == ggui.Tri(true)) {
		t.Fatalf("a chosen row is not checked:\n%s", tree)
	}
	if n, _ := tree.Find(ggui.RoleOption, "Gamma"); n.Checked == ggui.Tri(true) {
		t.Fatal("a row not chosen is checked")
	}
	list, _ := p.Find("Gamma")
	if list.Rect.Size.W > 300 {
		t.Fatalf("the list is wider than its field: %v", list.Rect)
	}
}

func TestComboboxMultipleTypesAfterItsChipsAndKeepsTheKeyboard(t *testing.T) {
	t.Parallel()
	values := ggui.State([]string{"Alpha"})
	combo := ui.ComboboxMultiple(values).Options([]string{"Alpha", "Beta", "Gamma"}).Name("Pick")
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(combo, ui.Button("After", nil))), ggui.Sz(400, 300))
	defer p.Close()
	p.Tap("Pick")
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	if got := focusedName(t, p); got != "Pick" {
		t.Fatalf("%q has the keyboard, not the input", got)
	}
	p.Text("am")
	if !combo.Popup().IsOpen() {
		t.Fatal("typing did not open the list")
	}
	if _, ok := p.Find("Beta"); ok {
		t.Fatal("a nonmatch is listed")
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	if !slices.Equal(values.Get(), []string{"Alpha", "Gamma"}) || !combo.Popup().IsOpen() {
		t.Fatalf("Enter toggled to %v, open=%v", values.Get(), combo.Popup().IsOpen())
	}
	if got := focusedName(t, p); got != "Pick" {
		t.Fatalf("%q has the keyboard", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	if combo.Popup().IsOpen() {
		t.Fatal("Escape did not close the list")
	}
	if n := node(t, p.Semantics(), ggui.RoleCombobox, "Pick"); n.Value != "am" || n.Description != "Alpha, Gamma" {
		t.Fatalf("after the first Escape the input holds %q, described %q", n.Value, n.Description)
	}
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	if n := node(t, p.Semantics(), ggui.RoleCombobox, "Pick"); n.Value != "" {
		t.Fatalf("the second Escape left %q", n.Value)
	}
	p.Text("x")
	p.Type(ggui.Mods{}, ggui.KeyTab)
	if combo.Popup().IsOpen() || node(t, p.Semantics(), ggui.RoleCombobox, "Pick").Value != "" {
		t.Fatal("leaving the input kept the list or what was typed")
	}
}
