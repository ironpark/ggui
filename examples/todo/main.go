// Command todo is a small but complete ggui app: a text field with IME
// input under a labelled Field, a keyed list that keeps each row's widgets
// across edits and animates rows in and out, inline editing of a row's
// title, controls bound to signals, a tweened progress bar, an AlertDialog
// and a theme switch.
//
// The UI lives in build so main_test.go can drive it headlessly with a
// Probe.
package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// Todo is a struct of signals: each row's title and done flag are cells of
// their own, so ticking one box or editing one title re-runs only what
// read that cell.
type Todo struct {
	ID    int
	Title *ggui.StateValue[string]
	Done  *ggui.StateValue[bool]
}

type filter int

const (
	all filter = iota
	active
	done
)

var filterNames = [...]string{"All", "Active", "Done"}

const maxTitle = 60

// titleError is the validation the field shows and add enforces.
func titleError(s string) string {
	if len([]rune(s)) > maxTitle {
		return fmt.Sprintf("Keep it short: at most %d characters", maxTitle)
	}
	return ""
}

// model holds one signal per piece of state. Derived values are created in
// build, so they belong to the app's or the probe's root owner.
type model struct {
	Todos   *ggui.StateValue[[]*Todo]
	Draft   *ggui.StateValue[string]
	Show    *ggui.StateValue[filter]
	Dark    *ggui.StateValue[bool]
	Confirm *ggui.StateValue[bool]
	nextID  int
}

func newModel() *model {
	return &model{
		Todos: ggui.State([]*Todo{}), Draft: ggui.State(""), Show: ggui.State(all),
		Dark: ggui.State(false), Confirm: ggui.State(false), nextID: 1,
	}
}

func (m *model) add() {
	title := strings.TrimSpace(ggui.Untrack(m.Draft.Get))
	if title == "" || titleError(title) != "" {
		return
	}
	ggui.Append(m.Todos, &Todo{ID: m.nextID, Title: ggui.State(title), Done: ggui.State(false)})
	m.nextID++
	m.Draft.Set("")
}

func (m *model) remove(id int) { ggui.Remove(m.Todos, func(t *Todo) bool { return t.ID == id }) }

func (m *model) clearDone() {
	ggui.Remove(m.Todos, func(t *Todo) bool { return ggui.Untrack(t.Done.Get) })
}

// askClearDone opens the confirmation when there is something to clear.
func (m *model) askClearDone() {
	for _, t := range ggui.Untrack(m.Todos.Get) {
		if ggui.Untrack(t.Done.Get) {
			m.Confirm.Set(true)
			return
		}
	}
}

// row builds one keyed row once; the checkbox and text follow the row's
// own signals afterwards without a rebuild. Tapping the title edits it in
// place; the row's editing signal and the Watch that keeps the accessible
// names current live as long as the row does.
func (m *model) row(item ggui.Readable[*Todo]) ggui.Widget {
	td := item.Get()
	editing := ggui.State(false)
	check := ui.Checkbox(td.Done, "")
	remove := ui.Button("×", func() { m.remove(td.ID) }).Outline().Pad(2, 8)
	ggui.Watch(td.Title, func(title string) {
		check.SetName("Done: " + title)
		remove.SetName("Remove " + title)
	})
	return ggui.Row(
		check,
		ggui.Expanded(ggui.If(editing, func() ggui.Widget {
			// The branch runs fresh each time editing turns on, so the
			// field's name follows the title as it was then.
			return ui.TextField(td.Title).Named("Edit: " + ggui.Untrack(td.Title.Get)).OnSubmit(func(string) { editing.Set(false) })
		}).Else(func() ggui.Widget {
			return ggui.Tap(ggui.If(td.Done, func() ggui.Widget {
				return ggui.TextOf(td.Title).AsCaption()
			}).Else(func() ggui.Widget {
				return ggui.TextOf(td.Title)
			}), func() { editing.Set(true) })
		})),
		remove,
	).Space(1)
}

