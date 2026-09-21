package ui

import "github.com/ironpark/ggui/internal/property"

import "github.com/ironpark/ggui"
import "slices"

// RadiosWidget is a group of Radio options built from a list of values.
// Build one with Radios.
type RadiosWidget[T comparable] struct {
	selected   ggui.Binding[T]
	options    []T
	optionProp property.Value[[]T]
	label      func(T) string
	onChange   func(T)

	props property.Owner
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
func Radios[T comparable](selected ggui.Binding[T]) *RadiosWidget[T] {
	g := &RadiosWidget[T]{selected: selected, label: sprint[T]}
	g.Role = ggui.RoleGroup
	g.row = ggui.Row()
	return g
}

// Options owns a shallow snapshot without writing the selected value.
func (g *RadiosWidget[T]) Options(options []T) *RadiosWidget[T] {
	changed := g.setOptions(options)
	detached := g.optionProp.Set(g.options)
	if changed || detached {
		g.props.Changed()
	}
	return g
}

// BindOptions follows a non-nil options reader during layout.
func (g *RadiosWidget[T]) BindOptions(r ggui.Readable[[]T]) *RadiosWidget[T] {
	if g.optionProp.Bind(r, "BindOptions") {
		g.props.Changed()
	}
	return g
}
func (g *RadiosWidget[T]) setOptions(options []T) bool {
	if slices.Equal(g.options, options) {
		return false
	}
	remaining := make(map[T][]*RadioWidget[T])
	for _, r := range g.radios {
		remaining[r.value] = append(remaining[r.value], r)
	}
	next := make([]*RadioWidget[T], 0, len(options))
	for _, v := range options {
		var r *RadioWidget[T]
		if old := remaining[v]; len(old) > 0 {
			r = old[0]
			remaining[v] = old[1:]
		} else {
			r = Radio(g.selected, v, g.label(v))
			r.Key(r)
		}
		r.OnChange(g.onChange)
		next = append(next, r)
	}
	// Removed handlers can still be referenced by an in-progress pointer event.
	for _, rs := range remaining {
		for _, r := range rs {
			r.onTap = nil
			r.SetInert(true)
		}
	}
	g.options = slices.Clone(options)
	g.radios = next
	g.row = ggui.Row(g.children()...)
	if g.column != nil {
		g.column = ggui.Column(g.children()...)
	}
	return true
}

// Format sets how each option is shown.
func (g *RadiosWidget[T]) Format(fn func(T) string) *RadiosWidget[T] {
	g.label = fn
	for _, r := range g.radios {
		name := fn(r.value)
		r.SetName(name)
		r.label = ggui.Text(name)
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
func (g *RadiosWidget[T]) Gap(v float64) *RadiosWidget[T] {
	defer property.Watch(&g.props, &g.gap)()
	defer property.Watch(&g.props, &g.gapSet)()
	g.gap, g.gapSet = v, true
	return g
}

// Disabled greys every option out and ignores input while v is true.
func (g *RadiosWidget[T]) Disabled(v bool) *RadiosWidget[T] {
	g.SetInert(v)
	return g
}

// BindDisabled follows r for Disabled without a rebuild.
func (g *RadiosWidget[T]) BindDisabled(r ggui.Readable[bool]) *RadiosWidget[T] {
	g.BindInert(r)
	return g
}

// OnChange fires with the value after a click selected it.
func (g *RadiosWidget[T]) OnChange(fn func(T)) *RadiosWidget[T] {
	g.onChange = fn
	for _, r := range g.radios {
		r.OnChange(fn)
	}
	return g
}

// Layout implements Widget.
func (g *RadiosWidget[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer g.props.Layout()()
	g.setOptions(g.optionProp.Get())
	g.Sync()
	for _, r := range g.radios {
		r.Disabled(g.IsInert())
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
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup, Name: g.SemanticName(), Disabled: g.IsInert(), Min: 1, Max: float64(len(g.radios))}, func(dst *ggui.Canvas) {
		if g.column != nil {
			dst.Paint(g.column, r)
			return
		}
		dst.Paint(g.row, r)
	})
}

// Name sets the accessible name of the radio group, without renaming its options.
func (g *RadiosWidget[T]) Name(s string) *RadiosWidget[T] { g.SetName(s); return g }
