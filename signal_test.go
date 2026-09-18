package ggui

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestSignalGetSet(t *testing.T) {
	s := State(1)
	if got := s.Get(); got != 1 {
		t.Fatalf("Get() = %d, want 1", got)
	}
	s.Update(func(n int) int { return n + 41 })
	if got := s.Get(); got != 42 {
		t.Fatalf("Get() = %d, want 42", got)
	}
}

func TestEffectRerunsOnChange(t *testing.T) {
	s := State("a")
	var seen []string
	dispose := observe(func() { seen = append(seen, s.Get()) })
	defer dispose()

	if len(seen) != 1 || seen[0] != "a" {
		t.Fatalf("effect did not run once on registration: %v", seen)
	}

	s.Set("b")
	effects.flush()
	effects.flushUsers(nil)
	if len(seen) != 2 || seen[1] != "b" {
		t.Fatalf("effect did not re-run after Set: %v", seen)
	}

	// A flush with nothing dirty must not re-run the effect.
	effects.flush()
	effects.flushUsers(nil)
	if len(seen) != 2 {
		t.Fatalf("effect re-ran while clean: %v", seen)
	}
}

func TestIdleEffectFlushDoesNotAllocate(t *testing.T) {
	s := State(0)
	dispose := Root(func() {
		for range 100 {
			observe(func() { s.Get() })
		}
	})
	defer dispose()
	if !effects.flush() {
		t.Fatal("initial flush did not settle")
	}
	if got := testing.AllocsPerRun(100, func() { effects.flush() }); got != 0 {
		t.Fatalf("idle flush allocated %g times", got)
	}
}

func TestEffectFlushIncludesCleanupWrites(t *testing.T) {
	trigger, result := State(0), State(0)
	var seen []int
	dispose := Root(func() {
		// The observer has already run when a later effect's cleanup
		// writes, so settling requires another pass in creation order.
		observe(func() { seen = append(seen, result.Get()) })
		observe(func() {
			trigger.Get()
			OnCleanup(func() { result.Set(Untrack(trigger.Get)) })
		})
	})
	defer dispose()
	effects.flush()
	effects.flushUsers(nil)
	for _, v := range []int{1, 2} {
		trigger.Set(v)
		if !effects.flush() {
			t.Fatal("cleanup write did not settle")
		}
		effects.flush()
		effects.flushUsers(nil)
	}
	if !slices.Equal(seen, []int{0, 1, 2}) {
		t.Fatalf("observer saw %v, want [0 1 2]", seen)
	}
}

func TestEffectFlushIncludesWritesAfterNestedFlush(t *testing.T) {
	trigger, before, after := State(0), State(0), State(0)
	var seenBefore, seenAfter []int
	dispose := Root(func() {
		observe(func() { seenBefore = append(seenBefore, before.Get()) })
		observe(func() { seenAfter = append(seenAfter, after.Get()) })
		observe(func() {
			if v := trigger.Get(); v != 0 {
				before.Set(v)
				if !effects.flush() {
					t.Fatal("nested flush did not settle")
				}
				after.Set(v)
			}
		})
	})
	defer dispose()
	effects.flush()
	effects.flushUsers(nil)
	trigger.Set(1)
	if !effects.flush() {
		t.Fatal("outer flush did not settle")
	}
	if !slices.Equal(seenBefore, []int{0, 1}) || !slices.Equal(seenAfter, []int{0, 1}) {
		t.Fatalf("observers saw %v and %v, want [0 1] each", seenBefore, seenAfter)
	}
}

func TestEffectCycleIsNotRecordedAsSettled(t *testing.T) {
	effects.flush()
	effects.flushUsers(nil)
	n := State(0)
	dispose := observe(func() { n.Set(n.Get() + 1) })
	defer dispose()
	for range 2 {
		before := Untrack(n.Get)
		if effects.flush() {
			t.Fatal("cyclic effect was considered settled")
		}
		if Untrack(n.Get) <= before {
			t.Fatal("flush skipped a cycle left pending by the previous flush")
		}
	}
}

func TestEffectDisposeStopsUpdates(t *testing.T) {
	s := State(0)
	runs := 0
	dispose := observe(func() { s.Get(); runs++ })
	dispose()

	s.Set(1)
	effects.flush()
	effects.flushUsers(nil)
	if runs != 1 {
		t.Fatalf("runs = %d, want 1 after dispose", runs)
	}
}

