package widget

import (
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/pkg/virtual"
)

// layoutFor lays a EachKeyed out with room for everything, which is when its
// children are built.
func layoutFor(w ggui.Widget) { w.Layout(ggui.Loose(ggui.Sz(500, ggui.Unbounded)), ggui.Env{}) }

type todo struct {
	ID   int
	Name string
}

func TestForReusesChildrenByKey(t *testing.T) {
	t.Parallel()
	items := ggui.State([]todo{{1, "a"}, {2, "b"}})
	setups := 0
	var f ggui.Widget
	shown := map[ggui.Widget]*TextWidget{} // each row's text, by the row's widget
	dispose := reactive.Observe(func() {
		f = EachKeyed(items, func(t todo) int { return t.ID }, func(rowItem EachItem[todo]) ggui.Widget {
			it := rowItem.Value
			var row ggui.Widget
			row = ggui.Component(func() ggui.Widget {
				setups++
				return ggui.Reactive(func() ggui.Widget { shown[row] = Text(it.Get().Name); return shown[row] })
			})
			return row
		})
	})
	defer dispose()
	layoutFor(f)
	items.Set([]todo{{2, "b"}, {1, "a"}, {3, "c"}})
	reactive.Flush()
	layoutFor(f)
	if setups != 3 {
		t.Fatalf("setups = %d after reorder plus one new item, want 3", setups)
	}
	names := func() []string {
		var out []string
		for _, w := range f.(*EachWidget[todo, int]).children {
			out = append(out, shown[w].value)
		}
		return out
	}
	if got := names(); len(got) != 3 || got[0] != "b" || got[1] != "a" || got[2] != "c" {
		t.Fatalf("order = %v, want [b a c]", got)
	}
}

func TestForUpdatesItemSignalInPlace(t *testing.T) {
	t.Parallel()
	items := ggui.State([]todo{{1, "a"}})
	builds := 0
	var f ggui.Widget
	var shown *TextWidget // the text the row built last
	dispose := reactive.Observe(func() {
		f = EachKeyed(items, func(t todo) int { return t.ID }, func(rowItem EachItem[todo]) ggui.Widget {
			it := rowItem.Value
			return ggui.Reactive(func() ggui.Widget { builds++; shown = Text(it.Get().Name); return shown })
		})
	})
	defer dispose()
	layoutFor(f)
	items.Set([]todo{{1, "renamed"}})
	reactive.Flush()
	layoutFor(f)
	if len(f.(*EachWidget[todo, int]).children) != 1 {
		t.Fatalf("%d rows, want the one kept", len(f.(*EachWidget[todo, int]).children))
	}
	got := shown.value
	if got != "renamed" || builds != 2 {
		t.Fatalf("child shows %q after %d builds, want renamed after 2", got, builds)
	}
}

func TestForDisposesRemovedAndAllOnParentRebuild(t *testing.T) {
	t.Parallel()
	items := ggui.State([]todo{{1, "a"}, {2, "b"}})
	parentDep := ggui.State(0)
	cleanups := 0
	var f ggui.Widget
	dispose := reactive.Observe(func() {
		parentDep.Get()
		f = EachKeyed(items, func(t todo) int { return t.ID }, func(rowItem EachItem[todo]) ggui.Widget {
			ggui.OnCleanup(func() { cleanups++ })
			return Box()
		})
	})
	defer dispose()
	layoutFor(f)
	items.Set([]todo{{1, "a"}})
	reactive.Flush()
	if cleanups != 1 {
		t.Fatalf("cleanups = %d after removing one item, want 1", cleanups)
	}
	parentDep.Set(1)
	reactive.Flush()
	if cleanups != 2 {
		t.Fatalf("cleanups = %d after the parent rebuilt, want 2 (the survivor disposed)", cleanups)
	}
}

func TestForDoesNotRebuildOnParentSignals(t *testing.T) {
	t.Parallel()
	items := ggui.State([]todo{{1, "a"}})
	setups := 0
	var f ggui.Widget
	dispose := reactive.Observe(func() {
		f = EachKeyed(items, func(t todo) int { return t.ID }, func(rowItem EachItem[todo]) ggui.Widget {
			setups++
			return Box()
		})
	})
	defer dispose()
	layoutFor(f)
	items.Set([]todo{{1, "a"}}) // equal slice contents, new slice: notifies
	reactive.Flush()
	layoutFor(f)
	if setups != 1 {
		t.Fatalf("setups = %d, want 1: same key must reuse the child", setups)
	}
}

func TestForRejectsDuplicateKeys(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate keys did not panic")
		}
	}()
	EachKeyed(ggui.State([]todo{{1, "a"}, {1, "b"}}), func(t todo) int { return t.ID }, func(EachItem[todo]) ggui.Widget { return Box() }).Layout(ggui.Loose(ggui.Sz(100, 100)), ggui.Env{})
}

