package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func TestMenuSubOpensBesideItsRowFromTheKeyboard(t *testing.T) {
	t.Parallel()
	ran := ""
	menu := ui.Menu("File",
		ui.MenuItem("New", func() { ran = "new" }),
		ui.MenuSub("Share",
			ui.MenuItem("Email", func() { ran = "email" }),
			ui.MenuItem("Message", func() { ran = "message" })),
		ui.MenuItem("Close", func() { ran = "close" }))
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Row(menu)), ggui.Sz(600, 400))
	defer p.Close()
	p.Tap("File")
	p.Key("down", "down")
	if _, ok := p.FindRole(ggui.RoleMenuItem, "Email"); ok {
		t.Fatal("the submenu opened before it was asked for")
	}
	p.Key("right")
	share := find(t, p, ggui.RoleMenuItem, "Share").Rect
	email := find(t, p, ggui.RoleMenuItem, "Email").Rect
	if email.Origin.X < share.Origin.X+share.Size.W || email.Origin.Y > share.Origin.Y+share.Size.H {
		t.Fatalf("the submenu is not beside its row: row=%v item=%v", share, email)
	}
	if n := node(t, p.Semantics(), ggui.RoleMenuItem, "Share"); n.Expanded != ggui.Expandable(true) {
		t.Fatalf("the row does not report its submenu open: %+v", n)
	}
	// Left closes the submenu and leaves the menu around it open.
	p.Key("left")
	if _, ok := p.FindRole(ggui.RoleMenuItem, "Email"); ok {
		t.Fatal("Left did not close the submenu")
	}
	if !menu.Popup().IsOpen() {
		t.Fatal("Left closed the whole menu")
	}
	// So does Escape; a second Escape closes the menu.
	p.Key("enter")
	find(t, p, ggui.RoleMenuItem, "Email")
	p.Key("escape")
	if _, ok := p.FindRole(ggui.RoleMenuItem, "Email"); ok || !menu.Popup().IsOpen() {
		t.Fatalf("Escape in a submenu: menu open=%v", menu.Popup().IsOpen())
	}
	p.Key("right", "down", "enter")
	if ran != "message" || menu.Popup().IsOpen() {
		t.Fatalf("choosing in a submenu ran %q and left the menu open=%v", ran, menu.Popup().IsOpen())
	}
	// A menu reopened starts with its submenus closed.
	p.Tap("File")
	if _, ok := p.FindRole(ggui.RoleMenuItem, "Email"); ok {
		t.Fatal("a reopened menu still showed its submenu")
	}
	p.Key("escape")
	if menu.Popup().IsOpen() {
		t.Fatal("Escape did not close the menu")
	}
}

func TestMenuSubOpensWhereThePointerRestsAndOnAClick(t *testing.T) {
	t.Parallel()
	ran := ""
	menu := ui.ContextMenu(ui.Button("Target", nil),
		ui.MenuItem("Open", func() { ran = "open" }),
		ui.MenuSub("More", ui.MenuItem("Rename", func() { ran = "rename" })),
	)
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(menu)), ggui.Sz(600, 400))
	defer p.Close()
	p.ClickButton(find(t, p, ggui.RoleButton, "Target").Center(), ggui.MouseButtonRight)
	more := find(t, p, ggui.RoleMenuItem, "More")
	p.Move(more.Center().Add(ggui.Pt(1, 0)))
	p.Move(more.Center())
	if _, ok := p.FindRole(ggui.RoleMenuItem, "Rename"); ok {
		t.Fatal("the submenu opened before the pointer rested")
	}
	p.Advance(300 * time.Millisecond)
	rename := find(t, p, ggui.RoleMenuItem, "Rename")
	// Resting on a plain row closes it again.
	open := find(t, p, ggui.RoleMenuItem, "Open")
	p.Move(open.Center().Add(ggui.Pt(1, 0)))
	p.Move(open.Center())
	p.Advance(300 * time.Millisecond)
	if _, ok := p.FindRole(ggui.RoleMenuItem, "Rename"); ok {
		t.Fatal("resting on another row left the submenu open")
	}
	p.Click(more.Center())
	find(t, p, ggui.RoleMenuItem, "Rename")
	p.Click(rename.Center())
	if ran != "rename" || menu.Popup().IsOpen() {
		t.Fatalf("a click in the submenu ran %q, menu open=%v", ran, menu.Popup().IsOpen())
	}
}

func TestMenubarKeepsLeftAndRightForAnOpenSubmenu(t *testing.T) {
	t.Parallel()
	bar := ui.Menubar(
		ui.Menu("File", ui.MenuSub("Recent", ui.MenuItem("notes.txt", nil)), ui.MenuItem("Quit", nil)),
		ui.Menu("Edit", ui.MenuItem("Undo", nil)))
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(bar)), ggui.Sz(600, 400))
	defer p.Close()
	// A click opens the menu on no row; Down goes to the first.
	p.Tap("File")
	p.Key("down", "right")
	find(t, p, ggui.RoleMenuItem, "notes.txt")
	p.Key("left")
	if _, ok := p.FindRole(ggui.RoleMenuItem, "notes.txt"); ok {
		t.Fatal("Left did not close the submenu")
	}
	if _, ok := p.FindRole(ggui.RoleMenuItem, "Quit"); !ok {
		t.Fatal("Left in a submenu moved along the bar")
	}
	p.Key("down", "right")
	if _, ok := p.FindRole(ggui.RoleMenuItem, "Undo"); !ok {
		t.Fatal("Right on a plain row did not move to the next menu")
	}
}

