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
	dispose := Effect(func() { seen = append(seen, s.Get()) })
	defer dispose()

	if len(seen) != 1 || seen[0] != "a" {
		t.Fatalf("effect did not run once on registration: %v", seen)
	}

	s.Set("b")
	effects.flush()
	if len(seen) != 2 || seen[1] != "b" {
		t.Fatalf("effect did not re-run after Set: %v", seen)
	}

	// A flush with nothing dirty must not re-run the effect.
	effects.flush()
	if len(seen) != 2 {
		t.Fatalf("effect re-ran while clean: %v", seen)
	}
}

func TestEffectDisposeStopsUpdates(t *testing.T) {
	s := State(0)
	runs := 0
	dispose := Effect(func() { s.Get(); runs++ })
	dispose()

	s.Set(1)
	effects.flush()
	if runs != 1 {
		t.Fatalf("runs = %d, want 1 after dispose", runs)
	}
}

func TestUntrackedReadDoesNotSubscribe(t *testing.T) {
	s := State(0)
	s.Get() // outside any effect
	s.Set(1)
	effects.flush() // must not panic on a nil subscriber
}

func TestSetSkipsEqualValue(t *testing.T) {
	s := State(1)
	runs := 0
	dispose := Effect(func() { s.Get(); runs++ })
	defer dispose()

	s.Set(1)
	effects.flush()
	if runs != 1 {
		t.Fatalf("runs = %d, want 1 after writing an equal value", runs)
	}
	s.Set(2)
	effects.flush()
	if runs != 2 {
		t.Fatalf("runs = %d, want 2 after writing a new value", runs)
	}
}

func TestSetNotifiesForUncomparableValue(t *testing.T) {
	s := State([]int{1})
	runs := 0
	dispose := Effect(func() { s.Get(); runs++ })
	defer dispose()

	s.Set([]int{1})
	effects.flush()
	if runs != 2 {
		t.Fatalf("runs = %d, want 2: slices have no default equality", runs)
	}
}

func TestWithEqualOverridesComparison(t *testing.T) {
	s := State([]int{1}).WithEqual(slices.Equal)
	runs := 0
	dispose := Effect(func() { s.Get(); runs++ })
	defer dispose()

	s.Set([]int{1})
	effects.flush()
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
	dispose := Effect(func() { seen = append(seen, label.Get()) })
	defer dispose()

	n.Set(21)
	effects.flush()
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
	dispose := Effect(func() { seen = append(seen, m.Get()) })
	defer dispose()

	n.Set(3) // different source, same result: the reader must not re-run
	effects.flush()
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
	m.Dispose()

	n.Set(10)
	effects.flush()
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
	if got := joined.Get(); got != "abab" {
		t.Fatalf("Get() = %q, want %q", got, "abab")
	}
}

func TestWatchRunsOnChange(t *testing.T) {
	s := State(0)
	var seen []int
	dispose := Watch(s, func(v int) { seen = append(seen, v) })
	defer dispose()

	s.Set(1)
	effects.flush()
	if len(seen) != 2 || seen[0] != 0 || seen[1] != 1 {
		t.Fatalf("seen = %v, want [0 1]", seen)
	}
}
