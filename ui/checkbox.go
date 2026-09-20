package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/icons"
)

// CheckboxWidget is a box that is ticked while its signal is true. Build one
// with Checkbox.
type CheckboxWidget struct {
	toggle
	checked  ggui.Binding[bool]
	onChange func(bool)
}

// Checkbox binds a tick box to checked; a click toggles it. label may be "".
func Checkbox(checked ggui.Binding[bool], label string) *CheckboxWidget {
	c := &CheckboxWidget{checked: checked}
	c.Role, c.Name = ggui.RoleCheckbox, label
	c.AutoKey()
	if label != "" {
		c.label = ggui.Text(label)
	}
	c.onTap = func() { setChanged(checked, !ggui.Untrack(checked.Get), c.onChange) }
	return c
}

// Disabled greys the box out and ignores the pointer while v is true.
func (c *CheckboxWidget) Disabled(v bool) *CheckboxWidget { c.SetInert(v); return c }

// DisabledWhen follows r for Disabled without a rebuild.
func (c *CheckboxWidget) DisabledWhen(r ggui.Readable[bool]) *CheckboxWidget {
	c.InertWhen(r)
	return c
}

// OnChange fires with the new value after a click toggled it.
func (c *CheckboxWidget) OnChange(fn func(bool)) *CheckboxWidget { c.onChange = fn; return c }

// Describe implements ggui.Describer: a checkbox reports its tick.
func (c *CheckboxWidget) Describe() ggui.Node {
	return ggui.Node{
		Role:     ggui.RoleCheckbox,
		Name:     c.Name,
		Checked:  ggui.Tri(ggui.Untrack(c.checked.Get)),
		Disabled: c.Inert,
		Actions:  ggui.ActionPress | ggui.ActionFocus,
	}
}

// Layout implements Widget.
func (c *CheckboxWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	return c.layout(cs, env, squareGlyph(env.Theme()))
}

// Paint implements Widget.
func (c *CheckboxWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := c.theme
	box := c.paint(dst, r, c)
	on := ggui.Untrack(c.checked.Get)
	radius := t.Radius * 0.4
	opacity := pick(c.Inert, .5, 1.0)
	fill, border := t.Input, colorOr(t.InputBorder, t.Border)
	if on {
		fill, border = t.Primary, t.Primary
	}
	dst.FillRoundRect(box, radius, fade(fill, opacity))
	dst.StrokeRoundRect(box, radius, 1, fade(border, opacity))
	if on {
		paintIcon(dst, c.env, icons.Check, box, fade(t.PrimaryFg, opacity), 0)
	}
	c.FocusRing(dst, box, radius, t.Ring)
}
