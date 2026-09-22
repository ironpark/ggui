package ui

import (
	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
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
	nameChild bool
	ggui.Interactive
	input             ggui.Widget
	leading, trailing ggui.Widget
	row               *ggui.RowWidget
	box               *ggui.BoxWidget
	theme             uitheme.Theme
	effectiveDisabled bool
	invalid           bool
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

	g.AutoKey()
	return g
}

// Leading places a widget before the editor.
func (g *InputGroupWidget) Leading(w ggui.Widget) *InputGroupWidget { g.leading = w; return g }

// Trailing places a widget after the editor.
func (g *InputGroupWidget) Trailing(w ggui.Widget) *InputGroupWidget { g.trailing = w; return g }

// Name names the group, and the editor inside it when that has no name.
func (g *InputGroupWidget) Name(s string) *InputGroupWidget {
	g.Interactive.SetName(s)
	if n, ok := g.input.(Named); ok {
		if g.nameChild || !n.HasName() {
			g.nameChild = true
			n.SetName(s)
		}
	}
	return g
}

// SetName is Name, for Field.
func (g *InputGroupWidget) SetName(s string) { g.Name(s) }

// HasName reports an explicit name on the group or its editor.
func (g *InputGroupWidget) HasName() bool {
	if n, ok := g.input.(Named); ok && n.HasName() {
		return true
	}
	return g.Interactive.HasName()
}

// Semantics reports the editor's resolved name, including its placeholder.
func (g *InputGroupWidget) Semantics() (ggui.Role, string) {
	if s, ok := g.input.(ggui.Semantic); ok {
		return s.Semantics()
	}
	return g.Interactive.Semantics()
}

// Disabled greys the group out and disables its editor. Addon controls remain
// independent. The editor's own Disabled and BindDisabled settings are preserved.
func (g *InputGroupWidget) Disabled(v bool) *InputGroupWidget { g.SetInert(v); return g }

// BindDisabled follows r for Disabled without a rebuild.
func (g *InputGroupWidget) BindDisabled(r ggui.Readable[bool]) *InputGroupWidget {
	g.BindInert(r)
	return g
}

// hasFocus reports whether the editor inside has the keyboard.
func (g *InputGroupWidget) hasFocus() bool { return g.focused != nil && g.focused() }

// Layout implements ggui.Widget.
func (g *InputGroupWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	if g.nameChild {
		if n, ok := g.input.(Named); ok {
			n.SetName(g.SemanticName())
		}
	}

	g.Sync()
	inherited, _ := env.Get(ggui.InputDisabled)
	g.effectiveDisabled = g.IsInert() || inherited
	t := uitheme.From(env)
	g.theme = t
	g.invalid, _ = env.Get(fieldInvalid)
	if g.row == nil {
		parts := []ggui.Widget{}
		if g.leading != nil {
			parts = append(parts, g.leading)
		}
		parts = append(parts, ggui.Expanded(groupInput{g}))
		if g.trailing != nil {
			parts = append(parts, g.trailing)
		}
		g.row = ggui.Row(parts...)
		g.box = ggui.Box(g.row)
	}
	g.row.Gap(t.Space)
	fieldBox(g.box, t, g.hasFocus(), g.effectiveDisabled)
	size := g.box.Layout(c, env)
	if editor, ok := g.input.(interface{ IsDisabled() bool }); ok {
		g.effectiveDisabled = g.effectiveDisabled || editor.IsDisabled()
	}
	fieldBox(g.box, t, g.hasFocus(), g.effectiveDisabled)
	return size
}

// Paint implements ggui.Widget.
func (g *InputGroupWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := g.theme
	// The chrome takes the click first so that the padding around the
	// editor focuses it, and the addons, painted after, sit on top.
	if h, ok := g.input.(ggui.Control); ok {
		if g.effectiveDisabled {
			dst.Describe(r, h)
		} else {
			g.Hit(dst, r, h, ggui.CursorShapeText)
		}
	} else {
		dst.Describe(r, g)
	}
	if g.hasFocus() && !g.effectiveDisabled {
		fieldHalo(dst, r, t.Radius, fieldRing(t, g.invalid))
	}
	if g.invalid {
		g.box.Border(t.BorderWidth, t.Destructive)
	}
	dst.Paint(g.box, r)
}

// groupInput applies inherited disabling only to the editor, not its addons.
type groupInput struct{ group *InputGroupWidget }

func (w groupInput) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return w.group.input.Layout(c, env.With(ggui.InputDisabled, w.group.IsInert()))
}
func (w groupInput) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(w.group.input, r) }
