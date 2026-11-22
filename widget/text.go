package widget

import (
	"image/color"
	"math"
	"slices"
	"strconv"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/gem"
	"github.com/ironpark/ggui/inspect"
	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/kernel"
	"github.com/ironpark/ggui/internal/paragraph"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/text"
	"github.com/ironpark/ggui/property"
)

// textBidi is a Text's lines in visual order, for the lines that need
// reordering: every line in a right-to-left paragraph, and a line holding
// right-to-left text in a left-to-right one. Other lines draw as they are.
//
// A Text keeps one only once it is laid out under DirRTL or with a line
// that needs reordering; most never do.
type textBidi struct {
	dir     ggui.TextDirection
	lang    string
	key     wrapKey
	laid    bool
	lines   [][]paragraph.Piece     // nil for a line drawn as it is
	styles  [][]paragraph.LineStyle // each reordered line's styles
	pieceOp text.DrawOptions        // reused by paragraph.Draw, which would allocate one a paint
}

// layoutBidi orders the lines Layout just wrapped for the Env's direction.
// rewrapped says the lines changed since the last call.
func (t *TextWidget) layoutBidi(env ggui.Env, rewrapped bool) {
	dir := env.Direction()
	if t.bidi == nil {
		// Under DirLTR, lines with no right-to-left text draw as they
		// are, whatever the language: they need no textBidi until the
		// lines or the direction change.
		if dir != ggui.DirRTL && (!rewrapped || !slices.ContainsFunc(t.lines, paragraph.HasRTL)) {
			return
		}
		t.bidi = new(textBidi)
	}
	b := t.bidi
	lang := env.Language()
	if b.laid && b.dir == dir && b.lang == lang && b.key == t.wrapped {
		return
	}
	b.laid, b.dir, b.lang, b.key = true, dir, lang, t.wrapped
	b.lines, b.styles = b.lines[:0], b.styles[:0]
	rtl := dir == ggui.DirRTL
	for _, line := range t.lines {
		if !paragraph.NeedsBidi(line, rtl) {
			b.lines, b.styles = append(b.lines, nil), append(b.styles, nil)
			continue
		}
		styles := t.lineStyles(line)
		b.lines, b.styles = append(b.lines, paragraph.LayoutLine(line, styles, rtl, lang)), append(b.styles, styles)
	}
}

// sanitizeScale returns s when it is a finite positive display scale, else 1.
func sanitizeScale(s float64) float64 {
	if s > 0 && !math.IsNaN(s) && !math.IsInf(s, 0) {
		return s
	}
	return 1
}

// lineStyles is a line of a Text in its one style, scaled to the physical
// display scale the lines were wrapped at so bidi measurement matches paint.
func (t *TextWidget) lineStyles(line string) []paragraph.LineStyle {
	st, _ := t.engine()
	st.Size *= sanitizeScale(t.wrapped.scale)
	return []paragraph.LineStyle{{Start: 0, End: len(line), Style: st}}
}

// paintBidi draws line i in visual order, aligned at align in r, and
// reports whether it did; a line that needs no reordering is left to the
// caller.
func (t *TextWidget) paintBidi(dst *ggui.Canvas, i int, r ggui.Rect, align float64, col color.Color, place func(op *text.DrawOptions, x, y float64)) bool {
	if t.bidi == nil || i >= len(t.bidi.lines) || t.bidi.lines[i] == nil {
		return false
	}
	ps, line := t.bidi.lines[i], t.lines[i]
	bidiScale := sanitizeScale(t.bidi.key.scale)
	scale := sanitizeScale(dst.Scale())
	x := r.Origin.X + (r.Size.W-paragraph.Width(ps)/bidiScale)*align
	y := t.lineY(r, i)
	baseline := t.faceAt(scale).Metrics().HAscent
	paragraph.Draw(dst.Image, &t.bidi.pieceOp, line, ps, t.bidi.styles[i], scale/bidiScale, baseline,
		func(int) color.Color { return col },
		func(op *text.DrawOptions) { place(op, x, y) })
	return true
}

