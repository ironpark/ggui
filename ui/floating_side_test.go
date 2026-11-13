package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestPopoverSideAndDensity(t *testing.T) {
	t.Parallel()
	pop := ui.Popover("Open", ui.Button("Inside", nil)).Title("Dimensions").Description("Set the dimensions.").Side(ggui.PopupRight).Align(ggui.PopupStart)
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Padding(pop, 100, 0, 0, 100)), ggui.Sz(800, 500))
	defer p.Close()
	p.Tap("Open")
	p.Frame()
	trigger, _ := p.Find("Open")
	r := pop.Popup().Rect()
	if r.Origin.X != trigger.Rect.Origin.X+trigger.Rect.Size.W+4 || r.Origin.Y != trigger.Rect.Origin.Y {
		t.Fatalf("a popover on the right is at %v beside its trigger at %v", r, trigger.Rect)
	}
	title := sem(t, p, ggui.RoleHeading, "Dimensions")
	desc := sem(t, p, ggui.RoleText, "Set the dimensions.")
	inside := find(t, p, ggui.RoleButton, "Inside").Rect
	if !near(title.Origin.X-r.Origin.X, 10) || !near(title.Origin.Y-r.Origin.Y, 10) {
		t.Errorf("the title starts %v, %v inside the panel, want 10, 10", title.Origin.X-r.Origin.X, title.Origin.Y-r.Origin.Y)
	}
	if gap := desc.Origin.Y - (title.Origin.Y + title.Size.H); !near(gap, 2) {
		t.Errorf("the description is %v below the title, want 2", gap)
	}
	if gap := inside.Origin.Y - (desc.Origin.Y + desc.Size.H); !near(gap, 10) {
		t.Errorf("the content is %v below the description, want 10", gap)
	}
	if title.Size.H != desc.Size.H {
		t.Errorf("the title is %v tall and the description %v: both are body text", title.Size.H, desc.Size.H)
	}
}

func TestHoverCardOpensOnTheSideItIsGiven(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		side  ggui.PopupSide
		check func(card, anchor ggui.Rect) bool
	}{
		{"bottom", ggui.PopupBottom, func(c, a ggui.Rect) bool {
			// shadcn's alignOffset = 4 moves a centered card toward the end.
			return c.Origin.Y == a.Origin.Y+a.Size.H+4 && near(c.Center().X, a.Center().X+4)
		}},
		{"top", ggui.PopupTop, func(c, a ggui.Rect) bool { return c.Origin.Y+c.Size.H == a.Origin.Y-4 }},
		{"right", ggui.PopupRight, func(c, a ggui.Rect) bool {
			return c.Origin.X == a.Origin.X+a.Size.W+4 && near(c.Center().Y, a.Center().Y+4)
		}},
		{"left", ggui.PopupLeft, func(c, a ggui.Rect) bool { return c.Origin.X+c.Size.W == a.Origin.X-4 }},
	} {
		card := ui.HoverCard(ggui.Box().Size(80, 24), ggui.Box().Height(60)).Name("Preview").Side(tc.side).Delay(0)
		p := ggui.NewProbe(ggui.Center(card), ggui.Sz(900, 500))
		p.Frame()
		p.Move(ggui.Pt(450, 250))
		p.Advance(time.Second)
		node, ok := p.Semantics().Find(ggui.RoleGroup, "Preview")
		anchor := ggui.Rct(ggui.Pt(410, 238), ggui.Sz(80, 24))
		if !ok || !tc.check(node.Rect, anchor) {
			t.Errorf("%s: the card is at %v (found %v) around its anchor at %v", tc.name, node.Rect, ok, anchor)
		}
		p.Close()
	}
}
