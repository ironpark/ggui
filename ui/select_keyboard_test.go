package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// An arrow on the closed field opens the list as Base UI's trigger does: on
// the chosen option, or on the near end when nothing is chosen.
func TestSelectArrowsOpenTheClosedList(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ key, start, want string }{
		{"down", "Beta", "Beta"}, {"up", "Beta", "Beta"}, {"down", "", "Alpha"}, {"up", "", "Gamma"},
	} {
		value := ggui.State(c.start)
		box := ui.Select(value).Options([]string{"Alpha", "Beta", "Gamma"}).Name("Choice").Placeholder("Pick")
		p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(box)), ggui.Sz(300, 300))
		p.Key("tab", c.key)
		if !box.Popup().IsOpen() || value.Get() != c.start {
			t.Fatalf("%s from %q: open=%v value=%q, want the list open and the value kept", c.key, c.start, box.Popup().IsOpen(), value.Get())
		}
		p.Key("enter")
		if value.Get() != c.want {
			t.Fatalf("%s from %q then Enter chose %q, want %q", c.key, c.start, value.Get(), c.want)
		}
		p.Close()
	}
}

// Home and End go to the ends of the open list, past disabled options, and
// are left to whatever is around the field while it is closed.
func TestSelectHomeAndEndMoveThroughTheOpenList(t *testing.T) {
	t.Parallel()
	value := ggui.State("Beta")
	box := ui.Select(value).Options([]string{"Alpha", "Beta", "Gamma", "Delta"}).Name("Choice").
		DisabledOption(func(v string) bool { return v == "Delta" })
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(box)), ggui.Sz(300, 300))
	defer p.Close()
	p.Key("tab", "end")
	if box.Popup().IsOpen() || value.Get() != "Beta" {
		t.Fatalf("End on the closed field: open=%v value=%q", box.Popup().IsOpen(), value.Get())
	}
	p.Key("space", "end", "enter")
	if value.Get() != "Gamma" {
		t.Fatalf("End chose %q, want Gamma, the last enabled option", value.Get())
	}
	p.Key("space", "home", "enter")
	if value.Get() != "Alpha" {
		t.Fatalf("Home chose %q, want Alpha", value.Get())
	}
}

// Typing goes to the option the letters start: chosen while the list is
// closed, highlighted while it is open. One letter again steps to the next
// option under it, and a pause starts the search over.
func TestSelectTypeahead(t *testing.T) {
	t.Parallel()
	value := ggui.State("Apple")
	changes := 0
	box := ui.Select(value).Options([]string{"Apple", "Banana", "Blueberry", "Cherry", "Cranberry"}).Name("Fruit").
		DisabledOption(func(v string) bool { return v == "Cherry" }).OnChange(func(string) { changes++ })
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(box)), ggui.Sz(300, 400))
	defer p.Close()
	p.Key("tab")
	p.Text("b")
	if value.Get() != "Banana" || box.Popup().IsOpen() || changes != 1 {
		t.Fatalf("b on the closed field: value=%q open=%v changes=%d, want Banana chosen", value.Get(), box.Popup().IsOpen(), changes)
	}
	p.Text("l")
	if value.Get() != "Blueberry" {
		t.Fatalf("b then l chose %q, want Blueberry", value.Get())
	}
	p.Advance(time.Second)
	p.Text("b")
	if value.Get() != "Banana" {
		t.Fatalf("b after a pause chose %q, want Banana, the next option under b", value.Get())
	}
	p.Text("b")
	if value.Get() != "Blueberry" {
		t.Fatalf("b again chose %q, want Blueberry", value.Get())
	}
	p.Advance(time.Second)
	p.Text("c")
	if value.Get() != "Cranberry" {
		t.Fatalf("c chose %q, want Cranberry past the disabled Cherry", value.Get())
	}
	p.Advance(time.Second)
	p.Text("z")
	if value.Get() != "Cranberry" {
		t.Fatalf("a letter no option starts changed the value to %q", value.Get())
	}
	// Open, the match is highlighted and Enter chooses it.
	p.Advance(time.Second)
	changes = 0
	p.Key("space")
	p.Text("a")
	if value.Get() != "Cranberry" || changes != 0 || !box.Popup().IsOpen() {
		t.Fatalf("a in the open list: value=%q changes=%d open=%v, want only the highlight moved", value.Get(), changes, box.Popup().IsOpen())
	}
	p.Key("enter")
	if value.Get() != "Apple" {
		t.Fatalf("a then Enter chose %q, want Apple", value.Get())
	}
}
