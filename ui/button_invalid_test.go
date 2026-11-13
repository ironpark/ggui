package ui

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// shadcn's button and toggle take aria-invalid, eased as the rest of a
// transition-all control is: Invalid and BindInvalid, as the other
// controls have them.
func TestButtonAndToggleInvalid(t *testing.T) {
	t.Parallel()
	bad := ggui.State(false)
	b := Button("Pick a date", nil).Outline().BindInvalid(bad)
	g := Toggle(ggui.State(false), "Bold").Outline().Invalid(true)
	var button, toggle float64
	root := ggui.Column(
		watched(b, &b.Interactive, buttonInvalidSlot, &button),
		watched(g, &g.Interactive, buttonInvalidSlot, &toggle),
	).Gap(12).Align(ggui.AlignStart)
	p := ggui.NewProbe(root, ggui.Sz(240, 120))
	defer p.Close()
	th := uitheme.Default()
	p.Advance(0)
	p.Advance(time.Second)
	p.Advance(0)
	if button > 0 {
		t.Fatalf("a valid button has its invalid state at %v", button)
	}
	if toggle != 1 {
		t.Fatalf("an invalid toggle has its invalid state at %v, want 1", toggle)
	}
	bad.Set(true)
	p.Advance(0)
	p.Advance(th.MotionFast / 2)
	if button <= 0 || button >= 1 {
		t.Fatalf("the invalid state is at %v half way through MotionFast, want between 0 and 1", button)
	}
	p.Advance(time.Second)
	p.Advance(0)
	if button != 1 {
		t.Fatalf("the invalid state settled at %v, want 1", button)
	}
	bad.Set(false)
	p.Advance(0)
	p.Advance(time.Second)
	p.Advance(0)
	if button != 0 {
		t.Fatalf("the invalid state stayed at %v once the button was valid again", button)
	}
}
