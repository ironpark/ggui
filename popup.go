package ggui

import (
	"slices"

	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
)

// popupEscape closes the innermost open popup.
var popupEscape = MustChord("escape")

// PopupWidget shows content floating over the tree, anchored below (or,
// when there is no room, above) its anchor widget: the base of dropdowns
// and menus. Side opens it on another side of the anchor, and Align and
// AlignOffset place it along that side. It enters with a fade, a slight
// scale from the edge that faces the anchor and a short slide away from it,
// and leaves the same way backwards. Build one with Popup. The anchor is laid out and painted in
// place; while the popup is open the content is painted through
// Canvas.Overlay, above everything, and a click anywhere outside it closes
// it without reaching what was clicked. The content is a focus scope: Tab
// cycles inside it, Escape closes it, and focus returns to the opener.
//
// Whether the popup is open lives in the widget, so keep it alive (in a
// Component, or adopted across rebuilds by the widget that owns it) or
// bind it to a StateValue with BindOpen. Widgets inside the content can find the
// popup with PopupOf and close it after acting.
type PopupWidget struct {
	props           property.Owner
	anchor          Widget
	content         Widget
	open            bool
	bound           Binding[bool]
	gap             float64
	keys            KeyHandler
	owner           any
	onClose         func()
	nonmodal        bool
	align           PopupAlignment
	side            PopupSide
	placed          PopupSide // side, once direction and collisions are settled
	alignOffset     float64
	still           bool // Animated(false): no entrance or exit
	anchorRect      Rect
	paintedOpen     bool
	hadContentFocus bool
	contentKeys     []KeyHandler
	children        []*PopupWidget
	parent          *PopupWidget
	opener          KeyHandler
	restore         bool
	focusContent    bool
	id              any
	point           *Point

	env         Env
	contentSize Size
	rect        Rect // where the content was last painted
	reveal      Motion
	effect      *TransitionWidget
}

var popupKey = NewEnvKey[*PopupWidget]("popup")

// PopupAlignment positions content horizontally relative to its anchor.
// Start and End follow the environment's writing direction.
type PopupAlignment uint8

const (
	PopupStart PopupAlignment = iota
	PopupCenter
	PopupEnd
)

// Align sets where the content sits along the anchor's side: for a popup
// below or above, its horizontal alignment (logical start by default); for
// one to the left or right, PopupStart lines the tops up and PopupEnd the
// bottoms.
func (p *PopupWidget) Align(v PopupAlignment) *PopupWidget {
	defer property.Watch(&p.props, &p.align)()
	p.align = v
	return p
}

// PopupSide is the side of its anchor a popup opens on.
type PopupSide uint8

const (
	// PopupBottom opens below the anchor. It is the default.
	PopupBottom PopupSide = iota
	// PopupTop opens above the anchor.
	PopupTop
	// PopupLeft opens to the left of the anchor.
	PopupLeft
	// PopupRight opens to the right of the anchor.
	PopupRight
	// PopupInlineStart opens where a line of text starts: to the left, or
	// to the right under a right-to-left direction.
	PopupInlineStart
	// PopupInlineEnd opens where a line of text ends: to the right, or to
	// the left under a right-to-left direction. A submenu opens here.
	PopupInlineEnd
)

// Side sets the side of the anchor the content opens on, PopupBottom by
// default. When the content does not fit there and the opposite side has
// more room, it opens on the opposite side instead; PlacedSide reports
// which. Below and above, the content is at least as wide as the anchor;
// to the left and right it takes its own width.
func (p *PopupWidget) Side(v PopupSide) *PopupWidget {
	defer property.Watch(&p.props, &p.side)()
	p.side = v
	return p
}

// AlignOffset moves the content along the anchor's side, away from the edge
// Align lines it up with: a positive offset moves a PopupStart popup toward
// the end and a PopupEnd one toward the start, and a negative one the other
// way, so that -4 lets either overhang its anchor by four pixels. A
// PopupCenter popup moves toward the end.
func (p *PopupWidget) AlignOffset(v float64) *PopupWidget {
	defer property.Watch(&p.props, &p.alignOffset)()
	p.alignOffset = v
	return p
}

