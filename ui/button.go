package ui

import (
	"image/color"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// ButtonStyle is the look of a button variant under a theme.
type ButtonStyle struct {
	Fill, Hover       color.Color // the surface at rest and under the pointer; nil draws none
	Label, HoverLabel color.Color // the label at rest and under the pointer; a nil HoverLabel keeps Label
	Border            color.Color // a line just inside the edge; nil draws none
	Ring              color.Color // the focus ring; nil is the theme's Ring
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
		return ButtonStyle{Fill: t.Primary, Hover: t.PrimaryHover, Label: t.PrimaryFg, Shadow: true}
	})
	// ButtonOutline is a border around the page's background.
	ButtonOutline = NewVariant("outline", func(t uitheme.Theme) ButtonStyle {
		return ButtonStyle{Fill: t.Bg, Hover: t.Accent, Label: t.Fg, HoverLabel: t.AccentFg, Border: t.Border, Shadow: true}
	})
	// ButtonSecondary is a supporting action on the Secondary surface.
	ButtonSecondary = NewVariant("secondary", func(t uitheme.Theme) ButtonStyle {
		return ButtonStyle{Fill: t.Secondary, Hover: t.Hovered(t.Secondary), Label: t.SecondaryFg}
	})
	// ButtonGhost has no surface until the pointer is over it.
	ButtonGhost = NewVariant("ghost", func(t uitheme.Theme) ButtonStyle {
		return ButtonStyle{Hover: t.Accent, Label: t.Fg, HoverLabel: t.AccentFg}
	})
	// ButtonDestructive is an irreversible action: the Destructive surface,
	// toned down on a dark page as shadcn does.
	ButtonDestructive = NewVariant("destructive", func(t uitheme.Theme) ButtonStyle {
		fill := t.Destructive
		if isDark(t.Bg) {
			fill = mix(fill, t.Bg, .4)
		}
		return ButtonStyle{Fill: fill, Hover: mix(fill, t.Bg, .1), Label: t.DestructiveFg, Ring: destructiveRing(t), Shadow: true}
	})
	// ButtonLink is a label in the Primary color, underlined under the
	// pointer.
	ButtonLink = NewVariant("link", func(t uitheme.Theme) ButtonStyle {
		return ButtonStyle{Label: t.Primary, Underline: true}
	})
)

// ButtonSize is the padding of a button size variant.
type ButtonSize struct {
	Pad    ggui.EdgeInsets
	Square bool // as wide as it is tall, for a button holding one icon
}

// ButtonSizeVariant is a size for Button.Size.
type ButtonSizeVariant = Variant[ButtonSize]

// The button sizes of shadcn/ui, from the theme's ButtonPad.
var (
	ButtonDefault = NewVariant("default", func(t uitheme.Theme) ButtonSize { return ButtonSize{Pad: t.ButtonPad} })
	ButtonSmall   = NewVariant("sm", func(t uitheme.Theme) ButtonSize {
		p := t.ButtonPad
		return ButtonSize{Pad: ggui.Insets(p.Top*.75, p.Left*.75)}
	})
	ButtonLarge = NewVariant("lg", func(t uitheme.Theme) ButtonSize {
		p := t.ButtonPad
		return ButtonSize{Pad: ggui.Insets(p.Top*1.25, p.Left*1.5)}
	})
	ButtonIcon = NewVariant("icon", func(t uitheme.Theme) ButtonSize {
		return ButtonSize{Pad: ggui.Insets(t.ButtonPad.Top), Square: true}
	})
)

// ButtonWidget is a clickable box with a label. Build one with Button.
type ButtonWidget struct {
	props property.Owner
	ggui.Interactive
	label       *ggui.TextWidget
	box         *ggui.BoxWidget
	onTap       func()
	defaultName string
	variant     *ButtonVariant
	size        *ButtonSizeVariant
	style       ButtonStyle
	padded      bool
	pad         ggui.EdgeInsets
	radius      float64 // set by Radius; negative follows the theme
	selected    bool    // current item for composite navigation controls
	expands     func() bool
	opener      ggui.Actor
	value       func() string // optional accessible value for composite triggers
	motion      time.Duration
	theme       uitheme.Theme
	chord       ggui.Chord
	chorded     bool
}

