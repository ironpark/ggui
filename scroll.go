package ggui

import (
	"image/color"
	"math"
	"time"

	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/pkg/kinetics"
)

// Viewport is what a Scroll tells the subtree it lays out: how far along
// the scroll axis the window starts and how long it is, in logical pixels.
// A list that knows its items' sizes can then lay out only the ones in
// view; EachKeyed does with ItemExtent.
type Viewport struct {
	Offset, Extent float64
	Horizontal     bool
}

var (
	viewportKey = NewEnvKey[Viewport]("viewport")
	// The window of the nearest Scroll along each axis: a list inside a
	// Scroll that scrolls the other way, as a diff's long lines do, is
	// still windowed by the one along its own.
	viewportXKey = NewEnvKey[Viewport]("viewport x")
	viewportYKey = NewEnvKey[Viewport]("viewport y")
)

// ScrollViewport returns the window of the nearest enclosing Scroll, if any.
func ScrollViewport(env Env) (Viewport, bool) { return env.Get(viewportKey) }

// viewportAlong returns the window of the nearest enclosing Scroll that
// scrolls along the x axis, for horizontal, or the y axis.
func viewportAlong(env Env, horizontal bool) (Viewport, bool) {
	return env.Get(pick(horizontal, viewportXKey, viewportYKey))
}

// ScrollWidget shows a window onto a child that may be taller (or, with
// Horizontal, wider) than the space it has, and moves that window with the
// wheel or by dragging the scrollbar thumb. Build one with Scroll.
type ScrollWidget struct {
	// win is where the last paint put the viewport in the window, placed
	// whether it could tell, and containedMove that the wheel moved the
	// content since the last layout; see wheel.
	win           Rect
	placed        bool
	containedMove bool
	props         property.Owner
	child         Widget
	horizontal    bool
	shrink        bool // along its axis, as long as its content and no longer
	speed         float64
	bar           color.Color
	barSet        bool
	bound         Binding[float64]
	offset        float64
	follow        bool
	id            any

	// FollowEnd's state. A scroll laid out for the first time cannot know
	// yet whether it replaces one that was at its end, as Adopt says only
	// at paint, so it leaves the decision to Paint.
	laidOut, decideAtPaint bool
	adoptedAtEnd           *bool // whether the scroll it adopted was at its end

	childSize             Size
	viewport              Size
	laidAt                float64 // the offset the child was last laid out for
	anchorShift           float64 // how far the child asked, as it laid out, to move to keep what is in view
	cache                 *CachedWidget
	rect                  Rect // where the window was last painted
	thumbHit              Rect
	barEnv                Env
	thumbHovered          bool
	dragging              bool
	dragStart, dragOffset float64

	phys scrollPhysics // overscroll, snapping and animated scrolls; see scrollPhysics

	fade float64 // how far in the content fades out at an end with more past it
}

// Key gives the scroll an identity, so a rebuilt one that also moved keeps
// its offset. Without one its Rect identifies it.
func (s *ScrollWidget) Key(k any) *ScrollWidget { s.id = k; return s }

// HitID implements Identified.
func (s *ScrollWidget) HitID() any { return s.id }

// Adopt implements Adopter: a rebuilt scroll keeps its offset.
func (s *ScrollWidget) Adopt(prev any) {
	if p, ok := prev.(*ScrollWidget); ok {
		s.dragging, s.dragStart, s.dragOffset = p.dragging, p.dragStart, p.dragOffset
		s.thumbHovered = p.thumbHovered
		s.phys.adopt(&p.phys)
		defer s.phys.reapply(s) // once the offset is adopted too
		atEnd := p.laidOut && p.atEnd()
		s.adoptedAtEnd = &atEnd
		if s.bound != nil {
			return
		}
		s.offset = p.position()
		if s.offset != s.laidAt {
			s.cache.invalidate()
			reactive.RequestLayout()
		}
	}
}

// Scroll lets child take any height and scrolls it within the space Scroll
// is given. The offset lives in the widget and carries across a rebuild;
// bind it to a StateValue with Offset to read or set it.
func Scroll(child Widget) *ScrollWidget {
	return &ScrollWidget{child: child, speed: 20, id: reactive.AutoID()}
}

