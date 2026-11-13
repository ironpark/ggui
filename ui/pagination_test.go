package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestPaginationPageButtonsAreSquareAndEllipsesAreIcons(t *testing.T) {
	t.Parallel()
	p := ggui.NewProbe(ggui.Column(ui.Pagination(ggui.State(6), ggui.State(12))), ggui.Sz(600, 120))
	defer p.Close()
	tree := p.Semantics()
	page, ok := tree.Find(ggui.RoleButton, "Page 6")
	if !ok {
		t.Fatal("no button for the current page")
	}
	if page.Rect.Size.W != page.Rect.Size.H || page.Rect.Size.H != 32 {
		t.Fatalf("a page button is %v, want the 32px square of an icon button", page.Rect.Size)
	}
	next, _ := tree.Find(ggui.RoleButton, "Page 7")
	if gap := next.Rect.Origin.X - (page.Rect.Origin.X + page.Rect.Size.W); gap != 2 {
		t.Fatalf("page buttons are %v apart, want 2", gap)
	}
	if got := count(p, ggui.RoleImage); got != 2 {
		t.Fatalf("%d images stand for the pages left out, want one at each end", got)
	}
	if _, ok := tree.Find(ggui.RoleImage, "More pages"); !ok {
		t.Fatal("the ellipsis is not named")
	}
}

func TestPaginationLabelsRenameThePreviousAndNextButtons(t *testing.T) {
	t.Parallel()
	page := ggui.State(2)
	p := ggui.NewProbe(ggui.Column(ui.Pagination(page, ggui.State(3)).Labels("Zurück", "Weiter")), ggui.Sz(600, 120))
	defer p.Close()
	if _, ok := p.Find("Previous"); ok {
		t.Fatal("the default label survived Labels")
	}
	p.Tap("Weiter")
	if got := ggui.Untrack(page.Get); got != 3 {
		t.Fatalf("the renamed Next moved to page %d, want 3", got)
	}
	p.Tap("Zurück")
	if got := ggui.Untrack(page.Get); got != 2 {
		t.Fatalf("the renamed Previous moved to page %d, want 2", got)
	}
}