// PaintRotated paints already-laid-out text clockwise around the center of r.
// It uses the regular glyph cache without allocating an intermediate image.
// Layout is unchanged; the caller reserves space for the rotated bounds.
func (t *TextWidget) PaintRotated(dst *ggui.Canvas, r ggui.Rect, radians float64) {
	if radians == 0 {
		t.Paint(dst, r)
		return
	}
	if math.IsNaN(radians) || math.IsInf(radians, 0) {
		return
	}
	center := r.Center()
	t.paintLines(dst, r, false, func(op *text.DrawOptions, x, y float64) {
		op.GeoM.Translate((x-center.X)*dst.Scale(), (y-center.Y)*dst.Scale())
		op.GeoM.Rotate(radians)
		op.GeoM.Translate(center.X*dst.Scale(), center.Y*dst.Scale())
	})
}

// TextWidget draws text, wrapping it to the width it is given. Build one
// with Text. Its style is resolved at layout: the Env's inherited TextStyle,
// then the widget's own setters on top, then built-in defaults for whatever
// is still unset.
type TextWidget struct {
	props     property.Owner
	value     string
	content   property.Value[string]
	style     ggui.TextStyle
	formatted bool
	wrap      bool
	ellipsis  bool
	alignSet  bool
	lineBox   bool // a line height was set: every line is a box of it
	maxLines  int
	align     float64
	role      ggui.Role
	opts      *textOpts // set by Textf and StyleKey only

	// Layout caches the wrapped lines, their widths and the size they add up
	// to, and re-measures only when the text, the face or the width it must
	// fit in changes.
	resolved ggui.TextStyle
	lines    []string
	// glyphs are each line's glyphs laid out in glyphFace, the face they
	// were last drawn in, kept until the lines are wrapped again: a frame
	// that draws them as they were, as a scroll's does, lays none out.
	glyphs    [][]text.LazyGlyph
	glyphFace text.Face
	widths    []float64
	natural   ggui.Size
	ascent    float64 // the first baseline, below the top
	lead      float64 // from the top to the first line's glyphs, in a line box; negative when they are taller than a line
	wrapped   wrapKey
	bidi      *textBidi        // the lines in visual order, when the direction or the text needs it
	rich      *selectableText  // what a Selectable Text lays out and paints as
	drawOp    text.DrawOptions // reused by paintLines, which would allocate one a paint
}

// textOpts are the setters of a Text that few Texts call, kept apart so
// that the rest do not carry them.
type textOpts struct {
	format        string
	formatArgs    []any
	styleKey      ggui.EnvKey[ggui.TextStyle]
	styleFallback ggui.TextStyle
}

// options returns the Text's textOpts, making them on first use.
func (t *TextWidget) options() *textOpts {
	if t.opts == nil {
		t.opts = new(textOpts)
	}
	return t.opts
}

// selectableText is the RichText a Selectable Text lays out and paints
// as, and the width and Env it last laid out in, to lay its lines out
// when rotated.
type selectableText struct {
	w    RichTextWidget
	maxW float64
	env  ggui.Env
}

// wrapKey is the input textlayout.Wrap was last run with.
type wrapKey struct {
	generation uint64
	value      string
	font       *ggui.Font
	weight     ggui.FontWeight
	size       float64
	lineHeight float64
	tracking   float64
	maxW       float64
	scale      float64
	maxLines   int32
	ellipsis   bool
	lineBox    bool
}

// Text draws s in the inherited style, wrapping at spaces when it is wider
// than the space it gets.
func Text(s string) *TextWidget {
	return (&TextWidget{wrap: true}).Content(s)
}

// StyleKey selects an inherited text style, merged below explicit setters.
// An optional fallback is used when the environment has no value for key.
func (t *TextWidget) StyleKey(key ggui.EnvKey[ggui.TextStyle], fallback ...ggui.TextStyle) *TextWidget {
	o := t.options()
	defer property.Watch(&t.props, &o.styleKey)()
	defer property.Watch(&t.props, &o.styleFallback)()
	o.styleKey = key
	o.styleFallback = ggui.TextStyle{}
	if len(fallback) > 0 {
		o.styleFallback = fallback[0]
	}
	return t
}

