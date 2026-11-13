package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
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

// The rows take shadcn/ui's sizes: a unit in from the column's sides, four
// units tall, three and a half for a small row and a sub-item, six for a
// large one, and the column is 256 wide until Width says otherwise.
func TestSidebarRowsTakeShadcnSizes(t *testing.T) {
	t.Parallel()
	bar := ui.Sidebar(ggui.State("a"),
		ui.SidebarSection("Group"),
		ui.SidebarItem("a", "Alpha").Icon(icons.File).Sub(ui.SidebarItem("a1", "Nested")),
		ui.SidebarItem("b", "Beta").Size(ui.SidebarSizeSmall),
		ui.SidebarItem("c", "Gamma").Size(ui.SidebarSizeLarge),
	).Name("Nav")
	p := ggui.NewProbe(ggui.Row(bar, ggui.Expanded(ggui.Box())), ggui.Sz(600, 400))
	defer p.Close()
	tree := p.Semantics()
	if col, _ := tree.Find(ggui.RoleTabs, "Nav"); col.Rect.Size.W != 256 {
		t.Fatalf("the column is %v wide, want 256", col.Rect.Size.W)
	}
	for _, tc := range []struct {
		name string
		x, w float64
		h    float64
	}{{"Alpha", 8, 240, 32}, {"Beta", 8, 240, 28}, {"Gamma", 8, 240, 48}, {"Nested", 33, 0, 28}} {
		n, ok := tree.Find(ggui.RoleTab, tc.name)
		if !ok {
			t.Fatalf("no row %q:\n%s", tc.name, tree)
		}
		r := n.Rect
		if r.Origin.X != tc.x || r.Size.H != tc.h || (tc.w > 0 && r.Size.W != tc.w) {
			t.Fatalf("%s is at %v, want x %v, %v wide (0: any), %v tall", tc.name, r, tc.x, tc.w, tc.h)
		}
	}
}

// Collapsed to icons the column is a rail six units wide whose rows keep
// their icons, drop their sub-items and name themselves in a tip.
func TestSidebarIconRailKeepsIconsAndTips(t *testing.T) {
	t.Parallel()
	collapsed := ggui.State(false)
	bar := ui.Sidebar(ggui.State("a"),
		ui.SidebarItem("a", "Alpha").Icon(icons.File).Tooltip("Alpha tip").Sub(ui.SidebarItem("a1", "Nested")),
		ui.SidebarItem("b", "Beta").Icon(icons.Search),
	).Name("Nav").Collapsible(ui.SidebarCollapseIcon).BindCollapsed(collapsed)
	p := ggui.NewProbe(ggui.Row(bar, ggui.Expanded(ggui.Box())), ggui.Sz(600, 400))
	defer p.Close()
	p.Advance(0)
	collapsed.Set(true)
	p.Advance(0)
	p.Advance(100 * time.Millisecond)
	if col, _ := p.Semantics().Find(ggui.RoleTabs, "Nav"); col.Rect.Size.W <= 48 || col.Rect.Size.W >= 256 {
		t.Fatalf("halfway through the collapse the column is %v wide", col.Rect.Size.W)
	}
	p.Advance(time.Second)
	tree := p.Semantics()
	if col, _ := tree.Find(ggui.RoleTabs, "Nav"); col.Rect.Size.W != 48 {
		t.Fatalf("the rail is %v wide, want 48", col.Rect.Size.W)
	}
	alpha, ok := p.FindRole(ggui.RoleTab, "Alpha")
	if !ok || alpha.Rect.Size.W != 32 || alpha.Rect.Size.H != 32 {
		t.Fatalf("on the rail Alpha is %v, want a 32 square", alpha.Rect)
	}
	if _, ok := p.FindRole(ggui.RoleTab, "Nested"); ok {
		t.Fatal("a sub-item takes input on the rail")
	}
	p.Move(alpha.Center())
	p.Advance(time.Second)
	if _, ok := p.Semantics().Find("", "Alpha tip"); !ok {
		t.Fatalf("no tip over the rail's row:\n%s", p.Semantics())
	}
	collapsed.Set(false)
	p.Frame()
	p.Advance(time.Second)
	if col, _ := p.Semantics().Find(ggui.RoleTabs, "Nav"); col.Rect.Size.W != 256 {
		t.Fatalf("expanded again the column is %v wide", col.Rect.Size.W)
	}
}

