package ui

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/icons"
	"github.com/ironpark/ggui/internal/property"
)

// CarouselItemWidget is one slide. Basis is a fraction of the viewport.
type CarouselItemWidget struct {
	props      property.Owner
	child      ggui.Widget
	basis      float64
	responsive func(ggui.Size) float64
}

func CarouselItem(child ggui.Widget) *CarouselItemWidget {
	return &CarouselItemWidget{child: child, basis: 1}
}
func (w *CarouselItemWidget) Basis(f float64) *CarouselItemWidget {
	defer property.Watch(&w.props, &w.basis)()
	w.basis = carouselBasis(f)
	return w
}

// BasisWhen computes a responsive fraction from the available viewport size.
func (w *CarouselItemWidget) BasisWhen(fn func(ggui.Size) float64) *CarouselItemWidget {
	w.responsive = fn
	return w
}
func (w *CarouselItemWidget) Layout(c ggui.Constraints, e ggui.Env) ggui.Size {
	defer w.props.Layout()()
	return w.child.Layout(c, e)
}
func (w *CarouselItemWidget) Paint(d *ggui.Canvas, r ggui.Rect) { d.Paint(w.child, r) }
func carouselBasis(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) || f <= 0 {
		return 1
	}
	return min(f, 1)
}

// CarouselContentWidget groups slides for composition with Carousel.
type CarouselContentWidget struct {
	items []ggui.Widget
	row   *ggui.RowWidget
}

func CarouselContent(items ...ggui.Widget) *CarouselContentWidget {
	return &CarouselContentWidget{items: append([]ggui.Widget(nil), items...), row: ggui.Row(items...)}
}
func (w *CarouselContentWidget) Layout(c ggui.Constraints, e ggui.Env) ggui.Size {
	return w.row.Layout(c, e)
}
func (w *CarouselContentWidget) Paint(d *ggui.Canvas, r ggui.Rect) {
	d.Paint(w.row, r)
}

// CarouselWidget is a clipped, draggable strip of arbitrary native widgets.
// Its binding selects a scroll snap (not necessarily an individual slide when
// several slides fit). Only visible slides paint or register input regions.
type CarouselWidget struct {
	props property.Owner
	ggui.Interactive
	selected                                   ggui.Binding[int]
	items                                      []*CarouselItemWidget
	prev, next                                 *CarouselNavigationWidget
	vertical, rtl, loop, controls, dragEnabled bool
	height, gap, align                         float64
	duration                                   time.Duration
	autoplay                                   time.Duration
	stopped, stopOnInteraction                 bool
	onChange                                   func(int)
	env                                        ggui.Env
	theme                                      ggui.Theme
	reduced, canLoop                           bool
	viewport                                   ggui.Rect
	starts, extents, snaps                     []float64
	total, view, offset, from, target          float64
	began, lastAuto                            time.Time
	ready, dragging                            bool
	dragStart, dragOffset, lastDrag, velocity  float64
	dragTime                                   time.Time
}