// Animated says whether the content plays its entrance and exit; it does by
// default. A popup that opens over its own anchor, as a list aligned with
// the item it shows does, has nowhere to come from and turns it off.
func (p *PopupWidget) Animated(v bool) *PopupWidget {
	defer property.Watch(&p.props, &p.still)()
	p.still = !v
	return p
}

// PlacedSide returns the side the content was last painted on: Side with
// the writing direction applied, or its opposite when it had to flip. It is
// one of PopupBottom, PopupTop, PopupLeft and PopupRight.
func (p *PopupWidget) PlacedSide() PopupSide { return p.placed }


// Modal controls focus trapping and outside-click interception. The default
// is true. A nonmodal popup focuses its content on opening, permits Tab to
// leave, closes when focus leaves both anchor and content, and lets outside
// presses reach their targets. Escape returns focus to the opener.
func (p *PopupWidget) Modal(v bool) *PopupWidget {
	defer property.Watch(&p.props, &p.nonmodal)()
	p.nonmodal = !v
	return p
}

// Popup creates a closed popup that opens content next to anchor.
func Popup(anchor, content Widget) *PopupWidget {
	return &PopupWidget{anchor: anchor, content: content, gap: 4, id: reactive.AutoID(), effect: PopIn(content)}
}

// BindOpen stores the open state in sig: writing it opens or closes the popup,
// and the popup writes it when it closes itself.
func (p *PopupWidget) BindOpen(sig Binding[bool]) *PopupWidget {
	property.Require(sig, "BindOpen")
	if !property.Same(p.bound, sig) {
		p.bound = sig
		p.props.Changed()
	}
	return p
}

// Open detaches an open-state binding and sets local popup visibility.
func (p *PopupWidget) Open(v bool) *PopupWidget {
	was := p.IsOpen()
	detached := p.bound != nil
	p.bound = nil
	p.open = v
	if detached || was != v {
		p.props.Changed()
	}
	if was && !v && p.onClose != nil {
		p.onClose()
	}
	return p
}

// Gap sets the space between the anchor and the content.
func (p *PopupWidget) Gap(v float64) *PopupWidget {
	defer property.Watch(&p.props, &p.gap)()
	p.gap = v
	return p
}

// Key gives the popup an identity, so a rebuilt one that also moved keeps
// its open state. Without one the anchor's Rect identifies it.
func (p *PopupWidget) Key(k any) *PopupWidget { p.id = k; return p }

// HitID implements Identified.
func (p *PopupWidget) HitID() any { return p.id }

// Adopt implements Adopter: a rebuilt popup stays open.
func (p *PopupWidget) Adopt(prev any) {
	if q, ok := prev.(*PopupWidget); ok {
		p.reveal = q.reveal
		p.paintedOpen, p.opener = q.paintedOpen, q.opener
		p.hadContentFocus, p.contentKeys = q.hadContentFocus, q.contentKeys
		p.children = q.children
		if p.bound == nil {
			p.open = q.IsOpen()
			p.point = q.point
		}
	}
}

// HandlePointer implements PointerHandler for the anchor region, which
// exists only so a rebuilt popup can adopt: it consumes nothing.
func (p *PopupWidget) HandlePointer(PointerEvent) bool { return false }

// HandleKey handles Escape when a text-only nonmodal panel owns focus.
func (p *PopupWidget) HandleKey(e KeyEvent) {
	if e.Kind == KeyPress && e.Key == KeyEscape {
		p.Hide()
	}
}

// ConsumesKey reserves Escape for a text-only popup's focus target.
func (p *PopupWidget) ConsumesKey(e KeyEvent) bool { return e.Kind == KeyPress && e.Key == KeyEscape }

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
	actual := p.IsOpen()
	if was != actual {
		Invalidate(p.env)
	}
	if was && !actual {
		Invalidate(p.env)
		if p.onClose != nil {
			p.onClose()
		}
	}
}

