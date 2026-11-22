package reactive

import (
	"slices"
	"strconv"
)

// An Computation is clean, or it may be stale. A StateValue write marks its direct
// subscribers dirty and everything downstream of a memo check: check means
// "an input of yours may have changed", and is resolved by refreshing the
// upstream memos, which turns it into dirty or back into clean. It is what
// keeps a reader from seeing one input updated and another not.
const (
	stateClean uint8 = iota
	stateCheck
	stateDirty
)

type Computation struct {
	fn         func()
	state      uint8
	user       bool
	persistent bool
	// rt is the Runtime whose flushes run this Computation: its owner's, or
	// the creating goroutine's Base for one created with no owner.
	rt *Runtime
	// loop is the frame loop this Computation belongs to, held only so that
	// flushUsers can tell one loop's effects from another's. It is never
	// dereferenced here, so the reactive core needs no frame loop type.
	loop     any
	running  bool
	disposed bool

	// cell is the memo's value this Computation computes, if it is a memo's:
	// what carries staleness on to the memo's own readers.
	cell downstream

	// region, when not nil, is the flag of the region the computation was
	// made in or its owner was; see InRegion.
	region *bool

	owner                    *Computation
	firstChild, lastChild    *Computation
	prevSibling, nextSibling *Computation
	cleanups                 []func()

	// registered is whether the runtime counts the computation; sources
	// are the signals it read when it last ran.
	registered bool
	sources    []source

	// order is the computation's place in its runtime's creation order,
	// which is the order a flush runs them in: an owner before what it
	// made. queued is set while it waits in that runtime's queue.
	order  uint64
	queued bool

	// Where this Computation was created, in the ggui_debug build only: what
	// lets ErrCycle name the effects a cycle is made of. Empty otherwise.
	origin string

	// Identity for what is constructed under this Computation: keyRoot is the
	// identity of the mounted component or block this Computation is the root of, path
	// is the Computation's place under the nearest such root, by construction
	// order, and seq counts what the current run constructed.
	keyRoot any
	path    string
	seq     int
}

// autoKey is the identity a widget gets from the keyed component it was
// constructed in: the component's mount instance and the widget's place, by
// construction order, under it. It is the same across the component's
// rebuilds, so the widget's hit region, retained state and adoption
// follow it without a Key of its own.
type autoKey struct {
	root any
	path string
}

// AutoID returns the identity for a widget being constructed now: nil
// outside a keyed component.
func AutoID() any {
	o := CurrentOwner()
	if o == nil {
		return nil
	}
	var root any
	for r := o; r != nil; r = r.owner {
		if r.keyRoot != nil {
			root = r.keyRoot
			break
		}
	}
	if root == nil {
		return nil
	}
	k := autoKey{root: root, path: o.path + "/" + strconv.Itoa(o.seq)}
	o.seq++
	return k
}

// place gives e its path under owner: owner's path plus e's construction
// ordinal, or elem when the caller names it, as EachKeyed does with the item key.
func (e *Computation) place(owner *Computation, elem string) {
	if owner == nil {
		return
	}
	if elem == "" {
		elem = strconv.Itoa(owner.seq)
		owner.seq++
	}
	e.path = owner.path + "/" + elem
}

// attach appends a child to its owner's ordered list.
func (e *Computation) attach(owner *Computation) {
	e.owner = owner
	e.prevSibling = owner.lastChild
	if owner.lastChild != nil {
		owner.lastChild.nextSibling = e
	} else {
		owner.firstChild = e
	}
	owner.lastChild = e
}

// detach releases the parent's reference before running cleanup. reset can
// then consume its first child repeatedly, even if cleanup disposes siblings.
func (e *Computation) detach() {
	if e.owner == nil {
		return
	}
	if e.prevSibling != nil {
		e.prevSibling.nextSibling = e.nextSibling
	} else {
		e.owner.firstChild = e.nextSibling
	}
	if e.nextSibling != nil {
		e.nextSibling.prevSibling = e.prevSibling
	} else {
		e.owner.lastChild = e.prevSibling
	}
	e.owner, e.prevSibling, e.nextSibling = nil, nil, nil
}

