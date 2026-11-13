package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// A Radios is one stop in the Tab order, on its chosen option, and the
// arrows move the choice past disabled options and around the ends, as
// Base UI's RadioGroup does.
func TestRadiosAreOneTabStopMovedByArrows(t *testing.T) {
	t.Parallel()
	plan := ggui.State("pro")
	changes := 0
	root := ggui.Column(
		ui.Button("Before", nil),
		ui.Radios(plan).Options([]string{"free", "pro", "team"}).Name("Plan").OnChange(func(string) { changes++ }),
		ui.Button("After", nil),
	)
	p := ggui.NewProbe(root, ggui.Sz(300, 300))
	defer p.Close()
	p.Key("tab", "tab")
	if got := focusedName(t, p); got != "pro" {
		t.Fatalf("Tab into the group focused %q, want the chosen option pro", got)
	}
	p.Key("down")
	p.Frame()
	if plan.Get() != "team" || focusedName(t, p) != "team" || changes != 1 {
		t.Fatalf("Down: value=%q focus=%q changes=%d, want team chosen and focused", plan.Get(), focusedName(t, p), changes)
	}
	p.Key("right")
	p.Frame()
	if plan.Get() != "free" || focusedName(t, p) != "free" {
		t.Fatalf("Right from the last option: value=%q focus=%q, want it around to free", plan.Get(), focusedName(t, p))
	}
	p.Key("up")
	p.Frame()
	if plan.Get() != "team" || focusedName(t, p) != "team" {
		t.Fatalf("Up from the first option: value=%q focus=%q, want team", plan.Get(), focusedName(t, p))
	}
	p.Key("left")
	p.Frame()
	if plan.Get() != "pro" {
		t.Fatalf("Left chose %q, want pro", plan.Get())
	}
	p.Key("tab")
	if got := focusedName(t, p); got != "After" {
		t.Fatalf("Tab from the group focused %q, want the button after it", got)
	}
	p.Key("shift+tab")
	if got := focusedName(t, p); got != "pro" {
		t.Fatalf("Shift+Tab back into the group focused %q, want pro", got)
	}
}

// With nothing chosen the first enabled option is the group's stop, and a
// Radio that is not in a Radios keeps its own.
func TestRadiosTabStopWithoutAChoice(t *testing.T) {
	t.Parallel()
	plan := ggui.State("")
	lone := ggui.State("a")
	root := ggui.Column(
		ui.Radios(plan).Options([]string{"free", "pro"}).Name("Plan"),
		ui.Radio(lone, "a", "A"),
		ui.Radio(lone, "b", "B"),
	)
	p := ggui.NewProbe(root, ggui.Sz(300, 300))
	defer p.Close()
	p.Key("tab")
	if got := focusedName(t, p); got != "free" {
		t.Fatalf("Tab focused %q, want free, the first option", got)
	}
	if plan.Get() != "" {
		t.Fatalf("focusing the group chose %q", plan.Get())
	}
	p.Key("tab", "tab")
	if got := focusedName(t, p); got != "B" {
		t.Fatalf("two Tabs on focused %q, want B: single radios are each a stop", got)
	}
	p.Key("down")
	if lone.Get() != "a" {
		t.Fatalf("an arrow on a single Radio chose %q", lone.Get())
	}
}