// Horizontal scrolls along the x axis instead of the y axis.
func (s *ScrollWidget) Horizontal() *ScrollWidget { s.horizontal = true; return s }

// FadeEdges fades the content out over width logical pixels at an end
// with more of it past that end, and as the end comes near, the fade
// narrows to nothing: a hint that the content goes on, without a line or
// a surface. It paints the content through a layer.
func (s *ScrollWidget) FadeEdges(width float64) *ScrollWidget { s.fade = max(width, 0); return s }

// Shrink makes the scroll, along its axis, only as long as its content
// while that fits, where it would otherwise take all it is offered: a row
// of tabs in a Flexible, with what follows them right after the last.
func (s *ScrollWidget) Shrink() *ScrollWidget { s.shrink = true; return s }

// Speed sets how many pixels one wheel unit moves. A unit is one mouse
// notch on the desktop; on the web, where browsers report pixels, it is
// 20 CSS pixels, so the default matches the browser's own scrolling.
func (s *ScrollWidget) Speed(px float64) *ScrollWidget {
	defer property.Watch(&s.props, &s.speed)()
	s.speed = px
	return s
}

// Bar overrides the inherited scrollbar color; nil hides the bar.
func (s *ScrollWidget) Bar(c color.Color) *ScrollWidget {
	defer property.Watch(&s.props, &s.bar)()
	defer property.Watch(&s.props, &s.barSet)()
	s.bar, s.barSet = c, true
	return s
}

// BindOffset binds the scroll position to sig: wheel input writes it, and
// writing it scrolls. Use it to keep the position across rebuilds or to
// scroll programmatically.
func (s *ScrollWidget) BindOffset(sig Binding[float64]) *ScrollWidget {
	property.Require(sig, "BindOffset")
	if !property.Same(s.bound, sig) {
		s.bound = sig
		s.props.Changed()
	}
	return s
}

// FollowEnd keeps the window at the end as the content grows, the way a
// log or a terminal follows new lines, for as long as it is there: scrolling
// back stops following and scrolling to the end again resumes it. A new
// scroll starts at the end.
func (s *ScrollWidget) FollowEnd() *ScrollWidget {
	defer property.Watch(&s.props, &s.follow)()
	s.follow = true
	return s
}

// Offset detaches a binding and sets local scroll position, clamped at layout.
func (s *ScrollWidget) Offset(v float64) *ScrollWidget {
	if s.bound != nil || !property.Equal(s.offset, v) {
		s.bound = nil
		s.offset = v
		s.props.Changed()
	}
	return s
}

func (s *ScrollWidget) extent(sz Size) float64 { return pick(s.horizontal, sz.W, sz.H) }

func (s *ScrollWidget) maxOffset() float64 {
	return max(s.extent(s.childSize)-s.extent(s.viewport), 0)
}

func (s *ScrollWidget) position() float64 {
	if s.bound != nil {
		return Untrack(s.bound.Get)
	}
	return s.offset
}

func (s *ScrollWidget) scrollTo(v float64) {
	v = clamp(v, 0, s.maxOffset())
	if s.bound != nil {
		s.bound.Set(v)
	} else if v != s.offset {
		s.offset = v
		reactive.RequestLayout() // what a virtualized child shows depends on it
	}
	if v != s.laidAt {
		s.cache.invalidate()
	}
}

// Layout implements Widget. After a move of the wheel's, all it changes
// laying the content out again for the new window, mounting what scrolls
// into it and keeping what is in view in place as it is measured, is
// contained in the viewport too, in every pass of the frame until Paint.
func (s *ScrollWidget) Layout(c Constraints, env Env) Size {
	if s.containedMove {
		var size Size
		reactive.Contained(func() { size = s.layout(c, env) })
		return size
	}
	return s.layout(c, env)
}

