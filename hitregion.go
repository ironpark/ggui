package ggui

import "github.com/ironpark/ggui/gem"

// A hitRegion is a part of the window a widget registered with the Canvas
// for input in the frame painted last: its handlers, what it is called,
// and the space it was painted in. Input reads the regions of the last
// paint, top down, and keeps copies of the ones it follows across frames.
type hitRegion struct {
	rect    Rect
	full    Rect // rect before clipping, for scrolling it into view
	reach   Rect // set by HitReach: where the pointer lands on it besides rect
	id      any  // from an Identified handler, else nil
	pointer PointerHandler
	key     KeyHandler
	cursor  CursorShape
	shaped  bool // the region asks for cursor, the default arrow included
	// within marks a region registered with HitWithin, whose handler is
	// kept beside the list, in inputState.withins: see withinOf.
	within bool
	// parent is the index, plus one, in the same list of the region this
	// one is inside, 0 for none: see parentFor. It is always below the
	// region's own index plus one, so a scan from the end of the list
	// reaches a region before any it is inside. A copy kept past its frame
	// has it cleared, as the list it indexes is gone. It sits beside shaped,
	// in what would be padding.
	parent int32
	scope  *focusScope // the focus trap the region was painted in, if any
	role   Role        // from a Semantic handler
	label  string
	group  any       // set by Group: the EachKeyed entry that painted it
	drag   Draggable // a DragSource beneath its child; see drag.go

	space  *paintSpace // the space it was painted in, nil for the window's
	local  Rect        // rect in that space, clipped there; see contains
	near   Rect        // reach in that space
	origin Point       // full's corner in that space, before placeRegion moved it
}

// merge folds o into r when they share a Rect and o only adds what r lacks.
func (r *hitRegion) merge(o hitRegion) bool {
	if !r.samePlace(&o) || r.clashes(&o) {
		return false
	}
	if o.pointer != nil {
		r.pointer = o.pointer
	}
	if o.key != nil {
		r.key = o.key
	}
	if o.shaped {
		r.cursor, r.shaped = o.cursor, true
	}
	r.within = r.within || o.within
	if o.id != nil {
		r.id = o.id
	}
	if o.role != "" {
		r.role, r.label = o.role, o.label
	}
	return true
}

// samePlace reports whether o was registered for r's Rect, reach and
// space, and for r's identity when both have one.
func (r *hitRegion) samePlace(o *hitRegion) bool {
	return r.rect == o.rect && r.reach == o.reach && r.space == o.space &&
		(o.id == nil || r.id == nil || gem.Same(o.id, r.id))
}

// clashes reports whether r and o both hold what a region holds one of,
// or either holds a drag source.
func (r *hitRegion) clashes(o *hitRegion) bool {
	return o.drag != nil || r.drag != nil ||
		(o.pointer != nil && r.pointer != nil) ||
		(o.key != nil && r.key != nil) ||
		(o.shaped && r.shaped) ||
		(o.within && r.within)
}

// inScope reports whether r takes part while scope, the focus trap
// showing or nil, is: always without one, else when painted inside it.
func (r *hitRegion) inScope(scope *focusScope) bool {
	return scope == nil || r.scope != nil && gem.Same(r.scope.owner, scope.owner)
}

// contains reports whether the window point p lands on r: in its Rect, or
// in the reach a control gave it around that.
func (r *hitRegion) contains(p Point) bool {
	if !r.rect.Contains(p) && !r.reach.Contains(p) {
		return false
	}
	if r.space == nil {
		return true
	}
	q, ok := r.space.reach(p)
	return ok && (r.local.Contains(q) || r.near.Contains(q))
}

// offset returns the window point p from the corner of the Rect r was
// registered for, in the coordinates it was painted in: where on a drag
// source the pointer took hold, or where in a drop zone it let go.
func (r *hitRegion) offset(p Point) Point {
	if r.space == nil {
		return p.Sub(r.full.Origin)
	}
	return r.at(p).Sub(r.origin)
}

