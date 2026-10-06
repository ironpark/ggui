package ggui

import (
	"reflect"

	"github.com/ironpark/ggui/internal/reactive"
)

// Remember returns the value made for key the last time the widgets around
// it were built, or makes one with init the first time. It is how state a
// View or Reactive builds outlives that View's rebuilds, as a field's draft
// should when the page around it is rebuilt:
//
//	ggui.View(fields, func(fs []Field) ggui.Widget {
//		rows := []ggui.Widget{}
//		for _, f := range fs {
//			text := ggui.Remember(f.ID, func() *ggui.StateValue[string] { return ggui.State(f.Value) })
//			rows = append(rows, ui.TextField(text))
//		}
//		return ggui.Column(rows...)
//	})
//
// A value is kept while the builds around it keep asking for it: one a
// View's build stops asking for is let go when that build ends, and so is
// one asked for by a View its parent rebuilt without, and everything made
// in a Component, an If or Key branch or an Each row goes with it. init
// runs untracked under an owner of its own, so a Derived or an Effect it
// makes lives exactly as long as the value.
//
// Keys belong to the nearest Component, branch or row, together with the
// type of the value, so different kinds of state can share a key; two
// Views asking for one key and type share the value. Outside every
// builder, Remember just returns init().
func Remember[K comparable, T any](key K, init func() T) T {
	f, _ := reactive.Memory().(*buildFrame)
	if f == nil || f.mem == nil {
		return Untrack(init)
	}
	k := memoKey{key, reflect.TypeFor[T]()}
	e := f.mem.entries[k]
	if e == nil {
		e = &memoEntry{}
		reactive.WithOwner(f.mem.owner, func() {
			e.dispose = reactive.Root(func() { e.value = init() })
		})
		if f.mem.entries == nil {
			f.mem.entries = map[memoKey]*memoEntry{}
		}
		f.mem.entries[k] = e
	}
	e.by = f.boundary
	if f.boundary != nil {
		e.seen = f.boundary.gen
	}
	return e.value.(T)
}

// memory is what one Component, branch, row or app keeps remembered values
// in; it goes when its owner does, and the values with it.
type memory struct {
	owner   *reactive.Computation
	entries map[memoKey]*memoEntry
}

type memoKey struct {
	key any
	typ reflect.Type
}

type memoEntry struct {
	value   any
	dispose func()
	by      *boundary // the View that asked for it last; nil for setup
	seen    uint64    // by's build that did
}

// boundary is one View or Reactive: a builder that runs again.
type boundary struct {
	mem      *memory
	gen      uint64
	disposed bool
}

// buildFrame is what a builder running now remembers in, and for.
type buildFrame struct {
	mem      *memory
	boundary *boundary
}

// remembering runs fn, a builder that runs once, with a memory of its own
// that lasts as long as the owner running it.
func remembering[T any](fn func() T) T {
	m := &memory{owner: reactive.CurrentOwner()}
	prev := reactive.SwapMemory(&buildFrame{mem: m})
	defer reactive.SwapMemory(prev)
	return reactive.Build(fn)
}

// newBoundary is the boundary of a View or Reactive being made now, in the
// memory of the builder making it.
func newBoundary() *boundary {
	f, _ := reactive.Memory().(*buildFrame)
	b := &boundary{}
	if f != nil {
		b.mem = f.mem
	}
	if reactive.CurrentOwner() != nil {
		OnCleanup(func() { b.disposed = true })
	}
	return b
}

// build runs one build of the boundary, then lets go of what this build no
// longer asked for: what the last one did, and what a View this build
// replaced did.
func (b *boundary) build(fn func() Widget) Widget {
	b.gen++
	prev := reactive.SwapMemory(&buildFrame{mem: b.mem, boundary: b})
	defer reactive.SwapMemory(prev)
	w := fn()
	if b.mem != nil {
		for k, e := range b.mem.entries {
			if (e.by == b && e.seen != b.gen) || (e.by != nil && e.by.disposed) {
				e.dispose()
				delete(b.mem.entries, k)
			}
		}
	}
	return w
}
