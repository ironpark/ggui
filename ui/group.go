package ui

import (
	"slices"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// ButtonGroupWidget joins several controls into one bordered strip, with a
// hairline between each pair. Build one with ButtonGroup.
//
// It is a container and nothing more: every child keeps its own tab stop,
// its own keyboard behavior and its own action. Ghost buttons suit it best,
// since the strip draws the border they would otherwise each draw.
type ButtonGroupWidget struct {
	nameReader ggui.Readable[string]
	props      property.Owner
	children   []ggui.Widget
	vertical   bool
	name       string

	theme uitheme.Theme
	sizes []ggui.Size
}

// ButtonGroup lines children up in one strip.
//
//	ui.ButtonGroup(
//		ui.Button("Copy", copyIt).Ghost(),
//		ui.Button("Paste", pasteIt).Ghost(),
//	)
func ButtonGroup(children ...ggui.Widget) *ButtonGroupWidget {
	return &ButtonGroupWidget{children: children, name: "Actions"}
}

// Vertical stacks the children instead of lining them up.
func (g *ButtonGroupWidget) Vertical() *ButtonGroupWidget { g.vertical = true; return g }

// Name sets the accessible name of the strip.
func (g *ButtonGroupWidget) Name(s string) *ButtonGroupWidget {
	if g.nameReader != nil {
		g.nameReader = nil
		g.props.Changed()
	}
	defer property.Watch(&g.props, &g.name)()
	g.name = s
	return g
}

// Layout implements ggui.Widget.
func (g *ButtonGroupWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer g.props.Layout()()
	if g.nameReader != nil {
		g.name = g.nameReader.Get()
	}
	g.theme = uitheme.From(env)
	g.sizes = g.sizes[:0]
	var main, cross float64
	for _, child := range g.children {
		s := child.Layout(c.Loosen(), env)
		g.sizes = append(g.sizes, s)
		if g.vertical {
			main, cross = main+s.H, max(cross, s.W)
		} else {
			main, cross = main+s.W, max(cross, s.H)
		}
	}
	if n := len(g.children); n > 1 {
		main += float64(n-1) * g.theme.BorderWidth
	}
	return c.Constrain(pick(g.vertical, ggui.Sz(cross, main), ggui.Sz(main, cross)))
}

// Paint implements ggui.Widget. The strip is one group, so a screen reader
// reads the actions as belonging together.
func (g *ButtonGroupWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleToolbar, Name: g.name}, func(dst *ggui.Canvas) {
		g.paint(dst, r)
	})
}

func (g *ButtonGroupWidget) paint(dst *ggui.Canvas, r ggui.Rect) {
	t := g.theme
	inner := dst.Clip(r)
	// The children are stretched across the strip, so a hairline between
	// two of them runs the whole way and the strip reads as one control.
	cross := pick(g.vertical, r.Size.W, r.Size.H)
	at := r.Origin
	for i, child := range g.children {
		s := g.sizes[i]
		if i > 0 {
			line := ggui.Rct(at, pick(g.vertical, ggui.Sz(cross, t.BorderWidth), ggui.Sz(t.BorderWidth, cross)))
			inner.FillRect(line, t.Border)
			at = at.Add(pick(g.vertical, ggui.Pt(0, t.BorderWidth), ggui.Pt(t.BorderWidth, 0)))
		}
		box := ggui.Rct(at, pick(g.vertical, ggui.Sz(cross, s.H), ggui.Sz(s.W, cross)))
		inner.Paint(child, box)
		at = at.Add(pick(g.vertical, ggui.Pt(0, s.H), ggui.Pt(s.W, 0)))
	}
	dst.StrokeRoundRect(r, t.Radius, t.BorderWidth, t.Border)
}