// Off its edge, the column gives its width back over MotionSlow at a
// constant speed.
func TestSidebarSlidesOffItsEdgeLinearly(t *testing.T) {
	t.Parallel()
	collapsed := ggui.State(false)
	bar := ui.Sidebar(ggui.State("a"), ui.SidebarItem("a", "Alpha")).Name("Nav").BindCollapsed(collapsed)
	p := ggui.NewProbe(ggui.Row(bar, ggui.Expanded(ggui.Box())), ggui.Sz(600, 400))
	defer p.Close()
	p.Advance(0)
	collapsed.Set(true)
	p.Advance(0)
	p.Advance(100 * time.Millisecond)
	col, _ := p.Semantics().Find(ggui.RoleTabs, "Nav")
	if w := col.Rect.Size.W; w < 100 || w > 156 {
		t.Fatalf("halfway through the slide the column is %v wide, want about 128", w)
	}
	p.Advance(time.Second)
	if _, ok := p.Semantics().Find(ggui.RoleTabs, "Nav"); ok {
		t.Fatal("the column is still there after the slide")
	}
}

// The trigger, the rail and the shortcut all toggle the one state.
func TestSidebarTriggerRailAndShortcutShareTheState(t *testing.T) {
	t.Parallel()
	collapsed := ggui.State(false)
	bar := ui.Sidebar(ggui.State("a"), ui.SidebarItem("a", "Alpha").Icon(icons.File)).
		Collapsible(ui.SidebarCollapseIcon).BindCollapsed(collapsed).Rail(true).Shortcut("cmd+b")
	trigger := ui.SidebarTrigger(collapsed)
	p := ggui.NewProbe(ggui.Row(bar, ggui.Expanded(ggui.Column(trigger))), ggui.Sz(600, 400))
	defer p.Close()
	toggles := p.FindAll(ggui.RoleButton)
	if len(toggles) != 2 {
		t.Fatalf("want the rail and the trigger, got %v", toggles)
	}
	rail, button := toggles[0], toggles[1]
	if rail.Rect.Origin.X >= 256 || button.Rect.Origin.X < 256 {
		t.Fatalf("rail %v, trigger %v", rail.Rect, button.Rect)
	}
	p.Click(button.Center())
	if !ggui.Untrack(collapsed.Get) {
		t.Fatal("the trigger did not collapse the sidebar")
	}
	p.Advance(time.Second)
	p.Key("cmd+b")
	if ggui.Untrack(collapsed.Get) {
		t.Fatal("the shortcut did not expand the sidebar")
	}
	p.Advance(time.Second)
	p.Click(rail.Center())
	if !ggui.Untrack(collapsed.Get) {
		t.Fatal("the rail did not collapse the sidebar")
	}
}

