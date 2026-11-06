package ggui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/paragraph"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/textedit"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/text"
	"github.com/ironpark/ggui/pkg/gem"
)

// engine is st as the text engine sets text in it: its font, size, weight
// and line height, beside the emoji font the active world chose. st must
// be resolved.
func (st TextStyle) engine() paragraph.Style {
	return paragraph.Style{Font: st.Font.engine(), Size: st.Size, Weight: fontsys.Weight(st.Weight), LineHeight: st.LineHeight, Emoji: activeEmoji()}
}

// RenderText draws s in st on the CPU, with no GPU, window or App, into an
// image just large enough to hold it: for icons, thumbnails, files and
// tests. Lines break at hard breaks and, when maxWidth is positive, wrap to
// maxWidth as Text wraps them. Fields st leaves unset are Text's defaults:
// the built-in font, DefaultTextSize, black, and 1.2 times the size between
// baselines. A pixel of the image is a pixel of st.Size; scale st.Size for a
// denser image.
//
// Glyphs come from st.Font and its fallbacks, as Text draws them, and emoji
// from the emoji font, in color. Text is shaped with go-text and drawn with
// go-text/render; emoji are drawn as Text draws them, from sbix, CBDT,
// COLRv0 or SVG glyphs, rasterized on the CPU.
func RenderText(s string, st TextStyle, maxWidth float64) *image.RGBA {
	st = st.resolved()
	return paragraph.Render(s, st.engine(), st.Color, maxWidth)
}

// textBidi is a Text's lines in visual order, for the lines that need
// reordering: every line in a right-to-left paragraph, and a line holding
// right-to-left text in a left-to-right one. Other lines draw as they are.
//
// A Text keeps one only once it is laid out under DirRTL or with a line
// that needs reordering; most never do.
type textBidi struct {
	dir     TextDirection
	lang    string
	key     wrapKey
	laid    bool
	lines   [][]paragraph.Piece     // nil for a line drawn as it is
	styles  [][]paragraph.LineStyle // each reordered line's styles
	pieceOp text.DrawOptions        // reused by paragraph.Draw, which would allocate one a paint
}

