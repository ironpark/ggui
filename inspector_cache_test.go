//go:build ggui_inspector

package ggui

import (
	"image/color"
	"slices"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestInspectorOutlinesAreOptIn(t *testing.T) {
	var in inspector
	in.apply(InspectorOptions{})
	if in.outlines {
		t.Fatal("default outlines enabled")
	}
	in.apply(InspectorOptions{ShowOutlines: true})
	if !in.outlines {
		t.Fatal("explicit outlines ignored")
	}
	in.apply(InspectorOptions{ShowOutlines: false})
	if in.outlines {
		t.Fatal("outlines did not turn off")
	}
}

func TestInspectorVisibleCacheTracksFoldingAndFiltering(t *testing.T) {
	tr := trace(entry("Column", 0, 0, 0, 100, 100), entry("Text", 1, 0, 0, 50, 20), entry("Box", 1, 0, 20, 50, 20))
	var in inspector
	first := in.visible(tr)
	if next := in.visible(tr); &first[0] != &next[0] {
		t.Fatal("unchanged visible list was rebuilt")
	}
	in.collapsed = map[inspectKey]bool{foldKey(keyOf(&tr[0])): true}
	if got := in.visible(tr); !slices.Equal(got, []int{0}) {
		t.Fatal(got)
	}
	in.filter = "text"
	if got := in.visible(tr); !slices.Equal(got, []int{0, 1}) {
		t.Fatal(got)
	}
	tr[1].name = "Gone"
	if got := in.visible(tr); len(got) != 0 {
		t.Fatal("stale search result", got)
	}
	in.filter = ""
	if got := in.visible(tr); !slices.Equal(got, []int{0}) {
		t.Fatal(got)
	}
	clear(in.collapsed)
	if got := in.visible(tr); len(got) != 3 {
		t.Fatal("stale collapsed list", got)
	}
}

func TestInspectorRowRange(t *testing.T) {
	for _, tc := range []struct {
		scroll, height float64
		first, end     int
	}{
		{0, 46, 0, 2}, {1, 46, 0, 3}, {23, 46, 1, 3}, {23000, 46, 1000, 1002},
	} {
		first, end := inspectRowRange(10000, tc.scroll, tc.height)
		if first != tc.first || end != tc.end {
			t.Fatalf("%+v: got %d,%d", tc, first, end)
		}
	}
}

func TestInspectorPanelSnapshotTracksLiveValues(t *testing.T) {
	box := Box(Text("hello")).Pad(4)
	c := inspectorCanvas(box, Sz(800, 600))
	in := inspector{}
	in.bounds(c.Size())
	in.tree = Rct(Point{}, Sz(300, 100))
	shown := in.visible(c.fs().trace)
	original := in.panelSnapshot(c, shown, 0, inspectPanelSnapshot{})
	if !original.equal(in.panelSnapshot(c, shown, 0, inspectPanelSnapshot{})) {
		t.Fatal("identical values differ")
	}
	tests := []struct {
		name    string
		change  func()
		restore func()
	}{
		{"computed color", func() { in.tab, box.fill = inspectTabComputed, color.NRGBA{R: 123, A: 255} }, func() { in.tab, box.fill = 0, nil }},
		{"padding", func() { box.padding.Top++ }, func() { box.padding.Top-- }},
		{"scroll", func() { in.scroll = 23 }, func() { in.scroll = 0 }},
		{"filter", func() { in.filter = "hello" }, func() { in.filter = "" }},
		{"tab", func() { in.tab = inspectTabComputed }, func() { in.tab = 0 }},
		{"focus", func() { in.filterFocus = true }, func() { in.filterFocus = false }},
		{"scale", func() { c.scale = 2 }, func() { c.scale = 1 }},
		{"geometry", func() { c.fs().trace[0].rect.Size.W++ }, func() { c.fs().trace[0].rect.Size.W-- }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := in.panelSnapshot(c, shown, 0, inspectPanelSnapshot{})
			tc.change()
			if before.equal(in.panelSnapshot(c, shown, 0, inspectPanelSnapshot{})) {
				t.Fatal("cache missed changed display")
			}
			tc.restore()
		})
	}
	// A value only a hidden tab shows does not repaint the panel.
	before := in.panelSnapshot(c, shown, 0, inspectPanelSnapshot{})
	box.fill = color.NRGBA{R: 123, A: 255}
	if !before.equal(in.panelSnapshot(c, shown, 0, inspectPanelSnapshot{})) {
		t.Fatal("cache missed on a value the Layout tab does not show")
	}
	// Pointer motion within one row keeps the panel; crossing rows does not.
	in.panel, in.rows = Rct(Point{}, c.Size()), []inspectRow{{y: 10, h: 23}, {y: 33, h: 23}}
	c.fs().pointer, c.fs().hasPointer = Pt(20, 12), true
	before = in.panelSnapshot(c, shown, 0, inspectPanelSnapshot{})
	c.fs().pointer = Pt(60, 30)
	if !before.equal(in.panelSnapshot(c, shown, 0, inspectPanelSnapshot{})) {
		t.Fatal("cache missed pointer motion within a row")
	}
	c.fs().pointer = Pt(60, 40)
	if before.equal(in.panelSnapshot(c, shown, 0, inspectPanelSnapshot{})) {
		t.Fatal("cache missed the pointer crossing to another row")
	}
	c.fs().hasPointer = false
	// Semantic buffers can change in place while widget identity stays the same.
	in.tab = inspectTabSemantics
	c.resetSemantics()
	c.Leaf(c.fs().trace[0].rect, Node{Role: RoleButton, Name: "before"})
	c.fs().trace[0].widget = nil
	before = in.panelSnapshot(c, shown, 0, inspectPanelSnapshot{})
	c.fs().sem[0].node.Value = "changed"
	if before.equal(in.panelSnapshot(c, shown, 0, inspectPanelSnapshot{})) {
		t.Fatal("cache missed live semantics")
	}
}

