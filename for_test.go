package ggui

import (
	"github.com/ironpark/ggui/internal/reactive"
	"maps"
	"testing"
	"time"
)

// layoutFor lays a EachKeyed out with room for everything, which is when its
// children are built.
func layoutFor(w Widget) { w.Layout(Loose(Sz(500, Unbounded)), Env{}) }

type todo struct {
	ID   int
	Name string
}

func TestForReusesChildrenByKey(t *testing.T) {
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
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate keys did not panic")
		}
	}()
	EachKeyed(State([]todo{{1, "a"}, {1, "b"}}), func(t todo) int { return t.ID }, func(EachItem[todo]) Widget { return Box() }).Layout(Loose(Sz(100, 100)), Env{})
}

func TestRootOutlivesOwnerRerunsUntilDisposed(t *testing.T) {
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
