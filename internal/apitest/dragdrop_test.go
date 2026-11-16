package apitest_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/widget"
)

// counting is a leaf that counts its layouts.
type counting struct{ layouts int }

func (c *counting) Layout(ggui.Constraints, ggui.Env) ggui.Size { c.layouts++; return ggui.Sz(10, 10) }
func (c *counting) Paint(*ggui.Canvas, ggui.Rect)               {}

func TestDragFromASourceDropsOnAZoneThatTakesIt(t *testing.T) {
	t.Parallel()
	taps, ended := 0, []bool{}
	var got []widget.Dropped[int]
	var hovers []bool
	source := widget.DragSource(widget.Tap(widget.Box().Size(100, 40), func() { taps++ }), 7).OnDragEnd(func(d bool) { ended = append(ended, d) })
	strings := widget.DropZone(widget.Box().Size(100, 40), func(widget.Dropped[string]) { t.Fatal("a zone for strings took an int") })
	zone := widget.DropZone(widget.Box().Size(100, 40), func(d widget.Dropped[int]) { got = append(got, d) }).OnHover(func(over bool) { hovers = append(hovers, over) })
	p := ggui.NewProbe(widget.Column(source, widget.Stack(zone, strings)), ggui.Sz(100, 120))
	defer p.Close()
	p.Frame()

	p.Click(ggui.Pt(50, 20))
	if taps != 1 || len(ended) != 0 {
		t.Fatalf("a click on the source: %d taps, %d drags; want the child's tap alone", taps, len(ended))
	}
	p.Press(ggui.Pt(50, 20))
	p.Move(ggui.Pt(52, 21)) // inside the threshold: still a press
	if source.Dragging() {
		t.Fatal("a drag started before the pointer moved far enough")
	}
	p.Move(ggui.Pt(50, 50))
	p.Move(ggui.Pt(50, 70))
	if !source.Dragging() || !zone.Over() {
		t.Fatalf("over the zone: dragging %v, zone over %v", source.Dragging(), zone.Over())
	}
	p.Release(ggui.Pt(50, 70))
	if taps != 1 {
		t.Fatal("the drag ended in a tap on the child")
	}
	if len(got) != 1 || got[0].Value != 7 || got[0].Pos != ggui.Pt(50, 30) || got[0].Size != ggui.Sz(100, 40) || !slices.Equal(ended, []bool{true}) {
		t.Fatalf("dropped %+v, ended %v; want 7 at (50,30) in the zone, ended with a drop", got, ended)
	}
	if !slices.Equal(hovers, []bool{true, false}) || source.Dragging() {
		t.Fatalf("hover %v, dragging after the drop %v", hovers, source.Dragging())
	}

	p.Press(ggui.Pt(50, 20))
	p.Move(ggui.Pt(50, 70))
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	p.Release(ggui.Pt(50, 70))
	if len(got) != 1 || !slices.Equal(ended, []bool{true, false}) || zone.Over() {
		t.Fatalf("Escape: dropped %d times, ended %v, zone over %v; want the drag cancelled", len(got), ended, zone.Over())
	}
}

func TestDropZoneAcceptNarrowsWhatItTakes(t *testing.T) {
	t.Parallel()
	dropped := 0
	zone := widget.DropZone(widget.Box().Size(100, 40), func(widget.Dropped[int]) { dropped++ }).Accept(func(v int) bool { return v > 10 })
	p := ggui.NewProbe(widget.Column(widget.DragSource(widget.Box().Size(100, 40), 7), zone), ggui.Sz(100, 80))
	defer p.Close()
	p.Frame()
	p.Press(ggui.Pt(50, 20))
	p.Move(ggui.Pt(50, 60))
	p.Release(ggui.Pt(50, 60))
	if dropped != 0 || zone.Over() {
		t.Fatal("a zone took a value its Accept refused")
	}
}

