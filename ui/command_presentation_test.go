package ui_test

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"testing"
)

func TestGroupedCommandFilteringAndNavigation(t *testing.T) {
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
