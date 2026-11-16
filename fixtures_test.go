package ggui

import "image/color"

// The kernel's tests stand on widgets of their own, made here on the
// kernel's widget protocol alone, rather than on the built-in ones: those
// are in package widget, which imports this one, and a test of the kernel
// could not import them back without a cycle. Each does the least a test of
// the kernel asks of a widget of its kind, and is no copy of the built-in
// widget it stands in for: a test of what a built-in widget does belongs
// with the widgets.

// block is as big as its constraints allow of size, whatever its child,
// filled with fill when it has one, and lays its child out at the top left
// under loose constraints of its own size: the stand-in for a Box of a
// Size.
type block struct {
	size  Size
	fill  color.Color
	child Widget
	inner Size // the child's, as laid out
}

// sized returns a block w by h that paints nothing.
func sized(w, h float64) *block { return &block{size: Sz(w, h)} }

// filled returns a block w by h filled with c.
func filled(w, h float64, c color.Color) *block { return &block{size: Sz(w, h), fill: c} }

// around returns a block w by h that holds child.
func around(w, h float64, child Widget) *block { return &block{size: Sz(w, h), child: child} }

func (b *block) Layout(c Constraints, env Env) Size {
	size := c.Constrain(b.size)
	if b.child != nil {
		b.inner = b.child.Layout(Loose(size), env)
	}
	return size
}

func (b *block) Paint(dst *Canvas, r Rect) {
	if b.fill != nil {
		dst.FillRect(r, b.fill)
	}
	if b.child != nil {
		dst.Paint(b.child, Rct(r.Origin, b.inner))
	}
}

// held clips its child to its own Rect, and holds a change of size made
// inside it there, with a LayoutBoundary: the stand-in for Contain.
type held struct {
	child Widget
	size  Size // the child's, as laid out
	hold  LayoutBoundary
}

func (h *held) Layout(c Constraints, env Env) Size {
	inner, check := h.hold.Begin(env)
	h.size = h.child.Layout(c, inner)
	size := c.Constrain(h.size)
	baseline, based := BaselineOf(h.child)
	h.hold.End(check, c, size, baseline, based)
	return size
}

func (h *held) Paint(dst *Canvas, r Rect) {
	h.hold.Paint(dst, r)
	dst.Clip(r).Paint(h.child, Rct(r.Origin, h.size))
}

// label is a leaf 10 high and 10 wide for each byte of its text that tells
// the semantics tree it shows that text: the stand-in for Text, for a test
// that tells one leaf from another or looks for text in the tree.
type label struct{ text string }

func labelled(text string) *label { return &label{text} }

func (l *label) Layout(c Constraints, _ Env) Size {
	return c.Constrain(Sz(10*float64(len(l.text)), 10))
}

func (l *label) Paint(dst *Canvas, r Rect) { dst.Leaf(r, Node{Role: RoleText, Name: l.text}) }

// InspectLabel implements inspect.Labeler: the inspector lists a label by
// its text, as it does a Text.
func (l *label) InspectLabel() string { return l.text }

// pile lays its children out under loose constraints, each at the size it
// takes, one after another across, as a row, or down, as a column, or each
// at the top left, as a stack, and is as big as they come to within its
// constraints: the stand-in for Row, Column, Stack and Align at the top
// left, with none of their alignment, gaps or flex.
type pile struct {
	axis  byte // 'x' for a row, 'y' for a column, 0 for a stack
	kids  []Widget
	rects []Rect
}

func row(kids ...Widget) *pile    { return &pile{axis: 'x', kids: kids} }
func column(kids ...Widget) *pile { return &pile{axis: 'y', kids: kids} }
func stack(kids ...Widget) *pile  { return &pile{kids: kids} }

func (p *pile) Layout(c Constraints, env Env) Size {
	p.rects = p.rects[:0]
	var at Point
	var size Size
	for _, k := range p.kids {
		s := k.Layout(Loose(c.Max()), env)
		p.rects = append(p.rects, Rct(at, s))
		size = Sz(max(size.W, at.X+s.W), max(size.H, at.Y+s.H))
		switch p.axis {
		case 'x':
			at.X += s.W
		case 'y':
			at.Y += s.H
		}
	}
	return c.Constrain(size)
}

func (p *pile) Paint(dst *Canvas, r Rect) {
	for i, k := range p.kids {
		dst.Paint(k, Rct(r.Origin.Add(p.rects[i].Origin), p.rects[i].Size))
	}
}

