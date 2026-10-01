package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestSidebarWidthHeaderFooterAndName(t *testing.T) {
	t.Parallel()
	bar := ui.Sidebar(ggui.State("inbox"), ui.SidebarItem("inbox", "Inbox")).
		Width(180).Name("Mailboxes").Header(ggui.Text("Acme")).Footer(ggui.Text("v1.0"))
	p := ggui.NewProbe(ggui.Row(bar, ggui.Expanded(ggui.Box())), ggui.Sz(600, 400))
	defer p.Close()
	tree := p.Semantics()
	column, ok := tree.Find(ggui.RoleTabs, "Mailboxes")
	if !ok {
		t.Fatalf("no tab list named Mailboxes:\n%s", tree)
	}
	if column.Rect.Size.W != 180 || column.Rect.Size.H != 400 {
		t.Fatalf("sidebar is %v, want 180 wide and the full 400 tall", column.Rect.Size)
	}
	header, _ := tree.Find("", "Acme")
	item, _ := tree.Find(ggui.RoleTab, "Inbox")
	footer, ok := tree.Find("", "v1.0")
	if !ok || !(header.Rect.Origin.Y < item.Rect.Origin.Y && item.Rect.Origin.Y < footer.Rect.Origin.Y) {
		t.Fatalf("want header above the items above the footer: %v %v %v", header.Rect, item.Rect, footer.Rect)
	}
	if footer.Rect.Origin.Y < 300 {
		t.Fatalf("footer at y=%v, want it pinned to the bottom of a 400 tall column", footer.Rect.Origin.Y)
	}
	bar.Width(-10)
	p.Frame()
	if n, _ := p.Semantics().Find(ggui.RoleTabs, "Mailboxes"); n.Rect.Size.W != 0 {
		t.Fatalf("Width(-10) left the sidebar %v wide, want 0", n.Rect.Size.W)
	}
}

func TestSidebarCollapsedDetachesItsBinding(t *testing.T) {
	t.Parallel()
	narrow := ggui.State(true)
	bar := ui.Sidebar(ggui.State("a"), ui.SidebarItem("a", "Home")).BindCollapsed(narrow)
	p := ggui.NewProbe(ggui.Row(bar, ggui.Expanded(ggui.Box())), ggui.Sz(400, 200))
	defer p.Close()
	if _, ok := p.Semantics().Find(ggui.RoleTab, "Home"); ok {
		t.Fatal("bound-collapsed sidebar is on screen")
	}
	bar.Collapsed(false)
	p.Frame()
	if _, ok := p.Semantics().Find(ggui.RoleTab, "Home"); !ok {
		t.Fatal("Collapsed(false) did not bring the sidebar back")
	}
	narrow.Set(true)
	p.Frame()
	if _, ok := p.Semantics().Find(ggui.RoleTab, "Home"); !ok {
		t.Fatal("Collapsed(false) did not detach the old reader")
	}
}

func TestSidebarSelectActionReportsOnlyRealChanges(t *testing.T) {
	t.Parallel()
	page := ggui.State("inbox")
	var changes []string
	bar := ui.Sidebar(page,
		ui.SidebarItem("inbox", "Inbox"),
		ui.SidebarItem("sent", "Sent"),
		ui.SidebarItem("spam", "Spam").Disabled(true),
	).OnChange(func(k string) { changes = append(changes, k) })
	p := ggui.NewProbe(bar, ggui.Sz(300, 300))
	defer p.Close()
	act := func(name string, kind ggui.ActionSet) {
		t.Helper()
		n, ok := p.Semantics().Find(ggui.RoleTab, name)
		if !ok {
			t.Fatalf("no tab %q", name)
		}
		p.Perform(n.ID, ggui.Action{Kind: kind})
	}
	act("Sent", ggui.ActionSelect)
	act("Sent", ggui.ActionPress)
	act("Spam", ggui.ActionSelect)
	act("Inbox", ggui.ActionPress)
	if got := ggui.Untrack(page.Get); got != "inbox" || len(changes) != 2 || changes[0] != "sent" || changes[1] != "inbox" {
		t.Fatalf("page %q, OnChange %q; want inbox and [sent inbox]", got, changes)
	}
	if n, _ := p.Semantics().Find(ggui.RoleTab, "Spam"); !n.Disabled {
		t.Fatal("disabled destination is not described as disabled")
	}
}

func TestSidebarHomeAndEndSkipHeadingsAndDisabledItems(t *testing.T) {
	t.Parallel()
	page := ggui.State("b")
	bar := ui.Sidebar(page,
		ui.SidebarSection("Top"),
		ui.SidebarItem("a", "Alpha"),
		ui.SidebarItem("b", "Beta"),
		ui.SidebarItem("c", "Gamma"),
		ui.SidebarItem("d", "Delta").Disabled(true),
	)
	p := ggui.NewProbe(bar, ggui.Sz(300, 300))
	defer p.Close()
	p.Tap("Beta")
	p.Type(ggui.Mods{}, ggui.KeyEnd, ggui.KeyEnter)
	if got := ggui.Untrack(page.Get); got != "c" {
		t.Fatalf("End, Enter went to %q, want the last enabled item c", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyHome, ggui.KeySpace)
	if got := ggui.Untrack(page.Get); got != "a" {
		t.Fatalf("Home, Space went to %q, want the first item a", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowUp, ggui.KeyEnter)
	if got := ggui.Untrack(page.Get); got != "c" {
		t.Fatalf("Up from the first item went to %q, want it to wrap to c", got)
	}
}

func TestSidebarHighlightFollowsItsKeyAcrossAReorderingRebuild(t *testing.T) {
	t.Parallel()
	page := ggui.State("a")
	reversed := ggui.State(false)
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Reactive(func() ggui.Widget {
			items := []ui.SidebarEntry{ui.SidebarItem("a", "Alpha"), ui.SidebarItem("b", "Beta"), ui.SidebarItem("c", "Gamma")}
			if reversed.Get() {
				items[0], items[2] = items[2], items[0]
			}
			return ui.Sidebar(page, items...).Key("nav")
		})
	}, ggui.Sz(300, 300))
	defer p.Close()
	p.Tap("Alpha")
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	if got := ggui.Untrack(page.Get); got != "b" {
		t.Fatalf("Down, Enter from Alpha went to %q, want b", got)
	}
	reversed.Set(true)
	p.Frame()
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	// Reversed, the order is Gamma, Beta, Alpha: one below Beta is Alpha.
	if got := ggui.Untrack(page.Get); got != "a" {
		t.Fatalf("after the rebuild Down, Enter went to %q, want a (the highlight stayed on Beta)", got)
	}
}
