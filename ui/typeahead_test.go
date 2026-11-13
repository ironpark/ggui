package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// Enter and Space open a Select with nothing chosen on its first enabled
// option, as an arrow does, and the open list does not wrap: Base UI's
// Select leaves useListNavigation's loopFocus off.
func TestSelectKeyboardOpenStartsOnTheFirstOptionAndDoesNotWrap(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"enter", "space"} {
		value := ggui.State("")
		box := ui.Select(value).Options([]string{"Alpha", "Beta", "Gamma"}).Name("Choice").Placeholder("Pick").
			DisabledOption(func(v string) bool { return v == "Alpha" })
		p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(box)), ggui.Sz(300, 300))
		p.Key("tab", key, "enter")
		if value.Get() != "Beta" {
			t.Fatalf("%s then Enter chose %q, want Beta, the first enabled option", key, value.Get())
		}
		// Up from the first enabled option and Down from the last stay.
		p.Key("space", "up", "up", "enter")
		if value.Get() != "Beta" {
			t.Fatalf("Up from the first option chose %q, want Beta", value.Get())
		}
		p.Key("space", "end", "down", "enter")
		if value.Get() != "Gamma" {
			t.Fatalf("Down from the last option chose %q, want Gamma", value.Get())
		}
		p.Close()
	}
}

// A Select's search by typing follows Base UI's useTypeahead: a space typed
// during a search is part of it rather than a press of the field, a letter
// that finds nothing ends the search at once, and a doubled letter spells
// a prefix when an option starts with one.
func TestSelectTypeaheadFollowsBaseUI(t *testing.T) {
	t.Parallel()
	space := func(p *ggui.Probe) { p.TypeText(ggui.Mods{}, ggui.KeySpace, " ") }
	value := ggui.State("")
	box := ui.Select(value).Options([]string{"New Jersey", "New York", "Nevada", "Ohio"}).Name("State").Placeholder("Pick")
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(box)), ggui.Sz(300, 400))
	defer p.Close()
	p.Key("tab")
	p.Text("n")
	p.Text("e")
	p.Text("w")
	space(p)
	if box.Popup().IsOpen() {
		t.Fatal("a space in a search opened the list")
	}
	p.Text("y")
	if value.Get() != "New York" {
		t.Fatalf("\"new y\" chose %q, want New York", value.Get())
	}
	// A letter that matches nothing ends the search: the next one starts
	// its own, after the chosen option.
	p.Text("q")
	p.Text("o")
	if value.Get() != "Ohio" {
		t.Fatalf("o straight after a miss chose %q, want Ohio", value.Get())
	}
	// The same in the open list, where a space would otherwise choose.
	p.Advance(time.Second)
	p.Key("enter")
	p.Text("n")
	p.Text("e")
	p.Text("w")
	space(p)
	if !box.Popup().IsOpen() || value.Get() != "Ohio" {
		t.Fatalf("a space in a search of the open list: open=%v value=%q", box.Popup().IsOpen(), value.Get())
	}
	p.Text("y")
	p.Key("enter")
	if value.Get() != "New York" {
		t.Fatalf("\"new y\" then Enter chose %q, want New York", value.Get())
	}
	// Once the search has lapsed, Space is a press again.
	p.Advance(time.Second)
	p.Text("n")
	p.Advance(time.Second)
	space(p)
	if !box.Popup().IsOpen() {
		t.Fatal("Space after a lapsed search did not open the list")
	}
	p.Key("escape")

	// "aa" steps through the options under a, unless one starts with the
	// pair.
	for _, c := range []struct {
		options []string
		want    string
	}{
		{[]string{"Ada", "Alan", "Bob"}, "Alan"},
		{[]string{"Ada", "Alan", "Aaron"}, "Aaron"},
	} {
		v := ggui.State("")
		s := ui.Select(v).Options(c.options).Name("Name")
		q := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(s)), ggui.Sz(300, 300))
		q.Key("tab")
		q.Text("a")
		q.Text("a")
		if v.Get() != c.want {
			t.Fatalf("a, a among %v chose %q, want %q", c.options, v.Get(), c.want)
		}
		q.Close()
	}
}