func (s *ScrollWidget) layout(c Constraints, env Env) Size {
	defer s.props.Layout().EndLayout()
	s.barEnv = env
	if !s.barSet {
		s.bar = env.ScrollStyle().Color
	}
	inner := c.Max()
	if s.horizontal {
		inner.W = Unbounded
	} else {
		inner.H = Unbounded
	}
	s.cache, _ = env.Get(cacheOwner)
	// Judged against the last layout, before the content can grow.
	follow := s.follow && s.laidOut && s.atEnd()
	if s.follow && !s.laidOut {
		s.decideAtPaint = true
	}
	s.phys.step(s)
	s.laidAt = s.position()
	vp := Viewport{Offset: s.laidAt, Extent: s.extent(c.Max()), Horizontal: s.horizontal}
	env = env.With(viewportKey, vp).With(scrollPortKey, s).
		With(pick(s.horizontal, viewportXKey, viewportYKey), vp).With(pick(s.horizontal, scrollPortXKey, scrollPortYKey), s)
	// What the content builds as it is laid out, rows mounting as they
	// scroll in, reacts within the viewport too; see Layout.
	reactive.InRegion(&s.containedMove, func() { s.childSize = s.child.Layout(Loose(inner), env) })
	s.viewport = c.Constrain(Sz(bounded(c.MaxW, s.childSize.W), bounded(c.MaxH, s.childSize.H)))
	if s.shrink {
		// Constrain keeps it within c, so a tight c still decides.
		if s.horizontal {
			s.viewport.W = c.Constrain(Sz(s.childSize.W, 0)).W
		} else {
			s.viewport.H = c.Constrain(Sz(0, s.childSize.H)).H
		}
	}
	s.laidOut = true
	shift := s.anchorShift
	s.anchorShift = 0
	if shift != 0 && s.phys.animating {
		s.phys.anim.from += shift
		s.phys.anim.to += shift
	}
	s.scrollTo(pick(follow, s.maxOffset(), s.position()+shift))
	s.phys.attach(s)
	return s.viewport
}

// keepInView moves the window by d once the child is laid out: a list
// whose rows above the window changed size asks for it, so what is in view
// stays where it is.
func (s *ScrollWidget) keepInView(d float64) { s.anchorShift += d }

// followFirst settles FollowEnd for a scroll's first layout, once Adopt
// has had its chance: a new scroll starts at the end, and one that
// replaces another follows if that one was at its end.
func (s *ScrollWidget) followFirst() {
	if !s.decideAtPaint {
		return
	}
	s.decideAtPaint = false
	if s.adoptedAtEnd == nil || *s.adoptedAtEnd {
		s.scrollTo(s.maxOffset())
	}
}

// atEnd reports whether the window shows the end of the content, within
// half a pixel.
func (s *ScrollWidget) atEnd() bool { return s.position() >= s.maxOffset()-0.5 }

// Paint implements Widget.
func (s *ScrollWidget) Paint(dst *Canvas, r Rect) {
	if s.position() != s.laidAt {
		// Written through the bound signal since the layout: a virtualized
		// child must be laid out again for the new window.
		s.cache.invalidate()
	}
	s.rect = r
	s.win, s.placed = dst.WindowRect(r)
	s.containedMove = false
	dst.HitPointer(r, s) // adopts the scroll this one replaces
	s.followFirst()
	s.phys.paint(s)
	dst.Node(r, Node{Role: RoleGroup, Actions: ActionScrollIntoView}, func(dst *Canvas) {
		clip := dst.Clip(r)
		if s.fade <= 0 {
			s.paintContent(clip, r)
			return
		}
		pos := s.position()
		lead, trail := min(s.fade, max(pos, 0)), min(s.fade, max(s.maxOffset()-pos, 0))
		clip.FadeEdges(r, s.horizontal, lead, trail, func(dst *Canvas) { s.paintContent(dst, r) })
	})
	s.paintBar(dst, r)
}

func (s *ScrollWidget) thumbLength() float64 {
	track := s.extent(s.rect.Size)
	if s.extent(s.childSize) <= 0 {
		return 0
	}
	return min(track, max(track*track/s.extent(s.childSize), 16))
}

