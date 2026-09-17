// Command todo is a small but complete ggui app: a text field with IME
// input, a keyed list that keeps each row's widgets across edits, controls
// bound to signals, a tweened progress bar and a theme switch.
package main

import (
	"fmt"
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

	row := func(item *ggui.Signal[*Todo]) ggui.Widget {
		td := item.Peek()
		return ggui.Row(
			ui.Checkbox(td.Done, ""),
			ggui.Expanded(ggui.Reactive(func() ggui.Widget {
				t := ggui.UseTheme()
				text := ggui.Text(td.Title.Get())
				if td.Done.Get() {
					text.Color(t.Muted)
				}
				return text
			})),
			ui.Button("×", func() { remove(td.ID) }).Secondary().Pad(2, 8),
		).Gap(8)
	}

	app := ggui.New(ggui.Config{Title: "ggui · todo", Width: 520, Height: 600, Resizable: true, Inspector: ebiten.KeyF1}, func() ggui.Widget {
		t := ggui.UseTheme()
		return ggui.Center(ggui.Box(ggui.Column(
			ggui.Row(
				ggui.Text("todo").Style(t.Title),
				ggui.Spacer(),
				ui.Switch(dark, "Dark"),
			),
			ggui.Row(
				ggui.Expanded(ui.TextField(draft).Placeholder("What needs doing?").OnSubmit(add)),
				ui.Button("Add", func() { add("") }),
			).Gap(t.Space),
			ui.Progress(progress).Height(4),
			ggui.Expanded(ggui.Scroll(
				ggui.For(visible, func(td *Todo) int { return td.ID }, row).Gap(t.Space/2).ItemExtent(28),
			)),
			ui.Divider(),
			ggui.Row(
				ggui.Styled(ggui.Reactive(func() ggui.Widget {
					c := counts.Get()
					return ggui.Text(fmt.Sprintf("%d left", c[0]-c[1])).NoWrap()
				})).Color(t.Muted),
				ggui.Spacer(),
				ui.Radios(show, []filter{all, active, done}, func(f filter) string { return [...]string{"All", "Active", "Done"}[f] }),
				ggui.Tooltip(ui.Button("Clear done", clearDone).Secondary(), "Removes every finished item"),
			).Gap(t.Space),
		).Gap(t.Space*1.5)).Pad(t.Space*3).Fill(t.Surface).Radius(t.Radius*2).Size(480, 540))
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
