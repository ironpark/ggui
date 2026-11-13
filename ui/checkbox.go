package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// CheckboxWidget is a box that is ticked while its signal is true. Build one
// with Checkbox.
type CheckboxWidget struct {
	toggle
	props    property.Owner
	checked  ggui.Binding[bool]
	onChange func(bool)
	mixed    ggui.Readable[bool]
}

// Checkbox binds a tick box to checked; a click toggles it. label may be "".
func Checkbox(checked ggui.Binding[bool], label string) *CheckboxWidget {
	c := &CheckboxWidget{checked: checked}
	c.Role = ggui.RoleCheckbox
	c.SetName(label)
	c.AutoKey()
	if label != "" {
		c.label = ggui.Text(label)
	}
	c.onTap = func() { setChanged(checked, !ggui.Untrack(checked.Get), c.onChange) }
	return c
}

// Disabled greys the box out and ignores the pointer while v is true.
func (c *CheckboxWidget) Disabled(v bool) *CheckboxWidget { c.SetInert(v); return c }

// BindDisabled follows r for Disabled without a rebuild.
func (c *CheckboxWidget) BindDisabled(r ggui.Readable[bool]) *CheckboxWidget {
	c.BindInert(r)
	return c
}

// Indeterminate sets the mixed state and detaches its binding.
func (c *CheckboxWidget) Indeterminate(v bool) *CheckboxWidget {
	return c.BindIndeterminate(ggui.Const(v))
}

// BindIndeterminate shows a mixed mark while r is true. Clicking still writes checked.
func (c *CheckboxWidget) BindIndeterminate(r ggui.Readable[bool]) *CheckboxWidget {
	property.Require(r, "BindIndeterminate")
	if !property.Same(c.mixed, r) {
		c.mixed = r
		c.props.Changed()
	}
	return c
}

// Invalid marks the box as having failed validation: a destructive border
// while it is not ticked and a destructive halo around it. A box inside a
// Field holding an error is invalid without it.
func (c *CheckboxWidget) Invalid(v bool) *CheckboxWidget { c.invalidProp.Set(v); return c }

// BindInvalid follows r for Invalid without a rebuild.
func (c *CheckboxWidget) BindInvalid(r ggui.Readable[bool]) *CheckboxWidget {
	c.invalidProp.Bind(r, "BindInvalid")
	return c
}

// OnChange fires with the new value after a click toggled it.
func (c *CheckboxWidget) OnChange(fn func(bool)) *CheckboxWidget { c.onChange = fn; return c }

// Describe implements ggui.Describer: a checkbox reports its tick.
func (c *CheckboxWidget) Describe() ggui.Node {
	n := ggui.Node{
		Role:     ggui.RoleCheckbox,
		Name:     c.SemanticName(),
		Checked:  ggui.Tri(ggui.Untrack(c.checked.Get)),
		Disabled: c.IsInert(),
		Actions:  ggui.ActionPress | ggui.ActionFocus,
	}
	if c.mixed != nil && ggui.Untrack(c.mixed.Get) {
		n.Checked = ggui.TriMixed
	}
	return n
}

// Layout implements Widget.
func (c *CheckboxWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	defer c.props.Layout().EndLayout()
	return c.layout(cs, env, squareGlyph(uitheme.Of(env)))
}

// checkSlot eases a checkbox's fill between unticked and ticked.
var checkSlot = ggui.NewSlot[*ggui.Motion]("checkbox fill")

// Paint implements Widget.
func (c *CheckboxWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := c.theme
	box := c.paint(dst, r, c)
	on := ggui.Untrack(c.checked.Get)
	mixed := c.mixed != nil && ggui.Untrack(c.mixed.Get)
	radius := t.ControlSize / 4
	invalid := c.invalidAmount(dst, box)
	// The surface and the border ease to Primary as shadcn's
	// transition-colors does; the mark itself appears at once.
	k := dst.Ease(c.Anchor(box), checkSlot, pick(on || mixed, 1.0, 0.0), c.motion)
	fill := mix(t.Input, t.Primary, k)
	border := mix(mix(t.InputBorder, invalidBorder(t), invalid), t.Primary, k)
	mark := t.PrimaryFg
	if c.IsInert() {
		fill, border, mark = t.Disabled(fill), t.Disabled(border), t.Disabled(mark)
	}
	dst.FillRoundRect(box, radius, fill)
	dst.StrokeRoundRect(box, radius, t.BorderWidth, border)
	if on || mixed {
		paintIconAt(dst, c.env, pick(mixed, icons.Minus, icons.Check), box.Center(), t.ControlSize*.875, mark, 0)
	}
	c.stateRing(dst, box, radius, invalid)
}
