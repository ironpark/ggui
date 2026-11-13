package ui

import (
	"image/color"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// ButtonStyle is the look of a button variant under a theme.
type ButtonStyle struct {
	Fill, Hover       color.Color // the surface at rest and under the pointer; nil draws none
	Label, HoverLabel color.Color // the label at rest and under the pointer; a nil HoverLabel keeps Label
	Border            color.Color // a line just inside the edge; nil draws none
	Ring              color.Color // the focus ring; nil is the theme's Ring
	FocusBorder       color.Color // the line at the edge while focused; nil is Ring
	Shadow            bool        // rests on the theme's CardShadow
	Underline         bool        // underlines the label under the pointer, as a link does
}

// ButtonVariant is a look for Button.Variant.
type ButtonVariant = Variant[ButtonStyle]

// The button variants of shadcn/ui. Restyle one to change every button
// drawn with it under a theme, or make more with NewVariant.
var (
	// ButtonPrimary is the main action: the Primary surface.
	ButtonPrimary = NewVariant("primary", func(t uitheme.Theme) ButtonStyle {
		return ButtonStyle{Fill: t.Primary, Hover: t.PrimaryHover, Label: t.PrimaryFg}
	})
	// ButtonOutline is a border around the page's background.
	ButtonOutline = NewVariant("outline", func(t uitheme.Theme) ButtonStyle {
		fill, hover, border := t.Bg, t.Muted, t.Border
		if isDark(t.Bg) {
			fill, hover, border = fade(t.InputBorder, .3), fade(t.InputBorder, .5), t.InputBorder
		}
		return ButtonStyle{Fill: fill, Hover: hover, Label: t.Fg, HoverLabel: t.Fg, Border: border}
	})
	// ButtonSecondary is a supporting action on the Secondary surface.
	ButtonSecondary = NewVariant("secondary", func(t uitheme.Theme) ButtonStyle {
		return ButtonStyle{Fill: t.Secondary, Hover: mix(t.Secondary, t.Fg, .05), Label: t.SecondaryFg}
	})
	// ButtonGhost has no surface until the pointer is over it.
	ButtonGhost = NewVariant("ghost", func(t uitheme.Theme) ButtonStyle {
		hover := t.Muted
		if isDark(t.Bg) {
			hover = fade(hover, .5)
		}
		return ButtonStyle{Hover: hover, Label: t.Fg, HoverLabel: t.Fg}
	})
	// ButtonDestructive is an irreversible action: the Destructive surface,
	// toned down on a dark page as shadcn does.
	ButtonDestructive = NewVariant("destructive", func(t uitheme.Theme) ButtonStyle {
		opacity := .1
		if isDark(t.Bg) {
			opacity = .2
		}
		return ButtonStyle{Fill: fade(t.Destructive, opacity), Hover: fade(t.Destructive, opacity+.1), Label: t.Destructive, Ring: destructiveRing(&t), FocusBorder: fade(t.Destructive, .4)}
	})
	// ButtonLink is a label in the Primary color, underlined under the
	// pointer.
	ButtonLink = NewVariant("link", func(t uitheme.Theme) ButtonStyle {
		return ButtonStyle{Label: t.Primary, Underline: true}
	})
)

// ButtonSize is the geometry of a button size variant.
type ButtonSize struct {
	Pad        ggui.EdgeInsets
	Height     float64 // minimum height; zero measures the content
	TextSize   float64 // optional label size
	LineHeight float64 // optional label line height
	Square     bool    // as wide as it is tall, for a button holding one icon
	Radius     float64 // corner radius; zero is the theme's Radius, which a ButtonGroup always uses
	Gap        float64 // between an inline icon and the label; zero is three quarters of the theme's Space
	IconSize   float64 // the side of an inline icon; zero is the theme's IconSize
	IconPad    float64 // horizontal padding on a side holding an inline icon; zero keeps Pad's
}

// ButtonSizeVariant is a size for Button.Size.
type ButtonSizeVariant = Variant[ButtonSize]

// The button sizes of shadcn/ui, from the theme's ButtonPad, Text size,
// Space, IconSize and radii.
var (
	ButtonDefault = NewVariant("default", func(t uitheme.Theme) ButtonSize {
		return ButtonSize{Pad: t.ButtonPad, Height: 20 + 2*t.ButtonPad.Top, Gap: t.Space * .75, IconSize: t.IconSize, IconPad: t.ButtonPad.Left - 2}
	})
	ButtonExtraSmall = NewVariant("xs", func(t uitheme.Theme) ButtonSize {
		size := buttonExtraSmall(t)
		size.Pad, size.IconPad = ggui.Insets(t.ButtonPad.Top-2, t.ButtonPad.Left*.8), t.ButtonPad.Left*.8-2
		return size
	})
	ButtonSmall = NewVariant("sm", func(t uitheme.Theme) ButtonSize {
		size := buttonSmall(t)
		size.Pad, size.IconPad = ggui.Insets(t.ButtonPad.Top-2, t.ButtonPad.Left), t.ButtonPad.Left-4
		return size
	})
	ButtonLarge = NewVariant("lg", func(t uitheme.Theme) ButtonSize {
		return ButtonSize{Pad: ggui.Insets(t.ButtonPad.Top+2, t.ButtonPad.Left), Height: 24 + 2*t.ButtonPad.Top, Gap: t.Space * .75, IconSize: t.IconSize, IconPad: t.ButtonPad.Left - 2}
	})
	ButtonIcon = NewVariant("icon", func(t uitheme.Theme) ButtonSize {
		return ButtonSize{Height: 20 + 2*t.ButtonPad.Top, Square: true, Gap: t.Space * .75, IconSize: t.IconSize}
	})
	ButtonIconExtraSmall = NewVariant("icon-xs", func(t uitheme.Theme) ButtonSize {
		size := buttonExtraSmall(t)
		size.Square, size.TextSize, size.LineHeight = true, 0, 0
		return size
	})
	ButtonIconSmall = NewVariant("icon-sm", func(t uitheme.Theme) ButtonSize {
		size := buttonSmall(t)
		size.Square, size.TextSize, size.LineHeight = true, 0, 0
		size.IconSize = t.IconSize // shadcn shrinks the icon of sm, not of icon-sm
		return size
	})
	ButtonIconLarge = NewVariant("icon-lg", func(t uitheme.Theme) ButtonSize {
		return ButtonSize{Height: 24 + 2*t.ButtonPad.Top, Square: true, Gap: t.Space * .75, IconSize: t.IconSize}
	})
)

// buttonExtraSmall is what the xs text and icon sizes share: shadcn's h-6,
// text-xs, gap-1, size-3 icons and rounded-[min(var(--radius-md),10px)].
func buttonExtraSmall(t uitheme.Theme) ButtonSize {
	return ButtonSize{
		Height: 12 + 2*t.ButtonPad.Top, TextSize: t.Text.Size * 12 / 14, LineHeight: 16.0 / 12,
		Radius: min(t.RadiusSm, 10), Gap: t.Space / 2, IconSize: t.IconSize * .75,
	}
}

// buttonSmall is what the sm text and icon sizes share: shadcn's h-7,
// text-[0.8rem], gap-1, size-3.5 icons and rounded-[min(var(--radius-md),12px)].
func buttonSmall(t uitheme.Theme) ButtonSize {
	return ButtonSize{
		Height: 16 + 2*t.ButtonPad.Top, TextSize: t.Text.Size * 12.8 / 14, LineHeight: 20.0 / 12.8,
		Radius: min(t.RadiusSm, 12), Gap: t.Space / 2, IconSize: t.IconSize * .875,
	}
}

// ButtonWidget is a clickable box with a label. Build one with Button.
type ButtonWidget struct {
	toggleValue func() bool
	props       property.Owner
	ggui.Interactive
	label       *ggui.TextWidget
	box         *ggui.BoxWidget
	content     *buttonContent
	onTap       func()
	defaultName string
	variant     *ButtonVariant
	size        *ButtonSizeVariant
	style       ButtonStyle
	padded      bool
	pad         ggui.EdgeInsets
	radius      float64 // set by Radius; negative follows the size and the theme
	corner      float64 // the radius drawn, from the last Layout
	sides       JoinedSides
	selected    bool // current item for composite navigation controls
	expands     func() bool
	opener      ggui.Actor
	value       func() string // optional accessible value for composite triggers
	motion      time.Duration
	theme       *uitheme.Theme
	chord       ggui.Chord
	chorded     bool
}

// Button creates a primary button: the Primary surface with a PrimaryFg
// label.
func Button(label string, onTap func()) *ButtonWidget {
	b := &ButtonWidget{onTap: onTap, label: ggui.Text(label).NoWrap(), variant: ButtonPrimary, size: ButtonDefault, radius: -1}
	b.content = &buttonContent{child: b.label}
	b.box = ggui.Box(b.content)
	b.Role = ggui.RoleButton
	b.SetName(label)
	b.AutoKey()
	return b
}

// ButtonOf creates a button around any content instead of a text label.
// Give it a name with Name, since nothing on it says what it is. An icon
// left at the theme's IconSize is drawn at the size variant's, smaller in
// an extra small button.
func ButtonOf(child ggui.Widget, onTap func()) *ButtonWidget {
	b := &ButtonWidget{onTap: onTap, content: &buttonContent{child: child}, variant: ButtonPrimary, size: ButtonDefault, radius: -1}
	b.box = ggui.Box(b.content)
	b.Role = ggui.RoleButton
	b.AutoKey()
	return b
}

// Icon puts an icon before the label, at the size variant's icon size and
// gap and tinted as the label is, and narrows the padding on that side as
// shadcn/ui does for an inline-start icon. Nil removes it. For a button
// holding nothing but an icon use ButtonOf with one of the ButtonIcon sizes.
//
//	ui.Button("New branch", create).Outline().Icon(ui.Icon(icons.Plus))
func (b *ButtonWidget) Icon(icon *icons.Widget) *ButtonWidget {
	defer property.Watch(&b.props, &b.content.lead)()
	b.content.lead = icon
	return b
}

// TrailingIcon puts an icon after the label, as Icon puts one before it.
func (b *ButtonWidget) TrailingIcon(icon *icons.Widget) *ButtonWidget {
	defer property.Watch(&b.props, &b.content.trail)()
	b.content.trail = icon
	return b
}

// Name names the button for Probe.Find and the inspector; Button takes
// its text, ButtonOf needs one.
func (b *ButtonWidget) Name(s string) *ButtonWidget { b.SetName(s); return b }

// BindName binds the button's name to r; see ggui.Interactive.BindName.
func (b *ButtonWidget) BindName(r ggui.Readable[string]) *ButtonWidget {
	b.Interactive.BindName(r)
	return b
}

// Shortcut presses the button on chord, such as "cmd+z", while the button
// is on screen and enabled; see ggui.ParseChord for the names, and
// ggui.Canvas.Shortcut for when it runs. A Tooltip around the button shows
// the chord. It panics on a chord ParseChord rejects.
func (b *ButtonWidget) Shortcut(chord string) *ButtonWidget {
	b.chord, b.chorded = ggui.MustChord(chord), true
	return b
}

// ShortcutChord returns the chord Shortcut set, if any.
func (b *ButtonWidget) ShortcutChord() (ggui.Chord, bool) { return b.chord, b.chorded }

// Expands makes the button report whether what it opens is showing, for a
// menu button or a combobox trigger; a plain button does not expand at all,
// which is not the same as being closed.
func (b *ButtonWidget) Expands(open func() bool) *ButtonWidget { b.expands = open; return b }

// Opens hands the expand and collapse actions to the widget that owns the
// popup, since the button describes the node but does not hold it.
func (b *ButtonWidget) Opens(a ggui.Actor) *ButtonWidget {
	defer property.Watch(&b.props, &b.opener)()
	b.opener = a
	return b
}

// Act implements ggui.Actor.
func (b *ButtonWidget) Act(a ggui.Action) bool {
	if b.IsInert() {
		return false
	}
	if b.opener != nil && b.opener.Act(a) {
		return true
	}
	if a.Kind == ggui.ActionPress && b.onTap != nil {
		b.onTap()
		return true
	}
	return false
}

// Describe implements ggui.Describer.
func (b *ButtonWidget) Describe() ggui.Node {
	n := ggui.Node{
		Role:     b.Role,
		Name:     b.name(),
		Disabled: b.IsInert(),
		Selected: b.selected,
		Actions:  ggui.ActionPress | ggui.ActionFocus,
	}
	if b.toggleValue != nil {
		n.Checked = ggui.Tri(b.toggleValue())
		n.Selected = b.toggleValue()
	}
	if b.expands != nil {
		open := b.expands()
		n.Expanded = ggui.Expandable(open)
		n.Actions |= pick(open, ggui.ActionCollapse, ggui.ActionExpand)
	}
	if b.value != nil {
		n.Value = b.value()
	}
	return n
}

// Variant draws the button in v, one of the Button variables such as
// ButtonOutline or a variant of your own; see NewVariant.
func (b *ButtonWidget) Variant(v *ButtonVariant) *ButtonWidget {
	defer property.Watch(&b.props, &b.variant)()
	b.variant = v
	return b
}

// Outline draws the button as a border around the window's background, with
// the normal text color, for actions that are not the main one.
func (b *ButtonWidget) Outline() *ButtonWidget { return b.Variant(ButtonOutline) }

// Secondary fills the button with the theme's Secondary surface, for a
// supporting action that should still read as a button.
func (b *ButtonWidget) Secondary() *ButtonWidget { return b.Variant(ButtonSecondary) }

// Ghost omits the resting background and border for a lightweight action.
func (b *ButtonWidget) Ghost() *ButtonWidget { return b.Variant(ButtonGhost) }

// Destructive uses the theme's Destructive color for an irreversible action.
func (b *ButtonWidget) Destructive() *ButtonWidget { return b.Variant(ButtonDestructive) }

// Link draws the button as a Primary-colored label, underlined under the
// pointer.
func (b *ButtonWidget) Link() *ButtonWidget { return b.Variant(ButtonLink) }

// Size sets a text or icon size variant, or a size of your own.
// ButtonExtraSmall, ButtonSmall and ButtonLarge size text actions;
// ButtonIcon and its ExtraSmall, Small and Large variants are square.
// Pad overrides the size variant's padding.
func (b *ButtonWidget) Size(v *ButtonSizeVariant) *ButtonWidget {
	defer property.Watch(&b.props, &b.size)()
	b.size = v
	return b
}

// Radius overrides the corner radius of the size variant and the theme for
// this button.
func (b *ButtonWidget) Radius(r float64) *ButtonWidget {
	defer property.Watch(&b.props, &b.radius)()
	b.radius = max(r, 0)
	return b
}

// Disabled greys the button out and ignores the pointer while v is true.
func (b *ButtonWidget) Disabled(v bool) *ButtonWidget { b.SetInert(v); return b }

// BindDisabled follows r for Disabled without a rebuild.
func (b *ButtonWidget) BindDisabled(r ggui.Readable[bool]) *ButtonWidget { b.BindInert(r); return b }

// Pad overrides the theme's padding, with the shorthand Insets accepts.
func (b *ButtonWidget) Pad(sides ...float64) *ButtonWidget {
	defer property.Watch(&b.props, &b.padded)()
	b.pad = ggui.Insets(sides...)
	b.box.Padding(b.pad)
	b.padded = true
	return b
}

// Layout implements Widget.
func (b *ButtonWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer b.props.Layout().EndLayout()
	b.Sync()
	t := uitheme.Of(env)
	b.theme = t
	b.motion = env.Motion(t.MotionFast)
	size := b.size.Style(*t)
	toggle := b.toggleValue != nil
	b.sides, env = takeJoinedSides(env)
	// A size's own radius gives way inside a ButtonGroup, whose strip is
	// rounded as a default button is whatever the size of its children.
	b.corner = pick(size.Radius > 0 && !b.sides.Any(), size.Radius, t.Radius)
	if b.radius >= 0 {
		b.corner = b.radius
	}
	b.box.Radius(b.corner)
	b.style = b.variant.Style(*t)
	label := b.style.Label
	if b.IsInert() {
		label = t.Disabled(label)
	}
	if b.label != nil {
		b.label.Color(label) // Paint swaps in HoverLabel
	}
	textStyle := t.Text.Merge(t.Label).Merge(ggui.TextStyle{Color: label})
	if size.TextSize > 0 {
		textStyle.Size, textStyle.LineHeight = size.TextSize, size.LineHeight
	}
	contentEnv := env.WithText(textStyle)
	ct := b.content
	ct.rtl = env.Direction() == ggui.DirRTL
	// A toggle keeps shadcn's gap-1 at every size.
	ct.gap = pick(size.Gap > 0 && !toggle, size.Gap, pick(toggle, t.Space/2, t.Space*.75))
	ct.iconSize = pick(size.IconSize > 0, size.IconSize, t.IconSize)
	ct.themeIcon = t.IconSize
	pad := b.pad
	if !b.padded {
		pad = size.Pad
		if size.IconPad > 0 {
			left, right := ct.lead != nil, ct.trail != nil
			if ct.rtl {
				left, right = right, left
			}
			if left {
				pad.Left = min(pad.Left, size.IconPad)
			}
			if right {
				pad.Right = min(pad.Right, size.IconPad)
			}
		}
		// CSS heights include the line box. Center the native font metrics in
		// that height instead of adding web line-height padding a second time.
		if b.label != nil && size.Height > 0 && env.TextScale() <= 1 {
			content := ct.Layout(pad.Shrink(c).Loosen(), contentEnv)
			if content.H <= size.Height {
				pad.Top, pad.Bottom = (size.Height-content.H)/2, (size.Height-content.H)/2
			}
		}
		b.pad = pad
		b.box.Padding(pad)
		c.MinH = min(c.MaxH, max(c.MinH, size.Height))
		if toggle {
			// shadcn's min-w-8: a toggle is never narrower than it is tall.
			c.MinW = min(c.MaxW, max(c.MinW, size.Height))
		}
	}
	b.box.Size(0, 0)

	out := b.box.Layout(c, contentEnv)
	b.box.Size(out.W, out.H)
	out = b.box.Layout(ggui.Tight(out), contentEnv)
	if size.Square && !b.padded {
		side := c.Constrain(ggui.Sz(max(out.W, out.H), max(out.W, out.H)))
		b.box.Size(side.W, side.H)
		out = b.box.Layout(ggui.Tight(side), contentEnv)
	}
	return out
}

// Baseline implements ggui.Baseliner: the label's, inside the padding.
func (b *ButtonWidget) Baseline() (float64, bool) { return b.box.Baseline() }

// Paint implements Widget.
func (b *ButtonWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t, st := b.theme, b.style
	inert := b.IsInert()
	fill, label := st.Fill, st.Label
	toggle := b.toggleValue != nil
	if toggle && b.toggleValue() {
		fill, label = t.Muted, t.Fg
	}
	// An open outline or ghost trigger reads as hovered; a secondary one
	// keeps its resting surface, as shadcn's aria-expanded:bg-secondary does.
	expanded := b.expands != nil && b.expands() && (b.variant == ButtonOutline || b.variant == ButtonGhost)
	hover := dst.Ease(b.Anchor(r), buttonHoverSlot, pick((b.Hovered || expanded) && !inert, 1.0, 0.0), b.motion)
	if hover >= 1 {
		fill = st.Hover
	} else if hover > 0 {
		fill = mix(colorOr(fill, color.Transparent), colorOr(st.Hover, color.Transparent), hover)
	}
	if st.HoverLabel != nil && hover > 0 {
		label = mix(label, st.HoverLabel, hover)
	}

	if inert {
		fill, label = t.Disabled(fill), t.Disabled(label)
	}
	if b.label != nil {
		b.label.Color(label)
	}
	b.content.tint(label)
	border := st.Border
	if border != nil && inert {
		border = t.Disabled(border)
	}
	radius := b.corner
	sides := b.sides.within(dst)

	b.Hit(dst, r, b, pick(b.variant == ButtonLink, ggui.CursorShapePointer, ggui.CursorShapeDefault))
	if b.chorded && !inert && b.onTap != nil {
		dst.Shortcut(b.chord, b.onTap)
	}
	// A menu trigger and a toggle stay put: shadcn nudges neither.
	if b.Pressed && b.Hovered && !inert && b.expands == nil && !toggle {
		r.Origin.Y++
	}
	if st.Shadow && !inert {
		dst.Shadow(r, radius, t.CardShadow)
	}
	if sides.Any() {
		// A box rounds every corner alike, so the surface of a button joined
		// to a neighbour is drawn here and the box draws the content alone.
		b.box.Fill(nil).Border(0, nil)
		paintJoinedBox(dst, r, radius, sides, fill, t.BorderWidth, border)
	} else {
		b.box.Fill(fill).Border(pick(border != nil, t.BorderWidth, 0), border)
	}
	dst.Paint(b.box, r)
	if st.Underline && hover > 0 {
		if base, ok := b.box.Baseline(); ok {
			pad := b.pad
			y := r.Origin.Y + base + 2
			dst.FillRect(ggui.Rct(ggui.Pt(r.Origin.X+pad.Left, y), ggui.Sz(r.Size.W-pad.Left-pad.Right, 1)), fade(label, hover))
		}
	}
	if b.Focused && b.FocusVisible && !inert {
		ring := colorOr(st.Ring, t.Ring)
		joinedFocusRing(dst, t, r, radius, sides, colorOr(st.FocusBorder, ring), ring)
	}
}

var buttonHoverSlot = ggui.NewSlot[*ggui.Motion]("button hover")

// HandleKey implements KeyHandler: Space or Enter presses the button.
func (b *ButtonWidget) HandleKey(ev ggui.KeyEvent) { b.Keyboard(ev, b.onTap) }

// HandlePointer implements PointerHandler.
func (b *ButtonWidget) HandlePointer(ev ggui.PointerEvent) bool { return b.Pointer(ev, b.onTap) }

func (b *ButtonWidget) name() string {
	return pick(b.SemanticName() != "", b.SemanticName(), b.defaultName)
}

// Semantics implements ggui.Semantic, including a composite trigger's fallback.
func (b *ButtonWidget) Semantics() (ggui.Role, string) { return b.Role, b.name() }

// buttonContent is what a button holds: its label or child, with an icon
// before and after it when Icon and TrailingIcon set them. It centers them
// without consuming loose space like a layout Align does.
type buttonContent struct {
	child       ggui.Widget
	lead, trail *icons.Widget
	gap         float64 // between an icon and the child
	iconSize    float64
	themeIcon   float64 // the theme's IconSize, which an icon left at its own size has
	rtl         bool
	size        ggui.Size // the child's
	all         ggui.Size // the child and the icons
	height      float64
}

func (b *buttonContent) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	inner, beside := c.Loosen(), 0.0
	for _, ic := range [...]*icons.Widget{b.lead, b.trail} {
		if ic != nil {
			ic.Layout(ggui.Tight(ggui.Sz(b.iconSize, b.iconSize)), env)
			beside += b.iconSize + b.gap
		}
	}
	inner.MaxW = max(inner.MaxW-beside, 0)
	b.size = b.child.Layout(inner, env)
	// An icon that is all a button holds follows the size variant too,
	// unless it was given a size of its own: shadcn's size-3 in an xs button.
	if _, bare := b.child.(*icons.Widget); bare && b.size == ggui.Sz(b.themeIcon, b.themeIcon) {
		b.size = ggui.Sz(b.iconSize, b.iconSize)
	}
	b.all = b.size
	if beside > 0 {
		if b.size.W == 0 {
			beside -= b.gap // nothing to keep the last icon away from
		}
		b.all = ggui.Sz(b.size.W+beside, max(b.size.H, b.iconSize))
	}
	out := c.Constrain(b.all)
	b.height = out.H
	return out
}

