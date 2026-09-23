//go:build ggui_inspector

package panel

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggfx"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/inspect"
)

func trace(entries ...inspect.Node) *inspect.Frame { return &inspect.Frame{Nodes: entries} }

func entry(name string, depth int, x, y, w, h float64) inspect.Node {
	return inspect.Node{Rect: ggui.Rct(ggui.Pt(x, y), ggui.Sz(w, h)), Depth: depth, Name: name}
}

// capture is an Overlay that keeps the Canvas it is painted on, so a test
// can paint the panel on a probe's canvas directly.
type capture struct{ dst *ggui.Canvas }

func (c *capture) Paint(dst *ggui.Canvas)                     { c.dst = dst }
func (c *capture) Input(ggui.OverlayInput) bool               { return false }
func (c *capture) Cursor(ggui.Point) (ggui.CursorShape, bool) { return 0, false }

// harness shows w in a probe of size and hands back the canvas frames
// paint on and the frame the last one built.
type harness struct {
	p   *ggui.Probe
	dst *ggui.Canvas
	fr  *inspect.Frame
}

func newHarness(t *testing.T, w ggui.Widget, size ggui.Size) *harness {
	t.Helper()
	h := &harness{p: ggui.NewProbe(w, size)}
	t.Cleanup(h.p.Close)
	cap := &capture{}
	h.p.SetOverlay(cap)
	h.p.OnInspect(func(fr *inspect.Frame) { h.fr = fr })
	h.p.Frame()
	h.dst = cap.dst
	return h
}

// frame runs a frame and returns what it built.
func (h *harness) frame() *inspect.Frame { h.p.Frame(); return h.fr }

// rendered gives the probe an image to draw into.
func (h *harness) rendered(t *testing.T, w, ht int) {
	t.Helper()
	img := ggfx.NewImage(w, ht)
	t.Cleanup(img.Deallocate)
	h.p.RenderTo(img)
	h.p.Frame()
}

// named describes itself with a role and name, like a control.
type named struct {
	ggui.Interactive
	role ggui.Role
	name string
}

func (n *named) Layout(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(40, 20)) }
func (n *named) Paint(dst *ggui.Canvas, r ggui.Rect)             { dst.Describe(r, n) }
func (n *named) Semantics() (ggui.Role, string)                  { return n.role, n.name }

func TestPanelFindsPinnedAmongSiblings(t *testing.T) {
	tr := trace(
		entry("Column", 0, 0, 0, 100, 60),
		entry("Button", 1, 0, 0, 100, 20),
		entry("Button", 1, 0, 20, 100, 20),
		entry("Button", 1, 0, 40, 100, 20),
	)
	in := &View{sel: inspectKey{name: "Button", depth: 1, rect: ggui.Rct(ggui.Pt(0, 20), ggui.Sz(100, 20))}, pinned: true}
	if got := in.find(tr); got != 2 {
		t.Fatalf("find = %d, want the middle button at 2", got)
	}
}

func TestPanelFollowsAWidgetThatMoved(t *testing.T) {
	in := &View{sel: inspectKey{name: "Button", depth: 1, rect: ggui.Rct(ggui.Pt(0, 20), ggui.Sz(100, 20))}, pinned: true}
	moved := trace(entry("Column", 0, 0, 0, 100, 60), entry("Button", 1, 0, 33, 100, 20))
	if got := in.find(moved); got != 1 {
		t.Fatalf("find = %d, want the moved button at 1", got)
	}
	if got := in.find(trace(entry("Column", 0, 0, 0, 100, 60))); got != -1 {
		t.Fatalf("find = %d, want -1 once the widget is gone", got)
	}
}

func TestPanelConsumesOnlyItsOwnPanel(t *testing.T) {
	in := &View{panel: ggui.Rct(ggui.Pt(200, 0), ggui.Sz(100, 300)), treeTop: 40}
	if in.Input(ggui.OverlayInput{Pos: ggui.Pt(50, 50)}) {
		t.Fatal("took a pointer that was over the app")
	}
	if !in.Input(ggui.OverlayInput{Pos: ggui.Pt(250, 50)}) {
		t.Fatal("let a pointer over the panel through to the app")
	}
	var off View
	if off.Input(ggui.OverlayInput{Pos: ggui.Pt(250, 50)}) {
		t.Fatal("a panel with no rect took input")
	}
}

