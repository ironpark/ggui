package ui

import "github.com/ironpark/ggui/internal/property"

import "github.com/ironpark/ggui"

// MarkerWidget is a muted conversation status, bordered update, or labeled
// separator. Content may contain links/buttons; decorative icons are inert.
type MarkerWidget struct {
	props                 property.Owner
	content               ggui.Widget
	icon                  ggui.Widget
	view                  *ggui.StyledWidget
	variant               string
	contentSize, iconSize ggui.Size
	theme                 ggui.Theme
}

func Marker(content ggui.Widget) *MarkerWidget {
	return &MarkerWidget{content: content, view: ggui.Styled(content)}
}
func (m *MarkerWidget) Icon(w ggui.Widget) *MarkerWidget { m.icon = w; return m }
func (m *MarkerWidget) Separator() *MarkerWidget {
	defer property.Watch(&m.props, &m.variant)()
	m.variant = "separator"
	return m
}
func (m *MarkerWidget) Border() *MarkerWidget {
	defer property.Watch(&m.props, &m.variant)()
	m.variant = "border"
	return m
}
func (m *MarkerWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer m.props.Layout()()
	m.theme = env.Theme()
	m.view.Color(m.theme.MutedFg).Size(14)
	width := c.MaxW
	m.iconSize = ggui.Size{}
	if m.icon != nil {
		m.iconSize = m.icon.Layout(ggui.Loose(ggui.Sz(16, 16)), env.WithText(ggui.TextStyle{Color: m.theme.MutedFg}))
		width = max(0, width-24)
	}
	if m.variant == "separator" {
		width = max(0, width-48)
	}
	m.contentSize = m.view.Layout(ggui.Loose(ggui.Sz(width, c.MaxH)), env)
	h := max(16, max(m.contentSize.H, m.iconSize.H))
	if m.variant == "border" {
		h += 9
	}
	return c.Constrain(ggui.Sz(bounded(c.MaxW, m.contentSize.W+m.iconSize.W+pick(m.icon != nil, 8.0, 0.0)), h))
}
func (m *MarkerWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleStatus}, func(dst *ggui.Canvas) {
		height := r.Size.H - pick(m.variant == "border", 9.0, 0.0)
		w := m.contentSize.W + pick(m.icon != nil, m.iconSize.W+8, 0.0)
		x := r.Origin.X
		if m.variant == "separator" {
			x += (r.Size.W - w) / 2
			cy := r.Origin.Y + height/2
			dst.StrokeLine(ggui.Pt(r.Origin.X, cy), ggui.Pt(max(r.Origin.X, x-12), cy), 1, m.theme.Border)
			dst.StrokeLine(ggui.Pt(min(r.Origin.X+r.Size.W, x+w+12), cy), ggui.Pt(r.Origin.X+r.Size.W, cy), 1, m.theme.Border)
		}
		if m.icon != nil {
			dst.Inert().Paint(m.icon, ggui.Rct(ggui.Pt(x, r.Origin.Y+(height-m.iconSize.H)/2), m.iconSize))
			x += m.iconSize.W + 8
		}
		dst.Paint(m.view, ggui.Rct(ggui.Pt(x, r.Origin.Y+(height-m.contentSize.H)/2), m.contentSize))
		if m.variant == "border" {
			y := r.Origin.Y + r.Size.H - .5
			dst.StrokeLine(ggui.Pt(r.Origin.X, y), ggui.Pt(r.Origin.X+r.Size.W, y), 1, m.theme.Border)
		}
	})
}
