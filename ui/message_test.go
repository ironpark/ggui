package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// A header starts where the bubble's text does, 12px in, and at the edge
// beside a ghost bubble, which has no padding of its own.
func TestMessageHeaderLinesUpWithTheBubble(t *testing.T) {
	t.Parallel()
	inset := func(content ggui.Widget) float64 {
		p := ggui.NewProbe(ui.Message(content).Header(ggui.Text("Ada")).Footer(ggui.Text("Sent")), ggui.Sz(400, 200))
		defer p.Close()
		header, footer := node(t, p.Semantics(), ggui.RoleText, "Ada"), node(t, p.Semantics(), ggui.RoleText, "Sent")
		if header.Rect.Origin.X != footer.Rect.Origin.X {
			t.Fatalf("header at %v, footer at %v", header.Rect.Origin.X, footer.Rect.Origin.X)
		}
		return header.Rect.Origin.X
	}
	if got := inset(ui.Bubble(ggui.Text("Hello"))); got != 12 {
		t.Errorf("header inset beside a bubble = %v, want 12", got)
	}
	if got := inset(ui.Bubble(ggui.Text("Hello")).Ghost()); got != 0 {
		t.Errorf("header inset beside a ghost bubble = %v, want 0", got)
	}
	if got := inset(ui.BubbleGroup(ui.Bubble(ggui.Text("Hello")).Ghost())); got != 0 {
		t.Errorf("header inset beside a group of ghost bubbles = %v, want 0", got)
	}
}

// With a footer the avatar rises 32px off the row's bottom, so it rests
// beside the bubble and not the footer.
func TestMessageAvatarRisesAboveTheFooter(t *testing.T) {
	t.Parallel()
	bubble := ui.Bubble(ggui.Text("One\nTwo\nThree"))
	p := ggui.NewProbe(ui.Message(bubble).Avatar(ui.Avatar("Ada").Size(32)).Footer(ggui.Text("Sent")), ggui.Sz(400, 300))
	defer p.Close()
	avatar, footer := node(t, p.Semantics(), ggui.RoleImage, "Ada"), node(t, p.Semantics(), ggui.RoleText, "Sent")
	bottom := footer.Rect.Origin.Y + footer.Rect.Size.H
	if got := bottom - (avatar.Rect.Origin.Y + avatar.Rect.Size.H); got != 32 {
		t.Fatalf("avatar ends %v above the row's bottom, want 32", got)
	}
}
