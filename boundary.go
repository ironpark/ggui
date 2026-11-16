package ggui

import "github.com/ironpark/ggui/internal/reactive"

// A widget whose own size and place do not change when what is inside it
// changes size, and which paints nothing of what is inside beyond its own
// Rect, holds such a change: nothing outside it moves, nothing outside it
// is painted differently, and a frame nothing else changed for need paint
// only where it is. A Scroll is one, its viewport keeping its size while
// its content grows; Contain makes one of any widget, and ui.Resizable's
// panes are each one. Invalidate asks the nearest one around the widget
// whose size changed to hold the layout it asks for, and the frame that
// runs it is painted within that boundary alone, as one a scroll's move
// asks for is; see damage.go.
//
// That a boundary keeps its size is assumed, not known: a Contain given
// loose constraints may well follow its content. So the boundary checks
// the layout it held, and every other it is given in the same settle, as
// a parent measuring a child twice gives it two: each must come out as the
// last one under the same constraints did, size and baseline, for what
// lies around it to be laid out as before. One that does not was sized by
// its content after all: the frame is painted whole, and it holds nothing
// again, deferring to the boundary around it. One given constraints it has
// not been laid out under lately cannot be checked, which happens when
// something outside changed too; that frame is painted whole, and the
// boundary stays one.
//
// A boundary clips what is inside it to its Rect: that is what keeps a
// shadow, a focus ring or an overflowing child within the part of the
// window the frame paints. Box and the flows do not clip, and their
// children may draw well past them, which is why they are not boundaries
// however tightly they are sized. An overlay a widget inside opens paints
// past it too; a frame after one was painted is painted whole anyway.
//
// A boundary does not stop Invalidate from marking the caches above it
// stale, though its size, once checked, did not change: layout starts at
// the root, and a cache above that held would hand back its size without
// laying out the boundary inside it at all.

// LayoutBoundary is what a widget keeps that holds a change of size made
// inside it, as described above: a Scroll's, a Contain's, a split pane's.
// The widget calls Begin and End around the layout of what is inside, lays
// that out under the Env Begin returns, and calls Paint from its own Paint
// with the Rect it was given, clipping what is inside to it:
//
//	func (p *pane) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
//		inner, check := p.hold.Begin(env)
//		size := c.Constrain(p.child.Layout(c, inner))
//		p.hold.End(check, c, size, 0, false)
//		return size
//	}
//
//	func (p *pane) Paint(dst *ggui.Canvas, r ggui.Rect) {
//		p.hold.Paint(dst, r)
//		dst.Clip(r).Paint(p.child, r)
//	}
//
// A widget that changes what it shows inside the boundary with no change
// of size, as a Scroll moving its content does, makes the change through
// Contained, for the frame to be painted within the boundary alone. The
// zero value is ready to use; a boundary is not copied once laid out.
type LayoutBoundary struct {
	// win is the part of the window the last paint showed the boundary in,
	// placed whether it could tell, as outside a Transform or a shape clip.
	win    Rect
	placed bool
	// held is that something inside asked, through Invalidate, for a layout
	// this boundary holds, which its next layout checks; checkSeq is the
	// settle whose layouts it checks, and laidOut that it was laid out.
	held     bool
	checkSeq uint64
	laidOut  bool
	// sizedByContent is that a layout it held changed its size or
	// baseline: it holds nothing from then on.
	sizedByContent bool
	outer          *LayoutBoundary // the nearest boundary around it
	// laid are its last layouts under as many different constraints, the
	// next to be replaced at next.
	laid [boundaryLayouts]boundaryLaid
	next int
	// inner is the Env Begin returned last, derived from the one whose
	// revision is from, and none before the first: laid out under that
	// again, as a boundary at rest is, it is the answer, found without the
	// Env's memo. Envs of one revision are copies of one another.
	inner Env
	from  uint64
}

// boundaryLayouts is how many layouts under different constraints a
// boundary keeps to check the next against: a parent measuring a child at
// its own size and again stretched gives it two.
const boundaryLayouts = 4

// boundaryLaid is a layout a boundary gave: what it was given and what
// the parent read of it.
type boundaryLaid struct {
	cons     Constraints
	size     Size
	baseline float64
	based    bool // it had a baseline
	set      bool
}

// boundaryKey carries a boundary to what is laid out inside it, for
// Invalidate. Each stores itself, the same pointer every layout, so the
// Env it hands down keeps its revision and the caches below hold.
var boundaryKey = NewEnvKey[*LayoutBoundary]("layout boundary")

// boundaryOf returns the nearest boundary around what is laid out under
// env, if any: one stored inside another takes its place in the Env.
func boundaryOf(env Env) *LayoutBoundary {
	b, _ := env.Get(boundaryKey)
	return b
}

