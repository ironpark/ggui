package widget

import (
	"fmt"
	"image/color"
	"slices"
	"strconv"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/inspect"
	"github.com/ironpark/ggui/pkg/gem"
	"github.com/ironpark/ggui/pkg/layout"
	"github.com/ironpark/ggui/property"
)

// Built-in widgets follow one shape: a constructor takes what the widget
// cannot do without (its text, its children), and chainable setters take the
// rest. Setters mutate and return the receiver, so
//
//	Box(Text("hi").Color(fg)).Pad(8).Fill(bg)
//
// reads as the tree it builds. Widget types end in Widget so the short names
// stay free for the constructors. Value setters work after mount; BindX borrows
// a reader and X replaces it with a literal. Configure structure, identity and
// construction modes before the first Layout.

// ContainWidget lays its child out under the constraints it is given,
// clips it to its own Rect and holds a change of size inside it there.
// Build one with Contain.
type ContainWidget struct {
	child ggui.Widget
	size  ggui.Size // the child's, as laid out
	hold  ggui.LayoutBoundary
}

// Contain clips child to the Rect it is painted in and makes it a layout
// boundary: a disclosure easing open inside, or anything else that calls
// Invalidate, is painted within it alone, and what lies beside it is left
// as the last frame painted it, while its size stays as it was. Wrap a
// panel, a card or a pane whose size its parent decides: a Box of fixed
// Size, an Expanded share of a Row, a split's pane.
//
//	widget.Row(widget.Contain(widget.Box(details).Size(320, 480)), rest)
//
// A Contain whose size does follow its content's, as one under loose
// constraints may, costs a frame painted whole the first time its content
// grows, and holds nothing from then on; the boundary around it, a Scroll's
// or another Contain's, holds the change instead. Under a Transform it
// holds nothing either.
func Contain(child ggui.Widget) *ContainWidget { return &ContainWidget{child: child} }

// Layout implements Widget.
func (w *ContainWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	inner, check := w.hold.Begin(env)
	w.size = w.child.Layout(c, inner)
	size := c.Constrain(w.size)
	baseline, based := ggui.BaselineOf(w.child)
	w.hold.End(check, c, size, baseline, based)
	return size
}

// Baseline implements Baseliner: the child's.
func (w *ContainWidget) Baseline() (float64, bool) { return ggui.BaselineOf(w.child) }

// Paint implements Widget: the child, clipped to r.
func (w *ContainWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	w.hold.Paint(dst, r)
	dst.Clip(r).Paint(w.child, ggui.Rct(r.Origin, w.size))
}

// BoxWidget paints a rectangle and lays an optional child inside its padding.
// Build one with Box.
type BoxWidget struct {
	widthProp   property.Value[float64]
	heightProp  property.Value[float64]
	props       property.Owner
	fill        shownColor
	deco        *boxDeco // shadows and gradient, made when one is first set
	radius      float64
	borderWidth float64
	borderColor shownColor

	padding    ggui.EdgeInsets
	width      float64 // 0 means "as small as the child allows"
	height     float64
	maxW, maxH float64 // 0 means no limit
	child      ggui.Widget

	childRect ggui.Rect // inside the padding, from the last Layout
}

// boxDeco is what few boxes paint: shadows and a gradient. A BoxWidget
// makes one only when one is set.
type boxDeco struct {
	shadows       []ggui.ShadowStyle
	gradient      ggui.Gradient      // as given; see BoxWidget.Gradient
	shownShadows  []ggui.ShadowStyle // shadows with their colors resolved
	shownGradient ggui.Gradient      // gradient with its colors resolved
}

// shownColor is a color property as it was given and as it is painted: the
// given color itself, or an EnvColor's value in the Env of the last layout.
type shownColor struct{ given, shown color.Color }

// set gives the property c. A widget lays out again for an EnvColor, given
// or replaced, since only layout knows its Env.
func (s *shownColor) set(c color.Color, changed func()) {
	if c == s.given {
		return
	}
	if ggui.IsEnvColor(c) || ggui.IsEnvColor(s.given) {
		changed()
	}
	s.given, s.shown = c, c
}

func (s *shownColor) layout(env ggui.Env) { s.shown = ggui.ResolveColor(s.given, env) }

// decorate returns the box's boxDeco, making it on first use.
func (b *BoxWidget) decorate() *boxDeco {
	if b.deco == nil {
		b.deco = new(boxDeco)
	}
	return b.deco
}