// Carousel binds the selected snap to selected. Slides default to full width;
// wrap them with CarouselItem to set a smaller Basis. Navigation buttons reserve
// 48 logical pixels on either side, so controls never overflow their parent.
func Carousel(selected ggui.Binding[int], items ...ggui.Widget) *CarouselWidget {
	c := &CarouselWidget{selected: selected, height: 240, gap: 16, align: .5, controls: true, dragEnabled: true, duration: 1800 * time.Millisecond, stopOnInteraction: true}
	c.Role = ggui.RoleGroup
	c.SetName("Carousel")
	c.AutoKey()
	var add func(ggui.Widget)
	add = func(w ggui.Widget) {
		if w == nil {
			return
		}
		if group, ok := w.(*CarouselContentWidget); ok {
			for _, it := range group.items {
				add(it)
			}
			return
		}
		if it, ok := w.(*CarouselItemWidget); ok {
			c.items = append(c.items, it)
		} else {
			c.items = append(c.items, CarouselItem(w))
		}
	}
	for _, w := range items {
		add(w)
	}
	c.prev, c.next = CarouselPrevious(c), CarouselNext(c)
	return c
}
func (c *CarouselWidget) Name(s string) *CarouselWidget { c.SetName(s); return c }
func (c *CarouselWidget) Height(h float64) *CarouselWidget {
	defer property.Watch(&c.props, &c.height)()
	c.height = max(0, h)
	return c
}
func (c *CarouselWidget) Vertical() *CarouselWidget { c.vertical = true; return c }
func (c *CarouselWidget) RTL(v bool) *CarouselWidget {
	defer property.Watch(&c.props, &c.rtl)()
	c.rtl = v
	return c
}
func (c *CarouselWidget) Loop(v bool) *CarouselWidget {
	defer property.Watch(&c.props, &c.loop)()
	c.loop = v
	return c
}
func (c *CarouselWidget) Controls(v bool) *CarouselWidget {
	defer property.Watch(&c.props, &c.controls)()
	c.controls = v
	return c
}
func (c *CarouselWidget) Draggable(v bool) *CarouselWidget {
	defer property.Watch(&c.props, &c.dragEnabled)()
	c.dragEnabled = v
	return c
}
func (c *CarouselWidget) Gap(g float64) *CarouselWidget {
	defer property.Watch(&c.props, &c.gap)()
	c.gap = max(0, g)
	return c
}

// Align sets snap alignment: 0 start, .5 center (default), 1 end.
func (c *CarouselWidget) Align(a float64) *CarouselWidget {
	defer property.Watch(&c.props, &c.align)()
	c.align = clamp(a, 0, 1)
	return c
}
func (c *CarouselWidget) Animation(d time.Duration) *CarouselWidget {
	defer property.Watch(&c.props, &c.duration)()
	c.duration = max(0, d)
	return c
}
func (c *CarouselWidget) Disabled(v bool) *CarouselWidget { c.SetInert(v); return c }
func (c *CarouselWidget) BindDisabled(r ggui.Readable[bool]) *CarouselWidget {
	c.BindInert(r)
	return c
}
func (c *CarouselWidget) OnChange(fn func(int)) *CarouselWidget { c.onChange = fn; return c }

