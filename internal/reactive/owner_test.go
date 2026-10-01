package reactive

import (
	"slices"
	"strings"
	"testing"
)

func TestEffectRequiresAnOwner(t *testing.T) {
	t.Parallel()
	msg, _ := panicOf(func() { Effect(func() Cleanup { return nil }) }).(string)
	if !strings.Contains(msg, "requires an owner") {
		t.Fatalf("Effect with no owner panicked with %q, want a message saying it needs one", msg)
	}
	msg, _ = panicOf(func() { OnCleanup(func() {}) }).(string)
	if !strings.Contains(msg, "requires an owner") {
		t.Fatalf("OnCleanup with no owner panicked with %q, want a message saying it needs one", msg)
	}
}

// A user effect waits for the post-layout pass, and its cleanup runs before
// each re-run and once on disposal.
func TestEffectRunsAfterLayoutAndCleansUpBeforeEachRun(t *testing.T) {
	t.Parallel()
	rt := NewRuntime()
	n := State(0)
	var log []string
	dispose := rt.Root(func() {
		Effect(func() Cleanup {
			v := n.Get()
			log = append(log, "run", string(rune('0'+v)))
			return func() { log = append(log, "cleanup") }
		})
	})
	if len(log) != 0 {
		t.Fatal("Effect ran before the post-layout pass")
	}
	if !rt.FlushUsers(nil) {
		t.Fatal("the first pass reported that no effect ran")
	}
	if rt.FlushUsers(nil) {
		t.Fatal("a pass with nothing changed reported a run")
	}
	n.Set(1)
	rt.Flush() // internal bindings only: the user effect stays pending
	if len(log) != 2 {
		t.Fatalf("Flush ran a user effect: %v", log)
	}
	rt.FlushUsers(nil)
	dispose()
	if want := []string{"run", "0", "cleanup", "run", "1", "cleanup"}; !slices.Equal(log, want) {
		t.Fatalf("effect log = %v, want %v", log, want)
	}
	if rt.Count() != 0 {
		t.Fatalf("disposing the root left %d computations", rt.Count())
	}
}

func TestWatchPassesTheValue(t *testing.T) {
	t.Parallel()
	n := State("a")
	var seen []string
	dispose := Root(func() { Watch[string](n, func(v string) { seen = append(seen, v) }) })
	defer dispose()
	FlushUsers(nil)
	n.Set("b")
	FlushUsers(nil)
	if !slices.Equal(seen, []string{"a", "b"}) {
		t.Fatalf("Watch saw %v, want [a b]", seen)
	}
}

// An effect belongs to its owner's frame loop, and a pass for another loop
// leaves it pending rather than dropping it.
func TestFlushUsersRunsOnlyTheGivenLoopsEffects(t *testing.T) {
	t.Parallel()
	loopA, loopB := new(int), new(int)
	runs := 0
	var eff *Computation
	dispose := Root(func() {
		CurrentOwner().SetLoop(loopA)
		Effect(func() Cleanup { runs++; return nil })
		eff = CurrentOwner().Children()[0]
	})
	defer dispose()
	if eff.Loop() != loopA {
		t.Fatal("the effect did not inherit its owner's loop")
	}
	if FlushUsers(loopB) || runs != 0 {
		t.Fatal("another loop's pass ran the effect")
	}
	if !FlushUsers(loopA) || runs != 1 {
		t.Fatalf("its own loop's pass ran the effect %d times, want 1", runs)
	}
}

func TestCleanupsRunInReverseOrderUntrackedAndOwnerless(t *testing.T) {
	t.Parallel()
	n, other := State(0), State(0)
	var order []int
	var ownerInCleanup *Computation
	runs := 0
	dispose := Observe(func() {
		n.Get()
		runs++
		for i := range 3 {
			OnCleanup(func() {
				order = append(order, i)
				other.Get()
				ownerInCleanup = CurrentOwner()
			})
		}
	})
	defer dispose()
	n.Set(1)
	Flush()
	if !slices.Equal(order, []int{2, 1, 0}) {
		t.Fatalf("cleanups ran in order %v, want [2 1 0]", order)
	}
	if ownerInCleanup != nil {
		t.Fatal("a cleanup ran with an owner")
	}
	runs = 0
	other.Set(1)
	Flush()
	if runs != 0 {
		t.Fatal("a read in a cleanup subscribed the effect")
	}
}

// Registering a cleanup on an owner that is already gone runs it at once:
// there is no later disposal to wait for.
func TestOnCleanupOnADisposedOwnerRunsImmediately(t *testing.T) {
	t.Parallel()
	c, dispose := EffectWith(func() {})
	dispose()
	if !c.Disposed() {
		t.Fatal("the disposed computation does not report Disposed")
	}
	ran := false
	WithOwner(c, func() { OnCleanup(func() { ran = true }) })
	if !ran {
		t.Fatal("a cleanup registered on a disposed owner did not run")
	}
}

// A re-run disposes what the last run created, except roots, which live
// until they are disposed or their owner is: that is what lets a keyed
// list keep its rows across its own re-runs. A RootWith is not persistent.
func TestRerunDisposesChildrenButKeepsRoots(t *testing.T) {
	t.Parallel()
	n := State(0)
	var disposed []string
	var kept func()
	first := true
	stop := Observe(func() {
		n.Get()
		Observe(func() { OnCleanup(func() { disposed = append(disposed, "child") }) })
		RootWith(nil, "", func() { OnCleanup(func() { disposed = append(disposed, "keyed") }) })
		if first {
			first = false
			kept = Root(func() { OnCleanup(func() { disposed = append(disposed, "root") }) })
		}
	})
	n.Set(1)
	Flush()
	if !slices.Equal(disposed, []string{"child", "keyed"}) {
		t.Fatalf("a re-run disposed %v, want [child keyed]", disposed)
	}
	disposed = nil
	stop()
	if !slices.Contains(disposed, "root") {
		t.Fatalf("disposing the owner disposed %v, not the root it kept", disposed)
	}
	kept() // already gone with its owner; disposing again is harmless
}