func TestRootOutlivesOwnerRerunsUntilDisposed(t *testing.T) {
	t.Parallel()
	dep, inner := ggui.State(0), ggui.State(0)
	innerRuns := 0
	var disposeRoot func()
	dispose := reactive.Observe(func() {
		dep.Get()
		if disposeRoot == nil {
			disposeRoot = ggui.Root(func() {
				reactive.Observe(func() { inner.Get(); innerRuns++ })
			})
		}
	})
	defer dispose()
	dep.Set(1) // owner reruns; its persistent Root remains alive
	reactive.Flush()
	inner.Set(1)
	reactive.Flush()
	if innerRuns != 2 {
		t.Fatalf("innerRuns = %d, want 2: Root survives owner reruns", innerRuns)
	}
}

func TestForWithItemExtentBuildsOnlyTheViewport(t *testing.T) {
	t.Parallel()
	var list []todo
	for i := range 1000 {
		list = append(list, todo{i, "x"})
	}
	items := ggui.State(list)
	builds := 0
	var f *EachWidget[todo, int]
	rects := make([]ggui.Rect, len(list)) // where each row was painted, by ID
	dispose := reactive.Observe(func() {
		f = EachKeyed(items, func(t todo) int { return t.ID }, func(rowItem EachItem[todo]) ggui.Widget {
			it := rowItem.Value
			builds++
			return probe(50, 20, &rects[it.Get().ID])
		}).ItemExtent(20).Gap(4)
	})
	defer dispose()
	offset := ggui.State(0.0)
	s := Scroll(f).BindOffset(offset)
	size := s.Layout(ggui.Tight(ggui.Sz(100, 100)), ggui.Env{})
	s.Paint(nil, ggui.Rct(ggui.Pt(0, 0), size))
	// 1000 rows of 24 pitch: 23996 tall; a 100-tall window shows rows 0..5.
	if f.Len() != 1000 || len(f.entries) != 5 || builds != 5 {
		t.Fatalf("len %d built %d builds %d, want 1000, 5, 5", f.Len(), len(f.entries), builds)
	}
	if s.childSize.H != 23996 {
		t.Fatalf("content height %v, want 23996", s.childSize.H)
	}
	if rects[1] != ggui.Rct(ggui.Pt(0, 24), ggui.Sz(50, 20)) {
		t.Fatalf("row 1 painted at %+v, want (0,24) 50x20", rects[1])
	}
	offset.Set(12000) // exactly row 500: rows 500..504 come into view
	size = s.Layout(ggui.Tight(ggui.Sz(100, 100)), ggui.Env{})
	s.Paint(nil, ggui.Rct(ggui.Pt(0, 0), size))
	if len(f.entries) != 10 || f.first != 500 || f.last != 505 {
		t.Fatalf("built %d, range [%d,%d); want 10 built and [500,505)", len(f.entries), f.first, f.last)
	}
	if rects[500] != ggui.Rct(ggui.Pt(0, 500*24-12000), ggui.Sz(50, 20)) {
		t.Fatalf("row 500 painted at %+v, want y=0 with the offset applied", rects[500])
	}
}

func TestForWithoutViewportLaysOutEverything(t *testing.T) {
	t.Parallel()
	items := ggui.State([]todo{{1, "a"}, {2, "b"}, {3, "c"}})
	var f *EachWidget[todo, int]
	dispose := reactive.Observe(func() {
		f = EachKeyed(items, func(t todo) int { return t.ID }, func(EachItem[todo]) ggui.Widget { return Box().Size(10, 5) }).ItemExtent(30)
	})
	defer dispose()
	got := f.Layout(ggui.Loose(ggui.Sz(100, 1000)), ggui.Env{})
	if got != ggui.Sz(10, 90) || len(f.entries) != 3 {
		t.Fatalf("size %v built %d, want 10x90 and 3", got, len(f.entries))
	}
}

func TestForRowsLeaveThroughTheirTransition(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	defer ggui.SetClock(func() time.Time { return now })()
	items := ggui.State([]int{1, 2, 3})
	painted := map[int]ggui.Rect{}
	list := EachKeyed(items, func(i int) int { return i }, func(rowItem EachItem[int]) ggui.Widget {
		r := rowItem.Value
		i := r.Get()
		return ggui.FromFuncs(
			func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(50, 10)) },
			func(_ *ggui.Canvas, rect ggui.Rect) { painted[i] = rect },
		)
	}).Transition(func(w ggui.Widget) *TransitionWidget {
		return Transition(w).Slide(-20, 0).Easing(ggui.EaseLinear).Duration(100 * time.Millisecond)
	})
	p := ggui.NewProbe(list, ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()
	now = now.Add(time.Second)
	p.Frame()
	if painted[2].Origin.X != 0 || painted[2].Origin.Y != 10 {
		t.Fatalf("row 2 at %+v once settled, want in place", painted[2])
	}
	ggui.Remove(items, func(i int) bool { return i == 2 })
	clear(painted)
	p.Frame()
	if _, ok := painted[2]; !ok {
		t.Fatal("the removed row vanished at once instead of leaving")
	}
	if painted[3].Origin.Y != 20 {
		t.Fatalf("row 3 at %+v while row 2 leaves, want still below it", painted[3])
	}
	now = now.Add(50 * time.Millisecond)
	clear(painted)
	p.Frame()
	if r := painted[2]; r.Origin.X != -10 {
		t.Fatalf("leaving row at %+v halfway, want slid 10 out", r)
	}
	now = now.Add(time.Second)
	clear(painted)
	p.Frame()
	p.Frame()
	if _, ok := painted[2]; ok || painted[3].Origin.Y != 10 {
		t.Fatalf("after leaving: row 2 painted %v, row 3 at %+v", ok, painted[3])
	}
}