func TestMoveReordersAsADragWould(t *testing.T) {
	t.Parallel()
	in := []string{"a", "b", "c", "d"}
	for _, c := range []struct {
		from, to int
		want     []string
	}{
		{0, 2, []string{"b", "a", "c", "d"}},
		{0, 4, []string{"b", "c", "d", "a"}},
		{3, 0, []string{"d", "a", "b", "c"}},
		{1, 1, in}, {1, 2, in}, {-1, 0, in}, {0, 5, in},
	} {
		if got := widget.MoveItem(in, c.from, c.to); !slices.Equal(got, c.want) {
			t.Errorf("Move(%v, %d, %d) = %v, want %v", in, c.from, c.to, got, c.want)
		}
	}
	if !slices.Equal(in, []string{"a", "b", "c", "d"}) {
		t.Fatal("Move changed its argument")
	}
}

func TestAListReordersByDraggingItsRows(t *testing.T) {
	t.Parallel()
	type item struct {
		ID   int
		Name string
	}
	items := ggui.State([]item{{1, "a"}, {2, "b"}, {3, "c"}})
	list := widget.EachKeyed(items, func(it item) int { return it.ID }, func(row widget.EachItem[item]) ggui.Widget {
		id := ggui.Untrack(row.Value.Get).ID
		return widget.DropZone(widget.DragSource(widget.Box().Size(100, 30), id), func(d widget.Dropped[int]) {
			list := ggui.Untrack(items.Get)
			from := slices.IndexFunc(list, func(it item) bool { return it.ID == d.Value })
			to := ggui.Untrack(row.Index.Get)
			if !d.Before() {
				to++
			}
			items.Set(widget.MoveItem(list, from, to))
		})
	}).Gap(0)
	p := ggui.NewProbe(list, ggui.Sz(100, 90))
	defer p.Close()
	p.Frame()
	drag := func(from, to ggui.Point) {
		p.Press(from)
		p.Move(ggui.Pt(from.X, from.Y+10))
		p.Move(to)
		p.Release(to)
	}
	names := func() string {
		s := ""
		for _, it := range ggui.Untrack(items.Get) {
			s += it.Name
		}
		return s
	}
	drag(ggui.Pt(50, 15), ggui.Pt(50, 80)) // a onto the lower half of c
	if got := names(); got != "bca" {
		t.Fatalf("after dragging a below c: %q, want bca", got)
	}
	drag(ggui.Pt(50, 75), ggui.Pt(50, 5)) // a onto the upper half of b
	if got := names(); got != "abc" {
		t.Fatalf("after dragging a above b: %q, want abc", got)
	}
}

func TestDragPreviewIsLaidOutOnlyWhileDragging(t *testing.T) {
	t.Parallel()
	preview := &counting{}
	source := widget.DragSource(widget.Box().Size(100, 40), 7).Preview(preview)
	p := ggui.NewProbe(widget.Column(source, widget.Box().Size(100, 40)), ggui.Sz(100, 80))
	defer p.Close()
	p.Frame()
	if preview.layouts != 0 {
		t.Fatalf("the preview was laid out %d times before a drag", preview.layouts)
	}
	p.Press(ggui.Pt(50, 20))
	p.Move(ggui.Pt(50, 60))
	if !source.Dragging() || preview.layouts == 0 {
		t.Fatalf("mid-drag: dragging %v, preview laid out %d times", source.Dragging(), preview.layouts)
	}
	p.Release(ggui.Pt(50, 60))
}

// A zone hears where a drag it accepts is as the drag moves over it, so it
// can show where the value would land.
func TestDropZoneFollowsADragOverIt(t *testing.T) {
	t.Parallel()
	var at []ggui.Point
	source := widget.DragSource(widget.Box().Size(100, 40), "tab")
	zone := widget.DropZone(widget.Box().Size(100, 40), func(widget.Dropped[string]) {}).OnOver(func(d widget.Dropped[string]) {
		if d.Value != "tab" || d.Size != ggui.Sz(100, 40) {
			t.Fatalf("over with %+v", d)
		}
		at = append(at, d.Pos)
	})
	p := ggui.NewProbe(widget.Column(source, zone), ggui.Sz(100, 80))
	defer p.Close()
	p.Frame()
	p.Press(ggui.Pt(50, 20))
	p.Move(ggui.Pt(50, 30)) // a drag, still over the source
	p.Move(ggui.Pt(20, 50))
	p.Move(ggui.Pt(80, 60))
	p.Release(ggui.Pt(80, 60))
	if !slices.Equal(at, []ggui.Point{ggui.Pt(20, 10), ggui.Pt(80, 20)}) {
		t.Fatalf("followed the drag at %v, want each move over the zone in its own coordinates", at)
	}
}

