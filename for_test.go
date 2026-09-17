package ggui

import "testing"

// layoutFor lays a For out with room for everything, which is when its
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
	dispose := Effect(func() {
		f = For(items, func(t todo) int { return t.ID }, func(it *Signal[todo]) Widget {
			return Component(func() Builder {
				setups++
				return func() Widget { return Text(it.Get().Name) }
			})
		})
	})
	defer dispose()
	layoutFor(f)
	items.Set([]todo{{2, "b"}, {1, "a"}, {3, "c"}})
	effects.flush()
	layoutFor(f)
	if setups != 3 {
		t.Fatalf("setups = %d after reorder plus one new item, want 3", setups)
	}
	names := func() []string {
		var out []string
		for _, w := range f.(*ForWidget[todo, int]).children {
			out = append(out, w.(*ComponentWidget).child.(*TextWidget).value)
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
	dispose := Effect(func() {
		f = For(items, func(t todo) int { return t.ID }, func(it *Signal[todo]) Widget {
			return Reactive(func() Widget { builds++; return Text(it.Get().Name) })
		})
	})
	defer dispose()
	layoutFor(f)
	items.Set([]todo{{1, "renamed"}})
	effects.flush()
	layoutFor(f)
	got := f.(*ForWidget[todo, int]).children[0].(*ComponentWidget).child.(*TextWidget).value
	if got != "renamed" || builds != 2 {
		t.Fatalf("child shows %q after %d builds, want renamed after 2", got, builds)
	}
}

func TestForDisposesRemovedAndAllOnParentRebuild(t *testing.T) {
	items := State([]todo{{1, "a"}, {2, "b"}})
	parentDep := State(0)
	cleanups := 0
	var f Widget
	dispose := Effect(func() {
		parentDep.Get()
		f = For(items, func(t todo) int { return t.ID }, func(it *Signal[todo]) Widget {
			OnCleanup(func() { cleanups++ })
			return Box()
		})
	})
	defer dispose()
	layoutFor(f)
	items.Set([]todo{{1, "a"}})
	effects.flush()
	if cleanups != 1 {
		t.Fatalf("cleanups = %d after removing one item, want 1", cleanups)
	}
	parentDep.Set(1)
	effects.flush()
	if cleanups != 2 {
		t.Fatalf("cleanups = %d after the parent rebuilt, want 2 (the survivor disposed)", cleanups)
	}
}

func TestForDoesNotRebuildOnParentSignals(t *testing.T) {
	items := State([]todo{{1, "a"}})
	setups := 0
	var f Widget
	dispose := Effect(func() {
		f = For(items, func(t todo) int { return t.ID }, func(it *Signal[todo]) Widget {
			setups++
			return Box()
		})
	})
	defer dispose()
	layoutFor(f)
	items.Set([]todo{{1, "a"}}) // equal slice contents, new slice: notifies
	effects.flush()
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
	For(State([]todo{{1, "a"}, {1, "b"}}), func(t todo) int { return t.ID }, func(*Signal[todo]) Widget { return Box() })
}

func TestRootOutlivesOwnerRerunsUntilDisposed(t *testing.T) {
	dep, inner := State(0), State(0)
	innerRuns := 0
	var disposeRoot func()
	dispose := Effect(func() {
		dep.Get()
		if disposeRoot == nil {
			disposeRoot = Root(func() {
				Effect(func() { inner.Get(); innerRuns++ })
			})
		}
	})
	defer dispose()
	dep.Set(1) // owner re-runs; the Root was created by an older run, so it is disposed
	effects.flush()
	inner.Set(1)
	effects.flush()
	if innerRuns != 1 {
		t.Fatalf("innerRuns = %d, want 1: a Root is owned by the run that created it", innerRuns)
	}
}

func TestForWithItemExtentBuildsOnlyTheViewport(t *testing.T) {
	var list []todo
	for i := range 1000 {
		list = append(list, todo{i, "x"})
	}
	items := State(list)
	builds := 0
	var f *ForWidget[todo, int]
	rects := make([]Rect, len(list)) // where each row was painted, by ID
	dispose := Effect(func() {
		f = For(items, func(t todo) int { return t.ID }, func(it *Signal[todo]) Widget {
			builds++
			return probe(50, 20, &rects[it.Peek().ID])
		}).ItemExtent(20).Gap(4)
	})
	defer dispose()
	offset := State(0.0)
	s := Scroll(f).Offset(offset)
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
	var f *ForWidget[todo, int]
	dispose := Effect(func() {
		f = For(items, func(t todo) int { return t.ID }, func(*Signal[todo]) Widget { return Box().Size(10, 5) }).ItemExtent(30)
	})
	defer dispose()
	got := f.Layout(Loose(Sz(100, 1000)), Env{})
	if got != Sz(10, 90) || len(f.entries) != 3 {
		t.Fatalf("size %v built %d, want 10x90 and 3", got, len(f.entries))
	}
}