func (s *ScrollWidget) paintBar(dst *Canvas, r Rect) {
	s.thumbHit = Rect{}
	track, content := s.extent(r.Size), s.extent(s.childSize)
	bar := ResolveColor(s.bar, s.barEnv)
	if bar == nil || content <= track || track <= 0 {
		s.dragging, s.thumbHovered = false, false
		return
	}
	const margin, hitWidth = 2.0, 10.0
	style := s.barEnv.ScrollStyle()
	active := s.thumbHovered || s.dragging
	amount := dst.Ease(Anchor{Rect: r, ID: scrollThumb{s}.HitID()}, scrollThumbHoverSlot, pick(active, 1.0, 0.0), s.barEnv.Motion(style.Duration))
	thickness := 3 + 1.5*amount
	fill := bar
	if amount > 0 {
		// Keep the thumb close to its resting gray, including while dragging.
		fill = LerpColor(bar, style.HoverColor, amount*pick(s.dragging, .28, .18))
	}
	thumb := s.thumbLength()
	at := (track - thumb) * s.position() / s.maxOffset()
	var visual Rect
	if s.horizontal {
		visual = Rct(Pt(r.Origin.X+at, r.Origin.Y+r.Size.H-thickness-margin), Sz(thumb, thickness))
		s.thumbHit = Rct(Pt(r.Origin.X+at, r.Origin.Y+max(0, r.Size.H-hitWidth)), Sz(thumb, min(hitWidth, r.Size.H)))
	} else {
		visual = Rct(Pt(r.Origin.X+r.Size.W-thickness-margin, r.Origin.Y+at), Sz(thickness, thumb))
		s.thumbHit = Rct(Pt(r.Origin.X+max(0, r.Size.W-hitWidth), r.Origin.Y+at), Sz(min(hitWidth, r.Size.W), thumb))
	}
	// Register after the content so grabbing the thumb never activates a row.
	dst.HitPointer(s.thumbHit, scrollThumb{s})
	dst.HitCursor(s.thumbHit, CursorShapePointer)
	dst.Clip(r).FillRoundRect(visual, thickness/2, fill)
}

// A separate handler keeps the thumb from being treated as a second scroll
// viewport when keyboard focus asks enclosing Revealer widgets to move.
type scrollThumb struct{ scroll *ScrollWidget }

var scrollThumbHoverSlot = NewSlot[*Motion]("scroll thumb hover")

func (h scrollThumb) HandlePointer(ev PointerEvent) bool {
	switch ev.Kind {
	case PointerMove, PointerEnter:
		h.scroll.thumbHovered = h.scroll.thumbHit.Contains(ev.Pos)
	case PointerExit:
		h.scroll.thumbHovered = false
	}
	return h.scroll.HandlePointer(ev)
}
func (h scrollThumb) CaptureTouchDrag() bool { return h.scroll.dragging }
func (h scrollThumb) HitID() any {
	if h.scroll.id != nil {
		return struct{ Thumb any }{h.scroll.id}
	}
	return struct{ Thumb Rect }{h.scroll.rect}
}

// CaptureTouchDrag keeps a thumb drag from becoming content panning.
func (s *ScrollWidget) CaptureTouchDrag() bool { return s.dragging }

// Reveal implements Revealer: the window moves the least it must for
// target, in window coordinates, to be inside it. Focus moved by the
// keyboard calls it.
func (s *ScrollWidget) Reveal(target Rect) {
	// Only a target inside the content, which may lie beyond the window
	// along the axis but not across it.
	content := Rct(s.rect.Origin, s.childSize)
	if s.horizontal {
		content.Origin.X -= s.position()
		content.Size.H = s.rect.Size.H
	} else {
		content.Origin.Y -= s.position()
		content.Size.W = s.rect.Size.W
	}
	if !content.Contains(target.Origin) {
		return
	}
	lo, hi := target.Origin.Y-s.rect.Origin.Y, target.Origin.Y+target.Size.H-s.rect.Origin.Y
	extent := s.rect.Size.H
	if s.horizontal {
		lo, hi = target.Origin.X-s.rect.Origin.X, target.Origin.X+target.Size.W-s.rect.Origin.X
		extent = s.rect.Size.W
	}
	switch {
	case lo < 0:
		s.phys.animating = false // the reveal wins over an eased scroll
		s.scrollTo(s.position() + lo)
	case hi > extent:
		s.phys.animating = false
		s.scrollTo(s.position() + hi - extent)
	}
}

