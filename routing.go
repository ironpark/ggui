package ggui

import (
	"cmp"
	"slices"
	"unsafe"

	"github.com/ironpark/ggui/gem"
)

// Routing through the region tree. Every region knows the region it was
// painted inside (hitRegion.parent, set by parentFor as the paint
// registers it), and what input follows from frame to frame along a chain
// of them it keeps as a path: copies of the regions, matched to the next
// frame's entry against entry.

// regionPath keeps the regions of a chain input follows across frames,
// the innermost first, as copies that outlive the list they were painted
// in. Its buffer is reused, so keeping a path allocates nothing once it has
// grown to the deepest chain kept.
type regionPath struct {
	at []hitRegion
}

// set keeps copies of the regions of rs at the indices idx, in that order,
// and lets go of the handlers of any it kept before past them.
func (p *regionPath) set(rs []hitRegion, idx []int32) {
	n := len(p.at)
	p.at = p.at[:0]
	for _, i := range idx {
		p.at = append(p.at, rs[i])
		p.at[len(p.at)-1].unlink()
	}
	if n > len(p.at) {
		clear(p.at[len(p.at):n])
	}
}

// sameRegion reports whether now, a region of this frame's list, is the
// region prev was copied from, by the pointer handler when key is unset and
// by the key handler otherwise: the same handler; or else, when prev's
// handler is painted nowhere this frame, the same ID, or for a handler with
// none the same Rect, which is a widget rebuilt in its place. It is
// continues for a pair: two regions sharing a Rect, as in a Stack, stay two
// while both are painted.
func (in *inputState) sameRegion(now, prev *hitRegion, key bool) bool {
	h := handlerOf(prev, key)
	if h == nil {
		return false
	}
	if gem.Same(handlerOf(now, key), h) {
		return true
	}
	if !now.handles(key) || in.regionOf(h, key) != nil {
		return false
	}
	if prev.id != nil {
		return gem.Same(now.id, prev.id)
	}
	return now.id == nil && now.rect == prev.rect
}

// sendAt is send by index: it offers ev to the regions under it from the
// top down until one consumes it, and returns that one's index, or -1.
//
// A press or a wheel, in a frame that registered a HitWithin region, is
// first offered to the within handlers on the chain of each region it is
// about to reach, the outermost first, each once: a handler that takes it
// takes it from everything inside. The index returned is then the taking
// region's, or -1 when it has no pointer handler to hold the press.
func (in *inputState) sendAt(ev PointerEvent) int32 {
	watch := (ev.Kind == PointerDown || ev.Kind == PointerScroll) && in.index().within
	if watch {
		in.offered = in.offered[:0]
	}
	for i := int32(len(in.regions)) - 1; i >= 0; i-- {
		r := &in.regions[i]
		if r.pointer == nil && !(watch && r.within) || !r.contains(ev.Pos) {
			continue
		}
		if watch {
			if w, took := in.offerWithin(i, ev); took {
				if in.regions[w].pointer == nil {
					return -1
				}
				return w
			}
		}
		if r.pointer != nil && in.deliver(r, ev) {
			return i
		}
	}
	return -1
}

// withinEntry is the handler of a region registered with HitWithin, at
// its index in the regions.
type withinEntry struct {
	at int32
	h  any
}

// withinOf returns the within handler of the region at i, or nil.
func (in *inputState) withinOf(i int32) any {
	if !in.regions[i].within {
		return nil
	}
	ws := in.withins
	if j, ok := slices.BinarySearchFunc(ws, i, func(e withinEntry, i int32) int { return cmp.Compare(e.at, i) }); ok {
		return ws[j].h
	}
	return nil
}

