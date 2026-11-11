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

// layoutBoundary is what a widget that holds a change of size inside it
// keeps: a Scroll's and a Contain's.
type layoutBoundary struct {
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
	outer          *layoutBoundary // the nearest boundary around it
	depth          int             // how many boundaries it is in, itself included
	// laid are its last layouts under as many different constraints, the
	// next to be replaced at next.
	laid [boundaryLayouts]boundaryLaid
	next int
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

// boundaryKey carries a Contain to its subtree, for Invalidate; a Scroll
// is found under scrollPortKey, which it stores already. Each stores
// itself, the same pointer every layout, so the Env it hands down keeps
// its revision and the caches below hold.
var boundaryKey = NewEnvKey[*layoutBoundary]("layout boundary")

// boundaryOf returns the nearest boundary in env, a Contain's or a
// Scroll's, if any. The nearest of each is in env; which of those two is
// nearer the list does not say, as a value stored under a key already
// there takes the old one's place in it, but the deeper is: one is
// inside the other.
func boundaryOf(env Env) *layoutBoundary {
	var contain, scroll *layoutBoundary
	for n := env.vals; n != nil && (contain == nil || scroll == nil); n = n.next {
		switch n.key {
		case boundaryKey.id:
			contain = n.val.(*layoutBoundary)
		case scrollPortKey.id:
			scroll = &n.val.(*ScrollWidget).hold
		}
	}
	if contain == nil || scroll != nil && scroll.depth > contain.depth {
		return scroll
	}
	return contain
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
func (b *layoutBoundary) holding() *layoutBoundary {
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
func (b *layoutBoundary) hold() {
	b.held = true
	noteContained(b.win)
}

// begin starts a layout of the boundary under env, before its content's,
// and reports whether this layout is checked: the one held, or one in the
// same settle. The boundary is then stored in the Env its content is laid
// out under, a Contain's under boundaryKey.
func (b *layoutBoundary) begin(env Env) bool {
	b.outer = boundaryOf(env)
	b.depth = 1
	if b.outer != nil {
		b.depth = b.outer.depth + 1
	}
	check := b.held
	if b.held {
		b.held = false
		b.checkSeq = settleSeq()
	} else if b.checkSeq != 0 && b.checkSeq == settleSeq() {
		check = true
	}
	return check
}

// end ends a layout begin started, which came out at size with the
// baseline given, under c. A layout checked that came out other than the
// last under c did asks for the frame to be painted whole, and, as one the
// content sized, demotes the boundary.
func (b *layoutBoundary) end(check bool, c Constraints, size Size, baseline float64, based bool) {
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

// paint notes where the boundary is painted at r. A layout asked for in
// the last layout, before this paint, is held where the boundary is now,
// which a move outside it may have changed, and taken whole where it
// cannot be told. A frame painted in part leaves it all as it was: it
// paints within its regions, not where the boundary shows.
func (b *layoutBoundary) paint(dst *Canvas, r Rect) {
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

// ContainWidget lays its child out under the constraints it is given,
// clips it to its own Rect and holds a change of size inside it there.
// Build one with Contain.
type ContainWidget struct {
	child Widget
	size  Size // the child's, as laid out
	hold  layoutBoundary
}

// Contain clips child to the Rect it is painted in and makes it a layout
// boundary: a disclosure easing open inside, or anything else that calls
// Invalidate, is painted within it alone, and what lies beside it is left
// as the last frame painted it, while its size stays as it was. Wrap a
// panel, a card or a pane whose size its parent decides: a Box of fixed
// Size, an Expanded share of a Row, a split's pane.
//
//	ggui.Row(ggui.Contain(ggui.Box(details).Size(320, 480)), rest)
//
// A Contain whose size does follow its content's, as one under loose
// constraints may, costs a frame painted whole the first time its content
// grows, and holds nothing from then on; the boundary around it, a Scroll's
// or another Contain's, holds the change instead. Under a Transform it
// holds nothing either.
func Contain(child Widget) *ContainWidget { return &ContainWidget{child: child} }

// Layout implements Widget.
func (w *ContainWidget) Layout(c Constraints, env Env) Size {
	check := w.hold.begin(env)
	w.size = w.child.Layout(c, env.With(boundaryKey, &w.hold))
	size := c.Constrain(w.size)
	baseline, based := baselineOf(w.child)
	w.hold.end(check, c, size, baseline, based)
	return size
}

// Baseline implements Baseliner: the child's.
func (w *ContainWidget) Baseline() (float64, bool) { return baselineOf(w.child) }

// Paint implements Widget: the child, clipped to r.
func (w *ContainWidget) Paint(dst *Canvas, r Rect) {
	w.hold.paint(dst, r)
	dst.Clip(r).Paint(w.child, Rct(r.Origin, w.size))
}
