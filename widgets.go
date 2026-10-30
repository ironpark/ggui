package ggui

import (
	"fmt"
	"image/color"
	"slices"

	"github.com/ironpark/ggui/internal/fn"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/text"
	"github.com/ironpark/ggui/pkg/layout"
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

// EdgeInsets is padding on the four sides of a box: Top, Right, Bottom and
// Left. Shrink removes it from constraints and Inflate adds it to a size.
type EdgeInsets = layout.Insets

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

// TextWidget draws text, wrapping it to the width it is given. Build one
// with Text. Its style is resolved at layout: the Env's inherited TextStyle,
// then the widget's own setters on top, then built-in defaults for whatever
// is still unset.
type TextWidget struct {
	props         property.Owner
	value         string
	content       property.Value[string]
	format        string
	formatArgs    []any
	formatted     bool
	style         TextStyle
	wrap          bool
	ellipsis      bool
	maxLines      int
	align         float64
	alignSet      bool
	role          Role
	styleKey      EnvKey[TextStyle]
	styleFallback TextStyle
	cache         *CachedWidget

	// Layout caches the wrapped lines, their widths and the size they add up
	// to, and re-measures only when the text, the face or the width it must
	// fit in changes.
	resolved TextStyle
	lines    []string
	widths   []float64
	natural  Size
	ascent   float64 // the first baseline, below the top
	wrapped  wrapKey
	bidi     textBidi         // the lines in visual order, when the direction or the text needs it
	rich     *RichTextWidget  // what a Selectable Text lays out and paints as
	richAt   richAt           // what a Selectable Text was laid out for, to lay its lines out when rotated
	drawOp   text.DrawOptions // reused by paintLines, which would allocate one a paint
	pieceOp  text.DrawOptions // reused by drawPieces for the same
}

// richAt is the width and Env a Selectable Text last laid out in.
type richAt struct {
	maxW float64
	env  Env
}

// wrapKey is the input wrapText was last run with.
type wrapKey struct {
	generation uint64
	value      string
	font       *Font
	weight     FontWeight
	size       float64
	lineHeight float64
	maxW       float64
	maxLines   int
	ellipsis   bool
}

// Text draws s in the inherited style, wrapping at spaces when it is wider
// than the space it gets.
func Text(s string) *TextWidget {
	return (&TextWidget{wrap: true}).Content(s)
}

// StyleKey selects an inherited text style, merged below explicit setters.
// An optional fallback is used when the environment has no value for key.
func (t *TextWidget) StyleKey(key EnvKey[TextStyle], fallback ...TextStyle) *TextWidget {
	defer property.Watch(&t.props, &t.styleKey)()
	defer property.Watch(&t.props, &t.styleFallback)()
	t.styleKey = key
	t.styleFallback = TextStyle{}
	if len(fallback) > 0 {
		t.styleFallback = fallback[0]
	}
	return t
}

// Role sets the accessibility role; the default is RoleText.
func (t *TextWidget) Role(role Role) *TextWidget {
	defer property.Watch(&t.props, &t.role)()
	t.role = role
	return t
}

// TextOf borrows r and follows it during layout without owning a subscription.
func TextOf(r Readable[string]) *TextWidget { return Text("").BindContent(r) }

// Textf formats during layout, reading GetAny arguments like Sprintf. It copies
// args and creates no computation; construction needs no owner.
func Textf(format string, args ...any) *TextWidget {
	t := Text("")
	t.format = format
	t.formatArgs = append([]any(nil), args...)
	t.formatted = true
	return t
}

// Sprintf formats like fmt.Sprintf, reading arguments that implement
// GetAny() any on each computation. StateValue, DerivedValue, Lens, Tweened
// and Sprung implement that method. Other arguments pass through unchanged;
// Readable's Get() alone is not enough. Adapt a custom Readable with
// Derived(reader.Get), or format its Get() inside a Derived callback.
func Sprintf(format string, args ...any) *DerivedValue[string] {
	args = append([]any(nil), args...)
	return Derived(func() string { return formatValues(format, args) })
}
func formatValues(format string, args []any) string {
	vals := make([]any, len(args))
	for i, a := range args {
		if r, ok := a.(reactive.AnyReader); ok {
			vals[i] = r.GetAny()
		} else {
			vals[i] = a
		}
	}
	return fmt.Sprintf(format, vals...)
}

// Style merges ts onto the widget's own style.
func (t *TextWidget) Style(ts TextStyle) *TextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style = t.style.Merge(ts)
	return t
}

// Color sets the text color; nil inherits. Changing one color for another
// is paint-only, so a control can recolor its label as the pointer moves
// over it without laying it out again.
func (t *TextWidget) Color(c color.Color) *TextWidget {
	if (c == nil) != (t.style.Color == nil) || IsEnvColor(c) || IsEnvColor(t.style.Color) {
		defer property.Watch(&t.props, &t.style)()
	}
	t.style.Color = c
	return t
}

// Font sets the face; nil inherits.
func (t *TextWidget) Font(f *Font) *TextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style.Font = f
	return t
}

// Weight sets the font weight, drawn with the Font's face nearest it; zero
// inherits.
func (t *TextWidget) Weight(w FontWeight) *TextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style.Weight = w
	return t
}

// Size sets the font size in pixels.
func (t *TextWidget) Size(px float64) *TextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style.Size = px
	return t
}

// LineHeight sets the distance between baselines as a multiple of Size.
func (t *TextWidget) LineHeight(mult float64) *TextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style.LineHeight = mult
	return t
}