func TestForPaintGroupsFollowRows(t *testing.T) {
	t.Parallel()
	for _, animate := range []bool{false, true} {
		name := "immediate"
		if animate {
			name = "transition"
		}
		t.Run(name, func(t *testing.T) {
			items := ggui.State([]int{1, 2, 3})
			var list *EachWidget[int, int]
			var painted []int
			groups := map[int]any{}
			p := ggui.ProbeBuilder(func() ggui.Widget {
				list = EachKeyed(items, func(id int) int { return id }, func(rowItem EachItem[int]) ggui.Widget {
					item := rowItem.Value
					id := item.Get()
					return ggui.FromFuncs(
						func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(20, 10)) },
						func(dst *ggui.Canvas, _ ggui.Rect) { painted = append(painted, id); groups[id] = dst.Grouped() },
					)
				})
				if animate {
					list.Transition(func(w ggui.Widget) *TransitionWidget {
						return Transition(w).Slide(-10, 0).Duration(100 * time.Millisecond)
					})
				}
				return list
			}, ggui.Sz(100, 100))
			defer p.Close()
			p.Advance(0)
			p.Advance(time.Second)
			original := map[int]*forEntry[int]{}
			maps.Copy(original, list.entries)
			check := func(want ...int) {
				t.Helper()
				painted = nil
				clear(groups)
				p.Frame()
				if len(painted) != len(want) {
					t.Fatalf("painted %v, want %v", painted, want)
				}
				for i, id := range want {
					if painted[i] != id || groups[id] != original[id] {
						t.Fatalf("painted %v: row %d has group %v, want %p", painted, id, groups[id], original[id])
					}
				}
			}
			check(1, 2, 3)
			items.Set([]int{3, 2, 1})
			check(3, 2, 1)
			items.Set([]int{3, 1})
			if animate {
				check(3, 2, 1)
				p.Advance(50 * time.Millisecond)
				check(3, 2, 1)
				// Restoring a leaving key reuses its entry without duplicating it.
				items.Set([]int{2, 1, 3})
				check(2, 1, 3)
				items.Set([]int{1, 3})
				check(2, 1, 3)
				p.Advance(time.Second)
				check(1, 3)
			} else {
				check(3, 1)
			}
			items.Set(nil)
			p.Frame()
			p.Advance(time.Second)
			check()
		})
	}
}

func TestForEvictionReleasesOwnerChildren(t *testing.T) {
	t.Parallel()
	ids := make([]int, 1000)
	for i := range ids {
		ids[i] = i
	}
	var f *EachWidget[int, int]
	created, cleaned := 0, 0
	dispose := ggui.Root(func() {
		f = EachKeyed(ggui.State(ids), func(i int) int { return i }, func(EachItem[int]) ggui.Widget {
			created++
			ggui.OnCleanup(func() { cleaned++ })
			return Box().Size(10, 20)
		}).ItemExtent(20).Retain(2)
	})
	defer dispose()
	offset := ggui.State(0.0)
	scroll := Scroll(f).BindOffset(offset)
	for i := 0; i < len(ids); i += 5 {
		offset.Set(float64(i * 20))
		scroll.Layout(ggui.Tight(ggui.Sz(100, 100)), ggui.Env{})
		// The visible and retained rows are alive, and every row let go of
		// was disposed; that what a Scope disposed is let go of by its
		// owner is the kernel's, see TestScopeLetsGoOfWhatItsCleanupDisposed.
		if live := created - cleaned; live != len(f.entries) || len(f.entries) > 7 {
			t.Fatalf("after row %d: %d rows alive, %d entries", i, live, len(f.entries))
		}
	}
	dispose()
	if created != cleaned {
		t.Fatalf("created %d rows but cleaned %d", created, cleaned)
	}
}

