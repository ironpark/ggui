package ui

import (
	"image/color"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// modal is the half of a modal panel that Dialog and Sheet share: what is
// shown, what it is called, whether a click beside it closes it, and the
// scrim, focus trap and sink that make it modal at all. The two differ only
// in where the panel goes and whether it slides there, which is the half
// each of them keeps.
type modal struct {
	surface     ggui.Widget // the panel under its close button, as paint hands it on
	open        ggui.Binding[bool]
	content     ggui.Widget
	title       *ggui.TextWidget
	name        string
	compact     bool
	dismissible bool
	onClose     func()
	hideClose   bool
	closeButton *ButtonWidget
	closeSize   ggui.Size

	panel *ggui.BoxWidget
	theme *uitheme.Theme
	env   ggui.Env
	rect  ggui.Rect
}

// setTitle puts a heading above the content and names the panel with it.
func (m *modal) setTitle(s string) { m.title, m.name = Heading(s), s }

// Close closes the panel.
func (m *modal) Close() {
	if !ggui.Untrack(m.open.Get) {
		return
	}
	m.open.Set(false)
	if !ggui.Untrack(m.open.Get) && m.onClose != nil {
		m.onClose()
	}
}

// Rect returns where the panel was last painted.
func (m *modal) Rect() ggui.Rect { return m.rect }

// Semantics implements ggui.Semantic.
func (m *modal) Semantics() (ggui.Role, string) { return ggui.RoleDialog, m.name }

// build makes the panel out of the title and the content, on the surface
// every floating thing uses. The caller finishes it with the border and
// radius its own shape wants, and lays it out when it knows the room.
func (m *modal) build(env ggui.Env) *ggui.BoxWidget {
	t := uitheme.Of(env)
	m.theme, m.env = t, env
	body := m.content
	if m.title != nil {
		body = ggui.Column(m.title, m.content).Gap(t.Space * 2).Align(ggui.AlignStretch)
	}
	if m.closeButton == nil {
		m.closeButton = ButtonOf(Icon(icons.Close), m.Close).Name("Close").Ghost().Size(ButtonIconSmall)
	}
	m.closeSize = m.closeButton.Layout(ggui.Loose(ggui.Sz(28, 28)), env)
	m.panel = ggui.Box(ggui.Styled(body).Color(t.PopoverFg)).Padding(t.CardPad).Fill(t.Popover).Shadow(t.OverlayShadow)
	if m.compact {
		m.panel.Pad(0)
	}
	return m.panel
}

// paint dims the window, takes what lands on it, and paints the panel at
// rect inside a focus trap. owner is the widget the node belongs to, so a
// screen reader and Probe.Find see the dialog or the sheet rather than this.
func (m *modal) paint(dst *ggui.Canvas, screen ggui.Size, rect ggui.Rect, scrim color.Color, owner ggui.Semantic) {
	if m.surface == nil {
		// Built once: the panel is asked for each time, since build replaces it.
		m.surface = ggui.FromFuncs(func(c ggui.Constraints, e ggui.Env) ggui.Size { return m.panel.Layout(c, e) }, m.paintSurface)
	}
	m.paintContent(dst, screen, rect, scrim, owner, m.surface)
}

func (m *modal) paintContent(dst *ggui.Canvas, screen ggui.Size, rect ggui.Rect, scrim color.Color, owner ggui.Semantic, content ggui.Widget) {
	m.rect = rect
	dst.BackdropBlur(ggui.Rect{Size: screen}, 0, 4)
	dst.FillRect(ggui.Rect{Size: screen}, scrim)
	dst.HitPointer(ggui.Rect{Size: screen}, modalScrim{m})
	// The panel is the node; the scrim is a way of taking clicks, not an
	// element, so it describes nothing.
	dst.DescribeNode(rect, owner, func(dst *ggui.Canvas) {
		dst.FocusTrap(owner, m.Close, func(dst *ggui.Canvas) {
			dst.HitPointer(rect, modalSink{owner})
			dst.Paint(content, rect)
		})
	})
}

// modalScrim covers the window under an open panel: a press closes it when
// it may be dismissed, and nothing reaches what is beneath either way.
type modalScrim struct{ m *modal }

func (s modalScrim) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind == ggui.PointerDown && ev.Button == ggui.MouseButtonLeft && s.m.dismissible {
		s.m.Close()
	}
	return true
}

// modalSink keeps presses inside the panel from reaching the scrim and
// names the panel for Probe.Find.
type modalSink struct{ owner ggui.Semantic }

func (s modalSink) HandlePointer(ev ggui.PointerEvent) bool { return ev.Kind != ggui.PointerScroll }

func (s modalSink) Semantics() (ggui.Role, string) { return s.owner.Semantics() }

func (m *modal) paintSurface(dst *ggui.Canvas, r ggui.Rect) {
	dst.Paint(m.panel, r)
	if !m.hideClose {
		dst.Paint(m.closeButton, ggui.Rct(ggui.Pt(r.Origin.X+r.Size.W-m.closeSize.W-8, r.Origin.Y+8), m.closeSize))
	}
}
