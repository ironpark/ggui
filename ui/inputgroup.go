package ui

import (
	"time"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// InputGroupWidget draws one field chrome around an editor and the small
// widgets that flank it: a prefix such as "https://", a unit such as "kg",
// a clear button, a search icon. Build one with InputGroup.
//
// The group owns the border, the fill, the padding and the focus ring, so
// it takes the bare editor -- ggui.TextInput -- rather than ui.TextField,
// which draws a box of its own. Anything else given to it is an addon:
// Leading and Trailing sit beside the editor, Top and Bottom on a row of
// their own above and below it. Addon text and icons are drawn muted and
// medium, as shadcn's are, and a Button given as an addon is by default
// its ghost, extra small one. A button among the addons keeps its own
// keyboard and pointer behavior, because the group registers its region
// before painting them, and a click anywhere else in the group goes to
// the editor.
type InputGroupWidget struct {
	nameChild bool
	keyChild  bool // the editor's identity is the group's; see Layout
	ggui.Interactive
	input             ggui.Widget
	leading, trailing ggui.Widget
	top, bottom       ggui.Widget
	multiline         bool
	row               *ggui.RowWidget
	theme             *uitheme.Theme
	addonEnv          ggui.Env        // what the addons are laid out under
	styled            []*ButtonWidget // the addon buttons given the group's defaults
	rowRect           ggui.Rect       // the editor's row, from the group's origin
	topRect, botRect  ggui.Rect
	motion            time.Duration
	sides             JoinedSides // the sides a ButtonGroup joins it on
	styledRadius      float64     // the radius the styled buttons were last given
	effectiveDisabled bool
	invalid           bool
	focused           func() bool
}

// InputGroup wraps an editor in a shared field chrome.
//
//	ui.InputGroup(ggui.TextInput(site).Placeholder("example.com")).
//		Leading(ggui.Text("https://")).
//		Trailing(ui.Button("Go", visit))
func InputGroup(input ggui.Widget) *InputGroupWidget {
	g := &InputGroupWidget{input: input, styledRadius: -1}
	g.Role = ggui.RoleTextField
	if f, ok := input.(interface{ Focused() bool }); ok {
		g.focused = f.Focused
	}

	g.AutoKey()
	return g
}

// Leading places a widget before the editor.
func (g *InputGroupWidget) Leading(w ggui.Widget) *InputGroupWidget {
	g.leading, g.row = g.addon(w), nil
	return g
}

// Trailing places a widget after the editor.
func (g *InputGroupWidget) Trailing(w ggui.Widget) *InputGroupWidget {
	g.trailing, g.row = g.addon(w), nil
	return g
}

// Top places a widget on a row of its own above the editor, starting at
// the group's leading edge: shadcn's block-start addon.
func (g *InputGroupWidget) Top(w ggui.Widget) *InputGroupWidget {
	g.top = g.addon(w)
	return g
}

// Bottom places a widget on a row of its own below the editor, starting at
// the group's leading edge: shadcn's block-end addon, where a composer
// keeps its actions.
func (g *InputGroupWidget) Bottom(w ggui.Widget) *InputGroupWidget {
	g.bottom = g.addon(w)
	return g
}

// Multiline makes the group a text area: the editor inside wraps and grows
// by the line, and the group is padded as shadcn's textarea is.
func (g *InputGroupWidget) Multiline() *InputGroupWidget {
	g.multiline = true
	if m, ok := g.input.(interface{ Multiline() *ggui.TextInputWidget }); ok {
		m.Multiline()
	}
	return g
}

// addon takes w as an addon. A Button still in its default look becomes
// shadcn's InputGroupButton: ghost, extra small, with a radius that
// follows the group's from inside it. A variant, size or radius the
// button was given before stays, and one set after it was handed over
// wins.
func (g *InputGroupWidget) addon(w ggui.Widget) ggui.Widget {
	b, ok := w.(*ButtonWidget)
	if !ok {
		return w
	}
	if b.variant == ButtonPrimary {
		b.Variant(ButtonGhost)
	}
	switch b.size {
	case ButtonDefault:
		b.Size(ButtonExtraSmall)
	case ButtonIcon:
		b.Size(ButtonIconExtraSmall)
	default:
		return w
	}
	if b.radius < 0 {
		g.styled = append(g.styled, b)
	}
	return w
}

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
	// The editor takes the group's input, so it is the editor's identity
	// that keeps focus across a rebuild: one without its own takes the
	// group's.
	if in, ok := g.input.(keyed); ok {
		if id := g.HitID(); id != nil && (g.keyChild || in.HitID() == nil) {
			in.SetKey(childKey{id, nil})
			g.keyChild = true
		}
	}

	g.Sync()
	inherited, _ := env.Get(ggui.InputDisabledKey)
	g.effectiveDisabled = g.IsInert() || inherited
	t := uitheme.Of(env)
	g.theme = t
	g.motion = env.Motion(t.MotionFast)
	g.sides, env = takeJoinedSides(env)
	g.invalid, _ = env.Get(fieldInvalid)
	if g.row == nil {
		parts := make([]ggui.Widget, 0, 3)
		if g.leading != nil {
			parts = append(parts, groupAddon{g, g.leading})
		}
		parts = append(parts, ggui.Expanded(groupInput{g}))
		if g.trailing != nil {
			parts = append(parts, groupAddon{g, g.trailing})
		}
		g.row = ggui.Row(parts...)
	}
	// shadcn's rounded-[calc(var(--radius)-3px)], so that a button's corner
	// runs alongside the group's.
	for _, b := range g.styled {
		if b.radius < 0 || b.radius == g.styledRadius {
			b.Radius(max(t.Radius-3, 0))
		}
	}
	g.styledRadius = max(t.Radius-3, 0)
	// Addons are muted, medium text; a disabled group dims them.
	muted := t.MutedFg
	if g.effectiveDisabled {
		muted = t.Disabled(muted)
	}
	g.addonEnv = env.WithText(ggui.TextStyle{Color: muted, Weight: t.Label.Weight})

	// A side with an addon is padded by the theme's Space, or by half of
	// it when the addon is a button, which brings its own padding; a side
	// without one keeps a field's. The editor sits three quarters of a
	// Space from an addon. These are shadcn's pl-2, its button offset and
	// pl-1.5.
	side := func(w ggui.Widget) float64 {
		switch w.(type) {
		case nil:
			return t.FieldPad.Left
		case *ButtonWidget:
			return t.Space / 2
		}
		return t.Space
	}
	left, right := side(g.leading), side(g.trailing)
	if env.Direction() == ggui.DirRTL {
		left, right = right, left
	}
	g.row.Gap(t.Space * .75)
	// A single line is as tall as a text field, its row centered; a text
	// area and a group with rows of addons take a Space above and below.
	tall := g.multiline || g.top != nil || g.bottom != nil
	padY := pick(tall, t.Space, t.Space/2)
	band := pick(tall, 0, 20+t.FieldPad.Top+t.FieldPad.Bottom-2*padY)
	blockPad, blockGap := t.FieldPad.Left, t.Space*.75

	width := max(c.MaxW-left-right, 0)
	row := g.row.Layout(ggui.Constraints{MinW: clamp(c.MinW-left-right, 0, width), MaxW: width, MaxH: max(c.MaxH-2*padY, 0)}, env)
	block := ggui.Loose(ggui.Sz(max(c.MaxW-2*blockPad, 0), c.MaxH))
	var top, bottom ggui.Size
	want := ggui.Sz(left+row.W+right, 2*padY+max(row.H, band))
	if g.top != nil {
		top = g.top.Layout(block, g.addonEnv)
		want.W, want.H = max(want.W, top.W+2*blockPad), want.H+top.H+blockGap
	}
	if g.bottom != nil {
		bottom = g.bottom.Layout(block, g.addonEnv)
		want.W, want.H = max(want.W, bottom.W+2*blockPad), want.H+bottom.H+blockGap
	}
	size := c.Constrain(want)
	// The rows of addons keep their heights at the two ends, and the
	// editor's row is centered in what is left between them.
	y, rest := padY, size.H-2*padY
	rtl := env.Direction() == ggui.DirRTL
	if g.top != nil {
		g.topRect = ggui.Rct(ggui.Pt(pick(rtl, size.W-blockPad-top.W, blockPad), y), top)
		y, rest = y+top.H+blockGap, rest-top.H-blockGap
	}
	if g.bottom != nil {
		g.botRect = ggui.Rct(ggui.Pt(pick(rtl, size.W-blockPad-bottom.W, blockPad), size.H-padY-bottom.H), bottom)
		rest -= bottom.H + blockGap
	}
	g.rowRect = ggui.Rct(ggui.Pt(left, y+max(rest-row.H, 0)/2), row)

	if editor, ok := g.input.(interface{ IsDisabled() bool }); ok {
		g.effectiveDisabled = g.effectiveDisabled || editor.IsDisabled()
	}
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
	fieldSurface(dst, g.Anchor(r), t, r, t.Radius, fieldState{g.hasFocus(), g.invalid, g.effectiveDisabled}, g.motion, g.sides.within(dst))
	place := func(at ggui.Rect) ggui.Rect { at.Origin = at.Origin.Add(r.Origin); return at }
	if g.top != nil {
		dst.Paint(g.top, place(g.topRect))
	}
	dst.Paint(g.row, place(g.rowRect))
	if g.bottom != nil {
		dst.Paint(g.bottom, place(g.botRect))
	}
}

// groupInput applies inherited disabling only to the editor, not its addons.
type groupInput struct{ group *InputGroupWidget }

func (w groupInput) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return w.group.input.Layout(c, env.With(ggui.InputDisabledKey, w.group.IsInert()))
}
func (w groupInput) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(w.group.input, r) }

// groupAddon lays an addon beside the editor out under the group's addon
// text style.
type groupAddon struct {
	group *InputGroupWidget
	child ggui.Widget
}

func (w groupAddon) Layout(c ggui.Constraints, _ ggui.Env) ggui.Size {
	return w.child.Layout(c, w.group.addonEnv)
}
func (w groupAddon) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(w.child, r) }
