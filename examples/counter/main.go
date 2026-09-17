// Command counter is a minimal ggui app: a struct of signals for state,
// derived text built with generic methods, and buttons that react to the
// pointer. Click the buttons or press space to count; up/down changes the
// step.
package main

import (
	"fmt"
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/ironpark/ggui"
)

// model holds one signal per piece of state. Each field is its own reactive
// cell, so a change to Count re-runs only what read Count.
type model struct {
	Count *ggui.Signal[int]
	Step  *ggui.Signal[int]
}

var (
	fg     = color.White
	dim    = color.RGBA{0x8a, 0x90, 0x9c, 0xff}
	panel  = color.RGBA{0x23, 0x27, 0x2f, 0xff}
	button = color.RGBA{0x3a, 0x40, 0x4c, 0xff}
	hover  = color.RGBA{0x4c, 0x54, 0x63, 0xff}
)

// buttonWidget is a component: a plain function from state to Widget. The
// hovered signal lives outside so it survives rebuilds.
func buttonWidget(label string, hovered *ggui.Signal[bool], onTap func()) ggui.Widget {
	bg := button
	if hovered.Get() {
		bg = hover
	}
	return ggui.Pointer(
		ggui.Box(ggui.Text(label).Color(fg).Size(18)).Pad(6, 16).Fill(bg),
	).OnTap(onTap).OnHover(hovered.Set)
}

func main() {
	state := model{Count: ggui.State(0), Step: ggui.State(1)}
	count, step := state.Count, state.Step
	minusHover, plusHover := ggui.State(false), ggui.State(false)

	// Map derives text from a cell; it recomputes only when the cell changes.
	label := count.Map(func(n int) string { return fmt.Sprintf("count: %d", n) })
	hints := ggui.Combine(count, step, func(n, s int) []string {
		return []string{
			fmt.Sprintf("click a button or press space to add %d", s),
			fmt.Sprintf("up/down changes the step (now %d)", s),
			fmt.Sprintf("%d more to reach 100", max(100-n, 0)),
		}
	})

	app := ggui.New(ggui.Config{
		Title:      "ggui · counter",
		Width:      480,
		Height:     320,
		Resizable:  true,
		Background: color.RGBA{0x14, 0x16, 0x1a, 0xff},
	}, func() ggui.Widget {
		return ggui.Center(
			ggui.Box(
				ggui.Column(
					ggui.Text(label.Get()).Color(fg).Size(28),
					ggui.Row(
						buttonWidget("-", minusHover, func() { ggui.Add(count, -step.Get()) }),
						buttonWidget("+", plusHover, func() { ggui.Add(count, step.Get()) }),
					).Gap(8).Justify(ggui.JustifyCenter),
					ggui.List(hints.Get(), func(s string) ggui.Widget {
						return ggui.Text(s).Color(dim)
					}).Gap(4),
				).Gap(12).Align(ggui.AlignCenter),
			).Pad(24).Fill(panel).Width(320),
		)
	})

	app.OnFrame(func() {
		switch {
		case inpututil.IsKeyJustPressed(ebiten.KeySpace):
			ggui.Add(count, step.Get())
		case inpututil.IsKeyJustPressed(ebiten.KeyArrowUp):
			ggui.Add(step, 1)
		case inpututil.IsKeyJustPressed(ebiten.KeyArrowDown):
			step.Update(func(s int) int { return max(s-1, 1) })
		}
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