// Gradient fills the box with g, over its Fill and under its border, with
// the same rounded corners. A zero Gradient removes it. EnvColor stops are
// resolved at layout, as Fill's color is.
func (b *BoxWidget) Gradient(g ggui.Gradient) *BoxWidget {
	var was ggui.Gradient
	if b.deco != nil {
		was = b.deco.gradient
	}
	if property.Equal(g, was) {
		return b
	}
	if g.Themed() || was.Themed() {
		b.props.Changed()
	}
	d := b.decorate()
	d.gradient, d.shownGradient = g, g
	return b
}

// Box wraps at most one child. With no child it is an empty rectangle, sized
// with Size, Width or Height.
func Box(child ...ggui.Widget) *BoxWidget {
	b := &BoxWidget{}
	switch len(child) {
	case 0:
	case 1:
		b.child = child[0]
	default:
		panic("ggui: Box takes at most one child; wrap several in a Column")
	}
	return b
}

// Fill sets the background color. Nil paints nothing.
func (b *BoxWidget) Fill(c color.Color) *BoxWidget {
	b.fill.set(c, b.props.Changed)
	return b
}

// Shadow replaces the outer shadow layers. Calling it without arguments clears
// them. Shadows paint in argument order and do not reserve layout space.
func (b *BoxWidget) Shadow(styles ...ggui.ShadowStyle) *BoxWidget {
	if b.deco == nil && len(styles) == 0 {
		return b
	}
	d := b.decorate()
	themed := func(s ggui.ShadowStyle) bool { return ggui.IsEnvColor(s.Color) }
	if !slices.Equal(d.shadows, styles) && (slices.ContainsFunc(d.shadows, themed) || slices.ContainsFunc(styles, themed)) {
		b.props.Changed()
	}
	d.shadows = append(d.shadows[:0], styles...)
	d.shownShadows = append(d.shownShadows[:0], styles...)
	return b
}

// Radius rounds the corners of the fill and border.
func (b *BoxWidget) Radius(r float64) *BoxWidget {
	b.radius = r
	return b
}

// Border draws a line of width w in color c just inside the edge.
func (b *BoxWidget) Border(w float64, c color.Color) *BoxWidget {
	b.borderWidth = w
	b.borderColor.set(c, b.props.Changed)
	return b
}

// Pad sets padding with the CSS shorthand Insets accepts.
func (b *BoxWidget) Pad(sides ...float64) *BoxWidget {
	defer property.Watch(&b.props, &b.padding)()
	b.padding = ggui.Insets(sides...)
	return b
}

// Padding sets per-side padding.
func (b *BoxWidget) Padding(e ggui.EdgeInsets) *BoxWidget {
	defer property.Watch(&b.props, &b.padding)()
	b.padding = e
	return b
}

// Size fixes both dimensions. Zero leaves that dimension to the child.
func (b *BoxWidget) Size(w, h float64) *BoxWidget { return b.Width(w).Height(h) }

// Width fixes the width. Zero leaves it to the child.
func (b *BoxWidget) Width(w float64) *BoxWidget {
	if b.widthProp.Set(w) {
		b.props.Changed()
	}
	b.width = w
	return b
}

// BindWidth follows a non-nil width reader without rebuilding the box.
func (b *BoxWidget) BindWidth(r ggui.Readable[float64]) *BoxWidget {
	if b.widthProp.Bind(r, "BindWidth") {
		b.props.Changed()
	}
	return b
}

// Height fixes the height. Zero leaves it to the child.
func (b *BoxWidget) Height(h float64) *BoxWidget {
	if b.heightProp.Set(h) {
		b.props.Changed()
	}
	b.height = h
	return b
}

// BindHeight follows a non-nil height reader without rebuilding the box.
func (b *BoxWidget) BindHeight(r ggui.Readable[float64]) *BoxWidget {
	if b.heightProp.Bind(r, "BindHeight") {
		b.props.Changed()
	}
	return b
}

// MaxWidth caps the width, padding included, so a form or a paragraph
// stops growing in a wide window; zero removes the cap. A parent that
// demands more, such as a stretching Column, still wins: put the box in an
// Align or Center to keep it narrow there. A fixed Width above the cap is
// cut to it.
func (b *BoxWidget) MaxWidth(w float64) *BoxWidget {
	defer property.Watch(&b.props, &b.maxW)()
	b.maxW = w
	return b
}

