package ggui

import (
	"fmt"
	"image/color"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
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
// with Text. Its style is resolved at layout: the Env's inherited TextStyle,
// then the widget's own setters on top, then built-in defaults for whatever
// is still unset.
type TextWidget struct {
	value string
	style TextStyle
	wrap  bool
	align float64
	role  textRole
	cache *CachedWidget

	// Layout caches the wrapped lines, their widths and the size they add up
	// to, and re-measures only when the text, the face or the width it must
	// fit in changes.
	resolved TextStyle
	lines    []string
	widths   []float64
	natural  Size
	wrapped  wrapKey
}

// wrapKey is the input wrapText was last run with.
type wrapKey struct {
	value string
	font  *Font
	size  float64
	maxW  float64
}

// textRole picks a named style from the theme at layout.
type textRole int

const (
	roleNone textRole = iota
	roleTitle
	roleCaption
)

// Text draws s in the inherited style, wrapping at spaces when it is wider
// than the space it gets.
func Text(s string) *TextWidget {
	return &TextWidget{value: s, wrap: true}
}

// Title draws s in the theme's Title style, resolved from the Env at layout,
// so a heading needs no UseTheme.
func Title(s string) *TextWidget { return &TextWidget{value: s, wrap: true, role: roleTitle} }

// Caption draws s in the theme's Caption style, resolved from the Env at
// layout.
func Caption(s string) *TextWidget { return &TextWidget{value: s, wrap: true, role: roleCaption} }

// AsTitle gives the text the theme's Title style, under its own setters.
func (t *TextWidget) AsTitle() *TextWidget { t.role = roleTitle; return t }

// AsCaption gives the text the theme's Caption style, under its own setters.
func (t *TextWidget) AsCaption() *TextWidget { t.role = roleCaption; return t }

// TextOf draws the string r holds and follows it: the effect it owns lives
// in the enclosing Builder and is disposed with it. It is a TextWidget, so
// every setter chains.
func TextOf(r Reader[string]) *TextWidget {
	t := Text("")
	Effect(func() {
		t.value = r.Get()
		t.cache.invalidate()
	})
	return t
}

// Textf is TextOf over Sprintf: a formatted text that follows the reactive
// values among its arguments.
//
//	ggui.Textf("count: %d", count).Style(t.Title)
func Textf(format string, args ...any) *TextWidget { return TextOf(Sprintf(format, args...)) }

// Sprintf formats like fmt.Sprintf and recomputes when a reactive argument
// (a Signal, Memo, Tweened or Sprung) changes; other arguments pass through.
func Sprintf(format string, args ...any) *Memo[string] {
	vals := make([]any, len(args))
	return Derived(func() string {
		for i, a := range args {
			if r, ok := a.(anyReader); ok {
				vals[i] = r.GetAny()
			} else {
				vals[i] = a
			}
		}
		return fmt.Sprintf(format, vals...)
	})
}

// Style merges ts onto the widget's own style.
func (t *TextWidget) Style(ts TextStyle) *TextWidget { t.style = t.style.Merge(ts); return t }

// Color sets the text color.
func (t *TextWidget) Color(c color.Color) *TextWidget { t.style.Color = c; return t }

// Font sets the face; nil inherits.
func (t *TextWidget) Font(f *Font) *TextWidget { t.style.Font = f; return t }

// Size sets the font size in pixels.
func (t *TextWidget) Size(px float64) *TextWidget { t.style.Size = px; return t }

// LineHeight sets the distance between baselines as a multiple of Size.
func (t *TextWidget) LineHeight(mult float64) *TextWidget { t.style.LineHeight = mult; return t }

// Set replaces the text. A widget that changes what it shows outside a
// rebuild calls it from Layout; TextOf does from an effect.
func (t *TextWidget) Set(s string) *TextWidget {
	if s != t.value {
		t.value = s
		t.cache.invalidate()
	}
	return t
}

// NoWrap keeps the text on one line per hard line break, however wide.
func (t *TextWidget) NoWrap() *TextWidget { t.wrap = false; return t }

// Align places each line within the widget's width by fraction: 0 is left,
// 0.5 centered, 1 right.
func (t *TextWidget) Align(x float64) *TextWidget { t.align = x; return t }

// current is the resolved style from the last Layout, or the widget's own
// style over the defaults before any Layout has run.
func (t *TextWidget) current() TextStyle {
	if t.resolved.Font == nil {
		return t.style.resolved()
	}
	return t.resolved
}

// faceAt returns the face at the resolved size times scale, so that on a
// HiDPI Canvas glyphs are rasterized at full resolution instead of scaled up.
func (t *TextWidget) faceAt(scale float64) text.Face {
	st := t.current()
	return st.Font.face(st.Size * scale)
}

// spacing is the distance between baselines.
func (t *TextWidget) spacing() float64 {
	st := t.current()
	return st.Size * st.LineHeight
}

// Layout implements Widget.
func (t *TextWidget) Layout(c Constraints, env Env) Size {
	t.cache, _ = env.Get(cacheOwner)
	base := env.Text()
	switch t.role {
	case roleTitle:
		base = base.Merge(env.Theme().Title)
	case roleCaption:
		base = base.Merge(env.Theme().Caption)
	}
	t.resolved = base.Merge(t.style).resolved()
	t.resolved.Size *= env.TextScale()
	face := t.faceAt(1)
	key := wrapKey{value: t.value, font: t.resolved.Font, size: t.resolved.Size, maxW: pick(t.wrap, c.MaxW, 0)}
	if key != t.wrapped {
		t.wrapped = key
		t.lines = wrapText(key.value, face, key.maxW)
		t.widths = t.widths[:0]
		var w float64
		for _, line := range t.lines {
			lw := lineWidth(line, face)
			t.widths = append(t.widths, lw)
			w = max(w, lw)
		}
		m := face.Metrics()
		t.natural = Sz(w, float64(len(t.lines)-1)*t.spacing()+m.HAscent+m.HDescent)
	}
	return c.Constrain(t.natural)
}

// Paint implements Widget.
func (t *TextWidget) Paint(dst *Canvas, r Rect) {
	// Text is half of what a screen reader reads, and none of it takes
	// input, so it goes in the semantics tree and never in the hit list,
	// which input scans backwards on every pointer event. Text a control
	// already painted as its own label is that control's name, not an
	// element of its own, so it stays quiet there.
	if t.value != "" && !dst.named(r) {
		dst.Leaf(r, Node{Role: pick(t.role == roleTitle, RoleHeading, RoleText), Name: t.value})
	}
	if dst == nil || dst.Image == nil {
		return
	}
	face := t.faceAt(dst.Scale())
	op := &text.DrawOptions{}
	op.ColorScale.ScaleWithColor(t.current().Color)
	for i, line := range t.lines {
		x := r.Origin.X + (r.Size.W-t.widths[i])*t.align
		y := r.Origin.Y + float64(i)*t.spacing()
		op.GeoM.Reset()
		op.GeoM.Translate(dst.px(x), dst.px(y))
		text.Draw(dst.Image, line, face, op)
	}
}

// StyledWidget sets the text style its subtree inherits. Build one with
// Styled.
type StyledWidget struct {
	style TextStyle
	child Widget
}

// Styled gives every Text below child a new base style: what the setters
// here set, over what was inherited from above. A Text's own setters still
// win over it.
//
//	Styled(Column(Text("a"), Text("b"))).Color(muted).Size(12)
func Styled(child Widget) *StyledWidget { return &StyledWidget{child: child} }

// Style merges ts onto the style the subtree inherits.
func (s *StyledWidget) Style(ts TextStyle) *StyledWidget { s.style = s.style.Merge(ts); return s }

// Color sets the inherited text color.
func (s *StyledWidget) Color(c color.Color) *StyledWidget { s.style.Color = c; return s }

// Font sets the inherited font.
func (s *StyledWidget) Font(f *Font) *StyledWidget { s.style.Font = f; return s }

// Size sets the inherited font size.
func (s *StyledWidget) Size(px float64) *StyledWidget { s.style.Size = px; return s }

// LineHeight sets the inherited line height.
func (s *StyledWidget) LineHeight(mult float64) *StyledWidget { s.style.LineHeight = mult; return s }

// Layout implements Widget.
func (s *StyledWidget) Layout(c Constraints, env Env) Size {
	return s.child.Layout(c, env.WithText(s.style))
}

// Paint implements Widget.
func (s *StyledWidget) Paint(dst *Canvas, r Rect) { dst.Paint(s.child, r) }

// EnvWidget hands its child a modified Env. Build one with Provide.
type EnvWidget struct {
	with  func(Env) Env
	child Widget
}

// Provide stores v under k for the subtree below child, where any widget can
// read it back from its Env with Get. It is how your own inherited values
// (a form's disabled state, a list's density) travel down the tree.
func Provide[T any](k Key[T], v T, child Widget) *EnvWidget {
	return &EnvWidget{with: func(e Env) Env { return e.With(k, v) }, child: child}
}

// Themed lays child out under theme t instead of the app's, for a panel
// that keeps its own look.
func Themed(t Theme, child Widget) *EnvWidget {
	return &EnvWidget{with: func(e Env) Env { return e.WithTheme(t).WithText(t.Text) }, child: child}
}

// Layout implements Widget.
func (e *EnvWidget) Layout(c Constraints, env Env) Size { return e.child.Layout(c, e.with(env)) }

// Paint implements Widget.
func (e *EnvWidget) Paint(dst *Canvas, r Rect) { dst.Paint(e.child, r) }

// BoxWidget paints a rectangle and lays an optional child inside its padding.
// Build one with Box.
type BoxWidget struct {
	shadows     []ShadowStyle
	fill        color.Color
	radius      float64
	borderWidth float64
	borderColor color.Color
	padding     EdgeInsets
	width       float64 // 0 means "as small as the child allows"
	height      float64
	child       Widget

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

// Shadow replaces the outer shadow layers. Calling it without arguments clears
// them. Shadows paint in argument order and do not reserve layout space.
func (b *BoxWidget) Shadow(styles ...ShadowStyle) *BoxWidget {
	b.shadows = append([]ShadowStyle(nil), styles...)
	return b
}

// Radius rounds the corners of the fill and border.
func (b *BoxWidget) Radius(r float64) *BoxWidget { b.radius = r; return b }

// Border draws a line of width w in color c just inside the edge.
func (b *BoxWidget) Border(w float64, c color.Color) *BoxWidget {
	b.borderWidth, b.borderColor = w, c
	return b
}

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
func (b *BoxWidget) Layout(c Constraints, env Env) Size {
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
		b.childSize = b.child.Layout(inner, env)
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
	for _, s := range b.shadows {
		dst.Shadow(r, b.radius, s)
	}
	dst.FillRoundRect(r, b.radius, b.fill)
	if b.borderWidth > 0 {
		dst.StrokeRoundRect(r, b.radius, b.borderWidth, b.borderColor)
	}
	if b.child != nil {
		dst.Paint(b.child, Rct(r.Origin.Add(Pt(b.padding.Left, b.padding.Top)), b.childSize))
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
	AlignStart   CrossAlign = iota // top of a Row, left of a Column (a Column's default)
	AlignCenter                    // centered across (a Row's default)
	AlignEnd                       // bottom of a Row, right of a Column
	AlignStretch                   // stretched to the widget's cross size, which fills the space given
)

// flow lays children out along one axis. Column and Row are the two
// orientations of it. FlexWidget children share whatever main-axis space the
// others leave, weighted by their flex.
type flow struct {
	horizontal bool
	gap        float64
	space      float64 // gap in theme Space units, when set
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

// stretched returns the cross-axis size to give a child (v = 0) or the
// flow itself (v = the widest child): with AlignStretch it fills the
// available cross space when that is bounded.
func (f *flow) stretched(crossMax, v float64) float64 {
	if f.align == AlignStretch {
		return bounded(crossMax, v)
	}
	return v
}

// gapFor returns the gap to use: Space times the theme's unit, else Gap.
func (f *flow) gapFor(env Env) float64 {
	if f.space > 0 {
		return f.space * env.Theme().Space
	}
	return f.gap
}

func (f *flow) layout(c Constraints, env Env) Size {
	gap := f.gapFor(env)
	n := len(f.children)
	f.sizes = resize(f.sizes, n)
	f.offsets = resize(f.offsets, n)

	mainMax, crossMax := f.main(c.Max()), f.cross(c.Max())
	crossMin := f.stretched(crossMax, 0)

	var gaps float64
	if n > 1 {
		gaps = gap * float64(n-1)
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
		f.sizes[i] = child.Layout(f.constraints(0, max(mainMax-used, 0), crossMin, crossMax), env)
		used += f.main(f.sizes[i])
	}
	free := max(mainMax-used, 0)
	for i, child := range f.children {
		if fw, ok := child.(*FlexWidget); ok && fw.flex > 0 && flexible {
			extent := free * fw.flex / totalFlex
			f.sizes[i] = child.Layout(f.constraints(extent, extent, crossMin, crossMax), env)
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
	result := c.Constrain(f.size(mainTotal, f.stretched(crossMax, crossUsed)))

	lead, between := 0.0, gap
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
		dst.Paint(child, Rct(r.Origin.Add(f.offsets[i]), f.sizes[i]))
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

// Space sets the gap to n times the theme's Space, resolved at layout.
func (col *ColumnWidget) Space(n float64) *ColumnWidget { col.space = n; return col }

// Justify distributes children along the vertical axis.
func (col *ColumnWidget) Justify(j Justify) *ColumnWidget { col.justify = j; return col }

// Align places children horizontally within the column.
func (col *ColumnWidget) Align(a CrossAlign) *ColumnWidget { col.align = a; return col }

// Layout implements Widget.
func (col *ColumnWidget) Layout(c Constraints, env Env) Size { return col.layout(c, env) }

// Paint implements Widget.
func (col *ColumnWidget) Paint(dst *Canvas, r Rect) { col.paint(dst, r) }

// RowWidget lines its children up horizontally. Build one with Row.
type RowWidget struct{ flow }

// Row lines children up left to right, centered on the row's height; a
// Column starts its children at the left.
func Row(children ...Widget) *RowWidget {
	return &RowWidget{flow{horizontal: true, align: AlignCenter, children: children}}
}

// Gap sets the space between consecutive children.
func (row *RowWidget) Gap(v float64) *RowWidget { row.gap = v; return row }

// Space sets the gap to n times the theme's Space, resolved at layout.
func (row *RowWidget) Space(n float64) *RowWidget { row.space = n; return row }

// Justify distributes children along the horizontal axis.
func (row *RowWidget) Justify(j Justify) *RowWidget { row.justify = j; return row }

// Align places children vertically within the row.
func (row *RowWidget) Align(a CrossAlign) *RowWidget { row.align = a; return row }

// Layout implements Widget.
func (row *RowWidget) Layout(c Constraints, env Env) Size { return row.layout(c, env) }

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
func (f *FlexWidget) Layout(c Constraints, env Env) Size { return f.child.Layout(c, env) }

// Paint implements Widget.
func (f *FlexWidget) Paint(dst *Canvas, r Rect) { dst.Paint(f.child, r) }

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
func (st *StackWidget) Layout(c Constraints, env Env) Size {
	st.sizes = st.sizes[:0]
	var total Size
	for _, child := range st.children {
		s := child.Layout(c.Loosen(), env)
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
		dst.Paint(child, Rct(r.Origin, st.sizes[i]))
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
func (a *AlignWidget) Layout(c Constraints, env Env) Size {
	a.childSize = a.child.Layout(c.Loosen(), env)
	return c.Constrain(Sz(bounded(c.MaxW, a.childSize.W), bounded(c.MaxH, a.childSize.H)))
}

// Paint implements Widget.
func (a *AlignWidget) Paint(dst *Canvas, r Rect) {
	dst.Paint(a.child, Rct(
		r.Origin.Add(Pt((r.Size.W-a.childSize.W)*a.x, (r.Size.H-a.childSize.H)*a.y)),
		a.childSize,
	))
}

// List builds one child per item and stacks them like a Column, which is
// what it returns: every Column setter applies to it.
func List[T any](items []T, item func(T) Widget) *ColumnWidget {
	return Column(Children(items, item)...)
}

// Viewport is what a Scroll tells the subtree it lays out: how far along
// the scroll axis the window starts and how long it is, in logical pixels.
// A list that knows its items' sizes can then lay out only the ones in
// view; For does with ItemExtent.
type Viewport struct {
	Offset, Extent float64
	Horizontal     bool
}

var viewportKey = NewKey[Viewport]("viewport")

// ScrollViewport returns the window of the nearest enclosing Scroll, if any.
func ScrollViewport(env Env) (Viewport, bool) { return env.Get(viewportKey) }

// ScrollWidget shows a window onto a child that may be taller (or, with
// Horizontal, wider) than the space it has, and moves that window with the
// wheel. Build one with Scroll.
type ScrollWidget struct {
	child      Widget
	horizontal bool
	speed      float64
	bar        color.Color
	bound      Binding[float64]
	offset     float64
	id         any

	childSize Size
	viewport  Size
	laidAt    float64 // the offset the child was last laid out for
	cache     *CachedWidget
	rect      Rect // where the window was last painted
}

// Key gives the scroll an identity, so a rebuilt one that also moved keeps
// its offset. Without one its Rect identifies it.
func (s *ScrollWidget) Key(k any) *ScrollWidget { s.id = k; return s }

// HitID implements Identified.
func (s *ScrollWidget) HitID() any { return s.id }

// Adopt implements Adopter: a rebuilt scroll keeps its offset.
func (s *ScrollWidget) Adopt(prev any) {
	if p, ok := prev.(*ScrollWidget); ok && s.bound == nil {
		s.offset = p.position()
		if s.offset != s.laidAt {
			s.cache.invalidate()
			requestLayout()
		}
	}
}

// Scroll lets child take any height and scrolls it within the space Scroll
// is given. The offset lives in the widget and carries across a rebuild;
// bind it to a Signal with Offset to read or set it.
func Scroll(child Widget) *ScrollWidget {
	return &ScrollWidget{child: child, speed: 20, bar: color.RGBA{0x80, 0x80, 0x80, 0x80}, id: autoID()}
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
func (s *ScrollWidget) Offset(sig Binding[float64]) *ScrollWidget { s.bound = sig; return s }

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
	} else if v != s.offset {
		s.offset = v
		requestLayout() // what a virtualized child shows depends on it
	}
	if v != s.laidAt {
		s.cache.invalidate()
	}
}

// Layout implements Widget.
func (s *ScrollWidget) Layout(c Constraints, env Env) Size {
	inner := c.Max()
	if s.horizontal {
		inner.W = Unbounded
	} else {
		inner.H = Unbounded
	}
	s.cache, _ = env.Get(cacheOwner)
	s.laidAt = s.position()
	vp := Viewport{Offset: s.laidAt, Extent: s.extent(c.Max()), Horizontal: s.horizontal}
	s.childSize = s.child.Layout(Loose(inner), env.With(viewportKey, vp))
	s.viewport = c.Constrain(Sz(bounded(c.MaxW, s.childSize.W), bounded(c.MaxH, s.childSize.H)))
	s.scrollTo(s.position())
	return s.viewport
}

// Paint implements Widget.
func (s *ScrollWidget) Paint(dst *Canvas, r Rect) {
	if s.position() != s.laidAt {
		// Written through the bound signal since the layout: a virtualized
		// child must be laid out again for the new window.
		s.cache.invalidate()
	}
	s.rect = r
	dst.HitPointer(r, s)
	origin := r.Origin
	if s.horizontal {
		origin.X -= s.position()
	} else {
		origin.Y -= s.position()
	}
	dst.Node(r, Node{Role: RoleGroup, Actions: ActionScrollIntoView}, func(dst *Canvas) {
		dst.Clip(r).Paint(s.child, Rct(origin, s.childSize))
	})
	s.paintBar(dst, r)
}

func (s *ScrollWidget) paintBar(dst *Canvas, r Rect) {
	track, content := s.extent(r.Size), s.extent(s.childSize)
	if content <= track {
		return
	}
	const thickness, margin, minThumb = 3.0, 2.0, 16.0
	thumb := max(track*track/content, minThumb)
	at := (track - thumb) * s.position() / s.maxOffset()
	if s.horizontal {
		dst.FillRect(Rct(Pt(r.Origin.X+at, r.Origin.Y+r.Size.H-thickness-margin), Sz(thumb, thickness)), s.bar)
	} else {
		dst.FillRect(Rct(Pt(r.Origin.X+r.Size.W-thickness-margin, r.Origin.Y+at), Sz(thickness, thumb)), s.bar)
	}
}

// Reveal implements Revealer: the window moves the least it must for
// target, in window coordinates, to be inside it. Focus moved by the
// keyboard calls it.
func (s *ScrollWidget) Reveal(target Rect) {
	// Only a target inside the content, which may lie beyond the window
	// along the axis but not across it.
	content := Rct(s.rect.Origin, s.childSize)
	if s.horizontal {
		content.Origin.X -= s.position()
		content.Size.H = s.rect.Size.H
	} else {
		content.Origin.Y -= s.position()
		content.Size.W = s.rect.Size.W
	}
	if !content.Contains(target.Origin) {
		return
	}
	lo, hi := target.Origin.Y-s.rect.Origin.Y, target.Origin.Y+target.Size.H-s.rect.Origin.Y
	extent := s.rect.Size.H
	if s.horizontal {
		lo, hi = target.Origin.X-s.rect.Origin.X, target.Origin.X+target.Size.W-s.rect.Origin.X
		extent = s.rect.Size.W
	}
	switch {
	case lo < 0:
		s.scrollTo(s.position() + lo)
	case hi > extent:
		s.scrollTo(s.position() + hi - extent)
	}
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

// WrapWidget lines its children up like a Row and starts a new line when
// the next child would not fit. Build one with Wrap.
type WrapWidget struct {
	children []Widget
	gap      float64 // between children on a line
	runGap   float64 // between lines
	space    float64 // both, in theme units, when set
	align    CrossAlign

	sizes   []Size
	offsets []Point
}

// Wrap flows children left to right, wrapping onto new lines at the width
// it is given, the way words fill a paragraph; tags and toolbars want it.
func Wrap(children ...Widget) *WrapWidget { return &WrapWidget{children: children} }

// Gap sets the space between children on a line and between lines.
func (w *WrapWidget) Gap(v float64) *WrapWidget { w.gap, w.runGap = v, v; return w }

// RunGap sets the space between lines alone.
func (w *WrapWidget) RunGap(v float64) *WrapWidget { w.runGap = v; return w }

// Space sets both gaps to n times the theme's Space, resolved at layout.
func (w *WrapWidget) Space(n float64) *WrapWidget { w.space = n; return w }

// Align places children vertically within their line.
func (w *WrapWidget) Align(a CrossAlign) *WrapWidget { w.align = a; return w }

// Layout implements Widget.
func (w *WrapWidget) Layout(c Constraints, env Env) Size {
	if w.space > 0 {
		w.gap = w.space * env.Theme().Space
		w.runGap = w.gap
	}
	n := len(w.children)
	w.sizes = resize(w.sizes, n)
	w.offsets = resize(w.offsets, n)
	for i, child := range w.children {
		w.sizes[i] = child.Layout(Loose(Sz(c.MaxW, c.MaxH)), env)
	}
	var x, y, lineH, width float64
	start := 0
	place := func(end int) {
		for i := start; i < end; i++ {
			w.offsets[i].Y = y + (lineH-w.sizes[i].H)*w.crossFraction()
		}
	}
	for i, s := range w.sizes {
		if i > start && x+s.W > c.MaxW {
			place(i)
			width = max(width, x-w.gap)
			y += lineH + w.runGap
			x, lineH, start = 0, 0, i
		}
		w.offsets[i].X = x
		x += s.W + w.gap
		lineH = max(lineH, s.H)
	}
	place(n)
	if n > 0 {
		width = max(width, x-w.gap)
		y += lineH
	}
	return c.Constrain(Sz(width, y))
}

func (w *WrapWidget) crossFraction() float64 {
	switch w.align {
	case AlignCenter:
		return 0.5
	case AlignEnd:
		return 1
	}
	return 0
}

// Paint implements Widget.
func (w *WrapWidget) Paint(dst *Canvas, r Rect) {
	for i, child := range w.children {
		dst.Paint(child, Rct(r.Origin.Add(w.offsets[i]), w.sizes[i]))
	}
}

// GridWidget lays its children out in equal-width columns. Build one with
// Grid.
type GridWidget struct {
	cols     int
	children []Widget
	gap      float64
	rowGap   float64
	space    float64 // both, in theme units, when set

	sizes   []Size
	offsets []Point
	cellW   float64
}

// Grid places children in rows of cols cells, left to right then top to
// bottom. Each column takes an equal share of the width; each row is as
// tall as its tallest cell, and children are given the cell width tight, so
// text and boxes align down the columns.
func Grid(cols int, children ...Widget) *GridWidget {
	return &GridWidget{cols: max(cols, 1), children: children}
}

// Gap sets the space between columns and between rows.
func (g *GridWidget) Gap(v float64) *GridWidget { g.gap, g.rowGap = v, v; return g }

// RowGap sets the space between rows alone.
func (g *GridWidget) RowGap(v float64) *GridWidget { g.rowGap = v; return g }

// Space sets both gaps to n times the theme's Space, resolved at layout.
func (g *GridWidget) Space(n float64) *GridWidget { g.space = n; return g }

// Layout implements Widget.
func (g *GridWidget) Layout(c Constraints, env Env) Size {
	if g.space > 0 {
		g.gap = g.space * env.Theme().Space
		g.rowGap = g.gap
	}
	n := len(g.children)
	g.sizes = resize(g.sizes, n)
	g.offsets = resize(g.offsets, n)
	cols := g.cols
	if math.IsInf(c.MaxW, 1) {
		// No width to share: every column is as wide as the widest child.
		g.cellW = 0
		for i, child := range g.children {
			g.sizes[i] = child.Layout(Loose(Sz(Unbounded, Unbounded)), env)
			g.cellW = max(g.cellW, g.sizes[i].W)
		}
	} else {
		g.cellW = max((c.MaxW-g.gap*float64(cols-1))/float64(cols), 0)
	}
	cell := Constraints{MinW: g.cellW, MaxW: g.cellW, MaxH: Unbounded}
	var y float64
	for row := 0; row*cols < n; row++ {
		var rowH float64
		for col := 0; col < cols && row*cols+col < n; col++ {
			i := row*cols + col
			g.sizes[i] = g.children[i].Layout(cell, env)
			g.offsets[i] = Pt(float64(col)*(g.cellW+g.gap), y)
			rowH = max(rowH, g.sizes[i].H)
		}
		y += rowH
		if (row+1)*cols < n {
			y += g.rowGap
		}
	}
	usedCols := min(cols, n)
	width := float64(usedCols)*g.cellW + g.gap*float64(max(usedCols-1, 0))
	return c.Constrain(Sz(width, y))
}

// Paint implements Widget.
func (g *GridWidget) Paint(dst *Canvas, r Rect) {
	for i, child := range g.children {
		dst.Paint(child, Rct(r.Origin.Add(g.offsets[i]), g.sizes[i]))
	}
}