// pointerAt is ev as r's handler sees it.
func (r *hitRegion) pointerAt(ev PointerEvent) PointerEvent {
	if r.space == nil {
		return ev
	}
	if ev.Scroll != (Point{}) {
		// A delta is a direction, so it turns and scales but does not move:
		// a scroll view turned on its side scrolls along its own axis, and
		// a pan under a zoom follows the finger.
		ev.Scroll = r.at(ev.Scroll).Sub(r.at(Point{}))
	}
	ev.Pos = r.at(ev.Pos)
	return ev
}

// keep returns a copy of r that outlives the regions buffer. The copy
// knows no parent: the list its link indexes is rebuilt by the next paint.
func keep(r *hitRegion) *hitRegion {
	if r == nil {
		return nil
	}
	c := *r
	c.unlink()
	return &c
}

// unlink clears what r knows of the list it was registered in, for a copy
// that outlives it: following a kept copy's link then finds nothing rather
// than a region of another frame.
func (r *hitRegion) unlink() { r.parent = 0 }

// parentOf returns the region r is inside, or nil. r must point into
// in.regions: a kept copy has no parent, the list it indexed being gone.
func (in *inputState) parentOf(r *hitRegion) *hitRegion {
	if r == nil || r.parent == 0 || int(r.parent) > len(in.regions) {
		return nil
	}
	return &in.regions[r.parent-1]
}

// regionSlot keeps a region the input follows every frame, as keep does,
// in one of two buffers it alternates between: keeping one allocates
// nothing, and the copy kept before stays valid while the next is made,
// as a hover's exit needs the region it leaves. A pointer it returned is
// good until the second keep after it, so only the field it fills may hold
// one past the call.
type regionSlot struct {
	buf [2]hitRegion
	cur uint8
}

func (s *regionSlot) keep(r *hitRegion) *hitRegion {
	s.cur ^= 1
	if r == nil {
		// Let go of the handlers kept here, all but the copy just
		// replaced, which its caller may still read.
		s.buf[s.cur] = hitRegion{}
		return nil
	}
	s.buf[s.cur] = *r
	s.buf[s.cur].unlink()
	return &s.buf[s.cur]
}

// handle delivers the window-space ev to r's PointerHandler in the
// coordinates r was painted in and reports whether it consumed it.
func (r *hitRegion) handle(ev PointerEvent) bool {
	return r.pointer.HandlePointer(r.pointerAt(ev))
}

// handles reports whether r has a key handler, when key is set, or else a
// pointer handler.
func (r *hitRegion) handles(key bool) bool {
	if key {
		return r.key != nil
	}
	return r.pointer != nil
}

// topmost returns the last region painted that match accepts, or nil. Later
// regions sit on top of earlier ones, so the scan runs backwards.
func (in *inputState) topmost(match func(*hitRegion) bool) *hitRegion {
	for i := len(in.regions) - 1; i >= 0; i-- {
		if r := &in.regions[i]; match(r) {
			return r
		}
	}
	return nil
}

// regionIndex is what input knows of a list of regions without scanning
// it: the topmost region in a focus scope, whether any asks for a cursor,
// and where the handlers input follows were found. A paint fills it as it
// registers the regions; for regions set otherwise, as a test sets them,
// one scan fills it the first time it is asked. It holds for the buffer
// and length it was made for alone, so a later paint into the same buffer
// must hand input a new one with the regions.
type regionIndex struct {
	made    bool
	base    *hitRegion // regions[0] of the list it describes, nil if empty
	n       int
	scopeAt int  // the topmost region with a focus scope, or -1
	cursors bool // a region asks for a cursor
	within  bool // a region was registered with HitWithin
	// The last key and pointer handlers looked up, and where.
	key, pointer handlerHint
}

// handlerHint is where the topmost region with handler h is, or -1 when
// none has it; a zero hint knows of no handler.
type handlerHint struct {
	h  any
	at int
}

// describes reports whether x is the index of rs.
func (x *regionIndex) describes(rs []hitRegion) bool {
	if !x.made || x.n != len(rs) {
		return false
	}
	return len(rs) == 0 || x.base == &rs[0]
}