func TestDropGoesToTheZoneUnderTheCursor(t *testing.T) {
	t.Parallel()
	var zone, host []string
	build := func() ggui.Widget {
		target := widget.Pointer(widget.Box(widget.Text("drop here")).Size(200, 100)).OnDrop(func(ev ggui.DropEvent) {
			for _, f := range ev.Files {
				zone = append(zone, f.Name)
			}
		})
		return widget.Column(target, widget.Box(widget.Text("elsewhere")).Size(200, 100))
	}
	p := ggui.ProbeBuilder(build, ggui.Sz(400, 400))
	defer p.Close()
	p.OnDrop(func(ev ggui.DropEvent) {
		for _, f := range ev.Files {
			host = append(host, f.Name)
		}
	})
	fsys := fstest.MapFS{
		"a.txt": {Data: []byte("hello")},
		"b.png": {Data: []byte{1, 2, 3}},
	}

	p.Drop(ggui.Pt(50, 50), fsys)
	if len(zone) != 2 || zone[0] != "a.txt" || zone[1] != "b.png" {
		t.Fatalf("zone got %q", zone)
	}
	if len(host) != 0 {
		t.Fatalf("host handler ran for a drop the zone took: %q", host)
	}

	p.Drop(ggui.Pt(50, 150), fsys)
	if len(host) != 2 {
		t.Fatalf("host got %q, want both files", host)
	}
	if len(zone) != 2 {
		t.Fatalf("zone took a drop outside it: %q", zone)
	}
}

func TestDroppedFileOpensWithoutAPath(t *testing.T) {
	t.Parallel()
	var got string
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return widget.Pointer(widget.Box(widget.Text("zone")).Size(100, 100)).OnDrop(func(ev ggui.DropEvent) {
			f, err := ev.Files[0].Open()
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			b, _ := io.ReadAll(f)
			got = string(b)
			if ev.Files[0].Path != "" {
				t.Fatalf("in-memory drop has a Path %q", ev.Files[0].Path)
			}
		})
	}, ggui.Sz(200, 200))
	defer p.Close()
	p.Drop(ggui.Pt(10, 10), fstest.MapFS{"note.txt": {Data: []byte("contents")}})
	if got != "contents" {
		t.Fatalf("read %q", got)
	}
}

func TestDropPathsCarriesRealFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(file, []byte("on disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	var ev ggui.DropEvent
	p := ggui.ProbeBuilder(func() ggui.Widget { return widget.Text("nothing accepts") }, ggui.Sz(200, 200))
	defer p.Close()
	p.OnDrop(func(e ggui.DropEvent) { ev = e })

	p.DropPaths(ggui.Pt(10, 10), file, sub)
	if len(ev.Files) != 2 {
		t.Fatalf("files = %+v", ev.Files)
	}
	if ev.Files[0].Name != "real.txt" || ev.Files[0].Path != file || ev.Files[0].Dir {
		t.Fatalf("file = %+v", ev.Files[0])
	}
	if ev.Files[1].Name != "sub" || !ev.Files[1].Dir {
		t.Fatalf("dir = %+v", ev.Files[1])
	}
	if paths := ev.Paths(); len(paths) != 2 || paths[0] != file {
		t.Fatalf("Paths = %q", paths)
	}
	f, err := ev.Files[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if b, _ := io.ReadAll(f); string(b) != "on disk" {
		t.Fatalf("read %q", b)
	}
}

func TestZoneWithoutOnDropLetsTheDropFallThrough(t *testing.T) {
	t.Parallel()
	var outer, host int
	p := ggui.ProbeBuilder(func() ggui.Widget {
		inner := widget.Pointer(widget.Box(widget.Text("tap only")).Size(50, 50)).OnTap(func() {})
		return widget.Pointer(widget.Box(inner).Size(200, 200)).OnDrop(func(ggui.DropEvent) { outer++ })
	}, ggui.Sz(200, 200))
	defer p.Close()
	p.OnDrop(func(ggui.DropEvent) { host++ })
	p.Drop(ggui.Pt(10, 10), fstest.MapFS{"x": {}})
	if outer != 1 || host != 0 {
		t.Fatalf("outer=%d host=%d; want the enclosing zone to take it", outer, host)
	}
}

func TestEmptyDropIsNotDispatched(t *testing.T) {
	t.Parallel()
	var host int
	p := ggui.ProbeBuilder(func() ggui.Widget { return widget.Text("x") }, ggui.Sz(100, 100))
	defer p.Close()
	p.OnDrop(func(ggui.DropEvent) { host++ })
	p.Drop(ggui.Pt(1, 1), fstest.MapFS{})
	if host != 0 {
		t.Fatal("a drop of nothing reached the host handler")
	}
}

func TestDragHoverFollowsTheZoneUnderTheCursor(t *testing.T) {
	t.Parallel()
	var top, bottom []bool
	build := func() ggui.Widget {
		return widget.Column(
			widget.Pointer(widget.Box(widget.Text("top")).Size(200, 100)).OnDrop(func(ggui.DropEvent) {}).OnDropHover(func(over bool) { top = append(top, over) }),
			widget.Pointer(widget.Box(widget.Text("bottom")).Size(200, 100)).OnDrop(func(ggui.DropEvent) {}).OnDropHover(func(over bool) { bottom = append(bottom, over) }),
			widget.Pointer(widget.Box(widget.Text("plain")).Size(200, 100)).OnDrop(func(ggui.DropEvent) {}),
		)
	}
	p := ggui.ProbeBuilder(build, ggui.Sz(400, 400))
	defer p.Close()

	p.DragOver(ggui.Pt(50, 50))
	p.DragOver(ggui.Pt(60, 60))
	if len(top) != 1 || !top[0] {
		t.Fatalf("top zone heard %v after two frames over it, want one enter", top)
	}
	p.DragOver(ggui.Pt(50, 150))
	if len(top) != 2 || top[1] || len(bottom) != 1 || !bottom[0] {
		t.Fatalf("moving to the bottom zone: top heard %v, bottom heard %v", top, bottom)
	}
	p.DragOver(ggui.Pt(50, 250))
	if len(bottom) != 2 || bottom[1] {
		t.Fatalf("moving over a zone without OnDropHover: bottom heard %v, want an exit", bottom)
	}
	p.DragOver(ggui.Pt(50, 50))
	p.Drop(ggui.Pt(50, 50), fstest.MapFS{"a.txt": {Data: []byte("hi")}})
	if len(top) != 4 || !top[2] || top[3] {
		t.Fatalf("a drop should end the hover: top heard %v", top)
	}
	p.DragOver(ggui.Pt(50, 50))
	p.DragEnd()
	if len(top) != 6 || top[5] {
		t.Fatalf("an abandoned drag should end the hover: top heard %v", top)
	}
}

// reorderFixture is a Reorderable of 30-high rows whose painted rects land
// in painted, keyed by item.
func reorderFixture(items *ggui.StateValue[[]string], painted map[string]ggui.Rect, handle bool) *widget.ReorderableWidget[string, string] {
	return widget.Reorderable(items, func(s string) string { return s }, func(it widget.EachItem[string]) ggui.Widget {
		v := it.Item
		row := ggui.FromFuncs(
			func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(80, 30)) },
			func(_ *ggui.Canvas, r ggui.Rect) { painted[v] = r },
		)
		if !handle {
			return row
		}
		return widget.Row(widget.ReorderHandle(widget.Box().Size(20, 30)).Name("Move "+v), row)
	})
}