// MaxHeight caps the height the way MaxWidth caps the width.
func (b *BoxWidget) MaxHeight(h float64) *BoxWidget {
	defer property.Watch(&b.props, &b.maxH)()
	b.maxH = h
	return b
}

// Layout implements Widget.
func (b *BoxWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer b.props.Layout().EndLayout()
	b.fill.layout(env)
	b.borderColor.layout(env)
	if d := b.deco; d != nil {
		d.shownGradient = d.gradient.Resolve(env)
		for i, s := range d.shadows {
			d.shownShadows[i].Color = ggui.ResolveColor(s.Color, env)
		}
	}
	b.width, b.height = b.widthProp.Get(), b.heightProp.Get()
	var measure layout.MeasureFunc
	if b.child != nil {
		measure = func(_ int, c ggui.Constraints) layout.Measurement { return measureWidget(b.child, c, env) }
	}
	result := layout.Box(c, layout.BoxStyle{
		Padding: b.padding, Width: b.width, Height: b.height,
		MaxWidth: b.maxW, MaxHeight: b.maxH,
	}, measure)
	b.childRect = result.Child
	return result.Size
}

// Baseline implements Baseliner: the child's, below the top padding.
func (b *BoxWidget) Baseline() (float64, bool) { return baselineAt(b.child, b.padding.Top) }

// baselineAt is BaselineOf for a child painted dy below its parent's top.
func baselineAt(w ggui.Widget, dy float64) (float64, bool) {
	b, ok := ggui.BaselineOf(w)
	return b + dy, ok
}

// Paint implements Widget.
func (b *BoxWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if b.deco != nil {
		for _, s := range b.deco.shownShadows {
			dst.Shadow(r, b.radius, s)
		}
	}
	dst.FillRoundRect(r, b.radius, b.fill.shown)
	if b.deco != nil {
		dst.FillGradientRoundRect(r, b.radius, b.deco.shownGradient)
	}
	if b.borderWidth > 0 {
		dst.StrokeRoundRect(r, b.radius, b.borderWidth, b.borderColor.shown)
	}
	if b.child != nil {
		dst.Paint(b.child, ggui.Rct(r.Origin.Add(b.childRect.Origin), b.childRect.Size))
	}
}

// InspectFields implements inspect.Fielder.
func (b *BoxWidget) InspectFields() []inspect.Field {
	out := []inspect.Field{{Key: "Box style"}, {Key: "background", Value: inspect.Color(b.fill.shown)}, {Key: "border", Value: inspect.Num(b.borderWidth) + " px " + inspect.Color(b.borderColor.shown)}, {Key: "radius", Value: inspect.Num(b.radius) + " px", Number: true}, {Key: "padding", Value: fmt.Sprintf("%s %s %s %s", inspect.Num(b.padding.Top), inspect.Num(b.padding.Right), inspect.Num(b.padding.Bottom), inspect.Num(b.padding.Left)), Number: true}}
	if b.width > 0 {
		out = append(out, inspect.Field{Key: "fixed width", Value: inspect.Num(b.width) + " px", Number: true})
	}
	if b.height > 0 {
		out = append(out, inspect.Field{Key: "fixed height", Value: inspect.Num(b.height) + " px", Number: true})
	}
	if b.maxW > 0 {
		out = append(out, inspect.Field{Key: "max width", Value: inspect.Num(b.maxW) + " px", Number: true})
	}
	if b.maxH > 0 {
		out = append(out, inspect.Field{Key: "max height", Value: inspect.Num(b.maxH) + " px", Number: true})
	}
	return out
}

// InspectBox implements inspect.Boxer: the box model as painted at size,
// its padding and border around the child, or around the space left when
// it has none.
func (b *BoxWidget) InspectBox(size ggui.Size) inspect.Box {
	content := b.childRect.Size
	if b.child == nil {
		content = ggui.Sz(max(size.W-b.padding.Horizontal(), 0), max(size.H-b.padding.Vertical(), 0))
	}
	p := b.padding
	return inspect.Box{Padding: inspect.Insets{Top: p.Top, Right: p.Right, Bottom: p.Bottom, Left: p.Left}, Border: b.borderWidth, Content: content, Valid: true}
}

// Padding surrounds child with empty space, using the CSS shorthand Insets
// accepts: Padding(w, 8), Padding(w, 4, 12) or Padding(w, 1, 2, 3, 4). It is
// a Box with no fill, so the rest of Box's setters stay available.
func Padding(child ggui.Widget, sides ...float64) *BoxWidget { return Box(child).Pad(sides...) }