// FocusContent requests the first focusable content control on the next paint.
// This is useful when a nonmodal popup switches between different panels.
func (p *PopupWidget) FocusContent() *PopupWidget {
	p.focusContent = true
	p.paintedOpen = false
	return p
}

// Show opens the popup.
func (p *PopupWidget) Show() { p.SetOpen(true) }

// ShowAt opens at a window-coordinate point instead of the anchor's edge.
// The point remains in effect until AnchorPosition is called.
func (p *PopupWidget) ShowAt(at Point) { p.point = &at; p.Show() }

// AnchorPosition restores placement relative to the anchor widget.
func (p *PopupWidget) AnchorPosition() { p.point = nil }

// Hide closes the popup.
func (p *PopupWidget) Hide() {
	was := p.IsOpen()
	p.SetOpen(false)
	if p.nonmodal && was && !p.IsOpen() {
		p.restore = p.opener != nil
	}
}

// Toggle opens a closed popup and closes an open one.
func (p *PopupWidget) Toggle() { p.SetOpen(!p.IsOpen()) }

// Rect returns where the content was last painted, for a widget that
// positions itself relative to the popup.
func (p *PopupWidget) Rect() Rect { return p.rect }

// PopupOf returns the popup whose content the widget laid out under env is
// part of, if any. A menu item closes its menu this way.
func PopupOf(env Env) (*PopupWidget, bool) { return env.Get(popupKey) }

// attach lists p among its parent popup's children, once. The parent
// empties the list as it paints, so both passes ask.
func (p *PopupWidget) attach() {
	if p.parent != nil && !slices.Contains(p.parent.children, p) {
		p.parent.children = append(p.parent.children, p)
	}
}

// Layout implements Widget.
func (p *PopupWidget) Layout(c Constraints, env Env) Size {
	defer p.props.Layout().EndLayout()
	if parent, ok := PopupOf(env); ok && parent != p {
		p.parent = parent
		p.attach()
	}
	p.env = env.With(popupKey, p)
	return p.anchor.Layout(c, env)
}

