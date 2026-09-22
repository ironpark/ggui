package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// MessageWidget lays out a conversation row independently of its surface.
// The avatar rests beside the content above the footer. End reverses the row
// and aligns its header, footer and a directly contained Bubble to the right.
type MessageWidget struct {
	props                                        property.Owner
	content, avatar                              ggui.Widget
	headerView, footerView                       *ggui.StyledWidget
	end                                          bool
	bodySize, avatarSize, headerSize, footerSize ggui.Size
	width, height                                float64
}

func Message(content ggui.Widget) *MessageWidget             { return &MessageWidget{content: content} }
func (m *MessageWidget) Avatar(w ggui.Widget) *MessageWidget { m.avatar = w; return m }
func (m *MessageWidget) Header(w ggui.Widget) *MessageWidget {
	m.headerView = ggui.Styled(ggui.Padding(w, 0, 12))
	return m
}
func (m *MessageWidget) Footer(w ggui.Widget) *MessageWidget {
	m.footerView = ggui.Styled(ggui.Padding(w, 0, 12))
	return m
}
func (m *MessageWidget) End() *MessageWidget {
	defer property.Watch(&m.props, &m.end)()
	m.end = true
	return m
}
func MessageGroup(messages ...ggui.Widget) *ggui.ColumnWidget {
	return ggui.Column(messages...).Gap(8).Align(ggui.AlignStretch)
}
func (m *MessageWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer m.props.Layout()()
	width := bounded(c.MaxW, 400)
	m.avatarSize = ggui.Size{}
	if m.avatar != nil {
		m.avatarSize = m.avatar.Layout(ggui.Loose(ggui.Sz(min(width, 40), c.MaxH)), env)
		width = max(0, width-m.avatarSize.W-8)
	}

	loose := ggui.Loose(ggui.Sz(width, c.MaxH))
	m.bodySize = m.content.Layout(loose, env.With(messageEndKey, m.end))
	m.headerSize, m.footerSize = ggui.Size{}, ggui.Size{}
	if m.headerView != nil {
		m.headerView.Size(12).Color(uitheme.From(env).MutedFg)
		m.headerSize = m.headerView.Layout(loose, env)
	}
	if m.footerView != nil {
		m.footerView.Size(12).Color(uitheme.From(env).MutedFg)
		m.footerSize = m.footerView.Layout(loose, env)
	}
	h := m.bodySize.H
	if m.headerView != nil {
		h += m.headerSize.H + 10
	}
	h = max(h, m.avatarSize.H)
	if m.footerView != nil {
		h += 10 + m.footerSize.H
	}
	m.width, m.height = width, h
	return c.Constrain(ggui.Sz(bounded(c.MaxW, width+m.avatarSize.W+pick(m.avatar != nil, 8.0, 0.0)), h))
}
func (m *MessageWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup}, func(dst *ggui.Canvas) {
		x, y := r.Origin.X, r.Origin.Y
		if m.avatar != nil && !m.end {
			x += m.avatarSize.W + 8
		}
		if m.headerView != nil {
			hx := x
			if m.end {
				hx += m.width - m.headerSize.W
			}
			dst.Paint(m.headerView, ggui.Rct(ggui.Pt(hx, y), m.headerSize))
			y += m.headerSize.H + 10
		}
		bodyX := x
		if m.end {
			bodyX += m.width - m.bodySize.W
		}
		dst.Paint(m.content, ggui.Rct(ggui.Pt(bodyX, y), m.bodySize))
		avatarBottom := r.Origin.Y + m.height
		if m.footerView != nil {
			avatarBottom -= m.footerSize.H + 10
			fx := x
			if m.end {
				fx += m.width - m.footerSize.W
			}
			dst.Paint(m.footerView, ggui.Rct(ggui.Pt(fx, avatarBottom+10), m.footerSize))
		}
		if m.avatar != nil {
			ax := r.Origin.X
			if m.end {
				ax += r.Size.W - m.avatarSize.W
			}
			dst.Paint(m.avatar, ggui.Rct(ggui.Pt(ax, avatarBottom-m.avatarSize.H), m.avatarSize))
		}
	})
}
