package ggui

import (
	"image/color"
	"slices"
	"strings"

	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/text"
	"github.com/ironpark/ggui/internal/textedit"
)

// TextSpan is one stretch of a RichText in a style of its own. It is a
// description value: build one with Span, and each setter returns a
// changed copy.
type TextSpan struct {
	text      string
	style     TextStyle
	underline bool
	onTap     func()
	name      string
}

// Span is s in the paragraph's style, until a setter changes it.
func Span(s string) TextSpan { return TextSpan{text: s} }

// Text returns the span's text.
func (s TextSpan) Text() string { return s.text }

// Style merges ts onto the span's style.
func (s TextSpan) Style(ts TextStyle) TextSpan { s.style = s.style.Merge(ts); return s }

// Color sets the span's color; nil inherits the paragraph's.
func (s TextSpan) Color(c color.Color) TextSpan { s.style.Color = c; return s }

// Font sets the span's face; nil inherits.
func (s TextSpan) Font(f *Font) TextSpan { s.style.Font = f; return s }

// Weight sets the span's font weight; zero inherits.
func (s TextSpan) Weight(w FontWeight) TextSpan { s.style.Weight = w; return s }

// Size sets the span's font size in pixels; zero inherits.
func (s TextSpan) Size(px float64) TextSpan { s.style.Size = px; return s }

// Underline draws a line under the span.
func (s TextSpan) Underline() TextSpan { s.underline = true; return s }

// OnTap makes the span a link: fn runs when it is clicked, or pressed
// with Enter or Space once Tab has reached it, and the pointer turns into
// a hand over it. A screen reader reads it as a link named by its text.
func (s TextSpan) OnTap(fn func()) TextSpan { s.onTap = fn; return s }

// Name sets the accessible name of a link span; its text by default.
func (s TextSpan) Name(name string) TextSpan { s.name = name; return s }

// RichTextWidget draws a paragraph of styled spans. Build one with
// RichText.
type RichTextWidget struct {
	props      property.Owner
	spans      property.Value[[]TextSpan]
	style      TextStyle
	wrap       bool
	ellipsis   bool
	maxLines   int
	align      float64
	alignSet   bool
	role       Role
	selectable bool

	// Layout keeps the spans it read, the paragraph they made and what it
	// was laid out for.
	cur       []TextSpan
	para      paragraph
	laid      richKey
	runKeys   []runKey
	keyBuf    []runKey // the other of runKeys' two arrays, which layouts take in turn
	selection color.Color
	links     []*spanLink

	// The selection, for a Selectable paragraph.
	ed      textedit.Editor
	focused bool
	area    *SelectionAreaWidget // the SelectionArea it is in, if any
	clicks  clickSelector
	rect    Rect

	pieceOp text.DrawOptions // reused by drawPieces, which would allocate one a piece
}

// richKey is what the paragraph was last laid out for, beside its runs.
type richKey struct {
	generation uint64
	text       string
	maxW       float64
	wrap       bool
	ellipsis   bool
	maxLines   int
	dir        TextDirection
	lang       string
	base       runKey
}

// runKey is what of a run's style changes its measure: not its color.
type runKey struct {
	start, end int
	font       *Font
	size       float64
	lineHeight float64
	weight     FontWeight
}

// RichText draws spans as one paragraph in the inherited style, each span
// drawn in its own on top: the paragraph wraps across span boundaries
// and lines its spans up on a shared baseline.
//
//	ggui.RichText(
//		ggui.Span("Read the "),
//		ggui.Span("guide").Color(link).Underline().OnTap(openGuide),
//		ggui.Span(" first.").Weight(ggui.WeightBold),
//	)
func RichText(spans ...TextSpan) *RichTextWidget {
	t := &RichTextWidget{wrap: true}
	t.spans.Set(slices.Clone(spans))
	return t
}

