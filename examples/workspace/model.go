package main

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ironpark/ggui"
)

var statuses = []string{"Planned", "In progress", "Done"}

type task struct {
	ID       int
	Title    string
	Assignee string
	Status   string
}

type summary struct{ Total, Active, Done int }

// The model owns application state; the view only binds to it. Construct it
// under Root so derived values and the progress animation have a lifetime.
type model struct {
	Tasks      *ggui.StateValue[[]task]
	Query      *ggui.StateValue[string]
	Filter     *ggui.StateValue[string]
	Selected   *ggui.StateValue[int]
	Draft      *ggui.StateValue[task]
	Editing    *ggui.StateValue[bool]
	Confirm    *ggui.StateValue[bool]
	Dark       *ggui.StateValue[bool]
	Extended   *ggui.StateValue[bool]
	Tab        *ggui.StateValue[int]
	Activity   *ggui.StateValue[[]string]
	CardWidth  *ggui.StateValue[float64]
	Visible    *ggui.DerivedValue[[]task]
	Team       *ggui.DerivedValue[[]string]
	Summary    *ggui.DerivedValue[summary]
	Validation *ggui.DerivedValue[string]
	Progress   *ggui.Tweened[float64]
	nextID     int
	pendingID  int
}

func newModel() *model {
	m := &model{
		Tasks: ggui.State([]task{
			{1, "Sketch the workspace", "Ada", "Done"},
			{2, "Build keyboard navigation", "Grace", "In progress"},
			{3, "Review accessibility", "Linus", "Planned"},
			{4, "Write the getting-started guide", "Ada", "Planned"},
		}),
		Query: ggui.State(""), Filter: ggui.State("All"),
		Selected: ggui.State(0), Draft: ggui.State(task{}),
		Editing: ggui.State(false), Confirm: ggui.State(false),
		Dark: ggui.State(false), Extended: ggui.State(false),
		Tab: ggui.State(0), CardWidth: ggui.State(320.0),
		Activity: ggui.State([]string{"Workspace ready. Select a task to get started."}),
		nextID:   5,
	}
	m.Team = m.Extended.Map(func(extended bool) []string {
		team := []string{"Ada", "Grace", "Linus"}
		if extended {
			team = append(team, "Barbara", "Ken")
		}
		return team
	})
	m.Visible = ggui.Derived(func() []task {
		query := strings.ToLower(strings.TrimSpace(m.Query.Get()))
		filter := m.Filter.Get()
		var visible []task
		for _, item := range m.Tasks.Get() {
			matches := strings.Contains(strings.ToLower(item.Title+" "+item.Assignee), query)
			if matches && (filter == "All" || item.Status == filter) {
				visible = append(visible, item)
			}
		}
		return visible
	})
	m.Summary = m.Tasks.Map(func(tasks []task) summary {
		result := summary{Total: len(tasks)}
		for _, item := range tasks {
			switch item.Status {
			case "In progress":
				result.Active++
			case "Done":
				result.Done++
			}
		}
		return result
	})
	m.Validation = m.Draft.Map(func(draft task) string {
		if strings.TrimSpace(draft.Title) == "" {
			return "Give the task a title."
		}
		if utf8.RuneCountInString(draft.Title) > 80 {
			return "Keep the title under 81 characters."
		}
		return ""
	})
	completion := m.Summary.Map(func(s summary) float64 {
		return float64(s.Done) / float64(max(s.Total, 1))
	})
	m.Progress = ggui.TweenOf(completion, 250*time.Millisecond)
	return m
}

func (m *model) create() {
	m.Draft.Set(task{Assignee: "Ada", Status: "Planned"})
	m.Editing.Set(true)
}

func (m *model) edit() {
	if item, ok := m.find(ggui.Untrack(m.Selected.Get)); ok {
		m.Draft.Set(item)
		m.Editing.Set(true)
	}
}

// find is the task with the given ID, read outside any tracking scope.
func (m *model) find(id int) (task, bool) {
	return findTask(ggui.Untrack(m.Tasks.Get), id)
}

func findTask(tasks []task, id int) (task, bool) {
	i := slices.IndexFunc(tasks, func(item task) bool { return item.ID == id })
	if i < 0 {
		return task{}, false
	}
	return tasks[i], true
}

func (m *model) save() bool {
	if ggui.Untrack(m.Validation.Get) != "" {
		return false
	}
	item := ggui.Untrack(m.Draft.Get)
	item.Title = strings.TrimSpace(item.Title)
	if item.ID == 0 {
		item.ID = m.nextID
		m.nextID++
		ggui.Append(m.Tasks, item)
	} else {
		tasks := slices.Clone(ggui.Untrack(m.Tasks.Get))
		index := slices.IndexFunc(tasks, func(existing task) bool { return existing.ID == item.ID })
		if index < 0 {
			return false
		}
		tasks[index] = item
		m.Tasks.Set(tasks)
	}
	m.Selected.Set(item.ID)
	m.Editing.Set(false)
	m.record("Saved " + item.Title)
	return true
}

func (m *model) askDelete() {
	id := ggui.Untrack(m.Selected.Get)
	if _, ok := m.find(id); ok {
		m.pendingID = id // confirmation applies to this task, even if selection changes
		m.Confirm.Set(true)
	}
}

func (m *model) delete() {
	ggui.Remove(m.Tasks, func(item task) bool { return item.ID == m.pendingID })
	if ggui.Untrack(m.Selected.Get) == m.pendingID {
		m.Selected.Set(0)
	}
	m.record(fmt.Sprintf("Deleted task #%d", m.pendingID))
	m.pendingID = 0
}

func (m *model) record(message string) {
	m.Activity.Update(func(previous []string) []string {
		entries := append([]string{message}, previous...)
		return entries[:min(len(entries), 8)]
	})
}
