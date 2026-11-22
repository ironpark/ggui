package ui

import (
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	"github.com/ironpark/ggui/ui/internal/controlkit"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

// selectAnchor is the popup's anchor: the field, painted by its Select.
type selectAnchor[T comparable] struct{ s *SelectWidget[T] }

func (a selectAnchor[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	s := a.s
	size := s.textSize
	if s.triggerContent != nil {
		size = s.triggerSize
	}
	s.box.Size(c.MaxW, s.pad.Inflate(size).H)
	return s.box.Layout(c, env)
}

func (a selectAnchor[T]) Paint(dst *ggui.Canvas, r ggui.Rect) { a.s.paintField(dst, r) }

// selectItem is one option row in the open list.
type selectItem[T comparable] struct {
	ggui.Interactive
	owner  *SelectWidget[T]
	index  int
	text   *widget.TextWidget
	active bool // highlighted from the keyboard

	pad      ggui.EdgeInsets
	textSize ggui.Size
	theme    *uitheme.Theme // the list's, which may be the menus' own
}

func (it *selectItem[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	it.Sync()
	t := uitheme.Of(env)
	it.theme = t
	// The label starts at the row's padding, and the room at the right is
	// the check mark's.
	it.pad = t.ItemPad
	it.pad.Right = menuMarkRoom(t)
	it.text.Color(pick(it.IsInert(), t.Disabled(t.PopoverFg), t.PopoverFg)) // Paint swaps in AccentFg under the highlight
	it.textSize = it.text.Layout(it.pad.Shrink(c).Loosen(), env)
	return c.Constrain(it.pad.Inflate(it.textSize))
}

// Describe implements ggui.Describer: whether this option is the value.
func (it *selectItem[T]) Describe() ggui.Node {
	return ggui.Node{
		Role:     ggui.RoleOption,
		Name:     it.SemanticName(),
		Selected: it.index == it.owner.selectedIndex,
		Disabled: it.IsInert(),
		Actions:  ggui.ActionSelect | ggui.ActionPress | ggui.ActionFocus,
	}
}

func (it *selectItem[T]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := it.theme
	dst.Describe(r, it)
	ink := t.Disabled(t.PopoverFg)
	if !it.IsInert() {
		dst.HitPointer(r, it)
		dst.HitCursor(r, ggui.CursorShapeDefault)
		lit := it.Hovered || it.active
		if lit {
			dst.FillRoundRect(r, t.RadiusSm, t.Accent)
		}
		ink = pick(lit, t.AccentFg, t.PopoverFg)
	}
	it.text.Color(ink)
	at := ggui.Pt(r.Origin.X+it.pad.Left, r.Origin.Y+(r.Size.H-it.textSize.H)/2)
	dst.Clip(r).Paint(it.text, ggui.Rct(at, it.textSize))
	if it.index == it.owner.selectedIndex {
		// The mark is the row's own color, not the theme's accent.
		center := ggui.Pt(r.Origin.X+r.Size.W-t.Space-t.IconSize/2, r.Origin.Y+r.Size.H/2)
		controlkit.PaintIconAt(dst, it.owner.env, controlkit.IconMark{Role: icons.Check, Center: center, Size: t.IconSize, Color: ink})
	}
}

// Act implements ggui.Actor: choosing this option.
func (it *selectItem[T]) Act(a ggui.Action) bool {
	if it.IsInert() || (a.Kind != ggui.ActionSelect && a.Kind != ggui.ActionPress) {
		return false
	}
	it.owner.choose(it.index)
	return true
}

func (it *selectItem[T]) HandlePointer(ev ggui.PointerEvent) bool {
	if it.IsInert() {
		return ev.Kind != ggui.PointerScroll
	}
	if ev.Kind == ggui.PointerEnter || ev.Kind == ggui.PointerMove {
		it.owner.highlight = it.index
	}
	return it.Pointer(ev, func() { it.owner.choose(it.index) })
}

// selectViewport measures the full list without changing its scroll position.
// Popup first measures with unlimited height to choose above/below placement;
// only the final painted viewport may clamp the scroll offset.
type selectViewport[T comparable] struct {
	owner    *SelectWidget[T]
	column   *menuColumn
	scroll   *widget.ScrollWidget
	offset   *ggui.StateValue[float64]
	reveal   bool
	env      ggui.Env
	natural  float64 // the height of the whole list
	up, down selectScrollArrow[T]
}

func (v *selectViewport[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	v.env = env
	natural := v.column.Layout(ggui.Constraints{MinW: c.MinW, MaxW: c.MaxW, MaxH: ggui.Unbounded}, env)
	v.natural = natural.H
	return c.Constrain(natural)
}

func (v *selectViewport[T]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := uitheme.Of(v.env)
	// What a scroll arrow covers of the viewport: its strip starts at the
	// panel's edge, in the padding around the list.
	coverTop, coverBottom := selectArrowHeight(t)-t.PanelPad.Top, selectArrowHeight(t)-t.PanelPad.Bottom
	limit := max(0, v.natural-r.Size.H)
	offset := scrollEdge(ggui.Untrack(v.offset.Get), limit)
	v.up.view, v.up.up, v.down.view = v, true, v
	for _, a := range []*selectScrollArrow[T]{&v.up, &v.down} {
		if a.due(offset, limit) {
			offset = a.step(offset, limit, r.Size.H, pick(a.up, coverTop, coverBottom))
			v.offset.Set(offset)
		}
	}
	if v.reveal {
		v.reveal = false
		if i := v.owner.highlight; i >= 0 && i < len(v.owner.items) {
			// The row is shown clear of the arrow that stands over the
			// list's edge while there is more that way.
			top, height := v.owner.rowSpan(i)
			if top < offset+coverTop {
				offset = scrollEdge(top-coverTop, limit)
				v.offset.Set(offset)
			} else if top+height > offset+r.Size.H-coverBottom {
				offset = scrollEdge(top+height-r.Size.H+coverBottom, limit)
				v.offset.Set(offset)
			}
		}
	}
	v.scroll.Layout(ggui.Constraints{MinW: r.Size.W, MaxW: r.Size.W, MinH: r.Size.H, MaxH: r.Size.H}, v.env)
	dst.Paint(v.scroll, r)
	v.up.paint(dst, t, r, offset > 0)
	v.down.paint(dst, t, r, offset < limit)
}

// selectArrowHeight is the height of a scroll arrow: shadcn's py-1 around
// a size-4 icon.
func selectArrowHeight(t *uitheme.Theme) float64 { return t.IconSize + t.Space }

// selectArrowPace is how often a list scrolls a row while the pointer is on
// one of its arrows, as Base UI's SelectScrollArrow has it.
const selectArrowPace = 40 * time.Millisecond

// scrollEdge is offset kept within a list that scrolls as far as limit, and
// brought to an end it is within a pixel of: Base UI's
// normalizeScrollOffset, which is what decides whether an arrow shows.
func scrollEdge(offset, limit float64) float64 {
	const tolerance = 1
	limit = max(limit, 0)
	offset = clamp(offset, 0, limit)
	switch {
	case offset <= tolerance && limit-offset <= tolerance:
		return pick(offset <= limit-offset, 0, limit)
	case offset <= tolerance:
		return 0
	case limit-offset <= tolerance:
		return limit
	}
	return offset
}

// selectScrollArrow is the chevron over the top or bottom edge of a list
// with more options that way: shadcn's SelectScrollUpButton and
// SelectScrollDownButton. The list scrolls a row at a time while the
// pointer is moved onto one and stays there.
type selectScrollArrow[T comparable] struct {
	view  *selectViewport[T]
	up    bool
	at    ggui.Point // where the pointer was last seen on it
	seen  bool
	shown bool      // painted last: there is more that way
	next  time.Time // when the list scrolls next, zero while it is not scrolling
}

// due reports whether the pointer has rested on the arrow long enough for
// the next row, and asks for the frame that is.
func (a *selectScrollArrow[T]) due(offset, limit float64) bool {
	if a.next.IsZero() {
		return false
	}
	if (a.up && offset <= 0) || (!a.up && offset >= limit) {
		a.next = time.Time{}
		return false
	}
	now := ggui.FrameTime()
	due := !now.Before(a.next)
	if due {
		a.next = now.Add(selectArrowPace)
	}
	ggui.WakeAt(a.next)
	return due
}

// step is the offset one row on from offset in the arrow's direction, in a
// viewport height tall whose arrow covers cover of it: the row before the
// first one wholly shown comes out from under the up arrow, and the row
// after the last one from under the down arrow.
func (a *selectScrollArrow[T]) step(offset, limit, height, cover float64) float64 {
	const tolerance = 1
	s := a.view.owner
	n := len(s.items)
	if n == 0 {
		return pick(a.up, 0, limit)
	}
	if a.up {
		first := 0
		for i := range n {
			if top, _ := s.rowSpan(i); top >= offset+cover-tolerance {
				first = i
				break
			}
		}
		if first == 0 {
			return 0
		}
		top, _ := s.rowSpan(first - 1)
		return scrollEdge(top-cover, limit)
	}
	last := n - 1
	for i := range n {
		if top, h := s.rowSpan(i); top+h > offset+height-cover+tolerance {
			last = max(0, i-1)
			break
		}
	}
	if last >= n-1 {
		return limit
	}
	top, h := s.rowSpan(last + 1)
	return scrollEdge(top+h-height+cover, limit)
}

// paint draws the arrow over its edge of the viewport r and takes the
// pointer there, while shown.
func (a *selectScrollArrow[T]) paint(dst *ggui.Canvas, t *uitheme.Theme, r ggui.Rect, shown bool) {
	if a.shown = shown; !shown {
		a.seen, a.next = false, time.Time{}
		return
	}
	// The strip runs from the panel's edge, inside its border, over the
	// padding and the first or last of the list.
	pad, line, h := t.PanelPad, t.BorderWidth, selectArrowHeight(t)
	panel := ggui.Rct(ggui.Pt(r.Origin.X-pad.Left+line, r.Origin.Y-pad.Top+line), ggui.Sz(r.Size.W+pad.Horizontal()-2*line, r.Size.H+pad.Vertical()-2*line))
	strip := ggui.Rct(panel.Origin, ggui.Sz(panel.Size.W, h-line))
	if !a.up {
		strip.Origin.Y = panel.Origin.Y + panel.Size.H - strip.Size.H
	}
	dst.Clip(strip).FillRoundRect(panel, max(t.Radius-line, 0), t.Popover)
	dst.HitPointer(strip, a)
	dst.HitCursor(strip, ggui.CursorShapeDefault)
	center := ggui.Pt(strip.Origin.X+strip.Size.W/2, pick(a.up, panel.Origin.Y-line+h/2, panel.Origin.Y+panel.Size.H+line-h/2))
	controlkit.PaintIconAt(dst, a.view.env, controlkit.IconMark{Role: pick(a.up, icons.ChevronUp, icons.ChevronDown), Center: center, Size: t.IconSize, Color: t.PopoverFg})
}

// HandlePointer implements ggui.PointerHandler: the pointer moving on the
// arrow starts the scroll and leaving it stops it. An arrow that appears
// under a pointer at rest does nothing until it moves, so a list opened
// over its trigger does not run off.
func (a *selectScrollArrow[T]) HandlePointer(ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerEnter:
		a.at, a.seen = ev.Pos, true
	case ggui.PointerMove:
		if a.seen && ev.Pos != a.at {
			a.view.owner.highlight = -1
			if a.next.IsZero() {
				a.next = ggui.FrameTime().Add(selectArrowPace)
				ggui.WakeAt(a.next)
			}
		}
		a.at, a.seen = ev.Pos, true
	case ggui.PointerExit:
		a.seen, a.next = false, time.Time{}
	case ggui.PointerScroll:
		return false
	}
	return true
}
