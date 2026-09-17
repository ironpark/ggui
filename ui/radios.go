package ui

import "github.com/ironpark/ggui"

// RadiosWidget is a group of Radio options built from a list of values.
// Build one with Radios.
type RadiosWidget[T comparable] struct {
	radios   []*RadioWidget[T]
	vertical bool
	flow     ggui.Widget
	gap      float64
}

// Radios creates one Radio per option, bound to selected and labelled
// through label, side by side with a theme gap between them. It is the
// Select signature for a choice small enough to show all at once.
func Radios[T comparable](selected *ggui.Signal[T], options []T, label func(T) string) *RadiosWidget[T] {
	g := &RadiosWidget[T]{gap: -1}
	for _, o := range options {
		g.radios = append(g.radios, Radio(selected, o, label(o)))
	}
	return g
}

// RadioStrings is Radios for plain strings, labelled as they are.
func RadioStrings(selected *ggui.Signal[string], options ...string) *RadiosWidget[string] {
	return Radios(selected, options, func(s string) string { return s })
}

// Vertical stacks the options instead of lining them up.
func (g *RadiosWidget[T]) Vertical() *RadiosWidget[T] { g.vertical = true; return g }

// Gap overrides the theme's space between options.
func (g *RadiosWidget[T]) Gap(v float64) *RadiosWidget[T] { g.gap = v; return g }

// Disabled greys every option out and ignores input while v is true.
func (g *RadiosWidget[T]) Disabled(v bool) *RadiosWidget[T] {
	for _, r := range g.radios {
		r.Disabled(v)
	}
	return g
}

// OnChange fires with the value after a click selected it.
func (g *RadiosWidget[T]) OnChange(fn func(T)) *RadiosWidget[T] {
	for _, r := range g.radios {
		r.OnChange(fn)
	}
	return g
}

// Layout implements Widget.
func (g *RadiosWidget[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	gap := g.gap
	if gap < 0 {
		gap = pick(g.vertical, t.Space, t.Space*2)
	}
	children := make([]ggui.Widget, len(g.radios))
	for i, r := range g.radios {
		children[i] = r
	}
	if g.vertical {
		g.flow = ggui.Column(children...).Gap(gap)
	} else {
		g.flow = ggui.Row(children...).Gap(gap)
	}
	return g.flow.Layout(c, env)
}

// Paint implements Widget.
func (g *RadiosWidget[T]) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(g.flow, r) }