// Button creates a primary button: the Primary surface with a PrimaryFg
// label.
func Button(label string, onTap func()) *ButtonWidget {
	b := &ButtonWidget{onTap: onTap, label: ggui.Text(label).NoWrap(), variant: ButtonPrimary, size: ButtonDefault, radius: -1}
	b.box = ggui.Box(b.label)
	b.Role = ggui.RoleButton
	b.SetName(label)
	b.AutoKey()
	return b
}

// ButtonOf creates a button around any content instead of a text label.
// Give it a name with Name, since nothing on it says what it is.
func ButtonOf(child ggui.Widget, onTap func()) *ButtonWidget {
	b := &ButtonWidget{onTap: onTap, box: ggui.Box(child), variant: ButtonPrimary, size: ButtonDefault, radius: -1}
	b.Role = ggui.RoleButton
	b.AutoKey()
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

// Size sets the button's size: ButtonSmall, ButtonLarge, ButtonIcon for a
// square button holding one icon, or a size of your own. Pad overrides its
// padding.
func (b *ButtonWidget) Size(v *ButtonSizeVariant) *ButtonWidget {
	defer property.Watch(&b.props, &b.size)()
	b.size = v
	return b
}

// Radius overrides the theme's corner radius for this button.
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
	defer b.props.Layout()()
	b.Sync()
	t := uitheme.From(env)
	b.theme = t
	b.motion = env.Motion(t.MotionFast)
	size := b.size.Style(t)
	if !b.padded {
		b.pad = size.Pad
		b.box.Padding(b.pad)
	}
	b.box.Radius(pick(b.radius >= 0, b.radius, t.Radius))
	b.style = b.variant.Style(t)
	label := b.style.Label
	if b.IsInert() {
		label = t.Disabled(label)
	}
	if b.label != nil {
		b.label.Color(label) // Paint swaps in HoverLabel
	}
	out := b.box.Layout(c, env.WithText(t.Label.Merge(ggui.TextStyle{Color: label})))
	if size.Square && !b.padded {
		side := c.Constrain(ggui.Sz(max(out.W, out.H), max(out.W, out.H)))
		out = b.box.Layout(ggui.Tight(side), env.WithText(t.Label.Merge(ggui.TextStyle{Color: label})))
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
	hover := dst.Ease(b.Anchor(r), buttonHoverSlot, pick(b.Hovered && !inert, 1.0, 0.0), b.motion)
	if hover >= 1 {
		fill = st.Hover
	} else if hover > 0 {
		fill = mix(colorOr(fill, color.Transparent), colorOr(st.Hover, color.Transparent), hover)
	}
	if st.HoverLabel != nil && hover > 0 {
		label = mix(label, st.HoverLabel, hover)
	}
	if b.Pressed && b.Hovered && !inert {
		fill = t.Pressed(fill)
	}
	if inert {
		fill, label = t.Disabled(fill), t.Disabled(label)
	}
	if b.label != nil {
		b.label.Color(label)
	}
	b.box.Border(0, nil)
	if st.Border != nil {
		b.box.Border(t.BorderWidth, pick(inert, t.Disabled(st.Border), st.Border))
	}
	b.box.Fill(fill)
	radius := pick(b.radius >= 0, b.radius, t.Radius)
	if st.Shadow && !inert {
		dst.Shadow(r, radius, t.CardShadow)
	}
	b.Hit(dst, r, b, ggui.CursorShapePointer)
	if b.chorded && !inert && b.onTap != nil {
		dst.Shortcut(b.chord, b.onTap)
	}
	dst.Paint(b.box, r)
	if st.Underline && hover > 0 {
		if base, ok := b.box.Baseline(); ok {
			pad := b.pad
			y := r.Origin.Y + base + 2
			dst.FillRect(ggui.Rct(ggui.Pt(r.Origin.X+pad.Left, y), ggui.Sz(r.Size.W-pad.Left-pad.Right, 1)), fade(label, hover))
		}
	}
	b.FocusRing(dst, r, radius, colorOr(st.Ring, t.Ring))
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
