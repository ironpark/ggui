package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestContextMenuPointerKeyboardAndAccessibility(t *testing.T) {
	t.Parallel()
	clicks, picked := 0, ""
	menu := ui.ContextMenu(ui.Button("Content", func() { clicks++ }),
		ui.MenuItem("Unavailable", func() { t.Fatal("disabled action") }).Disabled(true),
		ui.MenuItem("Copy", func() { picked = "copy" }), ui.MenuDivider(),
		ui.MenuItem("Delete", func() { picked = "delete" })).Name("File actions")
	p := ggui.NewProbe(ggui.Column(menu, ui.Button("Outside", func() { clicks++ })), ggui.Sz(320, 300))
	defer p.Close()
	p.Tap("Content")
	if clicks != 1 || menu.Popup().IsOpen() {
		t.Fatal("left click intercepted")
	}
	target, _ := p.Find("Content")
	p.ClickButton(target.Center(), ggui.MouseButtonRight)
	if !menu.Popup().IsOpen() || clicks != 1 {
		t.Fatal("secondary click did not open exclusively")
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if picked != "copy" || menu.Popup().IsOpen() {
		t.Fatal("initial enabled action", picked)
	}
	act(t, p, ggui.RoleMenu, "File actions", ggui.Action{Kind: ggui.ActionExpand})
	p.Type(ggui.Mods{}, ggui.KeyEnd, ggui.KeyEnter)
	if picked != "delete" {
		t.Fatal("End navigation", picked)
	}
	p.Type(ggui.Mods{Shift: true}, ggui.KeyF10)
	if !menu.Popup().IsOpen() {
		t.Fatal("focus restoration / Shift+F10")
	}
	p.Type(ggui.Mods{}, ggui.KeyHome, ggui.KeyArrowUp, ggui.KeyEnter)
	if picked != "delete" {
		t.Fatal("wrap navigation")
	}
	act(t, p, ggui.RoleMenu, "File actions", ggui.Action{Kind: ggui.ActionExpand})
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	if menu.Popup().IsOpen() {
		t.Fatal("Escape")
	}
	act(t, p, ggui.RoleMenu, "File actions", ggui.Action{Kind: ggui.ActionExpand})
	p.Click(ggui.Pt(310, 290))
	if menu.Popup().IsOpen() || clicks != 1 {
		t.Fatal("outside click")
	}
	act(t, p, ggui.RoleMenu, "File actions", ggui.Action{Kind: ggui.ActionExpand})
	act(t, p, ggui.RoleMenuItem, "Copy", ggui.Action{Kind: ggui.ActionPress})
	if picked != "copy" || menu.Popup().IsOpen() {
		t.Fatal("accessible item activation")
	}
	menu.Disabled(true)
	p.Frame()
	p.ClickButton(target.Center(), ggui.MouseButtonRight)
	if menu.Popup().IsOpen() {
		t.Fatal("disabled menu opened")
	}
	p.Tap("Content")
	if clicks != 2 {
		t.Fatal("disabled wrapper disabled content")
	}
}

func TestContextMenuNoEnabledActions(t *testing.T) {
	t.Parallel()
	for _, entries := range [][]ggui.Widget{nil, {ui.MenuItem("Disabled", func() { t.Fatal("disabled action ran") }).Disabled(true)}} {
		menu := ui.ContextMenu(ggui.Text("Target"), entries...).Name("Actions")
		p := ggui.NewProbe(menu, ggui.Sz(300, 200))
		act(t, p, ggui.RoleMenu, "Actions", ggui.Action{Kind: ggui.ActionExpand})
		p.Type(ggui.Mods{}, ggui.KeyArrowUp, ggui.KeyArrowDown, ggui.KeyHome, ggui.KeyEnd, ggui.KeyEnter)
		p.Type(ggui.Mods{}, ggui.KeyEscape)
		if menu.Popup().IsOpen() {
			t.Fatal("empty/disabled menu did not close")
		}
		p.Close()
	}
}

func TestContextMenuEdgePlacementAndRebuild(t *testing.T) {
	t.Parallel()
	var menu *ui.ContextMenuWidget
	version := ggui.State(0)
	p := ggui.ProbeBuilder(func() ggui.Widget {
		_ = version.Get()
		menu = ui.ContextMenu(ggui.Box().Size(300, 200), ui.MenuItem("A reasonably long action", nil))
		menu.Key("context")
		return menu
	}, ggui.Sz(300, 200))
	defer p.Close()
	p.ClickButton(ggui.Pt(298, 198), ggui.MouseButtonRight)
	version.Set(1)
	p.Frame()
	r := menu.Popup().Rect()
	if !menu.Popup().IsOpen() || r.Size.W < 100 || r.Origin.X < 0 || r.Origin.X+r.Size.W > 300 || r.Origin.Y+r.Size.H > 200 {
		t.Fatalf("menu clipped at edge: %+v, open %v", r, menu.Popup().IsOpen())
	}
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	if menu.Popup().IsOpen() {
		t.Fatal("rebuilt menu did not close")
	}
}

// A ContextMenuFunc makes its entries as it opens, not as it is rebuilt,
// and works as a ContextMenu does once open.
func TestContextMenuFuncMakesEntriesAsItOpens(t *testing.T) {
	t.Parallel()
	made, picked := 0, ""
	version := ggui.State(0)
	var menu *ui.ContextMenuWidget
	p := ggui.ProbeBuilder(func() ggui.Widget {
		_ = version.Get()
		menu = ui.ContextMenuFunc(ggui.Box().Size(300, 200), func() []ggui.Widget {
			made++
			return []ggui.Widget{
				ui.MenuItem("Copy", func() { picked = "copy" }), ui.MenuDivider(),
				ui.MenuItem("Delete", func() { picked = "delete" }),
			}
		}).Name("Actions")
		menu.Key("context")
		return menu
	}, ggui.Sz(300, 200))
	defer p.Close()
	for v := 1; v <= 3; v++ {
		version.Set(v)
		p.Frame()
	}
	if made != 0 {
		t.Fatalf("entries made %d times before the menu opened", made)
	}
	p.ClickButton(ggui.Pt(20, 20), ggui.MouseButtonRight)
	if !menu.Popup().IsOpen() || made != 1 {
		t.Fatalf("open %v, entries made %d times", menu.Popup().IsOpen(), made)
	}
	p.Type(ggui.Mods{}, ggui.KeyEnd, ggui.KeyEnter)
	if picked != "delete" || menu.Popup().IsOpen() {
		t.Fatal("keyboard in a made menu", picked)
	}
	act(t, p, ggui.RoleMenu, "Actions", ggui.Action{Kind: ggui.ActionExpand})
	// Rebuilt while open, the menu makes its entries again and shows them.
	version.Set(4)
	p.Frame()
	if !menu.Popup().IsOpen() {
		t.Fatal("rebuilt menu closed")
	}
	act(t, p, ggui.RoleMenuItem, "Copy", ggui.Action{Kind: ggui.ActionPress})
	if picked != "copy" {
		t.Fatal("item of a menu rebuilt open", picked)
	}
}

func TestNativeContextMenuFallback(t *testing.T) {
	t.Parallel()
	builds, runs := 0, 0
	enabled := ggui.State(false)
	menu := ui.NativeContextMenuFunc(ggui.Text("Target"), func() []ggui.MenuItem {
		builds++
		version := builds
		return []ggui.MenuItem{
			ggui.MenuAction("Unavailable", "", func() { t.Fatal("disabled action ran") }).BindEnabled(enabled),
			ggui.MenuSeparator(),
			ggui.MenuAction("Run", "", func() { runs += version }),
		}
	}).Name("Native actions")
	p := ggui.NewProbe(menu, ggui.Sz(320, 300))
	defer p.Close()
	if builds != 0 {
		t.Fatal("entries built before invocation")
	}
	act(t, p, ggui.RoleMenu, "Native actions", ggui.Action{Kind: ggui.ActionExpand})
	if !menu.Popup().IsOpen() || builds != 1 {
		t.Fatalf("fallback open=%v builds=%d", menu.Popup().IsOpen(), builds)
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if runs != 1 || menu.Popup().IsOpen() {
		t.Fatal("fallback selection failed")
	}
	p.Type(ggui.Mods{Shift: true}, ggui.KeyF10)
	if builds != 2 || !menu.Popup().IsOpen() {
		t.Fatal("entries were not rebuilt on invocation")
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if runs != 3 {
		t.Fatalf("stale menu action: runs=%d", runs)
	}
	menu.Disabled(true)
	actResult := menu.Act(ggui.Action{Kind: ggui.ActionExpand})
	if actResult || builds != 2 {
		t.Fatal("disabled native menu invoked")
	}
}