// Justify distributes children along a flow's main axis.
type Justify = layout.Justify

const (
	JustifyStart  = layout.JustifyStart
	JustifyCenter = layout.JustifyCenter
	JustifyEnd    = layout.JustifyEnd
	SpaceBetween  = layout.SpaceBetween
	SpaceAround   = layout.SpaceAround
	SpaceEvenly   = layout.SpaceEvenly
)

// CrossAlign places children across a flow's main axis.
type CrossAlign = layout.CrossAlign

const (
	AlignStart    = layout.AlignStart
	AlignCenter   = layout.AlignCenter
	AlignEnd      = layout.AlignEnd
	AlignStretch  = layout.AlignStretch
	AlignBaseline = layout.AlignBaseline
)

// flow lays children out along one axis. Column and Row are the two
// orientations of it. FlexWidget children share whatever main-axis space the
// others leave, weighted by their flex.
type flow struct {
	horizontal bool
	gap        float64
	space      float64 // gap in environment spacing units, when set
	justify    Justify
	align      CrossAlign
	children   []ggui.Widget

	rects  []ggui.Rect // borrowed from engine, or EachWidget.virtualRects on its virtual paths
	engine layout.Scratch
	items  []layout.Item
}

// Baseline implements Baseliner: the first child's that has one, moved by
// where it went. Under AlignBaseline every child was placed on the shared
// baseline, so that is what the first one reports. The engine's
// Result.Baseline is not used: layout only supplies child baselines under
// AlignBaseline, so it would be missing for every other alignment.
func (f *flow) Baseline() (float64, bool) {
	for i, rect := range f.rects { // empty before the first layout
		child := f.children[i]
		if ggui.IsVacant(child) {
			continue
		}
		if b, ok := baselineAt(child, rect.Origin.Y); ok {
			return b, true
		}
	}
	return 0, false
}

// axis is the flow's main axis in the layout package's terms; main, cross,
// size and constraints read sizes and build bounds along it.
func (f *flow) axis() layout.Axis { return gem.Pick(f.horizontal, layout.Horizontal, layout.Vertical) }

func (f *flow) main(s ggui.Size) float64  { return f.axis().Main(s) }
func (f *flow) cross(s ggui.Size) float64 { return f.axis().Cross(s) }

func (f *flow) size(main, cross float64) ggui.Size { return f.axis().Size(main, cross) }

func (f *flow) constraints(mainMin, mainMax, crossMin, crossMax float64) ggui.Constraints {
	return f.axis().Constraints(mainMin, mainMax, crossMin, crossMax)
}

// stretched returns the cross-axis size to give a child (v = 0) or the
// flow itself (v = the widest child): with AlignStretch it fills the
// available cross space when that is bounded.
func (f *flow) stretched(crossMax, v float64) float64 { return f.align.Stretched(crossMax, v) }

// placeRow puts virtual row j at main along the axis, aligned across a
// list that came out as big as result.
func (f *flow) placeRow(j int, main float64, result ggui.Size) {
	s := f.rects[j].Size
	f.rects[j].Origin = f.axis().Point(main, (f.cross(result)-f.cross(s))*f.align.Fraction())
}

// gapFor returns the gap to use: Space times the environment unit, else Gap.
func (f *flow) gapFor(env ggui.Env) float64 {
	if f.space > 0 {
		return f.space * env.Spacing()
	}
	return f.gap
}

func (f *flow) layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	f.items = resize(f.items, len(f.children))
	for i, child := range f.children {
		if fw, ok := child.(*FlexWidget); ok {
			f.items[i] = layout.Item{Weight: fw.flex, Loose: fw.loose}
		}
	}
	byBaseline := f.horizontal && f.align == AlignBaseline
	result := f.engine.Flow(c, layout.FlowStyle{
		Axis: f.axis(), Gap: f.gapFor(env), Justify: f.justify, Align: f.align,
		Direction: env.Direction(),
	}, f.items, func(i int, c ggui.Constraints) layout.Measurement {
		m := measureWidget(f.children[i], c, env)
		if byBaseline {
			m.Baseline, m.HasBaseline = ggui.BaselineOf(f.children[i])
		}
		return m
	})
	f.rects = result.Children
	return result.Size
}

