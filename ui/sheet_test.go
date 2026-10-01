package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// openSheet mounts s, open, on a 400x300 window and lets it slide in.
func openSheet(t *testing.T, s *ui.SheetWidget) *ggui.Probe {
	t.Helper()
	p := ggui.NewProbe(s, ggui.Sz(400, 300))
	t.Cleanup(p.Close)
	p.Frame()
	p.Advance(time.Second)
	return p
}

func TestSheetSidesAnchorThePanelToTheirEdge(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		side  func(*ui.SheetWidget) *ui.SheetWidget
		check func(r ggui.Rect) bool
	}{
		{"left", (*ui.SheetWidget).Left, func(r ggui.Rect) bool { return r.Origin.X == 0 && r.Size.W == 150 && r.Size.H == 300 }},
		{"right", (*ui.SheetWidget).Right, func(r ggui.Rect) bool { return r.Origin.X+r.Size.W == 400 && r.Size.W == 150 }},
		{"top", (*ui.SheetWidget).Top, func(r ggui.Rect) bool { return r.Origin.Y == 0 && r.Size.H == 150 && r.Size.W == 400 }},
		{"bottom", (*ui.SheetWidget).Bottom, func(r ggui.Rect) bool { return r.Origin.Y+r.Size.H == 300 && r.Size.H == 150 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := tc.side(ui.Sheet(ggui.State(true), ggui.Text("body")).Name("Panel").Size(150))
			p := openSheet(t, s)
			node, ok := p.Semantics().Find(ggui.RoleDialog, "Panel")
			if !ok || !tc.check(node.Rect) {
				t.Fatalf("panel at %v (found %v) is not anchored %s", node.Rect, ok, tc.name)
			}
		})
	}
}

func TestSheetThatIsNotDismissibleStillClosesOnEscape(t *testing.T) {
	t.Parallel()
	open := ggui.State(true)
	closed := 0
	s := ui.Sheet(open, ui.Button("Save", func() {})).Title("Filters").Size(200).Dismissible(false).OnClose(func() { closed++ })
	p := openSheet(t, s)
	p.Click(ggui.Pt(20, 150))
	if !ggui.Untrack(open.Get) || closed != 0 {
		t.Fatal("a click on the scrim closed a sheet that is not dismissible")
	}
	p.Click(ggui.Pt(390, 290)) // inside the panel, away from the button
	if !ggui.Untrack(open.Get) {
		t.Fatal("a click inside the panel closed it")
	}
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	if ggui.Untrack(open.Get) || closed != 1 {
		t.Fatalf("Escape left open=%v with %d OnClose calls, want closed once", ggui.Untrack(open.Get), closed)
	}
	p.Advance(time.Second)
	if closed != 1 {
		t.Fatalf("OnClose ran %d times after the sheet slid out, want 1", closed)
	}
}

func TestSheetCompactDropsThePanelPadding(t *testing.T) {
	t.Parallel()
	x := func(compact bool) float64 {
		s := ui.Sheet(ggui.State(true), ui.Button("Save", func() {})).Name("Panel").Left().Size(200)
		if compact {
			s.Compact()
		}
		p := openSheet(t, s)
		return find(t, p, ggui.RoleButton, "Save").Rect.Origin.X
	}
	if padded, flush := x(false), x(true); flush != 0 || padded <= 0 {
		t.Fatalf("content starts at x=%v padded and x=%v compact; want padding only without Compact", padded, flush)
	}
}

func TestSheetBindNameFollowsItsReaderUntilNameReplacesIt(t *testing.T) {
	t.Parallel()
	name := ggui.State("Filters")
	s := ui.Sheet(ggui.State(true), ggui.Text("body")).BindName(name)
	p := openSheet(t, s)
	if _, ok := p.Semantics().Find(ggui.RoleDialog, "Filters"); !ok {
		t.Fatalf("no dialog named Filters:\n%s", p.Semantics())
	}
	name.Set("Sorting")
	if _, ok := p.Semantics().Find(ggui.RoleDialog, "Sorting"); !ok {
		t.Fatalf("the name did not follow its reader:\n%s", p.Semantics())
	}
	s.Name("Fixed")
	name.Set("Ignored")
	if _, ok := p.Semantics().Find(ggui.RoleDialog, "Fixed"); !ok {
		t.Fatalf("Name did not detach the reader:\n%s", p.Semantics())
	}
}
