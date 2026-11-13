package ui

import (
	"image/color"
	"slices"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// ButtonGroupWidget joins several controls into one strip: the corners
// between two neighbours are squared and they share one border, so the row
// reads as a single control. Build one with ButtonGroup.
//
// It is a container and nothing more: it draws no frame of its own, and
// every child keeps its own tab stop, its own keyboard behavior and its own
// action. Outline buttons suit it best. A child learns where it sits from
// JoinedSidesKey; Button, Toggle, ButtonGroupText, the text fields and
// Select follow it, and a control that does not simply keeps its own
// corners.
//
// A ButtonGroupSeparator between two children draws a line where they have
// no border to tell them apart. A group whose children are groups keeps
// them a Space apart instead of joining them.
type ButtonGroupWidget struct {
	nameReader ggui.Readable[string]
	props      property.Owner
	children   []ggui.Widget
	vertical   bool
	rtl        bool
	name       string

	theme *uitheme.Theme
	sizes []ggui.Size
	gap   float64     // between children, for a group of groups
	focus joinedFocus // the focused child's ring, drawn over the others

	// What Paint hands to enclosed, made once: a closure a frame would be
	// garbage.
	paintDst  *ggui.Canvas
	paintRect ggui.Rect
	enclosed  func()
}

// ButtonGroup lines children up in one strip.
//
//	ui.ButtonGroup(
//		ui.Button("Copy", copyIt).Outline(),
//		ui.Button("Paste", pasteIt).Outline(),
//	)
func ButtonGroup(children ...ggui.Widget) *ButtonGroupWidget {
	g := &ButtonGroupWidget{children: children, name: "Actions"}
	g.enclosed = func() { g.paint(g.paintDst, g.paintRect) }
	return g
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

// nested reports whether a child is itself a ButtonGroup, which makes this
// one a row of strips rather than a strip.
func (g *ButtonGroupWidget) nested() bool {
	for _, child := range g.children {
		if _, ok := child.(*ButtonGroupWidget); ok {
			return true
		}
	}
	return false
}

// sidesAt are the sides on which child i meets a neighbour.
func (g *ButtonGroupWidget) sidesAt(i int, nested bool) JoinedSides {
	if nested {
		return JoinedSides{}
	}
	before, after := i > 0, i < len(g.children)-1
	switch {
	case g.vertical:
		return JoinedSides{Top: before, Bottom: after}
	case g.rtl:
		return JoinedSides{Right: before, Left: after}
	}
	return JoinedSides{Left: before, Right: after}
}

// Layout implements ggui.Widget.
func (g *ButtonGroupWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer g.props.Layout().EndLayout()
	if g.nameReader != nil {
		g.name = g.nameReader.Get()
	}
	g.theme = uitheme.Of(env)
	g.rtl = env.Direction() == ggui.DirRTL
	nested := g.nested()
	g.gap = pick(nested, g.theme.Space, 0)
	g.sizes = g.sizes[:0]
	var main, cross float64
	fields := 0
	for i, child := range g.children {
		if g.grows(child) {
			fields++
			g.sizes = append(g.sizes, ggui.Size{})
			continue
		}
		s := child.Layout(c.Loosen(), env.With(JoinedSidesKey, g.sidesAt(i, nested)))
		g.sizes = append(g.sizes, s)
		if g.vertical {
			main, cross = main+s.H, max(cross, s.W)
		} else {
			main, cross = main+s.W, max(cross, s.H)
		}
	}
	main += float64(max(len(g.children)-1, 0)) * g.gap
	// Text fields share the width the other children leave, as shadcn's
	// [&>input]:flex-1 has them do.
	if fields > 0 {
		share := c.Loosen()
		share.MaxW = max(c.MaxW-main, 0) / float64(fields)
		for i, child := range g.children {
			if g.grows(child) {
				s := child.Layout(share, env.With(JoinedSidesKey, g.sidesAt(i, nested)))
				g.sizes[i] = s
				main, cross = main+s.W, max(cross, s.H)
			}
		}
	}
	out := c.Constrain(pick(g.vertical, ggui.Sz(cross, main), ggui.Sz(main, cross)))
	cross = pick(g.vertical, out.W, out.H)
	// The children are stretched across the strip, as align-items: stretch
	// does, so one that came out shorter is laid out again at its height.
	for i, child := range g.children {
		s := g.sizes[i]
		if pick(g.vertical, s.W, s.H) == cross {
			continue
		}
		tight := ggui.Constraints{MinW: cross, MaxW: cross, MaxH: c.MaxH}
		if !g.vertical {
			tight = ggui.Constraints{MinH: cross, MaxH: cross, MaxW: c.MaxW}
			if g.grows(child) {
				tight.MinW, tight.MaxW = s.W, s.W
			}
		}
		g.sizes[i] = child.Layout(tight, env.With(JoinedSidesKey, g.sidesAt(i, nested)))
	}
	return out
}

// grows reports whether child takes the width the others leave: a text
// field, a number input or an input group in a horizontal group.
func (g *ButtonGroupWidget) grows(child ggui.Widget) bool {
	switch child.(type) {
	case *TextFieldWidget, *NumberInputWidget, *InputGroupWidget:
		return !g.vertical
	}
	return false
}

// Paint implements ggui.Widget. The strip is one group, so a screen reader
// reads the actions as belonging together.
func (g *ButtonGroupWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleToolbar, Name: g.name}, func(dst *ggui.Canvas) {
		// The children find the group by what encloses them: one painted
		// here is joined, one in a popup a child opened is not.
		g.paintDst, g.paintRect = dst, r
		dst.Enclose(g, g.enclosed)
		g.paintDst = nil
	})
}

