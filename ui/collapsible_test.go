package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// CollapsibleOf folds its content under a trigger of the caller's own.
func TestCollapsibleOfTogglesFromItsTrigger(t *testing.T) {
	t.Parallel()
	open := ggui.State(false)
	c := ui.CollapsibleOf(open, ggui.Text("Filters"), ui.Button("Apply", nil)).Name("Filters")
	p := ggui.NewProbe(ggui.Column(c), ggui.Sz(300, 300))
	defer p.Close()
	if _, ok := p.FindRole(ggui.RoleButton, "Apply"); ok {
		t.Fatal("closed content takes input")
	}
	p.Tap("Filters")
	p.Advance(time.Second)
	if !ggui.Untrack(open.Get) || c.Progress() != 1 {
		t.Fatalf("open %v, progress %v", ggui.Untrack(open.Get), c.Progress())
	}
	if _, ok := p.FindRole(ggui.RoleButton, "Apply"); !ok {
		t.Fatal("open content is not there")
	}
	if n, _ := p.Semantics().Find(ggui.RoleDisclosure, "Filters"); n.Expanded != ggui.Expandable(true) {
		t.Fatalf("the disclosure says %+v", n.Expanded)
	}
}
