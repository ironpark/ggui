// Command counter is a minimal ggui app: a struct of signals for state,
// derived text built with generic methods, and buttons with hover state of
// their own. Click the buttons or press space to count; up/down changes the
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

// buttonWidget is a Component: setup runs once and owns the hover state, the
// Builder it returns re-runs when hovered changes, and nothing else does.
func buttonWidget(label string, onTap func()) ggui.Widget {
	return ggui.Component(func() ggui.Builder {
		hovered := ggui.State(false)
		return func() ggui.Widget {
			bg := button
			if hovered.Get() {
				bg = hover
			}
			return ggui.Pointer(
				ggui.Box(ggui.Text(label).Color(fg).Size(18)).Pad(6, 16).Fill(bg),
			).OnTap(onTap).OnHover(hovered.Set)
		}
	})
}

func main() {
	state := model{Count: ggui.State(0), Step: ggui.State(1)}
	count, step := state.Count, state.Step

	// Map derives text from a cell; it recomputes only when the cell changes.
	label := count.Map(func(n int) string { return fmt.Sprintf("count: %d", n) })
	hints := ggui.Combine(count, step, func(n, s int) []string {
		return []string{
			fmt.Sprintf("click a button or press space to add %d", s),
			fmt.Sprintf("up/down changes the step (now %d)", s),
			fmt.Sprintf("%d more to reach 100", max(100-n, 0)),
		}
	})

	// The root Builder reads no signals, so it runs once. The parts that
	// change are Reactive islands; the buttons keep their hover state because
	// the root never rebuilds them.
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
					ggui.Reactive(func() ggui.Widget {
						return ggui.Text(label.Get()).Color(fg).Size(28)
					}),
					ggui.Row(
						buttonWidget("-", func() { ggui.Add(count, -step.Get()) }),
						buttonWidget("+", func() { ggui.Add(count, step.Get()) }),
					).Gap(8).Justify(ggui.JustifyCenter),
					ggui.Reactive(func() ggui.Widget {
						return ggui.List(hints.Get(), func(s string) ggui.Widget {
							return ggui.Text(s).Color(dim)
						}).Gap(4)
					}),
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
