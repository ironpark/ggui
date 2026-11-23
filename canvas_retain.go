package ggui

import (
	"maps"
	"time"
)

// retained is what Retain stored this frame, and last frame's, which
// Retained reads.
//
// A frame painted in part paints some widgets and leaves the rest as they
// were, so it keeps what the frame before stored and stores over it: keeps
// is then both. read and stored are the slots the widgets it painted read
// and stored, and a slot read but not stored again is dropped as the frame
// ends, as a whole frame drops it, while one no widget it painted touched
// stays.
type retained struct {
	keeps        map[retainKey]any
	prevKeeps    map[retainKey]any
	staged       map[retainKey]any // partial-frame writes, committed only on partial completion
	read, stored map[retainKey]struct{}
}

// next makes this frame's values last frame's.
func (r *retained) next() {
	r.prevKeeps, r.keeps = r.keeps, r.prevKeeps
	clear(r.keeps)
}

// endPartial commits staged writes from a completed partial frame and drops
// slots read in it and not stored again.
func (r *retained) endPartial() {
	for k := range r.read {
		if _, ok := r.stored[k]; !ok {
			delete(r.keeps, k)
		}
	}
	if len(r.staged) > 0 {
		if r.keeps == nil {
			r.keeps = make(map[retainKey]any, len(r.staged))
		}
		maps.Copy(r.keeps, r.staged)
	}
	r.abortPartial()
}

// abortPartial discards staged partial-frame writes without modifying keeps.
func (r *retained) abortPartial() {
	clear(r.staged)
	clear(r.read)
	clear(r.stored)
}

// adoptIndex is where last frame's regions are, for adopt. It is built on
// a frame's first handoff and not at all in a frame with none, which is
// most of them: only a handful of widgets adopt.
type adoptIndex struct {
	prevByID    map[any]int  // id to its place in prev, the last one painted
	prevByRect  map[Rect]int // the same for a region with no id
	prevHandler map[any]bool // the handlers of prev that are pointers
	prevIndexed bool
}

// reset forgets the index, which a new prev makes stale. The maps keep
// their storage for the next frame that needs them.
func (x *adoptIndex) reset() {
	x.prevIndexed = false
	clear(x.prevByID)
	clear(x.prevByRect)
	clear(x.prevHandler)
}

// Slot names one value a widget retains on the Canvas between frames,
// typed like an EnvKey. Make one per animated value or timer with
// NewSlot, as a package variable.
type Slot[T any] struct {
	id   *byte
	name string
}

// NewSlot creates a distinct Slot; name is for messages only.
func NewSlot[T any](name string) Slot[T] { return Slot[T]{id: new(byte), name: name} }

// Anchor is what retained state is attached to: the widget's ID when it
// has one, so the state follows it wherever it moves, or else the Rect it
// painted.
type Anchor struct {
	Rect Rect
	ID   any
}

// retainKey identifies a Retain slot: the anchor and the slot.
type retainKey struct {
	rect Rect
	id   any
	slot *byte
}

func (a Anchor) key(slot *byte) retainKey {
	if a.ID != nil {
		return retainKey{id: a.ID, slot: slot}
	}
	return retainKey{rect: a.Rect, slot: slot}
}

// Retain stores v for the next frame under at and s, and Retained returns
// what was stored under the same pair last frame. It is how a widget
// rebuilt every frame keeps state that has no signal: a ui.Tooltip's hover
// timer, a Transition's start time. A slot nobody retains again is
// dropped, so state goes away with the widget.
func (c *Canvas) Retain[T any](at Anchor, s Slot[T], v T) {
	if c == nil {
		return
	}
	f := c.fs()
	k := at.key(s.id)
	if f.partial {
		if f.staged == nil {
			f.staged = make(map[retainKey]any)
		}
		if f.stored == nil {
			f.stored = make(map[retainKey]struct{})
		}
		f.staged[k] = v
		f.stored[k] = struct{}{}
		return
	}
	if f.keeps == nil {
		f.keeps = make(map[retainKey]any)
	}
	f.keeps[k] = v
}

// Retained returns what Retain stored under at and s last frame.
func (c *Canvas) Retained[T any](at Anchor, s Slot[T]) (T, bool) {
	if c == nil {
		var zero T
		return zero, false
	}
	f := c.fs()
	k := at.key(s.id)
	keeps := f.prevKeeps
	if f.partial {
		// The last frame's values are what keeps holds; see retained. Read
		// from those the whole frame before that left, a Transition that
		// appeared in the last whole frame would start again at every frame
		// painted in part, and never end.
		keeps = f.keeps
		if f.read == nil {
			f.read = make(map[retainKey]struct{})
		}
		f.read[k] = struct{}{}
	}
	v, ok := keeps[k].(T)
	return v, ok
}

// Ease moves a Motion retained under at and s toward target over d and
// returns where it is now, so a control rebuilt every frame keeps
// animating; a switch knob and a tab underline use it. It moves along
// EaseOut; EaseWith takes another curve.
func (c *Canvas) Ease(at Anchor, s Slot[*Motion], target float64, d time.Duration) float64 {
	return c.EaseWith(at, s, target, d, nil)
}

// EaseWith is Ease along e, as Motion.Easing takes it: a design system
// whose transitions run on a curve of its own, CSS's
// cubic-bezier(0.4, 0, 0.2, 1) say, passes it here. Pass the same Easing
// every frame rather than one made anew, which would be an allocation a
// frame; a nil e is EaseOut.
func (c *Canvas) EaseWith(at Anchor, s Slot[*Motion], target float64, d time.Duration, e Easing) float64 {
	now := FrameTime()
	if c == nil {
		// Nothing keeps the motion: a new one would be at target at once.
		return target
	}
	m, ok := c.Retained(at, s)
	if !ok {
		m = new(Motion)
	}
	m.Easing(e).MoveTo(target, now, d)
	c.Retain(at, s, m)
	return m.Value(now)
}