// offerWithin offers ev to the within handlers of the region at i and of
// the regions it is inside that have not been offered it, the outermost
// first, and reports whether one took it, and which region's. A region
// offered the event has had its whole chain offered it, so the walk up
// stops at the first already offered.
func (in *inputState) offerWithin(i int32, ev PointerEvent) (int32, bool) {
	if slices.Contains(in.offered, i) {
		return -1, false
	}
	in.offered = append(in.offered, i)
	r := &in.regions[i]
	if r.parent != 0 {
		if w, took := in.offerWithin(r.parent-1, ev); took {
			return w, true
		}
	}
	if !r.within {
		return -1, false
	}
	if h, ok := in.withinOf(i).(PointerWithinHandler); ok && h.HandlePointerWithin(r.pointerAt(ev)) {
		in.touch(r)
		return i, true
	}
	return -1, false
}

// updateHover finds the hover path and tells the regions that joined it
// and left it. The topmost region that claims PointerMove is the hot one;
// each region it is inside is offered the move in turn, with Inner set,
// and those that claim it too are hovered with it. Exits go before enters,
// so the two never overlap.
func (in *inputState) updateHover(f frameInput) {
	move := PointerEvent{Kind: PointerMove, Pos: f.pos, Mods: f.mods}
	in.moved = !in.movedOnce || f.pos != in.lastMove
	in.lastMove, in.movedOnce = f.pos, true
	path := in.hoverAt[:0]
	if hot := in.sendAt(move); hot >= 0 {
		path = append(path, hot)
		move.Inner = true
		for a := in.regions[hot].parent; a != 0; a = in.regions[a-1].parent {
			if r := &in.regions[a-1]; r.pointer != nil && in.deliver(r, move) {
				path = append(path, a-1)
			}
		}
	}
	in.hoverAt = path
	in.hoverChanged(path, f.pos)
	in.hover.set(in.regions, path)
}

// hoverChanged compares the hover path kept last frame with path, this
// frame's, as indices into the regions, the hot one first. A region of the
// last with no match in path hears PointerExit, the innermost first; then
// one of path with no match in the last, or matched but with a handler
// rebuilt since, hears PointerEnter, the outermost first, with Inner set on
// all but the hot one.
func (in *inputState) hoverChanged(path []int32, pos Point) {
	last := in.hover.at
	for j := range last {
		if in.onPath(&last[j], path) {
			continue
		}
		// The exit goes to the handler holding the region now: one rebuilt
		// since it was entered has taken over its hover.
		left := &last[j]
		if cur := in.findPointer(left); cur != nil {
			left = cur
		}
		in.deliver(left, PointerEvent{Kind: PointerExit, Pos: pos})
	}
	for k, p := range slices.Backward(path) {
		now := &in.regions[p]
		if prev := in.matchOf(now, last); prev != nil && gem.Same(now.pointer, prev.pointer) {
			continue
		}
		// New on the path, or rebuilt under the pointer: a new handler has
		// a hover state of its own that never heard the enter, as a row of
		// a list redrawn for a change elsewhere would. The one it replaced
		// is gone, and hears no exit.
		in.deliver(now, PointerEvent{Kind: PointerEnter, Pos: pos, Inner: k > 0})
	}
}

// onPath reports whether prev, a region of the last path, is one of the
// regions at the indices path: by its handler before anything else, so
// that a wrapper and a child rebuilt at its Rect are told apart.
func (in *inputState) onPath(prev *hitRegion, path []int32) bool {
	for _, i := range path {
		if gem.Same(in.regions[i].pointer, prev.pointer) {
			return true
		}
	}
	for _, i := range path {
		if in.sameRegion(&in.regions[i], prev, false) {
			return true
		}
	}
	return false
}

// matchOf returns the entry of last that now continues, or nil: the one
// with its handler before one matched by ID or Rect.
func (in *inputState) matchOf(now *hitRegion, last []hitRegion) *hitRegion {
	for j := range last {
		if gem.Same(now.pointer, last[j].pointer) {
			return &last[j]
		}
	}
	for j := range last {
		if in.sameRegion(now, &last[j], false) {
			return &last[j]
		}
	}
	return nil
}