// In an open Select a search starts after the chosen option rather than
// the highlighted one, as useTypeahead's `selectedIndex ?? activeIndex` has
// it.
func TestSelectTypeaheadStartsAfterTheChosenOption(t *testing.T) {
	t.Parallel()
	value := ggui.State("Banana")
	box := ui.Select(value).Options([]string{"Apple", "Banana", "Blueberry", "Cherry", "Bilberry"}).Name("Fruit")
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(box)), ggui.Sz(300, 400))
	defer p.Close()
	p.Key("tab", "down", "down", "down") // open on Banana, then to Cherry
	p.Text("b")
	p.Key("enter")
	if value.Get() != "Blueberry" {
		t.Fatalf("b chose %q, want Blueberry, the next under b after the chosen Banana", value.Get())
	}
}

// A menu's search by typing lapses after Base UI's TYPEAHEAD_RESET_MS, half
// a second, and a letter that finds nothing ends it at once.
func TestMenuTypeaheadResetsAfterHalfASecond(t *testing.T) {
	t.Parallel()
	ran := ""
	item := func(label string) *ui.MenuItemWidget { return ui.MenuItem(label, func() { ran = label }) }
	menu := ui.Menu("File", item("Save"), item("Save as"), item("Aardvark"), item("Print"))
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(menu)), ggui.Sz(400, 400))
	defer p.Close()
	p.Tap("File")
	p.Text("s")
	p.Advance(400 * time.Millisecond)
	p.Text("a")
	p.Advance(600 * time.Millisecond)
	p.Text("p")
	p.Key("enter")
	if ran != "Print" {
		t.Fatalf("p after 600ms ran %q, want Print: the search had lapsed", ran)
	}
	p.Tap("File")
	p.Text("s")
	p.Text("z")
	p.Text("a")
	p.Key("enter")
	if ran != "Aardvark" {
		t.Fatalf("a straight after a miss ran %q, want Aardvark", ran)
	}
}

// Asking a menu to expand, as an assistive technology does, starts it on
// its first row whichever kind of menu it is: Base UI treats a virtual
// click as it does the keyboard.
func TestMenusExpandedByAnActionStartOnTheFirstRow(t *testing.T) {
	t.Parallel()
	ran := ""
	entries := func() []ggui.Widget {
		return []ggui.Widget{ui.MenuItem("Open", func() { ran = "open" }), ui.MenuItem("Save", func() { ran = "save" })}
	}
	menu := ui.Menu("File", entries()...)
	ctx := ui.ContextMenu(ggui.Box(ggui.Text("Target")).Size(100, 40), entries()...).Name("Target actions")
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(menu, ctx)), ggui.Sz(400, 400))
	defer p.Close()
	for name, expand := range map[string]func() bool{
		"dropdown": func() bool { return menu.Act(ggui.Action{Kind: ggui.ActionExpand}) },
		"context":  func() bool { return ctx.Act(ggui.Action{Kind: ggui.ActionExpand}) },
	} {
		ran = ""
		if !expand() {
			t.Fatalf("the %s menu refused ActionExpand", name)
		}
		p.Frame()
		p.Key("enter")
		if ran != "open" {
			t.Fatalf("Enter in a %s menu an action expanded ran %q, want its first row", name, ran)
		}
		p.Key("escape")
		p.Frame()
	}
}

// Down opens a menubar's menu on its first row and Up does not open it:
// Base UI opens a menu of a horizontal bar only on the arrow that points
// into it.
func TestMenubarOpensOnDownAndNotOnUp(t *testing.T) {
	t.Parallel()
	ran := ""
	bar := ui.Menubar(ui.Menu("Edit", ui.MenuItem("Undo", func() { ran = "undo" }), ui.MenuItem("Redo", func() { ran = "redo" })))
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(bar)), ggui.Sz(400, 300))
	defer p.Close()
	p.Key("tab", "up")
	if bar.Popup().IsOpen() {
		t.Fatal("Up opened the menubar's menu")
	}
	p.Key("down")
	if !bar.Popup().IsOpen() {
		t.Fatal("Down did not open the menubar's menu")
	}
	// Up from the first row wraps to the last: a menu loops.
	p.Key("up", "enter")
	if ran != "redo" {
		t.Fatalf("Down, Up, Enter ran %q, want redo", ran)
	}
}