func TestUntrackedReadDoesNotSubscribe(t *testing.T) {
	s := State(0)
	s.Get() // outside any effect
	s.Set(1)
	effects.flush()
	effects.flushUsers(nil) // must not panic on a nil subscriber
}

func TestSetSkipsEqualValue(t *testing.T) {
	s := State(1)
	runs := 0
	dispose := observe(func() { s.Get(); runs++ })
	defer dispose()

	s.Set(1)
	effects.flush()
	effects.flushUsers(nil)
	if runs != 1 {
		t.Fatalf("runs = %d, want 1 after writing an equal value", runs)
	}
	s.Set(2)
	effects.flush()
	effects.flushUsers(nil)
	if runs != 2 {
		t.Fatalf("runs = %d, want 2 after writing a new value", runs)
	}
}

func TestSetNotifiesForUncomparableValue(t *testing.T) {
	s := State([]int{1})
	runs := 0
	dispose := observe(func() { s.Get(); runs++ })
	defer dispose()

	s.Set([]int{1})
	effects.flush()
	effects.flushUsers(nil)
	if runs != 2 {
		t.Fatalf("runs = %d, want 2: slices have no default equality", runs)
	}
}

func TestWithEqualOverridesComparison(t *testing.T) {
	s := State([]int{1}).WithEqual(slices.Equal)
	runs := 0
	dispose := observe(func() { s.Get(); runs++ })
	defer dispose()

	s.Set([]int{1})
	effects.flush()
	effects.flushUsers(nil)
	if runs != 1 {
		t.Fatalf("runs = %d, want 1 with a custom equality", runs)
	}
}

func TestSignalMapDerivesValue(t *testing.T) {
	n := State(2)
	label := n.Map(func(v int) string { return strconv.Itoa(v * 10) })
	defer label.Dispose()

	if got := label.Get(); got != "20" {
		t.Fatalf("Get() = %q, want %q", got, "20")
	}
	n.Set(3)
	effects.flush()
	effects.flushUsers(nil)
	if got := label.Get(); got != "30" {
		t.Fatalf("Get() = %q, want %q after Set", got, "30")
	}
}

func TestMemoChainSettlesInOneFlush(t *testing.T) {
	n := State(1)
	doubled := n.Map(func(v int) int { return v * 2 })
	defer doubled.Dispose()
	label := doubled.Map(func(v int) string { return strconv.Itoa(v) })
	defer label.Dispose()

	var seen []string
	dispose := observe(func() { seen = append(seen, label.Get()) })
	defer dispose()

	n.Set(21)
	effects.flush()
	effects.flushUsers(nil)
	if len(seen) != 2 || seen[1] != "42" {
		t.Fatalf("seen = %v, want the chain to settle in one flush", seen)
	}
}

func TestMemoRecomputesOnlyWhenSourceChanges(t *testing.T) {
	n := State(1)
	runs := 0
	m := n.Map(func(v int) int { runs++; return v % 2 })
	defer m.Dispose()

	var seen []int
	dispose := observe(func() { seen = append(seen, m.Get()) })
	defer dispose()

	n.Set(3) // different source, same result: the reader must not re-run
	effects.flush()
	effects.flushUsers(nil)
	if runs != 2 {
		t.Fatalf("runs = %d, want 2 recomputes", runs)
	}
	if len(seen) != 1 {
		t.Fatalf("seen = %v, want the reader left alone for an unchanged result", seen)
	}
}

func TestMemoDisposeStopsRecomputation(t *testing.T) {
	n := State(1)
	m := n.Map(func(v int) int { return v + 1 })
	m.Get()
	m.Dispose()

	n.Set(10)
	effects.flush()
	effects.flushUsers(nil)
	if got := m.Get(); got != 2 {
		t.Fatalf("Get() = %d, want the last value 2 after Dispose", got)
	}
}

func TestToggleAndAdd(t *testing.T) {
	on := State(false)
	n := State(1.5)
	Toggle(on)
	Add(n, 2)
	if !on.Get() || n.Get() != 3.5 {
		t.Fatalf("on = %v, n = %v; want true, 3.5", on.Get(), n.Get())
	}
}

func TestCombineDerivesFromTwoSources(t *testing.T) {
	a := State(2)
	b := State("x")
	joined := Combine(a, b, func(n int, s string) string { return strings.Repeat(s, n) })
	defer joined.Dispose()

	if got := joined.Get(); got != "xx" {
		t.Fatalf("Get() = %q, want %q", got, "xx")
	}
	b.Set("ab")
	effects.flush()
	effects.flushUsers(nil)
	if got := joined.Get(); got != "abab" {
		t.Fatalf("Get() = %q, want %q", got, "abab")
	}
}

