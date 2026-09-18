package ui

import "github.com/ironpark/ggui"

// RadiosWidget is a group of Radio options built from a list of values.
// Build one with Radios.
type RadiosWidget[T comparable] struct {
	ggui.Interactive
	radios []*RadioWidget[T]
	row    *ggui.RowWidget
	column *ggui.ColumnWidget // set by Vertical
	gap    float64
	gapSet bool
}

// Radios creates one Radio per option, bound to selected and labelled
// through fmt.Sprint until Format says otherwise, side by side with a theme
// gap between them. It is the Select signature for a choice small enough to
// show all at once.
func Radios[T comparable](selected ggui.Binding[T], options []T) *RadiosWidget[T] {
	g := &RadiosWidget[T]{}
	g.Role = ggui.RoleGroup
	for _, o := range options {
		g.radios = append(g.radios, Radio(selected, o, sprint(o)))
	}
	g.row = ggui.Row(g.children()...)
	return g
}

// Format sets how each option is shown.
func (g *RadiosWidget[T]) Format(fn func(T) string) *RadiosWidget[T] {
	for _, r := range g.radios {
		r.Name = fn(r.value)
		r.label = ggui.Text(r.Name)
	}
	return g
}

func (g *RadiosWidget[T]) children() []ggui.Widget {
	return ggui.Children(g.radios, func(r *RadioWidget[T]) ggui.Widget { return r })
}

// Vertical stacks the options instead of lining them up.
func (g *RadiosWidget[T]) Vertical() *RadiosWidget[T] {
	g.column = ggui.Column(g.children()...)
	return g
}

// Gap overrides the theme's space between options.
func (g *RadiosWidget[T]) Gap(v float64) *RadiosWidget[T] { g.gap, g.gapSet = v, true; return g }

// Disabled greys every option out and ignores input while v is true.
func (g *RadiosWidget[T]) Disabled(v bool) *RadiosWidget[T] {
	g.SetInert(v)
	return g
}

// DisabledWhen follows r for Disabled without a rebuild.
func (g *RadiosWidget[T]) DisabledWhen(r ggui.Readable[bool]) *RadiosWidget[T] {
	g.InertWhen(r)
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
	g.Sync()
	for _, r := range g.radios {
		r.Disabled(g.Inert)
	}
	t := env.Theme()
	if g.column != nil {
		return g.column.Gap(pick(g.gapSet, g.gap, t.Space)).Layout(c, env)
	}
	return g.row.Gap(pick(g.gapSet, g.gap, t.Space*2)).Layout(c, env)
}

// Paint implements Widget. The options are one group, so a screen reader
// says how many there are and which of them is chosen.
func (g *RadiosWidget[T]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup, Name: g.Name, Disabled: g.Inert, Min: 1, Max: float64(len(g.radios))}, func(dst *ggui.Canvas) {
		if g.column != nil {
			dst.Paint(g.column, r)
			return
		}
		dst.Paint(g.row, r)
	})
}

// Named sets the accessible name of the radio group, without renaming its options.
func (g *RadiosWidget[T]) Named(s string) *RadiosWidget[T] { g.SetName(s); return g }
