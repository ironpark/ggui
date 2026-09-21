// The state of the todo app and the actions and derived views over it.
// Nothing here knows about widgets, so it reads the same in the test.
package main

import (
	"fmt"
	"slices"
	"strings"
	"time"
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

func isDone(t *Todo) bool { return ggui.Untrack(t.Done.Get) }

// filter chooses which rows the list shows.
type filter int

const (
	all filter = iota
	active
	done
)

func (f filter) String() string { return [...]string{"All", "Active", "Done"}[f] }

const maxTitle = 60

// titleError is the validation the field shows and add enforces.
func titleError(s string) string {
	if utf8.RuneCountInString(s) > maxTitle {
		return fmt.Sprintf("Keep it short: at most %d characters", maxTitle)
	}
	return ""
}

// model holds one signal per piece of state and the views derived from
// them. The derived values and the progress tween watch the signals, so
// newModel runs under an owner (ggui.Root or Setup) that disposes them.
type model struct {
	Todos   *ggui.StateValue[[]*Todo]
	Draft   *ggui.StateValue[string]
	Show    *ggui.StateValue[filter]
	Dark    *ggui.StateValue[bool]
	Confirm *ggui.StateValue[bool]

	Visible    *ggui.DerivedValue[[]*Todo] // the rows the filter lets through
	Left       *ggui.DerivedValue[int]     // how many are not done
	NoneDone   *ggui.DerivedValue[bool]
	DraftError *ggui.DerivedValue[string]
	Progress   *ggui.Tweened[float64] // eases toward the done fraction

	nextID int
}

func newModel() *model {
	m := &model{
		Todos:   ggui.State([]*Todo{}),
		Draft:   ggui.State(""),
		Show:    ggui.State(all),
		Dark:    ggui.State(false),
		Confirm: ggui.State(false),
		nextID:  1,
	}
	m.Visible = ggui.Derived(func() []*Todo {
		f, todos := m.Show.Get(), m.Todos.Get()
		out := make([]*Todo, 0, len(todos))
		for _, t := range todos {
			if f == all || (f == active) != t.Done.Get() {
				out = append(out, t)
			}
		}
		return out
	})
	// One pass counts the list for the three readers of its numbers.
	type tally struct{ total, done int }
	counts := ggui.Derived(func() tally {
		var c tally
		for _, t := range m.Todos.Get() {
			c.total++
			if t.Done.Get() {
				c.done++
			}
		}
		return c
	})
	m.Left = counts.Map(func(c tally) int { return c.total - c.done })
	m.NoneDone = counts.Map(func(c tally) bool { return c.done == 0 })
	completion := counts.Map(func(c tally) float64 {
		return float64(c.done) / float64(max(c.total, 1))
	})
	m.Progress = ggui.TweenOf(completion, 300*time.Millisecond)
	m.DraftError = m.Draft.Map(titleError)
	return m
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

func (m *model) clearDone() { ggui.Remove(m.Todos, isDone) }

// askClearDone opens the confirmation when there is something to clear.
func (m *model) askClearDone() {
	if slices.ContainsFunc(ggui.Untrack(m.Todos.Get), isDone) {
		m.Confirm.Set(true)
	}
}