// setRegions hands input the regions a paint registered, the handlers of
// those it registered with HitWithin, and what the paint learned of them as
// it did.
func (in *inputState) setRegions(rs []hitRegion, withins []withinEntry, found regionIndex) {
	in.regions, in.withins, in.ix = rs, withins, found
	in.ix.made, in.ix.n, in.ix.base = true, len(rs), nil
	if len(rs) > 0 {
		in.ix.base = &rs[0]
	}
}

// index returns the index of the regions, scanning them once for one
// made where they were not painted.
func (in *inputState) index() *regionIndex {
	x := &in.ix
	if x.describes(in.regions) {
		return x
	}
	*x = regionIndex{made: true, n: len(in.regions), scopeAt: -1}
	if len(in.regions) > 0 {
		x.base = &in.regions[0]
	}
	for i := range in.regions {
		r := &in.regions[i]
		if r.scope != nil {
			x.scopeAt = i
		}
		if r.shaped {
			x.cursors = true
		}
		if r.within {
			x.within = true
		}
	}
	return x
}

// regionOf returns the topmost region whose key handler, or pointer
// handler when key is false, is h, or nil. The answer is remembered for
// the next lookup of h over the same regions: a focused or pressed widget
// is looked up every frame, and the regions change only when painted.
func (in *inputState) regionOf(h any, key bool) *hitRegion {
	if h == nil {
		return nil
	}
	x := in.index()
	hint := &x.pointer
	if key {
		hint = &x.key
	}
	if hint.h != nil && gem.Same(hint.h, h) {
		if hint.at < 0 {
			return nil
		}
		if r := &in.regions[hint.at]; gem.Same(handlerOf(r, key), h) {
			return r
		}
	}
	at := -1
	for i := len(in.regions) - 1; i >= 0; i-- {
		if r := handlerOf(&in.regions[i], key); r != nil && gem.Same(r, h) {
			at = i
			break
		}
	}
	*hint = handlerHint{h: h, at: at}
	if at < 0 {
		return nil
	}
	return &in.regions[at]
}

// handlerOf is r's key handler when key is set, else its pointer handler.
func handlerOf(r *hitRegion, key bool) any {
	if key {
		if r.key == nil {
			return nil
		}
		return r.key
	}
	if r.pointer == nil {
		return nil
	}
	return r.pointer
}

// findKey returns the topmost region with a KeyHandler under p.
func (in *inputState) findKey(p Point) *hitRegion {
	return in.topmost(func(r *hitRegion) bool { return r.key != nil && r.contains(p) })
}

// findPointer returns the region that continues p this frame by its
// pointer handler; see continuing.
func (in *inputState) findPointer(p *hitRegion) *hitRegion { return in.continuing(p, false) }

// findKeyRegion is findPointer for a key region.
func (in *inputState) findKeyRegion(p *hitRegion) *hitRegion { return in.continuing(p, true) }

// continuing returns the region that continues p this frame, matched by
// its key handler when key is set and by its pointer handler otherwise:
// the one with the same handler, wherever it moved to, or else the topmost
// region with such a handler and p's ID, or with none and painted exactly
// where p was, which is a rebuilt widget.
func (in *inputState) continuing(p *hitRegion, key bool) *hitRegion {
	if r := in.regionOf(handlerOf(p, key), key); r != nil {
		return r
	}
	if p.id != nil {
		return in.topmost(func(r *hitRegion) bool { return r.handles(key) && gem.Same(r.id, p.id) })
	}
	return in.topmost(func(r *hitRegion) bool { return r.handles(key) && r.id == nil && r.rect == p.rect })
}

// continues reports whether now, a region painted this frame, is the one
// prev was recorded from, matched as findPointer and findKeyRegion match: by
// handler, then id, and by Rect only for a rebuilt widget. Two regions
// sharing a Rect, as in a Stack, stay two.
func (in *inputState) continues(now, prev *hitRegion) bool {
	if now == nil || prev == nil {
		return now == prev
	}
	if (prev.pointer != nil && gem.Same(now.pointer, prev.pointer)) || (prev.key != nil && gem.Same(now.key, prev.key)) {
		return true
	}
	if prev.pointer != nil && in.findPointer(prev) == now {
		return true
	}
	return prev.key != nil && in.findKeyRegion(prev) == now
}
