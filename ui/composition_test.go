package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestAspectRatioDerivesTheMissingSide(t *testing.T) {
	a := ui.AspectRatio(2, ggui.Box())
	if got := a.Layout(ggui.Loose(ggui.Sz(200, 500)), ggui.Env{}); got != ggui.Sz(200, 100) {
		t.Fatalf("width leads: %+v", got)
	}
	if got := a.Layout(ggui.Loose(ggui.Sz(200, 50)), ggui.Env{}); got != ggui.Sz(100, 50) {
		t.Fatalf("height leads when the derived one would not fit: %+v", got)
	}
	tall := ggui.Constraints{MaxW: ggui.Unbounded, MaxH: 90}
	if got := a.Layout(tall, ggui.Env{}); got != ggui.Sz(180, 90) {
		t.Fatalf("unbounded width: %+v", got)
	}
	free := ggui.Constraints{MaxW: ggui.Unbounded, MaxH: ggui.Unbounded}
	if got := a.Layout(free, ggui.Env{}); got != (ggui.Size{}) {
		t.Fatalf("nothing bounds it, so there is nothing to divide: %+v", got)
	}
	if got := ui.AspectRatio(0, ggui.Box()).Layout(ggui.Loose(ggui.Sz(80, 500)), ggui.Env{}); got != ggui.Sz(80, 80) {
		t.Fatalf("a ratio of zero falls back to square: %+v", got)
	}
}

func TestItemGroupsItsTextAndKeepsTheActionLive(t *testing.T) {
	runs := 0
	item := ui.Item("Backups", "Last run 2 hours ago").
		Media(ui.Avatar("Ada Lovelace").Size(24)).
		Action(ui.Button("Run", func() { runs++ }))
	p := ggui.NewProbe(item, ggui.Sz(360, 80))
	defer p.Close()
	p.Tap("Run")
	if runs != 1 {
		t.Fatal("the action inside an item is not clickable")
	}
	tree := p.Semantics()
	if _, ok := tree.Find(ggui.RoleListItem, ""); !ok {
		t.Fatal("an item is not one node")
	}
	for _, name := range []string{"Backups", "Last run 2 hours ago"} {
		if _, ok := tree.Find(ggui.RoleText, name); !ok {
			t.Fatalf("%q is not in the tree", name)
		}
	}
}

func TestBreadcrumbLinksEveryStepButTheCurrentOne(t *testing.T) {
	home := 0
	b := ui.Breadcrumb(
		ui.Crumb("Home", func() { home++ }),
		ui.Crumb("Projects", func() {}),
		ui.Crumb("ggui", func() {}),
	)
	p := ggui.NewProbe(b, ggui.Sz(400, 40))
	defer p.Close()
	p.Frame()
	find(t, p, ggui.RoleLink, "Home")
	if _, ok := p.FindRole(ggui.RoleLink, "ggui"); ok {
		t.Fatal("the page the user is on is a link")
	}
	p.Tap("Home")
	if home != 1 {
		t.Fatal("a crumb did not run its action")
	}
}

func TestBreadcrumbElidesTheMiddleAtMax(t *testing.T) {
	var crumbs []ui.BreadcrumbEntry
	for _, label := range []string{"A", "B", "C", "D", "E"} {
		crumbs = append(crumbs, ui.Crumb(label, func() {}))
	}
	p := ggui.NewProbe(ui.Breadcrumb(crumbs...).Max(3), ggui.Sz(400, 40))
	defer p.Close()
	tree := p.Semantics()
	if _, ok := tree.Find(ggui.RoleText, "…"); !ok {
		t.Fatal("nothing stands for the elided steps")
	}
	for _, gone := range []string{"B", "C", "D"} {
		if _, ok := tree.Find("", gone); ok {
			t.Fatalf("%q survived the elision", gone)
		}
	}
	if _, ok := tree.Find(ggui.RoleLink, "A"); !ok {
		t.Fatal("the first step was elided")
	}
	if _, ok := tree.Find(ggui.RoleText, "E"); !ok {
		t.Fatal("the current step was elided")
	}
}

func TestAvatarFallsBackToInitials(t *testing.T) {
	p := ggui.NewProbe(ui.Avatar("Ada Lovelace"), ggui.Sz(60, 60))
	defer p.Close()
	tree := p.Semantics()
	if _, ok := tree.Find(ggui.RoleImage, "Ada Lovelace"); !ok {
		t.Fatal("an avatar does not say whose it is")
	}
	if _, ok := tree.Find(ggui.RoleText, "AL"); !ok {
		t.Fatal("initials are not drawn for a nameless portrait")
	}
	one := ggui.NewProbe(ui.Avatar("ada"), ggui.Sz(60, 60))
	defer one.Close()
	if _, ok := one.Semantics().Find(ggui.RoleText, "A"); !ok {
		t.Fatal("a one-word name takes one initial")
	}
	none := ggui.NewProbe(ui.Avatar(""), ggui.Sz(60, 60))
	defer none.Close()
	if _, ok := none.Semantics().Find(ggui.RoleText, "?"); !ok {
		t.Fatal("an empty name has no fallback at all")
	}
}

