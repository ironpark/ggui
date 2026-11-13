package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestNavigationMenuLinksDropdownsAndFocus(t *testing.T) {
	t.Parallel()
	destination := ""
	nav := ui.NavigationMenu(
		ui.NavigationMenuLink("Home", func() { destination = "home" }).Active(true),
		ui.NavigationMenuLink("Disabled", func() { t.Fatal("disabled link activated") }).Disabled(true),
		ui.NavigationMenuItem("Products", ggui.Column(ui.NavigationMenuLink("Editor", func() { destination = "editor" }), ui.NavigationMenuLink("Tools", func() { destination = "tools" }).CloseOnClick(true))),
		ui.NavigationMenuItem("Learn", ggui.Column(ui.NavigationMenuLink("Docs", func() { destination = "docs" }))),
	).Name("Main navigation")
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(nav)), ggui.Sz(600, 400))
	defer p.Close()
	p.Key("tab")
	focused, _ := p.Semantics().Focused()
	if focused.Name != "Home" || focused.Role != ggui.RoleLink || !focused.Selected {
		t.Fatalf("first link=%+v", focused)
	}
	p.Key("right")
	focused, _ = p.Semantics().Focused()
	if focused.Name != "Products" {
		t.Fatalf("Right did not skip disabled: %+v", focused)
	}
	p.Key("down")
	p.Move(ggui.Pt(590, 390))
	focused, _ = p.Semantics().Focused()
	if focused.Name != "Editor" || focused.Role != ggui.RoleLink {
		t.Fatalf("dropdown initial focus=%+v", focused)
	}
	p.Key("tab")
	p.Key("enter")
	if destination != "tools" || nav.Popup().IsOpen() {
		t.Fatalf("link activation: %q, open=%v", destination, nav.Popup().IsOpen())
	}
	p.Move(ggui.Pt(590, 390))
	focused, _ = p.Semantics().Focused()
	if focused.Name != "Products" {
		t.Fatalf("restored focus=%s", focused.Name)
	}
	p.Key("end")
	p.Key("enter")
	if _, ok := p.FindRole(ggui.RoleLink, "Docs"); !ok {
		t.Fatal("End/Enter did not open last dropdown")
	}
	p.Key("escape")
	if nav.Popup().IsOpen() {
		t.Fatal("Escape did not dismiss")
	}
	p.Key("home")
	p.Key("enter")
	if destination != "home" {
		t.Fatal("Home link did not navigate")
	}
}

func TestNavigationMenuPointerSwitchAndDisabled(t *testing.T) {
	t.Parallel()
	selected := ""
	disabled := ggui.State(false)
	products := ui.NavigationMenuItem("Products", ui.NavigationMenuLink("Editor", nil)).BindDisabled(disabled)
	nav := ui.NavigationMenu(products, ui.NavigationMenuItem("Learn", ui.NavigationMenuLink("Docs", func() { selected = "docs" })), ui.NavigationMenuLink("Home", func() { selected = "home" }).CloseOnClick(true))
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(nav)), ggui.Sz(500, 350))
	defer p.Close()
	p.Tap("Products")
	p.Tap("Learn")
	if _, ok := p.Find("Docs"); !ok {
		t.Fatal("click did not switch open dropdown")
	}
	if _, ok := p.Find("Editor"); ok {
		t.Fatal("two dropdowns remained open")
	}
	p.Tap("Home")
	if selected != "home" || nav.Popup().IsOpen() {
		t.Fatal("top-level link blocked by popup")
	}
	p.Tap("Products")
	disabled.Set(true)
	p.Frame()
	if nav.Popup().IsOpen() {
		t.Fatal("disabled active item kept popup open")
	}
	nav.Disabled(true)
	p.Frame()
	if nav.Popup().IsOpen() {
		t.Fatal("disabled group stayed open")
	}
	if nav.Popup().Rect().Size.W > 500 {
		t.Fatal("panel exceeded viewport")
	}
}

func TestNavigationMenuAccessibleActionsAndRebuild(t *testing.T) {
	t.Parallel()
	revision := ggui.State(0)
	var nav *ui.NavigationMenuWidget
	p := ggui.ProbeBuilder(func() ggui.Widget {
		_ = revision.Get()
		nav = ui.NavigationMenu(ui.NavigationMenuItem("Browse", ui.NavigationMenuLink("Article", nil))).Key("nav")
		return ggui.Column(nav)
	}, ggui.Sz(300, 250))
	defer p.Close()
	act(t, p, ggui.RoleButton, "Browse", ggui.Action{Kind: ggui.ActionExpand})
	p.Advance(time.Second)
	revision.Set(1)
	p.Frame()
	if !nav.Popup().IsOpen() {
		t.Fatal("rebuild lost open dropdown")
	}
	if _, ok := p.FindRole(ggui.RoleLink, "Article"); !ok {
		t.Fatal("rebuild lost content")
	}
	p.Click(ggui.Pt(295, 245))
	if nav.Popup().IsOpen() {
		t.Fatal("outside click did not dismiss")
	}
}

func TestNavigationMenuEmptyAndAllDisabled(t *testing.T) {
	t.Parallel()
	for _, nav := range []*ui.NavigationMenuWidget{ui.NavigationMenu(), ui.NavigationMenu(ui.NavigationMenuLink("Disabled", nil).Disabled(true))} {
		p := ggui.NewProbe(nav, ggui.Sz(100, 100))
		p.Key("tab")
		p.Key("right")
		p.Key("enter")
		if nav.Popup().IsOpen() {
			t.Fatal("empty/disabled group opened")
		}
		p.Close()
	}
}

func TestNavigationMenuRTLAndNoWrap(t *testing.T) {
	t.Parallel()
	nav := ui.NavigationMenu(ui.NavigationMenuLink("One", nil), ui.NavigationMenuItem("Two", ui.NavigationMenuLink("Child", nil)), ui.NavigationMenuLink("Three", nil))
	p := ggui.NewProbe(ggui.Provide(ggui.DirectionKey, ggui.DirRTL, ggui.Column(nav)), ggui.Sz(500, 250))
	defer p.Close()
	p.Key("tab", "left")
	focused, _ := p.Semantics().Focused()
	if focused.Name != "Two" {
		t.Fatalf("RTL Left focus=%s", focused.Name)
	}
	p.Key("end", "left")
	focused, _ = p.Semantics().Focused()
	if focused.Name != "Three" {
		t.Fatalf("arrow wrapped: %s", focused.Name)
	}
}
