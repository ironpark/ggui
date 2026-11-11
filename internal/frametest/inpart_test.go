package frametest_test

import (
	"image/color"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// A row that appears with a write eases in through its Transition over
// frames painted in part, as only time moves it after the first: each
// frame reads where it started from what the frame before kept, and it
// comes to rest.
func TestARowAppearingEasesInInPart(t *testing.T) {
	frames := sameAsWhole(t, ggui.Sz(200, 160), func() (ggui.Builder, func(*ggui.WindowProbe)) {
		rows := ggui.State([]int{1})
		return func() ggui.Widget {
				list := ggui.EachKeyed(rows, func(i int) int { return i }, func(item ggui.EachItem[int]) ggui.Widget {
					return ggui.Box().Size(120, 24).Fill(red)
				}).Space(0.5).Transition(func(w ggui.Widget) *ggui.TransitionWidget { return ggui.Transition(w).Fade().Slide(-16, 0) })
				return ggui.Column(ggui.Box().Size(100, 20).Fill(blue), ggui.Expanded(ggui.Scroll(list)))
			}, func(*ggui.WindowProbe) {
				ggui.Append(rows, 2)
			}
	})
	if len(frames) < 4 {
		t.Fatalf("%d frames, want an easing over several", len(frames))
	}
	for i, fr := range frames[1:] {
		if fr.Painted != ggui.FramePartial {
			t.Fatalf("frame %d was painted %v, want in part: %v", i+1, fr.Painted, frames)
		}
	}
}

// A tooltip that opens after its delay, at a frame only time asked for,
// shows all of itself, though it opens over what lies beside its anchor.
func TestATooltipOpeningAtItsDelayShowsWhole(t *testing.T) {
	sameAsWhole(t, ggui.Sz(240, 160), func() (ggui.Builder, func(*ggui.WindowProbe)) {
		return func() ggui.Widget {
				anchor := ggui.Box().Size(60, 24).Fill(blue)
				return topLeft(ggui.Column(ui.Tooltip(anchor, "A tip that runs past its anchor"), ggui.Box().Size(200, 80).Fill(green)))
			}, func(p *ggui.WindowProbe) {
				p.Move(ggui.Pt(30, 12))
			}
	})
}

// A row removed with a write eases out through its Transition, inert, as
// the rows below close up, and shows at every frame what painting it whole
// does.
func TestARowLeavingEasesOut(t *testing.T) {
	frames := sameAsWhole(t, ggui.Sz(200, 160), func() (ggui.Builder, func(*ggui.WindowProbe)) {
		rows := ggui.State([]int{1, 2, 3})
		return func() ggui.Widget {
				list := ggui.EachKeyed(rows, func(i int) int { return i }, func(item ggui.EachItem[int]) ggui.Widget {
					return ggui.Box().Size(120, 24).Fill([]color.Color{red, green, blue}[item.Item-1])
				}).Space(0.5).Transition(func(w ggui.Widget) *ggui.TransitionWidget { return ggui.Transition(w).Fade().Slide(-16, 0) })
				return ggui.Column(ggui.Box().Size(100, 20).Fill(gray), ggui.Expanded(ggui.Scroll(list)))
			}, func(*ggui.WindowProbe) {
				ggui.Remove(rows, func(i int) bool { return i == 2 })
			}
	})
	if len(frames) < 4 {
		t.Fatalf("%d frames, want an easing over several", len(frames))
	}
}