// Paint implements Widget.
func (p *PopupWidget) Paint(dst *Canvas, r Rect) {
	p.attach()
	dst.HitPointer(r, p)
	p.anchorRect = r
	anchorFocused := dst.PaintFocus(p.anchor, r)
	if p.restore {
		for _, hit := range dst.fs().hits {
			if hit.key != nil && (sameAny(hit.key, p.opener) || idOf(p.opener) != nil && sameAny(hit.id, idOf(p.opener))) {
				dst.RequestFocus(hit.key)
				break
			}
		}
		p.restore = false
	}
	open := p.IsOpen()
	if p.nonmodal {
		if open && !p.paintedOpen {
			p.opener = dst.fs().focusKey
			p.focusContent = true
			p.hadContentFocus = false
		}
		inside := p.ownsFocus(dst)
		p.hadContentFocus = p.hadContentFocus || inside
		if open && p.hadContentFocus && !inside && !anchorFocused && dst.fs().focusKey != nil {
			present := false
			for _, hit := range dst.fs().prev {
				present = present || sameAny(hit.key, dst.fs().focusKey)
			}
			if present {
				p.SetOpen(false)
				open = p.IsOpen()
			} else {
				p.focusContent = true
			}
		}
	}
	p.paintedOpen = open
	now := FrameTime()
	duration := p.env.Motion(p.env.PopupDuration())
	if p.still {
		duration = 0
	}
	progress := p.reveal.Toggle(open, now, duration)
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
func (p *PopupWidget) ownsFocus(dst *Canvas) bool {
	for _, key := range p.contentKeys {
		if dst.HasFocus(key) {
			return true
		}
	}
	for _, child := range p.children {
		if (child.IsOpen() || child.restore || child.paintedOpen) && child.ownsFocus(dst) {
			return true
		}
	}
	return false
}

func (p *PopupWidget) paintContent(dst *Canvas, anchor Rect) {
	p.children = p.children[:0]
	screen := dst.Size()
	if screen == (Size{}) {
		screen = Sz(Unbounded, Unbounded)
	}
	if p.point != nil {
		// Point placement is clamped to the screen, so the whole width is
		// available regardless of where the anchor happened to be.
		anchor = Rct(Pt(clamp(p.point.X, 0, screen.W), clamp(p.point.Y, 0, screen.H)), Size{})
	}
	rtl := p.env.Direction() == DirRTL
	side := p.side
	switch side {
	case PopupInlineStart:
		side = pick(rtl, PopupRight, PopupLeft)
	case PopupInlineEnd:
		side = pick(rtl, PopupLeft, PopupRight)
	}
	var size Size
	var at Point
	var along float64
	if side == PopupLeft || side == PopupRight {
		size, at, along, side = p.placeBeside(screen, anchor, side)
	} else {
		size, at, along, side = p.placeBelow(screen, anchor, side, rtl)
	}
	p.placed = side
	p.contentSize = size
	p.rect = Rct(at, size)
	p.effect.PopFrom(side, along)

	dst.HitPointer(Rect{Size: screen}, popupScrim{p})
	paint := func(dst *Canvas) {
		dst.HitPointer(p.rect, popupSink{})
		if p.keys != nil {
			dst.HitKey(p.rect, p.keys)
		}
		first := len(dst.fs().hits)
		dst.Paint(p.effect, p.rect)
		p.contentKeys = p.contentKeys[:0]
		for _, hit := range dst.fs().hits[first:] {
			if hit.key != nil {
				p.contentKeys = append(p.contentKeys, hit.key)
			}
		}
		if p.nonmodal && len(p.contentKeys) == 0 {
			dst.HitKey(p.rect, p)
			p.contentKeys = append(p.contentKeys, p)
		}
		if p.nonmodal && p.focusContent {
			p.focusContent = false
			for _, hit := range dst.fs().hits[first:] {
				if hit.key != nil {
					dst.RequestFocus(hit.key)
					break
				}
			}
		}
	}
	if p.nonmodal {
		paint(dst)
		childOpen := false
		for _, child := range p.children {
			childOpen = childOpen || child.IsOpen()
		}
		if p.IsOpen() && !childOpen {
			dst.Shortcut(popupEscape, p.Hide)
		}
	} else {
		dst.FocusTrap(p, p.Hide, paint)
	}
}

// placeBelow lays the content out below or above the anchor, whichever side
// asks for unless only the other has the room, and aligns it across. It
// returns the content's size and place, how far along the anchor's side its
// entrance grows from, and the side it ended on.
func (p *PopupWidget) placeBelow(screen Size, anchor Rect, side PopupSide, rtl bool) (Size, Point, float64, PopupSide) {
	maxW := max(screen.W-anchor.Origin.X, anchor.Size.W)
	if p.align != PopupStart || rtl || p.point != nil {
		maxW = screen.W
	}
	natural := p.effect.Layout(Constraints{MinW: anchor.Size.W, MaxW: maxW, MaxH: Unbounded}, p.env)
	below := screen.H - (anchor.Origin.Y + anchor.Size.H + p.gap)
	above := anchor.Origin.Y - p.gap
	room, other := below, above
	if side == PopupTop {
		room, other = above, below
	}
	if natural.H > room && other > room {
		room, side = other, pick(side == PopupTop, PopupBottom, PopupTop)
	}
	size := natural
	if size.H > room {
		size = p.effect.Layout(Constraints{MinW: anchor.Size.W, MaxW: maxW, MaxH: max(room, 0)}, p.env)
	}
	y := anchor.Origin.Y + anchor.Size.H + p.gap
	if side == PopupTop {
		y = max(anchor.Origin.Y-p.gap-min(size.H, max(above, 0)), 0)
	}
	along := 0.0
	// The alignment is logical: its start is the right edge under RTL.
	x := anchor.Origin.X
	switch {
	case p.align == PopupCenter:
		x += (anchor.Size.W-size.W)/2 + pick(rtl, -p.alignOffset, p.alignOffset)
		along = .5
	case (p.align == PopupEnd) != rtl:
		x += anchor.Size.W - size.W - p.alignOffset
		along = 1
	default:
		x += p.alignOffset
	}
	if screen.W != Unbounded {
		x = clamp(x, 0, max(screen.W-size.W, 0))
	}
	return size, Pt(x, y), along, side
}

// placeBeside is placeBelow for a popup to the left or right of its anchor.
func (p *PopupWidget) placeBeside(screen Size, anchor Rect, side PopupSide) (Size, Point, float64, PopupSide) {
	natural := p.effect.Layout(Constraints{MaxW: screen.W, MaxH: Unbounded}, p.env)
	left := anchor.Origin.X - p.gap
	right := screen.W - (anchor.Origin.X + anchor.Size.W + p.gap)
	room, other := right, left
	if side == PopupLeft {
		room, other = left, right
	}
	if natural.W > room && other > room {
		room, side = other, pick(side == PopupLeft, PopupRight, PopupLeft)
	}
	size := natural
	if size.W > room || size.H > screen.H {
		size = p.effect.Layout(Constraints{MaxW: max(min(room, screen.W), 0), MaxH: screen.H}, p.env)
	}
	x := anchor.Origin.X + anchor.Size.W + p.gap
	if side == PopupLeft {
		x = max(anchor.Origin.X-p.gap-size.W, 0)
	}
	y, along := anchor.Origin.Y, 0.0
	switch p.align {
	case PopupCenter:
		y += (anchor.Size.H-size.H)/2 + p.alignOffset
		along = .5
	case PopupEnd:
		y += anchor.Size.H - size.H - p.alignOffset
		along = 1
	default:
		y += p.alignOffset
	}
	if screen.H != Unbounded {
		y = clamp(y, 0, max(screen.H-size.H, 0))
	}
	return size, Pt(x, y), along, side
}

// popupScrim covers the window under an open popup: a press closes the
// popup and goes no further, while hovering and scrolling pass through.
type popupScrim struct{ p *PopupWidget }

func (s popupScrim) HandlePointer(ev PointerEvent) bool {
	switch ev.Kind {
	case PointerDown:
		if s.p.nonmodal {
			s.p.SetOpen(false)
			return s.p.anchorRect.Contains(ev.Pos)
		}
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

// An overlay is drawn over the whole window after the widget tree, and is
// offered the frame's input before any widget: a development panel, a
// debugging readout, a screen-recording banner. The widget inspector is
// one. An overlay cannot be a widget, since it paints while the frame's
// trace is being read back, so it draws imperatively with the Canvas
// methods below; what a widget gets for free, text in particular, is here
// for it too.

// Overlay is something drawn over every frame that may take its input.
type Overlay interface {
	// Paint draws over the finished frame. dst covers the whole window.
	Paint(dst *Canvas)
	// Input is offered the frame's input before the widgets, and reports
	// whether it took it, in which case no widget sees it. It is not
	// offered while a widget holds a press, so an app drag finishes where
	// it started.
	Input(in OverlayInput) bool
	// Cursor is the mouse cursor the overlay wants at p, if any.
	Cursor(p Point) (CursorShape, bool)
}

// OverlayInput is one frame's input as an Overlay sees it.
type OverlayInput struct {
	Pos      Point
	Down, Up []MouseButton
	Wheel    Point
	Keys     []KeyboardKey // just pressed, plus repeats of held keys
	Text     string
	Mods     Mods
}

// overlayInput is the frame's input in the form an Overlay takes.
func overlayInput(f frameInput) OverlayInput {
	return OverlayInput{Pos: f.pos, Down: f.down, Up: f.up, Wheel: f.wheel, Keys: f.keys, Text: f.text, Mods: f.mods}
}

// SetOverlay installs o over the window, or removes the overlay when o is
// nil. There is one at a time: turning the inspector on installs it here
// and turning it off removes it.
func (w *Window) SetOverlay(o Overlay) { w.overlay = o }

// DefaultFont is the font Text uses when none is set.
func DefaultFont() *Font { return fallbackFont() }