// Spans replaces the spans and any reader.
func (t *RichTextWidget) Spans(spans ...TextSpan) *RichTextWidget {
	if t.spans.Set(slices.Clone(spans)) {
		t.props.Changed()
	}
	return t
}

// BindSpans follows r during layout without owning a subscription.
func (t *RichTextWidget) BindSpans(r Readable[[]TextSpan]) *RichTextWidget {
	if t.spans.Bind(r, "BindSpans") {
		t.props.Changed()
	}
	return t
}

// Style merges ts onto the style every span starts from.
func (t *RichTextWidget) Style(ts TextStyle) *RichTextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style = t.style.Merge(ts)
	return t
}

// Color sets the paragraph's text color; nil inherits.
func (t *RichTextWidget) Color(c color.Color) *RichTextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style.Color = c
	return t
}

// Font sets the paragraph's face; nil inherits.
func (t *RichTextWidget) Font(f *Font) *RichTextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style.Font = f
	return t
}

// Weight sets the paragraph's font weight; zero inherits.
func (t *RichTextWidget) Weight(w FontWeight) *RichTextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style.Weight = w
	return t
}

// Size sets the paragraph's font size in pixels.
func (t *RichTextWidget) Size(px float64) *RichTextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style.Size = px
	return t
}

// LineHeight sets the distance between baselines as a multiple of Size.
func (t *RichTextWidget) LineHeight(mult float64) *RichTextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style.LineHeight = mult
	return t
}

// NoWrap keeps the text on one line per hard line break, however wide.
func (t *RichTextWidget) NoWrap() *RichTextWidget {
	defer property.Watch(&t.props, &t.wrap)()
	t.wrap = false
	return t
}

// Ellipsis ends a line too wide for the widget with "…", as Text's does.
func (t *RichTextWidget) Ellipsis() *RichTextWidget {
	defer property.Watch(&t.props, &t.ellipsis)()
	t.ellipsis = true
	return t
}

// MaxLines shows at most n lines, ending the last with "…" when the text
// goes on past it; 0 shows them all.
func (t *RichTextWidget) MaxLines(n int) *RichTextWidget {
	defer property.Watch(&t.props, &t.maxLines)()
	t.maxLines = max(n, 0)
	return t
}

// Align places each line within the widget's width by fraction: 0 is left,
// 0.5 centered, 1 right. Unset, lines start at the reading direction's
// start: the left, or the right under DirRTL.
func (t *RichTextWidget) Align(x float64) *RichTextWidget {
	defer property.Watch(&t.props, &t.align)()
	defer property.Watch(&t.props, &t.alignSet)()
	t.align, t.alignSet = x, true
	return t
}

// Role sets the accessibility role; the default is RoleText.
func (t *RichTextWidget) Role(role Role) *RichTextWidget {
	defer property.Watch(&t.props, &t.role)()
	t.role = role
	return t
}

// Text returns the paragraph's text as last laid out: every span's, one
// after another.
func (t *RichTextWidget) Text() string { return t.para.text }

// alignment is the fraction lines are placed at.
func (t *RichTextWidget) alignment() float64 {
	return pick(t.alignSet, t.align, startAlign(t.para.dir))
}

