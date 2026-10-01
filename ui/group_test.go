package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
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

func TestToggleGroupVerticalMovesWithEveryArrow(t *testing.T) {
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
		{ggui.KeyArrowLeft, "b"},
		{ggui.KeyHome, "a"},
		{ggui.KeyEnd, "c"},
	} {
		p.Type(ggui.Mods{}, step.key)
		if got := ggui.Untrack(value.Get); got != step.want {
			t.Fatalf("after %v the group chose %q, want %q", step.key, got, step.want)
		}
	}
	p.Type(ggui.Mods{}, ggui.KeySpace) // re-picking the chosen segment is no change
	if len(changes) != 6 {
		t.Fatalf("OnChange ran %d times, want once per change (6)", len(changes))
	}
}
