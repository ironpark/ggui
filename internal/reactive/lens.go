package reactive

// Lens is a two-way view of part of a StateValue's value. Build one with
// StateValue.Lens.
type Lens[U any] struct {
	get func() U
	set func(U)
}

// Lens returns a Binding onto the part of s's value that get selects: Get
// subscribes through s, and Set reads s, applies set to store the new part
// and writes the whole back. It is how a control binds to one field of a
// struct held in a single signal.
//
//	name := form.Lens(func(f Form) string { return f.Name }, func(f Form, v string) Form { f.Name = v; return f })
//	ui.TextField(name)
func (s *StateValue[T]) Lens[U any](get func(T) U, set func(T, U) T) *Lens[U] {
	return &Lens[U]{
		get: func() U { return get(s.Get()) },
		set: func(u U) { s.Set(set(Untrack(s.Get), u)) },
	}
}

// Field is Lens for a field that can be addressed: sel receives a copy of
// the whole and returns a pointer to the part, which is both how the part is
// read and where a write goes. It is the common case Lens covers with two
// closures.
//
//	name := form.Field(func(f *Form) *string { return &f.Name })
//	ui.TextField(name)
func (s *StateValue[T]) Field[U any](sel func(*T) *U) *Lens[U] {
	return &Lens[U]{
		get: func() U { v := s.Get(); return *sel(&v) },
		set: func(u U) {
			v := Untrack(s.Get)
			*sel(&v) = u
			s.Set(v)
		},
	}
}

// Get returns the part and subscribes the running Effect.
func (l *Lens[U]) Get() U { return l.get() }

// Set stores the part into the whole.
func (l *Lens[U]) Set(v U) { l.set(v) }

// Update applies fn to the current part and writes it through to the whole.
// Like StateValue.Update it belongs on the UI thread.
func (l *Lens[U]) Update(fn func(U) U) { l.Set(fn(Untrack(l.Get))) }

// GetAny returns the value as any and subscribes, for Sprintf.
func (l *Lens[U]) GetAny() any { return l.Get() }