func TestReorderableDragMovesARowAndOpensAGap(t *testing.T) {
	t.Parallel()
	items := ggui.State([]string{"a", "b", "c", "d"})
	painted := map[string]ggui.Rect{}
	var moves [][2]int
	l := reorderFixture(items, painted, false).OnReorder(func(from, to int) { moves = append(moves, [2]int{from, to}) })
	p := ggui.NewProbe(l, ggui.Sz(100, 200))
	defer p.Close()
	p.Frame()

	p.Press(ggui.Pt(40, 15))
	p.Move(ggui.Pt(40, 25))
	p.Move(ggui.Pt(40, 90)) // the row's middle is now below c's
	p.Frame()
	p.Advance(time.Second)
	if !l.Dragging() {
		t.Fatal("dragging a row did not start a drag")
	}
	if painted["b"].Origin.Y != 0 || painted["c"].Origin.Y != 30 || painted["d"].Origin.Y != 90 {
		t.Fatalf("mid-drag b %v c %v d %v; want b and c moved up into the gap", painted["b"], painted["c"], painted["d"])
	}
	p.Release(ggui.Pt(40, 90))
	if got := ggui.Peek(items); !slices.Equal(got, []string{"b", "c", "a", "d"}) || !slices.Equal(moves, [][2]int{{0, 3}}) {
		t.Fatalf("after the drop items %v, OnReorder %v; want [b c a d] and (0, 3)", got, moves)
	}
	p.Frame() // the rows start easing from where they were drawn
	p.Advance(time.Second)
	if painted["a"].Origin.Y != 60 || l.Dragging() {
		t.Fatalf("a settled at %v, dragging %v", painted["a"], l.Dragging())
	}

	// Escape puts the row back.
	p.Press(ggui.Pt(40, 15))
	p.Move(ggui.Pt(40, 100))
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	p.Release(ggui.Pt(40, 100))
	if got := ggui.Peek(items); !slices.Equal(got, []string{"b", "c", "a", "d"}) || len(moves) != 1 {
		t.Fatalf("Escape still moved: %v", got)
	}
}

func TestReorderableHandlesAndKeyboard(t *testing.T) {
	t.Parallel()
	items := ggui.State([]string{"a", "b", "c"})
	painted := map[string]ggui.Rect{}
	l := reorderFixture(items, painted, true).Handles()
	p := ggui.NewProbe(l, ggui.Sz(100, 200))
	defer p.Close()
	p.Frame()

	p.Press(ggui.Pt(60, 15)) // the row's body, not its handle
	p.Move(ggui.Pt(60, 80))
	if l.Dragging() {
		t.Fatal("with Handles the row's body started a drag")
	}
	p.Release(ggui.Pt(60, 80))

	p.Press(ggui.Pt(10, 15))
	p.Move(ggui.Pt(10, 80))
	if !l.Dragging() {
		t.Fatal("the handle did not start a drag")
	}
	p.Release(ggui.Pt(10, 80))
	if got := ggui.Peek(items); !slices.Equal(got, []string{"b", "c", "a"}) {
		t.Fatalf("dropped by the handle: %v, want [b c a]", got)
	}

	p.Type(ggui.Mods{}, ggui.KeyTab) // the first handle, now b's
	p.Key("alt+down")
	if got := ggui.Peek(items); !slices.Equal(got, []string{"c", "b", "a"}) {
		t.Fatalf("Alt+Down: %v, want [c b a]", got)
	}
	p.Key("alt+down")
	if got := ggui.Peek(items); !slices.Equal(got, []string{"c", "a", "b"}) {
		t.Fatalf("Alt+Down again, following the row: %v, want [c a b]", got)
	}
	p.Key("alt+down") // already last
	p.Key("alt+up")
	if got := ggui.Peek(items); !slices.Equal(got, []string{"c", "b", "a"}) {
		t.Fatalf("Alt+Up: %v, want [c b a]", got)
	}

	l.Disabled(true)
	p.Key("alt+up")
	if got := ggui.Peek(items); !slices.Equal(got, []string{"c", "b", "a"}) {
		t.Fatalf("disabled list moved: %v", got)
	}
}

