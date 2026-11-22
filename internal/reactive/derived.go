package reactive

// Map returns a DerivedValue holding fn applied to s's value, recomputed whenever s
// changes. It belongs to the current owner: a Reactive or View callback
// disposes its previous computations on replacement, so retaining it leaves a frozen
// value. Without an owner, call Dispose explicitly; App.Close does not own it.
func (s *StateValue[T]) Map[U any](fn func(T) U) *DerivedValue[U] {
	return Derived(func() U { return fn(s.Get()) })
}

// DerivedValue is a derived value: it recomputes when one of the signals its function
// read changes, and notifies its own readers only when the result differs.
type DerivedValue[T any] struct {
	sig     *StateValue[T]
	eff     *Computation
	dispose func()
}

// Derived creates a lazy, read-only value. fn runs on the first Get and the
// first Get after its dependencies change. State writes inside fn panic.
// Computations created under an owner are disposed with it; otherwise call
// Dispose explicitly. WithEqual controls downstream change notification.
func Derived[T any](fn func() T) *DerivedValue[T] {
	m := &DerivedValue[T]{sig: State(*new(T))}
	e := newComputation(func() {
		// How deep the goroutine is inside Derived is what makes a state
		// write in there a panic rather than a silent cycle.
		sc := current()
		value := func() T {
			sc.derivedDepth++
			defer func() { sc.derivedDepth-- }()
			return fn()
		}()
		m.sig.store(sc, value)
	}, false)
	m.eff, m.dispose = e, e.dispose
	m.sig.memo, e.cell = e, m.sig
	e.mark(stateDirty)
	return m
}

// WithEqual controls whether a newly computed value notifies readers.
func (m *DerivedValue[T]) WithEqual(eq func(T, T) bool) *DerivedValue[T] {
	m.sig.WithEqual(eq)
	return m
}

// Map derives a value from any reactive source; StateValue.Map and DerivedValue.Map are
// the same for a source whose type is known.
func Map[T, U any](r Readable[T], fn func(T) U) *DerivedValue[U] {
	return Derived(func() U { return fn(r.Get()) })
}

// Combine derives a value from two reactive sources.
func Combine[A, B, C any](a Readable[A], b Readable[B], fn func(A, B) C) *DerivedValue[C] {
	return Derived(func() C { return fn(a.Get(), b.Get()) })
}

// Get returns the memoized value and subscribes the running Effect, if any.
// A memo whose inputs changed earlier in this frame is recomputed here, so a
// reader never sees one input updated and another not. Outside an effect
// this means fn may run at the call site rather than at the next flush.
func (m *DerivedValue[T]) Get() T {
	m.sig.checkThread("DerivedValue.Get")
	if m.eff.running {
		panic("ggui: cyclic Derived")
	}
	Refresh(m.eff)
	return m.sig.Get()
}

// GetAny returns the value as any and subscribes, for Sprintf.
func (m *DerivedValue[T]) GetAny() any { return m.Get() }

// Dispose stops recomputation. Readers keep seeing the last computed value.
func (m *DerivedValue[T]) Dispose() { m.dispose() }

// Map chains another derivation onto m.
func (m *DerivedValue[T]) Map[U any](fn func(T) U) *DerivedValue[U] {
	return Derived(func() U { return fn(m.Get()) })
}