// HandlePointer implements PointerHandler: wheel movement along the scroll
// axis moves the window.
func (s *ScrollWidget) HandlePointer(ev PointerEvent) bool {
	axis := pick(s.horizontal, ev.Pos.X, ev.Pos.Y)
	switch ev.Kind {
	case PointerDown:
		if ev.Button != MouseButtonLeft || !s.thumbHit.Contains(ev.Pos) || s.extent(s.rect.Size) <= s.thumbLength() {
			return false
		}
		s.dragging, s.dragStart, s.dragOffset = true, axis, s.position()
		return true
	case PointerDrag:
		if !s.dragging {
			return false
		}
		travel := s.extent(s.rect.Size) - s.thumbLength()
		s.phys.input(s)
		if travel > 0 {
			s.scrollTo(s.dragOffset + (axis-s.dragStart)*s.maxOffset()/travel)
		}
		return true
	case PointerUp:
		if !s.dragging || ev.Button != MouseButtonLeft {
			return false
		}
		s.dragging = false
		return true
	case PointerMove, PointerEnter:
		return s.thumbHit.Contains(ev.Pos)
	case PointerTap:
		return s.thumbHit.Contains(ev.Pos)
	}

	if ev.Kind != PointerScroll {
		return false
	}
	delta := pick(s.horizontal, ev.Scroll.X, ev.Scroll.Y)
	if delta == 0 || s.maxOffset() == 0 {
		return false
	}
	speed := s.speed
	if ev.ScrollPixels {
		speed = 1
	}
	return s.wheel(delta*speed, ev.ScrollMomentum)
}

// Overscroll says what a Scroll does when a touch pan, a fling or the wheel
// pushes past either end of its content.
type Overscroll uint8

const (
	// OverscrollClamp stops at the end. It is the default.
	OverscrollClamp Overscroll = iota
	// OverscrollBounce lets the content follow past the end with growing
	// resistance and springs it back once the input stops, as iOS does.
	OverscrollBounce
	// OverscrollStretch stretches the content along the axis from the end
	// it is pulled at, instead of moving it, and relaxes it the same way.
	OverscrollStretch
)

const (
	// scrollSettleDelay is how long input must pause before an overscroll
	// springs back and a snapping scroll settles. ggfx reports neither a
	// lifted finger on a trackpad nor the end of a wheel gesture, so a
	// pause is the signal.
	scrollSettleDelay = 120 * time.Millisecond
	scrollSpringBack  = 350 * time.Millisecond
	scrollAnimation   = 300 * time.Millisecond
)

// scrollPhysics is the part of a ScrollWidget that moves on its own:
// overscroll, snapping and the animated scrolls a ScrollController asks for.
type scrollPhysics struct {
	mode   Overscroll
	snap   float64 // snap interval; 0 snaps nowhere
	paging bool    // snap to the viewport's extent
	ctrl   *ScrollController

	scrollMotionState // carried across a rebuild by adopt

	applied *scrollRequest // what attach applied in this layout, until paint
	painted bool           // painted, so adopted what it replaces: requests apply at once
}

// scrollMotionState is what a scroll is doing on its own, which a rebuilt
// one carries on with.
type scrollMotionState struct {
	over       float64 // how far the content is pulled past an end: offset minus where it shows
	overMotion Motion
	relaxing   bool // over is springing back
	lastInput  time.Time
	from       float64 // the offset when the gesture began
	dir        float64 // the sign of the last movement
	settle     bool    // waiting for input to pause
	anim       Motion
	animating  bool
}

func (p *scrollPhysics) adopt(q *scrollPhysics) { p.scrollMotionState = q.scrollMotionState }

// Overscroll sets what happens at either end: OverscrollClamp, the
// default, stops there; OverscrollBounce and OverscrollStretch follow the
// input past it and spring back when it stops.
func (s *ScrollWidget) Overscroll(o Overscroll) *ScrollWidget {
	defer property.Watch(&s.props, &s.phys.mode)()
	s.phys.mode = o
	return s
}

// Snap settles the offset on a multiple of v, or on the end, once scrolling
// stops: rows of height v always line up with the top of the window. Zero
// turns snapping off.
func (s *ScrollWidget) Snap(v float64) *ScrollWidget {
	defer property.Watch(&s.props, &s.phys.snap)()
	s.phys.snap = max(v, 0)
	return s
}

// Paging snaps to whole viewports, as a pager does: a gesture that moves
// more than a sixth of a page settles on the next page in its direction,
// a shorter one returns to the page it started from.
func (s *ScrollWidget) Paging() *ScrollWidget {
	defer property.Watch(&s.props, &s.phys.paging)()
	s.phys.paging = true
	return s
}

