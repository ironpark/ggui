package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// buttonVariant selects a button's look. One value replaces a set of flags
// that would otherwise have to be kept mutually exclusive by hand.
type buttonVariant int

const (
	variantPrimary buttonVariant = iota
	variantOutline
	variantMuted
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
		return buttonStyle{fill: t.Surface, hover: mutedSurface(t), border: t.Border, label: t.Fg, elevated: true}
	case variantMuted:
		m := mutedSurface(t)
		return buttonStyle{fill: m, hover: mix(m, t.Fg, .06), label: t.Fg}
	case variantGhost:
		return buttonStyle{hover: mutedSurface(t), label: t.Fg}
	case variantDestructive:
		d := dangerColor(t)
		return buttonStyle{fill: d, hover: mix(d, t.Surface, .12), label: color.White, elevated: true}
	}
	return buttonStyle{fill: t.Accent, hover: t.AccentHover, label: t.OnAccent, elevated: true}
}

// ButtonWidget is a clickable box with a label. Build one with Button.
type ButtonWidget struct {
	ggui.Interactive
	label    *ggui.TextWidget
	box      *ggui.BoxWidget
	onTap    func()
	variant  buttonVariant
	style    buttonStyle
	padded   bool
	selected bool // current item for composite navigation controls
	expands  func() bool
	opener   ggui.Actor
	value    func() string // optional accessible value for composite triggers
	theme    ggui.Theme
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
// Give it a Label, since nothing on it says what it is.
func ButtonOf(child ggui.Widget, onTap func()) *ButtonWidget {
	b := &ButtonWidget{onTap: onTap, box: ggui.Box(child)}
	b.Role = ggui.RoleButton
	b.AutoKey()
	return b
}

// Label names the button for Probe.Find and the inspector; Button takes
// its text, ButtonOf needs one.
func (b *ButtonWidget) Label(s string) *ButtonWidget { b.Name = s; return b }

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
		Name:     b.Name,
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

// Secondary makes the button quiet: Surface background with a border and
// the normal text color, for actions that are not the main one.
func (b *ButtonWidget) Secondary() *ButtonWidget { b.variant = variantOutline; return b }

// Outline is an alias for Secondary, preserving the established outline style.
func (b *ButtonWidget) Outline() *ButtonWidget { return b.Secondary() }

// Muted uses a subdued filled surface for a supporting action.
func (b *ButtonWidget) Muted() *ButtonWidget { b.variant = variantMuted; return b }

// Ghost omits the resting background and border for a lightweight action.
func (b *ButtonWidget) Ghost() *ButtonWidget { b.variant = variantGhost; return b }

// Destructive uses DangerColor for an irreversible action.
func (b *ButtonWidget) Destructive() *ButtonWidget { b.variant = variantDestructive; return b }

// Disabled greys the button out and ignores the pointer while v is true.
func (b *ButtonWidget) Disabled(v bool) *ButtonWidget { b.Inert = v; return b }

// DisabledWhen follows r for Disabled without a rebuild.
func (b *ButtonWidget) DisabledWhen(r ggui.Reader[bool]) *ButtonWidget { b.InertWhen(r); return b }

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
		b.label.Color(pick(b.Inert, mix(b.style.label, t.Surface, .5), b.style.label))
	}
	return b.box.Layout(c, env)
}

// Paint implements Widget.
func (b *ButtonWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t, st := b.theme, b.style
	fill, border := st.fill, st.border
	if b.Hovered && !b.Inert {
		fill = st.hover
	}
	if b.Pressed && b.Hovered && !b.Inert {
		fill = mix(fill, t.Fg, .08)
	}
	if b.Inert && fill != nil {
		fill = mix(fill, t.Surface, .55)
	}
	b.box.Border(0, nil)
	if border != nil {
		b.box.Border(1, border)
	}
	b.box.Fill(fill)
	if st.elevated && !b.Inert {
		dst.Shadow(r, t.Radius, cardShadow(t))
	}
	b.Hit(dst, r, b, ebiten.CursorShapePointer)
	dst.Paint(b.box, r)
	b.FocusRing(dst, r, t.Radius, focusColor(t))
}

// HandleKey implements KeyHandler: Space or Enter presses the button.
func (b *ButtonWidget) HandleKey(ev ggui.KeyEvent) { b.Keyboard(ev, b.onTap) }

// HandlePointer implements PointerHandler.
func (b *ButtonWidget) HandlePointer(ev ggui.PointerEvent) bool { return b.Pointer(ev, b.onTap) }