// Layout implements Widget.
func (t *RichTextWidget) Layout(c Constraints, env Env) Size {
	defer t.props.Layout().EndLayout()
	t.area = nil
	if t.selectable {
		t.area, _ = env.Get(selectionAreaKey)
	}
	// The links outlive a layout that changed no span, so that one keeps
	// its focus and hover; a new tap func is picked up either way.
	spans := t.spans.Get()
	same := slices.EqualFunc(spans, t.cur, sameSpan)
	if !same {
		t.links = nil
	}
	// A paragraph is laid out far more often than its spans change, so
	// what it keeps is kept in arrays it reuses. The spans are copied in
	// either way, for their tap funcs.
	t.cur = append(t.cur[:0], spans...)
	base := env.ResolveText(t.style)
	p := &t.para
	p.runs = p.runs[:0]
	// The keys go in the array runKeys is not in, which they are compared
	// with below.
	keys := t.keyBuf[:0]
	end := 0
	for i, sp := range t.cur {
		if sp.text == "" {
			continue
		}
		st := env.ResolveText(t.style.Merge(sp.style))
		start := end
		end += len(sp.text)
		p.runs = append(p.runs, paraRun{start: start, end: end, style: st, span: i})
		keys = append(keys, runKey{start, end, st.Font, st.Size, st.LineHeight, st.Weight})
	}
	// The same spans hold the same text.
	text := p.text
	if !same || len(text) != end {
		var sb strings.Builder
		sb.Grow(end)
		for _, sp := range t.cur {
			sb.WriteString(sp.text)
		}
		text = sb.String()
	}
	p.base = base
	p.dir, p.lang = env.Direction(), env.Language()
	t.selection = env.EditorStyle().Selection
	// As Text's: the width only matters when something wraps or cuts to it.
	fits := t.wrap || t.ellipsis || t.maxLines > 0
	key := richKey{generation: fontGen(), text: text, maxW: pick(fits, c.MaxW, 0), wrap: t.wrap, ellipsis: t.ellipsis, maxLines: t.maxLines,
		dir: p.dir, lang: p.lang, base: runKey{font: base.Font, size: base.Size, lineHeight: base.LineHeight, weight: base.Weight}}
	if key != t.laid || !slices.Equal(keys, t.runKeys) || p.lines == nil {
		t.laid = key
		p.text = key.text
		p.layout(key.maxW, t.wrap, t.maxLines, t.ellipsis)
	}
	t.runKeys, t.keyBuf = keys, t.runKeys
	if t.ed.Text != p.text {
		t.ed.SetText(p.text)
	}
	if t.links == nil {
		for i, sp := range t.cur {
			if sp.onTap != nil {
				l := &spanLink{w: t, span: i}
				l.Role = RoleLink
				l.SetName(pick(sp.name != "", sp.name, sp.text))
				t.links = append(t.links, l)
			}
		}
	}
	return c.Constrain(p.size)
}

// sameSpan compares spans by what they show, and their tap by presence.
func sameSpan(a, b TextSpan) bool {
	return a.text == b.text && sameAny(a.style, b.style) && a.underline == b.underline && a.name == b.name && (a.onTap == nil) == (b.onTap == nil)
}

// Baseline implements Baseliner: the first line's baseline.
func (t *RichTextWidget) Baseline() (float64, bool) {
	if len(t.para.lines) == 0 {
		return 0, false
	}
	return t.para.lines[0].baseline, true
}

// roleOr returns role, or fallback when it is unset.
func roleOr(role, fallback Role) Role { return pick(role != "", role, fallback) }