// reset undoes everything the last run set up: child effects, cleanups and
// subscriptions. It runs before each re-run and on dispose.
func (e *Computation) reset() {
	var cleanups []func()
	defer func() {
		for _, s := range e.sources {
			s.unsubscribe(e)
		}
		clear(cleanups)
		clear(e.sources)
		if e.disposed {
			e.sources = nil
			clear(e.cleanups)
			e.cleanups = nil
			return
		}
		if e.cleanups == nil && cleanups != nil {
			e.cleanups = cleanups[:0]
		}
		e.sources = e.sources[:0]
	}()

	var children []*Computation
	for child := e.firstChild; child != nil; child = child.nextSibling {
		if e.disposed || !child.persistent {
			children = append(children, child)
		}
	}
	for _, child := range children {
		child.dispose()
	}

	// The cleanups and sources slices are kept for the next run, emptied,
	// so that a computation running again allocates nothing to record what
	// it reads: every re-run used to grow both afresh. The cleanups are
	// taken first, so that one which somehow registers another sees an
	// empty list rather than the one being run.
	cleanups = e.cleanups
	e.cleanups = nil
	if len(cleanups) > 0 {
		WithOwner(nil, func() {
			for _, v := range slices.Backward(cleanups) {
				v()
			}
		})
	}
}

func (e *Computation) dispose() {
	if e.disposed {
		return
	}
	e.disposed = true
	e.detach()
	defer e.rt.effects.remove(e)
	e.reset()
}

// Watch runs fn with src's value after layout, and after changes. It returns
// a dispose function, like Effect.
func Watch[T any](src Readable[T], fn func(T)) (dispose func()) {
	return Effect(func() Cleanup { fn(src.Get()); return nil })
}

// Observe runs fn now and again when what it read changes, without making
// it a user effect. It is Effect without the post-layout pass.
func Observe(fn func()) (dispose func()) {
	_, dispose = EffectWith(fn)
	return dispose
}

// Cleanup releases an effect or a mounted resource. It may be nil.
type Cleanup = func()

// Effect schedules a side effect after layout. It belongs to the current
// owner; its cleanup runs untracked before another execution and on disposal.
func Effect(fn func() Cleanup) Cleanup {
	CheckUIThread("Effect")
	e := newComputation(func() {
		if cleanup := fn(); cleanup != nil {
			OnCleanup(cleanup)
		}
	}, true)
	if e.disposed {
		return e.dispose
	}
	e.rt.effects.userPending = true
	e.rt.effects.dirtyGen++
	e.mark(stateDirty)
	return e.dispose
}

// newComputation makes a computation under the current owner, or with none
// in the goroutine's Base. A user effect requires an owner.
func newComputation(fn func(), user bool) *Computation {
	sc := current()
	if user && sc.owner == nil {
		panic("ggui: Effect requires an owner; use Component or Root")
	}
	e := &Computation{fn: fn, user: user, origin: effectOrigin(), region: sc.region}
	if owner := sc.owner; owner != nil {
		e.loop, e.rt = owner.loop, owner.rt
		if e.region == nil {
			e.region = owner.region
		}
		if owner.disposed {
			e.disposed = true
			return e
		}
		e.attach(owner)
		e.place(owner, "")
	} else {
		e.rt = sc.baseRuntime()
	}
	e.rt.effects.add(e)
	return e
}

// EffectWith runs an internal binding immediately. User effects use a
// separate post-layout phase and never drive widget construction.
func EffectWith(fn func()) (*Computation, func()) {
	e := newComputation(fn, false)
	if e.disposed {
		return e, e.dispose
	}
	ok := false
	defer func() {
		if !ok {
			e.dispose()
		}
	}()
	runEffect(e)
	ok = true
	return e, e.dispose
}