// measureWidget is the boundary between widget state and geometry calculation.
func measureWidget(w ggui.Widget, c ggui.Constraints, env ggui.Env) layout.Measurement {
	size := w.Layout(c, env)
	return layout.Measurement{Size: size, Absent: ggui.IsVacant(w)}
}

// measureChildren measures children[i] for a multi-child layout.
func measureChildren(children []ggui.Widget, env ggui.Env) layout.MeasureFunc {
	return func(i int, c ggui.Constraints) layout.Measurement { return measureWidget(children[i], c, env) }
}

// paintRects paints each child at its rectangle, moved to r's origin.
func paintRects(dst *ggui.Canvas, r ggui.Rect, children []ggui.Widget, rects []ggui.Rect) {
	for i, child := range children {
		dst.Paint(child, ggui.Rct(r.Origin.Add(rects[i].Origin), rects[i].Size))
	}
}

func (f *flow) paint(dst *ggui.Canvas, r ggui.Rect) { paintRects(dst, r, f.children, f.rects) }

func (f *flow) inspectFields() []inspect.Field {
	justify := []string{"start", "center", "end", "space-between", "space-around", "space-evenly"}
	align := []string{"start", "center", "end", "stretch"}
	out := []inspect.Field{{Key: "Flex layout"}, {Key: "direction", Value: gem.Pick(f.horizontal, "row", "column")}, {Key: "justify", Value: justify[gem.Clamp(int(f.justify), 0, len(justify)-1)]}, {Key: "align", Value: align[gem.Clamp(int(f.align), 0, len(align)-1)]}}
	if f.space > 0 {
		return append(out, inspect.Field{Key: "gap token", Value: inspect.Num(f.space) + " × env.spacing"})
	}
	return append(out, inspect.Field{Key: "gap", Value: inspect.Num(f.gap) + " px", Number: true})
}

// resize returns s with length n and every element zeroed, reusing s's array.
func resize[T any](s []T, n int) []T {
	s = slices.Grow(s[:0], n)[:n]
	clear(s)
	return s
}

// ColumnWidget stacks its children vertically. Build one with Column.
type ColumnWidget struct {
	props property.Owner
	flow
}

// Column stacks children top to bottom.
func Column(children ...ggui.Widget) *ColumnWidget {
	return &ColumnWidget{children: children}
}

// Gap sets the space between consecutive children.
func (col *ColumnWidget) Gap(v float64) *ColumnWidget {
	defer property.Watch(&col.props, &col.gap)()
	col.gap = v
	return col
}

// Space sets the gap to n times the environment spacing unit, resolved at layout.
func (col *ColumnWidget) Space(n float64) *ColumnWidget {
	defer property.Watch(&col.props, &col.space)()
	col.space = n
	return col
}

// Justify distributes children along the vertical axis.
func (col *ColumnWidget) Justify(j Justify) *ColumnWidget {
	defer property.Watch(&col.props, &col.justify)()
	col.justify = j
	return col
}

// Align places children horizontally within the column.
func (col *ColumnWidget) Align(a CrossAlign) *ColumnWidget {
	defer property.Watch(&col.props, &col.align)()
	col.align = a
	return col
}

// Stretch makes every child as wide as the column: Align(AlignStretch),
// the arrangement of a form or a page. When the column's width is
// unbounded it is as wide as its widest child, and the others are laid out
// again at that width.
func (col *ColumnWidget) Stretch() *ColumnWidget { return col.Align(AlignStretch) }

// Layout implements Widget.
func (col *ColumnWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer col.props.Layout().EndLayout()
	return col.layout(c, env)
}

