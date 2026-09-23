package main

import (
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

func workspaceProbe(t *testing.T) (*model, *ggui.Probe) {
	t.Helper()
	now := time.Unix(100, 0)
	restore := ggui.SetClock(func() time.Time { return now })
	var m *model
	var p *ggui.Probe
	dispose := ggui.Root(func() {
		m = newModel()
		p = ggui.ProbeBuilder(func() ggui.Widget { return ggui.Provide(ggui.ReducedMotionKey, true, build(m)) }, ggui.Sz(1040, 800))
	})
	p.Setup(func() {
		bindTheme(m)
		ggui.OnCleanup(dispose)
	})
	shortcuts(p, m)
	t.Cleanup(func() {
		p.Close()
		restore()
	})
	p.Frame()
	now = now.Add(time.Second)
	p.Frame()
	return m, p
}

func TestWorkspaceTaskFlow(t *testing.T) {
	m, p := workspaceProbe(t)
	p.Tap("New task")
	if !ggui.Untrack(m.Editing.Get) {
		t.Fatal("new task did not open the editor")
	}
	if m.save() {
		t.Fatal("empty title was accepted")
	}
	p.Tap("Task title")
	paste(p, "Ship the release")
	p.Tap("Save task")
	tasks := ggui.Untrack(m.Tasks.Get)
	if len(tasks) != 5 || tasks[4].Title != "Ship the release" || ggui.Untrack(m.Editing.Get) {
		t.Fatalf("new task was not saved: %+v", tasks)
	}

	p.Tap("Edit selected")
	p.Tap("Task title")
	p.Type(ggui.Mods{Meta: true}, ggui.KeyA)
	paste(p, "Ship version one")
	p.Tap("Save task")
	if got := ggui.Untrack(m.Tasks.Get)[4].Title; got != "Ship version one" {
		t.Fatalf("edited title = %q", got)
	}

	p.Tap("Search tasks")
	paste(p, "version")
	if got := len(ggui.Untrack(m.Visible.Get)); got != 1 {
		t.Fatalf("search matched %d tasks", got)
	}
	p.Tap("Delete selected")
	p.Tap("Cancel")
	if len(ggui.Untrack(m.Tasks.Get)) != 5 {
		t.Fatal("cancel deleted the task")
	}
	p.Tap("Delete selected")
	p.Tap("Delete task")
	if len(ggui.Untrack(m.Tasks.Get)) != 4 || ggui.Untrack(m.Selected.Get) != 0 {
		t.Fatal("confirmed deletion did not clear the task and selection")
	}
	p.Tap("Clear filters")
	if got := len(ggui.Untrack(m.Visible.Get)); got != 4 {
		t.Fatalf("clear filters left %d visible tasks", got)
	}
}

func TestWorkspaceBindingsAndNavigation(t *testing.T) {
	m, p := workspaceProbe(t)
	p.Tap("Done")
	if got := ggui.Untrack(m.Visible.Get); len(got) != 1 || got[0].Status != "Done" {
		t.Fatalf("status filter = %+v", got)
	}
	p.Tap("Settings")
	p.Tap("Include extended team")
	if got := len(ggui.Untrack(m.Team.Get)); got != 5 {
		t.Fatalf("extended team has %d people", got)
	}
	p.Tap("Available people")
	if _, ok := p.Find("Ken"); !ok {
		t.Fatal("bound options did not reach the people menu")
	}
	p.Type(ggui.Mods{}, ggui.KeyEnd, ggui.KeyEnter)
	p.Frame()
	p.Tap("Dark mode")
	if !ggui.Untrack(m.Dark.Get) {
		t.Fatal("theme switch did not update state")
	}
	m.CardWidth.Set(280)
	p.Frame()
	p.Tap("Insights")
	p.Frame()
	p.Tap("Tasks")
	p.Type(ggui.Mods{Meta: true}, ggui.KeyN)
	if !ggui.Untrack(m.Editing.Get) {
		t.Fatal("new-task shortcut did not open the editor")
	}
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	if ggui.Untrack(m.Editing.Get) {
		t.Fatal("Escape did not dismiss the editor")
	}
	for _, size := range []ggui.Size{ggui.Sz(760, 700), ggui.Sz(1040, 800)} {
		p.Resize(size)
		p.Frame()
		if _, ok := p.Find("New task"); !ok {
			t.Fatalf("new task button missing at %v", size)
		}
	}
}

func TestWorkspaceSnapshotsAndSummary(t *testing.T) {
	m, _ := workspaceProbe(t)
	before := ggui.Untrack(m.Tasks.Get)
	m.Selected.Set(2)
	m.edit()
	m.Draft.Update(func(item task) task {
		item.Status = "Done"
		return item
	})
	if !m.save() {
		t.Fatal("valid edit failed")
	}
	if before[1].Status != "In progress" {
		t.Fatal("save mutated the previous state slice")
	}
	if got := ggui.Untrack(m.Summary.Get); got.Done != 2 || got.Active != 0 {
		t.Fatalf("summary after edit = %+v", got)
	}
	m.askDelete()
	m.Selected.Set(1) // the pending confirmation must still target task 2
	m.delete()
	for _, item := range ggui.Untrack(m.Tasks.Get) {
		if item.ID == 2 {
			t.Fatal("delete followed a later selection")
		}
	}
	m.create()
	m.Draft.Update(func(item task) task {
		item.Title = strings.Repeat("x", 81)
		return item
	})
	if m.save() {
		t.Fatal("overlong title was accepted")
	}
}

// Native text fields receive IME commits; paste exercises that editor path
// without touching the system clipboard.
func paste(p *ggui.Probe, text string) {
	p.Clipboard().Write(text)
	p.Type(ggui.Mods{Meta: true}, ggui.KeyV)
}

// Check geometry as well as existence: a zero-width input can still have an
// accessibility name, and should not pass a responsive-layout regression test.
func TestWorkspaceResponsiveLayout(t *testing.T) {
	m, p := workspaceProbe(t)
	for _, width := range []float64{1040, 760, 480} {
		p.Resize(ggui.Sz(width, 800))
		p.Frame()
		for _, label := range []string{"New task", "Search tasks", "All"} {
			found, ok := p.Find(label)
			if !ok || found.Rect.Empty() ||
				found.Rect.Origin.X < 0 || found.Rect.Origin.Y < 0 ||
				found.Rect.Origin.X+found.Rect.Size.W > width ||
				found.Rect.Origin.Y+found.Rect.Size.H > 800 {
				t.Fatalf("width %.0f: %q is not visible: %+v", width, label, found)
			}
		}
		search, _ := p.Find("Search tasks")
		if search.Rect.Size.W < 200 {
			t.Fatalf("width %.0f: search collapsed to %.0f", width, search.Rect.Size.W)
		}
	}
	p.Tap("Sketch the workspace")
	if ggui.Untrack(m.Selected.Get) != 1 {
		t.Fatal("compact task card did not select its task")
	}
	p.Resize(ggui.Sz(1040, 800))
	p.Frame()
	if ggui.Untrack(m.Selected.Get) != 1 {
		t.Fatal("switching back to the table lost selection")
	}
}
