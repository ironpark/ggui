package reactive

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// panicOf runs fn and returns what it panicked with, or nil.
func panicOf(fn func()) (v any) {
	defer func() { v = recover() }()
	fn()
	return nil
}

func TestDerivedIsLazyAndMemoized(t *testing.T) {
	t.Parallel()
	n := State(2)
	runs := 0
	d := Derived(func() int { runs++; return n.Get() * 10 })
	defer d.Dispose()
	if runs != 0 {
		t.Fatal("Derived ran before anyone read it")
	}
	if first, second := d.Get(), d.Get(); first != 20 || second != 20 || runs != 1 {
		t.Fatalf("two reads ran fn %d times, want 1", runs)
	}
	n.Set(3)
	if runs != 1 {
		t.Fatal("a write recomputed an unread Derived eagerly")
	}
	if d.Get() != 30 || runs != 2 {
		t.Fatalf("the read after a write gave %d with %d runs, want 30 and 2", d.Get(), runs)
	}
	if !d.eff.Derived() {
		t.Fatal("a memo's computation does not report Derived")
	}
}

// A memo whose result did not change stops the change there: its readers
// are checked, found current, and left alone.
func TestDerivedCutsOffUnchangedResults(t *testing.T) {
	t.Parallel()
	n := State(1)
	parity := Map(n, func(v int) bool { return v%2 == 0 })
	defer parity.Dispose()
	runs := counter(t, parity)
	n.Set(3)
	Flush()
	if *runs != 0 {
		t.Fatalf("an unchanged parity ran its reader %d times", *runs)
	}
	n.Set(4)
	Flush()
	if *runs != 1 {
		t.Fatalf("a changed parity ran its reader %d times, want 1", *runs)
	}

	always := Map(n, func(v int) bool { return true }).WithEqual(nil)
	defer always.Dispose()
	alwaysRuns := counter(t, always)
	n.Set(5)
	Flush()
	if *alwaysRuns != 1 {
		t.Fatalf("a memo with no equality ran its reader %d times on an equal result, want 1", *alwaysRuns)
	}
}

// Diamond: two memos of one state feed one effect. The effect runs once
// per write and never sees one side updated and the other not.
func TestDiamondRunsTheReaderOnceWithConsistentInputs(t *testing.T) {
	t.Parallel()
	a := State(1)
	double := a.Map(func(v int) int { return v * 2 })
	next := a.Map(func(v int) int { return v + 1 })
	sum := Combine(double, next, func(x, y int) string { return fmt.Sprint(x, "+", y) })
	chain := sum.Map(strings.ToUpper)
	var seen []string
	dispose := Observe(func() { seen = append(seen, chain.Get()+"|"+fmt.Sprint(double.Get(), next.Get())) })
	defer dispose()
	for _, d := range []interface{ Dispose() }{double, next, sum, chain} {
		defer d.Dispose()
	}
	a.Set(5)
	Flush()
	if want := []string{"2+2|2 2", "10+6|10 6"}; !slices.Equal(seen, want) {
		t.Fatalf("the reader saw %q, want %q", seen, want)
	}
}

func TestDisposedDerivedKeepsItsLastValue(t *testing.T) {
	t.Parallel()
	n := State(1)
	d := n.Map(func(v int) int { return v + 100 })
	if d.Get() != 101 {
		t.Fatalf("Get = %d, want 101", d.Get())
	}
	d.Dispose()
	d.Dispose() // a second Dispose is harmless
	n.Set(2)
	if d.Get() != 101 {
		t.Fatalf("a disposed Derived read %d, want its last value 101", d.Get())
	}
	if !d.eff.Disposed() {
		t.Fatal("the memo's computation does not report Disposed")
	}
}

func TestStateWriteInsideDerivedPanics(t *testing.T) {
	t.Parallel()
	other := State(0)
	d := Derived(func() int { other.Set(1); return 0 })
	defer d.Dispose()
	msg, _ := panicOf(func() { d.Get() }).(string)
	if !strings.Contains(msg, "inside Derived") {
		t.Fatalf("a write inside Derived panicked with %q, want a message naming Derived", msg)
	}
	// The panic unwound the depth count: writes outside Derived still work.
	other.Set(2)
	if Untrack(other.Get) != 2 {
		t.Fatal("a write after the panic did not land")
	}
}

func TestCyclicDerivedPanics(t *testing.T) {
	t.Parallel()
	var d *DerivedValue[int]
	d = Derived(func() int { return d.Get() + 1 })
	defer d.Dispose()
	msg, _ := panicOf(func() { d.Get() }).(string)
	if !strings.Contains(msg, "cyclic") {
		t.Fatalf("a Derived reading itself panicked with %q, want a cyclic Derived panic", msg)
	}
	if d.eff.running {
		t.Fatal("the memo is still marked running after its panic")
	}
}

// A memo read by a layout reports its version, and asking the version
// settles the memo first, so a layout sees a change it has not yet read.
func TestDerivedLayoutVersionSettlesTheMemo(t *testing.T) {
	t.Parallel()
	n := State(1)
	parity := Map(n, func(v int) int { return v % 2 })
	defer parity.Dispose()
	var src LayoutSource
	Measure(func(s LayoutSource, _ uint64) { src = s }, func() { parity.Get() })
	if src == nil {
		t.Fatal("reading a memo in layout recorded nothing")
	}
	v := src.LayoutVersion()
	n.Set(3)
	if src.LayoutVersion() != v {
		t.Fatal("an unchanged memo advanced its layout version")
	}
	n.Set(4)
	if src.LayoutVersion() == v {
		t.Fatal("a changed memo kept its layout version")
	}
}
