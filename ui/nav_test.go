package ui

import (
	"testing"

	"github.com/ironpark/ggui"
)

func press(key ggui.KeyboardKey) ggui.KeyEvent { return ggui.KeyEvent{Kind: ggui.KeyPress, Key: key} }

func TestNavIndexMovesAsWAIARIACompositesDo(t *testing.T) {
	t.Parallel()
	odd := func(i int) bool { return i%2 == 1 } // 1, 3 of 5 enabled
	none := func(int) bool { return false }
	cases := []struct {
		name     string
		ev       ggui.KeyEvent
		axis     navAxis
		wrap     bool
		cur, n   int
		enabled  func(int) bool
		want     int
		consumed bool
	}{
		{"down steps", press(ggui.KeyArrowDown), navVertical, true, 1, 5, nil, 2, true},
		{"down wraps", press(ggui.KeyArrowDown), navVertical, true, 4, 5, nil, 0, true},
		{"up wraps", press(ggui.KeyArrowUp), navVertical, true, 0, 5, nil, 4, true},
		{"down stops at the end", press(ggui.KeyArrowDown), navVertical, false, 4, 5, nil, 4, true},
		{"up stops at the start", press(ggui.KeyArrowUp), navVertical, false, 0, 5, nil, 0, true},
		{"down from nothing starts at the top", press(ggui.KeyArrowDown), navVertical, false, -1, 5, nil, 0, true},
		{"up from nothing starts at the bottom", press(ggui.KeyArrowUp), navVertical, true, -1, 5, nil, 4, true},
		{"down skips disabled", press(ggui.KeyArrowDown), navVertical, true, 1, 5, odd, 3, true},
		{"down wraps past disabled", press(ggui.KeyArrowDown), navVertical, true, 3, 5, odd, 1, true},
		{"down stops past disabled", press(ggui.KeyArrowDown), navVertical, false, 3, 5, odd, 3, true},
		{"home is the first enabled", press(ggui.KeyHome), navVertical, false, 3, 5, odd, 1, true},
		{"end is the last enabled", press(ggui.KeyEnd), navVertical, false, 1, 5, odd, 3, true},
		{"a step with nothing enabled stays", press(ggui.KeyArrowDown), navVertical, true, 2, 5, none, 2, true},
		{"home with nothing enabled is nothing", press(ggui.KeyHome), navVertical, true, 2, 5, none, -1, true},
		{"right is not vertical", press(ggui.KeyArrowRight), navVertical, true, 1, 5, nil, 1, false},
		{"right is horizontal", press(ggui.KeyArrowRight), navHorizontal, true, 1, 5, nil, 2, true},
		{"left is horizontal", press(ggui.KeyArrowLeft), navBoth, true, 0, 5, nil, 4, true},
		{"down is not horizontal", press(ggui.KeyArrowDown), navHorizontal, true, 1, 5, nil, 1, false},
		{"a release does nothing", ggui.KeyEvent{Kind: ggui.KeyRelease, Key: ggui.KeyArrowDown}, navVertical, true, 1, 5, nil, 1, false},
		{"another key does nothing", press(ggui.KeyA), navBoth, true, 1, 5, nil, 1, false},
	}
	for _, c := range cases {
		got, ok := navIndex(c.ev, c.axis, c.wrap, c.cur, c.n, c.enabled)
		if got != c.want || ok != c.consumed {
			t.Errorf("%s: got %d, %v; want %d, %v", c.name, got, ok, c.want, c.consumed)
		}
		if navConsumes(c.ev, c.axis) != c.consumed {
			t.Errorf("%s: navConsumes disagrees with navIndex", c.name)
		}
	}
}

func TestNavTargetLeavesClampingToTheList(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		key  ggui.KeyboardKey
		want int
		ok   bool
	}{{ggui.KeyArrowUp, -1, true}, {ggui.KeyArrowDown, 1, true}, {ggui.KeyHome, 0, true}, {ggui.KeyEnd, 9, true}, {ggui.KeyArrowLeft, 0, false}} {
		if got, ok := navTarget(c.key, navVertical, 0, 10); got != c.want || ok != c.ok {
			t.Errorf("%v: got %d, %v; want %d, %v", c.key, got, ok, c.want, c.ok)
		}
	}
}

// The single-select composites agree: both arrow axes, wrapping, and Home
// and End at the ends.
func TestTabsAndToggleGroupNavigateAlike(t *testing.T) {
	t.Parallel()
	tab := ggui.State(0)
	tabs := Tabs(tab, Tab("A", nil), Tab("B", nil), Tab("C", nil))
	choice := ggui.State("a")
	group := ToggleGroup(choice).Options([]string{"a", "b", "c"})
	read := []func() int{func() int { return ggui.Peek(tab) }, func() int { return map[string]int{"a": 0, "b": 1, "c": 2}[ggui.Peek(choice)] }}
	for w, h := range []ggui.KeyHandler{tabs, group} {
		for _, step := range []struct {
			key  ggui.KeyboardKey
			want int
		}{{ggui.KeyArrowLeft, 2}, {ggui.KeyArrowRight, 0}, {ggui.KeyArrowUp, 2}, {ggui.KeyArrowDown, 0}, {ggui.KeyEnd, 2}, {ggui.KeyHome, 0}} {
			h.HandleKey(press(step.key))
			if got := read[w](); got != step.want {
				t.Fatalf("widget %d, %v: at %d, want %d", w, step.key, got, step.want)
			}
		}
	}
}

// Accordion, Sidebar and a context menu skip disabled items the same way.
func TestListsSkipDisabledItemsAlike(t *testing.T) {
	t.Parallel()
	a := Accordion(ggui.State([]string(nil)), AccordionItem("a", "A", nil).Disabled(true), AccordionItem("b", "B", nil), AccordionItem("c", "C", nil).Disabled(true), AccordionItem("d", "D", nil))
	s := Sidebar(ggui.State(""), SidebarItem("a", "A").Disabled(true), SidebarItem("b", "B"), SidebarItem("c", "C").Disabled(true), SidebarItem("d", "D"))
	m := Menu("Menu", MenuItem("A", nil).Disabled(true), MenuItem("B", nil), MenuItem("C", nil).Disabled(true), MenuItem("D", nil))
	active := []func() int{func() int { return a.active }, func() int { return s.active }, func() int { return m.current }}
	keys := []func(ggui.KeyEvent){a.HandleKey, s.HandleKey, func(ev ggui.KeyEvent) { m.navigate(ev) }}
	for w := range keys {
		for _, step := range []struct {
			key  ggui.KeyboardKey
			want int
		}{{ggui.KeyHome, 1}, {ggui.KeyArrowDown, 3}, {ggui.KeyArrowDown, 1}, {ggui.KeyArrowUp, 3}, {ggui.KeyEnd, 3}} {
			keys[w](press(step.key))
			if got := active[w](); got != step.want {
				t.Fatalf("widget %d, %v: at %d, want %d", w, step.key, got, step.want)
			}
		}
	}
}
