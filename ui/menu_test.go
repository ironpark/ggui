package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// openMenu taps the trigger named label and lets the popup open.
func openMenu(p *ggui.Probe, label string) {
	p.Tap(label)
	p.Advance(time.Second)
}

func TestMenuItemCheckMarksFollowTheirBinding(t *testing.T) {
	t.Parallel()
	// The mark is drawn before the label, so a checked item asks for more room.
	env := uitheme.Default().Apply(ggui.Env{})
	width := func(it *ui.MenuItemWidget) float64 {
		return it.Layout(ggui.Loose(ggui.Sz(ggui.Unbounded, 100)), env).W
	}
	plain := width(ui.MenuItem("Grid", nil))
	if checked := width(ui.MenuItem("Grid", nil).Checked(true)); checked <= plain {
		t.Fatalf("a checked item is %v wide, not wider than the plain %v", checked, plain)
	}
	if cleared := width(ui.MenuItem("Grid", nil).Checked(true).Checked(false)); cleared != plain {
		t.Fatalf("an unchecked item is %v wide, want the plain %v", cleared, plain)
	}
	grid := ggui.State(false)
	bound := ui.MenuItem("Grid", nil).BindChecked(grid)
	if got := width(bound); got != plain {
		t.Fatalf("a bound item reading false is %v wide, want %v", got, plain)
	}
	grid.Set(true)
	if got := width(bound); got <= plain {
		t.Fatalf("a bound item reading true is %v wide, want the check mark", got)
	}
	menu := ui.Menu("View", ui.MenuItem("Grid", nil).Checked(true))
	p := ggui.NewProbe(menu, ggui.Sz(300, 300))
	defer p.Close()
	openMenu(p, "View")
	if _, ok := p.Semantics().Find(ggui.RoleMenuItem, "Grid"); !ok {
		t.Fatalf("the check mark leaked into the item's accessible name:\n%s", p.Semantics())
	}
}

func TestMenuWidthSizesThePanel(t *testing.T) {
	t.Parallel()
	width := func(m *ui.MenuWidget) float64 {
		p := ggui.NewProbe(ggui.Row(m), ggui.Sz(500, 300))
		defer p.Close()
		openMenu(p, "File")
		item, ok := p.FindRole(ggui.RoleMenuItem, "Open")
		if !ok {
			t.Fatalf("the menu did not open:\n%s", p.Semantics())
		}
		return item.Rect.Size.W
	}
	narrow := width(ui.Menu("File", ui.MenuItem("Open", nil)).Width(120))
	wide := width(ui.Menu("File", ui.MenuItem("Open", nil)).Width(360))
	if wide-narrow != 240 {
		t.Fatalf("items are %v wide in a 120 panel and %v in a 360 one, want 240 apart", narrow, wide)
	}
	if capped := width(ui.Menu("File", ui.MenuItem("Open", nil)).Width(5000)); capped > 500 {
		t.Fatalf("a 5000px menu is %v wide in a 500px window", capped)
	}
}

func TestMenuStaysOpenAcrossARebuild(t *testing.T) {
	t.Parallel()
	tick := ggui.State(0)
	ran := ""
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Reactive(func() ggui.Widget {
			tick.Get()
			return ui.Menu("File", ui.MenuItem("Open", func() { ran = "open" }), ui.MenuItem("Save", func() { ran = "save" }))
		})
	}, ggui.Sz(300, 300))
	defer p.Close()
	openMenu(p, "File")
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyArrowDown)
	tick.Set(1)
	p.Frame()
	if _, ok := p.FindRole(ggui.RoleMenuItem, "Save"); !ok {
		t.Fatal("the rebuilt menu closed")
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if ran != "save" {
		t.Fatalf("Enter after the rebuild ran %q, want the highlighted save", ran)
	}
}

func TestMenuBindNameRenamesTheTrigger(t *testing.T) {
	t.Parallel()
	name := ggui.State("Actions")
	m := ui.MenuOf(ui.Icon("more"), ui.MenuItem("Open", nil)).BindName(name)
	p := ggui.NewProbe(m, ggui.Sz(300, 300))
	defer p.Close()
	if !m.HasName() {
		t.Fatal("a bound name does not count as a name")
	}
	if _, ok := p.Semantics().Find(ggui.RoleMenu, "Actions"); !ok {
		t.Fatalf("no menu named Actions:\n%s", p.Semantics())
	}
	name.Set("More actions")
	if _, ok := p.Semantics().Find(ggui.RoleMenu, "More actions"); !ok {
		t.Fatalf("the trigger's name did not follow its reader:\n%s", p.Semantics())
	}
}
