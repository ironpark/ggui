package ui

import (
	"github.com/ironpark/ggui"
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
	c.glyph = ggui.Sz(controlSize, controlSize)
	if label != "" {
		c.label = ggui.Text(label)
	}
	c.onTap = func() { setChanged(checked, !checked.Peek(), c.onChange) }
	return c
}

// Disabled greys the box out and ignores the pointer while v is true.
func (c *CheckboxWidget) Disabled(v bool) *CheckboxWidget { c.Inert = v; return c }

// DisabledWhen follows r for Disabled without a rebuild.
func (c *CheckboxWidget) DisabledWhen(r ggui.Reader[bool]) *CheckboxWidget { c.InertWhen(r); return c }

// OnChange fires with the new value after a click toggled it.
func (c *CheckboxWidget) OnChange(fn func(bool)) *CheckboxWidget { c.onChange = fn; return c }

// Describe implements ggui.Describer: a checkbox reports its tick.
func (c *CheckboxWidget) Describe() ggui.Node {
	return ggui.Node{
		Role:     ggui.RoleCheckbox,
		Name:     c.Name,
		Checked:  ggui.Tri(c.checked.Peek()),
		Disabled: c.Inert,
		Actions:  ggui.ActionPress | ggui.ActionFocus,
	}
}

// Layout implements Widget.
func (c *CheckboxWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	return c.layout(cs, env)
}

// Paint implements Widget.
func (c *CheckboxWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := c.theme
	box := c.paint(dst, r, c)
	on := c.checked.Peek()
	radius := t.Radius * 0.4
	switch {
	case c.Inert:
		dst.FillRoundRect(box, radius, t.Card)
		dst.StrokeRoundRect(box, radius, 1, t.Border)
	case on:
		dst.FillRoundRect(box, radius, pick(c.Hovered, t.PrimaryHover, t.Primary))
	default:
		dst.FillRoundRect(box, radius, t.Input)
		dst.StrokeRoundRect(box, radius, 1, pick(c.Hovered, t.Primary, t.Border))
	}
	if on {
		at := func(x, y float64) ggui.Point {
			return ggui.Pt(box.Origin.X+x*box.Size.W, box.Origin.Y+y*box.Size.H)
		}
		col := pick(c.Inert, t.MutedFg, t.PrimaryFg)
		dst.StrokeLine(at(0.24, 0.52), at(0.43, 0.72), 2, col)
		dst.StrokeLine(at(0.41, 0.72), at(0.78, 0.30), 2, col)
	}
	c.FocusRing(dst, box, radius, t.Ring)
}
