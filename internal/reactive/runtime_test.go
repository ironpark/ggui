package reactive

import "testing"

// A runtime's flush runs its own computations and Default's, never another
// runtime's: that is what keeps one Probe's frames from running the
// effects of another, or of one a test forgot to close.
func TestRuntimeFlushesOnlyItsOwnAndDefault(t *testing.T) {
	a, b := NewRuntime(), NewRuntime()
	n := State(0)
	var inA, inB, inDefault int
	disposeA := a.Root(func() { Observe(func() { n.Get(); inA++ }) })
	defer disposeA()
	disposeB := b.Root(func() { Observe(func() { n.Get(); inB++ }) })
	defer disposeB()
	disposeDefault := Observe(func() { n.Get(); inDefault++ })
	defer disposeDefault()
	inA, inB, inDefault = 0, 0, 0

	n.Set(1)
	if !a.Flush() {
		t.Fatal("a did not settle")
	}
	if inA != 1 || inB != 0 || inDefault != 1 {
		t.Fatalf("a's flush ran a=%d b=%d default=%d, want 1 0 1", inA, inB, inDefault)
	}
	if b.Settled() {
		t.Fatal("b took itself for settled with a stale computation")
	}
	b.Flush()
	if inB != 1 || inDefault != 1 {
		t.Fatalf("b's flush ran b=%d default=%d, want 1 1", inB, inDefault)
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
