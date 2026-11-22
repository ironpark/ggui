package reactive

import (
	"slices"
	"strings"
	"testing"
)

type uncompErr struct{ items []string }

func (uncompErr) Error() string { return "err" }

type asyncLike struct {
	Val int
	Err error
}

func TestComparableEqualRejectsStructWithInterfaceField(t *testing.T) {
	t.Parallel()
	s := State(asyncLike{Val: 1, Err: uncompErr{items: []string{"a"}}})
	// Must not panic when comparing two structs whose interface field holds an
	// uncomparable dynamic type.
	s.Set(asyncLike{Val: 1, Err: uncompErr{items: []string{"a"}}})
	if got := Untrack(s.Get); got.Val != 1 {
		t.Fatalf("got %+v, want Val=1", got)
	}
}

func TestDerivedStoreDoesNotBumpGensOrFreeWatch(t *testing.T) {
	t.Parallel()
	src := State(1)
	d := Derived(func() int { return src.Get() * 2 })
	defer d.Dispose()

	freeCalls := 0
	WatchFree(func() { freeCalls++ })
	defer WatchFree(nil)

	stopTrace := TraceWrites()
	defer stopTrace()

	st, lay, fr := StateGen(), LayoutGen(), FreeGen()
	if got := d.Get(); got != 2 {
		t.Fatalf("d.Get() = %d, want 2", got)
	}
	if StateGen() != st || LayoutGen() != lay || FreeGen() != fr {
		t.Fatal("lazy Derived recomputation bumped state/layout/free generation")
	}
	if freeCalls != 0 {
		t.Fatalf("lazy Derived recomputation called WatchFree %d times", freeCalls)
	}
	if writes := stopTrace(); len(writes) != 0 {
		t.Fatalf("lazy Derived recomputation recorded %d writes in TraceWrites", len(writes))
	}
}

func TestStoreUnlockOnEqPanicAndReentrantOnFree(t *testing.T) {
	t.Parallel()
	boom := true
	s := State(1).WithEqual(func(a, b int) bool {
		if boom {
			panic("eq panic")
		}
		return a == b
	})
	if v := panicOf(func() { s.Set(2) }); v != "eq panic" {
		t.Fatalf("panic = %v, want eq panic", v)
	}
	boom = false
	// Must not deadlock s.mu after s.eq panicked.
	var observed int
	WatchFree(func() { observed = Untrack(s.Get) })
	defer WatchFree(nil)
	s.Set(3)
	if observed != 3 || Untrack(s.Get) != 3 {
		t.Fatalf("observed=%d val=%d, want 3", observed, Untrack(s.Get))
	}
}

func TestDerivedEvalDuringMeasureDoesNotLeakUpstreamSources(t *testing.T) {
	t.Parallel()
	store := NewStore(&storeModel{title: "a"})
	sel := Select(store, func(m *storeModel) string { return m.title })
	defer sel.Dispose()

	var recorded []LayoutSource
	Measure(func(src LayoutSource, _ uint64) {
		recorded = append(recorded, src)
	}, func() {
		if got := sel.Get(); got != "a" {
			t.Fatalf("sel.Get() = %q, want a", got)
		}
	})
	if len(recorded) != 1 || recorded[0] != sel.sig {
		t.Fatalf("Measure recorded %d sources (%v), want only the Derived cell", len(recorded), recorded)
	}
}

