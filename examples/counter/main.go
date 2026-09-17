// Command counter is a minimal ggui app: a struct of signals for state,
// reactive text through Textf and View, a theme for the look, and the
// built-in Button. Click the buttons or press space to count; up/down
// changes the step; T flips the theme, through App.OnKey.
package main

import (
	"fmt"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// model holds one signal per piece of state. Each field is its own reactive
// cell, so a change to Count re-runs only what read Count.
type model struct {
	Count *ggui.Signal[int]
	Step  *ggui.Signal[int]
}

func main() {
	state := model{Count: ggui.State(0), Step: ggui.State(1)}
	count, step := state.Count, state.Step
	dark := ggui.State(true)
	ggui.BindTheme(dark, ggui.DarkTheme(), ggui.DefaultTheme())

	hints := ggui.Combine(count, step, func(n, s int) []string {
		return []string{
			fmt.Sprintf("click a button or press space to add %d", s),
			fmt.Sprintf("up/down changes the step (now %d), T flips the theme", s),
			fmt.Sprintf("%d more to reach 100", max(100-n, 0)),
		}
	})

	// The root Builder reads nothing reactive, so it runs once. Textf and
	// View are the islands that follow the signals.
	app := ggui.New(ggui.Config{
		Title:     "ggui · counter",
		Width:     480,
		Height:    320,
		Resizable: true,
		Inspector: ebiten.KeyF1,
	}, func() ggui.Widget {
		return ggui.Center(
			ggui.Box(ui.Card(
				ggui.Column(
					ggui.Textf("count: %d", count).AsTitle(),
					ggui.Row(
						ui.Button("-", func() { ggui.Add(count, -step.Get()) }).Pad(6, 16),
						ui.Button("+", func() { ggui.Add(count, step.Get()) }).Pad(6, 16),
					).Space(1).Justify(ggui.JustifyCenter),
					ggui.View(hints, func(h []string) *ggui.ColumnWidget {
						return ggui.List(h, func(s string) ggui.Widget { return ggui.Caption(s) }).Space(0.5)
					}),
				).Space(1.5).Align(ggui.AlignCenter),
			).Pad(24)).Width(360),
		)
	})

	app.OnKey(func(ev ggui.KeyEvent) bool {
		switch ev.Key {
		case ebiten.KeySpace:
			ggui.Add(count, step.Get())
		case ebiten.KeyArrowUp:
			ggui.Add(step, 1)
		case ebiten.KeyArrowDown:
			step.Update(func(s int) int { return max(s-1, 1) })
		case ebiten.KeyT:
			ggui.Toggle(dark)
		default:
			return false
		}
		return true
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
