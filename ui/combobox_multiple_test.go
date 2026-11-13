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
