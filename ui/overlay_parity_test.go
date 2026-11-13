package ui_test

import (
	"math"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
)

// near reports whether a and b are the same to within a rounding.
func near(a, b float64) bool { return math.Abs(a-b) < .01 }

// sem returns where the semantics tree has the node of that role and name,
// for text, which is described without being a region of its own.
func sem(t *testing.T, p *ggui.Probe, role ggui.Role, name string) ggui.Rect {
	t.Helper()
	n, ok := p.Semantics().Find(role, name)
	if !ok {
		t.Fatalf("no %s named %q:\n%s", role, name, p.Semantics())
	}
	return n.Rect
}

// openModal mounts w, open, on a window of the given size and lets it settle.
func openModal(t *testing.T, w ggui.Widget, size ggui.Size) *ggui.Probe {
	t.Helper()
	p := ggui.NewProbe(w, size)
	t.Cleanup(p.Close)
	p.Frame()
	p.Advance(time.Second)
	return p
}

func TestDialogDescriptionAndFooterBand(t *testing.T) {
	t.Parallel()
	saved := 0
	d := ui.Dialog(ggui.State(true), ggui.Text("Body")).
		Title("Edit profile").
		Description("Make changes to your profile here.").
		Footer(ui.Button("Cancel", nil).Outline(), ui.Button("Save", func() { saved++ }))
	p := openModal(t, d, ggui.Sz(640, 480))
	panel := d.Rect()
	title := sem(t, p, ggui.RoleHeading, "Edit profile")
	desc := sem(t, p, ggui.RoleText, "Make changes to your profile here.")
	if gap := desc.Origin.Y - (title.Origin.Y + title.Size.H); !near(gap, 8) {
		t.Errorf("the description is %v below the title, want 8", gap)
	}
	// The actions sit at the end of a band padded like the panel, which
	// reaches the panel's bottom edge.
	save := find(t, p, ggui.RoleButton, "Save").Rect
	cancel := find(t, p, ggui.RoleButton, "Cancel").Rect
	if right := panel.Origin.X + panel.Size.W - (save.Origin.X + save.Size.W); !near(right, 16) {
		t.Errorf("the last action ends %v from the panel's edge, want 16", right)
	}
	if below := panel.Origin.Y + panel.Size.H - (save.Origin.Y + save.Size.H); !near(below, 16) {
		t.Errorf("the actions end %v above the panel's bottom, want 16", below)
	}
	if gap := save.Origin.X - (cancel.Origin.X + cancel.Size.W); !near(gap, 8) {
		t.Errorf("the actions are %v apart, want 8", gap)
	}
	p.Tap("Save")
	if saved != 1 {
		t.Fatal("a footer action did not take its press")
	}
	// Both can be taken away again.
	before := panel.Size.H
	d.Footer().Description("")
	p.Frame()
	if _, ok := p.FindRole(ggui.RoleButton, "Save"); ok || d.Rect().Size.H >= before {
		t.Fatalf("Footer() and Description(\"\") left the panel %v tall (was %v)", d.Rect().Size.H, before)
	}
}

func TestAlertDialogHeaderGapFooterAndSizes(t *testing.T) {
	t.Parallel()
	a := ui.AlertDialog(ggui.State(true), "Delete chat?", "This cannot be undone.").Confirm("Delete", nil)
	p := openModal(t, a, ggui.Sz(640, 480))
	title := sem(t, p, ggui.RoleHeading, "Delete chat?")
	desc := sem(t, p, ggui.RoleText, "This cannot be undone.")
	if gap := desc.Origin.Y - (title.Origin.Y + title.Size.H); !near(gap, 6) {
		t.Errorf("the description is %v below the title, want 6", gap)
	}
	if _, ok := p.FindRole(ggui.RoleDialog, "Delete chat?"); !ok {
		t.Error("the dialog lost the name its title gives it")
	}
	panel := a.Dialog().Rect()
	del := find(t, p, ggui.RoleButton, "Delete").Rect
	if panel.Size.W != 384 || !near(panel.Origin.X+panel.Size.W-(del.Origin.X+del.Size.W), 16) {
		t.Errorf("default: panel %v with the confirming action at %v", panel, del)
	}

	small := ui.AlertDialog(ggui.State(true), "Allow?", "The accessory wants to connect.").Small().Media(ui.Icon(icons.Info))
	p = openModal(t, small, ggui.Sz(640, 480))
	panel = small.Dialog().Rect()
	cancel := find(t, p, ggui.RoleButton, "Cancel").Rect
	confirm := find(t, p, ggui.RoleButton, "Continue").Rect
	if panel.Size.W != 320 {
		t.Errorf("a Small alert dialog is %v wide, want 320", panel.Size.W)
	}
	if !near(cancel.Size.W, confirm.Size.W) || !near(cancel.Origin.X-panel.Origin.X, 16) || !near(panel.Origin.X+panel.Size.W-(confirm.Origin.X+confirm.Size.W), 16) {
		t.Errorf("Small: the actions at %v and %v do not share the footer of %v", cancel, confirm, panel)
	}
	title = sem(t, p, ggui.RoleHeading, "Allow?")
	if !near(title.Center().X, panel.Center().X) {
		t.Errorf("Small: the title is centered on %v, the panel on %v", title.Center().X, panel.Center().X)
	}
	// The media tile is 40px and stands above the centered text.
	if top := title.Origin.Y - panel.Origin.Y; top < 16+40 {
		t.Errorf("Small: the title starts %v below the panel's top, with no room for the media above it", top)
	}
}