// Root runs fn untracked under a fresh owner that never re-runs, so effects
// and components fn creates live until the returned dispose is called or the
// enclosing owner is disposed. It is how a container keeps children alive
// across its own re-runs; EachKeyed uses it per key.
func Root(fn func()) (dispose func()) {
	return rootIn(nil, nil, "", true, fn)
}

// RootWith is Root for a root that is the instance of a keyed component
// (identity) or has a name of its own under its owner (elem), for the
// identities AutoID derives.
func RootWith(identity any, elem string, fn func()) (dispose func()) {
	return rootIn(nil, identity, elem, false, fn)
}

// Root is the package Root for a root whose computations rt flushes,
// whatever the current owner's runtime: how a frame loop gives the tree it
// builds a graph of its own.
func (rt *Runtime) Root(fn func()) (dispose func()) {
	return rootIn(rt, nil, "", true, fn)
}

// rootIn is RootWith with the root's runtime chosen: rt, or when nil the
// owner's, or the goroutine's Base. A persistent root is Root's.
func rootIn(rt *Runtime, identity any, elem string, persistent bool, fn func()) (dispose func()) {
	r := &Computation{keyRoot: identity, origin: effectOrigin(), persistent: persistent}
	sc, prevListener, prevOwner := enter(nil, r)
	r.region = sc.region
	if prevOwner != nil {
		r.loop, r.rt = prevOwner.loop, prevOwner.rt
		if r.region == nil {
			r.region = prevOwner.region
		}
		if prevOwner.disposed {
			r.disposed = true
		} else {
			r.attach(prevOwner)
			r.place(prevOwner, elem)
		}
	}
	switch {
	case rt != nil:
		r.rt = rt
	case r.rt == nil:
		r.rt = sc.baseRuntime()
	}
	ok := false
	defer func() {
		sc.listener, sc.owner = prevListener, prevOwner
		if !ok {
			r.dispose()
		}
	}()
	fn()
	ok = true
	return r.dispose
}

// WithOwner runs fn with owner as the current owner and no listener.
func WithOwner(owner *Computation, fn func()) {
	sc, prevListener, prevOwner := enter(nil, owner)
	defer func() { sc.listener, sc.owner = prevListener, prevOwner }()
	fn()
}

// CurrentOwner returns the running goroutine's owner, or nil.
func CurrentOwner() *Computation { return current().owner }

// OnCleanup registers fn to run before the enclosing Effect re-runs and when
// it is disposed. Call it from inside an Effect, a Builder or a Component
// setup, for timers, subscriptions and anything else that must be undone.
func OnCleanup(fn func()) {
	o := CurrentOwner()
	if o == nil {
		panic("ggui: OnCleanup requires an owner")
	}
	if o.disposed {
		WithOwner(nil, fn)
		return
	}
	o.cleanups = append(o.cleanups, fn)
}

// The frame loop drives the reactive core from the root package. These are
// the entry points it needs; everything else here stays private.

// Disposed reports whether this computation's owner has been torn down.
func (e *Computation) Disposed() bool { return e.disposed }

// Origin is where this computation was created, in a ggui_debug build, for
// the ErrCycle message the frame loop builds.
func (e *Computation) Origin() string { return e.origin }

// Derived reports whether this computation computes a memo's value.
func (e *Computation) Derived() bool { return e.cell != nil }

// Loop and SetLoop carry the frame loop an effect belongs to. It is opaque
// here: the reactive core only ever compares one against another.
func (e *Computation) Loop() any     { return e.loop }
func (e *Computation) SetLoop(v any) { e.loop = v }

// Children lists this computation's live children, oldest first. The
// ownership tree is private; this is for a caller that only needs to see
// what is still attached.
func (e *Computation) Children() []*Computation {
	var out []*Computation
	for c := e.firstChild; c != nil; c = c.nextSibling {
		out = append(out, c)
	}
	return out
}