func TestRootDisposesItselfWhenItsFunctionPanics(t *testing.T) {
	t.Parallel()
	before := Count()
	cleaned := false
	v := panicOf(func() {
		Root(func() {
			OnCleanup(func() { cleaned = true })
			Observe(func() {})
			panic("boom")
		})
	})
	if v != "boom" {
		t.Fatalf("Root swallowed or replaced the panic: %v", v)
	}
	if !cleaned || Count() != before || CurrentOwner() != nil {
		t.Fatal("a panicking Root left its cleanup unrun, a computation registered or its owner set")
	}
}

func TestUntrackReadsWithoutSubscribingButKeepsTheOwner(t *testing.T) {
	t.Parallel()
	tracked, untracked := State(0), State(0)
	runs := 0
	var owners []*Computation
	var self *Computation
	dispose := Observe(func() {
		runs++
		self = CurrentOwner()
		tracked.Get()
		Untrack(func() int {
			owners = append(owners, CurrentOwner())
			return untracked.Get()
		})
	})
	defer dispose()
	untracked.Set(1)
	Flush()
	if runs != 1 {
		t.Fatal("a write to an untracked read re-ran the effect")
	}
	tracked.Set(1)
	Flush()
	if runs != 2 {
		t.Fatal("the tracked read no longer subscribes after an Untrack")
	}
	if owners[0] == nil || owners[1] != self {
		t.Fatal("Untrack dropped the owner")
	}
	if Untrack(func() int { return 7 }) != 7 {
		t.Fatal("Untrack outside an effect did not return fn's value")
	}
}

// Two effects that each write what the other reads never settle: Flush
// gives up after MaxFlushPasses and Unsettled names both.
func TestFlushReportsACycle(t *testing.T) {
	t.Parallel()
	rt := NewRuntime()
	x, y := State(0), State(0)
	var a, b *Computation
	dispose := rt.Root(func() {
		a, _ = EffectWith(func() { y.Set(x.Get() + 1) })
		b, _ = EffectWith(func() { x.Set(y.Get() + 1) })
	})
	defer dispose()
	if rt.Flush() {
		t.Fatal("Flush settled a cycle")
	}
	stuck, total := rt.Unsettled()
	if !slices.Contains(stuck, a) || !slices.Contains(stuck, b) {
		t.Fatalf("Unsettled named %d computations, want both halves of the cycle", len(stuck))
	}
	if total < 2 {
		t.Fatalf("Unsettled counted %d computations in all, want at least 2", total)
	}
	if Debug != (a.Origin() != "") {
		t.Fatalf("Origin = %q with Debug %v", a.Origin(), Debug)
	}
	if Debug && !strings.Contains(a.Origin(), "owner_test.go") {
		t.Fatalf("Origin = %q, want this test file", a.Origin())
	}
}

func TestRuntimeParentAndRelated(t *testing.T) {
	t.Parallel()
	rt := NewRuntime()
	if rt.Parent() != Base() || Base().Parent() != nil {
		t.Fatal("a runtime's parent is not the Base it was made beside")
	}
	if got := rt.Related(); got != [3]*Runtime{rt, Base(), nil} {
		t.Fatalf("Related = %v, want the runtime and its Base once each", got)
	}
	done := make(chan [3]*Runtime)
	go func() { done <- rt.Related() }()
	other := <-done
	if other[0] != rt || other[1] != Base() || other[2] == nil || other[2] == Base() {
		t.Fatal("on another goroutine Related does not add that goroutine's Base")
	}
}

// AutoID derives a widget's identity from the keyed component it is built
// in and its place there, so a rebuild gives the same widget the same id,
// and a named root keeps its ids whatever order it is built in.
func TestAutoIDIsStableAcrossRebuilds(t *testing.T) {
	t.Parallel()
	if AutoID() != nil {
		t.Fatal("AutoID outside any owner is not nil")
	}
	Root(func() {
		if AutoID() != nil {
			t.Error("AutoID outside a keyed component is not nil")
		}
	})()

	order := State([]string{"a", "b"})
	ids := map[string][]any{}
	var own []any
	dispose := RootWith("component", "", func() {
		Observe(func() {
			own = append(own, AutoID(), AutoID())
			for _, key := range order.Get() {
				RootWith(nil, key, func() { ids[key] = append(ids[key], AutoID()) })
			}
		})
	})
	defer dispose()
	order.Set([]string{"b", "a"})
	Flush()

	if own[0] == own[1] {
		t.Fatal("two widgets built in one run got the same id")
	}
	if own[0] != own[2] || own[1] != own[3] {
		t.Fatalf("a rebuild changed the ids: %v", own)
	}
	for key, got := range ids {
		if len(got) != 2 || got[0] != got[1] || got[0] == nil {
			t.Fatalf("keyed root %q got ids %v across a reorder, want one stable id", key, got)
		}
	}
	if ids["a"][0] == ids["b"][0] {
		t.Fatal("two keyed roots got the same id")
	}
	other := RootWith("another", "", func() { own = append(own, AutoID()) })
	defer other()
	if own[len(own)-1] == own[0] {
		t.Fatal("two component instances gave the same id")
	}
}