// Content replaces any literal, reader or formatted source and refreshes
// measurement when needed. Call it on the UI goroutine.
func (t *TextWidget) Content(s string) *TextWidget {
	changed := t.content.Set(s) || t.formatted || t.value != s
	t.formatted = false
	t.format = ""
	t.formatArgs = nil
	t.value = s
	if changed {
		t.props.Changed()
	}
	return t
}

// BindContent follows r during layout without owning a subscription.
func (t *TextWidget) BindContent(r Readable[string]) *TextWidget {
	changed := t.content.Bind(r, "BindContent") || t.formatted
	t.formatted = false
	t.format = ""
	t.formatArgs = nil
	if changed {
		t.props.Changed()
	}
	return t
}

// NoWrap keeps the text on one line per hard line break, however wide.
func (t *TextWidget) NoWrap() *TextWidget {
	defer property.Watch(&t.props, &t.wrap)()
	t.wrap = false
	return t
}

// Ellipsis ends a line too wide for the widget with "…" instead of letting it
// run past the edge: with NoWrap, every line wider than the width it gets.
// A screen reader still reads the whole text.
func (t *TextWidget) Ellipsis() *TextWidget {
	defer property.Watch(&t.props, &t.ellipsis)()
	t.ellipsis = true
	return t
}

// MaxLines shows at most n lines, ending the last with "…" when the text
// goes on past it; 0 shows them all. MaxLines(1) with wrapping on is one
// line cut to the width. A screen reader still reads the whole text.
func (t *TextWidget) MaxLines(n int) *TextWidget {
	defer property.Watch(&t.props, &t.maxLines)()
	t.maxLines = max(n, 0)
	return t
}

// Align places each line within the widget's width by fraction: 0 is left,
// 0.5 centered, 1 right. Unset, lines start at the reading direction's
// start: the left, or the right under DirRTL.
func (t *TextWidget) Align(x float64) *TextWidget {
	defer property.Watch(&t.props, &t.align)()
	defer property.Watch(&t.props, &t.alignSet)()
	t.align, t.alignSet = x, true
	return t
}

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
	return st.Font.face(st.Size*scale, st.Weight)
}

// spacing is the distance between baselines.
func (t *TextWidget) spacing() float64 {
	st := t.current()
	return st.Size * st.LineHeight
}

// Layout implements Widget.
func (t *TextWidget) Layout(c Constraints, env Env) Size {
	defer t.props.Layout().EndLayout()
	if t.formatted {
		t.value = formatValues(t.format, t.formatArgs)
	} else {
		t.value = t.content.Get()
	}
	t.cache, _ = env.Get(cacheOwner)
	style, ok := env.Get(t.styleKey)
	if !ok {
		style = t.styleFallback
	}
	t.resolved = env.ResolveText(style.Merge(t.style))
	if t.rich != nil {
		// A Selectable Text lays out and paints as its RichText. Only
		// PaintRotated draws the lines, so they wait for it.
		t.richAt = richAt{c.MaxW, env}
		return t.layoutRich(c, env, style.Merge(t.style))
	}
	t.layoutLines(c.MaxW, env)
	return c.Constrain(t.natural)
}

// layoutLines wraps, cuts and orders the lines for a width of maxW, when
// any of what they depend on changed since the last time.
func (t *TextWidget) layoutLines(maxW float64, env Env) {
	face := t.faceAt(1)
	fits := t.wrap || t.ellipsis || t.maxLines > 0
	key := wrapKey{generation: fontGen(), value: t.value, font: t.resolved.Font, weight: t.resolved.Weight, size: t.resolved.Size, lineHeight: t.resolved.LineHeight,
		maxW: pick(fits, maxW, 0), maxLines: t.maxLines, ellipsis: t.ellipsis}
	if key != t.wrapped {
		t.wrapped = key
		t.lines = wrapText(key.value, face, pick(t.wrap, key.maxW, 0))
		if n := key.maxLines; n > 0 && len(t.lines) > n {
			t.lines = t.lines[:n]
			t.lines[n-1] = ellipsize(t.lines[n-1], face, key.maxW, true)
		}
		if key.ellipsis {
			for i, line := range t.lines {
				t.lines[i] = ellipsize(line, face, key.maxW, false)
			}
		}
		t.widths = t.widths[:0]
		var w float64
		for _, line := range t.lines {
			lw := lineWidth(line, face)
			t.widths = append(t.widths, lw)
			w = max(w, lw)
		}
		m := face.Metrics()
		t.natural = Sz(w, float64(len(t.lines)-1)*t.spacing()+m.HAscent+m.HDescent)
		t.ascent = m.HAscent
	}
	t.layoutBidi(env)
}

// Baseline implements Baseliner: the first line's baseline is its ascent
// below the top, where paintLines puts the line.
func (t *TextWidget) Baseline() (float64, bool) {
	if t.rich != nil {
		return t.rich.Baseline()
	}
	return t.ascent, t.resolved.Font != nil
}

// Paint implements Widget.
func (t *TextWidget) Paint(dst *Canvas, r Rect) {
	// Text is half of what a screen reader reads, and none of it takes
	// input, so it goes in the semantics tree and never in the hit list,
	// which input scans backwards on every pointer event. Text a control
	// already painted as its own label is that control's name, not an
	// element of its own, so it stays quiet there.
	if t.rich != nil {
		t.rich.Paint(dst, r)
		return
	}
	t.paintLines(dst, r, true, func(op *text.DrawOptions, x, y float64) {
		op.GeoM.Translate(dst.px(x), dst.px(y))
	})
}