func (g *ButtonGroupWidget) paint(dst *ggui.Canvas, r ggui.Rect) {
	g.focus.on = false
	cross := pick(g.vertical, r.Size.W, r.Size.H)
	at := r.Origin
	for i, child := range g.children {
		s := g.sizes[i]
		box := ggui.Rct(at, pick(g.vertical, ggui.Sz(cross, s.H), ggui.Sz(s.W, cross)))
		dst.Paint(child, mirrorHorizontal(box, r, g.rtl && !g.vertical))
		at = at.Add(pick(g.vertical, ggui.Pt(0, s.H+g.gap), ggui.Pt(s.W+g.gap, 0)))
	}
	// Nothing clips the children, and the focused one's ring is drawn last:
	// its halo lies over its neighbours, as shadcn's focus-visible:z-10 has it.
	if g.focus.on {
		g.focus.paint(dst)
	}
}

// ButtonGroupSeparatorWidget is a line between two children of a
// ButtonGroup. Build one with ButtonGroupSeparator.
type ButtonGroupSeparatorWidget struct {
	theme *uitheme.Theme
	flat  bool // a horizontal line, in a vertical group
}

// ButtonGroupSeparator creates a line in the InputBorder color to put
// between two children of a ButtonGroup that have no border between them,
// as the two halves of a split button have none. It runs across the strip,
// whichever way the group is laid out.
//
//	ui.ButtonGroup(
//		ui.Button("Merge", merge).Secondary(),
//		ui.ButtonGroupSeparator(),
//		ui.ButtonOf(ui.Icon(icons.ChevronDown), more).Secondary().Size(ui.ButtonIcon).Name("More"),
//	)
func ButtonGroupSeparator() *ButtonGroupSeparatorWidget { return &ButtonGroupSeparatorWidget{} }

// Layout implements ggui.Widget.
func (s *ButtonGroupSeparatorWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	s.theme = uitheme.Of(env)
	sides := joinedSidesOf(env)
	s.flat = sides.Top || sides.Bottom
	w := s.theme.BorderWidth
	return c.Constrain(pick(s.flat, ggui.Sz(c.MinW, w), ggui.Sz(w, c.MinH)))
}

