package ggui

// PopupWidget shows content floating over the tree, anchored below (or,
// when there is no room, above) its anchor widget: the base of dropdowns
// and menus. Build one with Popup. The anchor is laid out and painted in
// place; while the popup is open the content is painted through
// Canvas.Overlay, above everything, and a click anywhere outside it closes
// it without reaching what was clicked. The content is a focus scope: Tab
// cycles inside it, Escape closes it, and focus returns to the opener.
//
// Whether the popup is open lives in the widget, so keep it alive (in a
// Component, or adopted across rebuilds by the widget that owns it) or
// bind it to a StateValue with Bind. Widgets inside the content can find the
// popup with PopupOf and close it after acting.
type PopupWidget struct {
	anchor  Widget
	content Widget
	open    bool
	bound   Binding[bool]
	gap     float64
	keys    KeyHandler
	owner   any
	onClose func()
	id      any
	point   *Point

	env         Env
	contentSize Size
	rect        Rect // where the content was last painted
	reveal      Motion
	effect      *TransitionWidget
}

var popupKey = NewEnvKey[*PopupWidget]("popup")

// Popup creates a closed popup that opens content next to anchor.
func Popup(anchor, content Widget) *PopupWidget {
	return &PopupWidget{anchor: anchor, content: content, gap: 4, id: autoID(), effect: PopIn(content)}
}

// Bind stores the open state in sig: writing it opens or closes the popup,
// and the popup writes it when it closes itself.
func (p *PopupWidget) Bind(sig Binding[bool]) *PopupWidget { p.bound = sig; return p }

// Gap sets the space between the anchor and the content.
func (p *PopupWidget) Gap(v float64) *PopupWidget { p.gap = v; return p }

// Key gives the popup an identity, so a rebuilt one that also moved keeps
// its open state. Without one the anchor's Rect identifies it.
func (p *PopupWidget) Key(k any) *PopupWidget { p.id = k; return p }

// HitID implements Identified.
func (p *PopupWidget) HitID() any { return p.id }

// Adopt implements Adopter: a rebuilt popup stays open.
func (p *PopupWidget) Adopt(prev any) {
	if q, ok := prev.(*PopupWidget); ok {
		p.reveal = q.reveal
		if p.bound == nil {
			p.open = q.IsOpen()
			p.point = q.point
		}
	}
}

// HandlePointer implements PointerHandler for the anchor region, which
// exists only so a rebuilt popup can adopt: it consumes nothing.
func (p *PopupWidget) HandlePointer(PointerEvent) bool { return false }

// Keys registers h as the key handler over the content, so a click inside
// the popup keeps keyboard focus on h (the widget that opened it) instead
// of blurring it.
func (p *PopupWidget) Keys(h KeyHandler) *PopupWidget { p.keys = h; return p }

// Owner names the widget the content belongs to in the accessibility
// tree: the combobox or menu button that opened it. Without one the
// content, which paints through Overlay long after the tree did, would
// stand beside the whole tree instead of inside its opener.
func (p *PopupWidget) Owner(h any) *PopupWidget { p.owner = h; return p }

// OnClose fires when the popup closes, however it closed.
func (p *PopupWidget) OnClose(fn func()) *PopupWidget { p.onClose = fn; return p }

// IsOpen reports whether the content is showing.
func (p *PopupWidget) IsOpen() bool {
	if p.bound != nil {
		return Untrack(p.bound.Get)
	}
	return p.open
}

// SetOpen opens or closes the popup.
func (p *PopupWidget) SetOpen(v bool) {
	was := p.IsOpen()
	if p.bound != nil {
		p.bound.Set(v)
	} else {
		p.open = v
	}
	if was != v {
		Invalidate(p.env)
	}
	if was && !v {
		Invalidate(p.env)
		if p.onClose != nil {
			p.onClose()
		}
	}
}

// Show opens the popup.
func (p *PopupWidget) Show() { p.SetOpen(true) }

// ShowAt opens at a window-coordinate point instead of the anchor's edge.
// The point remains in effect until AnchorPosition is called.
func (p *PopupWidget) ShowAt(at Point) { p.point = &at; p.Show() }