// paintLines emits the semantics leaf, then draws every laid-out line with the
// shared style and line-advance math. place positions one line: it receives the
// freshly reset GeoM and the line's logical origin, so Paint translates and
// PaintRotated translates, rotates and translates back.
// paintLines draws the lines with place moving each to where it is drawn.
// upright says place only translates them, so that a line wholly outside
// the clip is skipped rather than shaped and drawn to be clipped: a long
// Text in a Scroll draws only what shows.
func (t *TextWidget) paintLines(dst *Canvas, r Rect, upright bool, place func(op *text.DrawOptions, x, y float64)) {
	if t.value != "" && !dst.named(r) {
		dst.Leaf(r, Node{Role: roleOr(t.role, RoleText), Name: t.value})
	}
	if dst == nil || dst.Image == nil || dst.Image.Bounds().Empty() {
		return
	}
	if t.rich != nil {
		t.layoutLines(t.richAt.maxW, t.richAt.env)
	}
	face := t.faceAt(dst.Scale())
	op := &t.drawOp
	*op = text.DrawOptions{}
	col := pick(t.style.Color != nil && !IsEnvColor(t.style.Color), t.style.Color, t.current().Color)
	op.ColorScale.ScaleWithColor(col)
	align := pick(t.alignSet, t.align, startAlign(t.bidi.dir))
	for i, line := range t.lines {
		x := r.Origin.X + (r.Size.W-t.widths[i])*align
		y := r.Origin.Y + float64(i)*t.spacing()
		// A line's glyphs can reach past its box, by an accent, a tall
		// emoji or a long tail, by up to a line more.
		if upright && !dst.visiblePaintBounds(Rct(Pt(r.Origin.X, y), Sz(r.Size.W, t.spacing())), t.spacing()) {
			continue
		}
		if t.paintBidi(dst, i, r, align, y, col, place) {
			continue
		}
		op.GeoM.Reset()
		place(op, x, y)
		drawText(dst.Image, line, face, op)
	}
}

