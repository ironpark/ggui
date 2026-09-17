package ggui

import (
	"image/color"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Built-in widgets follow one shape: a constructor takes what the widget
// cannot do without (its text, its children), and chainable setters take the
// rest. Setters mutate and return the receiver, so
//
//	Box(Text("hi").Color(fg)).Pad(8).Fill(bg)
//
// reads as the tree it builds. Widget types end in Widget so the short names
// stay free for the constructors.

// EdgeInsets is padding on the four sides of a box.
type EdgeInsets struct {
	Top, Right, Bottom, Left float64
}

// Insets builds EdgeInsets with CSS shorthand: one value for every side, two
// for vertical then horizontal, four for top, right, bottom, left.
func Insets(sides ...float64) EdgeInsets {
	switch len(sides) {
	case 0:
		return EdgeInsets{}
	case 1:
		return EdgeInsets{Top: sides[0], Right: sides[0], Bottom: sides[0], Left: sides[0]}
	case 2:
		return EdgeInsets{Top: sides[0], Right: sides[1], Bottom: sides[0], Left: sides[1]}
	case 4:
		return EdgeInsets{Top: sides[0], Right: sides[1], Bottom: sides[2], Left: sides[3]}
	default:
		panic("ggui: Insets takes 1, 2 or 4 values")
	}
}

// Shrink returns constraints with the insets removed from the maximums.
func (e EdgeInsets) Shrink(c Constraints) Constraints {
	return Constraints{
		MinW: max(c.MinW-e.horizontal(), 0),
		MinH: max(c.MinH-e.vertical(), 0),
		MaxW: max(c.MaxW-e.horizontal(), 0),
		MaxH: max(c.MaxH-e.vertical(), 0),
	}
}

// Inflate returns s grown by the insets.
func (e EdgeInsets) Inflate(s Size) Size {
	return Size{W: s.W + e.horizontal(), H: s.H + e.vertical()}
}

func (e EdgeInsets) horizontal() float64 { return e.Left + e.Right }
func (e EdgeInsets) vertical() float64   { return e.Top + e.Bottom }

// TextWidget draws text, wrapping it to the width it is given. Build one
// with Text.
type TextWidget struct {
	value      string
	color      color.Color
	font       *Font
	size       float64
	lineHeight float64
	wrap       bool
	align      float64

	// Layout caches the wrapped lines and their widths, and re-wraps only
	// when the text, the face or the width it must fit in changes.
	lines   []string
	widths  []float64
	wrapped wrapKey
}

// wrapKey is the input wrapText was last run with.
type wrapKey struct {
	value string
	face  text.Face
	maxW  float64
}

// Text draws s in the default font at DefaultTextSize, wrapping at spaces
// when it is wider than the space it gets.
func Text(s string) *TextWidget {
	return &TextWidget{value: s, size: DefaultTextSize, lineHeight: 1.2, wrap: true}
}

// Color sets the text color.
func (t *TextWidget) Color(c color.Color) *TextWidget { t.color = c; return t }

// Font sets the face; nil means the default font.
func (t *TextWidget) Font(f *Font) *TextWidget { t.font = f; return t }

// Size sets the font size in pixels.
func (t *TextWidget) Size(px float64) *TextWidget { t.size = px; return t }

// LineHeight sets the distance between baselines as a multiple of Size.
func (t *TextWidget) LineHeight(mult float64) *TextWidget { t.lineHeight = mult; return t }

// NoWrap keeps the text on one line per hard line break, however wide.
func (t *TextWidget) NoWrap() *TextWidget { t.wrap = false; return t }

// Align places each line within the widget's width by fraction: 0 is left,
// 0.5 centered, 1 right.
func (t *TextWidget) Align(x float64) *TextWidget { t.align = x; return t }

func (t *TextWidget) face() text.Face {
	f := t.font
	if f == nil {
		f = fallbackFont()
	}
	return f.face(t.size)
}

// spacing is the distance between baselines.
func (t *TextWidget) spacing() float64 { return t.size * t.lineHeight }

// Layout implements Widget.
func (t *TextWidget) Layout(c Constraints) Size {
	face := t.face()
	key := wrapKey{value: t.value, face: face, maxW: pick(t.wrap, c.MaxW, 0)}
	if key != t.wrapped {
		t.wrapped = key
		t.lines = wrapText(key.value, key.face, key.maxW)
		t.widths = t.widths[:0]
		for _, line := range t.lines {
			t.widths = append(t.widths, lineWidth(line, face))
		}
	}
	var w float64
	for _, lw := range t.widths {
		w = max(w, lw)
	}
	m := face.Metrics()
	h := float64(len(t.lines)-1)*t.spacing() + m.HAscent + m.HDescent
	return c.Constrain(Sz(w, h))
}

// Paint implements Widget.
func (t *TextWidget) Paint(dst *Canvas, r Rect) {
	face := t.face()
	for i, line := range t.lines {
		op := &text.DrawOptions{}
		x := r.Origin.X + (r.Size.W-t.widths[i])*t.align
		op.GeoM.Translate(x, r.Origin.Y+float64(i)*t.spacing())
		if t.color != nil {
			op.ColorScale.ScaleWithColor(t.color)
		}
		text.Draw(dst.Image, line, face, op)
	}
}

// BoxWidget paints a rectangle and lays an optional child inside its padding.
// Build one with Box.
type BoxWidget struct {
	fill    color.Color
	padding EdgeInsets
	width   float64 // 0 means "as small as the child allows"
	height  float64
	child   Widget

	childSize Size
}

// Box wraps at most one child. With no child it is an empty rectangle, sized
// with Size, Width or Height.
func Box(child ...Widget) *BoxWidget {
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
func (b *BoxWidget) Fill(c color.Color) *BoxWidget { b.fill = c; return b }

// Pad sets padding with the CSS shorthand Insets accepts.
func (b *BoxWidget) Pad(sides ...float64) *BoxWidget { b.padding = Insets(sides...); return b }

// Padding sets per-side padding.
func (b *BoxWidget) Padding(e EdgeInsets) *BoxWidget { b.padding = e; return b }

// Size fixes both dimensions. Zero leaves that dimension to the child.
func (b *BoxWidget) Size(w, h float64) *BoxWidget { b.width, b.height = w, h; return b }

// Width fixes the width. Zero leaves it to the child.
func (b *BoxWidget) Width(w float64) *BoxWidget { b.width = w; return b }

// Height fixes the height. Zero leaves it to the child.
func (b *BoxWidget) Height(h float64) *BoxWidget { b.height = h; return b }

// Layout implements Widget.
func (b *BoxWidget) Layout(c Constraints) Size {
	// A fixed dimension is passed down tight, so a child that centers or
	// justifies does so within the box rather than the space around it.
	inner := b.padding.Shrink(c).Loosen()
	if b.width > 0 {
		w := max(clamp(b.width, c.MinW, c.MaxW)-b.padding.horizontal(), 0)
		inner.MinW, inner.MaxW = w, w
	}
	if b.height > 0 {
		h := max(clamp(b.height, c.MinH, c.MaxH)-b.padding.vertical(), 0)
		inner.MinH, inner.MaxH = h, h
	}
	b.childSize = Size{}
	if b.child != nil {
		b.childSize = b.child.Layout(inner)
	}
	want := b.padding.Inflate(b.childSize)
	if b.width > 0 {
		want.W = b.width
	}
	if b.height > 0 {
		want.H = b.height
	}
	return c.Constrain(want)
}

// Paint implements Widget.
func (b *BoxWidget) Paint(dst *Canvas, r Rect) {
	if b.fill != nil {
		vector.DrawFilledRect(dst.Image,
			float32(r.Origin.X), float32(r.Origin.Y),
			float32(r.Size.W), float32(r.Size.H),
			b.fill, true)
	}
	if b.child != nil {
		b.child.Paint(dst, Rct(r.Origin.Add(Pt(b.padding.Left, b.padding.Top)), b.childSize))
	}
}

// Padding surrounds child with empty space, using the CSS shorthand Insets
// accepts: Padding(w, 8), Padding(w, 4, 12) or Padding(w, 1, 2, 3, 4). It is
// a Box with no fill, so the rest of Box's setters stay available.
func Padding(child Widget, sides ...float64) *BoxWidget { return Box(child).Pad(sides...) }

// Justify distributes a Row's or Column's children along its main axis. Any
// value but JustifyStart makes the widget fill the main axis so there is
// space to distribute.
type Justify int

const (
	JustifyStart  Justify = iota // packed at the start (the default)
	JustifyCenter                // packed in the middle
	JustifyEnd                   // packed at the end
	SpaceBetween                 // free space split between children
	SpaceAround                  // free space split around each child
	SpaceEvenly                  // free space split evenly, edges included
)

// CrossAlign places a Row's or Column's children across its main axis.
type CrossAlign int

const (
	AlignStart   CrossAlign = iota // top of a Row, left of a Column (the default)
	AlignCenter                    // centered across
	AlignEnd                       // bottom of a Row, right of a Column
	AlignStretch                   // stretched to the widget's cross size, which fills the space given
)

// flow lays children out along one axis. Column and Row are the two
// orientations of it. FlexWidget children share whatever main-axis space the
// others leave, weighted by their flex.
type flow struct {
	horizontal bool
	gap        float64
	justify    Justify
	align      CrossAlign
	children   []Widget

	sizes   []Size
	offsets []Point
}

func (f *flow) main(s Size) float64  { return pick(f.horizontal, s.W, s.H) }
func (f *flow) cross(s Size) float64 { return pick(f.horizontal, s.H, s.W) }

func (f *flow) size(main, cross float64) Size {
	if f.horizontal {
		return Size{W: main, H: cross}
	}
	return Size{W: cross, H: main}
}

func (f *flow) constraints(mainMin, mainMax, crossMin, crossMax float64) Constraints {
	if f.horizontal {
		return Constraints{MinW: mainMin, MaxW: mainMax, MinH: crossMin, MaxH: crossMax}
	}
	return Constraints{MinH: mainMin, MaxH: mainMax, MinW: crossMin, MaxW: crossMax}
}

func (f *flow) layout(c Constraints) Size {
	n := len(f.children)
	f.sizes = resize(f.sizes, n)
	f.offsets = resize(f.offsets, n)

	mainMax, crossMax := f.main(c.Max()), f.cross(c.Max())
	var crossMin float64
	if f.align == AlignStretch {
		crossMin = bounded(crossMax, 0)
	}

	var gaps float64
	if n > 1 {
		gaps = f.gap * float64(n-1)
	}

	// Rigid children first, each offered what is left; then flex children
	// split the remainder by weight.
	used := gaps
	var totalFlex float64
	flexible := !math.IsInf(mainMax, 1) // no leftover to share on an unbounded axis
	for i, child := range f.children {
		if fw, ok := child.(*FlexWidget); ok && fw.flex > 0 && flexible {
			totalFlex += fw.flex
			continue
		}
		f.sizes[i] = child.Layout(f.constraints(0, max(mainMax-used, 0), crossMin, crossMax))
		used += f.main(f.sizes[i])
	}
	free := max(mainMax-used, 0)
	for i, child := range f.children {
		if fw, ok := child.(*FlexWidget); ok && fw.flex > 0 && flexible {
			extent := free * fw.flex / totalFlex
			f.sizes[i] = child.Layout(f.constraints(extent, extent, crossMin, crossMax))
		}
	}

	content := gaps
	var crossUsed float64
	for _, s := range f.sizes {
		content += f.main(s)
		crossUsed = max(crossUsed, f.cross(s))
	}
	mainTotal := content
	if totalFlex > 0 || f.justify != JustifyStart {
		mainTotal = bounded(mainMax, content)
	}
	crossTotal := crossUsed
	if f.align == AlignStretch {
		crossTotal = bounded(crossMax, crossUsed)
	}
	result := c.Constrain(f.size(mainTotal, crossTotal))

	lead, between := 0.0, f.gap
	if slack := max(f.main(result)-content, 0); n > 0 {
		switch f.justify {
		case JustifyCenter:
			lead = slack / 2
		case JustifyEnd:
			lead = slack
		case SpaceBetween:
			if n > 1 {
				between += slack / float64(n-1)
			}
		case SpaceAround:
			lead = slack / float64(n) / 2
			between += slack / float64(n)
		case SpaceEvenly:
			lead = slack / float64(n+1)
			between += slack / float64(n+1)
		}
	}
	pos := lead
	for i, s := range f.sizes {
		crossOff := (f.cross(result) - f.cross(s)) * f.crossFraction()
		if f.horizontal {
			f.offsets[i] = Pt(pos, crossOff)
		} else {
			f.offsets[i] = Pt(crossOff, pos)
		}
		pos += f.main(s) + between
	}
	return result
}

func (f *flow) crossFraction() float64 {
	switch f.align {
	case AlignCenter:
		return 0.5
	case AlignEnd:
		return 1
	}
	return 0
}

func (f *flow) paint(dst *Canvas, r Rect) {
	for i, child := range f.children {
		child.Paint(dst, Rct(r.Origin.Add(f.offsets[i]), f.sizes[i]))
	}
}

// resize returns s with length n and every element zeroed, reusing s's array.
func resize[T any](s []T, n int) []T {
	s = slices.Grow(s[:0], n)[:n]
	clear(s)
	return s
}

func pick[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

// ColumnWidget stacks its children vertically. Build one with Column.
type ColumnWidget struct{ flow }

// Column stacks children top to bottom.
func Column(children ...Widget) *ColumnWidget {
	return &ColumnWidget{flow{children: children}}
}

// Gap sets the space between consecutive children.
func (col *ColumnWidget) Gap(v float64) *ColumnWidget { col.gap = v; return col }

// Justify distributes children along the vertical axis.
func (col *ColumnWidget) Justify(j Justify) *ColumnWidget { col.justify = j; return col }

// Align places children horizontally within the column.
func (col *ColumnWidget) Align(a CrossAlign) *ColumnWidget { col.align = a; return col }

// Layout implements Widget.
func (col *ColumnWidget) Layout(c Constraints) Size { return col.layout(c) }

// Paint implements Widget.
func (col *ColumnWidget) Paint(dst *Canvas, r Rect) { col.paint(dst, r) }

// RowWidget lines its children up horizontally. Build one with Row.
type RowWidget struct{ flow }

// Row lines children up left to right.
func Row(children ...Widget) *RowWidget {
	return &RowWidget{flow{horizontal: true, children: children}}
}

// Gap sets the space between consecutive children.
func (row *RowWidget) Gap(v float64) *RowWidget { row.gap = v; return row }

// Justify distributes children along the horizontal axis.
func (row *RowWidget) Justify(j Justify) *RowWidget { row.justify = j; return row }

// Align places children vertically within the row.
func (row *RowWidget) Align(a CrossAlign) *RowWidget { row.align = a; return row }

// Layout implements Widget.
func (row *RowWidget) Layout(c Constraints) Size { return row.layout(c) }

// Paint implements Widget.
func (row *RowWidget) Paint(dst *Canvas, r Rect) { row.paint(dst, r) }

// FlexWidget marks a child of a Row or Column as one that takes a share of
// the leftover main-axis space. Anywhere else it is transparent. Build one
// with Flex, Expanded or Spacer.
type FlexWidget struct {
	flex  float64
	child Widget
}

// Flex gives child weight shares of the space its Row or Column has left
// after the rigid children are placed.
func Flex(child Widget, weight float64) *FlexWidget { return &FlexWidget{flex: weight, child: child} }

// Expanded is Flex with weight 1.
func Expanded(child Widget) *FlexWidget { return Flex(child, 1) }

// Spacer is an empty Expanded: it pushes its neighbours apart.
func Spacer() *FlexWidget { return Expanded(Box()) }

// Layout implements Widget.
func (f *FlexWidget) Layout(c Constraints) Size { return f.child.Layout(c) }

// Paint implements Widget.
func (f *FlexWidget) Paint(dst *Canvas, r Rect) { f.child.Paint(dst, r) }

// StackWidget layers its children on top of each other, first at the bottom.
// Build one with Stack.
type StackWidget struct {
	expand   bool
	children []Widget

	sizes []Size
}

// Stack layers children in order, all anchored at the top-left corner. It is
// as large as its largest child unless Expand is set. Wrap a child in Align
// to place it elsewhere.
func Stack(children ...Widget) *StackWidget {
	return &StackWidget{children: children}
}

// Expand makes the stack fill the space it is given instead of hugging its
// largest child.
func (st *StackWidget) Expand() *StackWidget { st.expand = true; return st }

// Layout implements Widget.
func (st *StackWidget) Layout(c Constraints) Size {
	st.sizes = st.sizes[:0]
	var total Size
	for _, child := range st.children {
		s := child.Layout(c.Loosen())
		st.sizes = append(st.sizes, s)
		total.W = max(total.W, s.W)
		total.H = max(total.H, s.H)
	}
	if st.expand {
		total = Sz(bounded(c.MaxW, total.W), bounded(c.MaxH, total.H))
	}
	return c.Constrain(total)
}

// Paint implements Widget.
func (st *StackWidget) Paint(dst *Canvas, r Rect) {
	for i, child := range st.children {
		child.Paint(dst, Rct(r.Origin, st.sizes[i]))
	}
}

// AlignWidget fills the space it is given and places one child within it.
// Build one with Align or Center.
type AlignWidget struct {
	x, y  float64
	child Widget

	childSize Size
}

// Align fills the available space and places child in it, centered until At
// or one of Left, Right, Top, Bottom moves it.
func Align(child Widget) *AlignWidget { return &AlignWidget{x: 0.5, y: 0.5, child: child} }

// Center is Align with the child in the middle.
func Center(child Widget) *AlignWidget { return Align(child) }

// At places the child at a fraction of the free space on each axis: (0, 0)
// is the top-left corner, (1, 1) the bottom-right, (0.5, 0.5) the center.
func (a *AlignWidget) At(x, y float64) *AlignWidget { a.x, a.y = x, y; return a }

// Left snaps the child to the left edge.
func (a *AlignWidget) Left() *AlignWidget { a.x = 0; return a }

// Right snaps the child to the right edge.
func (a *AlignWidget) Right() *AlignWidget { a.x = 1; return a }

// Top snaps the child to the top edge.
func (a *AlignWidget) Top() *AlignWidget { a.y = 0; return a }

// Bottom snaps the child to the bottom edge.
func (a *AlignWidget) Bottom() *AlignWidget { a.y = 1; return a }

// Layout implements Widget.
func (a *AlignWidget) Layout(c Constraints) Size {
	a.childSize = a.child.Layout(c.Loosen())
	return c.Constrain(Sz(bounded(c.MaxW, a.childSize.W), bounded(c.MaxH, a.childSize.H)))
}

// Paint implements Widget.
func (a *AlignWidget) Paint(dst *Canvas, r Rect) {
	a.child.Paint(dst, Rct(
		r.Origin.Add(Pt((r.Size.W-a.childSize.W)*a.x, (r.Size.H-a.childSize.H)*a.y)),
		a.childSize,
	))
}

// List builds one child per item and stacks them like a Column, which is
// what it returns: every Column setter applies to it.
func List[T any](items []T, item func(T) Widget) *ColumnWidget {
	return Column(Children(items, item)...)
}

// ScrollWidget shows a window onto a child that may be taller (or, with
// Horizontal, wider) than the space it has, and moves that window with the
// wheel. Build one with Scroll.
type ScrollWidget struct {
	child      Widget
	horizontal bool
	speed      float64
	bar        color.Color
	bound      *Signal[float64]
	offset     float64

	childSize Size
	viewport  Size
}

// Scroll lets child take any height and scrolls it within the space Scroll
// is given. The offset lives in the widget, so keep the widget alive (a
// static parent, or a Component) or bind it to a Signal with Offset.
func Scroll(child Widget) *ScrollWidget {
	return &ScrollWidget{child: child, speed: 20, bar: color.RGBA{0x80, 0x80, 0x80, 0x80}}
}

// Horizontal scrolls along the x axis instead of the y axis.
func (s *ScrollWidget) Horizontal() *ScrollWidget { s.horizontal = true; return s }

// Speed sets how many pixels one wheel unit moves.
func (s *ScrollWidget) Speed(px float64) *ScrollWidget { s.speed = px; return s }

// Bar sets the scrollbar color; nil hides the bar.
func (s *ScrollWidget) Bar(c color.Color) *ScrollWidget { s.bar = c; return s }

// Offset binds the scroll position to sig: wheel input writes it, and
// writing it scrolls. Use it to keep the position across rebuilds or to
// scroll programmatically.
func (s *ScrollWidget) Offset(sig *Signal[float64]) *ScrollWidget { s.bound = sig; return s }

func (s *ScrollWidget) extent(sz Size) float64 { return pick(s.horizontal, sz.W, sz.H) }

func (s *ScrollWidget) maxOffset() float64 {
	return max(s.extent(s.childSize)-s.extent(s.viewport), 0)
}

func (s *ScrollWidget) position() float64 {
	if s.bound != nil {
		return s.bound.Peek()
	}
	return s.offset
}

func (s *ScrollWidget) scrollTo(v float64) {
	v = clamp(v, 0, s.maxOffset())
	if s.bound != nil {
		s.bound.Set(v)
	} else {
		s.offset = v
	}
}

// Layout implements Widget.
func (s *ScrollWidget) Layout(c Constraints) Size {
	inner := c.Max()
	if s.horizontal {
		inner.W = Unbounded
	} else {
		inner.H = Unbounded
	}
	s.childSize = s.child.Layout(Loose(inner))
	s.viewport = c.Constrain(Sz(bounded(c.MaxW, s.childSize.W), bounded(c.MaxH, s.childSize.H)))
	s.scrollTo(s.position())
	return s.viewport
}

// Paint implements Widget.
func (s *ScrollWidget) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, s)
	origin := r.Origin
	if s.horizontal {
		origin.X -= s.position()
	} else {
		origin.Y -= s.position()
	}
	s.child.Paint(dst.Clip(r), Rct(origin, s.childSize))
	s.paintBar(dst, r)
}

func (s *ScrollWidget) paintBar(dst *Canvas, r Rect) {
	track, content := s.extent(r.Size), s.extent(s.childSize)
	if s.bar == nil || dst == nil || dst.Image == nil || content <= track {
		return
	}
	const thickness, margin, minThumb = 3.0, 2.0, 16.0
	thumb := max(track*track/content, minThumb)
	at := (track - thumb) * s.position() / s.maxOffset()
	var x, y, w, h float64
	if s.horizontal {
		x, y, w, h = r.Origin.X+at, r.Origin.Y+r.Size.H-thickness-margin, thumb, thickness
	} else {
		x, y, w, h = r.Origin.X+r.Size.W-thickness-margin, r.Origin.Y+at, thickness, thumb
	}
	vector.DrawFilledRect(dst.Image, float32(x), float32(y), float32(w), float32(h), s.bar, true)
}

// HandlePointer implements PointerHandler: wheel movement along the scroll
// axis moves the window.
func (s *ScrollWidget) HandlePointer(ev PointerEvent) bool {
	if ev.Kind != PointerScroll {
		return false
	}
	delta := pick(s.horizontal, ev.Scroll.X, ev.Scroll.Y)
	if delta == 0 || s.maxOffset() == 0 {
		return false
	}
	s.scrollTo(s.position() - delta*s.speed)
	return true
}
