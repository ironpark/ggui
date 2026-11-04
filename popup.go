package ggui

import (
	"image"
	"image/color"

	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/text"
)

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
// bind it to a StateValue with BindOpen. Widgets inside the content can find the
// popup with PopupOf and close it after acting.
type PopupWidget struct {
	props   property.Owner
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
	defer p.props.Layout().EndLayout()
	p.env = env.With(popupKey, p)
	return p.anchor.Layout(c, env)
}

// Paint implements Widget.
func (p *PopupWidget) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, p)
	dst.Paint(p.anchor, r)
	open := p.IsOpen()
	now := FrameTime()
	progress := p.reveal.Toggle(open, now, p.env.Motion(p.env.PopupDuration()))
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

// dispatchOver offers f to the overlay before the widgets, unless a widget
// holds a press, so that a drag that began in the app finishes there.
func (in *inputState) dispatchOver(o Overlay, f frameInput) {
	if o == nil || in.pressed != nil || !o.Input(overlayInput(f)) {
		in.dispatch(f)
	}
}

// cursorOver is the cursor at p: the overlay's when it wants one and no
// widget holds a press, else what the widgets asked for.
func (in *inputState) cursorOver(o Overlay, p Point) CursorShape {
	if o != nil && in.pressed == nil {
		if shape, ok := o.Cursor(p); ok {
			return shape
		}
	}
	return in.cursor
}

// DrawText draws s as one unwrapped line with its top-left corner at the
// logical point at, in font at size logical pixels; a nil font is the
// default. A custom widget's Paint draws a label with it in the style its
// Layout kept from Env.ResolveText, so the label follows the theme; text
// that wraps, selects or is read by a screen reader is a Text painted with
// Canvas.Paint.
func (c *Canvas) DrawText(s string, font *Font, size float64, at Point, col color.Color) {
	if c == nil || c.Image == nil {
		return
	}
	if font == nil {
		font = fallbackFont()
	}
	op := &text.DrawOptions{}
	op.ColorScale.ScaleWithColor(col)
	op.GeoM.Translate(c.px(at.X), c.px(at.Y))
	fontsys.DrawText(c.Image, s, font.face(c.px(size), 0), op)
}

// TextWidth measures s as DrawText would draw it, in logical pixels.
func (c *Canvas) TextWidth(s string, font *Font, size float64) float64 {
	if font == nil {
		font = fallbackFont()
	}
	return c.dp(textlayout.Width(s, font.face(c.px(size), 0)))
}

// DrawTextStyle is DrawText in a whole TextStyle: its Font, Size and Color,
// and its Weight, drawn with the font's face nearest it or along the weight
// axis of a variable font. A label kept from Env.ResolveText keeps its
// weight this way; zero fields fall back as a Text's would.
func (c *Canvas) DrawTextStyle(s string, st TextStyle, at Point) {
	if c == nil || c.Image == nil {
		return
	}
	st = st.resolved()
	op := &text.DrawOptions{}
	op.ColorScale.ScaleWithColor(st.Color)
	op.GeoM.Translate(c.px(at.X), c.px(at.Y))
	fontsys.DrawText(c.Image, s, st.Font.face(c.px(st.Size), st.Weight), op)
}

// TextStyleWidth measures s as DrawTextStyle would draw it, in logical
// pixels.
func (c *Canvas) TextStyleWidth(s string, st TextStyle) float64 {
	st = st.resolved()
	return c.dp(textlayout.Width(s, st.Font.face(c.px(st.Size), st.Weight)))
}

// FitText returns s as it fits in width at size, cut with "…" when it is
// too wide: what Text's Ellipsis draws, for a widget drawing its own.
func (c *Canvas) FitText(s string, font *Font, size, width float64) string {
	if font == nil {
		font = fallbackFont()
	}
	return textlayout.Ellipsize(s, font.face(c.px(size), 0), c.px(width), false)
}

// Physical is r in the pixels of Image, for an Overlay that keeps an image
// of its own the size of what it draws.
func (c *Canvas) Physical(r Rect) image.Rectangle { return c.physical(r) }

// DefaultFont is the font Text uses when none is set.
func DefaultFont() *Font { return fallbackFont() }