// Paint implements ggui.Widget. The line stops a border's width short of
// each end, as shadcn's my-px has it.
func (s *ButtonGroupSeparatorWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	w := s.theme.BorderWidth
	if s.flat {
		r = ggui.Rct(r.Origin.Add(ggui.Pt(w, 0)), ggui.Sz(max(r.Size.W-2*w, 0), r.Size.H))
	} else {
		r = ggui.Rct(r.Origin.Add(ggui.Pt(0, w)), ggui.Sz(r.Size.W, max(r.Size.H-2*w, 0)))
	}
	dst.FillRect(r, s.theme.InputBorder)
}

// ButtonGroupTextWidget is a caption inside a ButtonGroup: a unit, a
// prefix or a label on the Muted surface, bordered and joined as the
// controls beside it are. Build one with ButtonGroupText.
type ButtonGroupTextWidget struct {
	child ggui.Widget
	theme *uitheme.Theme
	sides JoinedSides
	size  ggui.Size // the child's
	pad   float64
}

// ButtonGroupText creates a text caption for a ButtonGroup.
//
//	ui.ButtonGroup(ui.ButtonGroupText("https://"), ui.TextField(host))
func ButtonGroupText(label string) *ButtonGroupTextWidget {
	return ButtonGroupTextOf(ggui.Text(label).NoWrap())
}

// ButtonGroupTextOf is ButtonGroupText around any content, such as an icon
// beside a label.
func ButtonGroupTextOf(child ggui.Widget) *ButtonGroupTextWidget {
	return &ButtonGroupTextWidget{child: child}
}

// Layout implements ggui.Widget.
func (x *ButtonGroupTextWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	x.theme, x.pad = t, t.ButtonPad.Left
	x.sides, env = takeJoinedSides(env)
	inner := ggui.Loose(ggui.Sz(max(c.MaxW-2*x.pad, 0), max(c.MaxH-2*t.BorderWidth, 0)))
	x.size = x.child.Layout(inner, env.WithText(t.Text.Merge(t.Label).Merge(ggui.TextStyle{Color: t.Fg})))
	return c.Constrain(ggui.Sz(x.size.W+2*x.pad, x.size.H+2*t.BorderWidth))
}

// Baseline implements ggui.Baseliner: the content's, at the top border. It
// is the one measured, before a ButtonGroup stretched the caption.
func (x *ButtonGroupTextWidget) Baseline() (float64, bool) {
	if child, ok := x.child.(ggui.Baseliner); ok {
		base, ok := child.Baseline()
		return base + x.theme.BorderWidth, ok
	}
	return 0, false
}

// Paint implements ggui.Widget.
func (x *ButtonGroupTextWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := x.theme
	paintJoinedBox(dst, r, t.Radius, x.sides.within(dst), t.Muted, t.BorderWidth, t.Border)
	dst.Paint(x.child, ggui.Rct(ggui.Pt(r.Origin.X+x.pad, r.Origin.Y+(r.Size.H-x.size.H)/2), x.size))
}