func TestWatchRunsOnChange(t *testing.T) {
	s := State(0)
	var seen []int
	dispose := Root(func() { Watch(s, func(v int) { seen = append(seen, v) }) })
	effects.flushUsers(nil)
	defer dispose()

	s.Set(1)
	effects.flush()
	effects.flushUsers(nil)
	if len(seen) != 2 || seen[0] != 0 || seen[1] != 1 {
		t.Fatalf("seen = %v, want [0 1]", seen)
	}
}

func TestSignalWritesAdvanceTheLayoutGeneration(t *testing.T) {
	s := State(1)
	before := layoutGen.Load()
	s.Set(1)
	if layoutGen.Load() != before {
		t.Fatal("an equal write must not request layout")
	}
	s.Set(2)
	if layoutGen.Load() != before+1 {
		t.Fatal("a write must request layout")
	}
	Invalidate(Env{})
	if layoutGen.Load() != before+2 {
		t.Fatal("Invalidate must request layout")
	}
}

func TestAppendAndRemoveOnSliceSignals(t *testing.T) {
	s := State([]int{1, 2, 3})
	runs := 0
	dispose := observe(func() { s.Get(); runs++ })
	defer dispose()
	Append(s, 4, 5)
	effects.flush()
	effects.flushUsers(nil)
	Remove(s, func(n int) bool { return n%2 == 0 })
	effects.flush()
	effects.flushUsers(nil)
	if got := Untrack(s.Get); len(got) != 3 || got[0] != 1 || got[1] != 3 || got[2] != 5 {
		t.Fatalf("slice = %v, want [1 3 5]", got)
	}
	if runs != 3 {
		t.Fatalf("effect ran %d times, want 3 (initial, append, remove)", runs)
	}
	Remove(s, func(int) bool { return false })
	effects.flush()
	effects.flushUsers(nil)
	if runs != 3 {
		t.Fatalf("effect ran %d times after a Remove that matched nothing, want still 3", runs)
	}
}

// A reader of two derived values must never see one of them updated and the
// other not, whatever order the effects were created in. Here the Watch is
// registered before the memo it reads, which is what an effect that outlives
// a rebuilt memo ends up looking like.
func TestReaderSeesNoPartialUpdate(t *testing.T) {
	s := State(0)
	var b *DerivedValue[int]
	var seen [][2]int

	a := Derived(func() int { return s.Get() * 10 })
	defer a.Dispose()
	defer Root(func() {
		Watch(a, func(av int) {
			if b != nil {
				seen = append(seen, [2]int{av, b.Get()})
			}
		})
	})()
	b = Derived(func() int { return s.Get() * 100 })
	defer b.Dispose()

	s.Set(1)
	effects.flush()
	effects.flushUsers(nil)
	if len(seen) != 1 || seen[0] != [2]int{10, 100} {
		t.Fatalf("watch saw %v, want one call with [10 100]", seen)
	}
}

// The same, with the second value two derivations away from the signal: the
// far memo is not itself marked by the write, only checked through the one
// above it.
func TestReaderSeesNoPartialUpdateThroughChain(t *testing.T) {
	s := State(0)
	var far *DerivedValue[int]
	var seen [][2]int

	a := Derived(func() int { return s.Get() * 10 })
	defer a.Dispose()
	defer Root(func() {
		Watch(a, func(av int) {
			if far != nil {
				seen = append(seen, [2]int{av, far.Get()})
			}
		})
	})()
	near := Derived(func() int { return s.Get() * 100 })
	defer near.Dispose()
	far = near.Map(func(v int) int { return v + 1 })
	defer far.Dispose()

	s.Set(1)
	effects.flush()
	effects.flushUsers(nil)
	if len(seen) != 1 || seen[0] != [2]int{10, 101} {
		t.Fatalf("watch saw %v, want one call with [10 101]", seen)
	}
}

// A memo read from outside any effect, between writes, reports the value its
// inputs imply now rather than the one the last flush left behind.
func TestMemoReadOutsideEffectIsCurrent(t *testing.T) {
	s := State(2)
	m := Derived(func() int { return s.Get() * 3 })
	defer m.Dispose()
	if got := m.Get(); got != 6 {
		t.Fatalf("got %d, want 6", got)
	}
	s.Set(5)
	if got := m.Get(); got != 15 {
		t.Fatalf("got %d before flush, want 15", got)
	}
}
