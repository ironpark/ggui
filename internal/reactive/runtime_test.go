package reactive

import (
	"fmt"
	"testing"
)

// A runtime's flush runs its own computations and its Base's, never another
// runtime's: that is what keeps one Probe's frames from running the
// effects of another, or of one a test forgot to close.
func TestRuntimeFlushesOnlyItsOwnAndItsBase(t *testing.T) {
	a, b := NewRuntime(), NewRuntime()
	n := State(0)
	var inA, inB, inBase int
	disposeA := a.Root(func() { Observe(func() { n.Get(); inA++ }) })
	defer disposeA()
	disposeB := b.Root(func() { Observe(func() { n.Get(); inB++ }) })
	defer disposeB()
	disposeBase := Observe(func() { n.Get(); inBase++ })
	defer disposeBase()
	inA, inB, inBase = 0, 0, 0

	n.Set(1)
	if !a.Flush() {
		t.Fatal("a did not settle")
	}
	if inA != 1 || inB != 0 || inBase != 1 {
		t.Fatalf("a's flush ran a=%d b=%d base=%d, want 1 0 1", inA, inB, inBase)
	}
	if b.Settled() {
		t.Fatal("b took itself for settled with a stale computation")
	}
	b.Flush()
	if inB != 1 || inBase != 1 {
		t.Fatalf("b's flush ran b=%d base=%d, want 1 1", inB, inBase)
	}
}

// A computation belongs to its owner's runtime, however deep, and one made
// with no owner to Default.
func TestComputationsInheritTheirOwnersRuntime(t *testing.T) {
	rt := NewRuntime()
	var inner *Computation
	dispose := rt.Root(func() {
		Observe(func() {
			Root(func() { inner = CurrentOwner() })
		})
	})
	if inner.rt != rt {
		t.Fatal("a nested root left its owner's runtime")
	}
	if rt.Count() == 0 {
		t.Fatal("the runtime registered nothing")
	}
	dispose()
	if rt.Count() != 0 {
		t.Fatalf("disposing the root left %d computations in the runtime", rt.Count())
	}
	free := Observe(func() {})
	defer free()
	if CurrentOwner() != nil {
		t.Fatal("an owner leaked out of Observe")
	}
}

// A memo in one runtime read by an effect in another reaches the reader
// through markCheck alone, which must be enough to make the reader's
// runtime flush.
func TestDerivedReadAcrossRuntimes(t *testing.T) {
	a, b := NewRuntime(), NewRuntime()
	n := State(1)
	var double *DerivedValue[int]
	disposeA := a.Root(func() { double = Derived(func() int { return n.Get() * 2 }) })
	defer disposeA()
	got := 0
	disposeB := b.Root(func() { Observe(func() { got = double.Get() }) })
	defer disposeB()
	if got != 2 {
		t.Fatalf("first run read %d, want 2", got)
	}
	n.Set(5)
	if b.Settled() {
		t.Fatal("a reader of a stale memo in another runtime took itself for settled")
	}
	b.Flush()
	if got != 10 {
		t.Fatalf("after the write the reader saw %d, want 10", got)
	}
}

// User effects are post-layout work: FlushUsers runs a runtime's own and
// Default's, and leaves another runtime's pending for its own pass.
func TestFlushUsersStaysInItsRuntime(t *testing.T) {
	a, b := NewRuntime(), NewRuntime()
	n := State(0)
	var inA, inB int
	disposeA := a.Root(func() { Effect(func() Cleanup { n.Get(); inA++; return nil }) })
	defer disposeA()
	disposeB := b.Root(func() { Effect(func() Cleanup { n.Get(); inB++; return nil }) })
	defer disposeB()
	a.FlushUsers(nil)
	if inA != 1 || inB != 0 {
		t.Fatalf("a's pass ran a=%d b=%d, want 1 0", inA, inB)
	}
	b.FlushUsers(nil)
	if inB != 1 {
		t.Fatalf("b's pass ran b=%d, want 1", inB)
	}
	n.Set(1)
	a.FlushUsers(nil)
	if inA != 2 || inB != 1 {
		t.Fatalf("after a write a's pass ran a=%d b=%d, want 2 1", inA, inB)
	}
}

// Runtimes on goroutines of their own track and flush independently: what
// one goroutine runs never becomes the listener or owner of another's reads.
// Run it with -race.
func TestRuntimesRunOnGoroutinesOfTheirOwn(t *testing.T) {
	const goroutines, writes = 8, 200
	done := make(chan error, goroutines)
	for g := range goroutines {
		go func() {
			rt := NewRuntime()
			n := State(0)
			var seen, runs int
			dispose := rt.Root(func() {
				double := Derived(func() int { return n.Get() * 2 })
				Observe(func() { seen = double.Get(); runs++ })
			})
			defer dispose()
			for i := 1; i <= writes; i++ {
				n.Set(i)
				if !rt.Flush() {
					done <- fmt.Errorf("goroutine %d: flush did not settle", g)
					return
				}
				if seen != 2*i {
					done <- fmt.Errorf("goroutine %d: saw %d after writing %d", g, seen, i)
					return
				}
			}
			if runs != writes+1 {
				done <- fmt.Errorf("goroutine %d: effect ran %d times, want %d", g, runs, writes+1)
				return
			}
			if CurrentOwner() != nil {
				done <- fmt.Errorf("goroutine %d: an owner leaked out", g)
				return
			}
			done <- nil
		}()
	}
	for range goroutines {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
}
