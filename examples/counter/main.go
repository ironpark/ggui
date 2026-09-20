// Command counter is a minimal ggui app: a struct of signals for state,
// reactive text through Textf and View, a theme for the look, and the
// built-in Button. Click the buttons or press space to count; up/down
// changes the step; T flips the theme, through App.Shortcut.
//
// The UI lives in build so main_test.go can drive it headlessly with a
// Probe.
package main

import (
	"fmt"
	"log"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// model holds one signal per piece of state. Each field is its own reactive
// cell, so a change to Count re-runs only what read Count.
type model struct {
	Count *ggui.StateValue[int]
	Step  *ggui.StateValue[int]
	Dark  *ggui.StateValue[bool]
}

func newModel() model {
	return model{Count: ggui.State(0), Step: ggui.State(1), Dark: ggui.State(true)}
}

func (m model) add()  { ggui.Add(m.Count, ggui.Untrack(m.Step.Get)) }
func (m model) sub()  { ggui.Add(m.Count, -ggui.Untrack(m.Step.Get)) }
func (m model) up()   { ggui.Add(m.Step, 1) }
func (m model) down() { m.Step.Update(func(s int) int { return max(s-1, 1) }) }

// build is the root Builder. It reads nothing reactive, so it runs once;
// Textf and View are the islands that follow the signals.
func (m model) build() ggui.Widget {
	hints := ggui.Combine(m.Count, m.Step, func(n, s int) []string {
		return []string{
			fmt.Sprintf("click a button or press space to add %d", s),
			fmt.Sprintf("up/down changes the step (now %d), T flips the theme", s),
			fmt.Sprintf("%d more to reach 100", max(100-n, 0)),
		}
	})
	return ggui.Center(
		ggui.Box(ui.Card(
			ggui.Column(
				ggui.Textf("count: %d", m.Count).AsTitle(),
				ggui.Row(
					ui.Button("-", m.sub).Pad(6, 16),
					ui.Button("+", m.add).Pad(6, 16),
				).Space(1).Justify(ggui.JustifyCenter),
				ggui.View(hints, func(h []string) *ggui.ColumnWidget {
					return ggui.List(h, func(s string) ggui.Widget { return ggui.Caption(s) }).Space(0.5)
				}),
			).Space(1.5).Align(ggui.AlignCenter),
		).Pad(24)).Width(360),
	)
}

// shortcuts binds the keys. Bare-key shortcuts reach the focused widget
// first: Space on a focused button presses the button, not this. Both App
// and Probe offer Shortcut, so the test installs the same bindings.
func (m model) shortcuts(s interface {
	Shortcut(chord string, fn func()) *ggui.ShortcutHandle
}) {
	s.Shortcut("space", m.add)
	s.Shortcut("up", m.up)
	s.Shortcut("down", m.down)
	s.Shortcut("t", func() { ggui.Toggle(m.Dark) })
}

func main() {
	m := newModel()
	app := ggui.New(ggui.Config{
		Title:     "ggui · counter",
		Width:     480,
		Height:    320,
		Resizable: true,
		Inspector: "f1",
	}, m.build)
	m.shortcuts(app)

	// Setup runs under the app's root owner, so the theme binding is
	// disposed with the app.
	app.Setup(func() { ggui.BindTheme(m.Dark, ggui.DarkTheme(), ggui.DefaultTheme()) })
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