// input notes a scroll gesture: what settling waits on and where it began.
// It stops a programmatic scroll and a spring-back under way.
func (p *scrollPhysics) input(s *ScrollWidget) {
	now := FrameTime()
	if p.lastInput.IsZero() || now.Sub(p.lastInput) >= scrollSettleDelay {
		p.from = s.position()
	}
	p.lastInput = now
	p.animating, p.relaxing = false, false
	p.settle = p.mode != OverscrollClamp || p.snap > 0 || p.paging
	if p.settle {
		WakeAt(now.Add(scrollSettleDelay))
	}
}

// wheel moves the content by d along the axis, as the wheel, a trackpad or
// a touch pan asks, and reports whether the event was used.
//
// A scroll placed in the window, as its last paint found it, moves in
// reactive.Contained: what the move changes shows in its viewport alone,
// for the frame to paint that alone.
func (s *ScrollWidget) wheel(d float64, momentum bool) bool {
	if !s.placed {
		return s.wheelBy(d, momentum)
	}
	var used bool
	before, over := s.position(), s.phys.over
	reactive.Contained(func() { used = s.wheelBy(d, momentum) })
	if s.position() != before || s.phys.over != over {
		s.containedMove = true
		noteContained(s.win)
	}
	return used
}

func (s *ScrollWidget) wheelBy(d float64, momentum bool) bool {
	p := &s.phys
	p.input(s)
	before, over := s.position(), p.over
	if p.mode == OverscrollClamp {
		s.scrollTo(before - d)
	} else {
		raw := s.rubber(before-p.over, -d, momentum)
		s.scrollTo(raw)
		p.over = s.position() - raw
	}
	if moved := s.position() - before; moved != 0 {
		p.dir = math.Copysign(1, moved)
	} else if d != 0 {
		p.dir = math.Copysign(1, -d)
	}
	if p.over != over {
		activeWorld().frame.animate()
	}
	if momentum && math.Abs(p.over) >= s.extent(s.viewport)/8 {
		return false // a fling that reached the end stops there and bounces back
	}
	return !momentum || s.position() != before || p.over != over
}

// rubber moves raw, the offset the content shows at, by d, with movement
// past either end resisted the further it goes; see kinetics.Rubber.
func (s *ScrollWidget) rubber(raw, d float64, momentum bool) float64 {
	return kinetics.Rubber(raw, d, 0, s.maxOffset(), s.extent(s.viewport), momentum)
}

// step advances a programmatic scroll; Layout calls it before laying out
// the child for the new window.
func (p *scrollPhysics) step(s *ScrollWidget) {
	if !p.animating {
		return
	}
	v := p.anim.Value(FrameTime())
	if v == p.anim.to {
		p.animating = false
	}
	s.scrollTo(v)
}

// attach applies what the controller asked for before the first layout.
func (p *scrollPhysics) attach(s *ScrollWidget) {
	c := p.ctrl
	if c == nil {
		return
	}
	c.scroll = s
	if r := c.pending; r != nil {
		c.pending = nil
		s.animateTo(r.offset, r.animate)
		// A rebuilt scroll adopts the one it replaces only at paint, which
		// would overwrite what this did; Adopt applies it again.
		p.applied = r
	}
}

// reapply redoes, after Adopt, a request attach applied before it.
func (p *scrollPhysics) reapply(s *ScrollWidget) {
	if r := p.applied; r != nil {
		p.applied = nil
		s.animateTo(r.offset, r.animate)
	}
}

// paint keeps a programmatic scroll and a spring-back moving, and settles
// a scroll once its input pauses.
func (p *scrollPhysics) paint(s *ScrollWidget) {
	now := FrameTime()
	p.applied, p.painted = nil, true // Adopt, if any, has run
	if p.animating {
		s.cache.invalidate() // the next frame lays out the child for the next step
	}
	if p.relaxing {
		p.over = p.overMotion.Value(now)
		p.relaxing = p.over != 0
	}
	if !p.settle || s.dragging {
		return
	}
	if wait := p.lastInput.Add(scrollSettleDelay); now.Before(wait) {
		WakeAt(wait)
		return
	}
	p.settle = false
	if p.over != 0 {
		p.overMotion = Motion{}
		p.overMotion.MoveTo(p.over, now, 0)
		p.overMotion.MoveTo(0, now, s.barEnv.Motion(scrollSpringBack))
		p.over = p.overMotion.Value(now)
		p.relaxing = p.over != 0
	}
	if t, ok := s.snapTarget(); ok && t != s.position() {
		s.animateTo(t, true)
	}
}