// ToggleGroupWidget is a row of toggles bound to one choice: one option of
// several, picked by clicking it or by the arrow keys. Build one with
// ToggleGroup, or with ToggleGroupMultiple for options pressed
// independently.
//
// Each option is drawn as a Toggle is: no surface until the pointer is
// over it or it is chosen, when it rests on the Muted one. Outline borders
// them, Size takes the Toggle's sizes, and Spacing sets the gap between
// them, where none joins them into one strip.
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
	multiple         ggui.Binding[[]T]
	onMultipleChange func([]T)
	active           int
	focusPending     bool
	options          []T
	labels           []*ggui.TextWidget
	names            []string
	icon             func(T) *icons.Widget // nil without icons
	icons            []*icons.Widget       // by option, nil where the option has none
	iconOnly         bool
	tips             []*TooltipWidget // by option, for IconOnly
	onChange         func(T)
	vertical         bool
	rtl              bool
	hover            int
	variant          *ButtonVariant
	size             *ButtonSizeVariant
	spacing          float64 // the gap, in halves of the theme's Space

	theme *uitheme.Theme
	style ButtonStyle
	sizes []ggui.Size    // each segment's content
	parts []segmentParts // and what is in it
	rects []ggui.Rect
	// segs are the segments' handlers, made once for each version of the
	// options: a segment painted as a value would be boxed twice a frame.
	segs []*toggleSegment[T]
	pad  ggui.EdgeInsets
	cur  int // the chosen segment, found once a frame by paint

	side     float64 // the least a segment measures either way
	radius   float64
	gap      float64 // between segments
	iconGap  float64 // between a segment's icon and its label
	iconSize float64
	motion   time.Duration
}

// ToggleGroup creates one segment per option, bound to value and labelled
// through fmt.Sprint until Format says otherwise. The options slice is
// shallow-copied; configure the group before layout.
//
//	align := ggui.State("left")
//	ui.ToggleGroup(align).Options([]string{"left", "center", "right"})
func ToggleGroup[T comparable](value ggui.Binding[T]) *ToggleGroupWidget[T] {
	g := &ToggleGroupWidget[T]{value: value, label: sprint[T], hover: -1, variant: ToggleDefault, size: ButtonDefault, spacing: 2}
	g.Role = ggui.RoleGroup
	g.AutoKey()
	if g.HitID() == nil {
		g.Key(g)
	}
	return g
}

// ToggleGroupMultiple binds a group of independently pressed options. Arrow keys
// move focus without changing the selection; Space and Enter toggle that option.
func ToggleGroupMultiple[T comparable](values ggui.Binding[[]T]) *ToggleGroupWidget[T] {
	g := ToggleGroup[T](nil)
	g.multiple = values
	return g
}

// OnValuesChange reports multiple selection changes with an owned snapshot.
func (g *ToggleGroupWidget[T]) OnValuesChange(fn func([]T)) *ToggleGroupWidget[T] {
	g.onMultipleChange = fn
	return g
}

func (g *ToggleGroupWidget[T]) chosen(i int) bool {
	if i < 0 || i >= len(g.options) {
		return false
	}
	if g.multiple != nil {
		return slices.Contains(ggui.Untrack(g.multiple.Get), g.options[i])
	}
	return i == g.index()
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

// Variant draws every segment in v: ToggleDefault, ToggleOutline or a
// variant of your own. A segment takes its border, its label and the
// surface under the pointer from it; the chosen one rests on Muted.
func (g *ToggleGroupWidget[T]) Variant(v *ButtonVariant) *ToggleGroupWidget[T] {
	defer property.Watch(&g.props, &g.variant)()
	g.variant = v
	return g
}

// Outline draws a border around every segment, as ToggleOutline does.
func (g *ToggleGroupWidget[T]) Outline() *ToggleGroupWidget[T] { return g.Variant(ToggleOutline) }

// Size sets the size of every segment: ButtonSmall, ButtonDefault or
// ButtonLarge, as for a Toggle.
func (g *ToggleGroupWidget[T]) Size(v *ButtonSizeVariant) *ToggleGroupWidget[T] {
	defer property.Watch(&g.props, &g.size)()
	g.size = v
	return g
}

// Spacing sets the gap between segments in shadcn/ui's spacing units, half
// the theme's Space each; the default is 2. With 0 the segments join into
// one strip: only the corners at its ends are rounded, the padding
// narrows, and outlined segments share the border between them.
func (g *ToggleGroupWidget[T]) Spacing(units float64) *ToggleGroupWidget[T] {
	defer property.Watch(&g.props, &g.spacing)()
	g.spacing = max(units, 0)
	return g
}

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
	if g.multiple != nil {
		if len(g.options) == 0 {
			return -1
		}
		return min(max(g.active, 0), len(g.options)-1)
	}
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
		if g.multiple != nil {
			g.active = i
			current := ggui.Untrack(g.multiple.Get)
			next := withValue(current, g.options[i], !slices.Contains(current, g.options[i]))
			g.multiple.Set(next)
			if g.onMultipleChange != nil {
				g.onMultipleChange(slices.Clone(next))
			}
		} else {
			setChanged(g.value, g.options[i], g.onChange)
		}
	}
}

