package ui

import (
	"testing"

	"github.com/ironpark/ggui"
)

// A tooltip and a hover card are placed as a Base UI positioner places its
// popup: 5px inside the window's edges, flipping when the side it asks for
// has no room that far inside.
func TestFloatingPanelsKeepTheCollisionPaddingFromTheEdges(t *testing.T) {
	screen, size := ggui.Sz(400, 300), ggui.Sz(100, 50)
	for _, c := range []struct {
		name   string
		anchor ggui.Rect
		side   ggui.PopupSide
		align  ggui.PopupAlignment
		want   ggui.Point
		on     ggui.PopupSide
	}{
		{"centered over an anchor at the left edge", ggui.Rct(ggui.Pt(0, 100), ggui.Sz(40, 20)), ggui.PopupTop, ggui.PopupCenter, ggui.Pt(5, 46), ggui.PopupTop},
		{"centered over an anchor at the right edge", ggui.Rct(ggui.Pt(360, 100), ggui.Sz(40, 20)), ggui.PopupTop, ggui.PopupCenter, ggui.Pt(295, 46), ggui.PopupTop},
		// 56px above the anchor less the 4px gap is room for 50 flush
		// against the top, and not 5px inside it.
		{"over an anchor with room only flush against the top", ggui.Rct(ggui.Pt(150, 56), ggui.Sz(40, 20)), ggui.PopupTop, ggui.PopupCenter, ggui.Pt(120, 80), ggui.PopupBottom},
		{"beside an anchor at the bottom edge", ggui.Rct(ggui.Pt(150, 280), ggui.Sz(40, 20)), ggui.PopupRight, ggui.PopupStart, ggui.Pt(194, 245), ggui.PopupRight},
	} {
		got, on, _ := floatBeside(screen, c.anchor, size, c.side, c.align, 0, 4, false)
		if got.Origin != c.want || on != c.on {
			t.Errorf("%s: at %v on side %v, want %v on side %v", c.name, got.Origin, on, c.want, c.on)
		}
	}
}
