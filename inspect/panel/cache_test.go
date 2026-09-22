//go:build ggui_inspector

package panel

import (
	"image/color"
	"slices"
	"strconv"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/inspect"
)

func TestPanelOutlinesAreOptIn(t *testing.T) {
	var in View
	in.Apply(ggui.InspectorOptions{})
	if in.outlines {
		t.Fatal("default outlines enabled")
	}
	in.Apply(ggui.InspectorOptions{ShowOutlines: true})
	if !in.outlines {
		t.Fatal("explicit outlines ignored")
	}
	in.Apply(ggui.InspectorOptions{ShowOutlines: false})
	if in.outlines {
		t.Fatal("outlines did not turn off")
	}
}

func TestPanelVisibleCacheTracksFoldingAndFiltering(t *testing.T) {
	tr := trace(entry("Column", 0, 0, 0, 100, 100), entry("Text", 1, 0, 0, 50, 20), entry("Box", 1, 0, 20, 50, 20))
	var in View
	first := in.visible(tr)
	if next := in.visible(tr); &first[0] != &next[0] {
		t.Fatal("unchanged visible list was rebuilt")
	}
	in.collapsed = map[inspectKey]bool{foldKey(keyOf(&tr.Nodes[0])): true}
	if got := in.visible(tr); !slices.Equal(got, []int{0}) {
		t.Fatal(got)
	}
	in.filter = "text"
	if got := in.visible(tr); !slices.Equal(got, []int{0, 1}) {
		t.Fatal(got)
	}
	tr.Nodes[1].Name = "Gone"
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

func TestPanelRowRange(t *testing.T) {
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

// described paints a semantic node whose value can change in place.
type described struct{ value string }

func (d *described) Layout(c ggui.Constraints, _ ggui.Env) ggui.Size {
	return c.Constrain(ggui.Sz(40, 20))
}
func (d *described) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Describe(r, d) }
func (d *described) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleButton, Name: "before", Value: d.value}
}

func TestPanelSnapshotTracksLiveValues(t *testing.T) {
	box := ggui.Box(ggui.Text("hello")).Pad(4)
	h := newHarness(t, box, ggui.Sz(800, 600))
	in := View{}
	in.bounds(h.dst.Size())
	in.tree = ggui.Rct(ggui.Point{}, ggui.Sz(300, 100))
	shown := in.visible(h.fr)
	snap := func() inspectPanelSnapshot {
		return in.panelSnapshot(h.dst, h.frame(), shown, 0, inspectPanelSnapshot{})
	}
	original := snap()
	if !original.equal(snap()) {
		t.Fatal("identical values differ")
	}
	tests := []struct {
		name    string
		change  func()
		restore func()
	}{
		{"computed color", func() { in.tab = inspectTabComputed; box.Fill(color.NRGBA{R: 123, A: 255}) }, func() { in.tab = 0; box.Fill(nil) }},
		{"padding", func() { box.Pad(5) }, func() { box.Pad(4) }},
		{"scroll", func() { in.scroll = 23 }, func() { in.scroll = 0 }},
		{"filter", func() { in.filter = "hello" }, func() { in.filter = "" }},
		{"tab", func() { in.tab = inspectTabComputed }, func() { in.tab = 0 }},
		{"focus", func() { in.filterFocus = true }, func() { in.filterFocus = false }},
		{"geometry", func() { h.p.Resize(ggui.Sz(801, 600)) }, func() { h.p.Resize(ggui.Sz(800, 600)) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := snap()
			tc.change()
			if before.equal(snap()) {
				t.Fatal("cache missed changed display")
			}
			tc.restore()
		})
	}
	// A value only a hidden tab shows does not repaint the panel.
	before := snap()
	box.Fill(color.NRGBA{R: 123, A: 255})
	if !before.equal(snap()) {
		t.Fatal("cache missed on a value the Layout tab does not show")
	}
	// Pointer motion within one row keeps the panel; crossing rows does not.
	in.panel, in.rows = ggui.Rct(ggui.Point{}, h.dst.Size()), []inspectRow{{y: 10, h: 23}, {y: 33, h: 23}}
	h.p.Move(ggui.Pt(20, 12))
	before = snap()
	h.p.Move(ggui.Pt(60, 30))
	if !before.equal(snap()) {
		t.Fatal("cache missed pointer motion within a row")
	}
	h.p.Move(ggui.Pt(60, 40))
	if before.equal(snap()) {
		t.Fatal("cache missed the pointer crossing to another row")
	}
}

func TestPanelSnapshotTracksLiveSemantics(t *testing.T) {
	d := &described{}
	h := newHarness(t, d, ggui.Sz(800, 600))
	in := View{tab: inspectTabSemantics}
	in.bounds(h.dst.Size())
	in.tree = ggui.Rct(ggui.Point{}, ggui.Sz(300, 100))
	shown := in.visible(h.fr)
	snap := func() inspectPanelSnapshot {
		return in.panelSnapshot(h.dst, h.frame(), shown, 0, inspectPanelSnapshot{})
	}
	before := snap()
	d.value = "changed"
	if before.equal(snap()) {
		t.Fatal("cache missed live semantics")
	}
}

func TestPanelWidePanelShowsLayoutColumn(t *testing.T) {
	h := newHarness(t, ggui.Box(ggui.Text("hello")).Pad(4), ggui.Sz(1400, 900))
	in := View{}
	in.paint(h.dst, h.fr)
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
	in.paint(h.dst, h.fr)
	if in.detailTab() != inspectTabSemantics {
		t.Fatal("Accessibility tab did not take the details pane")
	}
	in.dock = ggui.InspectorRight
	in.paint(h.dst, h.fr)
	if !in.layout.Empty() {
		t.Fatal("layout column survived docking right")
	}
}

func TestPanelPrunesStaleFolds(t *testing.T) {
	tr := trace(entry("Column", 0, 0, 0, 100, 60), entry("Text", 1, 0, 0, 100, 10))
	in := View{collapsed: map[inspectKey]bool{foldKey(keyOf(&tr.Nodes[0])): true}}
	for i := range inspectFoldLimit + 1 {
		in.collapsed[inspectKey{name: "Gone", path: "/9/" + strconv.Itoa(i)}] = true
	}
	in.visible(tr)
	if len(in.collapsed) != 1 || !in.folded(&tr.Nodes[0]) {
		t.Fatalf("collapsed = %d entries, want only the painted fold", len(in.collapsed))
	}
}

func TestPanelResetDropsPanelCache(t *testing.T) {
	h := newHarness(t, ggui.Box(ggui.Text("hello")), ggui.Sz(800, 600))
	h.rendered(t, 800, 600)
	in := New()
	in.paint(h.dst, h.fr)
	if in.cache.image == nil {
		t.Fatal("panel not cached")
	}
	rows := in.cache.snapshot.rows
	in.paint(h.dst, h.fr)
	if len(rows) == 0 || &rows[0] != &in.cache.snapshot.rows[0] {
		t.Fatal("unchanged panel was redrawn")
	}
	in.Reset()
	if in.cache.image != nil || in.cache.snapshot.rows != nil {
		t.Fatal("cache retained on close")
	}
	_ = inspect.Layout
}
