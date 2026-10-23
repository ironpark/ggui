package ggui

// paintSpace is the coordinate space a Canvas paints in when it is not the
// window's: inside a Transform, whose child paints untransformed, or inside
// a shape clip, which hit-tests against a shape rather than a Rect. A hit
// region keeps the space it was registered in, so input finds it through the
// transform and is told where it landed in the child's own coordinates.
// A nil space is the window's.
type paintSpace struct {
	parent *paintSpace
	// toParent takes this space's points to the parent's, and toLocal back.
	toParent, toLocal Affine
	// outer is the parent's clip where the space began, in the parent's
	// coordinates, when clipped is set. A transformed space starts
	// unclipped, so the clip it was painted under is kept here.
	outer   Rect
	clipped bool
	// inside reports whether a point in this space is within the shape a
	// ClipPath or ClipOval clips to; nil takes every point.
	inside func(Point) bool
}

// local returns where the window point p is in s, ignoring clips.
func (s *paintSpace) local(p Point) Point {
	if s == nil {
		return p
	}
	return s.toLocal.Apply(s.parent.local(p))
}

// reach returns where the window point p is in s, and false when a clip or
// a shape between the window and s hides it.
func (s *paintSpace) reach(p Point) (Point, bool) {
	if s == nil {
		return p, true
	}
	q, ok := s.parent.reach(p)
	if !ok || s.clipped && !s.outer.Contains(q) {
		return q, false
	}
	q = s.toLocal.Apply(q)
	if s.inside != nil && !s.inside(q) {
		return q, false
	}
	return q, true
}

// affine returns the transform from s's coordinates to the window's,
// leaving out the clips and shapes on the way.
func (s *paintSpace) affine() Affine {
	var m Affine
	for ; s != nil; s = s.parent {
		m = m.Then(s.toParent)
	}
	return m
}

// window returns the window Rect that holds r in s, cut to the clips
// between; full skips the cuts.
func (s *paintSpace) window(r Rect, full bool) Rect {
	for ; s != nil; s = s.parent {
		r = s.toParent.Bounds(r)
		if s.clipped && !full {
			r = r.Intersect(s.outer)
		}
	}
	return r
}

// toWindow returns the window Rect that holds the Rect r in c's space.
func (c *Canvas) toWindow(r Rect) Rect {
	if c == nil || c.space == nil {
		return r
	}
	return c.space.window(r, true)
}

// enter returns a Canvas like c that paints in a space reached from c's by
// m, in which a point counts only when inside accepts it. A space that moves
// nothing keeps c's clip, which is already in its coordinates; one that
// does starts unclipped and keeps c's clip as its outer bound.
func (c *Canvas) enter(m, inverse Affine, inside func(Point) bool) *Canvas {
	child := c.derive()
	s := &paintSpace{parent: c.space, toParent: m, toLocal: inverse, inside: inside}
	if !m.IsIdentity() {
		s.outer, s.clipped = c.clip, c.clipped
		child.clip, child.clipped = Rect{}, false
	}
	child.space = s
	return child
}

// placeRegion finishes a region registered in s: rect becomes the window
// Rect that holds it and local keeps it as it was painted. It reports false
// for a region left with no area and no key handler.
func (s *paintSpace) placeRegion(h *hitRegion) bool {
	h.space, h.local, h.origin = s, h.rect, h.full.Origin
	h.full = s.window(h.full, true)
	h.rect = s.window(h.rect, false)
	if h.local.Empty() || h.rect.Empty() {
		if h.key == nil {
			return false
		}
		h.rect = Rect{Origin: h.full.Origin}
	}
	return true
}

// contains reports whether the window point p lands on r.
func (r *hitRegion) contains(p Point) bool {
	if !r.rect.Contains(p) {
		return false
	}
	if r.space == nil {
		return true
	}
	q, ok := r.space.reach(p)
	return ok && r.local.Contains(q)
}

// at returns the window point p in the coordinates r was painted in, which
// is where its handler expects an event.
func (r *hitRegion) at(p Point) Point { return r.space.local(p) }

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