func TestInputGroupFocusesTheEditorFromItsPadding(t *testing.T) {
	value := ggui.State("ab")
	visits := 0
	group := ui.InputGroup(ggui.TextInput(value).Named("Site")).
		Leading(ggui.Text("https://")).
		Trailing(ui.Button("Go", func() { visits++ }).Ghost())
	p := ggui.NewProbe(group, ggui.Sz(320, 44))
	defer p.Close()
	// The chrome beside the editor belongs to the editor: the click focuses
	// it and puts the caret at the start, which is where Delete bites.
	p.Click(ggui.Pt(2, 22))
	if !p.Focused() {
		t.Fatal("clicking the chrome beside the editor did not focus it")
	}
	p.Type(ggui.Mods{}, ggui.KeyDelete)
	if ggui.Untrack(value.Get) != "b" {
		t.Fatalf("keys did not reach the editor: %q", ggui.Untrack(value.Get))
	}
	p.Tap("Go")
	if visits != 1 {
		t.Fatal("an addon button is covered by the group's own region")
	}
}

func TestHoverCardWaitsForTheCursorToRest(t *testing.T) {
	card := ui.HoverCard(ggui.Box(ggui.Text("@ada")).Size(80, 24), ggui.Text("Ada Lovelace"))
	p := ggui.NewProbe(ggui.Column(card, ggui.Spacer()), ggui.Sz(400, 300))
	defer p.Close()
	shown := func() bool {
		_, ok := p.Semantics().Find(ggui.RoleText, "Ada Lovelace")
		return ok
	}
	p.Move(ggui.Pt(300, 280))
	if shown() {
		t.Fatal("the card is up with the cursor elsewhere")
	}
	p.Move(ggui.Pt(20, 12))
	if shown() {
		t.Fatal("the card is up before the cursor has rested")
	}
	p.Advance(600 * time.Millisecond)
	if !shown() {
		t.Fatal("the card never came up")
	}
	p.Move(ggui.Pt(300, 280))
	if shown() {
		t.Fatal("the card stayed up after the cursor left")
	}
}

func TestButtonGroupJoinsChildrenWithoutTakingTheirInput(t *testing.T) {
	copies, pastes := 0, 0
	g := ui.ButtonGroup(
		ui.Button("Copy", func() { copies++ }).Ghost(),
		ui.Button("Paste", func() { pastes++ }).Ghost(),
	)
	p := ggui.NewProbe(ggui.Row(g, ggui.Spacer()), ggui.Sz(400, 60))
	defer p.Close()
	p.Tap("Copy")
	p.Tap("Paste")
	if copies != 1 || pastes != 1 {
		t.Fatalf("clicks = %d, %d", copies, pastes)
	}
	first, second := find(t, p, ggui.RoleButton, "Copy").Rect, find(t, p, ggui.RoleButton, "Paste").Rect
	if first.Origin.X+first.Size.W > second.Origin.X {
		t.Fatalf("the strip overlaps its children: %+v %+v", first, second)
	}
	if first.Size.H != second.Size.H {
		t.Fatalf("children are not stretched across the strip: %v %v", first.Size.H, second.Size.H)
	}
	if _, ok := p.Semantics().Find(ggui.RoleToolbar, "Actions"); !ok {
		t.Fatal("the strip is not one group")
	}
}

func TestToggleGroupPicksWithThePointerAndTheArrows(t *testing.T) {
	value := ggui.State("left")
	changes := 0
	g := ui.ToggleGroup(value, []string{"left", "center", "right"}).OnChange(func(string) { changes++ })
	p := ggui.NewProbe(g, ggui.Sz(300, 40))
	defer p.Close()
	p.Tap("center")
	if ggui.Untrack(value.Get) != "center" || changes != 1 {
		t.Fatalf("pointer: value=%q changes=%d", ggui.Untrack(value.Get), changes)
	}
	if node, ok := p.Semantics().Find(ggui.RoleRadio, "center"); !ok || node.Checked != ggui.TriOn {
		t.Fatal("the chosen segment is not marked")
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowRight)
	if ggui.Untrack(value.Get) != "right" {
		t.Fatalf("right arrow: %q", ggui.Untrack(value.Get))
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowRight)
	if ggui.Untrack(value.Get) != "left" {
		t.Fatalf("the choice does not wrap: %q", ggui.Untrack(value.Get))
	}
	p.Type(ggui.Mods{}, ggui.KeyEnd)
	if ggui.Untrack(value.Get) != "right" {
		t.Fatalf("End: %q", ggui.Untrack(value.Get))
	}
	p.Type(ggui.Mods{}, ggui.KeyHome)
	if ggui.Untrack(value.Get) != "left" {
		t.Fatalf("Home: %q", ggui.Untrack(value.Get))
	}
	was := changes
	p.Type(ggui.Mods{}, ggui.KeySpace)
	if changes != was {
		t.Fatal("re-picking the current segment fired OnChange")
	}
	g.Disabled(true)
	p.Frame()
	if _, ok := p.FindRole(ggui.RoleRadio, "center"); ok {
		t.Fatal("a disabled group still takes input")
	}
	if node, ok := p.Semantics().Find(ggui.RoleRadio, "center"); !ok || !node.Disabled {
		t.Fatal("a disabled group left the tree")
	}
}