// Paint implements Widget.
func (col *ColumnWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { col.paint(dst, r) }

// InspectFields implements inspect.Fielder.
func (col *ColumnWidget) InspectFields() []inspect.Field { return col.flow.inspectFields() }

// InspectKind implements inspect.Kinder.
func (col *ColumnWidget) InspectKind() string { return "flex" }

// RowWidget lines its children up horizontally. Build one with Row.
type RowWidget struct {
	props property.Owner
	flow
}

// Row lines children up left to right, centered on the row's height; a
// Column starts its children at the left.
func Row(children ...ggui.Widget) *RowWidget {
	return &RowWidget{horizontal: true, align: AlignCenter, children: children}
}

// Gap sets the space between consecutive children.
func (row *RowWidget) Gap(v float64) *RowWidget {
	defer property.Watch(&row.props, &row.gap)()
	row.gap = v
	return row
}

// Space sets the gap to n times the environment spacing unit, resolved at layout.
func (row *RowWidget) Space(n float64) *RowWidget {
	defer property.Watch(&row.props, &row.space)()
	row.space = n
	return row
}

// Justify distributes children along the horizontal axis.
func (row *RowWidget) Justify(j Justify) *RowWidget {
	defer property.Watch(&row.props, &row.justify)()
	row.justify = j
	return row
}

// Align places children vertically within the row.
func (row *RowWidget) Align(a CrossAlign) *RowWidget {
	defer property.Watch(&row.props, &row.align)()
	row.align = a
	return row
}

// Stretch makes every child as tall as the row: Align(AlignStretch). When
// the row's height is unbounded, as in a vertical Scroll, it is as tall as
// its tallest child, and the others are laid out again at that height: two
// cards side by side come out equally tall.
func (row *RowWidget) Stretch() *RowWidget { return row.Align(AlignStretch) }

// Layout implements Widget.
func (row *RowWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer row.props.Layout().EndLayout()
	return row.layout(c, env)
}

// Paint implements Widget.
func (row *RowWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { row.paint(dst, r) }

// InspectFields implements inspect.Fielder.
func (row *RowWidget) InspectFields() []inspect.Field { return row.flow.inspectFields() }

// InspectKind implements inspect.Kinder.
func (row *RowWidget) InspectKind() string { return "flex" }

// FlexWidget marks a child of a Row or Column as one that takes a share of
// the leftover main-axis space. Anywhere else it is transparent. Build one
// with Flex, Expanded or Spacer.
type FlexWidget struct {
	flex  float64
	child ggui.Widget
	loose bool // the share is the most it takes, not what it must
}

// Flex gives child weight shares of the space its Row or Column has left
// after the rigid children are placed.
func Flex(child ggui.Widget, weight float64) *FlexWidget {
	return &FlexWidget{flex: weight, child: child}
}

// Expanded is Flex with weight 1.
func Expanded(child ggui.Widget) *FlexWidget { return Flex(child, 1) }

// Flexible is Flex with weight 1 that takes up to its share, and no more
// than it needs: a Scroll of tabs as wide as its tabs until they fill the
// row, so what follows them follows them until it is pushed to the end.
func Flexible(child ggui.Widget) *FlexWidget { return &FlexWidget{flex: 1, child: child, loose: true} }

// Spacer is an empty Expanded: it pushes its neighbours apart.
func Spacer() *FlexWidget { return Expanded(Box()) }

// Layout implements Widget.
func (f *FlexWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return f.child.Layout(c, env)
}

// Baseline implements Baseliner: the child's.
func (f *FlexWidget) Baseline() (float64, bool) { return ggui.BaselineOf(f.child) }

// Paint implements Widget.
func (f *FlexWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(f.child, r) }

// InspectFields implements inspect.Fielder.
func (f *FlexWidget) InspectFields() []inspect.Field {
	return []inspect.Field{{Key: "flex", Value: inspect.Num(f.flex), Number: true}}
}

// StackWidget layers its children on top of each other, first at the bottom.
// Build one with Stack.
type StackWidget struct {
	engine layout.Scratch

	props    property.Owner
	expand   bool
	children []ggui.Widget

	rects []ggui.Rect // borrowed from engine
}

// Stack layers children in order, all anchored at the top-left corner. It is
// as large as its largest child unless Expand is set. Wrap a child in Align
// to place it elsewhere.
func Stack(children ...ggui.Widget) *StackWidget {
	return &StackWidget{children: children}
}

// Expand makes the stack fill the space it is given instead of hugging its
// largest child.
func (st *StackWidget) Expand() *StackWidget {
	defer property.Watch(&st.props, &st.expand)()
	st.expand = true
	return st
}

// Layout implements Widget.
func (st *StackWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer st.props.Layout().EndLayout()
	result := st.engine.Stack(c, st.expand, len(st.children), measureChildren(st.children, env))
	st.rects = result.Children
	return result.Size
}

// Paint implements Widget.
func (st *StackWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	paintRects(dst, r, st.children, st.rects)
}

// AlignWidget fills the space it is given and places one child within it.
// Build one with Align or Center.
type AlignWidget struct {
	props property.Owner
	x, y  float64
	child ggui.Widget

	childRect ggui.Rect // where child sits, from the last Layout
}

// Align fills the available space and places child in it, centered until At
// or one of Left, Right, Top, Bottom moves it.
func Align(child ggui.Widget) *AlignWidget { return &AlignWidget{x: 0.5, y: 0.5, child: child} }

// Center is Align with the child in the middle.
func Center(child ggui.Widget) *AlignWidget { return Align(child) }

// At places the child at a fraction of the free space on each axis: (0, 0)
// is the top-left corner, (1, 1) the bottom-right, (0.5, 0.5) the center.
func (a *AlignWidget) At(x, y float64) *AlignWidget {
	defer property.Watch(&a.props, &a.x)()
	defer property.Watch(&a.props, &a.y)()
	a.x, a.y = x, y
	return a
}

// Left snaps the child to the left edge.
func (a *AlignWidget) Left() *AlignWidget {
	defer property.Watch(&a.props, &a.x)()
	a.x = 0
	return a
}

// Right snaps the child to the right edge.
func (a *AlignWidget) Right() *AlignWidget {
	defer property.Watch(&a.props, &a.x)()
	a.x = 1
	return a
}

// Top snaps the child to the top edge.
func (a *AlignWidget) Top() *AlignWidget {
	defer property.Watch(&a.props, &a.y)()
	a.y = 0
	return a
}

// Bottom snaps the child to the bottom edge.
func (a *AlignWidget) Bottom() *AlignWidget {
	defer property.Watch(&a.props, &a.y)()
	a.y = 1
	return a
}

// Layout implements Widget.
func (a *AlignWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer a.props.Layout().EndLayout()
	result := layout.Align(c, a.x, a.y, func(_ int, c ggui.Constraints) layout.Measurement {
		return measureWidget(a.child, c, env)
	})
	a.childRect = result.Child
	return result.Size
}

// Baseline implements Baseliner: the child's, moved by where it sits.
func (a *AlignWidget) Baseline() (float64, bool) {
	return baselineAt(a.child, a.childRect.Origin.Y)
}

// Paint implements Widget.
func (a *AlignWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Paint(a.child, ggui.Rct(r.Origin.Add(a.childRect.Origin), a.childRect.Size))
}

// List builds one child per item and stacks them like a Column, which is
// what it returns: every Column setter applies to it.
func List[T any](items []T, item func(T) ggui.Widget) *ColumnWidget {
	return Column(ggui.Children(items, item)...)
}

// WrapWidget lines its children up like a Row and starts a new line when
// the next child would not fit. Build one with Wrap.
type WrapWidget struct {
	engine layout.Scratch

	props    property.Owner
	children []ggui.Widget
	gap      float64 // between children on a line
	runGap   float64 // between lines
	space    float64 // both, in environment spacing units, when set
	align    CrossAlign

	rects []ggui.Rect // borrowed from engine
}

// Wrap flows children left to right, wrapping onto new lines at the width
// it is given, the way words fill a paragraph; tags and toolbars want it.
func Wrap(children ...ggui.Widget) *WrapWidget { return &WrapWidget{children: children} }

// Gap sets the space between children on a line and between lines.
func (w *WrapWidget) Gap(v float64) *WrapWidget {
	defer property.Watch(&w.props, &w.gap)()
	defer property.Watch(&w.props, &w.runGap)()
	w.gap, w.runGap = v, v
	return w
}

// RunGap sets the space between lines alone.
func (w *WrapWidget) RunGap(v float64) *WrapWidget {
	defer property.Watch(&w.props, &w.runGap)()
	w.runGap = v
	return w
}

// Space sets both gaps to n times the environment spacing unit, resolved at layout.
func (w *WrapWidget) Space(n float64) *WrapWidget {
	defer property.Watch(&w.props, &w.space)()
	w.space = n
	return w
}

// Align places children vertically within their line.
func (w *WrapWidget) Align(a CrossAlign) *WrapWidget {
	defer property.Watch(&w.props, &w.align)()
	w.align = a
	return w
}

// Layout implements Widget.
func (w *WrapWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer w.props.Layout().EndLayout()
	if w.space > 0 {
		w.gap = w.space * env.Spacing()
		w.runGap = w.gap
	}
	result := w.engine.Wrap(c, layout.WrapStyle{Gap: w.gap, RunGap: w.runGap, Align: w.align}, len(w.children), measureChildren(w.children, env))
	w.rects = result.Children
	return result.Size
}

// Paint implements Widget.
func (w *WrapWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { paintRects(dst, r, w.children, w.rects) }

// InspectFields implements inspect.Fielder.
func (w *WrapWidget) InspectFields() []inspect.Field {
	return []inspect.Field{{Key: "Wrapping layout"}, {Key: "gap", Value: inspect.Num(w.gap) + " px", Number: true}, {Key: "run gap", Value: inspect.Num(w.runGap) + " px", Number: true}}
}

// InspectKind implements inspect.Kinder.
func (w *WrapWidget) InspectKind() string { return "wrap" }

// GridWidget lays its children out in equal-width columns. Build one with
// Grid.
type GridWidget struct {
	engine layout.Scratch

	props    property.Owner
	cols     int
	children []ggui.Widget
	gap      float64
	rowGap   float64
	space    float64 // both, in environment spacing units, when set

	rects []ggui.Rect // borrowed from engine
	cellW float64
}

// Grid places children in rows of cols cells, left to right then top to
// bottom. Each column takes an equal share of the width; each row is as
// tall as its tallest cell, and children are given the cell width tight, so
// text and boxes align down the columns.
func Grid(cols int, children ...ggui.Widget) *GridWidget {
	return &GridWidget{cols: max(cols, 1), children: children}
}

// Gap sets the space between columns and between rows.
func (g *GridWidget) Gap(v float64) *GridWidget {
	defer property.Watch(&g.props, &g.gap)()
	defer property.Watch(&g.props, &g.rowGap)()
	g.gap, g.rowGap = v, v
	return g
}

// RowGap sets the space between rows alone.
func (g *GridWidget) RowGap(v float64) *GridWidget {
	defer property.Watch(&g.props, &g.rowGap)()
	g.rowGap = v
	return g
}

// Space sets both gaps to n times the environment spacing unit, resolved at layout.
func (g *GridWidget) Space(n float64) *GridWidget {
	defer property.Watch(&g.props, &g.space)()
	g.space = n
	return g
}

// Layout implements Widget.
func (g *GridWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer g.props.Layout().EndLayout()
	if g.space > 0 {
		g.gap = g.space * env.Spacing()
		g.rowGap = g.gap
	}
	result := g.engine.Grid(c, layout.GridStyle{Columns: g.cols, Gap: g.gap, RowGap: g.rowGap}, len(g.children), measureChildren(g.children, env))
	g.rects, g.cellW = result.Children, result.CellWidth
	return result.Size
}

// Paint implements Widget.
func (g *GridWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { paintRects(dst, r, g.children, g.rects) }

// InspectFields implements inspect.Fielder.
func (g *GridWidget) InspectFields() []inspect.Field {
	return []inspect.Field{{Key: "Grid layout"}, {Key: "columns", Value: strconv.Itoa(g.cols), Number: true}, {Key: "column width", Value: inspect.Num(g.cellW) + " px", Number: true}, {Key: "column gap", Value: inspect.Num(g.gap) + " px", Number: true}, {Key: "row gap", Value: inspect.Num(g.rowGap) + " px", Number: true}}
}

// InspectKind implements inspect.Kinder.
func (g *GridWidget) InspectKind() string { return "grid" }

// ResponsiveWidget lays out one of two arrangements by the width it is
// given. Build one with Responsive.
type ResponsiveWidget struct {
	breakpoint   float64
	wide, narrow ggui.Widget
	current      ggui.Widget
}

// Responsive lays out wide while it is given at least breakpoint of width,
// and narrow below that. The two arrangements may hold the same widgets, so
// a control keeps its state, focus included, as the window crosses the
// breakpoint:
//
//	title, actions := ui.Title("Tasks"), ui.Button("New", create)
//	widget.Responsive(680,
//		widget.Row(widget.Expanded(title), actions),
//		widget.Column(title, actions).Stretch())
func Responsive(breakpoint float64, wide, narrow ggui.Widget) *ResponsiveWidget {
	return &ResponsiveWidget{breakpoint: breakpoint, wide: wide, narrow: narrow, current: wide}
}

// Layout implements Widget.
func (r *ResponsiveWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	r.current = gem.Pick(c.MaxW < r.breakpoint, r.narrow, r.wide)
	return r.current.Layout(c, env)
}

// Baseline implements Baseliner: the shown arrangement's.
func (r *ResponsiveWidget) Baseline() (float64, bool) { return ggui.BaselineOf(r.current) }

// Paint implements Widget.
func (r *ResponsiveWidget) Paint(dst *ggui.Canvas, rect ggui.Rect) { dst.Paint(r.current, rect) }