// Paint implements Widget.
func (t *RichTextWidget) Paint(dst *Canvas, r Rect) {
	t.rect = r
	p := &t.para
	align := t.alignment()
	described := func(dst *Canvas) {
		for _, l := range t.links {
			l.paint(dst, r, align)
		}
	}
	switch {
	case t.selectable:
		// The paragraph's regions go first, so that a link's, registered
		// after them, are on top and take the clicks and the cursor.
		dst.HitPointer(r, t)
		dst.HitKey(r, t)
		if t.area != nil {
			t.area.painted(t)
		}
		dst.HitCursor(r, CursorShapeText)
		dst.DescribeNode(r, t, described)
	case p.text != "" && !dst.named(r):
		dst.Node(r, Node{Role: roleOr(t.role, RoleText), Name: p.text}, described)
	default:
		described(dst)
	}
	if dst == nil || dst.Image == nil || dst.Image.Bounds().Empty() {
		return
	}
	// In a SelectionArea the selection shows wherever the focus is: one
	// selection runs through many texts, and only one has the focus.
	if t.selectable && (t.focused || t.area != nil) && t.ed.HasSelection() {
		lo, hi := t.ed.Selection()
		p.rects(lo, hi, r.Size.W, align, func(b Rect) {
			dst.FillRect(Rct(r.Origin.Add(b.Origin), b.Size), t.selection)
		})
	}
	scale := dst.Scale()
	for i := range p.lines {
		ln := &p.lines[i]
		// A line wholly outside the clip is not shaped and drawn to be
		// clipped, with a line's height to spare for what reaches past it.
		top, h := ln.baseline-ln.ascent, ln.ascent+ln.descent
		if !dst.visiblePaintBounds(Rct(Pt(r.Origin.X, r.Origin.Y+top), Sz(r.Size.W, h)), h) {
			continue
		}
		x := r.Origin.X + p.lineX(i, r.Size.W, align)
		col := func(style int) color.Color { return p.style(ln.styles[style].run).Color }
		drawPieces(dst.Image, &t.pieceOp, ln.s, ln.pieces, ln.styles, scale, dst.px(ln.baseline), col, func(op *text.DrawOptions) {
			op.GeoM.Translate(dst.px(x), dst.px(r.Origin.Y))
		})
		for _, pc := range ln.pieces {
			st := ln.styles[pc.style]
			if st.run < 0 || !t.cur[p.runs[st.run].span].underline {
				continue
			}
			rs := p.style(st.run)
			thick := max(1/scale, rs.Size/14)
			y := r.Origin.Y + ln.baseline + max(1, rs.Size*0.08)
			dst.FillRect(Rct(Pt(x+pc.x, y), Sz(pc.w, thick)), rs.Color)
		}
	}
}

// spanLink is the control a tappable span is: every box the span covers
// takes the pointer, the first one the focus.
type spanLink struct {
	Interactive
	w    *RichTextWidget
	span int
}

// tap runs the span's OnTap.
func (l *spanLink) tap() {
	if l.span < len(l.w.cur) && l.w.cur[l.span].onTap != nil {
		l.w.cur[l.span].onTap()
	}
}

// HandlePointer implements PointerHandler.
func (l *spanLink) HandlePointer(ev PointerEvent) bool { return l.Pointer(ev, l.tap) }

// HandleKey implements KeyHandler.
func (l *spanLink) HandleKey(ev KeyEvent) { l.Keyboard(ev, l.tap) }

// Describe implements Describer: a link that can be pressed and focused.
func (l *spanLink) Describe() Node {
	return Node{Role: RoleLink, Name: l.SemanticName(), Actions: ActionPress | ActionFocus}
}

// paint registers the span's boxes in a paragraph at r.
func (l *spanLink) paint(dst *Canvas, r Rect, align float64) {
	p := &l.w.para
	var boxes []Rect
	for _, run := range p.runs {
		if run.span != l.span {
			continue
		}
		p.rects(run.start, run.end, r.Size.W, align, func(b Rect) {
			boxes = append(boxes, Rct(r.Origin.Add(b.Origin), b.Size))
		})
	}
	if len(boxes) == 0 {
		return
	}
	all := boxes[0]
	for _, b := range boxes[1:] {
		all = all.Union(b)
	}
	dst.Describe(all, l)
	for i, b := range boxes {
		dst.HitPointer(b, l)
		if i == 0 {
			dst.HitKey(b, l)
		}
		dst.HitCursor(b, CursorShapePointer)
	}
	if l.Focused && l.FocusVisible && dst != nil && dst.Image != nil {
		col := l.w.para.base.Color
		for _, run := range p.runs {
			if run.span == l.span {
				col = run.style.Color
				break
			}
		}
		for _, b := range boxes {
			dst.StrokeRoundRect(Rct(b.Origin.Add(Pt(-2, -1)), Sz(b.Size.W+4, b.Size.H+2)), 2, 1, col)
		}
	}
}
