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

func TestNavigationMenuHoverOpensAndCloses(t *testing.T) {
	t.Parallel()
	build := func() *ui.NavigationMenuWidget {
		return ui.NavigationMenu(
			ui.NavigationMenuItem("Products", ui.NavigationMenuLink("Editor", nil)),
			ui.NavigationMenuItem("Learn", ui.NavigationMenuLink("Docs", nil)),
		)
	}
	nav := build()
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(nav)), ggui.Sz(500, 350))
	defer p.Close()
	p.Frame()
	products, _ := p.Find("Products")
	// Base UI's NavigationMenu waits 50ms to open and 50ms to close.
	p.Move(products.Center())
	p.Advance(30 * time.Millisecond)
	if nav.Popup().IsOpen() {
		t.Fatal("dropdown opened before the hover delay")
	}
	p.Advance(30 * time.Millisecond)
	if _, ok := p.FindRole(ggui.RoleLink, "Editor"); !ok || !nav.Popup().IsOpen() {
		t.Fatal("resting on a trigger did not open its dropdown")
	}
	learn, _ := p.Find("Learn")
	p.Move(learn.Center())
	if _, ok := p.FindRole(ggui.RoleLink, "Docs"); !ok {
		t.Fatal("moving to another trigger did not switch dropdowns")
	}
	p.Move(ggui.Pt(495, 345))
	p.Advance(30 * time.Millisecond)
	if !nav.Popup().IsOpen() {
		t.Fatal("dropdown closed before the pointer had been away a while")
	}
	p.Advance(30 * time.Millisecond)
	if nav.Popup().IsOpen() {
		t.Fatal("a dropdown the pointer opened stayed open after it left")
	}

	// A click opens it to stay, even once the pointer has left.
	p.Tap("Learn")
	p.Move(ggui.Pt(495, 345))
	p.Advance(time.Second)
	if !nav.Popup().IsOpen() {
		t.Fatal("a dropdown opened with a click closed when the pointer left")
	}
	p.Key("escape")

	// A click on a dropdown the pointer just opened keeps it open.
	p.Move(products.Center())
	p.Advance(300 * time.Millisecond)
	p.Click(products.Center())
	p.Move(ggui.Pt(495, 345))
	p.Advance(time.Second)
	if !nav.Popup().IsOpen() {
		t.Fatal("clicking a hover-opened trigger closed it or left it closing on leave")
	}
	p.Click(products.Center())
	p.Advance(time.Second)
	if nav.Popup().IsOpen() {
		t.Fatal("a second click did not close the dropdown")
	}

	off := build().OpenOnHover(false)
	q := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(off)), ggui.Sz(500, 350))
	defer q.Close()
	q.Frame()
	trigger, _ := q.Find("Products")
	q.Move(trigger.Center())
	q.Advance(time.Second)
	if off.Popup().IsOpen() {
		t.Fatal("OpenOnHover(false) still opened on hover")
	}
}

func TestNavigationMenuPanelMotionSettles(t *testing.T) {
	t.Parallel()
	nav := ui.NavigationMenu(
		ui.NavigationMenuItem("Products", ui.NavigationMenuLink("Editor", nil)).Width(200),
		ui.NavigationMenuItem("Learn", ui.NavigationMenuLink("Docs", nil)).Width(300),
	).OpenOnHover(false)
	p := ggui.NewProbe(ggui.Column(nav), ggui.Sz(600, 350))
	defer p.Close()
	p.Tap("Products")
	p.Advance(time.Second)
	first := nav.Popup().Rect()
	p.Tap("Learn")
	for range 5 {
		p.Advance(20 * time.Millisecond)
	}
	mid := nav.Popup().Rect()
	if mid.Size.W <= first.Size.W || mid.Size.W >= 300 {
		t.Fatalf("panel width mid-switch=%v, want between %v and 300", mid.Size.W, first.Size.W)
	}
	if _, ok := p.FindRole(ggui.RoleLink, "Docs"); !ok {
		t.Fatal("the arriving content is not in the panel")
	}
	p.Advance(time.Second)
	if end := nav.Popup().Rect(); end.Size.W != 300 || end.Origin.X <= first.Origin.X {
		t.Fatalf("panel after switch=%v, from %v", end, first)
	}
	if _, ok := p.FindRole(ggui.RoleLink, "Editor"); ok {
		t.Fatal("the leaving content stayed after the switch")
	}
}