// Layout implements ggui.Widget.
func (g *ToggleGroupWidget[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer g.props.Layout().EndLayout()
	g.setOptions(g.optionProp.Get())
	g.Sync()
	t := uitheme.Of(env)
	size := g.size.Style(*t)
	g.theme, g.style = t, g.variant.Style(*t)
	g.motion = env.Motion(t.MotionFast)
	g.rtl = env.Direction() == ggui.DirRTL
	g.gap = g.spacing * t.Space / 2
	// Joined segments are padded as shadcn's px-2 rather than px-2.5.
	g.pad = ggui.Insets(size.Pad.Top, pick(g.spacing > 0, size.Pad.Left, max(size.Pad.Left-2, 0)))
	g.side = pick(size.Height > 0, size.Height, 20+2*t.ButtonPad.Top)
	g.radius = pick(size.Radius > 0, size.Radius, t.Radius)
	g.iconGap = t.Space / 2
	g.iconSize = pick(size.IconSize > 0, size.IconSize, t.IconSize)
	fg := pick(g.IsInert(), t.Disabled(g.style.Label), g.style.Label)
	text := t.Text.Merge(t.Label).Merge(ggui.TextStyle{Color: fg})
	if size.TextSize > 0 {
		text.Size, text.LineHeight = size.TextSize, size.LineHeight
	}
	env = env.WithText(text)
	g.sizes = g.sizes[:0]
	var main, cross float64
	g.parts = g.parts[:0]
	for i, l := range g.labels {
		l.Color(fg)
		var p segmentParts
		if !g.iconOnly || g.iconAt(i) == nil {
			p.label = l.Layout(ggui.Loose(ggui.Sz(ggui.Unbounded, c.MaxH)), env)
		}
		s := p.label
		if ic := g.iconAt(i); ic != nil {
			p.icon = ic.Color(fg).Layout(ggui.Tight(ggui.Sz(g.iconSize, g.iconSize)), env)
			if s.W > 0 {
				s.W += g.iconGap
			}
			s = ggui.Sz(s.W+p.icon.W, max(s.H, p.icon.H))
		}
		g.parts = append(g.parts, p)
		g.sizes = append(g.sizes, s)
		o := g.outer(s)
		if g.vertical {
			main, cross = main+o.H, max(cross, o.W)
		} else {
			main, cross = main+o.W, max(cross, o.H)
		}
	}
	for i, tip := range g.tips {
		tip.Layout(ggui.Tight(g.outer(g.sizes[i])), env)
	}
	main += float64(max(len(g.sizes)-1, 0)) * g.gap
	return c.Constrain(pick(g.vertical, ggui.Sz(cross, main), ggui.Sz(main, cross)))
}

// outer is the size of a segment around content: padded, and at least as
// wide and as tall as the size variant's height, as a Toggle is.
func (g *ToggleGroupWidget[T]) outer(content ggui.Size) ggui.Size {
	return ggui.Sz(max(content.W+g.pad.Left+g.pad.Right, g.side), max(content.H+g.pad.Top+g.pad.Bottom, g.side))
}

// sidesAt are the sides on which segment i meets a neighbour: none unless
// Spacing is 0.
func (g *ToggleGroupWidget[T]) sidesAt(i int) JoinedSides {
	if g.spacing > 0 {
		return JoinedSides{}
	}
	before, after := i > 0, i < len(g.sizes)-1
	switch {
	case g.vertical:
		return JoinedSides{Top: before, Bottom: after}
	case g.rtl:
		return JoinedSides{Right: before, Left: after}
	}
	return JoinedSides{Left: before, Right: after}
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

var toggleSegmentSlot = ggui.NewSlot[*ggui.Motion]("toggle group segment")

func (g *ToggleGroupWidget[T]) paint(dst *ggui.Canvas, r ggui.Rect) {
	t, st := g.theme, g.style
	inert := g.IsInert()
	if g.multiple == nil && !inert && len(g.options) > 0 {
		dst.HitKey(r, g)
	}
	// Layout is skipped on a still frame, so the chosen segment is found
	// here, where the segments' own nodes read it back.
	g.cur = g.index()
	cur, hover := g.cur, pick(inert, -1, g.hover)
	g.rects = g.rects[:0]
	at := r.Origin
	cross := pick(g.vertical, r.Size.W, r.Size.H)
	border := st.Border
	if border != nil && inert {
		border = t.Disabled(border)
	}
	for i, s := range g.sizes {
		o := g.outer(s)
		main := pick(g.vertical, o.H, o.W)
		seg := ggui.Rct(at, pick(g.vertical, ggui.Sz(cross, main), ggui.Sz(main, cross)))
		seg = mirrorHorizontal(seg, r, g.rtl && !g.vertical)
		g.rects = append(g.rects, seg)
		h := g.segment(i)
		dst.Describe(seg, h)
		if !inert {
			dst.HitPointer(seg, h)
			if g.multiple != nil {
				dst.HitKey(seg, h)
			}
			dst.HitCursor(seg, ggui.CursorShapeDefault)
		}
		// A segment is a Toggle: nothing under it until it is chosen or the
		// pointer is over it, and then the surface eases in.
		on := g.chosen(i)
		shown := ease(dst, g.theme, ggui.Anchor{Rect: seg}, toggleSegmentSlot, pick(on || i == hover, 1.0, 0.0), g.motion)
		fill := st.Fill
		if shown > 0 {
			fill = mix(colorOr(st.Fill, color.Transparent), pick(on, t.Muted, colorOr(st.Hover, t.Muted)), shown)
		}
		if inert && fill != nil {
			fill = t.Disabled(fill)
		}
		paintJoinedBox(dst, seg, g.radius, g.sidesAt(i), fill, t.BorderWidth, border)
		// The icon and the label, centred together and each on the line.
		p, x := g.parts[i], seg.Origin.X+(seg.Size.W-s.W)/2
		if ic := g.iconAt(i); ic != nil {
			dst.Paint(ic, g.middle(seg, x, p.icon))
			x += p.icon.W + g.iconGap
		}
		if p.label.W > 0 {
			dst.Paint(g.labels[i], g.middle(seg, x, p.label))
		}
		if i < len(g.tips) {
			dst.Paint(g.tips[i], seg)
		}
		at = at.Add(pick(g.vertical, ggui.Pt(0, main+g.gap), ggui.Pt(main+g.gap, 0)))
	}
	if g.multiple != nil && g.focusPending && cur >= 0 {
		dst.RequestFocus(g.segment(cur))
		g.focusPending = false
	}
	// The ring is drawn after every segment, so in a joined strip its halo
	// lies over the neighbours rather than under the one painted next.
	// It eases in and out over the group's motion, as a Toggle's does.
	focused := cur >= 0 && g.Focused && g.FocusVisible && !inert
	if k := ease(dst, g.theme, g.Anchor(r), focusRingSlot, pick(focused, 1.0, 0.0), g.motion); k > 0 && cur >= 0 {
		ring := colorOr(st.Ring, t.Ring)
		f := joinedFocus{r: g.rects[cur], radius: g.radius, sides: g.sidesAt(cur), width: t.BorderWidth, line: fade(colorOr(st.FocusBorder, ring), k), halo: fade(t.Halo(ring), k)}
		f.paint(dst)
	}
}

// middle is the Rect of a part of a segment's content starting at x,
// centred on the segment's line.
func (g *ToggleGroupWidget[T]) middle(seg ggui.Rect, x float64, size ggui.Size) ggui.Rect {
	return mirrorHorizontal(ggui.Rct(ggui.Pt(x, seg.Origin.Y+(seg.Size.H-size.H)/2), size), seg, g.rtl)
}

// HandleKey implements ggui.KeyHandler: the arrows move the choice.
func (g *ToggleGroupWidget[T]) HandleKey(ev ggui.KeyEvent) {
	g.Keyboard(ev, func() { g.pick(g.index()) })
	if g.IsInert() || ev.Kind != ggui.KeyPress || len(g.options) == 0 {
		return
	}
	if i, ok := navIndex(directionalKey(ev, g.rtl && !g.vertical), navBoth, g.index(), len(g.options), nil); ok {
		if g.multiple != nil {
			g.active = i
			g.focusPending = true
		} else {
			g.pick(i)
		}
	}
}

// ConsumesKey implements ggui.KeyConsumer: the arrows, Home and End move.
func (g *ToggleGroupWidget[T]) ConsumesKey(ev ggui.KeyEvent) bool {
	if navMoveOf(ev.Key, navBoth) != navNone {
		return ev.Kind == ggui.KeyPress
	}
	return g.Interactive.ConsumesKey(ev)
}

// Adopt implements ggui.Adopter.
func (g *ToggleGroupWidget[T]) Adopt(prev any) {
	g.Interactive.Adopt(prev)
	if p, ok := prev.(*ToggleGroupWidget[T]); ok {
		g.hover = p.hover
		g.active = p.active
	}
}

// segment returns segment i's handler for the options' current version.
func (g *ToggleGroupWidget[T]) segment(i int) *toggleSegment[T] {
	if len(g.segs) > 0 && g.segs[0].version != g.optionsVersion {
		g.segs = g.segs[:0]
	}
	for len(g.segs) <= i {
		g.segs = append(g.segs, &toggleSegment[T]{g: g, i: len(g.segs), version: g.optionsVersion})
	}
	return g.segs[i]
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
	return pick(s.g.multiple != nil, ggui.RoleButton, ggui.RoleRadio), s.g.names[s.i]
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
	chosen := s.g.chosen(s.i)
	return ggui.Node{
		Role:     pick(s.g.multiple != nil, ggui.RoleButton, ggui.RoleRadio),
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
	if p, ok := prev.(*toggleSegment[T]); ok && s.valid() && p.valid() {
		s.g.Interactive.Adopt(p.g)
		s.g.active = p.g.active
		if p.g.hover == p.i {
			s.g.hover = s.i
		}
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

func (s *toggleSegment[T]) TabStop() bool {
	return s.valid() && s.g.multiple != nil && s.i == s.g.index()
}
func (s *toggleSegment[T]) HitID() any {
	if owner := s.g.HitID(); owner != nil {
		return childKey{owner: owner, part: s.i}
	}
	return nil
}
func (s *toggleSegment[T]) ConsumesKey(ev ggui.KeyEvent) bool { return s.g.ConsumesKey(ev) }
func (s *toggleSegment[T]) HandleKey(ev ggui.KeyEvent) {
	if !s.valid() {
		return
	}
	if ev.Kind == ggui.KeyFocus {
		s.g.active = s.i
	}
	s.g.HandleKey(ev)
}

// Act forwards multiple-group focus to its roving option. Single selection
// retains its existing group keyboard region and the runtime focuses that.
func (g *ToggleGroupWidget[T]) Act(a ggui.Action) bool {
	if g.multiple == nil || a.Kind != ggui.ActionFocus || g.IsInert() || g.index() < 0 {
		return false
	}
	g.focusPending = true
	return true
}