func TestPanelPinsAndScrolls(t *testing.T) {
	row := inspectRow{key: inspectKey{name: "Button", depth: 1, rect: ggui.Rct(ggui.Pt(0, 20), ggui.Sz(100, 20))}, index: 2, y: 60, h: 12}
	in := &View{panel: ggui.Rct(ggui.Pt(200, 0), ggui.Sz(100, 300)), tree: ggui.Rct(ggui.Pt(200, 40), ggui.Sz(100, 260)), treeTop: 40, rows: []inspectRow{row}, chips: []inspectChip{{rect: ggui.Rct(ggui.Pt(210, 0), ggui.Sz(30, 30)), act: inspectUnpin}}}
	in.Input(ggui.OverlayInput{Pos: ggui.Pt(250, 64), Down: []ggui.MouseButton{ggui.MouseButtonLeft}})
	if !in.pinned || in.sel != row.key {
		t.Fatalf("clicking a row did not pin it: pinned=%v sel=%+v", in.pinned, in.sel)
	}
	in.Input(ggui.OverlayInput{Pos: ggui.Pt(215, 10), Down: []ggui.MouseButton{ggui.MouseButtonLeft}})
	if in.pinned {
		t.Fatal("clicking the picker did not release the pin")
	}
	in.Input(ggui.OverlayInput{Pos: ggui.Pt(250, 100), Wheel: ggui.Pt(0, -2)})
	if in.scroll != 2*inspectWheel {
		t.Fatalf("scroll = %v, want %v", in.scroll, 2*inspectWheel)
	}
}

func TestPanelChipsAndKeys(t *testing.T) {
	in := &View{panel: ggui.Rct(ggui.Pt(200, 0), ggui.Sz(100, 300)), treeTop: 40, chips: []inspectChip{
		{rect: ggui.Rct(ggui.Pt(210, 20), ggui.Sz(30, 12)), act: inspectDockBottom},
		{rect: ggui.Rct(ggui.Pt(250, 20), ggui.Sz(30, 12)), act: inspectToggleOutlines},
	}}
	click := func(p ggui.Point) {
		in.Input(ggui.OverlayInput{Pos: p, Down: []ggui.MouseButton{ggui.MouseButtonLeft}})
	}
	click(ggui.Pt(215, 25))
	if in.dock != ggui.InspectorBottom {
		t.Fatal("the Bottom chip did not dock the panel")
	}
	click(ggui.Pt(255, 25))
	if !in.outlines {
		t.Fatal("the Outlines chip did not turn the outlines on")
	}
	in.Input(ggui.OverlayInput{Pos: ggui.Pt(250, 100), Keys: []ggui.KeyboardKey{ggui.KeyArrowDown, ggui.KeyArrowDown, ggui.KeyArrowUp}})
	if in.move != 1 {
		t.Fatalf("move = %d, want the net arrow step of 1", in.move)
	}
	in.pinned = true
	in.Input(ggui.OverlayInput{Pos: ggui.Pt(250, 100), Keys: []ggui.KeyboardKey{ggui.KeyEscape}})
	if in.pinned {
		t.Fatal("Escape did not release the pin")
	}
}

func rows(n int, depths bool) *inspect.Frame {
	fr := trace()
	for i := range n {
		d := 0
		if depths {
			d = i % 6
		}
		fr.Nodes = append(fr.Nodes, entry("Box", d, 0, float64(i), 40, 10))
	}
	return fr
}

func TestPanelDocksToTheBottom(t *testing.T) {
	h := newHarness(t, ggui.Box(), ggui.Sz(400, 400))
	h.rendered(t, 400, 400)
	h.p.Move(ggui.Pt(25, 25))
	in := New()
	in.Apply(ggui.InspectorOptions{Dock: ggui.InspectorBottom})
	in.paint(h.dst, rows(50, true))
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
	// with no pointer and nothing selected, Down starts from the top.
	fresh := newHarness(t, ggui.Box(), ggui.Sz(400, 400))
	in.pinned, in.move = false, 2
	in.sel = inspectKey{}
	in.paint(fresh.dst, rows(50, true))
	if !in.pinned || in.sel.rect.Origin.Y != 1 {
		t.Fatalf("arrow step did not pin the second row: pinned=%v sel=%+v", in.pinned, in.sel)
	}
}

