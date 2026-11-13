package ui

import (
	"image/color"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// modalBlur is how far the window behind a modal panel is blurred, in
// pixels: shadcn's backdrop-blur-xs.
const modalBlur = 4

// modal is the half of a modal panel that Dialog and Sheet share: what is
// shown, what it is called, whether a click beside it closes it, and the
// scrim, focus trap and sink that make it modal at all. The two differ only
// in where the panel goes and whether it slides there, which is the half
// each of them keeps.
type modal struct {
	open        ggui.Binding[bool]
	content     ggui.Widget
	title       *ggui.TextWidget
	description *ggui.TextWidget
	actions     []ggui.Widget // what Footer was given
	actionsRev  int           // counts the calls to setFooter
	evenActions bool          // the actions share the footer's width equally
	name        string
	compact     bool
	dismissible bool
	onClose     func()
	hideClose   bool
	closeButton *ButtonWidget
	closeSize   ggui.Size

	// What build assembled, and from which parts: it is put together again
	// only when one of them is replaced, and restyled every layout.
	parts  modalParts
	shape  modalShape
	header *ggui.ColumnWidget
	body   *ggui.ColumnWidget
	inner  *ggui.BoxWidget // pads the header and the content, not the footer
	styled *ggui.StyledWidget
	footer *modalFooter

	panel *ggui.BoxWidget
	theme *uitheme.Theme
	env   ggui.Env
	rect  ggui.Rect
}

// modalParts is what a panel was assembled from.
type modalParts struct {
	title, description *ggui.TextWidget
	actionsRev         int
	evenActions        bool
	band, fill         bool
}

// modalShape is what a kind of panel decides about the parts all of them
// share. Each owner passes its own to build every layout.
type modalShape struct {
	headerGap float64 // between the title and the description
	// titleLeading is the title's line height as a multiple of its size: 1
	// for a Dialog's leading-none, 1.5 for the text-base a Sheet's title
	// is left in. Zero leaves the theme's Heading as it is.
	titleLeading float64
	closeInset   float64 // from the close button to the panel's top and outer edge
	// band draws the footer as Dialog's does: a tinted strip from edge to
	// edge under a line, its actions in a row at the end. Without it the
	// actions are stacked at full width, as a Sheet's are.
	band bool
	// fill says the panel is stretched along its height, so the footer goes
	// to its bottom rather than under the content.
	fill bool
	// topInset is room kept clear above the header, inside the padding or
	// without it: where a drawer's handle goes.
	topInset    float64
	fillColor   color.Color         // nil for an owner that paints the surface itself
	shadow      [2]ggui.ShadowStyle // the layers; a nil Color leaves one out
	borderWidth float64
	border      color.Color
	radius      float64
}

// mutedText is the style of a description: the theme's body text in its
// muted color.
func mutedText(t *uitheme.Theme) ggui.TextStyle {
	s := t.Text
	s.Color = t.MutedFg
	return s
}

// setTitle puts a heading above the content and names the panel with it.
func (m *modal) setTitle(s string) { m.title, m.name = Heading(s), s }

// setDescription puts a line of muted text under the title; an empty
// string takes it away.
func (m *modal) setDescription(s string) {
	switch {
	case s == "":
		m.description = nil
	case m.description == nil:
		m.description = ggui.Text(s)
	default:
		m.description.Content(s)
	}
}

// setFooter puts actions at the foot of the panel; none takes the footer
// away.
func (m *modal) setFooter(actions []ggui.Widget) {
	m.actions = actions
	m.actionsRev++
}

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

// build makes the panel out of the title, the description, the content and
// the footer, on the surface every floating thing uses and in the shape its
// owner asks for. The caller lays it out when it knows the room.
func (m *modal) build(env ggui.Env, shape modalShape) *ggui.BoxWidget {
	t := uitheme.Of(env)
	m.theme, m.env, m.shape = t, env, shape
	if m.closeButton == nil {
		m.closeButton = ButtonOf(Icon(icons.Close), m.Close).Name("Close").Ghost().Size(ButtonIconSmall)
	}
	m.closeSize = m.closeButton.Layout(ggui.Loose(ggui.Sz(t.ControlSize*2, t.ControlSize*2)), env)
	parts := modalParts{m.title, m.description, m.actionsRev, m.evenActions, shape.band, shape.fill}
	if m.panel == nil || parts != m.parts {
		m.parts = parts
		m.assemble()
	}
	if m.description != nil {
		m.description.Style(mutedText(t))
	}
	if m.title != nil && shape.titleLeading != 0 {
		m.title.LineHeight(shape.titleLeading)
	}
	if m.header != nil {
		m.header.Gap(shape.headerGap)
	}
	m.body.Gap(t.Space * 2)
	m.styled.Color(t.PopoverFg)
	pad := pick(m.compact, ggui.EdgeInsets{}, t.CardPad)
	pad.Top += shape.topInset
	m.inner.Padding(pad)
	m.panel.Fill(shape.fillColor).Shadow(shape.shadow[:]...).Border(shape.borderWidth, shape.border).Radius(shape.radius)
	return m.panel
}

// assemble puts the parts together: the header and the content in a padded
// column, and under it the footer, which a band takes to the panel's edges.
func (m *modal) assemble() {
	var rows []ggui.Widget
	m.header = nil
	if m.title != nil || m.description != nil {
		var lines []ggui.Widget
		if m.title != nil {
			lines = append(lines, m.title)
		}
		if m.description != nil {
			lines = append(lines, m.description)
		}
		m.header = ggui.Column(lines...).Align(ggui.AlignStretch)
		rows = append(rows, m.header)
	}
	rows = append(rows, m.content)
	m.footer = nil
	if len(m.actions) > 0 {
		m.footer = &modalFooter{m: m, band: m.shape.band}
		m.footer.assemble(m.actions, m.evenActions)
		if !m.shape.band {
			// Stacked actions belong with the content, inside its padding,
			// and at the bottom of a panel taller than both.
			if m.shape.fill {
				rows = append(rows, ggui.Expanded(m.footer))
			} else {
				rows = append(rows, m.footer)
			}
		}
	}
	m.body = ggui.Column(rows...).Align(ggui.AlignStretch)
	m.styled = ggui.Styled(m.body)
	m.inner = ggui.Box(m.styled)
	if m.footer != nil && m.shape.band {
		m.panel = ggui.Box(ggui.Column(m.inner, m.footer).Align(ggui.AlignStretch))
		return
	}
	m.panel = ggui.Box(m.inner)
}

// modalFooter is a panel's actions. As a band it is the strip at the foot
// of a Dialog: tinted, under a line, as wide as the panel and rounded with
// it. Otherwise it is a Sheet's stack of full-width actions, which keeps to
// the bottom of whatever room it is given.
type modalFooter struct {
	m     *modal
	band  bool
	box   *ggui.BoxWidget
	row   *ggui.RowWidget
	stack *ggui.ColumnWidget
	size  ggui.Size
}

func (f *modalFooter) assemble(actions []ggui.Widget, even bool) {
	if !f.band {
		f.stack = ggui.Column(actions...).Align(ggui.AlignStretch)
		f.box = ggui.Box(f.stack)
		return
	}
	children := make([]ggui.Widget, 0, len(actions)+1)
	if !even {
		children = append(children, ggui.Spacer())
	}
	for _, a := range actions {
		if even {
			a = ggui.Expanded(a)
		}
		children = append(children, a)
	}
	f.row = ggui.Row(children...)
	f.box = ggui.Box(f.row)
}

// Layout implements ggui.Widget.
func (f *modalFooter) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := f.m.theme
	if !f.band {
		f.stack.Gap(t.Space)
		f.size = f.box.Layout(ggui.Constraints{MinW: c.MinW, MaxW: c.MaxW, MaxH: c.MaxH}, env)
		return c.Constrain(f.size)
	}
	f.row.Gap(t.Space)
	f.box.Padding(t.CardPad)
	f.size = f.box.Layout(c, env)
	return f.size
}

