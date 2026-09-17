package ggui

import "testing"

func TestInnerEffectDisposedWhenOuterReruns(t *testing.T) {
	outerDep, innerDep := State(0), State(0)
	innerRuns := 0
	dispose := Effect(func() {
		outerDep.Get()
		Effect(func() { innerDep.Get(); innerRuns++ })
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
	dispose := Effect(func() {
		rebuild.Get()
		Derived(func() int { computes++; return n.Get() * 2 })
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
	dispose := Effect(func() {
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
	dispose := Effect(func() {
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
	dispose := Effect(func() {
		runs++
		Untrack(func() { a.Get() })
		b.Peek()
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
	dispose := Effect(func() {
		dep.Get()
		gen := dep.Peek()
		Effect(func() {
			dep.Get()
			if gen != dep.Peek() {
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
	dispose := Effect(func() {
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
	var local *Signal[int]
	var c *ComponentWidget
	dispose := Effect(func() {
		c = Component(func() Builder {
			setups++
			local = State(0)
			return func() Widget { builds++; return Box().Size(float64(local.Get()), 1) }
		})
	})
	defer dispose()
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
	dispose := Effect(func() {
		parentBuilds++
		Component(func() Builder {
			dep.Get() // read during setup
			return func() Widget { return Box() }
		})
	})
	defer dispose()
	dep.Set(1)
	effects.flush()
	if parentBuilds != 1 {
		t.Fatalf("parent rebuilt %d times on a setup read, want 1", parentBuilds)
	}
}

func TestComponentDisposedWithParent(t *testing.T) {
	parentDep, leaf := State(0), State(0)
	setups, builds := 0, 0
	dispose := Effect(func() {
		parentDep.Get()
		Component(func() Builder {
			setups++
			return func() Widget { builds++; leaf.Get(); return Box() }
		})
	})
	defer dispose()
	parentDep.Set(1)
	effects.flush()
	builds = 0
	leaf.Set(1)
	effects.flush()
	if setups != 2 || builds != 1 {
		t.Fatalf("setups = %d, builds = %d; want 2 setups and 1 live builder", setups, builds)
	}
}

func TestPanicInEffectLeavesNoResidue(t *testing.T) {
	effects.mu.Lock()
	before := len(effects.list)
	effects.mu.Unlock()
	func() {
		defer func() { recover() }()
		Effect(func() {
			Effect(func() {})
			panic("boom")
		})
	}()
	if o := currentOwner(); o != nil {
		t.Fatal("owner left set after a panicking effect")
	}
	effects.mu.Lock()
	after := len(effects.list)
	effects.mu.Unlock()
	if after != before {
		t.Fatalf("%d effects left registered after a panicking effect, want 0", after-before)
	}
}
