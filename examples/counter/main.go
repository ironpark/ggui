// Command counter is a minimal ggui app: one struct of state, sliced into
// reactive fields with Field, and derived text built with generic methods.
// Click or press space to count; up/down changes the step.
package main

import (
	"fmt"
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/ironpark/ggui"
)

type model struct {
	Count int
	Step  int
}

func main() {
	state := ggui.State(model{Step: 1})

	// Field hands out one field of the state as its own read-write cell.
	count := state.Field(func(m *model) *int { return &m.Count })
	step := state.Field(func(m *model) *int { return &m.Step })

	// Map derives text from a cell; it recomputes only when the cell changes.
	label := count.Map(func(n int) string { return fmt.Sprintf("count: %d", n) })
	hints := ggui.Combine(count, step, func(n, s int) []string {
		return []string{
			fmt.Sprintf("click or press space to add %d", s),
			fmt.Sprintf("up/down changes the step (now %d)", s),
			fmt.Sprintf("%d more to reach 100", max(100-n, 0)),
		}
	})

	var (
		fg    = color.White
		dim   = color.RGBA{0x8a, 0x90, 0x9c, 0xff}
		panel = color.RGBA{0x23, 0x27, 0x2f, 0xff}
	)

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
					ggui.Text(label.Get()).Color(fg),
					ggui.List(hints.Get(), func(s string) ggui.Widget {
						return ggui.Text(s).Color(dim)
					}).Gap(4),
				).Gap(8),
			).Pad(24).Fill(panel),
		)
	})

	app.OnFrame(func() {
		switch {
		case inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft),
			inpututil.IsKeyJustPressed(ebiten.KeySpace):
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