func TestSheetDescriptionFooterAndCloseInset(t *testing.T) {
	t.Parallel()
	s := ui.Sheet(ggui.State(true), ggui.Text("Body")).
		Title("Edit profile").
		Description("Make changes here.").
		Footer(ui.Button("Save", nil), ui.Button("Close sheet", nil).Outline())
	p := openModal(t, s, ggui.Sz(800, 500))
	panel := s.Rect()
	title := sem(t, p, ggui.RoleHeading, "Edit profile")
	desc := sem(t, p, ggui.RoleText, "Make changes here.")
	if gap := desc.Origin.Y - (title.Origin.Y + title.Size.H); !near(gap, 2) {
		t.Errorf("the description is %v below the title, want 2", gap)
	}
	// The actions are stacked at the sheet's full width, at its foot.
	save := find(t, p, ggui.RoleButton, "Save").Rect
	shut := find(t, p, ggui.RoleButton, "Close sheet").Rect
	if !near(save.Size.W, panel.Size.W-32) || !near(shut.Origin.Y-(save.Origin.Y+save.Size.H), 8) {
		t.Errorf("the actions at %v and %v are not stacked across %v", save, shut, panel)
	}
	if below := panel.Origin.Y + panel.Size.H - (shut.Origin.Y + shut.Size.H); !near(below, 16) {
		t.Errorf("the actions end %v above the sheet's bottom, want 16", below)
	}
	x := find(t, p, ggui.RoleButton, "Close").Rect
	if right, top := panel.Origin.X+panel.Size.W-(x.Origin.X+x.Size.W), x.Origin.Y-panel.Origin.Y; !near(right, 12) || !near(top, 12) {
		t.Errorf("the close button is %v from the edge and %v from the top, want 12 and 12", right, top)
	}
}

func TestDrawerIsAsTallAsItsContentUpToItsHeadroom(t *testing.T) {
	t.Parallel()
	short := ui.Drawer(ggui.State(true), ggui.Box().Height(100)).Name("Short").CloseButton(false)
	openModal(t, short, ggui.Sz(400, 600))
	if r := short.Rect(); r.Size.H != 132 || r.Origin.Y+r.Size.H != 600 {
		t.Errorf("a drawer around 100px of content is at %v, want 132 tall on the bottom edge", r)
	}
	tall := ui.Drawer(ggui.State(true), ggui.Box().Height(900)).Name("Tall").CloseButton(false)
	openModal(t, tall, ggui.Sz(400, 600))
	if r := tall.Rect(); r.Size.H != 600-96 {
		t.Errorf("a drawer around 900px of content is %v tall, want the window less 96", r.Size.H)
	}
	// The handle is opt-in, and takes a strip of its own above the content.
	handled := ui.Drawer(ggui.State(true), ggui.Box().Height(100)).Name("Handled").CloseButton(false).Handle(true)
	openModal(t, handled, ggui.Sz(400, 600))
	if r := handled.Rect(); r.Size.H != 144 {
		t.Errorf("a drawer with its handle is %v tall, want 12 more than the 132 without", r.Size.H)
	}
}

func TestDrawerHasNoCloseButtonUntilAsked(t *testing.T) {
	t.Parallel()
	bare := ui.Drawer(ggui.State(true), ggui.Text("Body")).Title("Share")
	p := openModal(t, bare, ggui.Sz(400, 600))
	if _, ok := p.FindRole(ggui.RoleButton, "Close"); ok {
		t.Error("a drawer shows a close button nobody asked for")
	}
	asked := ui.Drawer(ggui.State(true), ggui.Text("Body")).Title("Share").CloseButton(true)
	p = openModal(t, asked, ggui.Sz(400, 600))
	if _, ok := p.FindRole(ggui.RoleButton, "Close"); !ok {
		t.Error("CloseButton(true) gave a drawer no close button")
	}
	sheet := ui.Sheet(ggui.State(true), ggui.Text("Body")).Title("Share")
	p = openModal(t, sheet, ggui.Sz(400, 600))
	if _, ok := p.FindRole(ggui.RoleButton, "Close"); !ok {
		t.Error("a sheet lost its close button")
	}
}

func TestDrawerRisesOverItsOwnDuration(t *testing.T) {
	t.Parallel()
	open := ggui.State(false)
	d := ui.Drawer(open, ggui.Box().Height(100)).Name("Details")
	p := ggui.NewProbe(d, ggui.Sz(400, 600))
	defer p.Close()
	p.Frame()
	open.Set(true)
	p.Frame()
	// A sheet is in place after the theme's 200ms; a drawer is still rising.
	p.Advance(250 * time.Millisecond)
	if y := d.Rect().Origin.Y; y <= 468 || y >= 600 {
		t.Fatalf("250ms in, the drawer's top is at %v, want it between the window's bottom and its place at 468", y)
	}
	p.Advance(250 * time.Millisecond)
	if y := d.Rect().Origin.Y; y != 468 {
		t.Fatalf("500ms in, the drawer's top is at %v, want 468", y)
	}
}
