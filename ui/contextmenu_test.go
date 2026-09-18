package ui_test

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"testing"
)

func TestContextMenuPointerKeyboardAndAccessibility(t *testing.T) {
	clicks, picked := 0, ""
	menu := ui.ContextMenu(ui.Button("Content", func() { clicks++ }),
		ui.MenuItem("Unavailable", func() { t.Fatal("disabled action") }).Disabled(true),
		ui.MenuItem("Copy", func() { picked = "copy" }), ui.MenuDivider(),
		ui.MenuItem("Delete", func() { picked = "delete" })).Named("File actions")
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
	for _, entries := range [][]ggui.Widget{nil, {ui.MenuItem("Disabled", func() { t.Fatal("disabled action ran") }).Disabled(true)}} {
		menu := ui.ContextMenu(ggui.Text("Target"), entries...).Named("Actions")
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
