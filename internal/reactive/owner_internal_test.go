package reactive

import "testing"

// These assert the core's own leak invariants -- that a signal drops stale
// subscribers and a panicking effect leaves nothing registered -- so they
// read the fields directly and live beside them.

func TestSubscriptionsFollowTheLastRun(t *testing.T) {
	flag, a := State(true), State(0)
	runs := 0
	dispose := Observe(func() {
		runs++
		if flag.Get() {
			a.Get()
		}
	})
	defer dispose()
	flag.Set(false)
	Flush()
	runs = 0
	a.Set(1)
	Flush()
	if runs != 0 {
		t.Fatalf("effect re-ran %d times on a signal it no longer reads", runs)
	}
	if len(a.subs) != 0 {
		t.Fatalf("signal keeps %d stale subscriptions, want 0", len(a.subs))
	}
}

func TestPanicInEffectLeavesNoResidue(t *testing.T) {
	effects.mu.Lock()
	before := effects.count
	effects.mu.Unlock()
	func() {
		defer func() { recover() }()
		Observe(func() {
			Observe(func() {})
			panic("boom")
		})
	}()
	if o := CurrentOwner(); o != nil {
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
	var owner *Computation
	var stops []func()
	var cleaned []int
	dispose := Root(func() {
		owner = CurrentOwner()
		for i := range 5 {
			stops = append(stops, Observe(func() {
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
