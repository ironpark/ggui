package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// ButtonWidget is a clickable box with a label. Build one with Button.
type ButtonWidget struct {
	ggui.Interactive
	label     *ggui.TextWidget
	box       *ggui.BoxWidget
	onTap     func()
	secondary bool
	padded    bool
	expands   func() bool
	theme     ggui.Theme
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

// Describe implements ggui.Describer.
func (b *ButtonWidget) Describe() ggui.Node {
	n := ggui.Node{
		Role:     b.Role,
		Name:     b.Name,
		Disabled: b.Inert,
		Actions:  ggui.ActionPress | ggui.ActionFocus,
	}
	if b.expands != nil {
		open := b.expands()
		n.Expanded = ggui.Expandable(open)
		n.Actions |= pick(open, ggui.ActionCollapse, ggui.ActionExpand)
	}
	return n
}

// Secondary makes the button quiet: Surface background with a border and
// the normal text color, for actions that are not the main one.
func (b *ButtonWidget) Secondary() *ButtonWidget { b.secondary = true; return b }

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
	if b.label != nil {
		switch {
		case b.Inert:
			b.label.Color(t.Muted)
		case b.secondary:
			b.label.Color(t.Fg)
		default:
			b.label.Color(t.OnAccent)
		}
	}
	return b.box.Layout(c, env)
}

// Paint implements Widget.
func (b *ButtonWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := b.theme
	var fill color.Color
	b.box.Border(0, nil)
	switch {
	case b.Inert:
		fill = pick(b.secondary, subtle(t), mix(t.Accent, t.Surface, .65))
		if b.secondary {
			b.box.Border(1, t.Border)
		}
	case b.secondary:
		fill = pick(b.Hovered, subtle(t), t.Surface)
		b.box.Border(1, t.Border)
	default:
		fill = pick(b.Hovered, t.AccentHover, t.Accent)
	}
	if b.Pressed && b.Hovered && !b.Inert {
		fill = tint(fill, pressTint)
	}
	b.box.Fill(fill)
	b.Hit(dst, r, b, ebiten.CursorShapePointer)
	dst.Paint(b.box, r)
	b.FocusRing(dst, r, t.Radius, t.Accent)
}

// HandleKey implements KeyHandler: Space or Enter presses the button.
func (b *ButtonWidget) HandleKey(ev ggui.KeyEvent) { b.Keyboard(ev, b.onTap) }

// HandlePointer implements PointerHandler.
func (b *ButtonWidget) HandlePointer(ev ggui.PointerEvent) bool { return b.Pointer(ev, b.onTap) }
