// The state of the todo app and the actions and derived views over it.
// Nothing here knows about widgets, so it reads the same in the test.
package main

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ironpark/ggui"
)

// Todo is a struct of signals: each row's title and done flag are cells of
// their own, so ticking one box or editing one title re-runs only what
// read that cell.
type Todo struct {
	ID    int
	Title *ggui.StateValue[string]
	Done  *ggui.StateValue[bool]
}

// filter chooses which rows the list shows.
type filter int

const (
	all filter = iota
	active
	done
)

func (f filter) String() string { return [...]string{"All", "Active", "Done"}[f] }

// tally counts the list once for everything that needs a number from it.
type tally struct{ total, done int }

func (t tally) left() int         { return t.total - t.done }
func (t tally) noneDone() bool    { return t.done == 0 }
func (t tally) fraction() float64 { return float64(t.done) / float64(max(t.total, 1)) }

const maxTitle = 60

// titleError is the validation the field shows and add enforces.
func titleError(s string) string {
	if utf8.RuneCountInString(s) > maxTitle {
		return fmt.Sprintf("Keep it short: at most %d characters", maxTitle)
	}
	return ""
}

// model holds one signal per piece of state. The derived views below are
// called from build, so they belong to the app's or the probe's root owner.
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
		Todos:   ggui.State([]*Todo{}),
		Draft:   ggui.State(""),
		Show:    ggui.State(all),
		Dark:    ggui.State(false),
		Confirm: ggui.State(false),
		nextID:  1,
	}
}

// Actions. Each reads with Untrack: they run from handlers, not effects.

func (m *model) add() {
	title := strings.TrimSpace(ggui.Untrack(m.Draft.Get))
	if title == "" || titleError(title) != "" {
		return
	}
	ggui.Append(m.Todos, &Todo{ID: m.nextID, Title: ggui.State(title), Done: ggui.State(false)})
	m.nextID++
	m.Draft.Set("")
}

func (m *model) remove(id int) {
	ggui.Remove(m.Todos, func(t *Todo) bool { return t.ID == id })
}

func isDone(t *Todo) bool { return ggui.Untrack(t.Done.Get) }

func (m *model) clearDone() { ggui.Remove(m.Todos, isDone) }

// askClearDone opens the confirmation when there is something to clear.
func (m *model) askClearDone() {
	if slices.ContainsFunc(ggui.Untrack(m.Todos.Get), isDone) {
		m.Confirm.Set(true)
	}
}

// Derived views.

// visible follows the list, the filter and every Done flag.
func (m *model) visible() *ggui.DerivedValue[[]*Todo] {
	return ggui.Derived(func() []*Todo {
		f, todos := m.Show.Get(), m.Todos.Get()
		out := make([]*Todo, 0, len(todos))
		for _, t := range todos {
			if f == all || (f == active) != t.Done.Get() {
				out = append(out, t)
			}
		}
		return out
	})
}

func (m *model) tally() *ggui.DerivedValue[tally] {
	return ggui.Derived(func() tally {
		var c tally
		for _, t := range m.Todos.Get() {
			c.total++
			if t.Done.Get() {
				c.done++
			}
		}
		return c
	})
}
