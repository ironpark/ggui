package ui

import (
	"slices"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

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
	radios      []*RadioWidget[T]
	row         *ggui.RowWidget
	column      *ggui.ColumnWidget
	horizontal  bool // set by Horizontal
	gap         float64
	gapSet      bool
	invalidProp property.Value[bool] // set by Invalid or BindInvalid
}

// Radios creates one Radio per option, bound to selected and labelled
// through fmt.Sprint until Format says otherwise, stacked in a column with
// the theme's Space between them as shadcn's RadioGroup is; Horizontal
// lines them up instead. It is the Select signature for a choice small
// enough to show all at once.
func Radios[T comparable](selected ggui.Binding[T]) *RadiosWidget[T] {
	g := &RadiosWidget[T]{selected: selected, label: sprint[T]}
	g.Role = ggui.RoleGroup
	g.row, g.column = ggui.Row(), ggui.Column()
	g.AutoKey()
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
	// Only the flow of the current direction is laid out and painted.
	g.row, g.column = ggui.Row(g.children()...), ggui.Column(g.children()...)
	return true
}

// radioPart is the part an option plays in Radios; see childKey.
type radioPart[T comparable] struct {
	value T
	nth   int
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

// Vertical stacks the options in a column, which is the default.
func (g *RadiosWidget[T]) Vertical() *RadiosWidget[T] {
	defer property.Watch(&g.props, &g.horizontal)()
	g.horizontal = false
	return g
}

// Horizontal lines the options up side by side, with twice the theme's
// Space between them, instead of stacking them.
func (g *RadiosWidget[T]) Horizontal() *RadiosWidget[T] {
	defer property.Watch(&g.props, &g.horizontal)()
	g.horizontal = true
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

// Invalid marks every option as having failed validation; see
// RadioWidget.Invalid. Options inside a Field holding an error are invalid
// without it.
func (g *RadiosWidget[T]) Invalid(v bool) *RadiosWidget[T] {
	if g.invalidProp.Set(v) {
		g.props.Changed()
	}
	return g
}

// BindInvalid follows when for Invalid without a rebuild.
func (g *RadiosWidget[T]) BindInvalid(when ggui.Readable[bool]) *RadiosWidget[T] {
	if g.invalidProp.Bind(when, "BindInvalid") {
		g.props.Changed()
	}
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
	defer g.props.Layout().EndLayout()
	g.setOptions(g.optionProp.Get())
	g.Sync()
	// Each option is keyed by the group and its value, and by how many
	// options before it share that value.
	owner, seen := ownerOf(&g.Interactive, g), map[T]int{}
	invalid := g.invalidProp.Get()
	for _, r := range g.radios {
		r.Disabled(g.IsInert())
		r.Invalid(invalid)
		r.Key(childKey{owner, radioPart[T]{r.value, seen[r.value]}})
		seen[r.value]++
	}
	t := uitheme.Of(env)
	if g.horizontal {
		return g.row.Gap(pick(g.gapSet, g.gap, t.Space*2)).Layout(c, env)
	}
	return g.column.Gap(pick(g.gapSet, g.gap, t.Space)).Layout(c, env)
}

// Paint implements Widget. The options are one group, so a screen reader
// says how many there are and which of them is chosen.
func (g *RadiosWidget[T]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup, Name: g.SemanticName(), Disabled: g.IsInert(), Min: 1, Max: float64(len(g.radios))}, func(dst *ggui.Canvas) {
		if g.horizontal {
			dst.Paint(g.row, r)
			return
		}
		dst.Paint(g.column, r)
	})
}

// Name sets the accessible name of the radio group, without renaming its options.
func (g *RadiosWidget[T]) Name(s string) *RadiosWidget[T] { g.SetName(s); return g }