// Role sets the accessibility role; the default is RoleText.
func (t *TextWidget) Role(role ggui.Role) *TextWidget {
	defer property.Watch(&t.props, &t.role)()
	t.role = role
	return t
}

// TextOf borrows r and follows it during layout without owning a subscription.
func TextOf(r ggui.Readable[string]) *TextWidget { return Text("").BindContent(r) }

// Textf formats during layout, reading GetAny arguments like [ggui.Sprintf]. It copies
// args and creates no computation; construction needs no owner.
func Textf(format string, args ...any) *TextWidget {
	t := Text("")
	o := t.options()
	o.format = format
	o.formatArgs = append([]any(nil), args...)
	t.formatted = true
	return t
}

// Style merges ts onto the widget's own style.
func (t *TextWidget) Style(ts ggui.TextStyle) *TextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style = t.style.Merge(ts)
	return t
}

// Color sets the text color; nil inherits. Changing one color for another
// is paint-only, so a control can recolor its label as the pointer moves
// over it without laying it out again.
func (t *TextWidget) Color(c color.Color) *TextWidget {
	if t.rich != nil || (c == nil) != (t.style.Color == nil) || ggui.IsEnvColor(c) || ggui.IsEnvColor(t.style.Color) {
		defer property.Watch(&t.props, &t.style)()
	}
	t.style.Color = c
	return t
}

// Font sets the face; nil inherits.
func (t *TextWidget) Font(f *ggui.Font) *TextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style.Font = f
	return t
}

// Weight sets the font weight, drawn with the Font's face nearest it; zero
// inherits.
func (t *TextWidget) Weight(w ggui.FontWeight) *TextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style.Weight = w
	return t
}

// Italic draws the text in its font's italic; see ggui.TextStyle.Italic.
func (t *TextWidget) Italic() *TextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style.Italic = true
	return t
}

// Size sets the font size in pixels.
func (t *TextWidget) Size(px float64) *TextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style.Size = px
	return t
}

// LineHeight sets the height of a line as a multiple of Size, as CSS's
// line-height does: every line is a box that high with its glyphs centered
// in it, so one line at LineHeight(1) is exactly Size high whatever the
// font's own ascent and descent, which then reach a little past it. Text
// that no line height was set for, its own or an inherited one, is as tall
// as its glyphs, with 1.2 times the size between baselines.
func (t *TextWidget) LineHeight(mult float64) *TextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style.LineHeight = mult
	return t
}

// Tracking sets the space added after each letter as a multiple of Size,
// as CSS letter-spacing in em: negative sets the text tighter. Zero
// inherits.
func (t *TextWidget) Tracking(em float64) *TextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style.Tracking = em
	return t
}

// Content replaces any literal, reader or formatted source and refreshes
// measurement when needed. Call it on the UI goroutine.
func (t *TextWidget) Content(s string) *TextWidget {
	changed := t.content.Set(s) || t.formatted || t.value != s
	t.formatted = false
	if t.opts != nil {
		t.opts.format, t.opts.formatArgs = "", nil
	}
	t.value = s
	if changed {
		t.props.Changed()
	}
	return t
}