// estimatedRows is a list of rows from 10 to 50 tall, by ID, virtualized
// on an estimate of 30, whose rows note where they are painted.
func estimatedRows(t *testing.T, items *ggui.StateValue[[]todo], painted map[int]ggui.Rect, builds *int) *EachWidget[todo, int] {
	t.Helper()
	var f *EachWidget[todo, int]
	dispose := reactive.Observe(func() {
		f = EachKeyed(items, func(t todo) int { return t.ID }, func(row EachItem[todo]) ggui.Widget {
			id := row.Item.ID
			*builds++
			return ggui.FromFuncs(
				func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(50, rowHeight(id))) },
				func(_ *ggui.Canvas, r ggui.Rect) { painted[id] = r },
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
func frame(s *ScrollWidget, painted map[int]ggui.Rect) {
	clear(painted)
	size := s.Layout(ggui.Tight(ggui.Sz(100, 100)), ggui.Env{})
	s.Paint(nil, ggui.Rct(ggui.Pt(0, 0), size))
}

func TestEstimatedRowsBuildOnlyNearTheWindow(t *testing.T) {
	t.Parallel()
	items := ggui.State(todos(0, 1000))
	painted, builds := map[int]ggui.Rect{}, 0
	f := estimatedRows(t, items, painted, &builds)
	s := Scroll(f)
	frame(s, painted)
	// Rows 0 to 4 are 10 to 50 tall: 150 of them, the window and half again.
	if builds != 5 || len(f.entries) != 5 {
		t.Fatalf("built %d rows, %d kept; want 5 of 1000", builds, len(f.entries))
	}
	if painted[2] != ggui.Rct(ggui.Pt(0, 30), ggui.Sz(50, 30)) {
		t.Fatalf("row 2 painted at %+v, want (0,30) 50x30, below rows of 10 and 20", painted[2])
	}
	// Those laid out at their size, the rest at the estimate.
	if want := 150 + 995*30.0; s.childSize.H != want {
		t.Fatalf("content height %v, want %v", s.childSize.H, want)
	}
}

func TestEstimatedRowsScrollByWhatTheReaderScrolls(t *testing.T) {
	t.Parallel()
	items := ggui.State(todos(0, 1000))
	painted, builds := map[int]ggui.Rect{}, 0
	f := estimatedRows(t, items, painted, &builds)
	offset := ggui.State(20000.0)
	s := Scroll(f).BindOffset(offset)
	frame(s, painted)
	frame(s, painted)
	// Scrolling up through rows laid out for the first time, whose sizes
	// are not the estimate: a row in view before and after a step moves by
	// the step, and the rows painted meet.
	for range 200 {
		before := maps.Clone(painted)
		offset.Set(ggui.Peek(offset) - 37)
		reactive.Flush()
		frame(s, painted)
		ids := sortedIDs(painted)
		for i, id := range ids[1:] {
			prev := painted[ids[i]]
			if got := painted[id].Origin.Y; got != prev.Origin.Y+prev.Size.H {
				t.Fatalf("at offset %v row %d starts at %v, want %v, where row %d ends", ggui.Peek(offset), id, got, prev.Origin.Y+prev.Size.H, ids[i])
			}
		}
		moved := false
		for id, r := range painted {
			if b, ok := before[id]; ok && r.Origin.Y >= 0 && r.Origin.Y < 100 {
				if r.Origin.Y-b.Origin.Y != 37 && ggui.Peek(offset) > 0 {
					t.Fatalf("at offset %v row %d moved %v on screen, want 37", ggui.Peek(offset), id, r.Origin.Y-b.Origin.Y)
				}
				moved = true
			}
		}
		if !moved {
			t.Fatalf("at offset %v no row stayed in view across a step of 37", ggui.Peek(offset))
		}
	}
}

func TestEstimatedRowsKeepTheirPlaceAsRowsComeInAbove(t *testing.T) {
	t.Parallel()
	items := ggui.State(todos(0, 1000))
	painted, builds := map[int]ggui.Rect{}, 0
	f := estimatedRows(t, items, painted, &builds)
	offset := ggui.State(3000.0)
	s := Scroll(f).BindOffset(offset)
	frame(s, painted)
	frame(s, painted)
	before := maps.Clone(painted)
	// Older rows read in above, as a conversation pages back.
	items.Set(append(todos(-40, 0), ggui.Peek(items)...))
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
	items := ggui.State(todos(0, 1000))
	painted, builds := map[int]ggui.Rect{}, 0
	f := estimatedRows(t, items, painted, &builds)
	var head ggui.Rect
	offset := ggui.State(5000.0)
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

func sortedIDs(m map[int]ggui.Rect) []int {
	ids := make([]int, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func TestEstimatedRowsAreWindowedThroughAScrollAcross(t *testing.T) {
	t.Parallel()
	items := ggui.State(todos(0, 1000))
	painted, builds := map[int]ggui.Rect{}, 0
	f := estimatedRows(t, items, painted, &builds)
	// A diff's long lines scroll sideways inside its scroll down.
	s := Scroll(Scroll(f).Horizontal())
	frame(s, painted)
	if builds == 0 || builds > 20 {
		t.Fatalf("built %d of 1000 rows inside a scroll across, want only those near the window", builds)
	}
}

func TestFixedRowsUnderAHeaderFillTheWindow(t *testing.T) {
	t.Parallel()
	items := ggui.State(todos(0, 1000))
	painted := map[int]ggui.Rect{}
	var f *EachWidget[todo, int]
	dispose := reactive.Observe(func() {
		f = EachKeyed(items, func(t todo) int { return t.ID }, func(row EachItem[todo]) ggui.Widget {
			id := row.Item.ID
			return ggui.FromFuncs(
				func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(50, 20)) },
				func(_ *ggui.Canvas, r ggui.Rect) { painted[id] = r },
			)
		}).ItemExtent(20)
	})
	defer dispose()
	var head ggui.Rect
	offset := ggui.State(5000.0)
	s := Scroll(Column(probe(100, 410, &head), f)).BindOffset(offset)
	frame(s, painted)
	frame(s, painted)
	// The list starts 410 into the content, so the window, 5000 in, shows
	// it from 4590: rows 229 to 234, the first from 10 above the window's
	// top, the last to 10 below its foot.
	for id := 229; id <= 234; id++ {
		if _, ok := painted[id]; !ok {
			t.Fatalf("row %d, in the window, not painted; painted %v", id, sortedIDs(painted))
		}
	}
	if r := painted[229]; r.Origin.Y != -10 {
		t.Fatalf("row 229 painted at %v, want 10 above the window's top", r.Origin.Y)
	}
}

func BenchmarkForFrame(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		for _, changed := range []bool{false, true} {
			b.Run(fmt.Sprintf("rows=%d/changed=%t", n, changed), func(b *testing.B) {
				values := make([]*ggui.StateValue[int], n)
				ids := make([]int, n)
				for i := range ids {
					ids[i], values[i] = i, ggui.State(0)
				}
				items := ggui.State(ids)
				p := ggui.ProbeBuilder(func() ggui.Widget {
					return EachKeyed(items, func(id int) int { return id }, func(rowItem EachItem[int]) ggui.Widget {
						item := rowItem.Value
						return ggui.Reactive(func() ggui.Widget { return Box().Size(float64(values[item.Get()].Get()%7+1), 1) })
					})
				}, ggui.Sz(100, 100))
				defer p.Close()
				p.Frame()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if changed {
						values[i%n].Set(i + 1)
					}
					p.Frame()
				}
				b.StopTimer()
			})
		}
	}
}

// Many rows removed at once with a Transition each leave from their own
// place: the leaving rows are merged back among the rest in the order of
// the places they left, however the removals were ranged over, and a row
// whose item comes back mid-leave returns to the list.
func TestForManyRowsLeaveInTheirPlaces(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	defer ggui.SetClock(func() time.Time { return now })()
	ids := make([]int, 20)
	for i := range ids {
		ids[i] = i
	}
	items := ggui.State(ids)
	painted := map[int]ggui.Rect{}
	list := EachKeyed(items, func(i int) int { return i }, func(row EachItem[int]) ggui.Widget {
		i := row.Item
		return ggui.FromFuncs(
			func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(50, 10)) },
			func(_ *ggui.Canvas, rect ggui.Rect) { painted[i] = rect },
		)
	}).Transition(func(w ggui.Widget) *TransitionWidget {
		return Transition(w).Fade().Duration(100 * time.Millisecond)
	})
	p := ggui.NewProbe(list, ggui.Sz(100, 400))
	defer p.Close()
	p.Frame()
	now = now.Add(time.Second)
	p.Frame()
	gone := map[int]bool{0: true, 1: true, 2: true, 7: true, 8: true, 13: true, 19: true}
	ggui.Remove(items, func(i int) bool { return gone[i] })
	clear(painted)
	p.Frame()
	for i := range ids {
		if r, ok := painted[i]; !ok || r.Origin.Y != float64(i*10) {
			t.Fatalf("row %d painted at %+v (painted %t) while rows leave, want at y=%d", i, r, ok, i*10)
		}
	}
	// 8 comes back before it has gone; the rest finish leaving.
	ggui.Append(items, 8)
	now = now.Add(time.Second)
	clear(painted)
	p.Frame()
	p.Frame()
	if len(list.leaving) != 0 || len(list.leaveKeys) != 0 {
		t.Fatalf("%d rows still leaving after their Transition ended", len(list.leaving))
	}
	if _, ok := painted[8]; !ok || len(painted) != len(ids)-len(gone)+1 {
		t.Fatalf("painted %d rows, with 8 %v, want %d with 8 back", len(painted), painted[8], len(ids)-len(gone)+1)
	}
	for k := range gone {
		if _, ok := painted[k]; ok && k != 8 {
			t.Fatalf("row %d painted after it left", k)
		}
	}
}

// Clearing a list whose rows have a Transition: every removed row's place
// is found in one map rather than by searching the list for each.
func BenchmarkForClearWithTransition(b *testing.B) {
	for _, n := range []int{1000, 4000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			ids := make([]int, n)
			for i := range ids {
				ids[i] = i
			}
			items := ggui.State(ids)
			p := ggui.ProbeBuilder(func() ggui.Widget {
				return EachKeyed(items, func(id int) int { return id }, func(EachItem[int]) ggui.Widget {
					return Box().Size(1, 1)
				}).Transition(func(w ggui.Widget) *TransitionWidget { return Transition(w).Fade().Duration(time.Hour) })
			}, ggui.Sz(100, 100))
			defer p.Close()
			p.Frame()
			b.ReportAllocs()
			for b.Loop() {
				items.Set(nil)
				p.Frame()
				b.StopTimer()
				items.Set(ids)
				p.Frame()
				b.StartTimer()
			}
		})
	}
}

