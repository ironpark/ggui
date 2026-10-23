package ggui

import (
	"slices"
	"testing"
	"time"
)

// reorderFixture is a Reorderable of 30-high rows whose painted rects land
// in painted, keyed by item.
func reorderFixture(items *StateValue[[]string], painted map[string]Rect, handle bool) *ReorderableWidget[string, string] {
	return Reorderable(items, func(s string) string { return s }, func(it EachItem[string]) Widget {
		v := it.Item
		row := FromFuncs(
			func(c Constraints, _ Env) Size { return c.Constrain(Sz(80, 30)) },
			func(_ *Canvas, r Rect) { painted[v] = r },
		)
		if !handle {
			return row
		}
		return Row(ReorderHandle(Box().Size(20, 30)).Name("Move "+v), row)
	})
}

func TestReorderableDragMovesARowAndOpensAGap(t *testing.T) {
	t.Parallel()
	items := State([]string{"a", "b", "c", "d"})
	painted := map[string]Rect{}
	var moves [][2]int
	l := reorderFixture(items, painted, false).OnReorder(func(from, to int) { moves = append(moves, [2]int{from, to}) })
	p := NewProbe(l, Sz(100, 200))
	defer p.Close()
	p.Frame()

	p.Press(Pt(40, 15))
	p.Move(Pt(40, 25))
	p.Move(Pt(40, 90)) // the row's middle is now below c's
	p.Frame()
	p.Advance(time.Second)
	if !l.Dragging() {
		t.Fatal("dragging a row did not start a drag")
	}
	if painted["b"].Origin.Y != 0 || painted["c"].Origin.Y != 30 || painted["d"].Origin.Y != 90 {
		t.Fatalf("mid-drag b %v c %v d %v; want b and c moved up into the gap", painted["b"], painted["c"], painted["d"])
	}
	p.Release(Pt(40, 90))
	if got := Peek(items); !slices.Equal(got, []string{"b", "c", "a", "d"}) || !slices.Equal(moves, [][2]int{{0, 3}}) {
		t.Fatalf("after the drop items %v, OnReorder %v; want [b c a d] and (0, 3)", got, moves)
	}
	p.Frame() // the rows start easing from where they were drawn
	p.Advance(time.Second)
	if painted["a"].Origin.Y != 60 || l.Dragging() {
		t.Fatalf("a settled at %v, dragging %v", painted["a"], l.Dragging())
	}

	// Escape puts the row back.
	p.Press(Pt(40, 15))
	p.Move(Pt(40, 100))
	p.Type(Mods{}, KeyEscape)
	p.Release(Pt(40, 100))
	if got := Peek(items); !slices.Equal(got, []string{"b", "c", "a", "d"}) || len(moves) != 1 {
		t.Fatalf("Escape still moved: %v", got)
	}
}

func TestReorderableHandlesAndKeyboard(t *testing.T) {
	t.Parallel()
	items := State([]string{"a", "b", "c"})
	painted := map[string]Rect{}
	l := reorderFixture(items, painted, true).Handles()
	p := NewProbe(l, Sz(100, 200))
	defer p.Close()
	p.Frame()

	p.Press(Pt(60, 15)) // the row's body, not its handle
	p.Move(Pt(60, 80))
	if l.Dragging() {
		t.Fatal("with Handles the row's body started a drag")
	}
	p.Release(Pt(60, 80))

	p.Press(Pt(10, 15))
	p.Move(Pt(10, 80))
	if !l.Dragging() {
		t.Fatal("the handle did not start a drag")
	}
	p.Release(Pt(10, 80))
	if got := Peek(items); !slices.Equal(got, []string{"b", "c", "a"}) {
		t.Fatalf("dropped by the handle: %v, want [b c a]", got)
	}

	p.Type(Mods{}, KeyTab) // the first handle, now b's
	p.Key("alt+down")
	if got := Peek(items); !slices.Equal(got, []string{"c", "b", "a"}) {
		t.Fatalf("Alt+Down: %v, want [c b a]", got)
	}
	p.Key("alt+down")
	if got := Peek(items); !slices.Equal(got, []string{"c", "a", "b"}) {
		t.Fatalf("Alt+Down again, following the row: %v, want [c a b]", got)
	}
	p.Key("alt+down") // already last
	p.Key("alt+up")
	if got := Peek(items); !slices.Equal(got, []string{"c", "b", "a"}) {
		t.Fatalf("Alt+Up: %v, want [c b a]", got)
	}

	l.Disabled(true)
	p.Key("alt+up")
	if got := Peek(items); !slices.Equal(got, []string{"c", "b", "a"}) {
		t.Fatalf("disabled list moved: %v", got)
	}
}

