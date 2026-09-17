// Command todo is a small but complete ggui app: a text field with IME
// input under a labelled Field, a keyed list that keeps each row's widgets
// across edits and animates rows in and out, controls bound to signals, a
// tweened progress bar, a confirm Dialog and a theme switch.
package main

import (
	"log"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// Todo is a struct of signals: each row's title and done flag are cells of
// their own, so ticking one box re-runs only what read that box.
type Todo struct {
	ID    int
	Title *ggui.Signal[string]
	Done  *ggui.Signal[bool]
}

type filter int

const (
	all filter = iota
	active
	done
)

var filterNames = [...]string{"All", "Active", "Done"}

const maxTitle = 60

func newTodoApp() *ggui.App {
	todos := ggui.State([]*Todo{})
	draft := ggui.State("")
	show := ggui.State(all)
	dark := ggui.State(false)
	confirm := ggui.State(false)
	nextID := 1

	// Derived views. visible follows the list, the filter and every Done
	// flag; the others are cheap reductions over the list.
	visible := ggui.Derived(func() []*Todo {
		f := show.Get()
		var out []*Todo
		for _, t := range todos.Get() {
			if f == all || (f == active) != t.Done.Get() {
				out = append(out, t)
			}
		}
		return out
	})
	counts := ggui.Derived(func() [2]int {
		var n, d int
		for _, t := range todos.Get() {
			n++
			if t.Done.Get() {
				d++
			}
		}
		return [2]int{n, d}
	})
	left := counts.Map(func(c [2]int) int { return c[0] - c[1] })
	anyDone := counts.Map(func(c [2]int) bool { return c[1] > 0 })
	noneDone := counts.Map(func(c [2]int) bool { return c[1] == 0 })
	draftError := draft.Map(func(s string) string {
		if len([]rune(s)) > maxTitle {
			return "Keep it short: at most 60 characters"
		}
		return ""
	})
	progress := ggui.Tween(0.0, 300*time.Millisecond)

	add := func() {
		title := strings.TrimSpace(draft.Peek())
		if title == "" || draftError.Peek() != "" {
			return
		}
		t := &Todo{ID: nextID, Title: ggui.State(title), Done: ggui.State(false)}
		nextID++
		ggui.Append(todos, t)
		draft.Set("")
	}
	remove := func(id int) { ggui.Remove(todos, func(t *Todo) bool { return t.ID == id }) }
	clearDone := func() {
		ggui.Remove(todos, func(t *Todo) bool { return t.Done.Peek() })
		confirm.Set(false)
	}

	// row builds one keyed row once; the checkbox and text follow the
	// row's own signals afterwards without a rebuild.
	row := func(item ggui.Reader[*Todo]) ggui.Widget {
		td := item.Get()
		return ggui.Row(
			ui.Checkbox(td.Done, ""),
			ggui.Expanded(ggui.When(td.Done, ggui.TextOf(td.Title).AsCaption(), ggui.TextOf(td.Title))),
			ui.Button("×", func() { remove(td.ID) }).Outline().Pad(2, 8),
		).Space(1)
	}

	// The root Builder reads nothing reactive: theme comes from the Env at
	// layout, and the parts that change are islands.
	app := ggui.New(ggui.Config{Title: "ggui · todo", Width: 520, Height: 600, Resizable: true, Inspector: ebiten.KeyF1}, func() ggui.Widget {
		return ggui.Center(ggui.Box(ui.Card(ggui.Column(
			ggui.Row(
				ggui.Title("todo"),
				ggui.Spacer(),
				ui.Switch(dark, "Dark"),
			),
			ui.Field("New item", ggui.Row(
				ggui.Expanded(ui.TextField(draft).Placeholder("What needs doing?").OnSubmit(func(string) { add() })),
				ui.Button("Add", add),
			).Space(1)).Help("Enter adds it").Error(draftError),
			ui.Progress(progress).Height(4),
			ggui.Expanded(ggui.Scroll(
				ggui.For(visible, func(td *Todo) int { return td.ID }, row).Space(0.5).
					Transition(func(w ggui.Widget) *ggui.TransitionWidget {
						return ggui.Transition(w).Fade().Slide(-16, 0)
					}),
			)),
			ui.Divider(),
			ggui.Row(
				ggui.Textf("%d left", left).AsCaption().NoWrap(),
				ggui.Spacer(),
				ui.Radios(show, []filter{all, active, done}).Format(func(f filter) string { return filterNames[f] }),
				ggui.Tooltip(
					ui.Button("Clear done", func() { confirm.Set(true) }).Outline().DisabledWhen(noneDone),
					"Removes every finished item (⌘/Ctrl+K)",
				),
			).Space(1),
			// The dialog takes no space here; it paints over the window
			// while confirm is true, and Escape or the scrim closes it.
			ui.Dialog(confirm, ggui.Column(
				ggui.Textf("Remove %d finished item(s)?", counts.Map(func(c [2]int) int { return c[1] })),
				ggui.Row(
					ui.Button("Remove", clearDone),
					ui.Button("Cancel", func() { confirm.Set(false) }).Outline(),
				).Space(1).Justify(ggui.JustifyEnd),
			).Space(1.5).Align(ggui.AlignStretch)).Title("Clear done"),
		).Space(1.5)).Pad(24)).Size(480, 540))
	})

	app.Shortcut("cmd+k", func() {
		if anyDone.Peek() {
			confirm.Set(true)
		}
	})

	// Watchers run under the app's root owner through Setup, so Close
	// disposes them with everything else.
	app.Setup(func() {
		ggui.Watch(counts, func(c [2]int) {
			if c[0] == 0 {
				progress.Set(0)
				return
			}
			progress.Set(float64(c[1]) / float64(c[0]))
		})
		ggui.BindTheme(dark, ggui.DarkTheme(), ggui.DefaultTheme())
	})
	return app
}

func run() error {
	var app *ggui.App
	dispose := ggui.Root(func() { app = newTodoApp() })
	defer dispose()
	defer app.Close()
	return app.Run()
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
