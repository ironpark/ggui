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
	in.apply(InspectorOptions{ShowOutlines: true, HideOutlines: true})
	if in.outlines {
		t.Fatal("legacy hide option ignored")
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
	original := in.panelSnapshot(c, shown, 0)
	if !original.equal(in.panelSnapshot(c, shown, 0)) {
		t.Fatal("identical values differ")
	}
	tests := []struct {
		name    string
		change  func()
		restore func()
	}{
		{"paint-only color", func() { box.fill = color.NRGBA{R: 123, A: 255} }, func() { box.fill = nil }},
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
			before := in.panelSnapshot(c, shown, 0)
			tc.change()
			if before.equal(in.panelSnapshot(c, shown, 0)) {
				t.Fatal("cache missed changed display")
			}
			tc.restore()
		})
	}
	// Semantic buffers can change in place while widget identity stays the same.
	c.resetSemantics()
	c.Leaf(c.fs().trace[0].rect, Node{Role: RoleButton, Name: "before"})
	c.fs().trace[0].widget = nil
	before := in.panelSnapshot(c, shown, 0)
	c.fs().sem[0].node.Value = "changed"
	if before.equal(in.panelSnapshot(c, shown, 0)) {
		t.Fatal("cache missed live semantics")
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