func TestReorderableOffersMoveActions(t *testing.T) {
	t.Parallel()
	items := State([]string{"a", "b", "c"})
	p := NewProbe(reorderFixture(items, map[string]Rect{}, true), Sz(100, 200))
	defer p.Close()
	tree := p.Semantics()
	var listItems []SemNode
	for _, n := range tree.Nodes(RoleListItem) {
		listItems = append(listItems, n)
	}
	if len(listItems) != 3 || listItems[0].Now != 1 || listItems[0].Max != 3 || listItems[0].Actions&ActionIncrement == 0 {
		t.Fatalf("list items %+v; want three that know their place and can move", listItems)
	}
	p.Perform(listItems[0].ID, Action{Kind: ActionIncrement})
	if got := Peek(items); !slices.Equal(got, []string{"b", "a", "c"}) {
		t.Fatalf("increment on the first row: %v, want [b a c]", got)
	}
	handle, ok := p.Semantics().Find(RoleButton, "Move c")
	if !ok || handle.Actions&ActionDecrement == 0 {
		t.Fatalf("handle %+v ok %v", handle, ok)
	}
	p.Perform(handle.ID, Action{Kind: ActionDecrement})
	if got := Peek(items); !slices.Equal(got, []string{"b", "c", "a"}) {
		t.Fatalf("decrement on c's handle: %v, want [b c a]", got)
	}
}

func TestReorderableScrollsItsScrollNearTheEdge(t *testing.T) {
	t.Parallel()
	list := make([]string, 20)
	for i := range list {
		list[i] = string(rune('a' + i))
	}
	items := State(list)
	offset := State(0.0)
	p := NewProbe(Scroll(reorderFixture(items, map[string]Rect{}, false)).BindOffset(offset), Sz(100, 100))
	defer p.Close()
	p.Frame()
	p.Press(Pt(40, 15))
	p.Move(Pt(40, 30))
	p.Move(Pt(40, 98))
	for range 10 {
		p.Advance(50 * time.Millisecond)
	}
	if Peek(offset) <= 0 {
		t.Fatalf("holding a row at the bottom edge left the offset at %v", Peek(offset))
	}
	p.Release(Pt(40, 98))
	if got := Peek(items); got[0] == "a" {
		t.Fatalf("the row dropped after scrolling stayed first: %v", got)
	}
}

func TestReorderableKeysIgnoreFocusBehindAScrolledOutRow(t *testing.T) {
	t.Parallel()
	focusable := State("")
	items := State([]string{"a", "b", "c", "d", "e", "f", "g", "h"})
	offset := State(60.0)
	// The field sits above the Scroll where row b, scrolled out of the
	// window, would be drawn.
	p := NewProbe(Column(
		Box(TextInput(focusable)).Size(100, 30),
		Box(Scroll(reorderFixture(items, map[string]Rect{}, true)).BindOffset(offset)).Size(100, 60),
	), Sz(100, 200))
	defer p.Close()
	p.Frame()
	p.Click(Pt(50, 15))
	p.Frame()
	p.Key("alt+down")
	if got := Peek(items); !slices.Equal(got, []string{"a", "b", "c", "d", "e", "f", "g", "h"}) {
		t.Fatalf("Alt+Down with focus outside the list moved a hidden row: %v", got)
	}
}

func TestReorderableRowsAnswerScrollToKey(t *testing.T) {
	t.Parallel()
	items := State([]string{"a", "b", "c", "d", "e", "f", "g", "h"})
	var sc ScrollController
	p := NewProbe(Scroll(reorderFixture(items, map[string]Rect{}, false)).Controller(&sc), Sz(100, 60))
	defer p.Close()
	p.Frame()
	sc.JumpToKey("f")
	p.Frame()
	p.Frame()
	if sc.Offset() != 150 {
		t.Fatalf("JumpToKey(f) reached %v, want 150", sc.Offset())
	}
}