// StyledWidget sets the text style its subtree inherits. Build one with
// Styled.
type StyledWidget struct {
	props property.Owner
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
func (s *StyledWidget) Style(ts TextStyle) *StyledWidget {
	defer property.Watch(&s.props, &s.style)()
	s.style = s.style.Merge(ts)
	return s
}

// Color sets the inherited text color.
func (s *StyledWidget) Color(c color.Color) *StyledWidget {
	defer property.Watch(&s.props, &s.style)()
	s.style.Color = c
	return s
}

// Font sets the inherited font.
func (s *StyledWidget) Font(f *Font) *StyledWidget {
	defer property.Watch(&s.props, &s.style)()
	s.style.Font = f
	return s
}

// Weight sets the inherited font weight.
func (s *StyledWidget) Weight(w FontWeight) *StyledWidget {
	defer property.Watch(&s.props, &s.style)()
	s.style.Weight = w
	return s
}

// Size sets the inherited font size.
func (s *StyledWidget) Size(px float64) *StyledWidget {
	defer property.Watch(&s.props, &s.style)()
	s.style.Size = px
	return s
}

// LineHeight sets the inherited line height.
func (s *StyledWidget) LineHeight(mult float64) *StyledWidget {
	defer property.Watch(&s.props, &s.style)()
	s.style.LineHeight = mult
	return s
}

// Layout implements Widget.
func (s *StyledWidget) Layout(c Constraints, env Env) Size {
	defer s.props.Layout().EndLayout()
	return s.child.Layout(c, env.WithText(s.style))
}

// Baseline implements Baseliner: the child's.
func (s *StyledWidget) Baseline() (float64, bool) { return baselineOf(s.child) }

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
func Provide[T any](k EnvKey[T], v T, child Widget) *EnvWidget {
	return &EnvWidget{with: func(e Env) Env { return e.With(k, v) }, child: child}
}

// WithEnv transforms the inherited environment for a subtree during layout.
// The transform should be pure so equivalent layouts reuse environment revisions.
func WithEnv(transform func(Env) Env, child Widget) *EnvWidget {
	return &EnvWidget{with: transform, child: child}
}

// Layout implements Widget.
func (e *EnvWidget) Layout(c Constraints, env Env) Size { return e.child.Layout(c, e.with(env)) }

// Baseline implements Baseliner: the child's.
func (e *EnvWidget) Baseline() (float64, bool) { return baselineOf(e.child) }

// Paint implements Widget.
func (e *EnvWidget) Paint(dst *Canvas, r Rect) { dst.Paint(e.child, r) }

// BoxWidget paints a rectangle and lays an optional child inside its padding.
// Build one with Box.
type BoxWidget struct {
	widthProp   property.Value[float64]
	heightProp  property.Value[float64]
	props       property.Owner
	shadows     []ShadowStyle
	fill        shownColor
	gradient    Gradient // as given; see BoxWidget.Gradient
	radius      float64
	borderWidth float64
	borderColor shownColor

	shownShadows  []ShadowStyle // shadows with their colors resolved
	shownGradient Gradient      // gradient with its colors resolved
	padding       EdgeInsets
	width         float64 // 0 means "as small as the child allows"
	height        float64
	maxW, maxH    float64 // 0 means no limit
	child         Widget

	childRect Rect // inside the padding, from the last Layout
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
func (b *BoxWidget) Fill(c color.Color) *BoxWidget {
	b.fill.set(c, b.props.Changed)
	return b
}

// Shadow replaces the outer shadow layers. Calling it without arguments clears
// them. Shadows paint in argument order and do not reserve layout space.
func (b *BoxWidget) Shadow(styles ...ShadowStyle) *BoxWidget {
	themed := func(s ShadowStyle) bool { return IsEnvColor(s.Color) }
	if !slices.Equal(b.shadows, styles) && (slices.ContainsFunc(b.shadows, themed) || slices.ContainsFunc(styles, themed)) {
		b.props.Changed()
	}
	b.shadows = append(b.shadows[:0], styles...)
	b.shownShadows = append(b.shownShadows[:0], styles...)
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
	b.padding = Insets(sides...)
	return b
}

// Padding sets per-side padding.
func (b *BoxWidget) Padding(e EdgeInsets) *BoxWidget {
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
func (b *BoxWidget) BindWidth(r Readable[float64]) *BoxWidget {
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
func (b *BoxWidget) BindHeight(r Readable[float64]) *BoxWidget {
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
func (b *BoxWidget) Layout(c Constraints, env Env) Size {
	defer b.props.Layout().EndLayout()
	b.fill.layout(env)
	b.borderColor.layout(env)
	b.shownGradient = b.gradient.resolve(env)
	for i, s := range b.shadows {
		b.shownShadows[i].Color = ResolveColor(s.Color, env)
	}
	b.width, b.height = b.widthProp.Get(), b.heightProp.Get()
	var measure layout.MeasureFunc
	if b.child != nil {
		measure = func(_ int, c Constraints) layout.Measurement { return measureWidget(b.child, c, env) }
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

// Paint implements Widget.
func (b *BoxWidget) Paint(dst *Canvas, r Rect) {
	for _, s := range b.shownShadows {
		dst.Shadow(r, b.radius, s)
	}
	dst.FillRoundRect(r, b.radius, b.fill.shown)
	dst.FillGradientRoundRect(r, b.radius, b.shownGradient)
	if b.borderWidth > 0 {
		dst.StrokeRoundRect(r, b.radius, b.borderWidth, b.borderColor.shown)
	}
	if b.child != nil {
		dst.Paint(b.child, Rct(r.Origin.Add(b.childRect.Origin), b.childRect.Size))
	}
}

// Padding surrounds child with empty space, using the CSS shorthand Insets
// accepts: Padding(w, 8), Padding(w, 4, 12) or Padding(w, 1, 2, 3, 4). It is
// a Box with no fill, so the rest of Box's setters stay available.
func Padding(child Widget, sides ...float64) *BoxWidget { return Box(child).Pad(sides...) }

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
	children   []Widget

	rects  []Rect // borrowed from engine, or EachWidget.virtualRects on its virtual paths
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
		if isAbsent(child) {
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
func (f *flow) axis() layout.Axis { return pick(f.horizontal, layout.Horizontal, layout.Vertical) }

func (f *flow) main(s Size) float64  { return f.axis().Main(s) }
func (f *flow) cross(s Size) float64 { return f.axis().Cross(s) }

func (f *flow) size(main, cross float64) Size { return f.axis().Size(main, cross) }

func (f *flow) constraints(mainMin, mainMax, crossMin, crossMax float64) Constraints {
	return f.axis().Constraints(mainMin, mainMax, crossMin, crossMax)
}

// stretched returns the cross-axis size to give a child (v = 0) or the
// flow itself (v = the widest child): with AlignStretch it fills the
// available cross space when that is bounded.
func (f *flow) stretched(crossMax, v float64) float64 { return f.align.Stretched(crossMax, v) }

// placeRow puts virtual row j at main along the axis, aligned across a
// list that came out as big as result.
func (f *flow) placeRow(j int, main float64, result Size) {
	s := f.rects[j].Size
	f.rects[j].Origin = f.axis().Point(main, (f.cross(result)-f.cross(s))*f.align.Fraction())
}

// gapFor returns the gap to use: Space times the environment unit, else Gap.
func (f *flow) gapFor(env Env) float64 {
	if f.space > 0 {
		return f.space * env.Spacing()
	}
	return f.gap
}

func (f *flow) layout(c Constraints, env Env) Size {
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
	}, f.items, func(i int, c Constraints) layout.Measurement {
		m := measureWidget(f.children[i], c, env)
		if byBaseline {
			m.Baseline, m.HasBaseline = baselineOf(f.children[i])
		}
		return m
	})
	f.rects = result.Children
	return result.Size
}

// measureWidget is the boundary between widget state and geometry calculation.
func measureWidget(w Widget, c Constraints, env Env) layout.Measurement {
	size := w.Layout(c, env)
	return layout.Measurement{Size: size, Absent: isAbsent(w)}
}

// measureChildren measures children[i] for a multi-child layout.
func measureChildren(children []Widget, env Env) layout.MeasureFunc {
	return func(i int, c Constraints) layout.Measurement { return measureWidget(children[i], c, env) }
}

// paintRects paints each child at its rectangle, moved to r's origin.
func paintRects(dst *Canvas, r Rect, children []Widget, rects []Rect) {
	for i, child := range children {
		dst.Paint(child, Rct(r.Origin.Add(rects[i].Origin), rects[i].Size))
	}
}

func (f *flow) paint(dst *Canvas, r Rect) { paintRects(dst, r, f.children, f.rects) }

// resize returns s with length n and every element zeroed, reusing s's array.
func resize[T any](s []T, n int) []T {
	s = slices.Grow(s[:0], n)[:n]
	clear(s)
	return s
}

// pick is fn.Pick under the name the package reads it by.
func pick[T any](cond bool, a, b T) T { return fn.Pick(cond, a, b) }

// ColumnWidget stacks its children vertically. Build one with Column.
type ColumnWidget struct {
	props property.Owner
	flow
}

// Column stacks children top to bottom.
func Column(children ...Widget) *ColumnWidget {
	return &ColumnWidget{flow: flow{children: children}}
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
// the arrangement of a form or a page.
func (col *ColumnWidget) Stretch() *ColumnWidget { return col.Align(AlignStretch) }

// Layout implements Widget.
func (col *ColumnWidget) Layout(c Constraints, env Env) Size {
	defer col.props.Layout().EndLayout()
	return col.layout(c, env)
}

// Paint implements Widget.
func (col *ColumnWidget) Paint(dst *Canvas, r Rect) { col.paint(dst, r) }

// RowWidget lines its children up horizontally. Build one with Row.
type RowWidget struct {
	props property.Owner
	flow
}

// Row lines children up left to right, centered on the row's height; a
// Column starts its children at the left.
func Row(children ...Widget) *RowWidget {
	return &RowWidget{flow: flow{horizontal: true, align: AlignCenter, children: children}}
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

// Stretch makes every child as tall as the row: Align(AlignStretch).
func (row *RowWidget) Stretch() *RowWidget { return row.Align(AlignStretch) }

// Layout implements Widget.
func (row *RowWidget) Layout(c Constraints, env Env) Size {
	defer row.props.Layout().EndLayout()
	return row.layout(c, env)
}

// Paint implements Widget.
func (row *RowWidget) Paint(dst *Canvas, r Rect) { row.paint(dst, r) }

// FlexWidget marks a child of a Row or Column as one that takes a share of
// the leftover main-axis space. Anywhere else it is transparent. Build one
// with Flex, Expanded or Spacer.
type FlexWidget struct {
	flex  float64
	child Widget
	loose bool // the share is the most it takes, not what it must
}

// Flex gives child weight shares of the space its Row or Column has left
// after the rigid children are placed.
func Flex(child Widget, weight float64) *FlexWidget { return &FlexWidget{flex: weight, child: child} }

// Expanded is Flex with weight 1.
func Expanded(child Widget) *FlexWidget { return Flex(child, 1) }

// Flexible is Flex with weight 1 that takes up to its share, and no more
// than it needs: a Scroll of tabs as wide as its tabs until they fill the
// row, so what follows them follows them until it is pushed to the end.
func Flexible(child Widget) *FlexWidget { return &FlexWidget{flex: 1, child: child, loose: true} }

// Spacer is an empty Expanded: it pushes its neighbours apart.
func Spacer() *FlexWidget { return Expanded(Box()) }

// Layout implements Widget.
func (f *FlexWidget) Layout(c Constraints, env Env) Size { return f.child.Layout(c, env) }

// Baseline implements Baseliner: the child's.
func (f *FlexWidget) Baseline() (float64, bool) { return baselineOf(f.child) }

// Paint implements Widget.
func (f *FlexWidget) Paint(dst *Canvas, r Rect) { dst.Paint(f.child, r) }

// StackWidget layers its children on top of each other, first at the bottom.
// Build one with Stack.
type StackWidget struct {
	engine layout.Scratch

	props    property.Owner
	expand   bool
	children []Widget

	rects []Rect // borrowed from engine
}

// Stack layers children in order, all anchored at the top-left corner. It is
// as large as its largest child unless Expand is set. Wrap a child in Align
// to place it elsewhere.
func Stack(children ...Widget) *StackWidget {
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
func (st *StackWidget) Layout(c Constraints, env Env) Size {
	defer st.props.Layout().EndLayout()
	result := st.engine.Stack(c, st.expand, len(st.children), measureChildren(st.children, env))
	st.rects = result.Children
	return result.Size
}

// Paint implements Widget.
func (st *StackWidget) Paint(dst *Canvas, r Rect) { paintRects(dst, r, st.children, st.rects) }

// AlignWidget fills the space it is given and places one child within it.
// Build one with Align or Center.
type AlignWidget struct {
	props property.Owner
	x, y  float64
	child Widget

	childRect Rect // where child sits, from the last Layout
}

// Align fills the available space and places child in it, centered until At
// or one of Left, Right, Top, Bottom moves it.
func Align(child Widget) *AlignWidget { return &AlignWidget{x: 0.5, y: 0.5, child: child} }

// Center is Align with the child in the middle.
func Center(child Widget) *AlignWidget { return Align(child) }

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
func (a *AlignWidget) Layout(c Constraints, env Env) Size {
	defer a.props.Layout().EndLayout()
	result := layout.Align(c, a.x, a.y, func(_ int, c Constraints) layout.Measurement {
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
func (a *AlignWidget) Paint(dst *Canvas, r Rect) {
	dst.Paint(a.child, Rct(r.Origin.Add(a.childRect.Origin), a.childRect.Size))
}

// List builds one child per item and stacks them like a Column, which is
// what it returns: every Column setter applies to it.
func List[T any](items []T, item func(T) Widget) *ColumnWidget {
	return Column(Children(items, item)...)
}

// Viewport is what a Scroll tells the subtree it lays out: how far along
// the scroll axis the window starts and how long it is, in logical pixels.
// A list that knows its items' sizes can then lay out only the ones in
// view; EachKeyed does with ItemExtent.
type Viewport struct {
	Offset, Extent float64
	Horizontal     bool
}

var (
	viewportKey = NewEnvKey[Viewport]("viewport")
	// The window of the nearest Scroll along each axis: a list inside a
	// Scroll that scrolls the other way, as a diff's long lines do, is
	// still windowed by the one along its own.
	viewportXKey = NewEnvKey[Viewport]("viewport x")
	viewportYKey = NewEnvKey[Viewport]("viewport y")
)

// ScrollViewport returns the window of the nearest enclosing Scroll, if any.
func ScrollViewport(env Env) (Viewport, bool) { return env.Get(viewportKey) }

// viewportAlong returns the window of the nearest enclosing Scroll that
// scrolls along the x axis, for horizontal, or the y axis.
func viewportAlong(env Env, horizontal bool) (Viewport, bool) {
	return env.Get(pick(horizontal, viewportXKey, viewportYKey))
}

// ScrollWidget shows a window onto a child that may be taller (or, with
// Horizontal, wider) than the space it has, and moves that window with the
// wheel or by dragging the scrollbar thumb. Build one with Scroll.
type ScrollWidget struct {
	props      property.Owner
	child      Widget
	horizontal bool
	shrink     bool // along its axis, as long as its content and no longer
	speed      float64
	bar        color.Color
	barSet     bool
	bound      Binding[float64]
	offset     float64
	follow     bool
	id         any

	// FollowEnd's state. A scroll laid out for the first time cannot know
	// yet whether it replaces one that was at its end, as Adopt says only
	// at paint, so it leaves the decision to Paint.
	laidOut, decideAtPaint bool
	adoptedAtEnd           *bool // whether the scroll it adopted was at its end

	childSize             Size
	viewport              Size
	laidAt                float64 // the offset the child was last laid out for
	anchorShift           float64 // how far the child asked, as it laid out, to move to keep what is in view
	cache                 *CachedWidget
	rect                  Rect // where the window was last painted
	thumbHit              Rect
	barEnv                Env
	thumbHovered          bool
	dragging              bool
	dragStart, dragOffset float64

	phys scrollPhysics // overscroll, snapping and animated scrolls; see scroll_physics.go

	fade float64 // how far in the content fades out at an end with more past it
}

// Key gives the scroll an identity, so a rebuilt one that also moved keeps
// its offset. Without one its Rect identifies it.
func (s *ScrollWidget) Key(k any) *ScrollWidget { s.id = k; return s }

// HitID implements Identified.
func (s *ScrollWidget) HitID() any { return s.id }

// Adopt implements Adopter: a rebuilt scroll keeps its offset.
func (s *ScrollWidget) Adopt(prev any) {
	if p, ok := prev.(*ScrollWidget); ok {
		s.dragging, s.dragStart, s.dragOffset = p.dragging, p.dragStart, p.dragOffset
		s.thumbHovered = p.thumbHovered
		s.phys.adopt(&p.phys)
		defer s.phys.reapply(s) // once the offset is adopted too
		atEnd := p.laidOut && p.atEnd()
		s.adoptedAtEnd = &atEnd
		if s.bound != nil {
			return
		}
		s.offset = p.position()
		if s.offset != s.laidAt {
			s.cache.invalidate()
			reactive.RequestLayout()
		}
	}
}

// Scroll lets child take any height and scrolls it within the space Scroll
// is given. The offset lives in the widget and carries across a rebuild;
// bind it to a StateValue with Offset to read or set it.
func Scroll(child Widget) *ScrollWidget {
	return &ScrollWidget{child: child, speed: 20, id: reactive.AutoID()}
}

// Horizontal scrolls along the x axis instead of the y axis.
func (s *ScrollWidget) Horizontal() *ScrollWidget { s.horizontal = true; return s }

// FadeEdges fades the content out over width logical pixels at an end
// with more of it past that end, and as the end comes near, the fade
// narrows to nothing: a hint that the content goes on, without a line or
// a surface. It paints the content through a layer.
func (s *ScrollWidget) FadeEdges(width float64) *ScrollWidget { s.fade = max(width, 0); return s }

// Shrink makes the scroll, along its axis, only as long as its content
// while that fits, where it would otherwise take all it is offered: a row
// of tabs in a Flexible, with what follows them right after the last.
func (s *ScrollWidget) Shrink() *ScrollWidget { s.shrink = true; return s }

// Speed sets how many pixels one wheel unit moves. A unit is one mouse
// notch on the desktop; on the web, where browsers report pixels, it is
// 20 CSS pixels, so the default matches the browser's own scrolling.
func (s *ScrollWidget) Speed(px float64) *ScrollWidget {
	defer property.Watch(&s.props, &s.speed)()
	s.speed = px
	return s
}

// Bar overrides the inherited scrollbar color; nil hides the bar.
func (s *ScrollWidget) Bar(c color.Color) *ScrollWidget {
	defer property.Watch(&s.props, &s.bar)()
	defer property.Watch(&s.props, &s.barSet)()
	s.bar, s.barSet = c, true
	return s
}

// BindOffset binds the scroll position to sig: wheel input writes it, and
// writing it scrolls. Use it to keep the position across rebuilds or to
// scroll programmatically.
func (s *ScrollWidget) BindOffset(sig Binding[float64]) *ScrollWidget {
	property.Require(sig, "BindOffset")
	if !property.Same(s.bound, sig) {
		s.bound = sig
		s.props.Changed()
	}
	return s
}

// FollowEnd keeps the window at the end as the content grows, the way a
// log or a terminal follows new lines, for as long as it is there: scrolling
// back stops following and scrolling to the end again resumes it. A new
// scroll starts at the end.
func (s *ScrollWidget) FollowEnd() *ScrollWidget {
	defer property.Watch(&s.props, &s.follow)()
	s.follow = true
	return s
}

// Offset detaches a binding and sets local scroll position, clamped at layout.
func (s *ScrollWidget) Offset(v float64) *ScrollWidget {
	if s.bound != nil || !property.Equal(s.offset, v) {
		s.bound = nil
		s.offset = v
		s.props.Changed()
	}
	return s
}

func (s *ScrollWidget) extent(sz Size) float64 { return pick(s.horizontal, sz.W, sz.H) }

func (s *ScrollWidget) maxOffset() float64 {
	return max(s.extent(s.childSize)-s.extent(s.viewport), 0)
}

func (s *ScrollWidget) position() float64 {
	if s.bound != nil {
		return Untrack(s.bound.Get)
	}
	return s.offset
}

func (s *ScrollWidget) scrollTo(v float64) {
	v = clamp(v, 0, s.maxOffset())
	if s.bound != nil {
		s.bound.Set(v)
	} else if v != s.offset {
		s.offset = v
		reactive.RequestLayout() // what a virtualized child shows depends on it
	}
	if v != s.laidAt {
		s.cache.invalidate()
	}
}

// Layout implements Widget.
func (s *ScrollWidget) Layout(c Constraints, env Env) Size {
	defer s.props.Layout().EndLayout()
	s.barEnv = env
	if !s.barSet {
		s.bar = env.ScrollStyle().Color
	}
	inner := c.Max()
	if s.horizontal {
		inner.W = Unbounded
	} else {
		inner.H = Unbounded
	}
	s.cache, _ = env.Get(cacheOwner)
	// Judged against the last layout, before the content can grow.
	follow := s.follow && s.laidOut && s.atEnd()
	if s.follow && !s.laidOut {
		s.decideAtPaint = true
	}
	s.phys.step(s)
	s.laidAt = s.position()
	vp := Viewport{Offset: s.laidAt, Extent: s.extent(c.Max()), Horizontal: s.horizontal}
	env = env.With(viewportKey, vp).With(scrollPortKey, s).
		With(pick(s.horizontal, viewportXKey, viewportYKey), vp).With(pick(s.horizontal, scrollPortXKey, scrollPortYKey), s)
	s.childSize = s.child.Layout(Loose(inner), env)
	s.viewport = c.Constrain(Sz(bounded(c.MaxW, s.childSize.W), bounded(c.MaxH, s.childSize.H)))
	if s.shrink {
		// Constrain keeps it within c, so a tight c still decides.
		if s.horizontal {
			s.viewport.W = c.Constrain(Sz(s.childSize.W, 0)).W
		} else {
			s.viewport.H = c.Constrain(Sz(0, s.childSize.H)).H
		}
	}
	s.laidOut = true
	shift := s.anchorShift
	s.anchorShift = 0
	if shift != 0 && s.phys.animating {
		s.phys.anim.from += shift
		s.phys.anim.to += shift
	}
	s.scrollTo(pick(follow, s.maxOffset(), s.position()+shift))
	s.phys.attach(s)
	return s.viewport
}

// keepInView moves the window by d once the child is laid out: a list
// whose rows above the window changed size asks for it, so what is in view
// stays where it is.
func (s *ScrollWidget) keepInView(d float64) { s.anchorShift += d }

// followFirst settles FollowEnd for a scroll's first layout, once Adopt
// has had its chance: a new scroll starts at the end, and one that
// replaces another follows if that one was at its end.
func (s *ScrollWidget) followFirst() {
	if !s.decideAtPaint {
		return
	}
	s.decideAtPaint = false
	if s.adoptedAtEnd == nil || *s.adoptedAtEnd {
		s.scrollTo(s.maxOffset())
	}
}

// atEnd reports whether the window shows the end of the content, within
// half a pixel.
func (s *ScrollWidget) atEnd() bool { return s.position() >= s.maxOffset()-0.5 }

// Paint implements Widget.
func (s *ScrollWidget) Paint(dst *Canvas, r Rect) {
	if s.position() != s.laidAt {
		// Written through the bound signal since the layout: a virtualized
		// child must be laid out again for the new window.
		s.cache.invalidate()
	}
	s.rect = r
	dst.HitPointer(r, s) // adopts the scroll this one replaces
	s.followFirst()
	s.phys.paint(s)
	dst.Node(r, Node{Role: RoleGroup, Actions: ActionScrollIntoView}, func(dst *Canvas) {
		clip := dst.Clip(r)
		if s.fade <= 0 {
			s.paintContent(clip, r)
			return
		}
		pos := s.position()
		lead, trail := min(s.fade, max(pos, 0)), min(s.fade, max(s.maxOffset()-pos, 0))
		clip.FadeEdges(r, s.horizontal, lead, trail, func(dst *Canvas) { s.paintContent(dst, r) })
	})
	s.paintBar(dst, r)
}

func (s *ScrollWidget) thumbLength() float64 {
	track := s.extent(s.rect.Size)
	if s.extent(s.childSize) <= 0 {
		return 0
	}
	return min(track, max(track*track/s.extent(s.childSize), 16))
}

func (s *ScrollWidget) paintBar(dst *Canvas, r Rect) {
	s.thumbHit = Rect{}
	track, content := s.extent(r.Size), s.extent(s.childSize)
	bar := ResolveColor(s.bar, s.barEnv)
	if bar == nil || content <= track || track <= 0 {
		s.dragging, s.thumbHovered = false, false
		return
	}
	const margin, hitWidth = 2.0, 10.0
	style := s.barEnv.ScrollStyle()
	active := s.thumbHovered || s.dragging
	amount := dst.Ease(Anchor{Rect: r, ID: scrollThumb{s}.HitID()}, scrollThumbHoverSlot, pick(active, 1.0, 0.0), s.barEnv.Motion(style.Duration))
	thickness := 3 + 1.5*amount
	fill := bar
	if amount > 0 {
		// Keep the thumb close to its resting gray, including while dragging.
		fill = LerpColor(bar, style.HoverColor, amount*pick(s.dragging, .28, .18))
	}
	thumb := s.thumbLength()
	at := (track - thumb) * s.position() / s.maxOffset()
	var visual Rect
	if s.horizontal {
		visual = Rct(Pt(r.Origin.X+at, r.Origin.Y+r.Size.H-thickness-margin), Sz(thumb, thickness))
		s.thumbHit = Rct(Pt(r.Origin.X+at, r.Origin.Y+max(0, r.Size.H-hitWidth)), Sz(thumb, min(hitWidth, r.Size.H)))
	} else {
		visual = Rct(Pt(r.Origin.X+r.Size.W-thickness-margin, r.Origin.Y+at), Sz(thickness, thumb))
		s.thumbHit = Rct(Pt(r.Origin.X+max(0, r.Size.W-hitWidth), r.Origin.Y+at), Sz(min(hitWidth, r.Size.W), thumb))
	}
	// Register after the content so grabbing the thumb never activates a row.
	dst.HitPointer(s.thumbHit, scrollThumb{s})
	dst.HitCursor(s.thumbHit, CursorShapePointer)
	dst.Clip(r).FillRoundRect(visual, thickness/2, fill)
}

// A separate handler keeps the thumb from being treated as a second scroll
// viewport when keyboard focus asks enclosing Revealer widgets to move.
type scrollThumb struct{ scroll *ScrollWidget }

var scrollThumbHoverSlot = NewSlot[*Motion]("scroll thumb hover")

func (h scrollThumb) HandlePointer(ev PointerEvent) bool {
	switch ev.Kind {
	case PointerMove, PointerEnter:
		h.scroll.thumbHovered = h.scroll.thumbHit.Contains(ev.Pos)
	case PointerExit:
		h.scroll.thumbHovered = false
	}
	return h.scroll.HandlePointer(ev)
}
func (h scrollThumb) CaptureTouchDrag() bool { return h.scroll.dragging }
func (h scrollThumb) HitID() any {
	if h.scroll.id != nil {
		return struct{ Thumb any }{h.scroll.id}
	}
	return struct{ Thumb Rect }{h.scroll.rect}
}

// CaptureTouchDrag keeps a thumb drag from becoming content panning.
func (s *ScrollWidget) CaptureTouchDrag() bool { return s.dragging }

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
		s.phys.animating = false // the reveal wins over an eased scroll
		s.scrollTo(s.position() + lo)
	case hi > extent:
		s.phys.animating = false
		s.scrollTo(s.position() + hi - extent)
	}
}

// HandlePointer implements PointerHandler: wheel movement along the scroll
// axis moves the window.
func (s *ScrollWidget) HandlePointer(ev PointerEvent) bool {
	axis := pick(s.horizontal, ev.Pos.X, ev.Pos.Y)
	switch ev.Kind {
	case PointerDown:
		if ev.Button != MouseButtonLeft || !s.thumbHit.Contains(ev.Pos) || s.extent(s.rect.Size) <= s.thumbLength() {
			return false
		}
		s.dragging, s.dragStart, s.dragOffset = true, axis, s.position()
		return true
	case PointerDrag:
		if !s.dragging {
			return false
		}
		travel := s.extent(s.rect.Size) - s.thumbLength()
		s.phys.input(s)
		if travel > 0 {
			s.scrollTo(s.dragOffset + (axis-s.dragStart)*s.maxOffset()/travel)
		}
		return true
	case PointerUp:
		if !s.dragging || ev.Button != MouseButtonLeft {
			return false
		}
		s.dragging = false
		return true
	case PointerMove, PointerEnter:
		return s.thumbHit.Contains(ev.Pos)
	case PointerTap:
		return s.thumbHit.Contains(ev.Pos)
	}

	if ev.Kind != PointerScroll {
		return false
	}
	delta := pick(s.horizontal, ev.Scroll.X, ev.Scroll.Y)
	if delta == 0 || s.maxOffset() == 0 {
		return false
	}
	speed := s.speed
	if ev.ScrollPixels {
		speed = 1
	}
	return s.wheel(delta*speed, ev.ScrollMomentum)
}

// WrapWidget lines its children up like a Row and starts a new line when
// the next child would not fit. Build one with Wrap.
type WrapWidget struct {
	engine layout.Scratch

	props    property.Owner
	children []Widget
	gap      float64 // between children on a line
	runGap   float64 // between lines
	space    float64 // both, in environment spacing units, when set
	align    CrossAlign

	rects []Rect // borrowed from engine
}

// Wrap flows children left to right, wrapping onto new lines at the width
// it is given, the way words fill a paragraph; tags and toolbars want it.
func Wrap(children ...Widget) *WrapWidget { return &WrapWidget{children: children} }

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
func (w *WrapWidget) Layout(c Constraints, env Env) Size {
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
func (w *WrapWidget) Paint(dst *Canvas, r Rect) { paintRects(dst, r, w.children, w.rects) }

// GridWidget lays its children out in equal-width columns. Build one with
// Grid.
type GridWidget struct {
	engine layout.Scratch

	props    property.Owner
	cols     int
	children []Widget
	gap      float64
	rowGap   float64
	space    float64 // both, in environment spacing units, when set

	rects []Rect // borrowed from engine
	cellW float64
}

// Grid places children in rows of cols cells, left to right then top to
// bottom. Each column takes an equal share of the width; each row is as
// tall as its tallest cell, and children are given the cell width tight, so
// text and boxes align down the columns.
func Grid(cols int, children ...Widget) *GridWidget {
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
func (g *GridWidget) Layout(c Constraints, env Env) Size {
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
func (g *GridWidget) Paint(dst *Canvas, r Rect) { paintRects(dst, r, g.children, g.rects) }