// ToggleGroupWidget is a segmented single choice: one option of several,
// picked by clicking a segment or by the arrow keys. Build one with
// ToggleGroup.
//
// The group is one tab stop, as a set of radio buttons is. Left and Right
// (Up and Down when Vertical) move the choice and wrap around, Home and End
// go to the ends, and Space or Enter re-picks where the choice already is.
type ToggleGroupWidget[T comparable] struct {
	optionProp     property.Value[[]T]
	label          func(T) string
	optionsVersion uint64

	props            property.Owner
	ggui.Interactive // hover holds the segment under the pointer instead
	value            ggui.Binding[T]
	options          []T
	labels           []*ggui.TextWidget
	names            []string
	icon             func(T) *icons.Widget // nil without icons
	icons            []*icons.Widget       // by option, nil where the option has none
	iconOnly         bool
	tips             []*TooltipWidget // by option, for IconOnly
	onChange         func(T)
	vertical         bool
	hover            int

	theme uitheme.Theme
	sizes []ggui.Size    // each segment's content
	parts []segmentParts // and what is in it
	rects []ggui.Rect
	pad   ggui.EdgeInsets
	cur   int // the chosen segment, found once a frame by paint
}

// ToggleGroup creates one segment per option, bound to value and labelled
// through fmt.Sprint until Format says otherwise. The options slice is
// shallow-copied; configure the group before layout.
//
//	align := ggui.State("left")
//	ui.ToggleGroup(align, []string{"left", "center", "right"})
func ToggleGroup[T comparable](value ggui.Binding[T]) *ToggleGroupWidget[T] {
	g := &ToggleGroupWidget[T]{value: value, label: sprint[T], hover: -1}
	g.Role = ggui.RoleGroup
	g.AutoKey()
	if g.HitID() == nil {
		g.Key(g)
	}
	return g
}

// Options replaces the option snapshot without changing the selected value.
func (g *ToggleGroupWidget[T]) Options(options []T) *ToggleGroupWidget[T] {
	changed := g.setOptions(options)
	detached := g.optionProp.Set(g.options)
	if changed || detached {
		g.props.Changed()
	}
	return g
}

// BindOptions follows a non-nil options reader during layout.
func (g *ToggleGroupWidget[T]) BindOptions(r ggui.Readable[[]T]) *ToggleGroupWidget[T] {
	if g.optionProp.Bind(r, "BindOptions") {
		g.props.Changed()
	}
	return g
}
func (g *ToggleGroupWidget[T]) setOptions(options []T) bool {
	if slices.Equal(g.options, options) {
		return false
	}
	g.options = slices.Clone(options)
	g.optionsVersion++
	g.hover = -1
	g.cur = -1
	g.names = make([]string, len(options))
	g.labels = make([]*ggui.TextWidget, len(options))
	g.sizes = nil
	g.rects = nil
	for i, v := range options {
		g.names[i] = g.label(v)
		g.labels[i] = ggui.Text(g.names[i]).NoWrap()
	}
	g.makeIcons()
	return true
}

// makeIcons makes the segments' icons, and for IconOnly the tooltips
// naming them.
func (g *ToggleGroupWidget[T]) makeIcons() {
	g.icons, g.tips = nil, nil
	if g.icon == nil {
		return
	}
	g.icons = make([]*icons.Widget, len(g.options))
	for i, v := range g.options {
		g.icons[i] = g.icon(v)
	}
	if g.iconOnly {
		g.tips = make([]*TooltipWidget, len(g.options))
		for i := range g.options {
			g.tips[i] = Tooltip(ggui.Box(), g.names[i])
		}
	}
}

// Icon puts an icon before each option's label, tinted as the label is;
// fn may return nil for an option without one.
//
//	ui.ToggleGroup(view).Options([]string{"grid", "list"}).
//		Icon(func(v string) *icons.Widget { return icons.Icon(glyphs[v]) })
func (g *ToggleGroupWidget[T]) Icon(fn func(T) *icons.Widget) *ToggleGroupWidget[T] {
	g.icon = fn
	g.makeIcons()
	g.props.Changed()
	return g
}

