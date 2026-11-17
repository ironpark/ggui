package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/widget"
)

// CollapsibleOf folds its content under a trigger of the caller's own.
func TestCollapsibleOfTogglesFromItsTrigger(t *testing.T) {
	t.Parallel()
	open := ggui.State(false)
	c := ui.CollapsibleOf(open, widget.Text("Filters"), ui.Button("Apply", nil)).Name("Filters")
	p := ggui.NewProbe(widget.Column(c), ggui.Sz(300, 300))
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

// A disclosure's header is hovered over itself, not over a control in the
// body it reveals, though the body is painted inside the header's region.
func TestACollapsiblesHeaderIsNotHoveredOverItsBody(t *testing.T) {
	t.Parallel()
	open := ggui.State(true)
	apply := ui.Button("Apply", nil)
	c := ui.Collapsible(open, "Filters", apply)
	p := ggui.NewProbe(widget.Column(c), ggui.Sz(300, 300))
	defer p.Close()
	p.Advance(time.Second)
	h, ok := p.FindRole(ggui.RoleDisclosure, "Filters")
	if !ok {
		t.Fatal("no header")
	}
	b, ok := p.FindRole(ggui.RoleButton, "Apply")
	if !ok {
		t.Fatal("no button in the body")
	}
	p.Move(ggui.Pt(h.Rect.Origin.X+10, h.Rect.Origin.Y+10))
	if !c.Hovered {
		t.Fatal("the header is not hovered under the pointer")
	}
	p.Move(b.Center())
	if c.Hovered || !apply.Hovered {
		t.Fatalf("over the body's button: header hovered %v, button hovered %v; want the button alone", c.Hovered, apply.Hovered)
	}
}
