// Command counter is a minimal ggui app: a struct of signals for state,
// derived text built with generic methods, a theme for the look, and the
// built-in Button. Click the buttons or press space to count; up/down
// changes the step; T flips the theme.
package main

import (
	"fmt"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
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
	ggui.SetTheme(ggui.DarkTheme())

	// Map derives text from a cell; it recomputes only when the cell changes.
	label := count.Map(func(n int) string { return fmt.Sprintf("count: %d", n) })
	hints := ggui.Combine(count, step, func(n, s int) []string {
		return []string{
			fmt.Sprintf("click a button or press space to add %d", s),
			fmt.Sprintf("up/down changes the step (now %d), T flips the theme", s),
			fmt.Sprintf("%d more to reach 100", max(100-n, 0)),
		}
	})

	// The root Builder reads only the theme, so it runs once per theme. The
	// parts that change are Reactive islands.
	app := ggui.New(ggui.Config{
		Title:     "ggui · counter",
		Width:     480,
		Height:    320,
		Resizable: true,
		Inspector: ebiten.KeyF1,
	}, func() ggui.Widget {
		t := ggui.UseTheme()
		return ggui.Center(
			ggui.Box(
				ggui.Column(
					ggui.Reactive(func() ggui.Widget {
						return ggui.Text(label.Get()).Style(t.Title)
					}),
					ggui.Row(
						ui.Button("-", func() { ggui.Add(count, -step.Get()) }).Pad(6, 16),
						ui.Button("+", func() { ggui.Add(count, step.Get()) }).Pad(6, 16),
					).Gap(t.Space).Justify(ggui.JustifyCenter),
					ggui.Styled(ggui.Reactive(func() ggui.Widget {
						return ggui.List(hints.Get(), func(s string) ggui.Widget {
							return ggui.Text(s)
						}).Gap(t.Space / 2)
					})).Color(t.Muted),
				).Gap(t.Space * 1.5).Align(ggui.AlignCenter),
			).Pad(t.Space * 3).Fill(t.Surface).Radius(t.Radius * 2).Width(360),
		)
	})

	dark := true
	app.OnFrame(func() {
		switch {
		case inpututil.IsKeyJustPressed(ebiten.KeySpace):
			ggui.Add(count, step.Get())
		case inpututil.IsKeyJustPressed(ebiten.KeyArrowUp):
			ggui.Add(step, 1)
		case inpututil.IsKeyJustPressed(ebiten.KeyArrowDown):
			step.Update(func(s int) int { return max(s-1, 1) })
		case inpututil.IsKeyJustPressed(ebiten.KeyT):
			dark = !dark
			if dark {
				ggui.SetTheme(ggui.DarkTheme())
			} else {
				ggui.SetTheme(ggui.DefaultTheme())
			}
		}
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