func (b *buttonContent) Paint(dst *ggui.Canvas, r ggui.Rect) {
	x := r.Origin.X + (r.Size.W-b.all.W)/2
	icon := ggui.Sz(b.iconSize, b.iconSize)
	if b.lead != nil {
		x = b.place(dst, r, b.lead, x, icon)
	}
	if b.size.W > 0 {
		x = b.place(dst, r, b.child, x, b.size)
	}
	if b.trail != nil {
		b.place(dst, r, b.trail, x, icon)
	}
}

// place paints w at x, centered on the line, and returns where the next
// part starts.
func (b *buttonContent) place(dst *ggui.Canvas, r ggui.Rect, w ggui.Widget, x float64, size ggui.Size) float64 {
	at := ggui.Rct(ggui.Pt(x, r.Origin.Y+(r.Size.H-size.H)/2), size)
	dst.Paint(w, mirrorHorizontal(at, r, b.rtl))
	return x + size.W + b.gap
}

// tint draws the icons in the label's color, which Paint eases under the
// pointer.
func (b *buttonContent) tint(c color.Color) {
	if b.lead != nil {
		b.lead.Color(c)
	}
	if b.trail != nil {
		b.trail.Color(c)
	}
}

func (b *buttonContent) Baseline() (float64, bool) {
	if child, ok := b.child.(ggui.Baseliner); ok {
		baseline, ok := child.Baseline()
		return baseline + (b.height-b.size.H)/2, ok
	}
	return 0, false
}
