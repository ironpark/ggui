package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// MessageWidget lays out a conversation row independently of its surface.
// The avatar rests beside the content above the footer. End reverses the row
// and aligns its header, footer and a directly contained Bubble to the right.
// Build one with Message.
type MessageWidget struct {
	props                                        property.Owner
	content, avatar                              ggui.Widget
	headerView, footerView                       *ggui.StyledWidget
	end                                          bool
	ghost                                        bool // a ghost Bubble was laid out inside
	bodySize, avatarSize, headerSize, footerSize ggui.Size
	width, height, inset, gap, lift, avatarGap   float64
}

// Message creates a conversation row around content, usually a Bubble or a
// BubbleGroup.
func Message(content ggui.Widget) *MessageWidget { return &MessageWidget{content: content} }

// Avatar sets the sender's picture beside the content, at its bottom.
func (m *MessageWidget) Avatar(w ggui.Widget) *MessageWidget { m.avatar = w; return m }

// Header sets a row above the content, such as the sender's name, in small
// medium muted text, inset to line up with a bubble's text.
func (m *MessageWidget) Header(w ggui.Widget) *MessageWidget {
	m.headerView = ggui.Styled(w)
	return m
}

// Footer sets a row under the content, such as a delivery status or
// actions, styled and inset as the header is.
func (m *MessageWidget) Footer(w ggui.Widget) *MessageWidget {
	m.footerView = ggui.Styled(w)
	return m
}

// End reverses the row for the reader's own messages.
func (m *MessageWidget) End() *MessageWidget {
	defer property.Watch(&m.props, &m.end)()
	m.end = true
	return m
}

// MessageGroup stacks consecutive messages with shadcn's 8px gap.
func MessageGroup(messages ...ggui.Widget) *ggui.ColumnWidget {
	return ggui.Column(messages...).Gap(8).Align(ggui.AlignStretch)
}

// Layout implements Widget.
func (m *MessageWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer m.props.Layout().EndLayout()
	t := uitheme.Of(env)
	m.gap, m.avatarGap = t.Space*1.25, t.Space
	width := bounded(c.MaxW, 400)
	m.avatarSize = ggui.Size{}
	if m.avatar != nil {
		m.avatarSize = m.avatar.Layout(ggui.Loose(ggui.Sz(min(width, 40), c.MaxH)), env)
		width = max(0, width-m.avatarSize.W-t.Space)
	}

	loose := ggui.Loose(ggui.Sz(width, c.MaxH))
	m.bodySize = m.content.Layout(loose, env.With(messageEndKey, m.end).With(messageGhostKey, &m.ghost))
	if b, ok := m.content.(*BubbleWidget); ok {
		// A bubble held directly says so itself; one inside a group reports
		// through the Env when it is laid out, which a cached layout skips,
		// so what it reported stands.
		m.ghost = b.variant == "ghost"
	}
	// The header and footer start where a bubble's text does, and at the
	// edge beside a ghost bubble, which has no padding.
	m.inset = pick(m.ghost, 0, t.Space*1.5)
	m.headerSize, m.footerSize = ggui.Size{}, ggui.Size{}
	row := ggui.Loose(ggui.Sz(max(0, width-2*m.inset), c.MaxH))
	if m.headerView != nil {
		m.headerView.Style(t.Caption).Weight(ggui.WeightMedium)
		m.headerSize = m.headerView.Layout(row, env)
	}
	if m.footerView != nil {
		m.footerView.Style(t.Caption).Weight(ggui.WeightMedium)
		m.footerSize = m.footerView.Layout(row, env)
	}
	h := m.bodySize.H
	if m.headerView != nil {
		h += m.headerSize.H + m.gap
	}
	h = max(h, m.avatarSize.H)
	m.lift = 0
	if m.footerView != nil {
		h += m.gap + m.footerSize.H
		// The avatar rises four spaces off the row's bottom, clear of a
		// footer one line tall, and further for a taller one.
		m.lift = max(4*t.Space, m.footerSize.H+m.gap)
	}
	m.width, m.height = width, h
	return c.Constrain(ggui.Sz(bounded(c.MaxW, width+m.avatarSize.W+pick(m.avatar != nil, t.Space, 0.0)), h))
}

// Paint implements Widget.
func (m *MessageWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup}, func(dst *ggui.Canvas) {
		x, y := r.Origin.X, r.Origin.Y
		if m.avatar != nil && !m.end {
			x += m.avatarSize.W + m.avatarGap
		}
		if m.headerView != nil {
			hx := x + m.inset
			if m.end {
				hx = x + m.width - m.inset - m.headerSize.W
			}
			dst.Paint(m.headerView, ggui.Rct(ggui.Pt(hx, y), m.headerSize))
			y += m.headerSize.H + m.gap
		}
		bodyX := x
		if m.end {
			bodyX += m.width - m.bodySize.W
		}
		dst.Paint(m.content, ggui.Rct(ggui.Pt(bodyX, y), m.bodySize))
		bottom := r.Origin.Y + m.height
		if m.footerView != nil {
			fx := x + m.inset
			if m.end {
				fx = x + m.width - m.inset - m.footerSize.W
			}
			dst.Paint(m.footerView, ggui.Rct(ggui.Pt(fx, bottom-m.footerSize.H), m.footerSize))
		}
		if m.avatar != nil {
			ax := r.Origin.X
			if m.end {
				ax += r.Size.W - m.avatarSize.W
			}
			dst.Paint(m.avatar, ggui.Rct(ggui.Pt(ax, max(r.Origin.Y, bottom-m.lift-m.avatarSize.H)), m.avatarSize))
		}
	})
}
