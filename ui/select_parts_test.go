package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func TestSelectDisabledOptionsAreSkippedAndCannotBeChosen(t *testing.T) {
	t.Parallel()
	value := ggui.State("Alpha")
	box := ui.Select(value).Options([]string{"Alpha", "Beta", "Gamma"}).Name("Choice").
		DisabledOption(func(v string) bool { return v == "Beta" })
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(box)), ggui.Sz(300, 300))
	defer p.Close()
	// Down opens the list on the value, and from there the highlight steps
	// over the disabled option.
	p.Key("tab", "down", "down", "enter")
	if value.Get() != "Gamma" {
		t.Fatalf("Down from Alpha chose %q, want Gamma past the disabled Beta", value.Get())
	}
	p.Key("space")
	if n := node(t, p.Semantics(), ggui.RoleOption, "Beta"); !n.Disabled {
		t.Fatalf("the disabled option is not reported disabled: %+v", n)
	}
	// The disabled row sits between its neighbours and takes no click.
	above, below := find(t, p, ggui.RoleOption, "Alpha").Rect, find(t, p, ggui.RoleOption, "Gamma").Rect
	p.Click(ggui.Pt(above.Center().X, (above.Center().Y+below.Center().Y)/2))
	if value.Get() != "Gamma" || !box.Popup().IsOpen() {
		t.Fatalf("a click on a disabled option: value=%q open=%v", value.Get(), box.Popup().IsOpen())
	}
	// Open, the highlight steps over it too.
	p.Key("up", "enter")
	if value.Get() != "Alpha" {
		t.Fatalf("Up from Gamma chose %q, want Alpha", value.Get())
	}
	if box.Act(ggui.Action{Kind: ggui.ActionSelect, Text: "Beta"}) || value.Get() != "Alpha" {
		t.Fatal("a Select action chose a disabled option")
	}
}

func TestSelectGroupsAddHeadingsAndSeparatorsAndKeepAlignment(t *testing.T) {
	t.Parallel()
	th := uitheme.Default()
	options := []string{"Apple", "Banana", "Carrot", "Daikon"}
	kind := func(v string) string {
		if v == "Apple" || v == "Banana" {
			return "Fruit"
		}
		return "Vegetables"
	}
	height := func(box *ui.SelectWidget[string]) float64 {
		// Away from the window's edges, which a popup keeps 5px from and an
		// aligned list 10px from above and below: Base UI
		// holds an aligned Select's popup that far inside too.
		p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Box(ggui.Column(ggui.Box().Height(160), box)).Pad(0, 16)), ggui.Sz(300, 400))
		defer p.Close()
		p.Tap("Choice")
		p.Frame()
		trigger := find(t, p, ggui.RoleSelect, "Choice").Rect
		chosen := find(t, p, ggui.RoleOption, "Carrot").Rect
		if chosen.Center().Y != trigger.Center().Y {
			t.Fatalf("the chosen row is not over the trigger: row=%v trigger=%v", chosen, trigger)
		}
		// The row's label starts where the trigger's does.
		if got, want := chosen.Origin.X+th.ItemPad.Left, trigger.Origin.X+th.FieldPad.Left; got != want {
			t.Fatalf("the row's label starts at %v, the trigger's at %v", got, want)
		}
		return box.Popup().Rect().Size.H
	}
	flat := height(ui.Select(ggui.State("Carrot")).Options(options).Name("Choice"))
	grouped := height(ui.Select(ggui.State("Carrot")).Options(options).GroupBy(kind).Name("Choice"))
	// Two headings of a caption's line in the row's padding, and one
	// separator.
	least := 2*(th.Caption.Size+th.ItemPad.Vertical()) + th.PanelPad.Vertical() + th.BorderWidth
	if added := grouped - flat; added < least || added > least+2*th.Caption.Size {
		t.Fatalf("groups add %v to the list's height, want two headings and a separator, about %v", added, least)
	}
}

func TestSelectSmallPlaceholderAndInvalid(t *testing.T) {
	t.Parallel()
	th := uitheme.Default()
	env := th.Apply(ggui.Env{})
	loose := ggui.Loose(ggui.Sz(500, 300))
	options := []string{"One", "Two"}
	plain := ui.Select(ggui.State("One")).Options(options).Layout(loose, env)
	if small := ui.Select(ggui.State("One")).Options(options).Small().Layout(loose, env); plain.H-small.H != th.Space/2 || small.W != plain.W {
		t.Fatalf("a small select is %v, the default %v: want %v shorter", small, plain, th.Space/2)
	}
	if invalid := ui.Select(ggui.State("One")).Options(options).Invalid(true).Layout(loose, env); invalid != plain {
		t.Fatalf("an invalid select is %v, want the default's %v", invalid, plain)
	}
	// A placeholder wider than every option widens the field, since it is
	// what the field shows until a value is chosen.
	empty := ui.Select(ggui.State("")).Options(options).Placeholder("Choose a number of things").Layout(loose, env)
	if empty.W <= plain.W || empty.H != plain.H {
		t.Fatalf("a select showing a long placeholder is %v, the default %v", empty, plain)
	}
	// With a value among the options the placeholder is not shown, nor
	// measured.
	if chosen := ui.Select(ggui.State("One")).Options(options).Placeholder("Choose a number of things").Layout(loose, env); chosen != plain {
		t.Fatalf("a select with a value and a placeholder is %v, want %v", chosen, plain)
	}
}
