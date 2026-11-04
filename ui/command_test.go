package ui_test

import (
	"strings"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestGroupedCommandFilteringAndNavigation(t *testing.T) {
	t.Parallel()
	query := ggui.State("")
	picked := ""
	command := ui.Command(query,
		ui.CommandItem("Alpha", func() { picked = "Alpha" }).Group("First"),
		ui.CommandItem("Disabled", nil).Group("First").Disabled(true),
		ui.CommandItem("Beta", func() { picked = "Beta" }).Group("Second"),
	).Height(220).InsetSearch()
	dialog := ui.CommandDialog(ggui.State(true), command)
	p := ggui.NewProbe(dialog, ggui.Sz(500, 400))
	defer p.Close()
	if _, ok := p.Semantics().Find(ggui.RoleDialog, "Commands"); !ok {
		t.Fatal("palette lost accessible name")
	}
	p.Tap("Search commands")
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	if picked != "Beta" {
		t.Fatalf("group headings or disabled row interrupted navigation: %s", picked)
	}
	query.Set("Beta")
	p.Frame()
	if _, ok := p.Find("Alpha"); ok {
		t.Fatal("filtered action remains visible")
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if picked != "Beta" {
		t.Fatal("filtered selection failed")
	}
}

func TestCommandDialogClosesOnChoosingAndTakesNewEntries(t *testing.T) {
	t.Parallel()
	open := ggui.State(true)
	picked := ""
	command := ui.Command(ggui.State(""), ui.CommandItem("Alpha", func() { picked = "Alpha" })).
		Empty("Nothing here").Hints().HintLabels("Move", "Run")
	p := ggui.NewProbe(ui.CommandDialog(open, command), ggui.Sz(500, 400))
	defer p.Close()
	if !strings.Contains(p.Semantics().String(), "Move") {
		t.Fatalf("the footer kept its own words:\n%s", p.Semantics())
	}
	p.Tap("Alpha")
	if picked != "Alpha" || ggui.Peek(open) {
		t.Fatalf("picked %q, open %v", picked, ggui.Peek(open))
	}
	command.Entries(ui.CommandItem("Beta", func() { picked = "Beta" }))
	open.Set(true)
	p.Frame()
	if _, ok := p.Find("Alpha"); ok {
		t.Fatal("the old entries remain")
	}
	p.Tap("Beta")
	if picked != "Beta" {
		t.Fatalf("picked %q", picked)
	}
	command.Entries()
	open.Set(true)
	if !strings.Contains(p.Semantics().String(), "Nothing here") {
		t.Fatal("the empty list kept its own words")
	}
}