func TestRunPassAndFlushUsersSurvivePanic(t *testing.T) {
	t.Parallel()
	rt := NewRuntime()
	s := State(0)
	boom := true
	var ranB, userB int
	dispose := rt.Root(func() {
		Observe(func() {
			if s.Get() > 0 && boom {
				panic("obs boom")
			}
		})
		Observe(func() {
			if s.Get() > 0 {
				ranB++
			}
		})
		Effect(func() Cleanup {
			if s.Get() > 0 && boom {
				panic("eff boom")
			}
			return nil
		})
		Effect(func() Cleanup {
			if s.Get() > 0 {
				userB++
			}
			return nil
		})
	})
	defer dispose()
	rt.FlushUsers(nil)

	s.Set(1)
	if v := panicOf(func() { rt.Flush() }); v != "obs boom" {
		t.Fatalf("Flush panic = %v, want obs boom", v)
	}
	boom = false
	if !rt.Flush() {
		t.Fatal("Flush after recovered panic did not settle")
	}
	if ranB != 1 {
		t.Fatalf("second observer ran %d times after recovery, want 1", ranB)
	}

	boom = true
	if v := panicOf(func() { rt.FlushUsers(nil) }); v != "eff boom" {
		t.Fatalf("FlushUsers panic = %v, want eff boom", v)
	}
	boom = false
	if !rt.FlushUsers(nil) {
		t.Fatal("FlushUsers after recovered panic did not run remaining effect")
	}
	if userB != 1 {
		t.Fatalf("second user effect ran %d times after recovery, want 1", userB)
	}
}

func TestIndirectCyclicDerivedPanics(t *testing.T) {
	t.Parallel()
	gate := State(false)
	var a, b *DerivedValue[int]
	a = Derived(func() int {
		if gate.Get() {
			return b.Get() + 1
		}
		return 1
	})
	b = Derived(func() int { return a.Get() + 1 })
	defer a.Dispose()
	defer b.Dispose()

	if b.Get() != 2 {
		t.Fatalf("initial b.Get() = %d, want 2", b.Get())
	}
	gate.Set(true)
	msg, _ := panicOf(func() { a.Get() }).(string)
	if !strings.Contains(msg, "cyclic Derived") {
		t.Fatalf("indirect cyclic Derived panicked with %q, want cyclic Derived", msg)
	}
}

func TestDisposedOwnerDoesNotLeakComputations(t *testing.T) {
	t.Parallel()
	rt := NewRuntime()
	var owner *Computation
	dispose := rt.Root(func() { owner = CurrentOwner() })
	dispose()

	before := rt.Count()
	ran := false
	WithOwner(owner, func() {
		Observe(func() { ran = true })
		Effect(func() Cleanup { ran = true; return nil })
		Root(func() {
			Observe(func() { ran = true })
		})
	})
	if ran {
		t.Fatal("computation attached to a disposed owner ran")
	}
	if rt.Count() != before {
		t.Fatalf("disposed owner leaked %d computations", rt.Count()-before)
	}
}

func TestResetAndDisposeCleanUpAfterCleanupPanic(t *testing.T) {
	t.Parallel()
	rt := NewRuntime()
	s := State(0)
	var comp *Computation
	dispose := rt.Root(func() {
		comp, _ = EffectWith(func() {
			s.Get()
			OnCleanup(func() { panic("cleanup boom") })
		})
	})
	if len(s.subs) != 1 || rt.Count() != 1 {
		t.Fatalf("before dispose: subs=%d count=%d", len(s.subs), rt.Count())
	}
	if v := panicOf(dispose); v != "cleanup boom" {
		t.Fatalf("dispose panic = %v, want cleanup boom", v)
	}
	if !comp.Disposed() || len(s.subs) != 0 || rt.Count() != 0 {
		t.Fatalf("after panicking cleanup: disposed=%v subs=%d count=%d", comp.Disposed(), len(s.subs), rt.Count())
	}
}

func TestRefreshSkipsWhenCleanupDisposesComputation(t *testing.T) {
	t.Parallel()
	rt := NewRuntime()
	s := State(0)
	var stop func()
	runs := 0
	var log []string
	rt.Root(func() {
		stop = Observe(func() {
			v := s.Get()
			runs++
			log = append(log, "run")
			if v == 0 {
				OnCleanup(func() {
					log = append(log, "cleanup-dispose")
					stop()
				})
			}
		})
	})
	s.Set(1)
	rt.Flush()
	if runs != 1 || !slices.Equal(log, []string{"run", "cleanup-dispose"}) {
		t.Fatalf("runs=%d log=%v, want 1 and [run cleanup-dispose]", runs, log)
	}
}