func TestPanelFoldsAndFilters(t *testing.T) {
	tr := trace(
		entry("Column", 0, 0, 0, 100, 60),
		entry("Card", 1, 0, 0, 100, 20),
		entry("Text", 2, 0, 0, 100, 10),
		entry("Button", 1, 0, 20, 100, 20),
	)
	in := New()
	if got := in.visible(tr); len(got) != 4 {
		t.Fatalf("visible = %v, want all four", got)
	}
	in.act(inspectChip{act: inspectCollapse, key: keyOf(&tr.Nodes[1])})
	if got := in.visible(tr); !slices.Equal(got, []int{0, 1, 3}) {
		t.Fatalf("visible = %v after folding Card, want [0 1 3]", got)
	}
	in.filter = "text"
	if got := in.visible(tr); !slices.Equal(got, []int{0, 1, 2}) {
		t.Fatalf("visible = %v while filtering, want the match and its ancestors", got)
	}
	in.Input(ggui.OverlayInput{Keys: []ggui.KeyboardKey{ggui.KeyEscape}})
	in.panel = ggui.Rct(ggui.Pt(0, 0), ggui.Sz(100, 100))
	in.Input(ggui.OverlayInput{Pos: ggui.Pt(5, 5), Keys: []ggui.KeyboardKey{ggui.KeyEscape}})
	if in.filter != "" {
		t.Fatal("Escape did not clear the filter")
	}
	in.filterFocus, in.focus = true, true
	in.Input(ggui.OverlayInput{Pos: ggui.Pt(5, 5), Text: "Bu"})
	in.Input(ggui.OverlayInput{Pos: ggui.Pt(5, 5), Keys: []ggui.KeyboardKey{ggui.KeyBackspace}})
	if in.filter != "B" {
		t.Fatalf("filter = %q, want typed text minus one Backspace", in.filter)
	}
	// The pointer's pick unfolds down to what it is over.
	h := newHarness(t, ggui.Box(), ggui.Sz(400, 400))
	h.rendered(t, 400, 400)
	h.p.Move(ggui.Pt(5, 5))
	in.filter = ""
	in.paint(h.dst, tr)
	if in.collapsed[keyOf(&tr.Nodes[1])] {
		t.Fatal("picking the Text did not unfold the Card above it")
	}
}

func TestPanelPaintsAndCullsTheTree(t *testing.T) {
	h := newHarness(t, ggui.Box(), ggui.Sz(400, 200))
	h.rendered(t, 400, 200)
	h.p.Move(ggui.Pt(25, 25))
	tr := rows(200, true)
	in := View{dock: ggui.InspectorRight}
	in.paint(h.dst, tr)
	if in.panel.Size.W == 0 || in.panel.Origin.X+in.panel.Size.W != 400 {
		t.Fatalf("panel is not docked to the right edge: %+v", in.panel)
	}
	if len(in.rows) == 0 || len(in.rows) >= len(tr.Nodes) {
		t.Fatalf("laid out %d rows of %d; the tree was not culled to the panel", len(in.rows), len(tr.Nodes))
	}
	for _, r := range in.rows {
		if r.y+r.h <= in.treeTop || r.y >= 200 {
			t.Fatalf("row at %v is outside the panel", r.y)
		}
	}
	if in.pinned || in.sel.name != "Box" {
		t.Fatalf("selection = %+v pinned=%v, want the unpinned widget under the pointer", in.sel, in.pinned)
	}
}

func TestPanelClampsScroll(t *testing.T) {
	h := newHarness(t, ggui.Box(), ggui.Sz(400, 200))
	h.rendered(t, 400, 200)
	tr := rows(200, false)
	in := &View{scroll: -500}
	in.paint(h.dst, tr)
	if in.scroll != 0 {
		t.Fatalf("scroll = %v, want 0 at the top", in.scroll)
	}
	in.scroll = 1e6
	in.paint(h.dst, tr)
	if in.scroll <= 0 || in.scroll >= 1e6 {
		t.Fatalf("scroll = %v, want it clamped to the last page", in.scroll)
	}
}

