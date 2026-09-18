package ggui

import (
	"slices"
	"testing"
)

func TestInnerEffectDisposedWhenOuterReruns(t *testing.T) {
	outerDep, innerDep := State(0), State(0)
	innerRuns := 0
	dispose := observe(func() {
		outerDep.Get()
		observe(func() { innerDep.Get(); innerRuns++ })
	})
	defer dispose()
	if innerRuns != 1 {
		t.Fatalf("innerRuns = %d after setup, want 1", innerRuns)
	}
	innerDep.Set(1)
	effects.flush()
	if innerRuns != 2 {
		t.Fatalf("innerRuns = %d after inner change, want 2", innerRuns)
	}
	outerDep.Set(1) // outer re-runs: old inner disposed, one new inner created
	effects.flush()
	innerDep.Set(2)
	effects.flush()
	if innerRuns != 4 {
		t.Fatalf("innerRuns = %d, want 4 (not 5: the stale inner must not fire)", innerRuns)
	}
}

func TestDerivedInsideBuilderDoesNotAccumulate(t *testing.T) {
	rebuild, n := State(0), State(1)
	computes := 0
	dispose := observe(func() {
		rebuild.Get()
		m := Derived(func() int { computes++; return n.Get() * 2 })
		observe(func() { m.Get() })
	})
	defer dispose()
	rebuild.Set(1)
	rebuild.Set(2)
	effects.flush()
	computes = 0
	n.Set(5)
	effects.flush()
	if computes != 1 {
		t.Fatalf("computes = %d after one change, want 1 live Derived", computes)
	}
}

func TestSubscriptionsFollowTheLastRun(t *testing.T) {
	flag, a := State(true), State(0)
	runs := 0
	dispose := observe(func() {
		runs++
		if flag.Get() {
			a.Get()
		}
	})
	defer dispose()
	flag.Set(false)
	effects.flush()
	runs = 0
	a.Set(1)
	effects.flush()
	if runs != 0 {
		t.Fatalf("effect re-ran %d times on a signal it no longer reads", runs)
	}
	if len(a.subs) != 0 {
		t.Fatalf("signal keeps %d stale subscriptions, want 0", len(a.subs))
	}
}

func TestOnCleanupRunsBeforeRerunAndOnDispose(t *testing.T) {
	dep := State(0)
	var log []string
	dispose := observe(func() {
		v := dep.Get()
		log = append(log, "run")
		OnCleanup(func() { log = append(log, "cleanup") })
		_ = v
	})
	dep.Set(1)
	effects.flush()
	dispose()
	dispose() // idempotent
	want := []string{"run", "cleanup", "run", "cleanup"}
	if len(log) != len(want) {
		t.Fatalf("log = %v, want %v", log, want)
	}
	for i := range want {
		if log[i] != want[i] {
			t.Fatalf("log = %v, want %v", log, want)
		}
	}
}

func TestOnCleanupOutsideEffectPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("OnCleanup outside an Effect did not panic")
		}
	}()
	OnCleanup(func() {})
}

func TestUntrackAndPeekDoNotSubscribe(t *testing.T) {
	a, b := State(0), State(0)
	runs := 0
	dispose := observe(func() {
		runs++
		Untrack(func() struct {
		} {
			a.Get()
			return struct {
			}{}
		})
		Untrack(b.Get)
	})
	defer dispose()
	a.Set(1)
	b.Set(1)
	effects.flush()
	if runs != 1 {
		t.Fatalf("runs = %d, want 1: untracked reads must not subscribe", runs)
	}
}

func TestFlushSkipsEffectsDisposedEarlierInThePass(t *testing.T) {
	dep := State(0)
	staleRuns := 0
	dispose := observe(func() {
		dep.Get()
		gen := Untrack(dep.Get)
		observe(func() {
			dep.Get()
			if gen != Untrack(dep.Get) {
				staleRuns++ // a child from an older parent run fired
			}
		})
	})
	defer dispose()
	dep.Set(1)
	effects.flush()
	if staleRuns != 0 {
		t.Fatalf("a disposed child effect ran %d times", staleRuns)
	}
}

