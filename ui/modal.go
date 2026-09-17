package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// modal is the half of a modal panel that Dialog and Sheet share: what is
// shown, what it is called, whether a click beside it closes it, and the
// scrim, focus trap and sink that make it modal at all. The two differ only
// in where the panel goes and whether it slides there, which is the half
// each of them keeps.
type modal struct {
	open        ggui.Binding[bool]
	content     ggui.Widget
	title       *ggui.TextWidget
	name        string
	compact     bool
	dismissible bool
	onClose     func()

	panel *ggui.BoxWidget
	theme ggui.Theme
	env   ggui.Env
	rect  ggui.Rect
}

// setTitle puts a heading above the content and names the panel with it.
func (m *modal) setTitle(s string) { m.title, m.name = ggui.Title(s).Size(18), s }

// Close closes the panel.
func (m *modal) Close() {
	if !m.open.Peek() {
		return
	}
	m.open.Set(false)
	if m.onClose != nil {
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
	t := env.Theme()
	m.theme, m.env = t, env
	body := m.content
	if m.title != nil {
		body = ggui.Column(m.title, m.content).Gap(t.Space * 2).Align(ggui.AlignStretch)
	}
	m.panel = ggui.Box(body).Padding(t.CardPad).Fill(t.Popover).Shadow(t.OverlayShadow)
	if m.compact {
		m.panel.Pad(0)
	}
	return m.panel
}

// paint dims the window, takes what lands on it, and paints the panel at
// rect inside a focus trap. owner is the widget the node belongs to, so a
// screen reader and Probe.Find see the dialog or the sheet rather than this.
func (m *modal) paint(dst *ggui.Canvas, screen ggui.Size, rect ggui.Rect, scrim color.Color, owner ggui.Semantic) {
	m.rect = rect
	dst.FillRect(ggui.Rect{Size: screen}, scrim)
	dst.HitPointer(ggui.Rect{Size: screen}, modalScrim{m})
	// The panel is the node; the scrim is a way of taking clicks, not an
	// element, so it describes nothing.
	dst.DescribeNode(rect, owner, func(dst *ggui.Canvas) {
		dst.FocusTrap(owner, m.Close, func(dst *ggui.Canvas) {
			dst.HitPointer(rect, modalSink{owner})
			dst.Paint(m.panel, rect)
		})
	})
}

// modalScrim covers the window under an open panel: a press closes it when
// it may be dismissed, and nothing reaches what is beneath either way.
type modalScrim struct{ m *modal }

func (s modalScrim) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind == ggui.PointerDown && ev.Button == ebiten.MouseButtonLeft && s.m.dismissible {
		s.m.Close()
	}
	return true
}

// modalSink keeps presses inside the panel from reaching the scrim and
// names the panel for Probe.Find.
type modalSink struct{ owner ggui.Semantic }

func (s modalSink) HandlePointer(ev ggui.PointerEvent) bool { return ev.Kind != ggui.PointerScroll }

func (s modalSink) Semantics() (ggui.Role, string) { return s.owner.Semantics() }