func TestPanelTracksIdentityAcrossReorderAndRemoval(t *testing.T) {
	one, two := ggui.Scroll(ggui.Box()).Key("one"), ggui.Scroll(ggui.Box()).Key("two")
	h := newHarness(t, ggui.Column(one, two), ggui.Sz(800, 600))
	selected := slices.IndexFunc(h.fr.Nodes, func(n inspect.Node) bool { return n.ID == "two" })
	var in View
	in.selectEntry(h.fr, selected)
	moved := newHarness(t, ggui.Column(ggui.Scroll(ggui.Box()).Key("two"), ggui.Scroll(ggui.Box()).Key("one")), ggui.Sz(800, 600))
	found := in.find(moved.fr)
	if found < 0 || moved.fr.Nodes[found].ID != "two" {
		t.Fatal("selection did not follow the keyed widget")
	}
	removed := newHarness(t, ggui.Column(ggui.Scroll(ggui.Box()).Key("one")), ggui.Sz(800, 600))
	if in.find(removed.fr) != -1 {
		t.Fatal("removed keyed widget selected its neighbour")
	}
}

func TestPanelResolvesRebuiltWidgetByStructuralPath(t *testing.T) {
	build := func() ggui.Widget { return ggui.Column(ggui.Text("first"), ggui.Text("second")) }
	h := newHarness(t, build(), ggui.Sz(800, 600))
	var in View
	in.selectEntry(h.fr, 2)
	next := newHarness(t, build(), ggui.Sz(900, 700))
	if got := in.find(next.fr); got != 2 {
		t.Fatalf("selected %d, want second Text", got)
	}
}

func TestPanelSearchesLabelsAndRoles(t *testing.T) {
	w := &named{role: ggui.RoleButton, name: "Save document"}
	h := newHarness(t, ggui.Column(ggui.Text("Heading"), w), ggui.Sz(800, 600))
	for _, query := range []string{"SAVE", "button"} {
		in := View{filter: query}
		if shown := in.visible(h.fr); !slices.Equal(shown, []int{0, 2}) {
			t.Fatalf("%q matched %v, want ancestor and named control", query, shown)
		}
	}
}

func TestPanelPickerConsumesPressAndRelease(t *testing.T) {
	h := newHarness(t, ggui.Box(), ggui.Sz(1000, 700))
	in := View{picking: true}
	in.paint(h.dst, h.fr)
	for _, f := range []ggui.OverlayInput{
		{Pos: ggui.Pt(20, 20), Down: []ggui.MouseButton{ggui.MouseButtonLeft}},
		{Pos: ggui.Pt(20, 20), Up: []ggui.MouseButton{ggui.MouseButtonLeft}},
	} {
		if !in.Input(f) {
			t.Fatal("picker leaked a click to the inspected app")
		}
	}
	if !in.pinned || in.picking || in.capture {
		t.Fatal("picker did not pin and finish its gesture")
	}
	if in.Input(ggui.OverlayInput{Pos: ggui.Pt(20, 20)}) {
		t.Fatal("keyboard focus blocked subsequent app hover")
	}
}

func TestPanelIndependentPanesAndFilterFocus(t *testing.T) {
	h := newHarness(t, ggui.Box(ggui.Text("Long value")).Pad(12), ggui.Sz(1200, 800))
	in := View{dock: ggui.InspectorBottom}
	in.selectEntry(h.fr, 0)
	in.paint(h.dst, h.fr)
	detailPoint := in.detail.Origin.Add(ggui.Pt(20.0, 60.0))
	in.Input(ggui.OverlayInput{Pos: detailPoint, Wheel: ggui.Pt(0, -2), Text: "unexpected"})
	if in.scroll != 0 || in.detailScroll != 2*inspectWheel || in.filter != "" {
		t.Fatal("details input affected the tree or unfocused filter")
	}
	in.Input(ggui.OverlayInput{Pos: in.filterRect.Origin.Add(ggui.Pt(10.0, 10.0)), Down: []ggui.MouseButton{ggui.MouseButtonLeft}})
	in.Input(ggui.OverlayInput{Pos: ggui.Pt(20, 20), Text: "Long"})
	if in.filter != "Long" {
		t.Fatal("focused filter stopped receiving text when pointer moved")
	}
}