// withinPath is a path of within regions: copies of the regions, the
// innermost first, and their within handlers, which a copy does not carry.
type withinPath struct {
	at []hitRegion
	h  []any
}

// set keeps the regions of in at the indices idx and their handlers, and
// lets go of those it kept before past them.
func (p *withinPath) set(in *inputState, idx []int32) {
	n := len(p.at)
	p.at, p.h = p.at[:0], p.h[:0]
	for _, i := range idx {
		p.at = append(p.at, in.regions[i])
		p.at[len(p.at)-1].unlink()
		p.h = append(p.h, in.withinOf(i))
	}
	if n > len(p.at) {
		clear(p.at[len(p.at):n])
		clear(p.h[len(p.h):n])
	}
}

// sameWithin reports whether the within region at i of this frame is the
// one prev, with the handler h, was copied from: the same handler; or else,
// when h is painted nowhere this frame, the same ID, or for a handler with
// none the same Rect.
func (in *inputState) sameWithin(i int32, prev *hitRegion, h any) bool {
	now := in.withinOf(i)
	if gem.Same(now, h) {
		return true
	}
	for _, e := range in.withins {
		if gem.Same(e.h, h) {
			return false
		}
	}
	r := &in.regions[i]
	if prev.id != nil {
		return gem.Same(r.id, prev.id)
	}
	return r.id == nil && r.rect == prev.rect
}

// updateFocusWithin finds the focus path of focused, the region of this
// frame's that has focus, or nil, and tells the FocusWithinHandlers that
// joined it and left it: those that left, the innermost first, that focus
// is no longer inside them; then those that joined, the outermost first,
// and those rebuilt since, that it is.
func (in *inputState) updateFocusWithin(focused *hitRegion) {
	last := &in.focusWithin
	if len(last.at) == 0 && (focused == nil || !in.index().within) {
		return
	}
	path := in.focusAt[:0]
	if focused != nil {
		for i := in.indexOf(focused); i >= 0; i = in.regions[i].parent - 1 {
			if _, ok := in.withinOf(i).(FocusWithinHandler); ok {
				path = append(path, i)
			}
		}
	}
	in.focusAt = path
	told := false
	for j := range last.at {
		if in.onWithinPath(&last.at[j], last.h[j], path) {
			continue
		}
		h := last.h[j]
		for _, e := range in.withins {
			if in.sameWithin(e.at, &last.at[j], h) {
				// Rebuilt since it was told: the handler holding it now.
				h = e.h
				break
			}
		}
		if fh, ok := h.(FocusWithinHandler); ok {
			fh.HandleFocusWithin(false)
			told = true
		}
	}
	for _, i := range slices.Backward(path) {

		h := in.withinOf(i)
		if prev := in.withinMatch(i, last); prev >= 0 && gem.Same(h, last.h[prev]) {
			continue
		}
		h.(FocusWithinHandler).HandleFocusWithin(true)
		told = true
	}
	if told {
		RepaintAll()
	}
	last.set(in, path)
}

// onWithinPath reports whether prev, with the handler h, is one of the
// within regions at the indices path: by its handler before anything else.
func (in *inputState) onWithinPath(prev *hitRegion, h any, path []int32) bool {
	for _, i := range path {
		if gem.Same(in.withinOf(i), h) {
			return true
		}
	}
	for _, i := range path {
		if in.sameWithin(i, prev, h) {
			return true
		}
	}
	return false
}

// withinMatch returns the index in p of the entry the within region at i
// continues, or -1: the one with its handler before one matched by ID or
// Rect.
func (in *inputState) withinMatch(i int32, p *withinPath) int {
	h := in.withinOf(i)
	for j := range p.at {
		if gem.Same(h, p.h[j]) {
			return j
		}
	}
	for j := range p.at {
		if in.sameWithin(i, &p.at[j], p.h[j]) {
			return j
		}
	}
	return -1
}

