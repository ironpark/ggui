package ui

import (
	"image/color"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// messageEndKey tells a Bubble that the Message around it is aligned to the end.
var messageEndKey = ggui.NewEnvKey[bool]("message end alignment")

// messageGhostKey lets a ghost Bubble tell the Message around it that its
// content has no padding, so the header and footer line up with it.
var messageGhostKey = ggui.NewEnvKey[*bool]("message holds a ghost bubble")

// BubbleWidget is a chat surface. Message supplies avatar, header and footer;
// Bubble supplies content, seven surface variants and optional edge reactions.
// Build one with Bubble.
type BubbleWidget struct {
	props      property.Owner
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
	theme                      *uitheme.Theme
	motion                     time.Duration
	action                     func()
}

// Bubble creates a chat surface around content, on the Primary color. Its
// width follows the content up to four fifths of the row.
func Bubble(content ggui.Widget) *BubbleWidget {
	b := &BubbleWidget{content: ggui.Styled(content), variant: "default"}
	b.box = ggui.Box(b.content)
	b.Role = ggui.RoleButton
	b.AutoKey()
	return b
}

// Secondary draws the bubble on the Secondary surface.
func (b *BubbleWidget) Secondary() *BubbleWidget {
	defer property.Watch(&b.props, &b.variant)()
	b.variant = "secondary"
	return b
}

// Muted draws the bubble on the Muted surface.
func (b *BubbleWidget) Muted() *BubbleWidget {
	defer property.Watch(&b.props, &b.variant)()
	b.variant = "muted"
	return b
}

// Tinted draws the bubble on a pale tint of Primary, or a deep one on a
// dark page.
func (b *BubbleWidget) Tinted() *BubbleWidget {
	defer property.Watch(&b.props, &b.variant)()
	b.variant = "tinted"
	return b
}

// Outline draws the bubble on the page's background inside a border.
func (b *BubbleWidget) Outline() *BubbleWidget {
	defer property.Watch(&b.props, &b.variant)()
	b.variant = "outline"
	return b
}

// Ghost drops the surface, the padding and the width limit: the content
// alone, for a reply that reads as running text.
func (b *BubbleWidget) Ghost() *BubbleWidget {
	defer property.Watch(&b.props, &b.variant)()
	b.variant = "ghost"
	return b
}

// Destructive draws the bubble as an error: destructive text on a wash of
// the same color.
func (b *BubbleWidget) Destructive() *BubbleWidget {
	defer property.Watch(&b.props, &b.variant)()
	b.variant = "destructive"
	return b
}

// End aligns the bubble to the row's end, as a Message's End does for the
// bubbles inside it.
func (b *BubbleWidget) End() *BubbleWidget {
	defer property.Watch(&b.props, &b.end)()
	b.end = true
	return b
}

// Action turns the surface into a named button without swallowing reaction clicks.
func (b *BubbleWidget) Action(name string, fn func()) *BubbleWidget {
	b.SetName(name)
	b.action = fn
	b.Role = ggui.RoleButton
	return b
}

// Link is a link-semantic action. The caller decides how to open the destination.
func (b *BubbleWidget) Link(name string, fn func()) *BubbleWidget {
	b.SetName(name)
	b.action = fn
	b.Role = ggui.RoleLink
	return b
}

// Disabled greys the bubble out and ignores input while v is true.
func (b *BubbleWidget) Disabled(v bool) *BubbleWidget { b.SetInert(v); return b }

// BindDisabled follows r for Disabled without a rebuild.
func (b *BubbleWidget) BindDisabled(r ggui.Readable[bool]) *BubbleWidget { b.BindInert(r); return b }

// Reactions places arbitrary content at the bottom end edge. Leave vertical
// space between rows for the overlap. Use named buttons for interactive reactions.
func (b *BubbleWidget) Reactions(w ggui.Widget) *BubbleWidget {
	b.reactions = w
	b.reactionBox = ggui.Box(w).Pad(2, 6).Radius(100)
	return b
}

// ReactionsTop moves the reactions to the top edge.
func (b *BubbleWidget) ReactionsTop() *BubbleWidget {
	defer property.Watch(&b.props, &b.reactionTop)()
	b.reactionTop = true
	return b
}

// ReactionsStart moves the reactions to the start edge.
func (b *BubbleWidget) ReactionsStart() *BubbleWidget {
	defer property.Watch(&b.props, &b.reactionStart)()
	b.reactionStart = true
	return b
}

// BubbleGroup stacks consecutive bubbles with shadcn's 8px gap.
func BubbleGroup(bubbles ...ggui.Widget) *ggui.ColumnWidget {
	return ggui.Column(bubbles...).Gap(8).Align(ggui.AlignStretch)
}

// luma is how bright c reads, from 0 to 1, as the eye weighs its channels.
func luma(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	return (.2126*float64(r) + .7152*float64(g) + .0722*float64(b)) / 0xffff
}

// tintOf mixes c into base until the mix is about as bright as target: a
// pale wash of a dark color takes little of it, and a deep one of a color
// that is itself dark takes most. It stands in for shadcn's relative color,
// which sets a lightness and keeps a share of the chroma; a mix toward the
// page keeps the hue and thins the chroma with it.
func tintOf(base, c color.Color, target float64) color.Color {
	from, to := luma(base), luma(c)
	amount := .12
	if to != from {
		amount = clamp((target-from)/(to-from), .06, .6)
	}
	return mix(base, c, amount)
}

// colors returns the variant's surface at rest, its text and its border.
func (b *BubbleWidget) colors() (fill, fg, border color.Color) {
	t := b.theme
	dark := isDark(t.Bg)
	switch b.variant {
	case "secondary":
		return t.Secondary, t.SecondaryFg, nil
	case "muted":
		return t.Muted, t.Fg, nil
	case "tinted":
		return tintOf(t.Bg, t.Primary, pick(dark, .18, .91)), t.Fg, nil
	case "outline":
		return t.Bg, t.Fg, t.Border
	case "ghost":
		return nil, t.Fg, nil
	case "destructive":
		return fade(t.Destructive, pick(dark, .2, .1)), t.Destructive, nil
	}
	return t.Primary, t.PrimaryFg, nil
}

// hoverFill is the variant's surface under the pointer, for a bubble with
// an action.
func (b *BubbleWidget) hoverFill() color.Color {
	t := b.theme
	dark := isDark(t.Bg)
	switch b.variant {
	case "secondary":
		return mix(t.Secondary, t.Fg, .05)
	case "muted":
		return mix(t.Muted, t.Fg, .05)
	case "tinted":
		return tintOf(t.Bg, t.Primary, pick(dark, .23, .85))
	case "outline":
		return pick(dark, fade(t.InputBorder, .3), t.Muted)
	case "ghost":
		return pick(dark, fade(t.Muted, .5), t.Muted)
	case "destructive":
		return fade(t.Destructive, pick(dark, .3, .2))
	}
	return t.PrimaryHover
}

// Layout implements Widget.
func (b *BubbleWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer b.props.Layout().EndLayout()
	b.Sync()
	inheritedEnd, _ := env.Get(messageEndKey)
	b.alignedEnd = b.end || inheritedEnd
	b.theme = uitheme.Of(env)
	b.motion = env.Motion(b.theme.MotionFast)
	_, fg, _ := b.colors()
	if b.IsInert() {
		fg = b.theme.Disabled(fg)
	}
	b.content.Size(b.theme.Text.Size).LineHeight(1.625).Color(fg)
	tokens := b.theme.Chat
	maxW := c.MaxW * .8
	if b.variant == "ghost" {
		b.box.Pad(0).Radius(0)
		maxW = c.MaxW
		if held, ok := env.Get(messageGhostKey); ok && held != nil {
			*held = true
		}
	} else {
		b.box.Padding(tokens.BubblePadding).Radius(tokens.BubbleRadius)
	}
	b.bodySize = b.box.Layout(ggui.Loose(ggui.Sz(maxW, c.MaxH)), env)
	if b.reactionBox != nil {
		b.reactionBox.Fill(b.theme.Muted)
		b.reactionSize = b.reactionBox.Layout(ggui.Loose(ggui.Sz(maxW, c.MaxH)), env)
	}
	return c.Constrain(ggui.Sz(bounded(c.MaxW, b.bodySize.W), b.bodySize.H))
}

// bubbleHoverSlot keeps a bubble's hover easing across frames.
var bubbleHoverSlot = ggui.NewSlot[*ggui.Motion]("bubble hover")

// Paint implements Widget.
func (b *BubbleWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	b.Sync()
	body := ggui.Rct(r.Origin, b.bodySize)
	if b.alignedEnd {
		body.Origin.X += max(0, r.Size.W-body.Size.W)
	}
	fill, fg, border := b.colors()
	if b.action != nil {
		// Each variant has its own surface under the pointer, eased into.
		hover := dst.Ease(b.Anchor(body), bubbleHoverSlot, pick(b.Hovered && !b.IsInert(), 1.0, 0.0), b.motion)
		if hover > 0 {
			if over := b.hoverFill(); fill == nil {
				fill = fade(over, hover)
			} else {
				fill = mix(fill, over, hover)
			}
		}
	}
	if b.IsInert() {
		fill, fg = b.theme.Disabled(fill), b.theme.Disabled(fg)
	}
	b.content.Color(fg)
	b.box.Fill(fill).Border(pick(border != nil, b.theme.BorderWidth, 0.0), border)
	if b.action != nil {
		b.Hit(dst, body, b, ggui.CursorShapePointer)
	}
	dst.Paint(b.box, body)
	if b.action != nil {
		controlFocusRing(dst, &b.Interactive, b.theme, body, pick(b.variant == "ghost", 0, b.theme.Chat.BubbleRadius), b.theme.Ring)
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

// HandlePointer implements PointerHandler.
func (b *BubbleWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if b.IsInert() {
		return false
	}
	return b.Pointer(ev, b.action)
}

// HandleKey implements KeyHandler: Space or Enter runs the action.
func (b *BubbleWidget) HandleKey(ev ggui.KeyEvent) {
	b.Keyboard(ev, func() {
		if !b.IsInert() && b.action != nil {
			b.action()
		}
	})
}

// Describe implements ggui.Describer: a bubble with an action is a button
// or a link.
func (b *BubbleWidget) Describe() ggui.Node {
	return ggui.Node{Role: b.Role, Name: b.SemanticName(), Disabled: b.IsInert(), Actions: ggui.ActionFocus | ggui.ActionPress}
}

// Act implements ggui.Actor: a press runs the action.
func (b *BubbleWidget) Act(a ggui.Action) bool {
	if a.Kind != ggui.ActionPress || b.IsInert() || b.action == nil {
		return false
	}
	b.action()
	return true
}
