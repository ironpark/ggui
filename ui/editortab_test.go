package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/widget"
)

// A tab picks what it stands for, and its close button closes it: the
// button is there, to be found and tapped, whether or not it shows.
func TestEditorTabSelectsAndCloses(t *testing.T) {
	var picked, closed string
	tab := func(name string) ggui.Widget {
		return ui.EditorTab(name, func() { picked = name }).OnClose(func() { closed = name }).CloseName("Shut " + name)
	}
	p := ggui.NewProbe(widget.Row(
		tab("a.go"),
		ui.TabGroup(tab("b.go").(*ui.EditorTabWidget).Dirty(true), ui.EditorTab("c.go", func() { picked = "c.go" }).Preview(true)),
	), ggui.Sz(600, 40))
	defer p.Close()
	p.Frame()
	p.Tap("b.go")
	if picked != "b.go" {
		t.Fatalf("tapping b.go picked %q", picked)
	}
	p.Tap("Shut a.go")
	if closed != "a.go" || picked != "b.go" {
		t.Fatalf("closing a.go closed %q and picked %q", closed, picked)
	}
	if _, ok := p.Find("Close c.go"); ok {
		t.Fatal("a tab with no OnClose has a close button")
	}
	p.Tap("c.go")
	if picked != "c.go" {
		t.Fatalf("tapping the tab in the group picked %q", picked)
	}
}