// Scroll frames over rows already measured, in a list of 100k rows of any
// size: what a frame costs when only the window moved.
func BenchmarkEstimatedScrollFrame(b *testing.B) {
	items := ggui.State(todos(0, 100000))
	painted, builds := map[int]ggui.Rect{}, 0
	var f *EachWidget[todo, int]
	dispose := reactive.Observe(func() {
		f = EachKeyed(items, func(t todo) int { return t.ID }, func(row EachItem[todo]) ggui.Widget {
			id := row.Item.ID
			builds++
			return ggui.FromFuncs(
				func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(50, rowHeight(id))) },
				func(_ *ggui.Canvas, r ggui.Rect) { painted[id] = r },
			)
		}).EstimatedExtent(30)
	})
	defer dispose()
	offset := ggui.State(1500000.0)
	s := Scroll(f).BindOffset(offset)
	for range 4 {
		frame(s, painted)
	}
	at := ggui.Untrack(offset.Get)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		offset.Set(at + float64(i%2)*5)
		frame(s, painted)
	}
}

// tagged is an item == cannot compare, for its slice.
type tagged struct {
	ID   int
	Tags []string
}

// Without a comparison a list of items == cannot compare tells every row
// of every change; with ItemEqual only the rows whose items differ run
// again.
func TestItemEqualKeepsUnchangedRowsStill(t *testing.T) {
	t.Parallel()
	for _, withEq := range []bool{false, true} {
		list := make([]tagged, 50)
		for i := range list {
			list[i] = tagged{i, []string{"a"}}
		}
		items := ggui.State(list)
		runs := map[int]int{}
		p := ggui.ProbeBuilder(func() ggui.Widget {
			each := EachKeyed(items, func(r tagged) int { return r.ID }, func(row EachItem[tagged]) ggui.Widget {
				return ggui.Reactive(func() ggui.Widget {
					r := row.Value.Get()
					runs[r.ID]++
					return Box().Size(1, 1)
				})
			})
			if withEq {
				each.ItemEqual(func(a, b tagged) bool { return a.ID == b.ID && slices.Equal(a.Tags, b.Tags) })
			}
			return each
		}, ggui.Sz(100, 100))
		p.Frame()
		clear(runs)
		next := slices.Clone(list)
		next[7] = tagged{7, []string{"a", "b"}}
		next = append(next, tagged{50, nil})
		items.Set(next)
		p.Frame()
		p.Close()
		if runs[7] != 1 || runs[50] != 1 {
			t.Fatalf("withEq=%t: the changed row ran %d times and the new one %d, want 1 each", withEq, runs[7], runs[50])
		}
		if want := map[bool]int{false: 51, true: 2}[withEq]; len(runs) != want {
			t.Fatalf("withEq=%t: %d rows ran again, want %d", withEq, len(runs), want)
		}
	}
}