func TestSheetSlidesInFromItsEdgeAndClosesOnTheScrim(t *testing.T) {
	open := ggui.State(false)
	sheet := ui.Sheet(open, ui.Button("Save", func() {})).Title("Filters").Size(200)
	p := ggui.NewProbe(ggui.Column(ui.Button("Open", func() { open.Set(true) }), sheet), ggui.Sz(400, 300))
	defer p.Close()
	p.Frame()
	if _, ok := p.Find("Save"); ok {
		t.Fatal("a closed sheet is on screen")
	}
	p.Tap("Open")
	p.Advance(400 * time.Millisecond)
	save := find(t, p, ggui.RoleButton, "Save").Rect
	if save.Origin.X < 200 {
		t.Fatalf("the sheet did not settle against the right edge: %+v", save)
	}
	p.Click(ggui.Pt(20, 150))
	if ggui.Untrack(open.Get) {
		t.Fatal("a click on the scrim did not close the sheet")
	}
	p.Advance(400 * time.Millisecond)
	if _, ok := p.Find("Save"); ok {
		t.Fatal("the sheet is still taking input after it left")
	}
}

func TestDrawerRisesFromTheBottom(t *testing.T) {
	open := ggui.State(true)
	p := ggui.NewProbe(ui.Drawer(open, ui.Button("Share", func() {})).Size(120), ggui.Sz(400, 300))
	defer p.Close()
	p.Advance(400 * time.Millisecond)
	share := find(t, p, ggui.RoleButton, "Share").Rect
	if share.Origin.Y < 180 {
		t.Fatalf("the drawer did not settle against the bottom edge: %+v", share)
	}
}

func TestSidebarNavigatesWithThePointerAndTheArrows(t *testing.T) {
	page := ggui.State("inbox")
	narrow := ggui.State(false)
	bar := ui.Sidebar(page,
		ui.SidebarSection("Mail"),
		ui.SidebarItem("inbox", "Inbox"),
		ui.SidebarItem("sent", "Sent"),
		ui.SidebarItem("spam", "Spam").Disabled(true),
	).Collapsed(narrow)
	p := ggui.NewProbe(ggui.Row(bar, ggui.Expanded(ggui.Box())), ggui.Sz(600, 400))
	defer p.Close()
	p.Tap("Sent")
	if ggui.Untrack(page.Get) != "sent" {
		t.Fatalf("pointer: %q", ggui.Untrack(page.Get))
	}
	if _, ok := p.FindRole(ggui.RoleTab, "Spam"); ok {
		t.Fatal("a disabled destination takes input")
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowDown)
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if ggui.Untrack(page.Get) != "inbox" {
		t.Fatalf("the arrows skip the heading and the disabled item: %q", ggui.Untrack(page.Get))
	}
	if node, ok := p.Semantics().Find(ggui.RoleTab, "Inbox"); !ok || !node.Selected {
		t.Fatal("the current destination is not marked")
	}
	narrow.Set(true)
	p.Frame()
	if _, ok := p.FindRole(ggui.RoleTab, "Inbox"); ok {
		t.Fatal("a collapsed sidebar is still on screen")
	}
	if got := bar.Layout(ggui.Loose(ggui.Sz(600, 400)), ggui.Env{}); got.W != 0 {
		t.Fatalf("a collapsed sidebar keeps its width: %+v", got)
	}
}

func TestAlertDialogHasToBeAnswered(t *testing.T) {
	open := ggui.State(true)
	deleted, cancelled := 0, 0
	d := ui.AlertDialog(open, "Delete the file?", "This cannot be undone.").
		Confirm("Delete", func() { deleted++ }).
		Cancel("Keep").
		OnCancel(func() { cancelled++ }).
		Destructive()
	p := ggui.NewProbe(d, ggui.Sz(400, 300))
	defer p.Close()
	p.Frame()
	p.Click(ggui.Pt(4, 4))
	if !ggui.Untrack(open.Get) || cancelled != 0 {
		t.Fatal("a click on the scrim dismissed the question")
	}
	p.Tap("Keep")
	if ggui.Untrack(open.Get) || cancelled != 1 || deleted != 0 {
		t.Fatalf("cancel: open=%v cancelled=%d deleted=%d", ggui.Untrack(open.Get), cancelled, deleted)
	}
	open.Set(true)
	p.Frame()
	p.Tap("Delete")
	if ggui.Untrack(open.Get) || deleted != 1 || cancelled != 1 {
		t.Fatalf("confirm: open=%v cancelled=%d deleted=%d", ggui.Untrack(open.Get), cancelled, deleted)
	}
	open.Set(true)
	p.Frame()
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	if ggui.Untrack(open.Get) || cancelled != 2 {
		t.Fatalf("escape: open=%v cancelled=%d", ggui.Untrack(open.Get), cancelled)
	}
}
