package ggui

import "github.com/ironpark/ggui/gem"

// paintedInput is what a paint registers for input to take when it ends:
// the hit regions, with the last frame's for Adopter handoff, and the
// requests, observers and shortcuts widgets made as they painted.
type paintedInput struct {
	hits []hitRegion
	prev []hitRegion // last frame's regions, for Adopter handoff
	// withins are the handlers of the hits registered with HitWithin, and
	// prevWithins last frame's, which input routes to until this paint
	// ends.
	withins, prevWithins []withinEntry
	focusRequest         KeyHandler
	focusClaim           *FocusRef // a Focus waiting for the next key region painted
	revealRequests       []Rect
	inputObservers       []inputObserver
	shortcuts            []paintedShortcut
}

// regionSwap records the spare buffers rotated out by swapBuffers so a frame
// that ends before painting can restore hits/prev without allocating a closure.
type regionSwap struct {
	spare        []hitRegion
	spareWithins []withinEntry
}

// swapBuffers rotates hits/withins into prev/prevWithins and clears the spare
// buffers for the new paint, keeping last frame's regions readable for Adopter
// handoff and input routing until the paint finishes.
func (p *paintedInput) swapBuffers() regionSwap {
	spare, spareWithins := p.prev, p.prevWithins
	clear(spare)
	clear(spareWithins)
	spare, spareWithins = spare[:0], spareWithins[:0]
	p.prev, p.hits = p.hits, spare
	p.prevWithins, p.withins = p.withins, spareWithins
	return regionSwap{spare: spare, spareWithins: spareWithins}
}

// unswapBuffers restores the double-buffer state after an aborted paint before
// anything painted into the spare buffers.
func (p *paintedInput) unswapBuffers(s regionSwap) {
	p.hits, p.prev = p.prev, s.spare
	p.withins, p.prevWithins = p.prevWithins, s.spareWithins
}

// regionCursor is where in the region tree the paint is, as indices plus
// one into the paint's regions, 0 for none. Canvas.Paint saves it as it
// opens a widget's scope and makes at the widget's outer, and restores it
// as the scope closes, so it needs nothing of a scope to be correct where a
// widget is painted with none open, as the root is in a test.
type regionCursor struct {
	at    int32 // the last region the widget painting registered, else outer
	outer int32 // the region the widget painting is inside
}

// inputView is where input stood as the paint began, which the Canvas's
// focus and pointer queries answer from: the paint cannot see input move.
type inputView struct {
	pointer    Point
	hasPointer bool
	// pointerRead records that a widget read the pointer in the last
	// whole paint, or in a part painted since: what such a widget draws
	// follows the pointer with no event reaching it, so a mouse move paints
	// it again. pointerAt are the bounds those widgets drew in, in the
	// window, which the move paints in part, as it would a widget that read
	// the time; pointerLost, that one read it where that cannot be told,
	// under a Transform or outside any widget's Paint, and the move repaints
	// the whole window.
	pointerRead  bool
	pointerLost  bool
	pointerAt    []Rect
	focusBounds  Rect       // focused input region at the start of this paint
	focusKey     KeyHandler // its handler
	focusPainted bool       // the focused handler registered since PaintFocus began
	focusVisible bool       // focus came there by keyboard
	pressed      bool       // a pointer button is held
	pressPointer PointerHandler

	// found is what the paint learns of its regions as it registers them,
	// for input to start from: see regionIndex.
	found regionIndex
}

// take records where in stands, for a paint about to begin.
func (v *inputView) take(in *inputState) {
	v.focusBounds, v.focusKey, v.focusPainted = Rect{}, nil, false
	if in.focused != nil {
		v.focusBounds, v.focusKey = in.focused.rect, in.focused.key
	}
	v.focusVisible = in.focused != nil && in.focusVisible
	v.pressed, v.pressPointer = in.pressed != nil, nil
	if in.pressed != nil {
		v.pressPointer = in.pressed.pointer
	}
	v.pointerRead, v.pointerLost, v.pointerAt = false, false, v.pointerAt[:0]
	v.found = regionIndex{scopeAt: -1}
	if v.focusKey != nil {
		v.found.key = handlerHint{h: v.focusKey, at: -1}
	}
	if v.pressPointer != nil {
		v.found.pointer = handlerHint{h: v.pressPointer, at: -1}
	}
}

// note records that the region at i of the paint's regions is r, which
// was just registered there or merged into: the last registered of a
// handler or a scope is the topmost.
func (v *inputView) note(r *hitRegion, i int) {
	x := &v.found
	if r.scope != nil {
		x.scopeAt = i
	}
	if r.shaped {
		x.cursors = true
	}
	if r.within {
		x.within = true
	}
	if r.key != nil && x.key.h != nil && gem.Same(r.key, x.key.h) {
		x.key.at = i
	}
	if r.pointer != nil && x.pointer.h != nil && gem.Same(r.pointer, x.pointer.h) {
		x.pointer.at = i
	}
}

// HitMark is a place in the frame's hit regions, which KeysSince reads
// from: what Canvas.HitMark returned was where the regions registered next
// begin. The zero HitMark is the start of the frame. A mark is good for
// the frame it was taken in.
type HitMark struct{ n int }

// focusScope is a focus trap for one frame: while regions carrying it
// exist, Tab cycles within them and an unconsumed Escape calls escape.
// Frames compare scopes by owner.
type focusScope struct {
	owner  any
	escape func()
}

// paintedShortcut is a chord a widget listens for while it is painted.
type paintedShortcut struct {
	chord Chord
	fn    func()
	scope *focusScope
}

type inputObserver struct {
	rect   Rect
	scope  *focusScope
	notify func()
}