// snapTarget returns where a snapping scroll settles from where it is.
func (s *ScrollWidget) snapTarget() (float64, bool) {
	p := &s.phys
	step := p.snap
	if p.paging {
		step = s.extent(s.viewport)
	}
	if step <= 0 || !s.laidOut {
		return 0, false
	}
	return kinetics.SnapTarget(s.position(), s.maxOffset(), step, p.from, p.dir, p.paging), true
}

// animateTo moves the window to v, eased over the scroll animation unless
// animate is false or motion is reduced.
func (s *ScrollWidget) animateTo(v float64, animate bool) {
	p := &s.phys
	if s.laidOut {
		v = clamp(v, 0, s.maxOffset())
	}
	d := s.barEnv.Motion(scrollAnimation)
	if !animate || d <= 0 || !s.laidOut || v == s.position() {
		p.animating = false
		s.jumpTo(v)
		return
	}
	now := FrameTime()
	p.anim = Motion{}
	p.anim.MoveTo(s.position(), now, 0)
	p.anim.MoveTo(v, now, d)
	p.animating = true
	s.cache.invalidate()
}

// jumpTo moves the window to v at once. Before the first layout the extent
// is unknown, so v is kept for Layout to clamp.
func (s *ScrollWidget) jumpTo(v float64) {
	if s.laidOut {
		s.scrollTo(v)
		return
	}
	if s.bound != nil {
		s.bound.Set(v)
	} else {
		s.offset = v
	}
	reactive.RequestLayout()
}

// paintContent paints the child at the window's offset, moved or stretched
// by any overscroll.
func (s *ScrollWidget) paintContent(dst *Canvas, r Rect) {
	p := &s.phys
	off := s.visualOffset()
	origin := r.Origin
	if s.horizontal {
		origin.X -= off
	} else {
		origin.Y -= off
	}
	content := Rct(origin, s.childSize)
	extent := s.extent(r.Size)
	if p.mode != OverscrollStretch || p.over == 0 || extent <= 0 || dst == nil || dst.Image == nil {
		dst.Paint(s.child, content)
		return
	}
	// Stretched from the end being pulled: the start when the content is
	// pulled down (or right), the far end when it is pulled up.
	k := kinetics.Stretch(p.over, extent)
	about := r.Origin
	if p.over < 0 {
		about = about.Add(pick(s.horizontal, Pt(r.Size.W, 0), Pt(0, r.Size.H)))
	}
	sx, sy := pick(s.horizontal, k, 1.0), pick(s.horizontal, 1.0, k)
	dst.Transform(Affine{}.Scale(sx, sy).About(about), func(dst *Canvas) { dst.Paint(s.child, content) })
}

// visualOffset is the offset the content is drawn at, past either end
// while it is pulled there.
func (s *ScrollWidget) visualOffset() float64 {
	if s.phys.mode == OverscrollStretch {
		return s.position()
	}
	return s.position() - s.phys.over
}

// scrollPortKey carries the nearest Scroll to its subtree, so a sticky
// header can find the window's edge and a list can answer ScrollToKey.
var (
	scrollPortKey  = NewEnvKey[*ScrollWidget]("scroll port")
	scrollPortXKey = NewEnvKey[*ScrollWidget]("scroll port x")
	scrollPortYKey = NewEnvKey[*ScrollWidget]("scroll port y")
)

// scrollPortAlong returns the nearest enclosing Scroll along the x axis,
// for horizontal, or the y axis.
func scrollPortAlong(env Env, horizontal bool) *ScrollWidget {
	s, _ := env.Get(pick(horizontal, scrollPortXKey, scrollPortYKey))
	return s
}

// scrollPortOf returns the nearest enclosing Scroll in env, if any.
func scrollPortOf(env Env) *ScrollWidget {
	s, _ := env.Get(scrollPortKey)
	return s
}