// indexOf returns the index of r in the regions, or -1 when r does not
// point into them.
func (in *inputState) indexOf(r *hitRegion) int32 {
	if len(in.regions) == 0 {
		return -1
	}
	off := uintptr(unsafe.Pointer(r)) - uintptr(unsafe.Pointer(&in.regions[0]))
	if i := off / unsafe.Sizeof(hitRegion{}); i < uintptr(len(in.regions)) && &in.regions[i] == r {
		return int32(i)
	}
	return -1
}

// The scroll latch: a trackpad's gesture, a scroll in pixels with a
// ScrollPhase, goes along an axis to the view that took it as it began, as
// a browser latches a gesture, its momentum included, whatever that view
// reports: a view at its end does not hand the rest of the gesture to the
// view around it. A wheel's notches and a scroll with no phase are not
// latched, and chain from the view under the pointer outward as before.

// gesture notes the frame's phase: the fingers starting to move begin a
// new gesture, which they also do when they move again after lifting, for
// a ScrollPhaseBegan merged into the frame of the lift before it.
func (in *inputState) gesture(phase ScrollPhase) {
	switch phase {
	case ScrollPhaseBegan:
		in.unlatch()
		in.lifted = false
	case ScrollPhaseChanged:
		if in.lifted {
			in.unlatch()
		}
		in.lifted = false
	case ScrollPhaseEnded, ScrollPhaseMomentum, ScrollPhaseMomentumEnded:
		in.lifted = true
	}
}

// unlatch ends the latch on both axes.
func (in *inputState) unlatch() {
	for a := range in.latched {
		if in.latched[a] != nil {
			in.latched[a] = in.latch[a].keep(nil)
		}
	}
}

// sendGesture sends a scroll along one axis, or along none for a lift: a
// gesture's to the region latched on its axis, if one is, after the within
// handlers around it, and otherwise through send, latching what takes it.
func (in *inputState) sendGesture(ev PointerEvent) {
	if !ev.ScrollPixels || ev.ScrollPhase == ScrollPhaseNone {
		in.send(ev)
		return
	}
	if ev.Scroll == (Point{}) {
		// The fingers lifting, which moves nothing: to the views that
		// took the gesture, else as any scroll.
		x, y := in.latchedOn(0), in.latchedOn(1)
		if x != nil {
			in.sendLatched(x, ev)
		}
		if y != nil && y != x {
			in.sendLatched(y, ev)
		}
		if x == nil && y == nil {
			in.send(ev)
		}
		return
	}
	a := 1
	if ev.Scroll.Y == 0 {
		a = 0
	}
	if cur := in.latchedOn(a); cur != nil {
		in.sendLatched(cur, ev)
		return
	}
	if r := in.send(ev); r != nil {
		in.latched[a] = in.latch[a].keep(r)
	}
}

// latchedOn returns the region of this frame latched on axis a, or nil,
// ending the latch when the region is gone or a focus trap it is not in
// has opened.
func (in *inputState) latchedOn(a int) *hitRegion {
	l := in.latched[a]
	if l == nil {
		return nil
	}
	cur := in.findPointer(l)
	if cur != nil {
		if scope := in.activeScope(); scope != nil && (cur.scope == nil || !gem.Same(scope.owner, cur.scope.owner)) {
			cur = nil
		}
	}
	if cur == nil {
		in.latched[a] = in.latch[a].keep(nil)
	}
	return cur
}

// sendLatched hands ev to cur, the region latched, after the within
// handlers on its chain, any of which may take it. What cur returns is not
// acted on: the gesture stays with it.
func (in *inputState) sendLatched(cur *hitRegion, ev PointerEvent) {
	if in.index().within {
		in.offered = in.offered[:0]
		if i := in.indexOf(cur); i >= 0 {
			if _, took := in.offerWithin(i, ev); took {
				return
			}
		}
	}
	in.deliver(cur, ev)
}