func TestReactiveRebuildsWithoutParent(t *testing.T) {
	leaf := State("a")
	parentBuilds, leafBuilds := 0, 0
	var root Widget
	dispose := observe(func() {
		parentBuilds++
		root = Column(Reactive(func() Widget { leafBuilds++; return Text(leaf.Get()) }))
	})
	defer dispose()
	leaf.Set("b")
	effects.flush()
	if parentBuilds != 1 || leafBuilds != 2 {
		t.Fatalf("parentBuilds = %d, leafBuilds = %d; want 1 and 2", parentBuilds, leafBuilds)
	}
	if got := root.(*ColumnWidget).children[0].(*ComponentWidget).child.(*TextWidget).value; got != "b" {
		t.Fatalf("leaf shows %q, want b", got)
	}
}

func TestComponentRunsSetupOnceAndKeepsState(t *testing.T) {
	setups, builds := 0, 0
	var local *StateValue[int]
	var c *ComponentWidget
	dispose := observe(func() {
		c = Component(func() Widget {
			setups++
			local = State(0)
			return Reactive(func() Widget { builds++; return Box().Size(float64(local.Get()), 1) })
		})
	})
	defer dispose()
	c.Layout(Loose(Sz(100, 100)), Env{}) // mounts
	local.Set(7)
	effects.flush()
	if setups != 1 || builds != 2 {
		t.Fatalf("setups = %d, builds = %d; want 1 and 2", setups, builds)
	}
	if got := c.Layout(Loose(Sz(100, 100)), Env{}); got.W != 7 {
		t.Fatalf("component laid out at %v wide, want 7 from its own state", got.W)
	}
}

func TestComponentSetupDoesNotSubscribeParent(t *testing.T) {
	dep := State(0)
	parentBuilds := 0
	var c *ComponentWidget
	dispose := observe(func() {
		parentBuilds++
		c = Component(func() Widget {
			dep.Get() // read during setup
			return Reactive(func() Widget { return Box() })
		})
	})
	defer dispose()
	c.Layout(Loose(Sz(100, 100)), Env{})
	dep.Set(1)
	effects.flush()
	if parentBuilds != 1 {
		t.Fatalf("parent rebuilt %d times on a setup read, want 1", parentBuilds)
	}
}

func TestComponentDisposedWithParent(t *testing.T) {
	parentDep, leaf := State(0), State(0)
	setups, builds := 0, 0
	var c *ComponentWidget
	dispose := observe(func() {
		parentDep.Get()
		c = Component(func() Widget {
			setups++
			return Reactive(func() Widget { builds++; leaf.Get(); return Box() })
		})
	})
	defer dispose()
	c.Layout(Loose(Sz(100, 100)), Env{})
	parentDep.Set(1)
	effects.flush()
	c.Layout(Loose(Sz(100, 100)), Env{})
	builds = 0
	leaf.Set(1)
	effects.flush()
	if setups != 2 || builds != 1 {
		t.Fatalf("setups = %d, builds = %d; want 2 setups and 1 live builder", setups, builds)
	}
}

func TestPanicInEffectLeavesNoResidue(t *testing.T) {
	effects.mu.Lock()
	before := effects.count
	effects.mu.Unlock()
	func() {
		defer func() { recover() }()
		observe(func() {
			observe(func() {})
			panic("boom")
		})
	}()
	if o := currentOwner(); o != nil {
		t.Fatal("owner left set after a panicking effect")
	}
	effects.mu.Lock()
	after := effects.count
	effects.mu.Unlock()
	if after != before {
		t.Fatalf("%d effects left registered after a panicking effect, want 0", after-before)
	}
}