// IconOnly shows the icons without the labels, which name the segments
// for a screen reader and in a tooltip each.
func (g *ToggleGroupWidget[T]) IconOnly() *ToggleGroupWidget[T] {
	g.iconOnly = true
	g.makeIcons()
	g.props.Changed()
	return g
}

// segmentParts are the sizes of a segment's icon and label, zero for one
// it does not show.
type segmentParts struct{ icon, label ggui.Size }

// iconGap is the space between a segment's icon and its label.
const iconGap = 6

// Format sets how each option is shown and named.
func (g *ToggleGroupWidget[T]) Format(fn func(T) string) *ToggleGroupWidget[T] {
	g.label = fn
	for i, o := range g.options {
		g.names[i] = fn(o)
		g.labels[i].Content(g.names[i])
	}
	g.makeIcons()
	return g
}

// Name sets the accessible name of the group.
func (g *ToggleGroupWidget[T]) Name(s string) *ToggleGroupWidget[T] { g.SetName(s); return g }

// Vertical stacks the segments instead of lining them up.
func (g *ToggleGroupWidget[T]) Vertical() *ToggleGroupWidget[T] { g.vertical = true; return g }

// Disabled greys every segment out and ignores input while v is true.
func (g *ToggleGroupWidget[T]) Disabled(v bool) *ToggleGroupWidget[T] { g.SetInert(v); return g }

// BindDisabled follows r for Disabled without a rebuild.
func (g *ToggleGroupWidget[T]) BindDisabled(r ggui.Readable[bool]) *ToggleGroupWidget[T] {
	g.BindInert(r)
	return g
}

// OnChange fires with the value after the user picked another segment.
func (g *ToggleGroupWidget[T]) OnChange(fn func(T)) *ToggleGroupWidget[T] { g.onChange = fn; return g }

// index returns the segment the binding currently names, or -1.
func (g *ToggleGroupWidget[T]) index() int {
	cur := ggui.Untrack(g.value.Get)
	for i, o := range g.options {
		if o == cur {
			return i
		}
	}
	return -1
}

func (g *ToggleGroupWidget[T]) pick(i int) {
	if i >= 0 && i < len(g.options) && !g.IsInert() {
		setChanged(g.value, g.options[i], g.onChange)
	}
}

