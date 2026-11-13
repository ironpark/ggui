package ui

import (
	"image/color"
	"testing"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// Each variant has its own surface under the pointer: the primary's hover
// color, the muted surface, a deeper wash of the destructive color.
func TestBubbleVariantsHoverOnTheirOwnSurfaces(t *testing.T) {
	t.Parallel()
	for _, th := range []uitheme.Theme{uitheme.Default(), uitheme.Dark()} {
		dark := isDark(th.Bg)
		for _, tc := range []struct {
			name string
			set  func(*BubbleWidget) *BubbleWidget
			want color.Color
		}{
			{"default", func(b *BubbleWidget) *BubbleWidget { return b }, th.PrimaryHover},
			{"outline", (*BubbleWidget).Outline, pick(dark, fade(th.InputBorder, .3), th.Muted)},
			{"ghost", (*BubbleWidget).Ghost, pick(dark, fade(th.Muted, .5), th.Muted)},
			{"destructive", (*BubbleWidget).Destructive, fade(th.Destructive, pick(dark, .3, .2))},
		} {
			b := tc.set(Bubble(ggui.Text("hi")).Action("Reply", func() {}))
			p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, uitheme.With(th, b)), ggui.Sz(300, 80))
			p.Frame()
			p.Move(ggui.Pt(8, 8))
			p.Frame()
			if got := b.hoverFill(); !b.Hovered || !sameColor(got, tc.want) {
				t.Errorf("%s dark=%v: hovered=%v fill %v, want %v", tc.name, dark, b.Hovered, got, tc.want)
			}
			p.Close()
		}
	}
}

// A tint is a wash of the color about as bright as asked, whatever the
// color: pale on a light page and deep on a dark one, for a neutral primary
// and a colored one alike.
func TestTintOfReachesTheBrightnessAskedFor(t *testing.T) {
	t.Parallel()
	white, black := color.RGBA{255, 255, 255, 255}, color.RGBA{10, 10, 10, 255}
	for _, c := range []color.RGBA{{23, 23, 23, 255}, {229, 229, 229, 255}, {20, 71, 230, 255}, {231, 0, 11, 255}} {
		if got := luma(tintOf(white, c, .91)); got < .86 || got > .96 {
			t.Errorf("light tint of %v has luma %.2f, want about .91", c, got)
		}
		// A color darker than the tint asked for gives as much of itself
		// as a tint may take.
		if got := luma(tintOf(black, c, .18)); got < .04 || got > .22 {
			t.Errorf("dark tint of %v has luma %.2f, want at most about .18", c, got)
		}
	}
}
