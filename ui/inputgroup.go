package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// InputGroupWidget draws one field chrome around an editor and the small
// widgets that flank it: a prefix such as "https://", a unit such as "kg",
// a clear button, a search icon. Build one with InputGroup.
//
// The group owns the border, the fill, the padding and the focus ring, so
// it takes the bare editor -- ggui.TextInput -- rather than ui.TextField,
// which draws a box of its own. Anything else given to it is simply placed
// in the row; a button among the addons keeps its own keyboard and pointer
// behavior, because the group registers its region before painting them.
type InputGroupWidget struct {
	ggui.Interactive
	input             ggui.Widget
	leading, trailing ggui.Widget
	row               *ggui.RowWidget
	box               *ggui.BoxWidget
	theme             ggui.Theme
	focused           func() bool
}

// InputGroup wraps an editor in a shared field chrome.
//
//	ui.InputGroup(ggui.TextInput(site).Placeholder("example.com")).
//		Leading(ggui.Text("https://")).
//		Trailing(ui.Button("Go", visit).Ghost())
func InputGroup(input ggui.Widget) *InputGroupWidget {
	g := &InputGroupWidget{input: input}
	g.Role = ggui.RoleTextField
	if f, ok := input.(interface{ Focused() bool }); ok {
		g.focused = f.Focused
	}
	if n, ok := input.(Named); ok {
		if _, name := n.Semantics(); name != "" {
			g.Name = name
		}
	}
	g.AutoKey()
	return g
}

// Leading places a widget before the editor.
func (g *InputGroupWidget) Leading(w ggui.Widget) *InputGroupWidget { g.leading = w; return g }

// Trailing places a widget after the editor.
func (g *InputGroupWidget) Trailing(w ggui.Widget) *InputGroupWidget { g.trailing = w; return g }

// Named names the group, and the editor inside it when that has no name.
func (g *InputGroupWidget) Named(s string) *InputGroupWidget {
	g.Name = s
	if n, ok := g.input.(Named); ok {
		if _, name := n.Semantics(); name == "" {
			n.SetName(s)
		}
	}
	return g
}

// SetName is Named, for Field.
func (g *InputGroupWidget) SetName(s string) { g.Named(s) }

// Disabled greys the group out while v is true. The editor and the addons
// keep their own Disabled: this one is the chrome's.
func (g *InputGroupWidget) Disabled(v bool) *InputGroupWidget { g.Inert = v; return g }

// DisabledWhen follows r for Disabled without a rebuild.
func (g *InputGroupWidget) DisabledWhen(r ggui.Reader[bool]) *InputGroupWidget {
	g.InertWhen(r)
	return g
}

// Layout implements ggui.Widget.
func (g *InputGroupWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	g.Sync()
	t := env.Theme()
	g.theme = t
	if g.row == nil {
		parts := []ggui.Widget{}
		if g.leading != nil {
			parts = append(parts, g.leading)
		}
		parts = append(parts, ggui.Expanded(g.input))
		if g.trailing != nil {
			parts = append(parts, g.trailing)
		}
		g.row = ggui.Row(parts...)
		g.box = ggui.Box(g.row)
	}
	g.row.Gap(t.Space)
	g.box.Padding(t.FieldPad).Radius(t.Radius).Fill(pick(g.Inert, t.Card, t.Input))
	return g.box.Layout(c, env)
}

// Paint implements ggui.Widget.
func (g *InputGroupWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := g.theme
	focused := g.focused != nil && g.focused()
	g.box.Border(t.BorderWidth, pick(focused, t.Ring, t.Border))
	// The chrome takes the click first so that the padding around the
	// editor focuses it, and the addons, painted after, sit on top.
	if h, ok := g.input.(ggui.Control); ok {
		g.Hit(dst, r, h, ebiten.CursorShapeText)
	} else {
		dst.Describe(r, g)
	}
	if focused && !g.Inert {
		fieldHalo(dst, r, t.Radius, t)
	}
	dst.Paint(g.box, r)
}
