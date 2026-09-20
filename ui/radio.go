package ui

import (
	"github.com/ironpark/ggui"
)

// RadioWidget is one option of a group that shares a signal. Build one with
// Radio.
type RadioWidget[T comparable] struct {
	toggle
	selected ggui.Binding[T]
	value    T
	onChange func(T)
}

// Radio creates a round option that is filled while selected holds value
// and selects it when clicked. Every Radio bound to the same signal is one
// group.
func Radio[T comparable](selected ggui.Binding[T], value T, label string) *RadioWidget[T] {
	r := &RadioWidget[T]{selected: selected, value: value}
	r.Role, r.Name = ggui.RoleRadio, label
	r.AutoKey()
	if label != "" {
		r.label = ggui.Text(label)
	}
	r.onTap = func() { setChanged(selected, value, r.onChange) }
	return r
}

// OnChange fires with value after a click selected this option.
func (r *RadioWidget[T]) OnChange(fn func(T)) *RadioWidget[T] { r.onChange = fn; return r }

// Disabled greys the option out and ignores the pointer while v is true.
func (r *RadioWidget[T]) Disabled(v bool) *RadioWidget[T] { r.SetInert(v); return r }

// DisabledWhen follows r for Disabled without a rebuild.
func (r *RadioWidget[T]) DisabledWhen(when ggui.Readable[bool]) *RadioWidget[T] {
	r.InertWhen(when)
	return r
}

// Describe implements ggui.Describer: an option reports whether the group
// currently holds its value.
func (r *RadioWidget[T]) Describe() ggui.Node {
	on := ggui.Untrack(r.selected.Get) == r.value
	return ggui.Node{
		Role:     ggui.RoleRadio,
		Name:     r.Name,
		Checked:  ggui.Tri(on),
		Selected: on,
		Disabled: r.Inert,
		Actions:  ggui.ActionPress | ggui.ActionSelect | ggui.ActionFocus,
	}
}

// Layout implements Widget.
func (r *RadioWidget[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return r.layout(c, env, squareGlyph(env.Theme()))
}

// Paint implements Widget.
func (r *RadioWidget[T]) Paint(dst *ggui.Canvas, rect ggui.Rect) {
	t := r.theme
	box := r.paint(dst, rect, r)
	center := ggui.Pt(box.Origin.X+box.Size.W/2, box.Origin.Y+box.Size.H/2)
	radius := box.Size.W / 2
	on := ggui.Untrack(r.selected.Get) == r.value
	opacity := pick(r.Inert, .5, 1.0)
	border := colorOr(t.InputBorder, t.Border)
	if on {
		border = t.Primary
	}
	dst.FillCircle(center, radius, fade(border, opacity))
	dst.FillCircle(center, radius-1, fade(t.Input, opacity))
	if on {
		dst.FillCircle(center, radius*.45, fade(t.Primary, opacity))
	}
	r.FocusRing(dst, box, radius, t.Ring)
}