// inset lays child out within by of every side of it: the stand-in for
// Padding.
type inset struct {
	child Widget
	by    EdgeInsets
}

// padded returns child inset by sides, read as Insets reads them.
func padded(child Widget, sides ...float64) *inset { return &inset{child, Insets(sides...)} }

func (in *inset) Layout(c Constraints, env Env) Size {
	return c.Constrain(in.by.Inflate(in.child.Layout(in.by.Shrink(c), env)))
}

func (in *inset) Paint(dst *Canvas, r Rect) {
	b := in.by
	dst.Paint(in.child, Rct(r.Origin.Add(Pt(b.Left, b.Top)), Sz(r.Size.W-b.Horizontal(), r.Size.H-b.Vertical())))
}

// pointed makes child's Rect a pointer region whose events go to handle,
// which says whether it took each, and asks for cursor there when
// withCursor set one: the stand-in for Pointer.
type pointed struct {
	child  Widget
	handle func(PointerEvent) bool
	cursor CursorShape
	shaped bool
}

func onPointer(child Widget, handle func(PointerEvent) bool) *pointed {
	return &pointed{child: child, handle: handle}
}

// tapped returns child as a region that takes a press, its release and
// the tap they make, calling fn for the tap: the stand-in for Tap.
func tapped(child Widget, fn func()) *pointed {
	return onPointer(child, func(ev PointerEvent) bool {
		switch ev.Kind {
		case PointerTap:
			fn()
			return true
		case PointerDown, PointerUp:
			return true
		}
		return false
	})
}

// takesNothing is the handler of a region that takes no pointer event.
func takesNothing(PointerEvent) bool { return false }

func (p *pointed) withCursor(shape CursorShape) *pointed {
	p.cursor, p.shaped = shape, true
	return p
}

func (p *pointed) HandlePointer(ev PointerEvent) bool { return p.handle(ev) }

func (p *pointed) Layout(c Constraints, env Env) Size { return p.child.Layout(c, env) }

func (p *pointed) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, p)
	if p.shaped {
		dst.HitCursor(r, p.cursor)
	}
	dst.Paint(p.child, r)
}

// focusable makes child's Rect a region that takes focus when clicked and
// hands handle the key events it hears: the stand-in for Focus.
type focusable struct {
	child  Widget
	handle func(KeyEvent)
}

func onKey(child Widget, handle func(KeyEvent)) *focusable {
	return &focusable{child: child, handle: handle}
}

func (f *focusable) HandleKey(ev KeyEvent) {
	if f.handle != nil {
		f.handle(ev)
	}
}

func (f *focusable) Layout(c Constraints, env Env) Size { return f.child.Layout(c, env) }

func (f *focusable) Paint(dst *Canvas, r Rect) {
	dst.HitKey(r, f)
	dst.Paint(f.child, r)
}

// scroller shows its child under a viewport its constraints' size, moved
// up by an offset that the scrolls reaching it move, between none and the
// child's overflow. Like a scroll, it takes a scroll that goes up or down
// a child that overflows, and of a fling's only one that moves it; and its
// region lies under its child's, which hears the pointer first. It is the
// stand-in for Scroll, for a test of how input reaches one.
type scroller struct {
	child    Widget
	offset   float64
	overflow float64
	content  Size
}

func (s *scroller) Layout(c Constraints, env Env) Size {
	s.content = s.child.Layout(Constraints{MaxW: c.MaxW, MaxH: Unbounded}, env)
	size := c.Constrain(c.Max())
	s.overflow = max(s.content.H-size.H, 0)
	s.offset = min(s.offset, s.overflow)
	return size
}

func (s *scroller) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, s)
	dst.Clip(r).Paint(s.child, Rct(r.Origin.Add(Pt(0, -s.offset)), s.content))
}

func (s *scroller) HandlePointer(ev PointerEvent) bool {
	dy := ev.ScrollBy().Y
	if ev.Kind != PointerScroll || dy == 0 || s.overflow == 0 {
		return false
	}
	before := s.offset
	s.offset = min(max(s.offset-dy, 0), s.overflow)
	return !ev.ScrollMomentum || s.offset != before
}

// readsAtLayout is a leaf 10 square that reads r each time it is laid out,
// as a Text bound to r does.
func readsAtLayout[T any](r Readable[T]) Widget {
	return FromFuncs(func(c Constraints, _ Env) Size {
		r.Get()
		return c.Constrain(Sz(10, 10))
	}, func(*Canvas, Rect) {})
}