// settleSeq numbers the settles of the frame loop running on this
// goroutine, zero outside one.
func settleSeq() uint64 {
	if l := runningLoop(); l != nil {
		return l.settles
	}
	return 0
}

// holding returns the boundary that holds a change of size made inside b:
// b, or the nearest one around it whose size does not follow its
// content's, and nil when there is none or where it was painted cannot be
// told.
func (b *LayoutBoundary) holding() *LayoutBoundary {
	for ; b != nil; b = b.outer {
		if !b.placed || !b.laidOut {
			return nil
		}
		if !b.sizedByContent {
			return b
		}
	}
	return nil
}

// hold takes a layout asked for inside b: the change shows within b alone,
// where the last paint put it, and b's next layout checks it.
func (b *LayoutBoundary) hold() {
	b.held = true
	noteContained(b.win)
}

// Begin starts a layout of the boundary's widget under env, before what is
// inside it is laid out, which is laid out under inner: env carrying the
// boundary, so that a change of size made there asks this boundary to hold
// it. check is End's, and says whether this layout is checked: the one a
// change held asked for, or another in the same settle.
func (b *LayoutBoundary) Begin(env Env) (inner Env, check bool) {
	b.outer = boundaryOf(env)
	check = b.held
	if b.held {
		b.held = false
		b.checkSeq = settleSeq()
	} else if b.checkSeq != 0 && b.checkSeq == settleSeq() {
		check = true
	}
	if b.inner.vals == nil || env.rev != b.from {
		b.inner, b.from = env.With(boundaryKey, b), env.rev
	}
	return b.inner, check
}

// End ends the layout Begin started, under c, which came out at size and,
// when based is set, with baseline: what the widget hands its parent. check
// is Begin's, or set for a layout the widget knows to change nothing
// outside it either, as a Scroll's after a move made in Contained. A
// layout checked that came out other than the last under c did has the
// frame painted whole and, as one its content sized, demotes the boundary.
func (b *LayoutBoundary) End(check bool, c Constraints, size Size, baseline float64, based bool) {
	now := boundaryLaid{cons: c, size: size, baseline: baseline, based: based, set: true}
	at := -1
	for i, l := range b.laid {
		if l.set && l.cons == c {
			at = i
			break
		}
	}
	if check && b.laidOut {
		switch {
		case at < 0:
			// Constraints it was not laid out under lately: something
			// outside changed as well, and what moved cannot be told.
			paintWhole()
		case b.laid[at] != now:
			b.sizedByContent = true
			paintWhole()
			if b.held {
				// Asked for again as it was laid out, by an easing on its
				// way: the boundary around holds that, or none does.
				b.held = false
				if o := b.outer.holding(); o != nil {
					o.hold()
				} else {
					reactive.RequestLayout()
				}
			}
		}
	}
	if at < 0 {
		at = b.next
		b.next = (b.next + 1) % boundaryLayouts
	}
	b.laid[at] = now
	b.laidOut = true
}

// paintWhole has the frame being laid out painted whole, though a boundary
// held what it was to paint: the frame need not be laid out again, as the
// parent has the size that changed already, nor the next painted whole.
func paintWhole() {
	if l := runningLoop(); l != nil {
		l.unheld = true
		return
	}
	reactive.RequestLayout()
}

// Paint notes where the boundary is painted, at r: the widget calls it
// from its own Paint, with the Rect it was given. A layout asked for in
// the last layout, before this paint, is held where the boundary is now,
// which a move outside it may have changed, and taken whole where it
// cannot be told. A frame painted in part leaves it all as it was: it
// paints within its regions, not where the boundary shows.
func (b *LayoutBoundary) Paint(dst *Canvas, r Rect) {
	if dst == nil || dst.fs().partial {
		return
	}
	win, placed := dst.WindowRect(r)
	if placed {
		win = dst.Visible(win)
	}
	b.win, b.placed = win, placed
	if !b.held {
		return
	}
	if placed {
		noteContained(win)
	} else {
		reactive.RequestLayout()
	}
}

// Placed reports whether the last frame painted whole could tell where in
// the window the boundary is: whether it was painted, and not under a
// Transform or a shape clip. Contained holds a change within it only then.
func (b *LayoutBoundary) Placed() bool { return b.placed }

// Contained makes change, which changes what is inside the boundary and
// nothing else, and reports whether the change was held there: where the
// boundary is placed, the signals change writes and the layouts it asks
// for, Relayout's among them, are taken to show within the boundary alone,
// and a frame nothing else changed for is painted there and nowhere else.
// It is how a Scroll moves its content. Where the boundary is not placed,
// change is made as any change is, and the frame is painted whole. What
// reacts outside the boundary to a signal change writes reacts as to any
// write, and has the frame painted whole too.
func (b *LayoutBoundary) Contained(change func()) bool {
	if !b.placed {
		change()
		return false
	}
	reactive.Contained(change)
	noteContained(b.win)
	return true
}
