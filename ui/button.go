package ui

import (
	"image/color"

	"github.com/ironpark/ggui"
)

// buttonVariant selects a button's look. One value replaces a set of flags
// that would otherwise have to be kept mutually exclusive by hand.
type buttonVariant int

const (
	variantPrimary buttonVariant = iota
	variantOutline
	variantSecondary
	variantGhost
	variantDestructive
)

// buttonStyle is a variant resolved against a theme.
type buttonStyle struct {
	fill, hover, border, label color.Color
	elevated                   bool
}

func (v buttonVariant) resolve(t ggui.Theme) buttonStyle {
	switch v {
	case variantOutline:
		return buttonStyle{fill: t.Bg, hover: colorOr(t.Accent, t.Muted), border: t.Border, label: t.Fg, elevated: true}
	case variantSecondary:
		return buttonStyle{fill: t.Secondary, hover: mix(t.Secondary, t.Fg, t.HoverMix), label: t.SecondaryFg}
	case variantGhost:
		return buttonStyle{hover: colorOr(t.Accent, t.Muted), label: t.Fg}
	case variantDestructive:
		d := t.Destructive
		return buttonStyle{fill: d, hover: mix(d, t.Card, t.HoverMix*2), label: t.DestructiveFg, elevated: true}
	}
	return buttonStyle{fill: t.Primary, hover: t.PrimaryHover, label: t.PrimaryFg, elevated: true}
}

// ButtonWidget is a clickable box with a label. Build one with Button.
type ButtonWidget struct {
	ggui.Interactive
	label       *ggui.TextWidget
	box         *ggui.BoxWidget
	onTap       func()
	defaultName string
	variant     buttonVariant
	style       buttonStyle
	padded      bool
	selected    bool // current item for composite navigation controls
	expands     func() bool
	opener      ggui.Actor
	value       func() string // optional accessible value for composite triggers
	theme       ggui.Theme
}

// Button creates a primary button: Accent background, OnAccent label.
func Button(label string, onTap func()) *ButtonWidget {
	b := &ButtonWidget{onTap: onTap, label: ggui.Text(label).NoWrap()}
	b.box = ggui.Box(b.label)
	b.Role, b.Name = ggui.RoleButton, label
	b.AutoKey()
	return b
}

// ButtonOf creates a button around any content instead of a text label.
// Give it a name with Named, since nothing on it says what it is.
func ButtonOf(child ggui.Widget, onTap func()) *ButtonWidget {
	b := &ButtonWidget{onTap: onTap, box: ggui.Box(child)}
	b.Role = ggui.RoleButton
	b.AutoKey()
	return b
}

// Named names the button for Probe.Find and the inspector; Button takes
// its text, ButtonOf needs one.
func (b *ButtonWidget) Named(s string) *ButtonWidget { b.Name = s; return b }

// Expands makes the button report whether what it opens is showing, for a
// menu button or a combobox trigger; a plain button does not expand at all,
// which is not the same as being closed.
func (b *ButtonWidget) Expands(open func() bool) *ButtonWidget { b.expands = open; return b }

// Opens hands the expand and collapse actions to the widget that owns the
// popup, since the button describes the node but does not hold it.
func (b *ButtonWidget) Opens(a ggui.Actor) *ButtonWidget { b.opener = a; return b }

// Act implements ggui.Actor.
func (b *ButtonWidget) Act(a ggui.Action) bool {
	if b.Inert {
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
		Disabled: b.Inert,
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

// Outline draws the button as a border around the window's background, with
// the normal text color, for actions that are not the main one.
func (b *ButtonWidget) Outline() *ButtonWidget { b.variant = variantOutline; return b }

// Secondary fills the button with the theme's Secondary surface, for a
// supporting action that should still read as a button.
func (b *ButtonWidget) Secondary() *ButtonWidget { b.variant = variantSecondary; return b }

// Muted is the former name of Secondary.
//
// Deprecated: use Secondary.
func (b *ButtonWidget) Muted() *ButtonWidget { return b.Secondary() }

// Ghost omits the resting background and border for a lightweight action.
func (b *ButtonWidget) Ghost() *ButtonWidget { b.variant = variantGhost; return b }

// Destructive uses the theme's Destructive color for an irreversible action.
func (b *ButtonWidget) Destructive() *ButtonWidget { b.variant = variantDestructive; return b }

// Disabled greys the button out and ignores the pointer while v is true.
func (b *ButtonWidget) Disabled(v bool) *ButtonWidget { b.SetInert(v); return b }

// DisabledWhen follows r for Disabled without a rebuild.
func (b *ButtonWidget) DisabledWhen(r ggui.Readable[bool]) *ButtonWidget { b.InertWhen(r); return b }

// Pad overrides the theme's padding, with the shorthand Insets accepts.
func (b *ButtonWidget) Pad(sides ...float64) *ButtonWidget {
	b.box.Pad(sides...)
	b.padded = true
	return b
}

// Layout implements Widget.
func (b *ButtonWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	b.Sync()
	t := env.Theme()
	b.theme = t
	if !b.padded {
		b.box.Padding(t.ButtonPad)
	}
	b.box.Radius(t.Radius)
	b.style = b.variant.resolve(t)
	if b.label != nil {
		b.label.Color(pick(b.Inert, mix(b.style.label, t.Card, t.DisabledMix), b.style.label))
	}
	return b.box.Layout(c, env.WithText(ggui.TextStyle{Color: pick(b.Inert, mix(b.style.label, t.Card, t.DisabledMix), b.style.label)}))
}

// Paint implements Widget.
func (b *ButtonWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t, st := b.theme, b.style
	fill, border := st.fill, st.border
	if b.Hovered && !b.Inert {
		fill = st.hover
	}
	if b.Pressed && b.Hovered && !b.Inert {
		fill = mix(fill, t.Fg, t.PressMix)
	}
	if b.Inert && fill != nil {
		fill = mix(fill, t.Card, t.DisabledMix)
	}
	b.box.Border(0, nil)
	if border != nil {
		b.box.Border(t.BorderWidth, border)
	}
	b.box.Fill(fill)
	if st.elevated && !b.Inert {
		dst.Shadow(r, t.Radius, t.CardShadow)
	}
	b.Hit(dst, r, b, ggui.CursorShapePointer)
	dst.Paint(b.box, r)
	b.FocusRing(dst, r, t.Radius, t.Ring)
}

// HandleKey implements KeyHandler: Space or Enter presses the button.
func (b *ButtonWidget) HandleKey(ev ggui.KeyEvent) { b.Keyboard(ev, b.onTap) }

// HandlePointer implements PointerHandler.
func (b *ButtonWidget) HandlePointer(ev ggui.PointerEvent) bool { return b.Pointer(ev, b.onTap) }

func (b *ButtonWidget) name() string { return pick(b.Name != "", b.Name, b.defaultName) }

// Semantics implements ggui.Semantic, including a composite trigger's fallback.
func (b *ButtonWidget) Semantics() (ggui.Role, string) { return b.Role, b.name() }
