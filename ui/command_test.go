package ui_test

import (
	"strings"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestGroupedCommandFilteringAndNavigation(t *testing.T) {
	t.Parallel()
	query := ggui.State("")
	picked := ""
	command := ui.Command(query,
		ui.CommandItem("Alpha", func() { picked = "Alpha" }).Group("First"),
		ui.CommandItem("Disabled", nil).Group("First").Disabled(true),
		ui.CommandItem("Beta", func() { picked = "Beta" }).Group("Second"),
	).Height(220).InsetSearch()
	dialog := ui.CommandDialog(ggui.State(true), command)
	p := ggui.NewProbe(dialog, ggui.Sz(500, 400))
	defer p.Close()
	if _, ok := p.Semantics().Find(ggui.RoleDialog, "Commands"); !ok {
		t.Fatal("palette lost accessible name")
	}
	p.Tap("Search commands")
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	if picked != "Beta" {
		t.Fatalf("group headings or disabled row interrupted navigation: %s", picked)
	}
	query.Set("Beta")
	p.Frame()
	if _, ok := p.Find("Alpha"); ok {
		t.Fatal("filtered action remains visible")
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if picked != "Beta" {
		t.Fatal("filtered selection failed")
	}
}

func TestCommandDialogClosesOnChoosingAndTakesNewEntries(t *testing.T) {
	t.Parallel()
	open := ggui.State(true)
	picked := ""
	command := ui.Command(ggui.State(""), ui.CommandItem("Alpha", func() { picked = "Alpha" })).
		Empty("Nothing here").Hints().HintLabels("Move", "Run")
	p := ggui.NewProbe(ui.CommandDialog(open, command), ggui.Sz(500, 400))
	defer p.Close()
	if !strings.Contains(p.Semantics().String(), "Move") {
		t.Fatalf("the footer kept its own words:\n%s", p.Semantics())
	}
	p.Tap("Alpha")
	if picked != "Alpha" || ggui.Peek(open) {
		t.Fatalf("picked %q, open %v", picked, ggui.Peek(open))
	}
	command.Entries(ui.CommandItem("Beta", func() { picked = "Beta" }))
	open.Set(true)
	p.Frame()
	if _, ok := p.Find("Alpha"); ok {
		t.Fatal("the old entries remain")
	}
	p.Tap("Beta")
	if picked != "Beta" {
		t.Fatalf("picked %q", picked)
	}
	command.Entries()
	open.Set(true)
	if !strings.Contains(p.Semantics().String(), "Nothing here") {
		t.Fatal("the empty list kept its own words")
	}
}

func TestCommandRowsSeparatorsAndDialogWidth(t *testing.T) {
	t.Parallel()
	query := ggui.State("")
	command := ui.Command(query,
		ui.CommandItem("Calendar", nil).Group("Suggestions"),
		ui.CommandItem("Calculator", nil).Group("Suggestions"),
		ui.CommandSeparator(),
		ui.CommandItem("Profile", nil).Group("Settings"),
	)
	p := ggui.NewProbe(ui.CommandDialog(ggui.State(true), command), ggui.Sz(800, 600))
	defer p.Close()
	if got := separators(p); got != 1 {
		t.Fatalf("%d separators with no query, want 1", got)
	}
	calendar, _ := p.Find("Calendar")
	calculator, _ := p.Find("Calculator")
	// A row is its line and 12 of padding: 32 with a 20 pixel line, and as
	// little as 28 with the shorter line of a runner's system font.
	if h := calendar.Rect.Size.H; h < 28 || h > 33 {
		t.Fatalf("a row is %v tall, want about 32", h)
	}
	if gap := calculator.Rect.Origin.Y - calendar.Rect.Origin.Y; gap != calendar.Rect.Size.H {
		t.Fatalf("rows of one group are %v apart", gap)
	}
	dialog, _ := p.FindRole(ggui.RoleDialog, "Commands")
	if w := dialog.Rect.Size.W; w != 384 {
		t.Fatalf("the palette is %v wide, want 384", w)
	}
	if y := dialog.Rect.Origin.Y; y != 200 {
		t.Fatalf("the palette's top is at %v, want a third of the 600 high window", y)
	}
	query.Set("o")
	p.Frame()
	if got := separators(p); got != 0 {
		t.Fatalf("%d separators under a query, want none", got)
	}
}

// separators counts the separators p's semantics hold.
func separators(p *ggui.Probe) int {
	n := 0
	for range p.Semantics().Nodes(ggui.RoleSeparator) {
		n++
	}
	return n
}

func TestComboboxListChecksItsValueAndIsAtLeastItsTriggerWide(t *testing.T) {
	t.Parallel()
	value := ggui.State("Beta")
	combo := ui.Combobox(value).Options([]string{"Alpha", "Beta"}).Name("Pick")
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(ggui.Box(combo).Width(260))), ggui.Sz(400, 400))
	defer p.Close()
	p.Tap("Pick")
	trigger, _ := p.Find("Pick")
	tree := p.Semantics()
	if n, ok := tree.Find(ggui.RoleOption, "Beta"); !ok || !n.Selected || !(n.Checked == ggui.Tri(true)) {
		t.Fatalf("the chosen row is not checked:\n%s", tree)
	}
	alpha, _ := p.Find("Alpha")
	if alpha.Rect.Size.W < trigger.Rect.Size.W-16 {
		t.Fatalf("the list's rows are %v wide under a %v trigger", alpha.Rect.Size.W, trigger.Rect.Size.W)
	}
	if gap := alpha.Rect.Origin.Y - trigger.Rect.Origin.Y - trigger.Rect.Size.H; gap < 6 {
		t.Fatalf("the list is %v below its trigger", gap)
	}
}

// cmdk stops at the ends of its list unless it is given loop, and takes
// Home and End to the first and last item.
func TestCommandStopsAtItsEndsUnlessItLoops(t *testing.T) {
	t.Parallel()
	picked := ""
	entries := func() []ui.CommandEntry {
		var out []ui.CommandEntry
		for _, name := range []string{"Alpha", "Beta", "Gamma"} {
			out = append(out, ui.CommandItem(name, func() { picked = name }))
		}
		return out
	}
	command := ui.Command(ggui.State(""), entries()...)
	p := ggui.NewProbe(command, ggui.Sz(400, 400))
	defer p.Close()
	p.Tap("Search commands")
	run := func(want string, keys ...ggui.KeyboardKey) {
		t.Helper()
		p.Type(ggui.Mods{}, append(keys, ggui.KeyEnter)...)
		if picked != want {
			t.Fatalf("picked %q, want %q", picked, want)
		}
	}
	run("Alpha", ggui.KeyArrowUp)
	run("Gamma", ggui.KeyArrowDown, ggui.KeyArrowDown, ggui.KeyArrowDown)
	run("Alpha", ggui.KeyHome)
	run("Gamma", ggui.KeyEnd)
	command.Loop(true)
	run("Alpha", ggui.KeyArrowDown)
	run("Gamma", ggui.KeyArrowUp)
}