// Autoplay advances at interval, pauses while hovered/focused, and respects
// reduced motion. User interaction stops playback until Play is called.
func (c *CarouselWidget) Autoplay(interval time.Duration) *CarouselWidget {
	defer property.Watch(&c.props, &c.autoplay)()
	c.autoplay = max(0, interval)
	return c
}
func (c *CarouselWidget) StopOnInteraction(v bool) *CarouselWidget {
	defer property.
		Watch(
			&c.props, &c.stopOnInteraction,
		)()

	c.stopOnInteraction = v
	return c
}
func (c *CarouselWidget) Play()          { c.stopped = false; c.lastAuto = ggui.Now() }
func (c *CarouselWidget) Pause()         { c.stopped = true }
func (c *CarouselWidget) SnapCount() int { return len(c.snaps) }
func (c *CarouselWidget) Selected() int {
	if len(c.snaps) == 0 {
		return -1
	}
	return max(0, min(ggui.Untrack(c.selected.Get), len(c.snaps)-1))
}
func (c *CarouselWidget) CanPrevious() bool {
	return !c.IsInert() && len(c.snaps) > 1 && (c.canLoop || c.Selected() > 0)
}
func (c *CarouselWidget) CanNext() bool {
	return !c.IsInert() && len(c.snaps) > 1 && (c.canLoop || c.Selected() < len(c.snaps)-1)
}
func (c *CarouselWidget) Previous()          { c.scroll(c.Selected()-1, -1, true) }
func (c *CarouselWidget) Next()              { c.scroll(c.Selected()+1, 1, true) }
func (c *CarouselWidget) ScrollTo(index int) { c.scroll(index, 0, true) }
func (c *CarouselWidget) axis(p ggui.Point) float64 {
	if c.vertical {
		return p.Y
	}
	if c.rtl {
		return -p.X
	}
	return p.X
}
func (c *CarouselWidget) interact() {
	if c.stopOnInteraction {
		c.stopped = true
	}
	c.lastAuto = ggui.Now()
}
func (c *CarouselWidget) scroll(i, dir int, user bool) {
	if c.IsInert() || len(c.snaps) == 0 {
		return
	}
	if user {
		c.interact()
	}
	n := len(c.snaps)
	if c.canLoop {
		i = (i%n + n) % n
	} else {
		i = max(0, min(i, n-1))
	}
	old := c.Selected()
	c.animate(c.snapTarget(i, dir))
	if old != i {
		setChanged(c.selected, i, c.onChange)
	}
}
func (c *CarouselWidget) snapTarget(i, dir int) float64 {
	v := c.snaps[i]
	if !c.canLoop {
		return v
	}
	v += math.Round((c.offset-v)/c.total) * c.total
	if dir > 0 && v < c.offset-.01 {
		v += c.total
	}
	if dir < 0 && v > c.offset+.01 {
		v -= c.total
	}
	return v
}
func (c *CarouselWidget) animate(target float64) {
	c.offset = c.position()
	c.from, c.target, c.began = c.offset, target, ggui.Now()
	if c.reduced || c.duration == 0 {
		c.offset = target
	}
}
func (c *CarouselWidget) position() float64 {
	if c.dragging {
		return c.offset
	}
	if c.reduced || c.duration <= 0 || c.began.IsZero() {
		return c.target
	}
	t := clamp(float64(ggui.Now().Sub(c.began))/float64(c.duration), 0, 1)
	// Closed form of Embla's 60 Hz spring (duration 25, friction .68).
	// This retains its glide without iterating physics steps or allocating.
	const friction = .68
	const trace = 1 + friction - friction/25
	root := math.Sqrt(trace*trace - 4*friction)
	r1, r2 := (trace+root)/2, (trace-root)/2
	a := (1 - friction/25 - r2) / (r1 - r2)
	remaining := a*math.Pow(r1, 108*t) + (1-a)*math.Pow(r2, 108*t)
	if t >= 1 || math.Abs((c.target-c.from)*remaining) < .001 {
		return c.target
	}
	return c.target + (c.from-c.target)*remaining
}
func (c *CarouselWidget) Layout(con ggui.Constraints, e ggui.Env) ggui.Size {
	defer c.props.Layout()()
	c.Sync()
	c.env, c.theme, c.reduced = e, e.Theme(), e.ReducedMotion()
	size := con.Constrain(ggui.Sz(bounded(con.MaxW, 416), c.height))
	gutter := 0.
	if c.controls {
		gutter = 48
	}
	if c.vertical {
		gutter = min(gutter, size.H/2)
	} else {
		gutter = min(gutter, size.W/2)
	}
	viewSize := size
	origin := ggui.Point{}
	if c.vertical {
		viewSize.H = max(0, size.H-2*gutter)
		origin.Y = gutter
	} else {
		viewSize.W = max(0, size.W-2*gutter)
		origin.X = gutter
	}
	oldView := c.view
	c.viewport = ggui.Rct(origin, viewSize)
	c.view = viewSize.W
	if c.vertical {
		c.view = viewSize.H
	}
	c.starts = c.starts[:0]
	c.extents = c.extents[:0]
	c.snaps = c.snaps[:0]
	c.total = 0
	maxExtent := 0.
	for _, it := range c.items {
		basis := it.basis
		if it.responsive != nil {
			basis = carouselBasis(it.responsive(viewSize))
		}
		extent := max(0, (c.view+c.gap)*basis-c.gap)
		c.starts = append(c.starts, c.total)
		c.extents = append(c.extents, extent)
		maxExtent = max(maxExtent, extent)
		c.total += extent + c.gap
		s := viewSize
		if c.vertical {
			s.H = extent
		} else {
			s.W = extent
		}
		it.Layout(ggui.Tight(s), e)
	}
	c.canLoop = c.loop && len(c.items) > 1 && c.total-maxExtent >= c.view
	limit := max(0, c.total-c.gap-c.view)
	for i, start := range c.starts {
		snap := start - (c.view-c.extents[i])*c.align
		if !c.canLoop {
			snap = clamp(snap, 0, limit)
		}
		if len(c.snaps) == 0 || math.Abs(snap-c.snaps[len(c.snaps)-1]) > .01 {
			c.snaps = append(c.snaps, snap)
		}
	}
	if !c.ready || oldView != c.view {
		if i := c.Selected(); i >= 0 {
			c.offset = c.snaps[i]
			c.target = c.offset
			c.from = c.offset
		}
		c.ready = true
		c.dragging = false
	}
	c.prev.Layout(ggui.Tight(ggui.Sz(28, 28)), e)
	c.next.Layout(ggui.Tight(ggui.Sz(28, 28)), e)
	return size
}
func (c *CarouselWidget) Paint(d *ggui.Canvas, r ggui.Rect) {
	c.Sync()
	v := c.viewport
	v.Origin = v.Origin.Add(r.Origin)
	if i := c.Selected(); i >= 0 && !c.dragging {
		want := c.snapTarget(i, 0)
		if math.Abs(want-c.target) > .01 {
			c.animate(want)
		}
	}
	c.offset = c.position()
	now := ggui.Now()
	p, has := d.Pointer()
	hover := has && r.Contains(p)
	paused := c.IsInert() || c.reduced || c.dragging || hover || d.FocusWithin(r) || c.Focused || c.prev.Focused || c.next.Focused
	if c.autoplay > 0 && !c.IsInert() {
		d.ObserveInput(r, c.interact)
	}
	if c.lastAuto.IsZero() || paused {
		c.lastAuto = now
	}
	if c.autoplay > 0 && !c.stopped && !paused && now.Sub(c.lastAuto) >= c.autoplay {
		c.lastAuto = now
		i := c.Selected() + 1
		if i >= len(c.snaps) {
			i = 0
		}
		c.scroll(i, 1, false)
	}
	d.DescribeNode(r, c, func(d *ggui.Canvas) {
		if !c.IsInert() {
			d.HitPointer(v, c)
			d.HitKey(v, c)
		}
		clip := d.Clip(v)
		// Locate the visible range once. Looping uses the same ordered lookup;
		// no cloned widgets or scan over offscreen slides is necessary.
		local := c.offset
		if c.canLoop {
			local = math.Mod(local, c.total)
			if local < 0 {
				local += c.total
			}
		}
		first := max(0, sort.SearchFloat64s(c.starts, local)-1)
		for step := 0; step < len(c.items); step++ {
			i := first + step
			cycle := 0.0
			if i >= len(c.items) {
				if !c.canLoop {
					break
				}
				i -= len(c.items)
				cycle = c.total
			}
			pos := c.starts[i] + cycle - local
			if pos >= c.view {
				break
			}
			if pos+c.extents[i] <= 0 {
				continue
			}
			at := v
			if c.vertical {
				at.Origin.Y += pos
				at.Size.H = c.extents[i]
			} else {
				if c.rtl {
					pos = c.view - pos - c.extents[i]
				}
				at.Origin.X += pos
				at.Size.W = c.extents[i]
			}
			clip.Node(at, ggui.Node{Role: ggui.RoleGroup, Name: fmt.Sprintf("Slide %d of %d", i+1, len(c.items))}, func(dst *ggui.Canvas) { dst.Paint(c.items[i], at) })
		}
		c.FocusRing(d, v, c.theme.Radius, c.theme.Ring)
		if c.controls && ((!c.vertical && r.Size.W >= 56) || (c.vertical && r.Size.H >= 56)) {
			a, b := ggui.Rct(ggui.Pt(r.Origin.X, r.Center().Y-14), ggui.Sz(28, 28)), ggui.Rct(ggui.Pt(r.Origin.X+r.Size.W-28, r.Center().Y-14), ggui.Sz(28, 28))
			if c.vertical {
				a.Origin = ggui.Pt(r.Center().X-14, r.Origin.Y)
				b.Origin = ggui.Pt(r.Center().X-14, r.Origin.Y+r.Size.H-28)
			} else if c.rtl {
				a, b = b, a
			}
			d.Paint(c.prev, a)
			d.Paint(c.next, b)
		}
	})
}
func (c *CarouselWidget) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleGroup, Name: c.SemanticName(), Value: fmt.Sprintf("Slide %d of %d", c.Selected()+1, len(c.snaps)), Disabled: c.IsInert(), Min: 0, Max: float64(max(0, len(c.snaps)-1)), Now: float64(max(0, c.Selected())), Actions: ggui.ActionFocus | ggui.ActionIncrement | ggui.ActionDecrement | ggui.ActionSetValue}
}
func (c *CarouselWidget) Act(a ggui.Action) bool {
	if c.IsInert() {
		return false
	}
	switch a.Kind {
	case ggui.ActionIncrement:
		c.Next()
	case ggui.ActionDecrement:
		c.Previous()
	case ggui.ActionSetValue:
		if math.IsNaN(a.Num) || math.IsInf(a.Num, 0) {
			return false
		}
		c.ScrollTo(int(a.Num))
	default:
		return false
	}
	return true
}
func (c *CarouselWidget) ConsumesKey(e ggui.KeyEvent) bool {
	if e.Kind != ggui.KeyPress {
		return false
	}
	if e.Key == ggui.KeyHome || e.Key == ggui.KeyEnd {
		return true
	}
	if c.vertical {
		return e.Key == ggui.KeyArrowUp || e.Key == ggui.KeyArrowDown
	}
	return e.Key == ggui.KeyArrowLeft || e.Key == ggui.KeyArrowRight
}
func (c *CarouselWidget) HandleKey(e ggui.KeyEvent) {
	c.Keyboard(e, nil)
	if c.IsInert() || !c.ConsumesKey(e) {
		return
	}
	switch e.Key {
	case ggui.KeyHome:
		c.ScrollTo(0)
	case ggui.KeyEnd:
		c.ScrollTo(len(c.snaps) - 1)
	default:
		prev := e.Key == ggui.KeyArrowLeft || e.Key == ggui.KeyArrowUp
		if c.rtl && !c.vertical {
			prev = !prev
		}
		if prev {
			c.Previous()
		} else {
			c.Next()
		}
	}
}
func (c *CarouselWidget) CaptureTouchDrag() bool {
	return !c.IsInert() && c.dragEnabled && len(c.snaps) > 1
}
func (c *CarouselWidget) HandlePointer(e ggui.PointerEvent) bool {
	if c.IsInert() {
		return false
	}
	switch e.Kind {
	case ggui.PointerDown:
		if e.Button != ggui.MouseButtonLeft || !c.dragEnabled || len(c.snaps) < 2 {
			return false
		}
		c.interact()
		c.offset = c.position()
		c.dragging = true
		c.dragStart = c.axis(e.Pos)
		c.dragOffset = c.offset
		c.lastDrag = c.dragStart
		c.dragTime = ggui.Now()
		c.velocity = 0
	case ggui.PointerDrag:
		if !c.dragging {
			return false
		}
		p := c.axis(e.Pos)
		dt := ggui.Now().Sub(c.dragTime).Seconds()
		if dt > 0 && p != c.lastDrag {
			c.velocity = (c.lastDrag - p) / dt
			c.lastDrag = p
			c.dragTime = ggui.Now()
		}
		v := c.dragOffset + c.dragStart - p
		if !c.canLoop {
			lo, hi := c.snaps[0], c.snaps[len(c.snaps)-1]
			if v < lo {
				v = lo + (v-lo)*.25
			}
			if v > hi {
				v = hi + (v-hi)*.25
			}
		}
		c.offset = v
		return true
	case ggui.PointerUp:
		if c.dragging {
			projected := c.offset
			if ggui.Now().Sub(c.dragTime) < 100*time.Millisecond {
				projected += clamp(c.velocity*.12, -c.view, c.view)
			}
			best, dist := 0, math.Inf(1)
			for i := range c.snaps {
				v := c.snapTarget(i, 0)
				if delta := math.Abs(v - projected); delta < dist {
					dist = delta
					best = i
				}
			}
			c.dragging = false
			c.target = c.offset
			c.began = time.Time{}
			c.scroll(best, 0, false)
		}
	}
	return c.Pointer(e, nil)
}
func (c *CarouselWidget) Adopt(prev any) {
	c.Interactive.Adopt(prev)
	if p, ok := prev.(*CarouselWidget); ok {
		c.offset, c.from, c.target, c.began = p.offset, p.from, p.target, p.began
		c.ready = p.ready
		c.view = p.view
		c.lastAuto, c.stopped = p.lastAuto, p.stopped
		c.dragging, c.dragStart, c.dragOffset, c.lastDrag, c.velocity, c.dragTime = p.dragging, p.dragStart, p.dragOffset, p.lastDrag, p.velocity, p.dragTime
	}
}

