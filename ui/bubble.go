package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
	"image/color"
)

// BubbleWidget is a chat surface. Message supplies avatar, header and footer;
// Bubble supplies content, seven surface variants and optional edge reactions.
var messageEndKey = ggui.NewEnvKey[bool]("message end alignment")

type BubbleWidget struct {
	alignedEnd bool
	ggui.Interactive
	content                    *ggui.StyledWidget
	box                        *ggui.BoxWidget
	variant                    string
	end                        bool
	reactions                  ggui.Widget
	reactionBox                *ggui.BoxWidget
	reactionTop, reactionStart bool
	bodySize, reactionSize     ggui.Size
	theme                      ggui.Theme
	action                     func()
}

func Bubble(content ggui.Widget) *BubbleWidget {
	b := &BubbleWidget{content: ggui.Styled(content), variant: "default"}
	b.box = ggui.Box(b.content)
	b.Role = ggui.RoleButton
	b.AutoKey()
	return b
}
func (b *BubbleWidget) Secondary() *BubbleWidget   { b.variant = "secondary"; return b }
func (b *BubbleWidget) Muted() *BubbleWidget       { b.variant = "muted"; return b }
func (b *BubbleWidget) Tinted() *BubbleWidget      { b.variant = "tinted"; return b }
func (b *BubbleWidget) Outline() *BubbleWidget     { b.variant = "outline"; return b }
func (b *BubbleWidget) Ghost() *BubbleWidget       { b.variant = "ghost"; return b }
func (b *BubbleWidget) Destructive() *BubbleWidget { b.variant = "destructive"; return b }
func (b *BubbleWidget) End() *BubbleWidget         { b.end = true; return b }

// Action turns the surface into a named button without swallowing reaction clicks.
func (b *BubbleWidget) Action(name string, fn func()) *BubbleWidget {
	b.Name, b.action, b.Role = name, fn, ggui.RoleButton
	return b
}

// Link is a link-semantic action. The caller decides how to open the destination.
func (b *BubbleWidget) Link(name string, fn func()) *BubbleWidget {
	b.Name, b.action, b.Role = name, fn, ggui.RoleLink
	return b
}
func (b *BubbleWidget) Disabled(v bool) *BubbleWidget                  { b.SetInert(v); return b }
func (b *BubbleWidget) DisabledWhen(r ggui.Reader[bool]) *BubbleWidget { b.InertWhen(r); return b }

// Reactions places arbitrary content at the bottom end edge. Leave vertical
// space between rows for the overlap. Use named buttons for interactive reactions.
func (b *BubbleWidget) Reactions(w ggui.Widget) *BubbleWidget {
	b.reactions = w
	b.reactionBox = ggui.Box(w).Pad(2, 6).Radius(100)
	return b
}
func (b *BubbleWidget) ReactionsTop() *BubbleWidget   { b.reactionTop = true; return b }
func (b *BubbleWidget) ReactionsStart() *BubbleWidget { b.reactionStart = true; return b }

