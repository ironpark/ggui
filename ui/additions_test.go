package ui_test

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestPaginationNavigation(t *testing.T) {
	page, pages := ggui.State(1), ggui.State(12)
	changes := 0
	nav := ui.Pagination(page, pages).OnChange(func(int) { changes++ })
	p := ggui.NewProbe(nav, ggui.Sz(500, 120))
	defer p.Close()
	p.Frame()
	if _, ok := p.Find("Previous"); ok {
		t.Fatal("first page has an enabled Previous button")
	}
	if page.Peek() != 1 || changes != 0 {
		t.Fatal("previous changed the first page")
	}
	p.Tap("Page 3")
	if node, ok := p.Semantics().Find(ggui.RoleButton, "Page 3"); !ok || !node.Selected {
		t.Fatal("current page is not marked selected")
	}
	if page.Peek() != 3 || changes != 1 {
		t.Fatal("numbered navigation failed")
	}
	p.Type(ggui.Mods{}, ebiten.KeySpace)
	if changes != 1 {
		t.Fatal("selecting the current page fired OnChange")
	}
	p.Tap("Next")
	p.Type(ggui.Mods{}, ebiten.KeySpace)
	if page.Peek() != 5 || changes != 3 {
		t.Fatalf("keyboard navigation: page=%d changes=%d", page.Peek(), changes)
	}
	page.Set(12)
	p.Frame()
	if _, ok := p.Find("Next"); ok {
		t.Fatal("last page has an enabled Next button")
	}
	if page.Peek() != 12 {
		t.Fatal("next moved past the last page")
	}
	nav.Disabled(true)
	p.Frame()
	if _, ok := p.Find("Previous"); ok {
		t.Fatal("disabled navigation registers input")
	}
	if page.Peek() != 12 {
		t.Fatal("disabled navigation changed page")
	}
	nav.Disabled(false)
	pages.Set(2)
	p.Frame()
	if page.Peek() != 12 {
		t.Fatal("layout wrote to the page binding")
	}
	if _, ok := p.Find("Page 3"); ok {
		t.Fatal("stale page button after page count shrank")
	}
	p.Tap("Previous")
	if page.Peek() != 1 {
		t.Fatal("navigation did not use clamped displayed page")
	}
	for _, n := range []int{0, -1} {
		pages.Set(n)
		p.Frame()
		if _, ok := p.Find("Next"); ok {
			t.Fatal("empty navigation enables Next")
		}
		if _, ok := p.Find("Previous"); ok {
			t.Fatal("empty navigation enables Previous")
		}
		if page.Peek() != 1 {
			t.Fatal("empty pagination changed the binding")
		}
		if _, ok := p.Find("Page 1"); ok {
			t.Fatal("empty pagination contains a page button")
		}
	}
	pages.Set(1000000)
	page.Set(500000)
	p.Frame()
	for _, label := range []string{"Page 499998", "Page 500000", "Page 500002"} {
		find(t, p, ggui.RoleButton, label)
	}
	if _, ok := p.Find("Page 500003"); ok {
		t.Fatal("pagination window is not bounded")
	}
}

func TestNoticeActionsAndKeyboard(t *testing.T) {
	retries, creates := 0, 0
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Column(
			ui.Alert("Upload failed", "Try again when connected.").Destructive().Action(ui.Button("Retry", func() { retries++ })),
			ui.Empty("No files", "Create your first file.").Media(ui.Kbd("N")).Action(ui.Button("Create", func() { creates++ })),
			ui.Skeleton(120, 16), ui.Spinner(),
		)
	}, ggui.Sz(320, 400))
	defer p.Close()
	p.Type(ggui.Mods{}, ebiten.KeyTab)
	p.Type(ggui.Mods{}, ebiten.KeyEnter)
	if retries != 1 {
		t.Fatal("alert action is not keyboard reachable")
	}
	p.Type(ggui.Mods{}, ebiten.KeyTab)
	p.Type(ggui.Mods{}, ebiten.KeySpace)
	if creates != 1 {
		t.Fatal("empty-state action is not keyboard reachable")
	}
	p.Resize(ggui.Sz(160, 800))
	for _, label := range []string{"Retry", "Create"} {
		r := find(t, p, ggui.RoleButton, label).Rect
		if r.Origin.X < 0 || r.Origin.X+r.Size.W > 160 {
			t.Fatalf("%s overflows narrow panel: %+v", label, r)
		}
	}
	p.Tap("Retry")
	if retries != 2 {
		t.Fatal("alert pointer action failed")
	}
}