func TestMenuCheckboxAndRadioRowsKeepTheMenuOpen(t *testing.T) {
	t.Parallel()
	grid, zoom := ggui.State(false), ggui.State("100%")
	changes := 0
	menu := ui.Menu("View",
		ui.MenuLabel("Appearance"),
		ui.MenuCheckbox("Grid", grid),
		ui.MenuDivider(),
		ui.MenuRadioGroup(zoom, "50%", "100%", "200%").OnChange(func(string) { changes++ }),
		ui.MenuItem("Delete", nil).Destructive().Icon(ui.Icon("close")))
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Row(menu)), ggui.Sz(400, 400))
	defer p.Close()
	p.Tap("View")
	p.Tap("Grid")
	if !grid.Get() || !menu.Popup().IsOpen() {
		t.Fatalf("checkbox row: grid=%v open=%v", grid.Get(), menu.Popup().IsOpen())
	}
	if n := node(t, p.Semantics(), ggui.RoleMenuItem, "Grid"); n.Checked != ggui.Tri(true) {
		t.Fatalf("the row does not report its check: %+v", n)
	}
	p.Tap("200%")
	if zoom.Get() != "200%" || changes != 1 || !menu.Popup().IsOpen() {
		t.Fatalf("radio row: zoom=%q changes=%d open=%v", zoom.Get(), changes, menu.Popup().IsOpen())
	}
	// The keyboard walks the radio rows as the menu's own, and skips the
	// label and the divider.
	p.Key("home", "down", "enter")
	if zoom.Get() != "50%" {
		t.Fatalf("Down from the first row chose %q, want the first radio row", zoom.Get())
	}
}

func TestMenuPanelsFitTheirEntriesAndTheDividerBleeds(t *testing.T) {
	t.Parallel()
	th := uitheme.Default()
	menu := ui.ContextMenu(ui.Button("Target", nil), ui.MenuItem("Go", nil), ui.MenuDivider(), ui.MenuItem("Stop", nil))
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(menu)), ggui.Sz(600, 400))
	defer p.Close()
	p.ClickButton(find(t, p, ggui.RoleButton, "Target").Center(), ggui.MouseButtonRight)
	p.Frame()
	if w := menu.Popup().Rect().Size.W; w != th.Space*18 {
		t.Fatalf("a context menu of short entries is %v wide, want the least width %v", w, th.Space*18)
	}
	wide := ui.ContextMenu(ui.Button("Target", nil), ui.MenuItem("An entry far wider than the least a context menu is", nil))
	q := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(wide)), ggui.Sz(600, 400))
	defer q.Close()
	q.ClickButton(find(t, q, ggui.RoleButton, "Target").Center(), ggui.MouseButtonRight)
	q.Frame()
	item := find(t, q, ggui.RoleMenuItem, "An entry far wider than the least a context menu is").Rect
	if w := wide.Popup().Rect().Size.W; w <= th.Space*18 || w != item.Size.W+th.PanelPad.Horizontal() {
		t.Fatalf("a context menu is %v wide around an entry %v wide", w, item.Size.W)
	}
}

// A menu starts on its first row only when the keyboard opened it, as Base
// UI's does: one the pointer opened shows no highlight until the pointer or
// a key gives it one.
func TestMenusStartOnARowOnlyFromTheKeyboard(t *testing.T) {
	t.Parallel()
	ran := ""
	entries := func() []ggui.Widget {
		return []ggui.Widget{ui.MenuItem("Open", func() { ran = "open" }), ui.MenuItem("Save", func() { ran = "save" })}
	}
	menu := ui.Menu("File", entries()...)
	bar := ui.Menubar(ui.Menu("Edit", entries()...), ui.Menu("View", ui.MenuItem("Zoom", func() { ran = "zoom" })))
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(menu, bar)), ggui.Sz(400, 300))
	defer p.Close()

	// The dropdown: a click, then Enter, runs nothing.
	p.Tap("File")
	p.Key("enter")
	if ran != "" || !menu.Popup().IsOpen() {
		t.Fatalf("a dropdown the pointer opened ran %q", ran)
	}
	p.Key("escape")
	// Enter on its trigger opens it on the first row.
	p.Key("enter")
	if !menu.Popup().IsOpen() {
		t.Fatal("Enter did not open the dropdown")
	}
	p.Key("enter")
	if ran != "open" {
		t.Fatalf("a dropdown the keyboard opened ran %q, want its first row", ran)
	}

	// The menubar: the same for a click, and for the menu the pointer moves
	// on to.
	ran = ""
	p.Tap("Edit")
	p.Key("enter")
	if ran != "" || !bar.Popup().IsOpen() {
		t.Fatalf("a menubar menu the pointer opened ran %q", ran)
	}
	view, _ := p.Find("View")
	p.Move(view.Center().Add(ggui.Pt(-2, 0)))
	p.Move(view.Center())
	find(t, p, ggui.RoleMenuItem, "Zoom")
	p.Key("enter")
	if ran != "" {
		t.Fatalf("a menubar menu the pointer moved to ran %q", ran)
	}
	// Left moves to the menu before it, on its first row.
	p.Key("left", "enter")
	if ran != "open" {
		t.Fatalf("a menubar menu the arrows opened ran %q, want its first row", ran)
	}
}

