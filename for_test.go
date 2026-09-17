package ggui

import "testing"

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
	items.Set([]todo{{2, "b"}, {1, "a"}, {3, "c"}})
	effects.flush()
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
	items.Set([]todo{{1, "renamed"}})
	effects.flush()
	got := f.(*ForWidget[todo, int]).children[0].(*ComponentWidget).child.(*TextWidget).value
	if got != "renamed" || builds != 2 {
		t.Fatalf("child shows %q after %d builds, want renamed after 2", got, builds)
	}
}

func TestForDisposesRemovedAndAllOnParentRebuild(t *testing.T) {
	items := State([]todo{{1, "a"}, {2, "b"}})
	parentDep := State(0)
	cleanups := 0
	dispose := Effect(func() {
		parentDep.Get()
		For(items, func(t todo) int { return t.ID }, func(it *Signal[todo]) Widget {
			OnCleanup(func() { cleanups++ })
			return Box()
		})
	})
	defer dispose()
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
	dispose := Effect(func() {
		For(items, func(t todo) int { return t.ID }, func(it *Signal[todo]) Widget {
			setups++
			return Box()
		})
	})
	defer dispose()
	items.Set([]todo{{1, "a"}}) // equal slice contents, new slice: notifies
	effects.flush()
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