func TestDisposedChildrenLeaveOwnerInCreationOrder(t *testing.T) {
	var owner *effect
	var stops []func()
	var cleaned []int
	dispose := Root(func() {
		owner = currentOwner()
		for i := range 5 {
			stops = append(stops, observe(func() {
				OnCleanup(func() { cleaned = append(cleaned, i) })
			}))
		}
	})
	defer dispose()
	// Remove the middle, first, and last child; only 1 and 3 must remain.
	for _, i := range []int{2, 0, 4} {
		stops[i]()
	}
	if owner.firstChild == nil || owner.lastChild == nil ||
		owner.firstChild.nextSibling != owner.lastChild ||
		owner.lastChild.prevSibling != owner.firstChild ||
		owner.firstChild.prevSibling != nil || owner.lastChild.nextSibling != nil {
		t.Fatal("disposed children remain linked or surviving order changed")
	}
	cleaned = nil
	dispose()
	if len(cleaned) != 2 || cleaned[0] != 1 || cleaned[1] != 3 {
		t.Fatalf("remaining cleanup order = %v, want [1 3]", cleaned)
	}
	if owner.firstChild != nil || owner.lastChild != nil {
		t.Fatal("closed owner retains children")
	}
}

func TestChildCleanupCanDisposeSibling(t *testing.T) {
	var stopSibling func()
	var cleaned []int
	dispose := Root(func() {
		observe(func() { OnCleanup(func() { cleaned = append(cleaned, 0); stopSibling() }) })
		stopSibling = observe(func() { OnCleanup(func() { cleaned = append(cleaned, 1) }) })
		observe(func() { OnCleanup(func() { cleaned = append(cleaned, 2) }) })
	})
	defer dispose()
	dispose()
	if len(cleaned) != 3 || cleaned[0] != 0 || cleaned[1] != 1 || cleaned[2] != 2 {
		t.Fatalf("cleanup skipped or repeated a sibling: %v", cleaned)
	}
}

func TestEffectRemovalPreservesExecutionOrder(t *testing.T) {
	n := State(0)
	var seen []int
	var stops []func()
	dispose := Root(func() {
		for i := range 5 {
			stops = append(stops, observe(func() { n.Get(); seen = append(seen, i) }))
		}
	})
	defer dispose()
	stops[1]()
	stops[3]()
	seen = nil
	n.Set(1)
	if !effects.flush() {
		t.Fatal("effects did not settle")
	}
	if len(seen) != 3 || seen[0] != 0 || seen[1] != 2 || seen[2] != 4 {
		t.Fatalf("effect order = %v, want [0 2 4]", seen)
	}
}

// mountProps holds a slice, so == cannot compare it, and declares its own
// equality instead.
type mountProps struct {
	Title string
	Tags  []string
}

func (p mountProps) Equal(o mountProps) bool {
	return p.Title == o.Title && slices.Equal(p.Tags, o.Tags)
}

func TestEqualDerivedPropsDoNotRebuild(t *testing.T) {
	tick := State(0)
	builds := 0
	p := ProbeBuilder(func() Widget {
		props := Derived(func() mountProps { tick.Get(); return mountProps{Title: "a", Tags: []string{"x"}} })
		return View(props, func(value mountProps) Widget { builds++; return Text(value.Title) })
	}, Sz(200, 200))
	defer p.Close()
	p.Frame()
	for range 3 {
		tick.Update(func(n int) int { return n + 1 })
		p.Frame()
	}
	if builds != 1 {
		t.Fatalf("equal props rebuilt %d times", builds)
	}
}

// A type without an Equal method and without comparable fields notifies on
// every write, as it always has: there is nothing to compare it with.
func TestSignalWithoutEqualityNotifiesEveryWrite(t *testing.T) {
	s := State([]int{1})
	runs := 0
	defer observe(func() { s.Get(); runs++ })()
	s.Set([]int{1})
	effects.flush()
	if runs != 2 {
		t.Fatalf("effect ran %d times, want a notification for an uncomparable write", runs)
	}
}