// ScrollController moves a Scroll from code, eased the way the theme's
// motion is or at once, and scrolls a keyed row into view. The zero value
// is ready; attach it with Scroll.Controller and call it from handlers,
// effects or posted work. A request made before the scroll is first laid
// out is kept until it is.
//
//	var sc ggui.ScrollController
//	ggui.Scroll(list).Controller(&sc)
//	// in a handler:
//	sc.ScrollTo(0)
//	sc.ScrollToKey(todo.ID)
//
// BindOffset still reports and sets the offset; the controller adds the
// animation and the lookup by key.
type ScrollController struct {
	scroll  *ScrollWidget
	pending *scrollRequest
	key     any
	keyed   bool // key is waiting for a list to place it
	animate bool // whether the key's scroll is animated
}

type scrollRequest struct {
	offset  float64
	animate bool
}

// Controller attaches c, which then drives this scroll and every scroll
// rebuilt in its place. Configure it before mount.
func (s *ScrollWidget) Controller(c *ScrollController) *ScrollWidget {
	s.phys.ctrl = c
	if c != nil {
		c.scroll = s
	}
	return s
}

// Offset returns the scroll's offset, or zero while it is not attached.
func (c *ScrollController) Offset() float64 {
	if c.scroll == nil {
		return 0
	}
	return c.scroll.position()
}

// MaxOffset returns the largest offset the content allows, as of the last
// layout, or zero before one.
func (c *ScrollController) MaxOffset() float64 {
	if c.scroll == nil || !c.scroll.laidOut {
		return 0
	}
	return c.scroll.maxOffset()
}

// Animating reports whether a scroll the controller started is under way.
func (c *ScrollController) Animating() bool { return c.scroll != nil && c.scroll.phys.animating }

// JumpTo moves the window to offset v at once, clamped to the content.
func (c *ScrollController) JumpTo(v float64) { c.request(v, false) }

// ScrollTo eases the window to offset v, clamped to the content: 0 is the
// start and math.Inf(1) the end. Wheel or touch input takes over from it.
// Under reduced motion it jumps.
func (c *ScrollController) ScrollTo(v float64) { c.request(v, true) }

// ScrollBy eases the window by d from where the scroll is heading, so
// repeated calls add up.
func (c *ScrollController) ScrollBy(d float64) {
	base := c.Offset()
	if r := c.pending; r != nil {
		base = r.offset
	} else if c.scroll != nil && c.scroll.phys.animating {
		base = c.scroll.phys.anim.to
	}
	c.request(base+d, true)
}

// ScrollToKey eases the window until the row of an EachKeyed list keyed k
// sits at its start, or as near as the content allows. The list inside the
// scroll places the row when it is next painted, so a row a virtualized
// list has not built yet is found too. k must have the list's key type.
func (c *ScrollController) ScrollToKey(k any) {
	c.key, c.keyed, c.animate = k, true, true
	c.pending = nil
	reactive.RequestLayout()
}

// JumpToKey is ScrollToKey without the animation.
func (c *ScrollController) JumpToKey(k any) {
	c.ScrollToKey(k)
	c.animate = false
}

func (c *ScrollController) request(v float64, animate bool) {
	c.keyed, c.key = false, nil
	if c.scroll == nil || !c.scroll.phys.painted {
		c.pending = &scrollRequest{offset: v, animate: animate}
		reactive.RequestLayout()
		return
	}
	c.pending = nil
	c.scroll.animateTo(v, animate)
}

// placeKey is how a list inside s answers a ScrollToKey: match reports
// whether the waiting key is one of its rows and, if so, where that row
// starts in window coordinates along the axis.
func (s *ScrollWidget) placeKey(match func(k any) (float64, bool)) {
	if !s.keyPending() {
		return
	}
	c := s.phys.ctrl
	at, ok := match(c.key)
	if !ok {
		return
	}
	c.keyed, c.key = false, nil
	start := pick(s.horizontal, s.rect.Origin.X, s.rect.Origin.Y)
	// at is where the row is drawn now; the content moved by the visible
	// offset to put it there.
	s.animateTo(s.visualOffset()+at-start, c.animate)
}

// keyPending reports whether a ScrollToKey on s waits for a list to place
// its key, which a list checks before it builds the match.
func (s *ScrollWidget) keyPending() bool {
	c := s.phys.ctrl
	return c != nil && c.keyed && c.scroll == s
}
