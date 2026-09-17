// Command todo is a small but complete ggui app: a text field with IME
// input, a keyed list that keeps each row's widgets across edits, controls
// bound to signals, a tweened progress bar and a theme switch.
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

func main() {

	todos := ggui.State([]*Todo{})
	draft := ggui.State("")
	show := ggui.State(all)
	dark := ggui.State(false)
	nextID := 1

	add := func(string) {
		title := strings.TrimSpace(draft.Peek())
		if title == "" {
			return
		}
		t := &Todo{ID: nextID, Title: ggui.State(title), Done: ggui.State(false)}
		nextID++
		ggui.Append(todos, t)
		draft.Set("")
	}
	remove := func(id int) { ggui.Remove(todos, func(t *Todo) bool { return t.ID == id }) }
	clearDone := func() { ggui.Remove(todos, func(t *Todo) bool { return t.Done.Peek() }) }

	// Derived views. visible follows the list, the filter and every Done flag.
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
	progress := ggui.Tween(0.0, 300*time.Millisecond)
	ggui.Watch(counts, func(c [2]int) {
		if c[0] == 0 {
			progress.Set(0)
			return
		}
		progress.Set(float64(c[1]) / float64(c[0]))
	})
	ggui.BindTheme(dark, ggui.DarkTheme(), ggui.DefaultTheme())

	row := func(item ggui.Reader[*Todo]) ggui.Widget {
		td := item.Get()
		return ggui.Row(
			ui.Checkbox(td.Done, ""),
			ggui.Expanded(ggui.When(td.Done, ggui.TextOf(td.Title).AsCaption(), ggui.TextOf(td.Title))),
			ui.Button("×", func() { remove(td.ID) }).Secondary().Pad(2, 8),
		).Space(1)
	}

	left := counts.Map(func(c [2]int) int { return c[0] - c[1] })

	// The root Builder reads nothing reactive: theme comes from the Env at
	// layout, and the parts that change are islands.
	app := ggui.New(ggui.Config{Title: "ggui · todo", Width: 520, Height: 600, Resizable: true, Inspector: ebiten.KeyF1}, func() ggui.Widget {
		return ggui.Center(ggui.Box(ui.Card(ggui.Column(
			ggui.Row(
				ggui.Title("todo"),
				ggui.Spacer(),
				ui.Switch(dark, "Dark"),
			),
			ggui.Row(
				ggui.Expanded(ui.TextField(draft).Placeholder("What needs doing?").OnSubmit(add)),
				ui.Button("Add", func() { add("") }),
			).Space(1),
			ui.Progress(progress).Height(4),
			ggui.Expanded(ggui.Scroll(
				ggui.For(visible, func(td *Todo) int { return td.ID }, row).Space(0.5).ItemExtent(28),
			)),
			ui.Divider(),
			ggui.Row(
				ggui.Textf("%d left", left).AsCaption().NoWrap(),
				ggui.Spacer(),
				ui.Radios(show, []filter{all, active, done}).Label(func(f filter) string { return [...]string{"All", "Active", "Done"}[f] }),
				ggui.Tooltip(ui.Button("Clear done", clearDone).Secondary(), "Removes every finished item"),
			).Space(1),
		).Space(1.5)).Pad(24)).Size(480, 540))
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