// Frames that only scroll an EstimatedExtent list keep its rows where
// placing them afresh puts them, and rows measured as they scroll in keep
// what is in view in place as before.
func TestEstimatedScrollFramesPlaceAsAFreshPlace(t *testing.T) {
	t.Parallel()
	items := ggui.State(todos(0, 2000))
	painted, builds := map[int]ggui.Rect{}, 0
	f := estimatedRows(t, items, painted, &builds)
	offset := ggui.State(30000.0)
	s := Scroll(f).BindOffset(offset)
	frame(s, painted)
	frame(s, painted)
	for step := range 120 {
		// Back and forth over rows measured, and on down into new ones.
		d := []float64{-23, 23, 41}[step%3]
		offset.Set(ggui.Peek(offset) + d)
		reactive.Flush()
		frame(s, painted)
		var fresh virtual.Estimate[int]
		for i, k := range f.keys {
			if e := f.entries[k]; e != nil && e.seen > 0 {
				fresh.Measure(k, rowHeight(i))
			}
		}
		fresh.Place(f.keys, f.estimate, 0)
		for i := range len(f.keys) + 1 {
			got, _ := f.est.Start(i)
			want, _ := fresh.Start(i)
			if got != want {
				t.Fatalf("step %d: row %d starts at %v, a fresh Place puts it at %v", step, i, got, want)
			}
		}
	}
}

// Scroll frames deep into a list of 100k fixed rows with sticky headers
// far apart: finding the header of the section in view.
func BenchmarkStickyScrollFrame(b *testing.B) {
	ids := make([]int, 100000)
	for i := range ids {
		ids[i] = i
	}
	painted := map[int]ggui.Rect{}
	var f *EachWidget[int, int]
	dispose := reactive.Observe(func() {
		f = EachKeyed(ggui.State(ids), func(i int) int { return i }, func(row EachItem[int]) ggui.Widget {
			id := row.Item
			return ggui.FromFuncs(
				func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(50, 20)) },
				func(_ *ggui.Canvas, r ggui.Rect) { painted[id] = r },
			)
		}).ItemExtent(20).StickyHeaders(func(i int) bool { return i%50000 == 0 })
	})
	defer dispose()
	offset := ggui.State(1900000.0)
	s := Scroll(f).BindOffset(offset)
	frame(s, painted)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		offset.Set(1900000 + float64(i%2)*5)
		frame(s, painted)
	}
}

