package ggui

import (
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/ironpark/ggui/internal/reactive"
)

// layoutFor lays a EachKeyed out with room for everything, which is when its
// children are built.
func layoutFor(w Widget) { w.Layout(Loose(Sz(500, Unbounded)), Env{}) }

type todo struct {
	ID   int
	Name string
}

func TestForReusesChildrenByKey(t *testing.T) {
	t.Parallel()
	items := State([]todo{{1, "a"}, {2, "b"}})
	setups := 0
	var f Widget
	dispose := reactive.Observe(func() {
		f = EachKeyed(items, func(t todo) int { return t.ID }, func(rowItem EachItem[todo]) Widget {
			it := rowItem.Value
			return Component(func() Widget {
				setups++
				return Reactive(func() Widget { return Text(it.Get().Name) })
			})
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
			out = append(out, blockChild(w).(*TextWidget).value)
		}
		return out
	}
	if got := names(); len(got) != 3 || got[0] != "b" || got[1] != "a" || got[2] != "c" {
		t.Fatalf("order = %v, want [b a c]", got)
	}
}

func TestForUpdatesItemSignalInPlace(t *testing.T) {
	t.Parallel()
	items := State([]todo{{1, "a"}})
	builds := 0
	var f Widget
	dispose := reactive.Observe(func() {
		f = EachKeyed(items, func(t todo) int { return t.ID }, func(rowItem EachItem[todo]) Widget {
			it := rowItem.Value
			return Reactive(func() Widget { builds++; return Text(it.Get().Name) })
		})
	})
	defer dispose()
	layoutFor(f)
	items.Set([]todo{{1, "renamed"}})
	reactive.Flush()
	layoutFor(f)
	got := f.(*EachWidget[todo, int]).children[0].(*ComponentWidget).child.(*TextWidget).value
	if got != "renamed" || builds != 2 {
		t.Fatalf("child shows %q after %d builds, want renamed after 2", got, builds)
	}
}

func TestForDisposesRemovedAndAllOnParentRebuild(t *testing.T) {
	t.Parallel()
	items := State([]todo{{1, "a"}, {2, "b"}})
	parentDep := State(0)
	cleanups := 0
	var f Widget
	dispose := reactive.Observe(func() {
		parentDep.Get()
		f = EachKeyed(items, func(t todo) int { return t.ID }, func(rowItem EachItem[todo]) Widget {
			OnCleanup(func() { cleanups++ })
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
	items := State([]todo{{1, "a"}})
	setups := 0
	var f Widget
	dispose := reactive.Observe(func() {
		f = EachKeyed(items, func(t todo) int { return t.ID }, func(rowItem EachItem[todo]) Widget {
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
	EachKeyed(State([]todo{{1, "a"}, {1, "b"}}), func(t todo) int { return t.ID }, func(EachItem[todo]) Widget { return Box() }).Layout(Loose(Sz(100, 100)), Env{})
}

func TestRootOutlivesOwnerRerunsUntilDisposed(t *testing.T) {
	t.Parallel()
	dep, inner := State(0), State(0)
	innerRuns := 0
	var disposeRoot func()
	dispose := reactive.Observe(func() {
		dep.Get()
		if disposeRoot == nil {
			disposeRoot = Root(func() {
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
	items := State(list)
	builds := 0
	var f *EachWidget[todo, int]
	rects := make([]Rect, len(list)) // where each row was painted, by ID
	dispose := reactive.Observe(func() {
		f = EachKeyed(items, func(t todo) int { return t.ID }, func(rowItem EachItem[todo]) Widget {
			it := rowItem.Value
			builds++
			return probe(50, 20, &rects[it.Get().ID])
		}).ItemExtent(20).Gap(4)
	})
	defer dispose()
	offset := State(0.0)
	s := Scroll(f).BindOffset(offset)
	size := s.Layout(Tight(Sz(100, 100)), Env{})
	s.Paint(nil, Rct(Pt(0, 0), size))
	// 1000 rows of 24 pitch: 23996 tall; a 100-tall window shows rows 0..5.
	if f.Len() != 1000 || len(f.entries) != 5 || builds != 5 {
		t.Fatalf("len %d built %d builds %d, want 1000, 5, 5", f.Len(), len(f.entries), builds)
	}
	if s.childSize.H != 23996 {
		t.Fatalf("content height %v, want 23996", s.childSize.H)
	}
	if rects[1] != Rct(Pt(0, 24), Sz(50, 20)) {
		t.Fatalf("row 1 painted at %+v, want (0,24) 50x20", rects[1])
	}
	offset.Set(12000) // exactly row 500: rows 500..504 come into view
	size = s.Layout(Tight(Sz(100, 100)), Env{})
	s.Paint(nil, Rct(Pt(0, 0), size))
	if len(f.entries) != 10 || f.first != 500 || f.last != 505 {
		t.Fatalf("built %d, range [%d,%d); want 10 built and [500,505)", len(f.entries), f.first, f.last)
	}
	if rects[500] != Rct(Pt(0, 500*24-12000), Sz(50, 20)) {
		t.Fatalf("row 500 painted at %+v, want y=0 with the offset applied", rects[500])
	}
}

func TestForWithoutViewportLaysOutEverything(t *testing.T) {
	t.Parallel()
	items := State([]todo{{1, "a"}, {2, "b"}, {3, "c"}})
	var f *EachWidget[todo, int]
	dispose := reactive.Observe(func() {
		f = EachKeyed(items, func(t todo) int { return t.ID }, func(EachItem[todo]) Widget { return Box().Size(10, 5) }).ItemExtent(30)
	})
	defer dispose()
	got := f.Layout(Loose(Sz(100, 1000)), Env{})
	if got != Sz(10, 90) || len(f.entries) != 3 {
		t.Fatalf("size %v built %d, want 10x90 and 3", got, len(f.entries))
	}
}

func TestForRowsLeaveThroughTheirTransition(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	defer SetClock(func() time.Time { return now })()
	items := State([]int{1, 2, 3})
	painted := map[int]Rect{}
	list := EachKeyed(items, func(i int) int { return i }, func(rowItem EachItem[int]) Widget {
		r := rowItem.Value
		i := r.Get()
		return FromFuncs(
			func(c Constraints, _ Env) Size { return c.Constrain(Sz(50, 10)) },
			func(_ *Canvas, rect Rect) { painted[i] = rect },
		)
	}).Transition(func(w Widget) *TransitionWidget {
		return Transition(w).Slide(-20, 0).Easing(EaseLinear).Duration(100 * time.Millisecond)
	})
	p := NewProbe(list, Sz(100, 100))
	defer p.Close()
	p.Frame()
	now = now.Add(time.Second)
	p.Frame()
	if painted[2].Origin.X != 0 || painted[2].Origin.Y != 10 {
		t.Fatalf("row 2 at %+v once settled, want in place", painted[2])
	}
	Remove(items, func(i int) bool { return i == 2 })
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
			items := State([]int{1, 2, 3})
			var list *EachWidget[int, int]
			var painted []int
			groups := map[int]any{}
			p := ProbeBuilder(func() Widget {
				list = EachKeyed(items, func(id int) int { return id }, func(rowItem EachItem[int]) Widget {
					item := rowItem.Value
					id := item.Get()
					return FromFuncs(
						func(c Constraints, _ Env) Size { return c.Constrain(Sz(20, 10)) },
						func(dst *Canvas, _ Rect) { painted = append(painted, id); groups[id] = dst.group },
					)
				})
				if animate {
					list.Transition(func(w Widget) *TransitionWidget { return Transition(w).Slide(-10, 0).Duration(100 * time.Millisecond) })
				}
				return list
			}, Sz(100, 100))
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
	dispose := Root(func() {
		f = EachKeyed(State(ids), func(i int) int { return i }, func(EachItem[int]) Widget {
			created++
			OnCleanup(func() { cleaned++ })
			return Box().Size(10, 20)
		}).ItemExtent(20).Retain(2)
	})
	defer dispose()
	offset := State(0.0)
	scroll := Scroll(f).BindOffset(offset)
	for i := 0; i < len(ids); i += 5 {
		offset.Set(float64(i * 20))
		scroll.Layout(Tight(Sz(100, 100)), Env{})
		count := 0
		for _, c := range f.owner.Children() {
			if c.Disposed() {
				t.Fatal("owner retains a disposed row")
			}
			count++
		}
		// One list-watching effect plus the visible and retained row roots.
		if count != 1+len(f.entries) || len(f.entries) > 7 {
			t.Fatalf("after row %d: %d children, %d entries", i, count, len(f.entries))
		}
	}
	dispose()
	if created != cleaned {
		t.Fatalf("created %d rows but cleaned %d", created, cleaned)
	}
}

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

func TestEstimatedRowsAreWindowedThroughAScrollAcross(t *testing.T) {
	t.Parallel()
	items := State(todos(0, 1000))
	painted, builds := map[int]Rect{}, 0
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
	items := State(todos(0, 1000))
	painted := map[int]Rect{}
	var f *EachWidget[todo, int]
	dispose := reactive.Observe(func() {
		f = EachKeyed(items, func(t todo) int { return t.ID }, func(row EachItem[todo]) Widget {
			id := row.Item.ID
			return FromFuncs(
				func(c Constraints, _ Env) Size { return c.Constrain(Sz(50, 20)) },
				func(_ *Canvas, r Rect) { painted[id] = r },
			)
		}).ItemExtent(20)
	})
	defer dispose()
	var head Rect
	offset := State(5000.0)
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
				values := make([]*StateValue[int], n)
				ids := make([]int, n)
				for i := range ids {
					ids[i], values[i] = i, State(0)
				}
				items := State(ids)
				p := ProbeBuilder(func() Widget {
					return EachKeyed(items, func(id int) int { return id }, func(rowItem EachItem[int]) Widget {
						item := rowItem.Value
						return Reactive(func() Widget { return Box().Size(float64(values[item.Get()].Get()%7+1), 1) })
					})
				}, Sz(100, 100))
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