func TestPanelResizesAndClampsPanel(t *testing.T) {
	h := newHarness(t, ggui.Box(), ggui.Sz(1200, 800))
	in := View{dock: ggui.InspectorRight}
	in.paint(h.dst, h.fr)
	in.Input(ggui.OverlayInput{Pos: in.edge.Origin.Add(ggui.Pt(2.0, 40.0)), Down: []ggui.MouseButton{ggui.MouseButtonLeft}})
	in.Input(ggui.OverlayInput{Pos: ggui.Pt(-500, 40), Up: []ggui.MouseButton{ggui.MouseButtonLeft}})
	in.paint(h.dst, h.fr)
	if in.panel.Size.W != 1200*.85 || in.drag != inspectNoDrag {
		t.Fatalf("unbounded panel: %+v", in.panel)
	}
	in.act(inspectChip{act: inspectDockBottom})
	in.paint(h.dst, h.fr)
	in.Input(ggui.OverlayInput{Pos: in.edge.Origin.Add(ggui.Pt(40.0, 2.0)), Down: []ggui.MouseButton{ggui.MouseButtonLeft}})
	in.Input(ggui.OverlayInput{Pos: ggui.Pt(40, 760), Up: []ggui.MouseButton{ggui.MouseButtonLeft}})
	in.paint(h.dst, h.fr)
	if in.panel.Size.H < 180 {
		t.Fatal("panel collapsed below usable height")
	}
}

func TestPanelArrowNavigationRevealsPinnedRows(t *testing.T) {
	h := newHarness(t, ggui.Box(), ggui.Sz(800, 600))
	tr := rows(100, false)
	var in View
	in.paint(h.dst, tr)
	in.focus = true
	in.Input(ggui.OverlayInput{Keys: []ggui.KeyboardKey{ggui.KeyEnd}})
	in.paint(h.dst, tr)
	if !in.pinned || in.sel.rect.Origin.Y != 99 || in.scroll <= 0 {
		t.Fatal("keyboard selection did not scroll into view")
	}
	in.Input(ggui.OverlayInput{Pos: in.tree.Origin.Add(ggui.Pt(20.0, 20.0)), Wheel: ggui.Pt(0, 2)})
	before := in.scroll
	in.paint(h.dst, tr)
	if in.scroll != before {
		t.Fatal("pinned selection undid manual tree scrolling")
	}
}

func TestPanelCopyAndClose(t *testing.T) {
	h := newHarness(t, ggui.Box(ggui.Text("Example")).Pad(12), ggui.Sz(1200, 800))
	mem := h.p.Clipboard()
	in := View{dock: ggui.InspectorBottom}
	in.selectEntry(h.fr, 0)
	in.paint(h.dst, h.fr)
	in.act(inspectChip{act: inspectCopy})
	if !strings.Contains(mem.Read(), "padding: 12 12 12 12") || !strings.Contains(mem.Read(), "width:") {
		t.Fatal("copy missed widget properties")
	}
	in.act(inspectChip{act: inspectClose})
	if !in.Closed() {
		t.Fatal("the close chip did not ask to be turned off")
	}
	in.Reset()
	if in.Closed() || !in.panel.Empty() {
		t.Fatal("Reset did not clear the close request and the panel")
	}
}

// A press held by a widget keeps the release from the panel, so an app
// drag finishes where it began even when it ends over the panel.
func TestPanelLetsApplicationCaptureFinish(t *testing.T) {
	drags := 0
	w := ggui.Pointer(ggui.Box()).OnDrag(func(ggui.PointerEvent) { drags++ })
	p := ggui.NewProbe(w, ggui.Sz(800, 600))
	defer p.Close()
	in := New()
	p.SetOverlay(in)
	p.OnInspect(in.Frame)
	p.Frame()
	p.Press(ggui.Pt(20, 20))
	over := in.panel.Origin.Add(ggui.Pt(30, 100))
	p.Move(over)
	p.Release(over)
	if drags == 0 {
		t.Fatal("the panel interrupted the application's drag")
	}
	if !in.Input(ggui.OverlayInput{Pos: over}) {
		t.Fatal("the panel should take the pointer over itself once the drag is over")
	}
}