// layoutBidi orders the lines Layout just wrapped for the Env's direction.
// rewrapped says the lines changed since the last call.
func (t *TextWidget) layoutBidi(env Env, rewrapped bool) {
	dir := env.Direction()
	if t.bidi == nil {
		// Under DirLTR, lines with no right-to-left text draw as they
		// are, whatever the language: they need no textBidi until the
		// lines or the direction change.
		if dir != DirRTL && (!rewrapped || !slices.ContainsFunc(t.lines, paragraph.HasRTL)) {
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
	rtl := dir == DirRTL
	for _, line := range t.lines {
		if !paragraph.NeedsBidi(line, rtl) {
			b.lines, b.styles = append(b.lines, nil), append(b.styles, nil)
			continue
		}
		styles := t.lineStyles(line)
		b.lines, b.styles = append(b.lines, paragraph.LayoutLine(line, styles, rtl, lang)), append(b.styles, styles)
	}
}

// lineStyles is a line of a Text in its one style.
func (t *TextWidget) lineStyles(line string) []paragraph.LineStyle {
	return []paragraph.LineStyle{{Start: 0, End: len(line), Style: t.current().engine()}}
}

// paintBidi draws line i in visual order at y, aligned at align in r, and
// reports whether it did; a line that needs no reordering is left to the
// caller.
func (t *TextWidget) paintBidi(dst *Canvas, i int, r Rect, align, y float64, col color.Color, place func(op *text.DrawOptions, x, y float64)) bool {
	if t.bidi == nil || i >= len(t.bidi.lines) || t.bidi.lines[i] == nil {
		return false
	}
	ps, line := t.bidi.lines[i], t.lines[i]
	x := r.Origin.X + (r.Size.W-paragraph.Width(ps))*align
	baseline := t.faceAt(dst.Scale()).Metrics().HAscent
	paragraph.Draw(dst.Image, &t.bidi.pieceOp, line, ps, t.bidi.styles[i], dst.Scale(), baseline,
		func(int) color.Color { return col },
		func(op *text.DrawOptions) { place(op, x, y) })
	return true
}

// PaintRotated paints already-laid-out text clockwise around the center of r.
// It uses the regular glyph cache without allocating an intermediate image.
// Layout is unchanged; the caller reserves space for the rotated bounds.
func (t *TextWidget) PaintRotated(dst *Canvas, r Rect, radians float64) {
	if radians == 0 {
		t.Paint(dst, r)
		return
	}
	if math.IsNaN(radians) || math.IsInf(radians, 0) {
		return
	}
	center := r.Center()
	t.paintLines(dst, r, false, func(op *text.DrawOptions, x, y float64) {
		op.GeoM.Translate(dst.px(x-center.X), dst.px(y-center.Y))
		op.GeoM.Rotate(radians)
		op.GeoM.Translate(dst.px(center.X), dst.px(center.Y))
	})
}

// DrawTextCells draws s as DrawTextStyle does, but with each grapheme at
// the start of a cell of its own, cellW wide, from at: the layout of a
// terminal or any other grid of monospaced text. A grid's cells are best a
// whole number of device pixels, so that fills meet without seams, while a
// font's advance rarely is (SF Mono is 8.03 pixels wide at 13pt), and text
// drawn at its own advance would drift off the grid along a row. Each
// grapheme is placed on a whole device pixel, so the error stays under
// half a pixel instead of growing with the column. The run is shaped
// once, so it costs what DrawTextStyle does rather than a draw per cell.
func (c *Canvas) DrawTextCells(s string, st TextStyle, at Point, cellW float64) {
	if c == nil || c.Image == nil || c.Image.Bounds().Empty() || s == "" {
		return
	}
	st = st.resolved()
	// A terminal draws a run per row and colour every frame, each a string
	// of its own: the run's graphemes, glyphs, shifts and images are kept
	// in buffers it reuses.
	buf := theCellsBufPool.Get()
	defer func() {
		clear(buf.glyphs)
		clear(buf.entries)
		theCellsBufPool.Put(buf)
	}()
	buf.bounds = textedit.AppendBounds(buf.bounds[:0], s)
	face := st.Font.face(c.px(st.Size), st.Weight)
	face = fontsys.PlainFace(face, s)
	if face == nil {
		// An emoji is drawn from a font of its own, with its own
		// metrics: one grapheme at a time keeps it in its cell.
		x := at.X
		for k := 0; k+1 < len(buf.bounds); k++ {
			c.DrawTextStyle(s[buf.bounds[k]:buf.bounds[k+1]], st, Pt(x, at.Y))
			x += cellW
		}
		return
	}

	var tint, colored ggfx.ColorScale
	tint.ScaleWithColor(st.Color)
	colored.ScaleAlpha(tint.A())
	x0, y0, cell := c.px(at.X), c.px(at.Y), c.px(cellW)

	buf.glyphs = text.AppendLazyGlyphs(buf.glyphs[:0], s, face, nil)
	buf.shifts = cellShifts(buf.shifts, buf.bounds, buf.glyphs, x0, cell)
	// Realize every image before drawing any, as text.Draw does, so the
	// atlas is written in one batch, then draw those on one atlas page as
	// one.
	buf.entries = buf.entries[:0]
	for i, g := range buf.glyphs {
		if g.ImageBounds.Empty() {
			continue
		}
		if img := g.Image(); img != nil {
			e := ggfx.ImageDraw{Image: img, X: x0 + (float64(g.ImageBounds.Min.X) + buf.shifts[i]), Y: y0 + float64(g.ImageBounds.Min.Y), ColorScale: tint}
			if g.Colored() {
				e.ColorScale = colored
			}
			buf.entries = append(buf.entries, e)
		}
	}
	c.Image.DrawImages(buf.entries, nil)
}

// cellsBuf holds what DrawTextCells works out for a run between runs.
type cellsBuf struct {
	bounds  []int
	glyphs  []text.LazyGlyph
	shifts  []float64
	entries []ggfx.ImageDraw
}

var theCellsBufPool = gem.Pool[*cellsBuf]{
	New: func() *cellsBuf { return &cellsBuf{} },
}

// cellShifts is how far each glyph moves from where its run shaped it, in
// device pixels, to start its grapheme's cell: the kth grapheme, from
// bounds[k] to bounds[k+1] of the run as textedit.Bounds gives them, has
// its first glyph at the whole pixel nearest x0+k·cell, and the rest of a
// grapheme, its marks or the parts of a ligature, move with it. It is kept
// in dst's storage when that has room.
func cellShifts(dst []float64, bounds []int, glyphs []text.LazyGlyph, x0, cell float64) []float64 {
	shifts := slices.Grow(dst[:0], len(glyphs))[:len(glyphs)]
	k, shift := -1, 0.0
	for i, g := range glyphs {
		if g.StartIndexInBytes >= bounds[k+1] {
			for k+1 < len(bounds)-1 && g.StartIndexInBytes >= bounds[k+1] {
				k++
			}
			shift = math.Round(x0+float64(k)*cell) - x0 - g.OriginX
		}
		shifts[i] = shift
	}
	return shifts
}

// TextWidget draws text, wrapping it to the width it is given. Build one
// with Text. Its style is resolved at layout: the Env's inherited TextStyle,
// then the widget's own setters on top, then built-in defaults for whatever
// is still unset.
type TextWidget struct {
	props     property.Owner
	value     string
	content   property.Value[string]
	formatted bool
	style     TextStyle
	wrap      bool
	ellipsis  bool
	maxLines  int
	align     float64
	alignSet  bool
	role      Role
	opts      *textOpts // set by Textf and StyleKey only
	cache     *CachedWidget

	// Layout caches the wrapped lines, their widths and the size they add up
	// to, and re-measures only when the text, the face or the width it must
	// fit in changes.
	resolved TextStyle
	lines    []string
	// glyphs are each line's glyphs laid out in glyphFace, the face they
	// were last drawn in, kept until the lines are wrapped again: a frame
	// that draws them as they were, as a scroll's does, lays none out.
	glyphs    [][]text.LazyGlyph
	glyphFace text.Face
	widths    []float64
	natural   Size
	ascent    float64 // the first baseline, below the top
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
	styleKey      EnvKey[TextStyle]
	styleFallback TextStyle
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
	env  Env
}

// wrapKey is the input textlayout.Wrap was last run with.
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
	o := t.options()
	defer property.Watch(&t.props, &o.styleKey)()
	defer property.Watch(&t.props, &o.styleFallback)()
	o.styleKey = key
	o.styleFallback = TextStyle{}
	if len(fallback) > 0 {
		o.styleFallback = fallback[0]
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
	o := t.options()
	o.format = format
	o.formatArgs = append([]any(nil), args...)
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
func (t *TextWidget) BindContent(r Readable[string]) *TextWidget {
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
func (t *TextWidget) current() TextStyle {
	if t.resolved.Font == nil {
		return t.style.resolved()
	}
	return t.resolved
}

// dir is the direction the lines were last ordered for: DirLTR, unless
// they were laid out in another.
func (t *TextWidget) dir() TextDirection {
	if t.bidi == nil {
		return DirLTR
	}
	return t.bidi.dir
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
	var opts textOpts
	if t.opts != nil {
		opts = *t.opts
	}
	if t.formatted {
		t.value = formatValues(opts.format, opts.formatArgs)
	} else {
		t.value = t.content.Get()
	}
	t.cache, _ = env.Get(cacheOwner)
	style, ok := env.Get(opts.styleKey)
	if !ok {
		style = opts.styleFallback
	}
	t.resolved = env.ResolveText(style.Merge(t.style))
	if t.rich != nil {
		// A Selectable Text lays out and paints as its RichText. Only
		// PaintRotated draws the lines, so they wait for it.
		t.rich.maxW, t.rich.env = c.MaxW, env
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
	rewrapped := key != t.wrapped
	if rewrapped {
		t.wrapped = key
		t.glyphFace = nil // the lines' glyphs are of the lines before
		t.lines = textlayout.Wrap(key.value, face, pick(t.wrap, key.maxW, 0))
		if n := key.maxLines; n > 0 && len(t.lines) > n {
			t.lines = t.lines[:n]
			t.lines[n-1] = textlayout.Ellipsize(t.lines[n-1], face, key.maxW, true)
		}
		if key.ellipsis {
			for i, line := range t.lines {
				t.lines[i] = textlayout.Ellipsize(line, face, key.maxW, false)
			}
		}
		t.widths = t.widths[:0]
		var w float64
		for _, line := range t.lines {
			lw := textlayout.Width(line, face)
			t.widths = append(t.widths, lw)
			w = max(w, lw)
		}
		m := face.Metrics()
		t.natural = Sz(w, float64(len(t.lines)-1)*t.spacing()+m.HAscent+m.HDescent)
		t.ascent = m.HAscent
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
func (t *TextWidget) Paint(dst *Canvas, r Rect) {
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
		t.layoutLines(t.rich.maxW, t.rich.env)
	}
	face := t.faceAt(dst.Scale())
	op := &t.drawOp
	*op = text.DrawOptions{}
	col := pick(t.style.Color != nil && !IsEnvColor(t.style.Color), t.style.Color, t.current().Color)
	op.ColorScale.ScaleWithColor(col)
	align := pick(t.alignSet, t.align, startAlign(t.dir()))
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
		t.drawLine(dst.Image, i, line, face, op)
	}
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