// AnchorPosition restores placement relative to the anchor widget.
func (p *PopupWidget) AnchorPosition() { p.point = nil }

// Hide closes the popup.
func (p *PopupWidget) Hide() { p.SetOpen(false) }

// Toggle opens a closed popup and closes an open one.
func (p *PopupWidget) Toggle() { p.SetOpen(!p.IsOpen()) }

// Rect returns where the content was last painted, for a widget that
// positions itself relative to the popup.
func (p *PopupWidget) Rect() Rect { return p.rect }

// PopupOf returns the popup whose content the widget laid out under env is
// part of, if any. A menu item closes its menu this way.
func PopupOf(env Env) (*PopupWidget, bool) { return env.Get(popupKey) }

// Layout implements Widget.
func (p *PopupWidget) Layout(c Constraints, env Env) Size {
	p.env = env.With(popupKey, p)
	return p.anchor.Layout(c, env)
}

// Paint implements Widget.
func (p *PopupWidget) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, p)
	dst.Paint(p.anchor, r)
	open := p.IsOpen()
	now := Now()
	progress := p.reveal.Toggle(open, now, p.env.Motion(p.env.Theme().MotionFast))
	if !open && progress <= 0 {
		return
	}
	p.effect.Progress(progress, !open)
	dst.Overlay(func(dst *Canvas) {
		if !open {
			dst = dst.Inert()
		}
		p.paintContent(dst, r)
	}, dst.SemanticRef(p.owner))
}

// paintContent lays the content out for the room around the anchor and
// paints it, on a scrim that closes the popup when clicked.
func (p *PopupWidget) paintContent(dst *Canvas, anchor Rect) {
	screen := dst.Size()
	if screen == (Size{}) {
		screen = Sz(Unbounded, Unbounded)
	}
	maxW := max(screen.W-anchor.Origin.X, anchor.Size.W)
	if p.point != nil {
		// Point placement is clamped to the screen, so the whole width is
		// available regardless of where the anchor happened to be.
		anchor = Rct(Pt(clamp(p.point.X, 0, screen.W), clamp(p.point.Y, 0, screen.H)), Size{})
		maxW = screen.W
	}
	natural := p.effect.Layout(Constraints{MinW: anchor.Size.W, MaxW: maxW, MaxH: Unbounded}, p.env)
	below := screen.H - (anchor.Origin.Y + anchor.Size.H + p.gap)
	above := anchor.Origin.Y - p.gap
	room, y := below, anchor.Origin.Y+anchor.Size.H+p.gap
	if natural.H > below && above > below {
		room = above
		y = max(anchor.Origin.Y-p.gap-min(natural.H, above), 0)
	}
	size := natural
	if size.H > room {
		size = p.effect.Layout(Constraints{MinW: anchor.Size.W, MaxW: maxW, MaxH: max(room, 0)}, p.env)
	}
	x := anchor.Origin.X
	if screen.W != Unbounded {
		x = clamp(x, 0, max(screen.W-size.W, 0))
	}
	p.contentSize = size
	p.rect = Rct(Pt(x, y), size)

	dst.HitPointer(Rect{Size: screen}, popupScrim{p})
	// A focus scope: Tab stays inside the content and Escape closes.
	dst.FocusTrap(p, p.Hide, func(dst *Canvas) {
		dst.HitPointer(p.rect, popupSink{})
		if p.keys != nil {
			dst.HitKey(p.rect, p.keys)
		}
		dst.Paint(p.effect, p.rect)
	})
}

// popupScrim covers the window under an open popup: a press closes the
// popup and goes no further, while hovering and scrolling pass through.
type popupScrim struct{ p *PopupWidget }

func (s popupScrim) HandlePointer(ev PointerEvent) bool {
	switch ev.Kind {
	case PointerDown:
		s.p.Hide()
		return true
	case PointerUp, PointerTap:
		return true
	}
	return false
}

// popupSink keeps presses inside the content from reaching the scrim.
type popupSink struct{}

func (popupSink) HandlePointer(ev PointerEvent) bool { return ev.Kind != PointerScroll }
