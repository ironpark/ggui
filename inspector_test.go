//go:build ggui_inspector

package ggui

import (
	"slices"
	"strings"
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
	in := &inspector{sel: inspectKey{name: "Button", depth: 1, rect: Rct(Pt(0, 20), Sz(100, 20))}, pinned: true}
	if got := in.find(tr); got != 2 {
		t.Fatalf("find = %d, want the middle button at 2", got)
	}
}

// A widget that moved is still the same widget: the exact match fails and
// the looser one keeps the selection alive rather than dropping it.
func TestInspectorFollowsAWidgetThatMoved(t *testing.T) {
	in := &inspector{sel: inspectKey{name: "Button", depth: 1, rect: Rct(Pt(0, 20), Sz(100, 20))}, pinned: true}
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

// Clicking a row pins it; clicking the picker goes back to following the
// pointer. The wheel scrolls the tree the way ScrollWidget reads it.
func TestInspectorPinsAndScrolls(t *testing.T) {
	row := inspectRow{key: inspectKey{name: "Button", depth: 1, rect: Rct(Pt(0, 20), Sz(100, 20))}, index: 2, y: 60, h: 12}
	in := &inspector{panel: Rct(Pt(200, 0), Sz(100, 300)), tree: Rct(Pt(200, 40), Sz(100, 260)), treeTop: 40, rows: []inspectRow{row}, chips: []inspectChip{{rect: Rct(Pt(210, 0), Sz(30, 30)), act: inspectUnpin}}}

	in.input(frameInput{pos: Pt(250, 64), down: []MouseButton{MouseButtonLeft}})
	if !in.pinned || in.sel != row.key {
		t.Fatalf("clicking a row did not pin it: pinned=%v sel=%+v", in.pinned, in.sel)
	}
	in.input(frameInput{pos: Pt(215, 10), down: []MouseButton{MouseButtonLeft}})
	if in.pinned {
		t.Fatal("clicking the picker did not release the pin")
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
	click := func(p Point) { in.input(frameInput{pos: p, down: []MouseButton{MouseButtonLeft}}) }
	click(Pt(215, 25))
	if in.dock != InspectorBottom {
		t.Fatal("the Bottom chip did not dock the panel")
	}
	click(Pt(255, 25))
	if !in.outlines {
		t.Fatal("the Outlines chip did not turn the outlines on")
	}
	in.input(frameInput{pos: Pt(250, 100), keys: []KeyboardKey{KeyArrowDown, KeyArrowDown, KeyArrowUp}})
	if in.move != 1 {
		t.Fatalf("move = %d, want the net arrow step of 1", in.move)
	}
	in.pinned = true
	in.input(frameInput{pos: Pt(250, 100), keys: []KeyboardKey{KeyEscape}})
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
	c := frameCanvas(img, Sz(400, 400))
	c.fs().pointer, c.fs().hasPointer = Pt(25, 25), true
	for i := range 50 {
		c.fs().trace = append(c.fs().trace, entry("Box", i%6, 0, float64(i), 40, 10))
	}
	in := &inspector{}
	in.apply(InspectorOptions{Dock: InspectorBottom})
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
	c.fs().hasPointer = false
	in.pinned, in.move = false, 2
	in.sel = inspectKey{}
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
	in.input(frameInput{keys: []KeyboardKey{KeyEscape}})
	in.panel = Rct(Pt(0, 0), Sz(100, 100))
	in.input(frameInput{pos: Pt(5, 5), keys: []KeyboardKey{KeyEscape}})
	if in.filter != "" {
		t.Fatal("Escape did not clear the filter")
	}
	in.filterFocus, in.focus = true, true
	in.input(frameInput{pos: Pt(5, 5), text: "Bu"})
	in.input(frameInput{pos: Pt(5, 5), keys: []KeyboardKey{KeyBackspace}})
	if in.filter != "B" {
		t.Fatalf("filter = %q, want typed text minus one Backspace", in.filter)
	}

	img := ebiten.NewImage(400, 400)
	defer img.Deallocate()
	c := frameCanvas(img, Sz(400, 400))
	c.fs().trace = tr
	c.fs().pointer, c.fs().hasPointer = Pt(5, 5), true
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
	c := frameCanvas(img, Sz(400, 200))
	c.fs().pointer, c.fs().hasPointer = Pt(25, 25), true
	for i := range 200 {
		c.fs().trace = append(c.fs().trace, entry("Box", i%6, 0, float64(i), 40, 10))
	}
	in := inspector{dock: InspectorRight}
	in.paint(c)

	if in.panel.Size.W == 0 || in.panel.Origin.X+in.panel.Size.W != 400 {
		t.Fatalf("panel is not docked to the right edge: %+v", in.panel)
	}
	if len(in.rows) == 0 {
		t.Fatal("no rows laid out")
	}
	if len(in.rows) >= len(c.fs().trace) {
		t.Fatalf("laid out %d rows of %d; the tree was not culled to the panel", len(in.rows), len(c.fs().trace))
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
	c := frameCanvas(img, Sz(400, 200))
	for i := range 200 {
		c.fs().trace = append(c.fs().trace, entry("Box", 0, 0, float64(i), 40, 10))
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

func inspectorCanvas(w Widget, size Size) *Canvas {
	c := frameCanvas(nil, size)
	c.fs().tracing = true
	c.Paint(w, Rct(Point{}, w.Layout(Tight(size), rootEnv())))
	return c
}

func TestInspectorTracksIdentityAcrossReorderAndRemoval(t *testing.T) {
	one, two := Scroll(Box()).Key("one"), Scroll(Box()).Key("two")
	c := inspectorCanvas(Column(one, two), Sz(800, 600))
	selected := -1
	for i := range c.fs().trace {
		if c.fs().trace[i].widget == two {
			selected = i
		}
	}
	var in inspector
	in.selectEntry(c.fs().trace, selected)
	moved := inspectorCanvas(Column(Scroll(Box()).Key("two"), Scroll(Box()).Key("one")), Sz(800, 600))
	found := in.find(moved.fs().trace)
	if found < 0 || moved.fs().trace[found].id != "two" {
		t.Fatal("selection did not follow the keyed widget")
	}
	removed := inspectorCanvas(Column(Scroll(Box()).Key("one")), Sz(800, 600))
	if in.find(removed.fs().trace) != -1 {
		t.Fatal("removed keyed widget selected its neighbour")
	}
}

func TestInspectorResolvesRebuiltWidgetByStructuralPath(t *testing.T) {
	build := func() Widget { return Column(Text("first"), Text("second")) }
	c := inspectorCanvas(build(), Sz(800, 600))
	var in inspector
	in.selectEntry(c.fs().trace, 2)
	next := inspectorCanvas(build(), Sz(900, 700))
	if got := in.find(next.fs().trace); got != 2 {
		t.Fatalf("selected %d, want second Text", got)
	}
}

func TestInspectorSearchesLabelsAndRoles(t *testing.T) {
	w := &twice{}
	w.Role = RoleButton
	w.SetName("Save document")
	c := inspectorCanvas(Column(Text("Heading"), w), Sz(800, 600))
	for _, query := range []string{"SAVE", "button"} {
		in := inspector{filter: query}
		shown := in.visible(c.fs().trace)
		if !slices.Equal(shown, []int{0, 2}) {
			t.Fatalf("%q matched %v, want ancestor and named control", query, shown)
		}
	}
}

func TestInspectorPickerRespectsClipAndPaintOrder(t *testing.T) {
	tr := trace(entry("Content", 0, 0, 0, 100, 100), entry("Hidden", 4, 0, 0, 100, 100))
	tr[1].clipped, tr[1].clip = true, Rct(Pt(0, 0), Sz(10, 10))
	if deepest(tr, Pt(50, 50)) != 0 {
		t.Fatal("picked a clipped-out widget")
	}
	tr = append(tr, entry("Overlay", 0, 0, 0, 100, 100))
	if deepest(tr, Pt(5, 5)) != 2 {
		t.Fatal("picked an earlier deep child through a later overlay")
	}
}

func TestInspectorPickerConsumesPressAndRelease(t *testing.T) {
	c := inspectorCanvas(Box(), Sz(1000, 700))
	in := inspector{picking: true}
	in.paint(c)
	for _, f := range []frameInput{
		{pos: Pt(20, 20), down: []MouseButton{MouseButtonLeft}},
		{pos: Pt(20, 20), up: []MouseButton{MouseButtonLeft}},
	} {
		if !in.input(f) {
			t.Fatal("picker leaked a click to the inspected app")
		}
	}
	if !in.pinned || in.picking || in.capture {
		t.Fatal("picker did not pin and finish its gesture")
	}
	if in.input(frameInput{pos: Pt(20, 20)}) {
		t.Fatal("keyboard focus blocked subsequent app hover")
	}
}

func TestInspectorIndependentPanesAndFilterFocus(t *testing.T) {
	c := inspectorCanvas(Box(Text("Long value")).Pad(12), Sz(1200, 800))
	in := inspector{dock: InspectorBottom}
	in.selectEntry(c.fs().trace, 0)
	in.paint(c)
	detailPoint := in.detail.Origin.Add(Pt(20.0, 60.0))
	in.input(frameInput{pos: detailPoint, wheel: Pt(0, -2), text: "unexpected"})
	if in.scroll != 0 || in.detailScroll != 2*inspectWheel || in.filter != "" {
		t.Fatal("details input affected the tree or unfocused filter")
	}
	in.input(frameInput{pos: in.filterRect.Origin.Add(Pt(10.0, 10.0)), down: []MouseButton{MouseButtonLeft}})
	in.input(frameInput{pos: Pt(20, 20), text: "Long"})
	if in.filter != "Long" {
		t.Fatal("focused filter stopped receiving text when pointer moved")
	}
}

func TestInspectorResizesAndClampsPanel(t *testing.T) {
	c := inspectorCanvas(Box(), Sz(1200, 800))
	in := inspector{dock: InspectorRight}
	in.paint(c)
	in.input(frameInput{pos: in.edge.Origin.Add(Pt(2.0, 40.0)), down: []MouseButton{MouseButtonLeft}})
	in.input(frameInput{pos: Pt(-500, 40), up: []MouseButton{MouseButtonLeft}})
	in.paint(c)
	if in.panel.Size.W != 1200*.85 || in.drag != inspectNoDrag {
		t.Fatalf("unbounded panel: %+v", in.panel)
	}
	in.act(inspectChip{act: inspectDockBottom})
	in.paint(c)
	in.input(frameInput{pos: in.edge.Origin.Add(Pt(40.0, 2.0)), down: []MouseButton{MouseButtonLeft}})
	in.input(frameInput{pos: Pt(40, 760), up: []MouseButton{MouseButtonLeft}})
	in.paint(c)
	if in.panel.Size.H < 180 {
		t.Fatal("panel collapsed below usable height")
	}
}

func TestInspectorBoxModelUsesRealLayout(t *testing.T) {
	box := Box(Box().Size(80, 30)).Pad(7, 11, 13, 17).Border(2, red)
	c := frameCanvas(nil, Sz(1200, 800))
	c.fs().tracing = true
	c.Paint(box, Rct(Pt(20, 30), box.Layout(Loose(Sz(400, 400)), rootEnv())))
	info := inspectedBox(c.fs().trace, 0)
	if !info.valid || info.padding != Insets(7, 11, 13, 17) || info.border != 2 || info.content != Sz(80, 30) {
		t.Fatalf("incorrect box model: %+v", info)
	}
	in := inspector{tab: inspectTabComputed}
	if !slices.ContainsFunc(inspectDetails(in.tab, c, 0), func(f inspectField) bool { return f.key == "border" && f.value == "2 px #ff0000" }) {
		t.Fatal("computed properties missed the actual border")
	}
}

func TestInspectorArrowNavigationRevealsPinnedRows(t *testing.T) {
	c := frameCanvas(nil, Sz(800, 600))
	for i := range 100 {
		c.fs().trace = append(c.fs().trace, entry("Box", 0, 0, float64(i), 30, 10))
	}
	var in inspector
	in.paint(c)
	in.focus = true
	in.input(frameInput{keys: []KeyboardKey{KeyEnd}})
	in.paint(c)
	if !in.pinned || in.sel.rect.Origin.Y != 99 || in.scroll <= 0 {
		t.Fatal("keyboard selection did not scroll into view")
	}
	in.input(frameInput{pos: in.tree.Origin.Add(Pt(20.0, 20.0)), wheel: Pt(0, 2)})
	before := in.scroll
	in.paint(c)
	if in.scroll != before {
		t.Fatal("pinned selection undid manual tree scrolling")
	}
}

func TestInspectorCopyAndClose(t *testing.T) {
	old := currentClipboard()
	defer SetClipboard(old)
	mem := &MemoryClipboard{}
	SetClipboard(mem)
	c := inspectorCanvas(Box(Text("Example")).Pad(12), Sz(1200, 800))
	in := inspector{dock: InspectorBottom}
	in.selectEntry(c.fs().trace, 0)
	in.paint(c)
	in.act(inspectChip{act: inspectCopy})
	if !strings.Contains(mem.Read(), "padding: 12 12 12 12") || !strings.Contains(mem.Read(), "width:") {
		t.Fatal("copy missed widget properties")
	}
	a := New(Config{}, func() Widget { return Box() })
	a.inspect = true
	a.insp = in
	a.insp.act(inspectChip{act: inspectClose})
	a.dispatchInput(frameInput{pos: a.insp.panel.Origin})
	if a.inspect || !a.insp.panel.Empty() {
		t.Fatal("close button did not disable the inspector")
	}
}

func TestInspectorLetsApplicationCaptureFinish(t *testing.T) {
	a := &App{inspect: true}
	drags := 0
	w := Pointer(Box()).OnDrag(func(PointerEvent) { drags++ })
	paintFrame(&a.input, w, Sz(800, 600))
	a.insp.paint(frameCanvas(nil, Sz(800, 600)))
	a.dispatchInput(frameInput{pos: Pt(20, 20), down: []MouseButton{MouseButtonLeft}})
	if a.input.pressed == nil {
		t.Fatal("app did not capture pointer")
	}
	p := a.insp.panel.Origin.Add(Pt(30, 100))
	a.dispatchInput(frameInput{pos: p})
	a.dispatchInput(frameInput{pos: p, up: []MouseButton{MouseButtonLeft}})
	if a.input.pressed != nil || drags == 0 {
		t.Fatal("inspector interrupted application drag")
	}
}

func TestInspectorCloseReleasesWidgetReferences(t *testing.T) {
	c := inspectorCanvas(Box(Text("temporary")), Sz(800, 600))
	a := &App{inspect: true}
	a.insp.selectEntry(c.fs().trace, 0)
	a.insp.paint(c)
	a.Inspector(false)
	if a.insp.sel.id != nil || a.insp.lastTrace != nil || a.insp.copySource != nil || a.insp.rows != nil || a.insp.chips != nil || a.insp.collapsed != nil {
		t.Fatal("closed inspector retained widget references")
	}
}

func TestInspectorCopyResolvesNewSelectionWithoutAnotherPaint(t *testing.T) {
	old := currentClipboard()
	defer SetClipboard(old)
	mem := &MemoryClipboard{}
	SetClipboard(mem)
	one, two := Box(Text("one")).Pad(11), Box(Text("two")).Pad(22)
	c := inspectorCanvas(Column(one, two), Sz(800, 600))
	var in inspector
	find := func(w Widget) int {
		for i, e := range c.fs().trace {
			if e.widget == w {
				return i
			}
		}
		t.Fatal("widget missing from trace")
		return -1
	}
	in.selectEntry(c.fs().trace, find(one))
	in.paint(c)
	if mem.Read() != "" {
		t.Fatal("paint wrote to the clipboard")
	}
	in.selectEntry(c.fs().trace, find(two))
	in.act(inspectChip{act: inspectCopy})
	if !strings.Contains(mem.Read(), "padding: 22 22 22 22") || strings.Contains(mem.Read(), "padding: 11 11 11 11") {
		t.Fatal("copy used details of the previous selection")
	}
	mem.Write("unchanged")
	c.fs().trace = nil
	in.act(inspectChip{act: inspectCopy})
	if mem.Read() != "unchanged" {
		t.Fatal("copy used a removed widget")
	}
}

func TestInspectorFilterScratchDoesNotKeepOldMatches(t *testing.T) {
	in := inspector{filter: "text"}
	tr := trace(entry("Column", 0, 0, 0, 100, 60), entry("Text", 1, 0, 0, 100, 20))
	if got := in.visible(tr); len(got) != 2 {
		t.Fatal("initial match missing")
	}
	tr[1].name = "Box"
	if got := in.visible(tr); len(got) != 0 {
		t.Fatalf("old filter matches survived buffer reuse: %v", got)
	}
	in.filter = "column"
	if got := in.visible(tr[:1]); !slices.Equal(got, []int{0}) {
		t.Fatalf("shrinking the trace retained old ancestors: %v", got)
	}
	in.filter = ""
	if got := in.visible(tr); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("clearing the filter lost rows: %v", got)
	}
}

// frameCanvas is a root Canvas with a frame already started, for a test that
// pokes at frame state directly instead of going through App or Probe.
func frameCanvas(img *ebiten.Image, logical Size) *Canvas {
	c := &Canvas{Image: img}
	if img != nil {
		c.scale = 1
	}
	c.fs().logical = logical
	return c
}
