package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
)

func TestButtonGroupVerticalStacksAndNamesTheToolbar(t *testing.T) {
	t.Parallel()
	name := ggui.State("Clipboard")
	group := ui.ButtonGroup(ui.Button("Copy", func() {}).Ghost(), ui.Button("Paste", func() {}).Ghost()).Vertical().BindName(name)
	p := ggui.NewProbe(ggui.Row(group), ggui.Sz(300, 200))
	defer p.Close()
	copyB := find(t, p, ggui.RoleButton, "Copy").Rect
	paste := find(t, p, ggui.RoleButton, "Paste").Rect
	if paste.Origin.Y <= copyB.Origin.Y || paste.Origin.X != copyB.Origin.X || paste.Size.W != copyB.Size.W {
		t.Fatalf("vertical group placed Copy at %v and Paste at %v; want Paste stretched below Copy", copyB, paste)
	}
	if _, ok := p.Semantics().Find(ggui.RoleToolbar, "Clipboard"); !ok {
		t.Fatalf("no toolbar named Clipboard:\n%s", p.Semantics())
	}
	name.Set("Edit")
	if _, ok := p.Semantics().Find(ggui.RoleToolbar, "Edit"); !ok {
		t.Fatal("the toolbar's name did not follow its reader")
	}
	group.Name("Fixed")
	name.Set("Ignored")
	if _, ok := p.Semantics().Find(ggui.RoleToolbar, "Fixed"); !ok {
		t.Fatal("Name did not detach the reader")
	}
}

// A vertical group's keyboard moves with Up and Down, Home and End, not
// with Left and Right, and only Space or Enter changes the choice: Base
// UI's ToggleGroup is a CompositeRoot on the group's orientation whose
// items are pressed, not chosen by focus.
func TestToggleGroupVerticalMovesWithUpAndDown(t *testing.T) {
	t.Parallel()
	value := ggui.State("b")
	var changes []string
	g := ui.ToggleGroup(value).Options([]string{"a", "b", "c"}).Vertical().Name("Align").OnChange(func(s string) { changes = append(changes, s) })
	p := ggui.NewProbe(ggui.Row(g), ggui.Sz(300, 300))
	defer p.Close()
	a := find(t, p, ggui.RoleRadio, "a").Rect
	b := find(t, p, ggui.RoleRadio, "b").Rect
	if b.Origin.Y <= a.Origin.Y || b.Origin.X != a.Origin.X {
		t.Fatalf("vertical segments at %v and %v, want b below a", a, b)
	}
	group, ok := p.Semantics().Find(ggui.RoleGroup, "Align")
	if !ok {
		t.Fatalf("no group named Align:\n%s", p.Semantics())
	}
	p.Perform(group.ID, ggui.Action{Kind: ggui.ActionFocus})
	for _, step := range []struct {
		key  ggui.KeyboardKey
		want string
	}{
		{ggui.KeyArrowDown, "c"},
		{ggui.KeyArrowDown, "a"}, // wraps
		{ggui.KeyArrowUp, "c"},
		{ggui.KeyArrowLeft, "c"}, // not this group's axis
		{ggui.KeyHome, "a"},
		{ggui.KeyEnd, "c"},
	} {
		was := ggui.Untrack(value.Get)
		p.Type(ggui.Mods{}, step.key)
		if got := ggui.Untrack(value.Get); got != was {
			t.Fatalf("%v alone changed the choice to %q", step.key, got)
		}
		p.Type(ggui.Mods{}, ggui.KeySpace)
		if got := ggui.Untrack(value.Get); got != step.want {
			t.Fatalf("after %v and Space the group chose %q, want %q", step.key, got, step.want)
		}
	}
	p.Type(ggui.Mods{}, ggui.KeySpace) // re-picking the chosen segment is no change
	if len(changes) != 5 {
		t.Fatalf("OnChange ran %d times (%v), want once per change (5)", len(changes), changes)
	}
}

// An Optional group clears its choice when the chosen option is pressed
// again, by the pointer or the keyboard, as Base UI's single ToggleGroup
// does; without it the choice stays.
func TestToggleGroupOptionalClearsOnASecondPress(t *testing.T) {
	t.Parallel()
	kept, cleared := ggui.State("b"), ggui.State("b")
	var changes []string
	root := ggui.Column(
		ui.ToggleGroup(kept).Options([]string{"a", "b"}).Name("Kept").Format(func(s string) string { return "kept " + s }),
		ui.ToggleGroup(cleared).Options([]string{"a", "b"}).Name("Cleared").Format(func(s string) string { return "cleared " + s }).
			Optional().OnChange(func(s string) { changes = append(changes, s) }),
	)
	p := ggui.NewProbe(root, ggui.Sz(300, 300))
	defer p.Close()
	p.Tap("kept b")
	p.Tap("cleared b")
	if kept.Get() != "b" || cleared.Get() != "" {
		t.Fatalf("a second press: kept=%q cleared=%q, want b and none", kept.Get(), cleared.Get())
	}
	if n, ok := p.Semantics().Find(ggui.RoleRadio, "cleared b"); !ok || n.Checked == ggui.TriOn {
		t.Fatal("the cleared option is still marked")
	}
	// The keyboard stays on the option it pressed: Space chooses it again,
	// and once more clears it.
	p.Type(ggui.Mods{}, ggui.KeySpace)
	if cleared.Get() != "b" {
		t.Fatalf("Space on the cleared option chose %q, want b", cleared.Get())
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	p.Type(ggui.Mods{}, ggui.KeyArrowLeft, ggui.KeySpace)
	if cleared.Get() != "a" {
		t.Fatalf("Enter, Left, Space chose %q, want a", cleared.Get())
	}
	if want := []string{"", "b", "", "a"}; len(changes) != len(want) {
		t.Fatalf("OnChange saw %q, want %q", changes, want)
	}
}

func TestToggleGroupIcons(t *testing.T) {
	t.Parallel()
	value := ggui.State("grid")
	opts := []string{"grid", "list"}
	glyph := func(string) *icons.Widget { return ui.Icon(icons.Search) }
	labelled := ui.ToggleGroup(value).Options(opts).Icon(glyph)
	bare := ui.ToggleGroup(value).Options(opts)
	only := ui.ToggleGroup(value).Options(opts).Icon(glyph).IconOnly()
	p := ggui.NewProbe(ggui.Column(labelled, bare, only), ggui.Sz(400, 300))
	defer p.Close()
	p.Frame()
	var widths []float64
	for _, f := range p.FindAll(ggui.RoleRadio) {
		if f.Label == "list" {
			widths = append(widths, f.Rect.Size.W)
		}
	}
	if len(widths) != 3 {
		t.Fatalf("segments named list: %v", widths)
	}
	if !(widths[0] > widths[1] && widths[2] < widths[1]) {
		t.Fatalf("widths with icon %v, label alone %v, icon alone %v", widths[0], widths[1], widths[2])
	}
	p.Tap("list")
	if ggui.Peek(value) != "list" {
		t.Fatalf("value %q", ggui.Peek(value))
	}
}