// The header rows a virtual list pins are found again when its items
// change, so the header pinned is the one now above the window.
func TestStickyHeadersFollowTheItems(t *testing.T) {
	t.Parallel()
	values := func(from int) []int {
		v := make([]int, 100)
		for i := range v {
			v[i] = from + i
		}
		return v
	}
	items := ggui.State(values(0))
	painted := map[int]ggui.Rect{}
	each := EachKeyed(items, func(v int) int { return v }, func(it EachItem[int]) ggui.Widget {
		v := it.Item
		return ggui.FromFuncs(
			func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(100, 20)) },
			func(_ *ggui.Canvas, r ggui.Rect) { painted[v] = r },
		)
	}).ItemExtent(20).StickyHeaders(func(v int) bool { return v%10 == 0 })
	p := ggui.NewProbe(Scroll(each).BindOffset(ggui.State(250.0)), ggui.Sz(100, 60))
	defer p.Close()
	p.Frame()
	if painted[10].Origin.Y != 0 {
		t.Fatalf("header 10 at %v, want pinned at 0", painted[10])
	}
	// Now 10 is the row at index 7, and the row at index 10 is no header.
	items.Set(values(3))
	clear(painted)
	p.Frame()
	if r, ok := painted[10]; !ok || r.Origin.Y != 0 {
		t.Fatalf("after the items moved, header 10 at %v (painted %t), want pinned at 0", r, ok)
	}
	if r, ok := painted[13]; ok && r.Origin.Y == 0 {
		t.Fatal("the row where a header used to be is pinned")
	}
}

// ItemEqual(nil) tells every kept row of every change, even of items ==
// could compare, as WithEqual(nil) does for a State.
func TestItemEqualNilTellsEveryRow(t *testing.T) {
	t.Parallel()
	items := ggui.State([]int{1, 2, 3})
	runs := 0
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return Each(items, func(row EachItem[int]) ggui.Widget {
			return ggui.Reactive(func() ggui.Widget { row.Value.Get(); runs++; return Box().Size(1, 1) })
		}).ItemEqual(nil)
	}, ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()
	runs = 0
	items.Set([]int{1, 2, 3, 4})
	p.Frame()
	if runs != 4 {
		t.Fatalf("%d rows ran after an append, want all 3 kept and the new one", runs)
	}
}

// Frames of a list of 5000 rows, every one laid out, with sticky headers,
// scrolled into its middle: finding the header pinned over the window.
func BenchmarkStickyPlainFrame(b *testing.B) {
	ids := make([]int, 5000)
	for i := range ids {
		ids[i] = i
	}
	painted := map[int]ggui.Rect{}
	var f *EachWidget[int, int]
	dispose := reactive.Observe(func() {
		f = EachKeyed(ggui.State(ids), func(i int) int { return i }, func(row EachItem[int]) ggui.Widget {
			id := row.Item
			return ggui.FromFuncs(
				func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(50, 20)) },
				func(_ *ggui.Canvas, r ggui.Rect) { painted[id] = r },
			)
		}).StickyHeaders(func(i int) bool { return i%1000 == 0 })
	})
	defer dispose()
	offset := ggui.State(50000.0)
	s := Scroll(f).BindOffset(offset)
	frame(s, painted)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		offset.Set(50000 + float64(i%2)*5)
		frame(s, painted)
	}
}

// Rows leaving a list with sticky headers keep their places, and the
// header pinned over the window is still the one of the section it starts
// in, found among the live rows and pushed out by the next header's.
func TestStickyHeadersPinAmongLeavingRows(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	defer ggui.SetClock(func() time.Time { return now })()
	list := make([]int, 100)
	for i := range list {
		list[i] = i
	}
	items := ggui.State(list)
	painted := map[int]ggui.Rect{}
	each := EachKeyed(items, func(v int) int { return v }, func(it EachItem[int]) ggui.Widget {
		v := it.Item
		return ggui.FromFuncs(
			func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(100, 20)) },
			func(_ *ggui.Canvas, r ggui.Rect) { painted[v] = r },
		)
	}).StickyHeaders(func(v int) bool { return v%10 == 0 }).Transition(func(w ggui.Widget) *TransitionWidget {
		return Transition(w).Fade().Duration(time.Hour)
	})
	offset := ggui.State(250.0)
	p := ggui.NewProbe(Scroll(each).BindOffset(offset), ggui.Sz(100, 60))
	defer p.Close()
	p.Frame()
	ggui.Remove(items, func(v int) bool { return v == 3 || v == 15 })
	clear(painted)
	p.Frame()
	if painted[10].Origin.Y != 0 {
		t.Fatalf("with rows leaving, header 10 at %v, want pinned at 0", painted[10])
	}
	offset.Set(390)
	clear(painted)
	p.Frame()
	if painted[10].Origin.Y != -10 || painted[20].Origin.Y != 10 {
		t.Fatalf("with rows leaving at 390: header 10 at %v, 20 at %v; want 10 pushed to -10", painted[10], painted[20])
	}
}