// Paint implements ggui.Widget.
func (f *modalFooter) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if !f.band {
		// At the bottom of the room: a sheet's actions sit at its foot.
		dst.Paint(f.box, ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+r.Size.H-f.size.H), ggui.Sz(r.Size.W, f.size.H)))
		return
	}
	t, s := f.m.theme, f.m.shape
	// The tint stays inside the panel's border and follows its bottom
	// corners: a rounded box as tall again as the corners, with only the
	// footer's part of it let through.
	bw := s.borderWidth
	in := ggui.Rct(r.Origin.Add(ggui.Pt(bw, 0)), ggui.Sz(max(r.Size.W-2*bw, 0), max(r.Size.H-bw, 0)))
	radius := max(s.radius-bw, 0)
	tall := ggui.Rct(in.Origin.Sub(ggui.Pt(0, radius)), ggui.Sz(in.Size.W, in.Size.H+radius))
	dst.Clip(in).FillRoundRect(tall, radius, fade(t.Muted, .5))
	dst.FillRect(ggui.Rct(in.Origin, ggui.Sz(in.Size.W, t.BorderWidth)), t.Border)
	dst.Paint(f.box, r)
}

// paintContent dims the window, takes what lands on it, and paints content
// at rect inside a focus trap. owner is the widget the node belongs to, so a
// screen reader and Probe.Find see the dialog or the sheet rather than this.
func (m *modal) paintContent(dst *ggui.Canvas, screen ggui.Size, rect ggui.Rect, scrim color.Color, owner ggui.Semantic, content ggui.Widget) {
	m.rect = rect
	dst.BackdropBlur(ggui.Rect{Size: screen}, 0, modalBlur)
	dst.FillRect(ggui.Rect{Size: screen}, scrim)
	dst.HitPointer(ggui.Rect{Size: screen}, modalScrim{m})
	// Nothing under the scrim can be pointed at, so nothing there chooses
	// the cursor: the panel's own controls ask over it.
	dst.HitCursor(ggui.Rect{Size: screen}, ggui.CursorShapeDefault)
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
	m.paintClose(dst, r)
}

// paintClose paints the close button in the panel's top right corner.
func (m *modal) paintClose(dst *ggui.Canvas, r ggui.Rect) {
	if m.hideClose {
		return
	}
	inset := m.shape.closeInset
	dst.Paint(m.closeButton, ggui.Rct(ggui.Pt(r.Origin.X+r.Size.W-m.closeSize.W-inset, r.Origin.Y+inset), m.closeSize))
}
