package ggui

import (
	"slices"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func trace(entries ...traceEntry) []traceEntry { return entries }

func entry(name string, depth int, x, y, w, h float64) traceEntry {
	return traceEntry{rect: Rct(Pt(x, y), Sz(w, h)), depth: depth, name: name}
}

// A pinned widget is found again by name, depth and place, so that two rows
// of a list are not confused for one another.
func TestInspectorFindsPinnedAmongSiblings(t *testing.T) {
	tr := trace(
		entry("Column", 0, 0, 0, 100, 60),
		entry("Button", 1, 0, 0, 100, 20),
		entry("Button", 1, 0, 20, 100, 20),
		entry("Button", 1, 0, 40, 100, 20),
	)
	in := &inspector{sel: inspectKey{"Button", 1, Rct(Pt(0, 20), Sz(100, 20))}, pinned: true}
	if got := in.find(tr); got != 2 {
		t.Fatalf("find = %d, want the middle button at 2", got)
	}
}

// A widget that moved is still the same widget: the exact match fails and
// the looser one keeps the selection alive rather than dropping it.
func TestInspectorFollowsAWidgetThatMoved(t *testing.T) {
	in := &inspector{sel: inspectKey{"Button", 1, Rct(Pt(0, 20), Sz(100, 20))}, pinned: true}
	moved := trace(entry("Column", 0, 0, 0, 100, 60), entry("Button", 1, 0, 33, 100, 20))
	if got := in.find(moved); got != 1 {
		t.Fatalf("find = %d, want the moved button at 1", got)
	}
	if got := in.find(trace(entry("Column", 0, 0, 0, 100, 60))); got != -1 {
		t.Fatalf("find = %d, want -1 once the widget is gone", got)
	}
}

// deepest picks what a click would reach, not what was painted first.
func TestInspectorPicksTheInnermostWidget(t *testing.T) {
	tr := trace(
		entry("Column", 0, 0, 0, 100, 100),
		entry("Box", 1, 10, 10, 80, 80),
		entry("Text", 2, 20, 20, 30, 10),
	)
	if got := deepest(tr, Pt(25, 25)); got != 2 {
		t.Fatalf("deepest = %d, want the Text at 2", got)
	}
	if got := deepest(tr, Pt(85, 85)); got != 1 {
		t.Fatalf("deepest = %d, want the Box at 1", got)
	}
	if got := deepest(tr, Pt(500, 500)); got != -1 {
		t.Fatalf("deepest = %d, want -1 outside everything", got)
	}
}

// The panel takes the pointer only over itself, so the app keeps working
// while the inspector is open.
func TestInspectorConsumesOnlyItsOwnPanel(t *testing.T) {
	in := &inspector{panel: Rct(Pt(200, 0), Sz(100, 300)), treeTop: 40}
	if in.input(frameInput{pos: Pt(50, 50)}) {
		t.Fatal("took a pointer that was over the app")
	}
	if !in.input(frameInput{pos: Pt(250, 50)}) {
		t.Fatal("let a pointer over the panel through to the app")
	}
	var off inspector
	if off.input(frameInput{pos: Pt(250, 50)}) {
		t.Fatal("an inspector with no panel took input")
	}
}

// Clicking a row pins it; clicking the header goes back to following the
// pointer. The wheel scrolls the tree the way ScrollWidget reads it.
func TestInspectorPinsAndScrolls(t *testing.T) {
	row := inspectRow{key: inspectKey{"Button", 1, Rct(Pt(0, 20), Sz(100, 20))}, index: 2, y: 60, h: 12}
	in := &inspector{panel: Rct(Pt(200, 0), Sz(100, 300)), treeTop: 40, rows: []inspectRow{row}}

	in.input(frameInput{pos: Pt(250, 64), down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	if !in.pinned || in.sel != row.key {
		t.Fatalf("clicking a row did not pin it: pinned=%v sel=%+v", in.pinned, in.sel)
	}
	in.input(frameInput{pos: Pt(250, 10), down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	if in.pinned {
		t.Fatal("clicking the header did not release the pin")
	}
	in.input(frameInput{pos: Pt(250, 100), wheel: Pt(0, -2)})
	if in.scroll != 2*inspectWheel {
		t.Fatalf("scroll = %v, want %v", in.scroll, 2*inspectWheel)
	}
}

// The toolbar chips dock the panel and toggle the outlines; the arrow keys
// and Escape step and release the pin.
func TestInspectorChipsAndKeys(t *testing.T) {
	in := &inspector{panel: Rct(Pt(200, 0), Sz(100, 300)), treeTop: 40, chips: []inspectChip{
		{rect: Rct(Pt(210, 20), Sz(30, 12)), act: inspectDockBottom},
		{rect: Rct(Pt(250, 20), Sz(30, 12)), act: inspectToggleOutlines},
	}}
	click := func(p Point) { in.input(frameInput{pos: p, down: []ebiten.MouseButton{ebiten.MouseButtonLeft}}) }
	click(Pt(215, 25))
	if in.dock != InspectorBottom {
		t.Fatal("the Bottom chip did not dock the panel")
	}
	click(Pt(255, 25))
	if !in.noOutlines {
		t.Fatal("the Outlines chip did not turn the outlines off")
	}
	in.input(frameInput{pos: Pt(250, 100), keys: []ebiten.Key{ebiten.KeyArrowDown, ebiten.KeyArrowDown, ebiten.KeyArrowUp}})
	if in.move != 1 {
		t.Fatalf("move = %d, want the net arrow step of 1", in.move)
	}
	in.pinned = true
	in.input(frameInput{pos: Pt(250, 100), keys: []ebiten.Key{ebiten.KeyEscape}})
	if in.pinned {
		t.Fatal("Escape did not release the pin")
	}
}

// Docked to the bottom, the panel spans the width and the tree and details
// sit side by side; without outlines, the app is left untouched apart from
// the selection.
func TestInspectorDocksToTheBottom(t *testing.T) {
	img := ebiten.NewImage(400, 400)
	defer img.Deallocate()
	c := &Canvas{Image: img, scale: 1, logical: Sz(400, 400)}
	c.pointer, c.hasPointer = Pt(25, 25), true
	for i := range 50 {
		c.trace = append(c.trace, entry("Box", i%6, 0, float64(i), 40, 10))
	}
	in := &inspector{}
	in.apply(InspectorOptions{Dock: InspectorBottom, HideOutlines: true})
	in.paint(c)
	if in.panel.Origin.X != 0 || in.panel.Size.W != 400 || in.panel.Origin.Y+in.panel.Size.H != 400 {
		t.Fatalf("panel is not docked to the bottom edge: %+v", in.panel)
	}
	if len(in.rows) == 0 {
		t.Fatal("no rows laid out")
	}
	for _, r := range in.rows {
		if r.y+r.h <= in.treeTop || r.y >= 400 {
			t.Fatalf("row at %v is outside the tree area", r.y)
		}
	}
	for _, act := range []inspectAction{inspectUnpin, inspectDockRight, inspectDockBottom, inspectToggleOutlines} {
		if !slices.ContainsFunc(in.chips, func(c inspectChip) bool { return c.act == act }) {
			t.Fatalf("toolbar is missing chip %d", act)
		}
	}
	// Arrow keys resolve against this frame's trace and pin where they land;
	// with nothing selected, Down starts from the top.
	c.hasPointer = false
	in.pinned, in.move = false, 2
	in.paint(c)
	if !in.pinned || in.sel.rect.Origin.Y != 1 {
		t.Fatalf("arrow step did not pin the second row: pinned=%v sel=%+v", in.pinned, in.sel)
	}
}

// Folding a widget hides what it painted; the filter shows matches with
// what they sit in, ignoring folds; the pointer's pick unfolds down to it.
func TestInspectorFoldsAndFilters(t *testing.T) {
	tr := trace(
		entry("Column", 0, 0, 0, 100, 60),
		entry("Card", 1, 0, 0, 100, 20),
		entry("Text", 2, 0, 0, 100, 10),
		entry("Button", 1, 0, 20, 100, 20),
	)
	in := &inspector{}
	if got := in.visible(tr); len(got) != 4 {
		t.Fatalf("visible = %v, want all four", got)
	}
	in.act(inspectChip{act: inspectCollapse, key: keyOf(&tr[1])})
	if got := in.visible(tr); !slices.Equal(got, []int{0, 1, 3}) {
		t.Fatalf("visible = %v after folding Card, want [0 1 3]", got)
	}
	in.filter = "text"
	if got := in.visible(tr); !slices.Equal(got, []int{0, 1, 2}) {
		t.Fatalf("visible = %v while filtering, want the match and its ancestors", got)
	}
	in.input(frameInput{keys: []ebiten.Key{ebiten.KeyEscape}})
	in.panel = Rct(Pt(0, 0), Sz(100, 100))
	in.input(frameInput{pos: Pt(5, 5), keys: []ebiten.Key{ebiten.KeyEscape}})
	if in.filter != "" {
		t.Fatal("Escape did not clear the filter")
	}
	in.input(frameInput{pos: Pt(5, 5), text: "Bu"})
	in.input(frameInput{pos: Pt(5, 5), keys: []ebiten.Key{ebiten.KeyBackspace}})
	if in.filter != "B" {
		t.Fatalf("filter = %q, want typed text minus one Backspace", in.filter)
	}

	img := ebiten.NewImage(400, 400)
	defer img.Deallocate()
	c := &Canvas{Image: img, scale: 1, logical: Sz(400, 400), trace: tr}
	c.pointer, c.hasPointer = Pt(5, 5), true
	in.filter = ""
	in.paint(c)
	if in.collapsed[keyOf(&tr[1])] {
		t.Fatal("picking the Text did not unfold the Card above it")
	}
}

// A whole pass over a real Canvas: the panel renders, the tree is laid out
// and culled to what fits, and the rows it leaves behind are the ones the
// next frame's click will hit.
func TestInspectorPaintsAndCullsTheTree(t *testing.T) {
	img := ebiten.NewImage(400, 200)
	defer img.Deallocate()
	c := &Canvas{Image: img, scale: 1, logical: Sz(400, 200)}
	c.pointer, c.hasPointer = Pt(25, 25), true
	for i := range 200 {
		c.trace = append(c.trace, entry("Box", i%6, 0, float64(i), 40, 10))
	}
	var in inspector
	in.paint(c)

	if in.panel.Size.W == 0 || in.panel.Origin.X+in.panel.Size.W != 400 {
		t.Fatalf("panel is not docked to the right edge: %+v", in.panel)
	}
	if len(in.rows) == 0 {
		t.Fatal("no rows laid out")
	}
	if len(in.rows) >= len(c.trace) {
		t.Fatalf("laid out %d rows of %d; the tree was not culled to the panel", len(in.rows), len(c.trace))
	}
	for _, r := range in.rows {
		if r.y+r.h <= in.treeTop || r.y >= 200 {
			t.Fatalf("row at %v is outside the panel", r.y)
		}
	}
	// Nothing is pinned, so the pointer chose, and the tree scrolled to it.
	if in.pinned {
		t.Fatal("paint pinned a selection on its own")
	}
	if in.sel.name != "Box" {
		t.Fatalf("selection = %+v, want the widget under the pointer", in.sel)
	}
}

// Scrolling past either end settles at the end rather than running away.
func TestInspectorClampsScroll(t *testing.T) {
	img := ebiten.NewImage(400, 200)
	defer img.Deallocate()
	c := &Canvas{Image: img, scale: 1, logical: Sz(400, 200)}
	for i := range 200 {
		c.trace = append(c.trace, entry("Box", 0, 0, float64(i), 40, 10))
	}
	in := &inspector{scroll: -500}
	in.paint(c)
	if in.scroll != 0 {
		t.Fatalf("scroll = %v, want 0 at the top", in.scroll)
	}
	in.scroll = 1e6
	in.paint(c)
	if in.scroll <= 0 || in.scroll >= 1e6 {
		t.Fatalf("scroll = %v, want it clamped to the last page", in.scroll)
	}
}