// CarouselNavigationWidget is a standalone previous/next control. Use
// Controls(false) when placing these elsewhere in an application layout.
type CarouselNavigationWidget struct {
	ggui.Interactive
	carousel *CarouselWidget
	previous bool
	env      ggui.Env
}

func CarouselPrevious(c *CarouselWidget) *CarouselNavigationWidget {
	return carouselNavigation(c, true)
}
func CarouselNext(c *CarouselWidget) *CarouselNavigationWidget { return carouselNavigation(c, false) }
func carouselNavigation(c *CarouselWidget, prev bool) *CarouselNavigationWidget {
	b := &CarouselNavigationWidget{carousel: c, previous: prev}
	b.Role = ggui.RoleButton
	b.SetName(pick(prev, "Previous slide", "Next slide"))
	return b
}
func (b *CarouselNavigationWidget) Layout(c ggui.Constraints, e ggui.Env) ggui.Size {
	b.env = e
	return c.Constrain(ggui.Sz(28, 28))
}
func (b *CarouselNavigationWidget) enabled() bool {
	if b.previous {
		return b.carousel.CanPrevious()
	}
	return b.carousel.CanNext()
}
func (b *CarouselNavigationWidget) activate() {
	if !b.enabled() {
		return
	}
	if b.previous {
		b.carousel.Previous()
	} else {
		b.carousel.Next()
	}
}
func (b *CarouselNavigationWidget) Paint(d *ggui.Canvas, r ggui.Rect) {
	b.SetInert(!b.enabled())
	b.Hit(d, r, b, ggui.CursorShapePointer)
	t := b.env.Theme()
	fill, col, border := t.Bg, t.Fg, t.Border
	if b.Hovered && !b.IsInert() {
		fill = colorOr(t.Accent, t.Muted)
	}
	if b.IsInert() {
		col = fade(col, .5)
		border = fade(border, .5)
	}
	d.FillRoundRect(r, min(r.Size.W, r.Size.H)/2, fill)
	d.StrokeRoundRect(r, min(r.Size.W, r.Size.H)/2, 1, border)
	role := icons.ChevronRight
	if b.previous {
		role = icons.ChevronLeft
	}
	angle := 0.
	if b.carousel.vertical {
		angle = math.Pi / 2
	} else if b.carousel.rtl {
		angle = math.Pi
	}
	paintIcon(d, b.env, role, ggui.Rct(r.Center().Add(ggui.Pt(-8, -8)), ggui.Sz(16, 16)), col, angle)
	b.FocusRing(d, r, 14, t.Ring)
}
func (b *CarouselNavigationWidget) HandlePointer(e ggui.PointerEvent) bool {
	return b.Pointer(e, b.activate)
}
func (b *CarouselNavigationWidget) HandleKey(e ggui.KeyEvent) {
	b.Keyboard(e, b.activate)
	if b.carousel.ConsumesKey(e) {
		b.carousel.HandleKey(e)
	}
}
func (b *CarouselNavigationWidget) ConsumesKey(e ggui.KeyEvent) bool {
	return ggui.Activates(e) || b.carousel.ConsumesKey(e)
}
func (b *CarouselNavigationWidget) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleButton, Name: b.SemanticName(), Disabled: !b.enabled(), Actions: ggui.ActionPress | ggui.ActionFocus}
}
func (b *CarouselNavigationWidget) Act(a ggui.Action) bool {
	if a.Kind != ggui.ActionPress || !b.enabled() {
		return false
	}
	b.activate()
	return true
}