// BindContent follows r during layout without owning a subscription.
func (t *TextWidget) BindContent(r ggui.Readable[string]) *TextWidget {
	changed := t.content.Bind(r, "BindContent") || t.formatted
	t.formatted = false
	if t.opts != nil {
		t.opts.format, t.opts.formatArgs = "", nil
	}
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
//
// An EnvColor is kept as it is then, with no Env to resolve it in, and
// paints its RGBA.
func (t *TextWidget) current() ggui.TextStyle {
	if t.resolved.Font == nil {
		st := ggui.Env{}.ResolveText(t.style)
		if t.style.Color != nil {
			st.Color = t.style.Color
		}
		return st
	}
	return t.resolved
}

// engine is the current style as the text engine sets text in it, and the
// generation of its fonts.
// textEngineStyle is the root package's: see kernel.TextEngineStyle.
var textEngineStyle = kernel.TextEngineStyle.(func(ggui.TextStyle) (paragraph.Style, uint64))

func (t *TextWidget) engine() (paragraph.Style, uint64) { return textEngineStyle(t.current()) }

// dir is the direction the lines were last ordered for: DirLTR, unless
// they were laid out in another.
func (t *TextWidget) dir() ggui.TextDirection {
	if t.bidi == nil {
		return ggui.DirLTR
	}
	return t.bidi.dir
}

// startAlign is where a line of text starts by default: the left edge, or
// the right one under DirRTL.
func startAlign(d ggui.TextDirection) float64 {
	if d == ggui.DirRTL {
		return 1
	}
	return 0
}

// faceAt returns the face at the resolved size times scale, so that on a
// HiDPI Canvas glyphs are rasterized at full resolution instead of scaled up.
func (t *TextWidget) faceAt(scale float64) text.Face {
	st, _ := t.engine()
	return faceOf(st, scale)
}

// faceOf is the face text in st is drawn in at scale times its size, its
// tracking with it.
func faceOf(st paragraph.Style, scale float64) text.Face {
	size := st.Size * scale
	return st.Font.SpacedFaceFor(size, st.Weight, st.Tracking*size, st.Emoji)
}

// spacing is the distance between baselines. It is rounded as it is
// returned, whether or not it is inlined: fused into the sums Layout makes
// of it, it would come out a rounding error off, and a line box of 20
// would be 20.000000000000004 tall.
func (t *TextWidget) spacing() float64 {
	st := t.current()
	return float64(st.Size * st.LineHeight)
}

// Layout implements Widget.
func (t *TextWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer t.props.Layout().EndLayout()
	var opts textOpts
	if t.opts != nil {
		opts = *t.opts
	}
	if t.formatted {
		t.value = reactive.Format(opts.format, opts.formatArgs)
	} else {
		t.value = t.content.Get()
	}
	style, ok := env.Get(opts.styleKey)
	if !ok {
		style = opts.styleFallback
	}
	own := style.Merge(t.style)
	t.resolved = env.ResolveText(own)
	t.lineBox = lineHeightSet(env, own)
	if t.rich != nil {
		// A Selectable Text lays out and paints as its RichText. Only
		// PaintRotated draws the lines, so they wait for it.
		t.rich.maxW, t.rich.env = c.MaxW, env
		return t.layoutRich(c, env, own)
	}
	t.layoutLines(c.MaxW, env, sanitizeScale(ggui.UseScale()))
	return c.Constrain(t.natural)
}

// lineHeightSet reports whether text in style s under env was given a line
// height, by s or by a style it inherits, rather than left the default:
// its lines are then boxes of that height.
func lineHeightSet(env ggui.Env, s ggui.TextStyle) bool {
	return s.LineHeight != 0 || env.Text().LineHeight != 0
}

// layoutLines wraps, cuts and orders the lines for a width of maxW at scale,
// when any of what they depend on changed since the last time. Measuring at
// the display scale avoids advance drift between layout and paint caused by
// integer/fixed-point shaping rounding at fractional scales (125%/150%/175%).
func (t *TextWidget) layoutLines(maxW float64, env ggui.Env, scale float64) {
	st, gen := t.engine()
	face := faceOf(st, scale)
	fits := t.wrap || t.ellipsis || t.maxLines > 0
	key := wrapKey{generation: gen, value: t.value, font: t.resolved.Font, weight: t.resolved.Weight, size: t.resolved.Size, lineHeight: t.resolved.LineHeight, tracking: t.resolved.Tracking,
		maxW: gem.Pick(fits, maxW, 0), scale: scale, maxLines: int32(min(t.maxLines, math.MaxInt32)), ellipsis: t.ellipsis, lineBox: t.lineBox}
	rewrapped := key != t.wrapped
	if rewrapped {
		t.wrapped = key
		t.glyphFace = nil // the lines' glyphs are of the lines before
		t.lines = textlayout.Wrap(key.value, face, gem.Pick(t.wrap, key.maxW, 0)*scale)
		if n := int(key.maxLines); n > 0 && len(t.lines) > n {
			t.lines = t.lines[:n]
			t.lines[n-1] = textlayout.Ellipsize(t.lines[n-1], face, key.maxW*scale, true)
		}
		if key.ellipsis {
			for i, line := range t.lines {
				t.lines[i] = textlayout.Ellipsize(line, face, key.maxW*scale, false)
			}
		}
		t.widths = t.widths[:0]
		var w float64
		for _, line := range t.lines {
			lw := textlayout.Width(line, face) / scale
			t.widths = append(t.widths, lw)
			w = max(w, lw)
		}
		m := face.Metrics()
		ascent, descent := m.HAscent/scale, m.HDescent/scale
		t.lead = 0
		if t.lineBox {
			// Half of what the line height leaves beside the glyphs goes
			// above them and half below, as CSS's half-leading does.
			t.lead = (t.spacing() - ascent - descent) / 2
		}
		t.natural = ggui.Sz(w, float64(len(t.lines)-1)*t.spacing()+ascent+descent+2*t.lead)
		t.ascent = t.lead + ascent
	}
	t.layoutBidi(env, rewrapped)
}

// Baseline implements Baseliner: the first line's baseline is its ascent
// below the top, where paintLines puts the line.
func (t *TextWidget) Baseline() (float64, bool) {
	if t.rich != nil {
		return t.rich.w.Baseline()
	}
	return t.ascent, t.resolved.Font != nil
}

// Paint implements Widget.
func (t *TextWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	// Text is half of what a screen reader reads, and none of it takes
	// input, so it goes in the semantics tree and never in the hit list,
	// which input scans backwards on every pointer event. Text a control
	// already painted as its own label is that control's name, not an
	// element of its own, so it stays quiet there.
	if t.rich != nil {
		t.rich.w.Paint(dst, r)
		return
	}
	t.paintLines(dst, r, true, func(op *text.DrawOptions, x, y float64) {
		op.GeoM.Translate(x*dst.Scale(), y*dst.Scale())
	})
}

// InspectFields implements inspect.Fielder.
func (t *TextWidget) InspectFields() []inspect.Field {
	return []inspect.Field{{Key: "Typography"}, {Key: "font size", Value: inspect.Num(t.resolved.Size) + " px", Number: true}, {Key: "line height", Value: inspect.Num(t.resolved.LineHeight), Number: true}, {Key: "color", Value: inspect.Color(t.resolved.Color)}, {Key: "wrap", Value: strconv.FormatBool(t.wrap)}, {Key: "lines", Value: strconv.Itoa(len(t.lines)), Number: true}}
}

// InspectLabel implements inspect.Labeler: the text it shows.
func (t *TextWidget) InspectLabel() string { return t.value }

// paintLines emits the semantics leaf, then draws every laid-out line with the
// shared style and line-advance math. place positions one line: it receives the
// freshly reset GeoM and the line's logical origin, so Paint translates and
// PaintRotated translates, rotates and translates back.
// paintLines draws the lines with place moving each to where it is drawn.
// upright says place only translates them, so that a line wholly outside
// the clip is skipped rather than shaped and drawn to be clipped: a long
// Text in a Scroll draws only what shows.
func (t *TextWidget) paintLines(dst *ggui.Canvas, r ggui.Rect, upright bool, place func(op *text.DrawOptions, x, y float64)) {
	if t.value != "" && !dst.Named(r) {
		dst.Leaf(r, ggui.Node{Role: roleOr(t.role, ggui.RoleText), Name: t.value})
	}
	if dst == nil || dst.Image == nil || dst.Image.Bounds().Empty() {
		return
	}
	scale := sanitizeScale(dst.Scale())
	if t.rich != nil {
		t.layoutLines(t.rich.maxW, t.rich.env, scale)
	}
	face := t.faceAt(scale)
	op := &t.drawOp
	*op = text.DrawOptions{}
	col := gem.Pick(t.style.Color != nil && !ggui.IsEnvColor(t.style.Color), t.style.Color, t.current().Color)
	op.ColorScale.ScaleWithColor(col)
	align := gem.Pick(t.alignSet, t.align, startAlign(t.dir()))
	for i, line := range t.lines {
		lw := t.widths[i]
		if align != 0 && scale != t.wrapped.scale {
			lw = textlayout.Width(line, face) / scale
		}
		x := r.Origin.X + (r.Size.W-lw)*align
		y := t.lineY(r, i)
		// A line's glyphs can reach past its box, by an accent, a tall
		// emoji or a long tail, by up to a line more.
		if upright && !dst.Draws(ggui.Rct(ggui.Pt(r.Origin.X, y), ggui.Sz(r.Size.W, t.spacing())), t.spacing()) {
			continue
		}
		if t.paintBidi(dst, i, r, align, col, place) {
			continue
		}
		op.GeoM.Reset()
		place(op, x, y)
		t.drawLine(dst.Image, i, line, face, op)
	}
}

// lineY is where line i starts down r.
func (t *TextWidget) lineY(r ggui.Rect, i int) float64 {
	return r.Origin.Y + t.lead + float64(i)*t.spacing()
}

// drawLine draws line i, from the glyphs it keeps when it can.
func (t *TextWidget) drawLine(dst *ggfx.Image, i int, line string, face text.Face, op *text.DrawOptions) {
	plain := fontsys.PlainFace(face, line)
	if plain == nil {
		fontsys.DrawText(dst, line, face, op)
		return
	}
	if t.glyphFace != plain || len(t.glyphs) != len(t.lines) {
		for j := range t.glyphs {
			t.glyphs[j] = t.glyphs[j][:0]
		}
		t.glyphs = slices.Grow(t.glyphs[:0], len(t.lines))[:len(t.lines)]
		for j := range t.glyphs {
			t.glyphs[j] = t.glyphs[j][:0]
		}
		t.glyphFace = plain
	}
	if len(t.glyphs[i]) == 0 && line != "" {
		t.glyphs[i] = text.AppendLazyGlyphs(t.glyphs[i][:0], line, plain, nil)
	}
	text.DrawLazyGlyphs(dst, t.glyphs[i], op)
}

// StyledWidget sets the text style its subtree inherits. Build one with
// Styled.
type StyledWidget struct {
	props property.Owner
	style ggui.TextStyle
	child ggui.Widget
}

// Styled gives every Text below child a new base style: what the setters
// here set, over what was inherited from above. A Text's own setters still
// win over it.
//
//	Styled(Column(Text("a"), Text("b"))).Color(muted).Size(12)
func Styled(child ggui.Widget) *StyledWidget { return &StyledWidget{child: child} }

// Style merges ts onto the style the subtree inherits.
func (s *StyledWidget) Style(ts ggui.TextStyle) *StyledWidget {
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
func (s *StyledWidget) Font(f *ggui.Font) *StyledWidget {
	defer property.Watch(&s.props, &s.style)()
	s.style.Font = f
	return s
}

// Weight sets the inherited font weight.
func (s *StyledWidget) Weight(w ggui.FontWeight) *StyledWidget {
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

// Tracking sets the inherited letter spacing, as a multiple of Size.
func (s *StyledWidget) Tracking(em float64) *StyledWidget {
	defer property.Watch(&s.props, &s.style)()
	s.style.Tracking = em
	return s
}

// Layout implements Widget.
func (s *StyledWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer s.props.Layout().EndLayout()
	return s.child.Layout(c, env.WithText(s.style))
}

// Baseline implements Baseliner: the child's.
func (s *StyledWidget) Baseline() (float64, bool) { return ggui.BaselineOf(s.child) }

// Paint implements Widget.
func (s *StyledWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(s.child, r) }