func TestReorderableOffersMoveActions(t *testing.T) {
	t.Parallel()
	items := ggui.State([]string{"a", "b", "c"})
	p := ggui.NewProbe(reorderFixture(items, map[string]ggui.Rect{}, true), ggui.Sz(100, 200))
	defer p.Close()
	tree := p.Semantics()
	var listItems []ggui.SemNode
	for _, n := range tree.Nodes(ggui.RoleListItem) {
		listItems = append(listItems, n)
	}
	if len(listItems) != 3 || listItems[0].Now != 1 || listItems[0].Max != 3 || listItems[0].Actions&ggui.ActionIncrement == 0 {
		t.Fatalf("list items %+v; want three that know their place and can move", listItems)
	}
	p.Perform(listItems[0].ID, ggui.Action{Kind: ggui.ActionIncrement})
	if got := ggui.Peek(items); !slices.Equal(got, []string{"b", "a", "c"}) {
		t.Fatalf("increment on the first row: %v, want [b a c]", got)
	}
	handle, ok := p.Semantics().Find(ggui.RoleButton, "Move c")
	if !ok || handle.Actions&ggui.ActionDecrement == 0 {
		t.Fatalf("handle %+v ok %v", handle, ok)
	}
	p.Perform(handle.ID, ggui.Action{Kind: ggui.ActionDecrement})
	if got := ggui.Peek(items); !slices.Equal(got, []string{"b", "c", "a"}) {
		t.Fatalf("decrement on c's handle: %v, want [b c a]", got)
	}
}

func TestReorderableScrollsItsScrollNearTheEdge(t *testing.T) {
	t.Parallel()
	list := make([]string, 20)
	for i := range list {
		list[i] = string(rune('a' + i))
	}
	items := ggui.State(list)
	offset := ggui.State(0.0)
	p := ggui.NewProbe(widget.Scroll(reorderFixture(items, map[string]ggui.Rect{}, false)).BindOffset(offset), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()
	p.Press(ggui.Pt(40, 15))
	p.Move(ggui.Pt(40, 30))
	p.Move(ggui.Pt(40, 98))
	for range 10 {
		p.Advance(50 * time.Millisecond)
	}
	if ggui.Peek(offset) <= 0 {
		t.Fatalf("holding a row at the bottom edge left the offset at %v", ggui.Peek(offset))
	}
	p.Release(ggui.Pt(40, 98))
	if got := ggui.Peek(items); got[0] == "a" {
		t.Fatalf("the row dropped after scrolling stayed first: %v", got)
	}
}

func TestReorderableKeysIgnoreFocusBehindAScrolledOutRow(t *testing.T) {
	t.Parallel()
	focusable := ggui.State("")
	items := ggui.State([]string{"a", "b", "c", "d", "e", "f", "g", "h"})
	offset := ggui.State(60.0)
	// The field sits above the Scroll where row b, scrolled out of the
	// window, would be drawn.
	p := ggui.NewProbe(widget.Column(
		widget.Box(widget.TextInput(focusable)).Size(100, 30),
		widget.Box(widget.Scroll(reorderFixture(items, map[string]ggui.Rect{}, true)).BindOffset(offset)).Size(100, 60),
	), ggui.Sz(100, 200))
	defer p.Close()
	p.Frame()
	p.Click(ggui.Pt(50, 15))
	p.Frame()
	p.Key("alt+down")
	if got := ggui.Peek(items); !slices.Equal(got, []string{"a", "b", "c", "d", "e", "f", "g", "h"}) {
		t.Fatalf("Alt+Down with focus outside the list moved a hidden row: %v", got)
	}
}

func TestReorderableRowsAnswerScrollToKey(t *testing.T) {
	t.Parallel()
	items := ggui.State([]string{"a", "b", "c", "d", "e", "f", "g", "h"})
	var sc widget.ScrollController
	p := ggui.NewProbe(widget.Scroll(reorderFixture(items, map[string]ggui.Rect{}, false)).Controller(&sc), ggui.Sz(100, 60))
	defer p.Close()
	p.Frame()
	sc.JumpToKey("f")
	p.Frame()
	p.Frame()
	if sc.Offset() != 150 {
		t.Fatalf("JumpToKey(f) reached %v, want 150", sc.Offset())
	}
}

// A frame of a Reorderable list at rest: each row painted as a list item.
func BenchmarkReorderableFrame(b *testing.B) {
	names := make([]string, 300)
	for i := range names {
		names[i] = fmt.Sprint("row ", i)
	}
	items := ggui.State(names)
	p := ggui.NewProbe(reorderFixture(items, map[string]ggui.Rect{}, false), ggui.Sz(200, 400))
	defer p.Close()
	p.Frame()
	b.ReportAllocs()
	for b.Loop() {
		p.Frame()
	}
}