// build is the root Builder. It reads nothing reactive: the theme comes
// from the Env at layout, and the parts that change are islands.
func (m *model) build() ggui.Widget {
	// Derived views. visible follows the list, the filter and every Done
	// flag; the others are cheap reductions over the list.
	visible := ggui.Derived(func() []*Todo {
		f := m.Show.Get()
		var out []*Todo
		for _, t := range m.Todos.Get() {
			if f == all || (f == active) != t.Done.Get() {
				out = append(out, t)
			}
		}
		return out
	})
	counts := ggui.Derived(func() [2]int {
		var n, d int
		for _, t := range m.Todos.Get() {
			n++
			if t.Done.Get() {
				d++
			}
		}
		return [2]int{n, d}
	})
	left := counts.Map(func(c [2]int) int { return c[0] - c[1] })
	noneDone := counts.Map(func(c [2]int) bool { return c[1] == 0 })
	draftError := m.Draft.Map(titleError)
	progress := ggui.Tween(0.0, 300*time.Millisecond)
	ggui.Watch(counts, func(c [2]int) {
		if c[0] == 0 {
			progress.Set(0)
			return
		}
		progress.Set(float64(c[1]) / float64(c[0]))
	})

	return ggui.Center(ggui.Box(ui.Card(ggui.Column(
		ggui.Row(
			ggui.Title("todo"),
			ggui.Spacer(),
			ui.Switch(m.Dark, "Dark"),
		),
		ui.Field("New item", ggui.Row(
			ggui.Expanded(ui.TextField(m.Draft).Placeholder("What needs doing?").OnSubmit(func(string) { m.add() })),
			ui.Button("Add", m.add),
		).Space(1)).Help("Enter adds it").Error(draftError),
		ui.Progress(progress).Height(4),
		ggui.Expanded(ggui.Scroll(
			ggui.EachKeyed(visible, func(td *Todo) int { return td.ID }, func(item ggui.EachItem[*Todo]) ggui.Widget { return m.row(item.Value) }).Space(0.5).
				Transition(func(w ggui.Widget) *ggui.TransitionWidget {
					return ggui.Transition(w).Fade().Slide(-16, 0)
				}).
				Else(func() ggui.Widget {
					return ui.Empty("Nothing here", "Add an item above, or pick another filter.")
				}),
		)),
		ui.Divider(),
		ggui.Row(
			ggui.Textf("%d left", left).AsCaption().NoWrap(),
			ggui.Spacer(),
			ui.Radios(m.Show, []filter{all, active, done}).Format(func(f filter) string { return filterNames[f] }),
			ggui.Tooltip(
				ui.Button("Clear done", m.askClearDone).Outline().DisabledWhen(noneDone),
				"Removes every finished item (⌘/Ctrl+K)",
			),
		).Space(1),
		// The dialog takes no space here; it paints over the window while
		// Confirm is true, closes itself, then runs the confirmation.
		ui.AlertDialog(m.Confirm, "Clear done?", "Every finished item is removed.").
			Confirm("Remove", m.clearDone).Destructive(),
	).Space(1.5)).Pad(24)).Size(480, 540))
}

// shortcuts binds the keys. Both App and Probe offer Shortcut, so the test
// installs the same bindings.
func (m *model) shortcuts(s ggui.Host) {
	s.Shortcut("cmd+k", m.askClearDone)
}

func main() {
	m := newModel()
	app := ggui.New(ggui.Config{Title: "ggui · todo", Width: 520, Height: 600, Resizable: true, Inspector: "f1"}, m.build)
	m.shortcuts(app)

	// Setup runs under the app's root owner, so the theme binding is
	// disposed with the app.
	app.Setup(func() { ggui.BindTheme(m.Dark, ggui.DarkTheme(), ggui.DefaultTheme()) })
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
