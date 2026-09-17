package ui

import (
	"github.com/ironpark/ggui"
)

// RadioWidget is one option of a group that shares a signal. Build one with
// Radio.
type RadioWidget[T comparable] struct {
	toggle
	selected *ggui.Signal[T]
	value    T
	onChange func(T)
}

// Radio creates a round option that is filled while selected holds value
// and selects it when clicked. Every Radio bound to the same signal is one
// group.
func Radio[T comparable](selected *ggui.Signal[T], value T, label string) *RadioWidget[T] {
	r := &RadioWidget[T]{selected: selected, value: value}
	r.glyph = ggui.Sz(controlSize, controlSize)
	if label != "" {
		r.label = ggui.Text(label)
	}
	r.onTap = func() { setChanged(selected, value, r.onChange) }
	return r
}

// OnChange fires with value after a click selected this option.
func (r *RadioWidget[T]) OnChange(fn func(T)) *RadioWidget[T] { r.onChange = fn; return r }

// Disabled greys the option out and ignores the pointer while v is true.
func (r *RadioWidget[T]) Disabled(v bool) *RadioWidget[T] { r.Inert = v; return r }

// Layout implements Widget.
func (r *RadioWidget[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size { return r.layout(c, env) }

// Paint implements Widget.
func (r *RadioWidget[T]) Paint(dst *ggui.Canvas, rect ggui.Rect) {
	t := r.theme
	box := r.paint(dst, rect, r)
	center := ggui.Pt(box.Origin.X+box.Size.W/2, box.Origin.Y+box.Size.H/2)
	radius := box.Size.W / 2
	on := r.selected.Peek() == r.value
	switch {
	case r.Inert:
		dst.FillCircle(center, radius, t.Border)
		dst.FillCircle(center, radius-1, t.Surface)
		if on {
			dst.FillCircle(center, radius*0.4, t.Muted)
		}
	case on:
		dst.FillCircle(center, radius, pick(r.Hovered, t.AccentHover, t.Accent))
		dst.FillCircle(center, radius*0.4, t.OnAccent)
	default:
		dst.FillCircle(center, radius, pick(r.Hovered, t.Accent, t.Border))
		dst.FillCircle(center, radius-1, t.Field)
	}
	r.FocusRing(dst, box, radius, t.Accent)
}
