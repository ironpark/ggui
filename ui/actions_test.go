package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// act performs kind on the first node with role and name.
func act(t *testing.T, p *ggui.Probe, role ggui.Role, name string, a ggui.Action) {
	t.Helper()
	n, ok := p.Semantics().Find(role, name)
	if !ok {
		t.Fatalf("no %s named %q", role, name)
	}
	p.Perform(n.ID, a)
}

func TestActPressesThroughTheKeyPath(t *testing.T) {
	taps := 0
	p := ggui.NewProbe(ui.Button("Save", func() { taps++ }), ggui.Sz(200, 60))
	defer p.Close()
	act(t, p, ggui.RoleButton, "Save", ggui.Action{Kind: ggui.ActionPress})
	if taps != 1 {
		t.Fatalf("taps = %d, want 1", taps)
	}
}

func TestActLeavesDisabledControlsAlone(t *testing.T) {
	taps := 0
	p := ggui.NewProbe(ui.Button("Save", func() { taps++ }).Disabled(true), ggui.Sz(200, 60))
	defer p.Close()
	act(t, p, ggui.RoleButton, "Save", ggui.Action{Kind: ggui.ActionPress})
	if taps != 0 {
		t.Fatalf("a disabled button was pressed %d times", taps)
	}
}

func TestActStepsASlider(t *testing.T) {
	value := ggui.State(5.0)
	committed := 0.0
	p := ggui.NewProbe(ui.Slider(value, 0, 10).Step(1).Name("Vol").OnCommit(func(v float64) { committed = v }), ggui.Sz(200, 40))
	defer p.Close()
	act(t, p, ggui.RoleSlider, "Vol", ggui.Action{Kind: ggui.ActionIncrement})
	if ggui.Untrack(value.Get) != 6 || committed != 6 {
		t.Fatalf("value = %g, committed = %g, want 6", ggui.Untrack(value.Get), committed)
	}
	act(t, p, ggui.RoleSlider, "Vol", ggui.Action{Kind: ggui.ActionDecrement})
	act(t, p, ggui.RoleSlider, "Vol", ggui.Action{Kind: ggui.ActionDecrement})
	if ggui.Untrack(value.Get) != 4 {
		t.Fatalf("value = %g, want 4", ggui.Untrack(value.Get))
	}
	act(t, p, ggui.RoleSlider, "Vol", ggui.Action{Kind: ggui.ActionSetValue, Num: 99})
	if ggui.Untrack(value.Get) != 10 {
		t.Fatalf("value = %g, want the range's top", ggui.Untrack(value.Get))
	}
}

func TestActExpandsAndCollapses(t *testing.T) {
	open := ggui.State(false)
	p := ggui.NewProbe(ui.Collapsible(open, "More", ggui.Text("body")), ggui.Sz(200, 200))
	defer p.Close()
	act(t, p, ggui.RoleDisclosure, "More", ggui.Action{Kind: ggui.ActionExpand})
	if !ggui.Untrack(open.Get) {
		t.Fatal("expand did not open it")
	}
	// Expanding what is already open leaves it open, where a press would
	// have shut it again.
	act(t, p, ggui.RoleDisclosure, "More", ggui.Action{Kind: ggui.ActionExpand})
	if !ggui.Untrack(open.Get) {
		t.Fatal("expanding an open disclosure closed it")
	}
	act(t, p, ggui.RoleDisclosure, "More", ggui.Action{Kind: ggui.ActionCollapse})
	if ggui.Untrack(open.Get) {
		t.Fatal("collapse did not close it")
	}
}

func TestActOpensAndChoosesInASelect(t *testing.T) {
	value := ggui.State(1)
	p := ggui.NewProbe(ggui.Column(ui.Select(value).Options([]int{1, 2, 3}).Name("Count")), ggui.Sz(200, 300))
	defer p.Close()
	act(t, p, ggui.RoleSelect, "Count", ggui.Action{Kind: ggui.ActionExpand})
	if n, _ := p.Semantics().Find(ggui.RoleSelect, "Count"); n.Expanded == nil || !*n.Expanded {
		t.Fatal("the list did not open")
	}
	act(t, p, ggui.RoleOption, "3", ggui.Action{Kind: ggui.ActionSelect})
	if ggui.Untrack(value.Get) != 3 {
		t.Fatalf("value = %d, want 3", ggui.Untrack(value.Get))
	}
}

func TestActSelectsATabAndARow(t *testing.T) {
	sel := ggui.State(0)
	tabs := ui.Tabs(sel, ui.Tab("One", ggui.Text("first")), ui.Tab("Two", ggui.Text("second")))
	p := ggui.NewProbe(tabs, ggui.Sz(300, 200))
	defer p.Close()
	act(t, p, ggui.RoleTab, "Two", ggui.Action{Kind: ggui.ActionSelect})
	if ggui.Untrack(sel.Get) != 1 {
		t.Fatalf("tab = %d, want 1", ggui.Untrack(sel.Get))
	}

	chosen := ggui.State("")
	table := ui.Table(ggui.State([]string{"Ada", "Alan"}), func(s string) string { return s },
		ui.TextCol("Name", func(s string) string { return s })).BindSelected(chosen)
	q := ggui.NewProbe(table, ggui.Sz(300, 200))
	defer q.Close()
	act(t, q, ggui.RoleRow, "Alan", ggui.Action{Kind: ggui.ActionSelect})
	if ggui.Untrack(chosen.Get) != "Alan" {
		t.Fatalf("row = %q, want Alan", ggui.Untrack(chosen.Get))
	}
}

func TestActSetsATextFieldOutright(t *testing.T) {
	value := ggui.State("Ada")
	p := ggui.NewProbe(ui.TextField(value).Name("Name"), ggui.Sz(200, 60))
	defer p.Close()
	act(t, p, ggui.RoleTextField, "Name", ggui.Action{Kind: ggui.ActionSetValue, Text: "Alan"})
	if ggui.Untrack(value.Get) != "Alan" {
		t.Fatalf("value = %q, want Alan", ggui.Untrack(value.Get))
	}
}

func TestActRunsAMenuItem(t *testing.T) {
	ran := 0
	m := ui.Menu("File", ui.MenuItem("Open", func() { ran++ }))
	p := ggui.NewProbe(ggui.Column(m), ggui.Sz(200, 300))
	defer p.Close()
	act(t, p, ggui.RoleMenu, "File", ggui.Action{Kind: ggui.ActionExpand})
	act(t, p, ggui.RoleMenuItem, "Open", ggui.Action{Kind: ggui.ActionPress})
	if ran != 1 {
		t.Fatalf("the item ran %d times, want 1", ran)
	}
}
