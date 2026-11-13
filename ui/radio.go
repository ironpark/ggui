package ui

import (
	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// RadioWidget is one option of a group that shares a signal. Build one with
// Radio.
type RadioWidget[T comparable] struct {
	toggle
	selected ggui.Binding[T]
	value    T
	onChange func(T)

	group   *RadiosWidget[T] // the Radios it is an option of, if any
	pending bool             // an arrow key moved here: take the keyboard at the next paint
}

// Radio creates a round option that is filled while selected holds value
// and selects it when clicked. Every Radio bound to the same signal is one
// group.
func Radio[T comparable](selected ggui.Binding[T], value T, label string) *RadioWidget[T] {
	r := &RadioWidget[T]{selected: selected, value: value}
	r.Role = ggui.RoleRadio
	r.SetName(label)
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

// BindDisabled follows r for Disabled without a rebuild.
func (r *RadioWidget[T]) BindDisabled(when ggui.Readable[bool]) *RadioWidget[T] {
	r.BindInert(when)
	return r
}

// Invalid marks the option as having failed validation: a destructive
// border while it is not chosen and a destructive halo around it. An
// option inside a Field holding an error is invalid without it.
func (r *RadioWidget[T]) Invalid(v bool) *RadioWidget[T] { r.invalidProp.Set(v); return r }

// BindInvalid follows when for Invalid without a rebuild.
func (r *RadioWidget[T]) BindInvalid(when ggui.Readable[bool]) *RadioWidget[T] {
	r.invalidProp.Bind(when, "BindInvalid")
	return r
}

// Describe implements ggui.Describer: an option reports whether the group
// currently holds its value.
func (r *RadioWidget[T]) Describe() ggui.Node {
	on := ggui.Untrack(r.selected.Get) == r.value
	return ggui.Node{
		Role:     ggui.RoleRadio,
		Name:     r.SemanticName(),
		Checked:  ggui.Tri(on),
		Selected: on,
		Disabled: r.IsInert(),
		Actions:  ggui.ActionPress | ggui.ActionSelect | ggui.ActionFocus,
	}
}

// Layout implements Widget.
func (r *RadioWidget[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return r.layout(c, env, squareGlyph(uitheme.Of(env)))
}

// Paint implements Widget.
func (r *RadioWidget[T]) Paint(dst *ggui.Canvas, rect ggui.Rect) {
	t := r.theme
	box := r.paint(dst, rect, r)
	if r.pending {
		r.pending = false
		dst.RequestFocus(r)
	}
	center := box.Center()
	radius := box.Size.W / 2
	on := ggui.Untrack(r.selected.Get) == r.value
	invalid := r.invalidAmount(dst, box)
	// shadcn's radio item has no transition: the fill switches at once.
	border, fill, dot := mix(t.InputBorder, invalidBorder(t), invalid), t.Input, t.PrimaryFg
	if on {
		border, fill = t.Primary, t.Primary
	}
	if r.IsInert() {
		border, fill, dot = t.Disabled(border), t.Disabled(fill), t.Disabled(dot)
	}
	// The border is stroked over the fill rather than filled under it, so
	// a translucent Input does not show a disc of the border color.
	dst.FillCircle(center, radius, fill)
	dst.StrokeRoundRect(box, radius, t.BorderWidth, border)
	if on {
		dst.FillCircle(center, radius*.5, dot)
	}
	r.stateRing(dst, box, radius, invalid)
}

// TabStop implements ggui.TabStopper. The options of a Radios are one stop
// in the Tab order, as a radio group is: the chosen option, or the first
// enabled one while none is chosen. A Radio on its own is always a stop.
func (r *RadioWidget[T]) TabStop() bool { return r.group == nil || r.group.stop() == r }

// ConsumesKey implements ggui.KeyConsumer: in a Radios the arrows move the
// choice.
func (r *RadioWidget[T]) ConsumesKey(ev ggui.KeyEvent) bool {
	if r.group != nil && navConsumes(ev, navBoth) && ev.Key != ggui.KeyHome && ev.Key != ggui.KeyEnd {
		return true
	}
	return r.toggle.ConsumesKey(ev)
}

// HandleKey implements ggui.KeyHandler: Space or Enter chooses the option,
// and in a Radios the arrows move to the option before or after it.
func (r *RadioWidget[T]) HandleKey(ev ggui.KeyEvent) {
	r.toggle.HandleKey(ev)
	if r.group != nil && !r.IsInert() && r.ConsumesKey(ev) && !ggui.Activates(ev) {
		r.group.move(r, directionalKey(ev, r.env.Direction() == ggui.DirRTL))
	}
}

// Adopt implements ggui.Adopter: a move from the keyboard that a rebuild
// came between still takes the keyboard to the option it chose.
func (r *RadioWidget[T]) Adopt(prev any) {
	r.toggle.Adopt(prev)
	if p, ok := prev.(*RadioWidget[T]); ok && p != r {
		r.pending, p.pending = r.pending || p.pending, false
	}
}
