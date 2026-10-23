package main

import (
	"image/color"
	"math"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// effectsDemo shows the painting effects: a card that turns and still takes
// clicks where it is drawn, gradient fills, shape clips and blurs.
func effectsDemo() ggui.Widget {
	return ggui.Component(func() ggui.Widget {
		turned := ggui.State(false)
		angle := ggui.Tween(0.0, 300*time.Millisecond)
		sigma := ggui.State(3.0)
		clicks := ggui.State(0)
		red, blue, green := color.NRGBA{R: 230, G: 72, B: 72, A: 255}, color.NRGBA{R: 64, G: 110, B: 230, A: 255}, color.NRGBA{R: 52, G: 180, B: 110, A: 255}
		swatch := func(g ggui.Gradient, radius float64) ggui.Widget {
			return ggui.Box().Size(72, 72).Gradient(g).Radius(radius)
		}
		stripes := ggui.Row(
			ggui.Box().Size(12, 72).Fill(red), ggui.Box().Size(12, 72).Fill(blue),
			ggui.Box().Size(12, 72).Fill(red), ggui.Box().Size(12, 72).Fill(blue),
			ggui.Box().Size(12, 72).Fill(red), ggui.Box().Size(12, 72).Fill(blue),
		)
		return ggui.Column(
			ggui.Wrap(
				ggui.Transform(ui.Button("Click me", func() { clicks.Set(clicks.Get() + 1) })).BindRotate(angle),
				ui.Button("Turn", func() {
					turned.Set(!turned.Get())
					target := 0.0
					if turned.Get() {
						target = math.Pi / 10
					}
					angle.Set(target)
				}).Outline(),
				ggui.Textf("%d clicks", clicks),
			).Gap(16),
			ggui.Wrap(
				swatch(ggui.LinearGradient(math.Pi/4, ggui.Stops(red, green, blue)...), 12),
				swatch(ggui.RadialGradient(ggui.Stops(color.White, blue)...), 12),
				swatch(ggui.SweepGradient(ggui.Stops(red, green, blue, red)...), 36),
				ggui.ClipOval(swatch(ggui.LinearGradient(math.Pi/2, ggui.Stops(green, blue)...), 0)),
				ggui.Opacity(swatch(ggui.LinearGradient(0, ggui.Stops(red, blue)...), 12), 0.5),
			).Gap(16),
			ggui.Wrap(
				ggui.Blur(stripes, 0).BindSigma(sigma),
				ggui.Stack(stripes, ggui.Padding(ggui.BackdropBlur(ggui.Box().Size(48, 32).Fill(color.NRGBA{R: 255, G: 255, B: 255, A: 60}).Radius(8), 4).Radius(8), 20, 12)),
				ggui.Box(ui.Slider(sigma, 0, 12)).Width(160),
			).Gap(16),
		).Gap(20)
	})
}