// A collapsible row folds its sub-items with a press and the arrows, and
// opens by itself around the current destination.
func TestSidebarCollapsibleRowFolds(t *testing.T) {
	t.Parallel()
	page := ggui.State("a")
	bar := ui.Sidebar(page,
		ui.SidebarItem("a", "Alpha"),
		ui.SidebarItem("m", "Models").Collapsible().Sub(
			ui.SidebarItem("g", "Genesis"),
			ui.SidebarItem("x", "Explorer"),
		),
	)
	p := ggui.NewProbe(bar, ggui.Sz(300, 400))
	defer p.Close()
	if _, ok := p.FindRole(ggui.RoleTab, "Genesis"); ok {
		t.Fatal("a closed row shows its sub-items")
	}
	models, ok := p.Semantics().Find(ggui.RoleDisclosure, "Models")
	if !ok || models.Expanded != ggui.Expandable(false) {
		t.Fatalf("Models is not a closed disclosure: %+v", models)
	}
	p.Tap("Models")
	p.Advance(time.Second)
	if _, ok := p.FindRole(ggui.RoleTab, "Genesis"); !ok {
		t.Fatal("a press did not open the row")
	}
	if got := ggui.Untrack(page.Get); got != "a" {
		t.Fatalf("a press on a collapsible row went to %q", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	if got := ggui.Untrack(page.Get); got != "g" {
		t.Fatalf("Down, Enter from Models went to %q, want g", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowLeft, ggui.KeyArrowLeft)
	p.Advance(time.Second)
	if _, ok := p.FindRole(ggui.RoleTab, "Genesis"); ok {
		t.Fatal("Left, Left from a sub-item did not close the row over it")
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowRight)
	p.Advance(time.Second)
	if _, ok := p.FindRole(ggui.RoleTab, "Genesis"); !ok {
		t.Fatal("Right did not open the row")
	}

	closed := ui.Sidebar(ggui.State("x"), ui.SidebarItem("m", "Models").Collapsible().Sub(ui.SidebarItem("x", "Explorer")))
	q := ggui.NewProbe(closed, ggui.Sz(300, 400))
	defer q.Close()
	if _, ok := q.FindRole(ggui.RoleTab, "Explorer"); !ok {
		t.Fatal("the row holding the current destination starts closed")
	}
}

// The header and the footer stay put while the groups scroll between them.
func TestSidebarGroupsScrollBetweenHeaderAndFooter(t *testing.T) {
	t.Parallel()
	var items []ui.SidebarEntry
	for _, k := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p"} {
		items = append(items, ui.SidebarItem(k, "Item "+k))
	}
	bar := ui.Sidebar(ggui.State("a"), items...).
		Header(ggui.Text("Head")).Footer(ggui.Text("Foot")).
		FooterItems(ui.SidebarItem("me", "Account"))
	p := ggui.NewProbe(ggui.Row(bar, ggui.Expanded(ggui.Box())), ggui.Sz(600, 300))
	defer p.Close()
	tree := p.Semantics()
	head, _ := tree.Find("", "Head")
	foot, _ := tree.Find("", "Foot")
	account, _ := tree.Find(ggui.RoleTab, "Account")
	first, _ := tree.Find(ggui.RoleTab, "Item a")
	if foot.Rect.Origin.Y < 260 || account.Rect.Origin.Y > foot.Rect.Origin.Y {
		t.Fatalf("footer %v and its item %v are not at the bottom", foot.Rect, account.Rect)
	}
	p.Scroll(first.Rect.Center(), ggui.Pt(0, -5))
	p.Advance(time.Second)
	tree = p.Semantics()
	head2, _ := tree.Find("", "Head")
	account2, _ := tree.Find(ggui.RoleTab, "Account")
	first2, _ := tree.Find(ggui.RoleTab, "Item a")
	if head2.Rect != head.Rect || account2.Rect != account.Rect {
		t.Fatalf("the header or the footer moved: %v→%v, %v→%v", head.Rect, head2.Rect, account.Rect, account2.Rect)
	}
	if first2.Full.Origin.Y >= first.Full.Origin.Y {
		t.Fatalf("the groups did not scroll: %v→%v", first.Full, first2.Full)
	}
}

// An action shown on hover is there only while the pointer is on its row.
func TestSidebarActionOnHover(t *testing.T) {
	t.Parallel()
	pressed := 0
	bar := ui.Sidebar(ggui.State("a"),
		ui.SidebarItem("a", "Alpha").Action(ui.ButtonOf(ui.Icon(icons.Plus), func() { pressed++ }).Name("More")).ActionOnHover(),
		ui.SidebarItem("b", "Beta"),
	)
	p := ggui.NewProbe(bar, ggui.Sz(300, 300))
	defer p.Close()
	if _, ok := p.FindRole(ggui.RoleButton, "More"); ok {
		t.Fatal("the action shows with the pointer away")
	}
	alpha, _ := p.FindRole(ggui.RoleTab, "Alpha")
	p.Move(alpha.Center())
	more, ok := p.FindRole(ggui.RoleButton, "More")
	if !ok {
		t.Fatal("the action does not show under the pointer")
	}
	p.Click(more.Center())
	if pressed != 1 {
		t.Fatalf("the action ran %d times", pressed)
	}
}
