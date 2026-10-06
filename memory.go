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
	b, _ := reactive.Memory().(*boundary)
	if b == nil {
		return Untrack(init)
	}
	m := b.mem
	k := memoKey{key, reflect.TypeFor[T]()}
	e := m.entries[k]
	if e == nil {
		e = &memoEntry{}
		reactive.WithOwner(m.owner, func() {
			e.dispose = reactive.Root(func() { e.value = init() })
		})
		if m.entries == nil {
			m.entries = map[memoKey]*memoEntry{}
		}
		m.entries[k] = e
	}
	if e.by != b || e.seen != b.gen {
		e.by, e.seen = b, b.gen
		b.asked = append(b.asked, k)
	}
	return e.value.(T)
}

// memory is what one Component, branch, row or app keeps remembered values
// in; it goes when its owner does, and the values with it.
type memory struct {
	owner   *reactive.Computation
	entries map[memoKey]*memoEntry
	orphans []memoKey // asked for by Views since disposed; see boundary.build
}

type memoKey struct {
	key any
	typ reflect.Type
}

type memoEntry struct {
	value   any
	dispose func()
	by      *boundary // the builder that asked for it last
	seen    uint64    // by's build that did
}

// boundary is a builder Remember is asked in: a View or Reactive, which
// runs again, or the setup of a Component, branch, row or app, which runs
// once and owns the memory.
type boundary struct {
	mem      *memory
	own      memory    // the memory, when this builder owns it
	gen      uint64    // its builds so far
	asked    []memoKey // what its last build asked for
	disposed bool
}

// remembering runs fn, a builder that runs once, with a memory of its own
// that lasts as long as the owner running it.
func remembering[T any](fn func() T) T {
	b := &boundary{}
	b.own.owner = reactive.CurrentOwner()
	b.mem = &b.own
	prev := reactive.SwapMemory(b)
	defer reactive.SwapMemory(prev)
	return reactive.Build(fn)
}

// newBoundary is the boundary of a View or Reactive being made now, in the
// memory of the builder making it, or nil outside every builder.
func newBoundary() *boundary {
	parent, _ := reactive.Memory().(*boundary)
	if parent == nil {
		return nil
	}
	b := &boundary{mem: parent.mem}
	if reactive.CurrentOwner() != nil {
		OnCleanup(func() {
			b.disposed = true
			b.mem.orphans = append(b.mem.orphans, b.asked...)
		})
	}
	return b
}

// build runs one build of the boundary, then lets go of what it asked for
// last time and not this time, and of what a View this build replaced
// asked for and no new one did.
func (b *boundary) build(fn func() Widget) Widget {
	if b == nil {
		return fn()
	}
	b.gen++
	last := b.asked
	b.asked = nil
	prev := reactive.SwapMemory(b)
	defer reactive.SwapMemory(prev)
	w := fn()
	b.mem.drop(last, func(e *memoEntry) bool { return e.by == b && e.seen != b.gen })
	orphans := b.mem.orphans
	b.mem.orphans = nil
	b.mem.drop(orphans, func(e *memoEntry) bool { return e.by.disposed })
	return w
}

// drop disposes the entries of keys that gone reports gone.
func (m *memory) drop(keys []memoKey, gone func(*memoEntry) bool) {
	for _, k := range keys {
		if e := m.entries[k]; e != nil && gone(e) {
			e.dispose()
			delete(m.entries, k)
		}
	}
}