// Layout implements ggui.Widget.
func (g *ToggleGroupWidget[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer g.props.Layout()()
	g.setOptions(g.optionProp.Get())
	g.Sync()
	t := uitheme.From(env)
	g.theme, g.pad = t, t.TabPad
	g.sizes = g.sizes[:0]
	cur := g.index()
	var main, cross float64
	g.parts = g.parts[:0]
	for i, l := range g.labels {
		fg := pick(g.IsInert(), t.MutedFg, pick(i == cur, t.Fg, t.MutedFg))
		l.Color(fg)
		var p segmentParts
		if !g.iconOnly || g.iconAt(i) == nil {
			p.label = l.Layout(ggui.Loose(ggui.Sz(ggui.Unbounded, c.MaxH)), env)
		}
		s := p.label
		if ic := g.iconAt(i); ic != nil {
			p.icon = ic.Color(fg).Layout(ggui.Loose(ggui.Sz(ggui.Unbounded, c.MaxH)), env)
			if s.W > 0 {
				s.W += iconGap
			}
			s = ggui.Sz(s.W+p.icon.W, max(s.H, p.icon.H))
		}
		g.parts = append(g.parts, p)
		g.sizes = append(g.sizes, s)
		if g.vertical {
			main, cross = main+s.H+g.pad.Top+g.pad.Bottom, max(cross, s.W+g.pad.Left+g.pad.Right)
		} else {
			main, cross = main+s.W+g.pad.Left+g.pad.Right, max(cross, s.H+g.pad.Top+g.pad.Bottom)
		}
	}
	for i, tip := range g.tips {
		s := g.sizes[i]
		tip.Layout(ggui.Tight(ggui.Sz(s.W+g.pad.Left+g.pad.Right, s.H+g.pad.Top+g.pad.Bottom)), env)
	}
	main, cross = main+2*tabInset, cross+2*tabInset
	return c.Constrain(pick(g.vertical, ggui.Sz(cross, main), ggui.Sz(main, cross)))
}

// iconAt is option i's icon, or nil.
func (g *ToggleGroupWidget[T]) iconAt(i int) *icons.Widget {
	if i < len(g.icons) {
		return g.icons[i]
	}
	return nil
}

// Paint implements ggui.Widget. The group is one node holding a segment
// each, which the hit regions alone could not say: the group's key region
// and the segments' pointer regions are siblings.
func (g *ToggleGroupWidget[T]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.DescribeNode(r, g, func(dst *ggui.Canvas) { g.paint(dst, r) })
}

func (g *ToggleGroupWidget[T]) paint(dst *ggui.Canvas, r ggui.Rect) {
	t := g.theme
	dst = dst.Clip(r)
	dst.FillRoundRect(r, t.Radius, t.Muted)
	if !g.IsInert() && len(g.options) > 0 {
		dst.HitKey(r, g)
	}
	// Layout is skipped on a still frame, so the chosen segment is found
	// here, where the segments' own nodes read it back.
	g.cur = g.index()
	cur, hover := g.cur, pick(g.IsInert(), -1, g.hover)
	g.rects = g.rects[:0]
	at := r.Origin.Add(ggui.Pt(tabInset, tabInset))
	cross := pick(g.vertical, r.Size.W, r.Size.H) - 2*tabInset
	radius := max(t.Radius-tabInset, 0)
	for i, s := range g.sizes {
		main := pick(g.vertical, s.H+g.pad.Top+g.pad.Bottom, s.W+g.pad.Left+g.pad.Right)
		seg := ggui.Rct(at, pick(g.vertical, ggui.Sz(cross, main), ggui.Sz(main, cross)))
		g.rects = append(g.rects, seg)
		dst.Describe(seg, toggleSegment[T]{g: g, i: i, version: g.optionsVersion})
		if !g.IsInert() {
			dst.HitPointer(seg, toggleSegment[T]{g: g, i: i, version: g.optionsVersion})
			dst.HitCursor(seg, ggui.CursorShapePointer)
		}
		switch {
		case i == cur:
			dst.Shadow(seg, radius, t.CardShadow)
			dst.FillRoundRect(seg, radius, t.Card)
		case i == hover:
			dst.FillRoundRect(seg, radius, mix(t.Muted, t.Fg, t.HoverMix))
		}
		// The icon and the label, centred together and each on the line.
		p, x := g.parts[i], seg.Origin.X+(seg.Size.W-s.W)/2
		middle := func(sz ggui.Size) ggui.Rect {
			return ggui.Rct(ggui.Pt(x, seg.Origin.Y+(seg.Size.H-sz.H)/2), sz)
		}
		if ic := g.iconAt(i); ic != nil {
			dst.Paint(ic, middle(p.icon))
			x += p.icon.W + iconGap
		}
		if p.label.W > 0 {
			dst.Paint(g.labels[i], middle(p.label))
		}
		if i < len(g.tips) {
			dst.Paint(g.tips[i], seg)
		}
		at = at.Add(pick(g.vertical, ggui.Pt(0, main), ggui.Pt(main, 0)))
	}
	if cur >= 0 {
		g.FocusRing(dst, g.rects[cur], radius, t.Ring)
	}
}

// HandleKey implements ggui.KeyHandler: the arrows move the choice.
func (g *ToggleGroupWidget[T]) HandleKey(ev ggui.KeyEvent) {
	g.Keyboard(ev, func() { g.pick(g.index()) })
	if ev.Kind != ggui.KeyPress || len(g.options) == 0 {
		return
	}
	switch ev.Key {
	case ggui.KeyArrowLeft, ggui.KeyArrowUp:
		g.pick(stepIndex(g.index(), -1, len(g.options), nil))
	case ggui.KeyArrowRight, ggui.KeyArrowDown:
		g.pick(stepIndex(g.index(), 1, len(g.options), nil))
	case ggui.KeyHome:
		g.pick(0)
	case ggui.KeyEnd:
		g.pick(len(g.options) - 1)
	}
}

// ConsumesKey implements ggui.KeyConsumer: the arrows, Home and End move.
func (g *ToggleGroupWidget[T]) ConsumesKey(ev ggui.KeyEvent) bool {
	switch ev.Key {
	case ggui.KeyArrowLeft, ggui.KeyArrowRight, ggui.KeyArrowUp, ggui.KeyArrowDown, ggui.KeyHome, ggui.KeyEnd:
		return ev.Kind == ggui.KeyPress
	}
	return g.Interactive.ConsumesKey(ev)
}

// Adopt implements ggui.Adopter.
func (g *ToggleGroupWidget[T]) Adopt(prev any) {
	g.Interactive.Adopt(prev)
	if p, ok := prev.(*ToggleGroupWidget[T]); ok {
		g.hover = p.hover
	}
}

// toggleSegment is one option's pointer handler and node.
type toggleSegment[T comparable] struct {
	version uint64
	g       *ToggleGroupWidget[T]
	i       int
}

// Semantics implements ggui.Semantic.
func (s toggleSegment[T]) Semantics() (ggui.Role, string) {
	if !s.valid() {
		return ggui.RoleRadio, ""
	}
	return ggui.RoleRadio, s.g.names[s.i]
}
func (s toggleSegment[T]) valid() bool {
	return s.version == s.g.optionsVersion && s.i >= 0 && s.i < len(s.g.options)
}

// Describe implements ggui.Describer: a segment is one of a set, and says
// whether it is the one chosen.
func (s toggleSegment[T]) Describe() ggui.Node {
	if !s.valid() {
		return ggui.Node{Role: ggui.RoleRadio, Disabled: true}
	}
	chosen := s.i == s.g.cur
	return ggui.Node{
		Role:     ggui.RoleRadio,
		Name:     s.g.names[s.i],
		Checked:  ggui.Tri(chosen),
		Selected: chosen,
		Disabled: s.g.IsInert(),
		Actions:  ggui.ActionSelect | ggui.ActionPress | ggui.ActionFocus,
	}
}

// Act implements ggui.Actor.
func (s toggleSegment[T]) Act(a ggui.Action) bool {
	if !s.valid() {
		return false
	}
	if s.g.IsInert() || (a.Kind != ggui.ActionSelect && a.Kind != ggui.ActionPress) {
		return false
	}
	s.g.pick(s.i)
	return true
}

func (s toggleSegment[T]) HandlePointer(ev ggui.PointerEvent) bool {
	if !s.valid() {
		return false
	}
	return hoverPick(ev, s.i, &s.g.hover, func() { s.g.pick(s.i) })
}

func (s toggleSegment[T]) Adopt(prev any) {
	if p, ok := prev.(toggleSegment[T]); ok && s.valid() && p.valid() && p.g.hover == p.i {
		s.g.hover = s.i
	}
}

func (g *ToggleGroupWidget[T]) name() string {
	return pick(g.SemanticName() != "", g.SemanticName(), "Options")
}

// Semantics implements ggui.Semantic, including the built-in fallback name.
func (g *ToggleGroupWidget[T]) Semantics() (ggui.Role, string) { return g.Role, g.name() }

// BindName follows a non-nil accessible-name reader.
func (g *ButtonGroupWidget) BindName(r ggui.Readable[string]) *ButtonGroupWidget {
	property.Require(r, "BindName")
	if !property.Same(g.nameReader, r) {
		g.nameReader = r
		g.props.Changed()
	}
	return g
}
