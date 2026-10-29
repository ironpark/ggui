package ggui

import (
	"maps"
	"slices"
	"testing"

	"github.com/ironpark/ggui/internal/reactive"
)

// estimatedRows is a list of rows from 10 to 50 tall, by ID, virtualized
// on an estimate of 30, whose rows note where they are painted.
func estimatedRows(t *testing.T, items *StateValue[[]todo], painted map[int]Rect, builds *int) *EachWidget[todo, int] {
	t.Helper()
	var f *EachWidget[todo, int]
	dispose := reactive.Observe(func() {
		f = EachKeyed(items, func(t todo) int { return t.ID }, func(row EachItem[todo]) Widget {
			id := row.Item.ID
			*builds++
			return FromFuncs(
				func(c Constraints, _ Env) Size { return c.Constrain(Sz(50, rowHeight(id))) },
				func(_ *Canvas, r Rect) { painted[id] = r },
			)
		}).EstimatedExtent(30)
	})
	t.Cleanup(dispose)
	return f
}

func rowHeight(id int) float64 { return float64(10 + (id%5+5)%5*10) }

func todos(from, to int) []todo {
	var list []todo
	for i := from; i < to; i++ {
		list = append(list, todo{i, "x"})
	}
	return list
}

// frame lays out and paints s in a 100 by 100 window, noting afresh where
// each row is painted.
func frame(s *ScrollWidget, painted map[int]Rect) {
	clear(painted)
	size := s.Layout(Tight(Sz(100, 100)), Env{})
	s.Paint(nil, Rct(Pt(0, 0), size))
}

func TestEstimatedRowsBuildOnlyNearTheWindow(t *testing.T) {
	t.Parallel()
	items := State(todos(0, 1000))
	painted, builds := map[int]Rect{}, 0
	f := estimatedRows(t, items, painted, &builds)
	s := Scroll(f)
	frame(s, painted)
	// Rows 0 to 4 are 10 to 50 tall: 150 of them, the window and half again.
	if builds != 5 || len(f.entries) != 5 {
		t.Fatalf("built %d rows, %d kept; want 5 of 1000", builds, len(f.entries))
	}
	if painted[2] != Rct(Pt(0, 30), Sz(50, 30)) {
		t.Fatalf("row 2 painted at %+v, want (0,30) 50x30, below rows of 10 and 20", painted[2])
	}
	// Those laid out at their size, the rest at the estimate.
	if want := 150 + 995*30.0; s.childSize.H != want {
		t.Fatalf("content height %v, want %v", s.childSize.H, want)
	}
}

func TestEstimatedRowsScrollByWhatTheReaderScrolls(t *testing.T) {
	t.Parallel()
	items := State(todos(0, 1000))
	painted, builds := map[int]Rect{}, 0
	f := estimatedRows(t, items, painted, &builds)
	offset := State(20000.0)
	s := Scroll(f).BindOffset(offset)
	frame(s, painted)
	frame(s, painted)
	// Scrolling up through rows laid out for the first time, whose sizes
	// are not the estimate: a row in view before and after a step moves by
	// the step, and the rows painted meet.
	for range 200 {
		before := maps.Clone(painted)
		offset.Set(Peek(offset) - 37)
		reactive.Flush()
		frame(s, painted)
		ids := sortedIDs(painted)
		for i, id := range ids[1:] {
			prev := painted[ids[i]]
			if got := painted[id].Origin.Y; got != prev.Origin.Y+prev.Size.H {
				t.Fatalf("at offset %v row %d starts at %v, want %v, where row %d ends", Peek(offset), id, got, prev.Origin.Y+prev.Size.H, ids[i])
			}
		}
		moved := false
		for id, r := range painted {
			if b, ok := before[id]; ok && r.Origin.Y >= 0 && r.Origin.Y < 100 {
				if r.Origin.Y-b.Origin.Y != 37 && Peek(offset) > 0 {
					t.Fatalf("at offset %v row %d moved %v on screen, want 37", Peek(offset), id, r.Origin.Y-b.Origin.Y)
				}
				moved = true
			}
		}
		if !moved {
			t.Fatalf("at offset %v no row stayed in view across a step of 37", Peek(offset))
		}
	}
}

func TestEstimatedRowsKeepTheirPlaceAsRowsComeInAbove(t *testing.T) {
	t.Parallel()
	items := State(todos(0, 1000))
	painted, builds := map[int]Rect{}, 0
	f := estimatedRows(t, items, painted, &builds)
	offset := State(3000.0)
	s := Scroll(f).BindOffset(offset)
	frame(s, painted)
	frame(s, painted)
	before := maps.Clone(painted)
	// Older rows read in above, as a conversation pages back.
	items.Set(append(todos(-40, 0), Peek(items)...))
	reactive.Flush()
	frame(s, painted)
	kept := 0
	for id, r := range before {
		got, ok := painted[id]
		if !ok || r.Origin.Y+r.Size.H <= 0 || r.Origin.Y >= 100 {
			continue
		}
		if got != r {
			t.Fatalf("row %d painted at %+v once rows came in above, want %+v where it was", id, got, r)
		}
		kept++
	}
	if kept == 0 {
		t.Fatal("no row in view stayed in view once rows came in above")
	}
}

func TestEstimatedRowsFollowWhereTheListStarts(t *testing.T) {
	t.Parallel()
	items := State(todos(0, 1000))
	painted, builds := map[int]Rect{}, 0
	f := estimatedRows(t, items, painted, &builds)
	var head Rect
	offset := State(5000.0)
	s := Scroll(Column(probe(100, 400, &head), f)).BindOffset(offset)
	frame(s, painted)
	frame(s, painted)
	// The list starts 400 into the content: the window, 5000 in, shows
	// rows from 4600 into the list.
	ids := sortedIDs(painted)
	if len(ids) == 0 {
		t.Fatal("no rows painted under a header")
	}
	var covered bool
	for _, id := range ids {
		r := painted[id]
		if r.Origin.Y <= 0 && r.Origin.Y+r.Size.H > 0 {
			covered = true
		}
	}
	if !covered {
		t.Fatalf("rows %v painted at %v.., none at the window's top", ids, painted[ids[0]])
	}
}

func sortedIDs(m map[int]Rect) []int {
	ids := make([]int, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