func TestPanelResetReleasesWidgetReferences(t *testing.T) {
	h := newHarness(t, ggui.Box(ggui.Text("temporary")), ggui.Sz(800, 600))
	in := New()
	in.selectEntry(h.fr, 0)
	in.paint(h.dst, h.fr)
	in.Reset()
	if in.sel.id != nil || in.frame != nil || in.rows != nil || in.chips != nil || in.collapsed != nil {
		t.Fatal("reset panel retained widget references")
	}
}

func TestPanelCopyResolvesNewSelectionWithoutAnotherPaint(t *testing.T) {
	one, two := ggui.Box(ggui.Text("one")).Pad(11), ggui.Box(ggui.Text("two")).Pad(22)
	h := newHarness(t, ggui.Column(one, two), ggui.Sz(800, 600))
	mem := h.p.Clipboard()
	var in View
	in.selectEntry(h.fr, 1)
	in.paint(h.dst, h.fr)
	if mem.Read() != "" {
		t.Fatal("paint wrote to the clipboard")
	}
	in.selectEntry(h.fr, 3)
	in.act(inspectChip{act: inspectCopy})
	if !strings.Contains(mem.Read(), "padding: 22 22 22 22") || strings.Contains(mem.Read(), "padding: 11 11 11 11") {
		t.Fatal("copy used details of the previous selection")
	}
	mem.Write("unchanged")
	in.frame.Nodes = nil
	in.act(inspectChip{act: inspectCopy})
	if mem.Read() != "unchanged" {
		t.Fatal("copy used a removed widget")
	}
}

func TestPanelFilterScratchDoesNotKeepOldMatches(t *testing.T) {
	in := View{filter: "text"}
	tr := trace(entry("Column", 0, 0, 0, 100, 60), entry("Text", 1, 0, 0, 100, 20))
	if got := in.visible(tr); len(got) != 2 {
		t.Fatal("initial match missing")
	}
	tr.Nodes[1].Name = "Box"
	if got := in.visible(tr); len(got) != 0 {
		t.Fatalf("old filter matches survived buffer reuse: %v", got)
	}
	in.filter = "column"
	if got := in.visible(trace(tr.Nodes[:1]...)); !slices.Equal(got, []int{0}) {
		t.Fatalf("shrinking the trace retained old ancestors: %v", got)
	}
	in.filter = ""
	if got := in.visible(tr); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("clearing the filter lost rows: %v", got)
	}
}

// Installed through the app's registration, the panel paints the frame the
// probe published and takes the pointer over itself.
func TestPanelRunsAsTheProbesOverlay(t *testing.T) {
	p := ggui.NewProbe(ggui.Box(ggui.Text("hello")), ggui.Sz(800, 600))
	defer p.Close()
	in := New()
	p.SetOverlay(in)
	p.OnInspect(in.Frame)
	p.Frame()
	if in.panel.Empty() || len(in.rows) != 2 {
		t.Fatalf("panel = %+v with %d rows, want the two painted widgets", in.panel, len(in.rows))
	}
	p.Move(in.filterRect.Origin.Add(ggui.Pt(10, 10)))
	if p.Cursor() != ggui.CursorShapeText {
		t.Fatalf("cursor over the filter field = %v, want the text cursor the panel asks for", p.Cursor())
	}
}

func TestFrameRateCountsTheLastSecond(t *testing.T) {
	var f frameRate
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 60 {
		f.count(t0.Add(time.Duration(i) * time.Second / 60))
	}
	if got := f.count(t0.Add(time.Second)); got != 60 {
		t.Fatalf("60 frames a second read %d; want 60", got)
	}
	if got := f.count(t0.Add(5 * time.Second)); got != 1 {
		t.Fatalf("a frame after an idle pause read %d; want 1", got)
	}
}