// Wide bottom panels give Layout its own column; the details pane then
// defaults to Computed and the Layout tab is not offered.
func TestInspectorWidePanelShowsLayoutColumn(t *testing.T) {
	c := inspectorCanvas(Box(Text("hello")).Pad(4), Sz(1400, 900))
	in := inspector{}
	in.paint(c)
	if in.layout.Empty() {
		t.Fatalf("no layout column on a %v panel", in.panel.Size)
	}
	if in.detailTab() != inspectTabComputed {
		t.Fatalf("details show %d, want Computed beside the layout column", in.detailTab())
	}
	tabs := 0
	for _, ch := range in.chips {
		if ch.act == inspectSelectTab {
			tabs++
			if ch.tab == inspectTabLayout {
				t.Fatal("Layout tab offered while it has its own column")
			}
		}
	}
	if tabs != 2 {
		t.Fatalf("%d tabs, want Computed and Accessibility", tabs)
	}
	in.act(inspectChip{act: inspectSelectTab, tab: inspectTabSemantics})
	in.paint(c)
	if in.detailTab() != inspectTabSemantics {
		t.Fatal("Accessibility tab did not take the details pane")
	}
	in.dock = InspectorRight
	in.paint(c)
	if !in.layout.Empty() {
		t.Fatal("layout column survived docking right")
	}
}

// Folds of widgets no longer painted are dropped once the map outgrows the
// limit, so a long session does not pin every rebuilt widget.
func TestInspectorPrunesStaleFolds(t *testing.T) {
	tr := trace(entry("Column", 0, 0, 0, 100, 60), entry("Text", 1, 0, 0, 100, 10))
	in := inspector{collapsed: map[inspectKey]bool{foldKey(keyOf(&tr[0])): true}}
	for i := range inspectFoldLimit + 1 {
		in.collapsed[inspectKey{name: "Gone", path: "/9/" + itoa(i)}] = true
	}
	in.visible(tr)
	if len(in.collapsed) != 1 || !in.folded(&tr[0]) {
		t.Fatalf("collapsed = %d entries, want only the painted fold", len(in.collapsed))
	}
}

func TestInspectorCloseDropsPanelCache(t *testing.T) {
	c := inspectorCanvas(Box(Text("hello")), Sz(800, 600))
	c.Image = ebiten.NewImage(800, 600)
	defer c.Image.Deallocate()
	a := &App{}
	a.insp.paint(c)
	if a.insp.cache.image == nil {
		t.Fatal("panel not cached")
	}
	rows := a.insp.cache.snapshot.rows
	a.insp.paint(c)
	if len(rows) == 0 || &rows[0] != &a.insp.cache.snapshot.rows[0] {
		t.Fatal("unchanged panel was redrawn")
	}
	a.Inspector(false)
	if a.insp.cache.image != nil || a.insp.cache.snapshot.rows != nil {
		t.Fatal("cache retained on close")
	}
}