// Rows leaving a list under a layout cache, as every Component is, slide
// out in a window's own frames, one layout a frame with no input, until
// they have gone and the window rests. The layout each frame asks for is
// asked as the list is laid out, within the measurement of the caches
// above it.
func TestForRowsLeaveInAWindowsFrames(t *testing.T) {
	items := ggui.State([]int{1, 2, 3})
	painted := map[int]ggui.Rect{}
	p := ggui.NewWindowProbe(func() ggui.Widget {
		list := EachKeyed(items, func(i int) int { return i }, func(rowItem EachItem[int]) ggui.Widget {
			i := rowItem.Value.Get()
			return ggui.FromFuncs(
				func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(50, 10)) },
				func(_ *ggui.Canvas, rect ggui.Rect) { painted[i] = rect },
			)
		}).Transition(func(w ggui.Widget) *TransitionWidget {
			return Transition(w).Slide(-20, 0).Easing(ggui.EaseLinear).Duration(100 * time.Millisecond)
		})
		return Column(ggui.Component(func() ggui.Widget { return Column(list) }), Box().Size(50, 10))
	}, ggui.Sz(200, 200))
	defer p.Close()
	p.Run(16*time.Millisecond, 10)
	ggui.Remove(items, func(i int) bool { return i == 2 })
	var xs []float64
	var frames []ggui.WindowFrame
	for range 30 {
		clear(painted)
		fr := p.Step(16 * time.Millisecond)
		frames = append(frames, fr)
		if r, ok := painted[2]; ok {
			xs = append(xs, r.Origin.X)
		}
		if !fr.Next {
			break
		}
	}
	if n := len(frames); frames[n-1].Next {
		t.Fatalf("the window never came to rest: %v", frames)
	}
	if len(xs) < 4 || xs[len(xs)-1] > -15 {
		t.Fatalf("the leaving row was painted at %v, want it sliding out over several frames", xs)
	}
	if _, ok := painted[2]; ok || painted[3].Origin.Y != 10 {
		t.Fatalf("after leaving: row 2 painted %v, row 3 at %+v", ok, painted[3])
	}
}

func TestEachKeysByValue(t *testing.T) {
	t.Parallel()
	tags := ggui.State([]string{"a", "b"})
	builds := 0
	list := Each(tags, func(rowItem EachItem[string]) ggui.Widget { r := rowItem.Value; builds++; return TextOf(r) })
	p := ggui.NewProbe(list, ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()
	tags.Set([]string{"b", "a", "c"})
	p.Frame()
	if builds != 3 {
		t.Fatalf("builds = %d, want 3: a and b are kept by value", builds)
	}
}

func TestForRetainEvictsOffscreenRows(t *testing.T) {
	t.Parallel()
	var items []int
	for i := range 100 {
		items = append(items, i)
	}
	src := ggui.State(items)
	var f *EachWidget[int, int]
	off := ggui.State(0.0)
	dispose := reactive.Observe(func() {
		f = EachKeyed(src, func(i int) int { return i }, func(EachItem[int]) ggui.Widget { return Box().Size(10, 10) }).ItemExtent(10).Retain(2)
	})
	defer dispose()
	p := ggui.NewProbe(Scroll(f).BindOffset(off), ggui.Sz(10, 50))
	p.Frame()
	off.Set(500)
	p.Frame()
	if n := len(f.entries); n != 5+2 {
		t.Fatalf("mounted rows = %d, want 7", n)
	}
}

func TestSemanticsListCountsItems(t *testing.T) {
	t.Parallel()
	items := ggui.State([]string{"a", "b", "c"})
	p := ggui.NewProbe(Each(items, func(rowItem EachItem[string]) ggui.Widget { s := rowItem.Value; return TextOf(s) }), ggui.Sz(100, 200))
	defer p.Close()
	tree := p.Semantics()
	list, ok := tree.Find(ggui.RoleList, "")
	if !ok {
		t.Fatalf("no list:\n%s", tree)
	}
	if list.Max != 3 {
		t.Errorf("list.Max = %g, want 3", list.Max)
	}
	if len(list.Children) != 3 {
		t.Fatalf("%d items, want 3:\n%s", len(list.Children), tree)
	}
	third := tree.At(list.Children[2])
	if third.Role != ggui.RoleListItem || third.Now != 3 || third.Max != 3 {
		t.Errorf("third item = %v, want item 3 of 3", third.Node)
	}
	if kids := third.Children; len(kids) != 1 || tree.At(kids[0]).Name != "c" {
		t.Errorf("the item does not hold its row:\n%s", tree)
	}
}