// BubbleGroup stacks consecutive bubbles with shadcn's 8px gap.
func BubbleGroup(bubbles ...ggui.Widget) *ggui.ColumnWidget {
	return ggui.Column(bubbles...).Gap(8).Align(ggui.AlignStretch)
}
func (b *BubbleWidget) colors() (fill, fg, border color.Color) {
	t := b.theme
	switch b.variant {
	case "secondary":
		return t.Secondary, t.SecondaryFg, nil
	case "muted":
		return t.Muted, t.Fg, nil
	case "tinted":
		return mix(t.Card, t.Primary, .12), t.Fg, nil
	case "outline":
		return t.Bg, t.Fg, t.Border
	case "ghost":
		return nil, t.Fg, nil
	case "destructive":
		return mix(t.Card, t.Destructive, .12), t.Destructive, nil
	}
	return t.Primary, t.PrimaryFg, nil
}
func (b *BubbleWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	b.Sync()
	inheritedEnd, _ := env.Get(messageEndKey)
	b.alignedEnd = b.end || inheritedEnd
	b.theme = env.Theme()
	_, fg, _ := b.colors()
	if b.Inert {
		fg = mix(fg, b.theme.Card, b.theme.DisabledMix)
	}
	b.content.Size(14).LineHeight(1.625).Color(fg)
	tokens := b.theme.ChatTokens()
	b.box.Padding(tokens.BubblePadding).Radius(tokens.BubbleRadius)
	maxW := c.MaxW * .8
	if b.variant == "ghost" {
		b.box.Pad(0).Radius(0)
		maxW = c.MaxW
	}
	b.bodySize = b.box.Layout(ggui.Loose(ggui.Sz(maxW, c.MaxH)), env)
	if b.reactionBox != nil {
		b.reactionBox.Fill(b.theme.Muted)
		b.reactionSize = b.reactionBox.Layout(ggui.Loose(ggui.Sz(maxW, c.MaxH)), env)
	}
	return c.Constrain(ggui.Sz(bounded(c.MaxW, b.bodySize.W), b.bodySize.H))
}
func (b *BubbleWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	b.Sync()
	body := ggui.Rct(r.Origin, b.bodySize)
	if b.alignedEnd {
		body.Origin.X += max(0, r.Size.W-body.Size.W)
	}
	fill, fg, border := b.colors()
	if b.action != nil && b.Hovered && !b.Inert {
		if fill == nil {
			fill = b.theme.Muted
		} else {
			fill = mix(fill, b.theme.Fg, .05)
		}
	}
	if b.Inert {
		if fill != nil {
			fill = mix(fill, b.theme.Card, b.theme.DisabledMix)
		}
		fg = mix(fg, b.theme.Card, b.theme.DisabledMix)
	}
	b.content.Color(fg)
	b.box.Fill(fill).Border(pick(border != nil, 1.0, 0.0), border)
	if b.action != nil {
		b.Hit(dst, body, b, ebiten.CursorShapePointer)
	}
	dst.Paint(b.box, body)
	if b.action != nil {
		b.FocusRing(dst, body, b.theme.ChatTokens().BubbleRadius, b.theme.Ring)
	}
	if b.reactionBox != nil {
		x := body.Origin.X + body.Size.W - b.reactionSize.W - 12
		if b.reactionStart {
			x = body.Origin.X + 12
		}
		x = max(r.Origin.X, min(x, r.Origin.X+r.Size.W-b.reactionSize.W))
		y := body.Origin.Y + body.Size.H - b.reactionSize.H*.25
		if b.reactionTop {
			y = body.Origin.Y - b.reactionSize.H*.75
		}
		reaction := ggui.Rct(ggui.Pt(x, y), b.reactionSize)
		// CSS rings sit outside the pill instead of consuming its content padding.
		dst.FillRoundRect(ggui.Rct(reaction.Origin.Add(ggui.Pt(-3, -3)), ggui.Sz(reaction.Size.W+6, reaction.Size.H+6)), 100, b.theme.Card)
		dst.Paint(b.reactionBox, reaction)
	}
}
func (b *BubbleWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if b.Inert {
		return false
	}
	return b.Pointer(ev, b.action)
}
func (b *BubbleWidget) HandleKey(ev ggui.KeyEvent) {
	b.Keyboard(ev, func() {
		if !b.Inert && b.action != nil {
			b.action()
		}
	})
}
func (b *BubbleWidget) Describe() ggui.Node {
	return ggui.Node{Role: b.Role, Name: b.Name, Disabled: b.Inert, Actions: ggui.ActionFocus | ggui.ActionPress}
}
func (b *BubbleWidget) Act(a ggui.Action) bool {
	if a.Kind != ggui.ActionPress || b.Inert || b.action == nil {
		return false
	}
	b.action()
	return true
}