// Typing in an open menu moves to the row whose label starts with it, as
// Base UI's typeahead does.
func TestMenuTypeaheadMovesToTheRowTyped(t *testing.T) {
	t.Parallel()
	ran := ""
	item := func(label string) *ui.MenuItemWidget { return ui.MenuItem(label, func() { ran = label }) }
	entries := func() []ggui.Widget {
		return []ggui.Widget{item("Open"), item("Save"), item("Share").Disabled(true), item("Save as"), item("Print")}
	}
	menu := ui.Menu("File", entries()...)
	ctx := ui.ContextMenu(ggui.Box(ggui.Text("Target")).Size(100, 40), entries()...).Name("Target actions")
	bar := ui.Menubar(ui.Menu("Edit", entries()...))
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(menu, bar, ctx)), ggui.Sz(400, 400))
	defer p.Close()
	pick := func(want string) {
		t.Helper()
		p.Key("enter")
		if ran != want {
			t.Fatalf("ran %q, want %q", ran, want)
		}
	}

	p.Tap("File")
	p.Text("s")
	pick("Save")
	// The same letter again goes on to the next row under it, past a
	// disabled one, and around.
	p.Tap("File")
	p.Text("s")
	p.Text("s")
	pick("Save as")
	p.Tap("File")
	p.Text("s")
	p.Text("s")
	p.Text("s")
	pick("Save")
	// More letters narrow the search, a space among them; after a pause a
	// letter starts a new one.
	p.Tap("File")
	p.Text("s")
	p.Text("a")
	p.Text("v")
	p.Text("e")
	p.TypeText(ggui.Mods{}, ggui.KeySpace, " ")
	if !menu.Popup().IsOpen() {
		t.Fatal("a space in a search ran the row")
	}
	pick("Save as")
	p.Tap("File")
	p.Text("s")
	p.Advance(time.Second)
	p.Text("p")
	pick("Print")

	// A menubar's menu and a context menu search the same way.
	p.Tap("Edit")
	p.Text("p")
	pick("Print")
	target, _ := p.Find("Target actions")
	p.ClickButton(target.Center(), ggui.MouseButtonRight)
	p.Text("o")
	pick("Open")
}

// A row is lit under the pointer and not where the pointer last was, as
// Base UI clears the highlight when the pointer leaves an item; a row the
// keyboard went to stays lit.
func TestMenuRowLosesThePointersHighlightWhenThePointerLeaves(t *testing.T) {
	t.Parallel()
	ran := ""
	menu := ui.Menu("File",
		ui.MenuItem("Open", func() { ran = "open" }),
		ui.MenuSub("Recent", ui.MenuItem("notes.txt", func() { ran = "notes" })),
		ui.MenuItem("Save", func() { ran = "save" }))
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(menu)), ggui.Sz(500, 400))
	defer p.Close()
	p.Tap("File")
	open := find(t, p, ggui.RoleMenuItem, "Open").Rect
	p.Move(open.Center())
	p.Move(ggui.Pt(450, 350))
	p.Key("enter")
	if ran != "" || !menu.Popup().IsOpen() {
		t.Fatalf("the row the pointer left ran %q", ran)
	}
	// The keyboard's highlight is its own: the pointer leaving the row it
	// rests on does not take it.
	p.Move(open.Center())
	p.Key("end")
	p.Move(ggui.Pt(450, 350))
	p.Key("enter")
	if ran != "save" {
		t.Fatalf("ran %q, want the row the keyboard went to", ran)
	}
	// The row of an open submenu stays lit while the pointer is in it.
	p.Tap("File")
	recent := find(t, p, ggui.RoleMenuItem, "Recent").Rect
	p.Move(recent.Center())
	p.Advance(200 * time.Millisecond)
	notes := find(t, p, ggui.RoleMenuItem, "notes.txt").Rect
	p.Move(notes.Center())
	p.Move(ggui.Pt(450, 350))
	if _, ok := p.FindRole(ggui.RoleMenuItem, "notes.txt"); !ok {
		t.Fatal("the submenu closed as the pointer left it")
	}
	p.Key("enter")
	if ran != "save" {
		t.Fatalf("the submenu's row the pointer left ran %q", ran)
	}
}
