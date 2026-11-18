package widget

import (
	"image/color"
	"slices"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/paragraph"
	"github.com/ironpark/ggui/internal/textedit"
	"github.com/ironpark/ggui/internal/third_party/text"
	"github.com/ironpark/ggui/pkg/gem"
	"github.com/ironpark/ggui/property"
)

// TextSpan is one stretch of a RichText in a style of its own. It is a
// description value: build one with Span, and each setter returns a
// changed copy.
type TextSpan struct {
	text       string
	style      ggui.TextStyle
	underline  bool
	strike     bool
	background color.Color // behind the span's text, nil for none
	onTap      func()
	name       string
}

// Span is s in the paragraph's style, until a setter changes it.
func Span(s string) TextSpan { return TextSpan{text: s} }

// Text returns the span's text.
func (s TextSpan) Text() string { return s.text }

// Style merges ts onto the span's style.
func (s TextSpan) Style(ts ggui.TextStyle) TextSpan { s.style = s.style.Merge(ts); return s }

// Color sets the span's color; nil inherits the paragraph's.
func (s TextSpan) Color(c color.Color) TextSpan { s.style.Color = c; return s }

// Font sets the span's face; nil inherits.
func (s TextSpan) Font(f *ggui.Font) TextSpan { s.style.Font = f; return s }

// Italic draws the span in its font's italic.
func (s TextSpan) Italic() TextSpan { s.style.Italic = true; return s }

// Weight sets the span's font weight; zero inherits.
func (s TextSpan) Weight(w ggui.FontWeight) TextSpan { s.style.Weight = w; return s }

// Size sets the span's font size in pixels; zero inherits.
func (s TextSpan) Size(px float64) TextSpan { s.style.Size = px; return s }

// Underline draws a line under the span.
func (s TextSpan) Underline() TextSpan { s.underline = true; return s }

// Strike draws a line through the span, as struck out.
func (s TextSpan) Strike() TextSpan { s.strike = true; return s }

// Background fills the line's height behind the span with c, as inline
// code is set apart; nil fills nothing.
func (s TextSpan) Background(c color.Color) TextSpan { s.background = c; return s }

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
	style      ggui.TextStyle
	wrap       bool
	ellipsis   bool
	maxLines   int
	align      float64
	alignSet   bool
	role       ggui.Role
	selectable bool

	// Layout keeps the spans it read, the paragraph they made and what it
	// was laid out for.
	cur       []TextSpan
	para      paragraph.Paragraph
	styles    []ggui.TextStyle   // each of para.Runs' style, resolved, for what the engine does not take
	base      ggui.TextStyle     // the paragraph's style, resolved
	dir       ggui.TextDirection // what para was laid out in
	laid      richKey
	runKeys   []runKey
	keyBuf    []runKey // the other of runKeys' two arrays, which layouts take in turn
	selection color.Color
	links     []*spanLink

	// The selection, for a Selectable paragraph.
	ed      textedit.Editor
	focused bool
	area    *SelectionAreaWidget // the SelectionArea it is in, if any
	clicks  textedit.Clicks
	rect    ggui.Rect

	pieceOp text.DrawOptions // reused by paragraph.Draw, which would allocate one a piece
}

// richKey is what the paragraph was last laid out for, beside its runs.
type richKey struct {
	generation uint64
	text       string
	maxW       float64
	wrap       bool
	ellipsis   bool
	maxLines   int
	dir        ggui.TextDirection
	lang       string
	base       runKey
	lineBox    bool
}

// runKey is what of a run's style changes its measure: not its color.
type runKey struct {
	start, end int
	font       *ggui.Font
	size       float64
	lineHeight float64
	weight     ggui.FontWeight
	tracking   float64
}

// RichText draws spans as one paragraph in the inherited style, each span
// drawn in its own on top: the paragraph wraps across span boundaries
// and lines its spans up on a shared baseline.
//
//	widget.RichText(
//		widget.Span("Read the "),
//		widget.Span("guide").Color(link).Underline().OnTap(openGuide),
//		widget.Span(" first.").Weight(ggui.WeightBold),
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
func (t *RichTextWidget) BindSpans(r ggui.Readable[[]TextSpan]) *RichTextWidget {
	if t.spans.Bind(r, "BindSpans") {
		t.props.Changed()
	}
	return t
}

// Style merges ts onto the style every span starts from.
func (t *RichTextWidget) Style(ts ggui.TextStyle) *RichTextWidget {
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
func (t *RichTextWidget) Font(f *ggui.Font) *RichTextWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style.Font = f
	return t
}

// Weight sets the paragraph's font weight; zero inherits.
func (t *RichTextWidget) Weight(w ggui.FontWeight) *RichTextWidget {
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

// LineHeight sets the height of a line as a multiple of Size, as
// TextWidget.LineHeight does.
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
func (t *RichTextWidget) Role(role ggui.Role) *RichTextWidget {
	defer property.Watch(&t.props, &t.role)()
	t.role = role
	return t
}

// Text returns the paragraph's text as last laid out: every span's, one
// after another.
func (t *RichTextWidget) Text() string { return t.para.Text }

// alignment is the fraction lines are placed at.
func (t *RichTextWidget) alignment() float64 {
	return gem.Pick(t.alignSet, t.align, startAlign(t.dir))
}

// Layout implements Widget.
func (t *RichTextWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
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
	// A span's background, resolved here for the paint, which has no Env.
	for i := range t.cur {
		if t.cur[i].background != nil {
			t.cur[i].background = ggui.ResolveColor(t.cur[i].background, env)
		}
	}
	base := env.ResolveText(t.style)
	p := &t.para
	p.Runs, t.styles = p.Runs[:0], t.styles[:0]
	// The keys go in the array runKeys is not in, which they are compared
	// with below.
	keys := t.keyBuf[:0]
	end := 0
	lineBox := lineHeightSet(env, t.style)
	for i, sp := range t.cur {
		if sp.text == "" {
			continue
		}
		lineBox = lineBox || sp.style.LineHeight != 0
		st := env.ResolveText(t.style.Merge(sp.style))
		start := end
		end += len(sp.text)
		run, _ := textEngineStyle(st)
		p.Runs = append(p.Runs, paragraph.Run{Start: start, End: end, Style: run, Span: i})
		t.styles = append(t.styles, st)
		keys = append(keys, runKey{start, end, st.Font, st.Size, st.LineHeight, st.Weight, st.Tracking})
	}
	// The same spans hold the same text.
	text := p.Text
	if !same || len(text) != end {
		var sb strings.Builder
		sb.Grow(end)
		for _, sp := range t.cur {
			sb.WriteString(sp.text)
		}
		text = sb.String()
	}
	var gen uint64
	p.Base, gen = textEngineStyle(base)
	t.base = base
	t.dir, p.Lang = env.Direction(), env.Language()
	p.RTL = t.dir == ggui.DirRTL
	t.selection = env.EditorStyle().Selection
	// As Text's: the width only matters when something wraps or cuts to it.
	fits := t.wrap || t.ellipsis || t.maxLines > 0
	key := richKey{generation: gen, text: text, maxW: gem.Pick(fits, c.MaxW, 0), wrap: t.wrap, ellipsis: t.ellipsis, maxLines: t.maxLines,
		dir: t.dir, lang: p.Lang, base: runKey{font: base.Font, size: base.Size, lineHeight: base.LineHeight, weight: base.Weight, tracking: base.Tracking}, lineBox: lineBox}
	p.LineBox = lineBox
	if key != t.laid || !slices.Equal(keys, t.runKeys) || p.Lines == nil {
		t.laid = key
		p.Text = key.text
		p.Layout(key.maxW, t.wrap, t.maxLines, t.ellipsis)
	}
	t.runKeys, t.keyBuf = keys, t.runKeys
	if t.ed.Text != p.Text {
		t.ed.SetText(p.Text)
	}
	if t.links == nil {
		for i, sp := range t.cur {
			if sp.onTap != nil {
				l := &spanLink{w: t, span: i}
				l.Role = ggui.RoleLink
				l.SetName(gem.Pick(sp.name != "", sp.name, sp.text))
				t.links = append(t.links, l)
			}
		}
	}
	return c.Constrain(p.Size)
}

// runStyle returns run i's resolved style, or the paragraph's for -1.
func (t *RichTextWidget) runStyle(i int) ggui.TextStyle {
	if i < 0 {
		return t.base
	}
	return t.styles[i]
}

// sameSpan compares spans by what they show, and their tap by presence.
func sameSpan(a, b TextSpan) bool {
	return a.text == b.text && gem.Same(a.style, b.style) && a.underline == b.underline && a.strike == b.strike &&
		gem.Same(a.background, b.background) && a.name == b.name && (a.onTap == nil) == (b.onTap == nil)
}

// Baseline implements Baseliner: the first line's baseline.
func (t *RichTextWidget) Baseline() (float64, bool) {
	if len(t.para.Lines) == 0 {
		return 0, false
	}
	return t.para.Lines[0].Baseline, true
}

// roleOr returns role, or fallback when it is unset.
func roleOr(role, fallback ggui.Role) ggui.Role { return gem.Pick(role != "", role, fallback) }

// Paint implements Widget.
func (t *RichTextWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t.rect = r
	p := &t.para
	align := t.alignment()
	described := func(dst *ggui.Canvas) {
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
		if t.area != nil && dst.TakesInput() {
			t.area.painted(t)
		}
		dst.HitCursor(r, ggui.CursorShapeText)
		dst.DescribeNode(r, t, described)
	case p.Text != "" && !dst.Named(r):
		dst.Node(r, ggui.Node{Role: roleOr(t.role, ggui.RoleText), Name: p.Text}, described)
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
		p.Rects(lo, hi, r.Size.W, align, func(b ggui.Rect) {
			dst.FillRect(ggui.Rct(r.Origin.Add(b.Origin), b.Size), t.selection)
		})
	}
	scale := dst.Scale()
	for i := range p.Lines {
		ln := &p.Lines[i]
		// A line wholly outside the clip is not shaped and drawn to be
		// clipped, with a line's height to spare for what reaches past it.
		top, h := ln.Baseline-ln.Ascent, ln.Ascent+ln.Descent
		if !dst.Draws(ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+top), ggui.Sz(r.Size.W, h)), h) {
			continue
		}
		x := r.Origin.X + p.LineX(i, r.Size.W, align)
		for _, pc := range ln.Pieces {
			st := ln.Styles[pc.Style]
			if st.Run < 0 || t.cur[p.Runs[st.Run].Span].background == nil {
				continue
			}
			rs := t.runStyle(st.Run)
			// The run's own ascent and descent, a little past them.
			pad := rs.Size * 0.12
			bg := ggui.Rct(ggui.Pt(x+pc.X-pad, r.Origin.Y+ln.Baseline-rs.Size*0.92), ggui.Sz(pc.W+2*pad, rs.Size*1.18))
			dst.FillRoundRect(bg, rs.Size*0.2, t.cur[p.Runs[st.Run].Span].background)
		}
		col := func(style int) color.Color { return t.runStyle(ln.Styles[style].Run).Color }
		paragraph.Draw(dst.Image, &t.pieceOp, ln.S, ln.Pieces, ln.Styles, scale, ln.Baseline*scale, col, func(op *text.DrawOptions) {
			op.GeoM.Translate(x*scale, r.Origin.Y*scale)
		})
		for _, pc := range ln.Pieces {
			st := ln.Styles[pc.Style]
			if st.Run < 0 {
				continue
			}
			span := t.cur[p.Runs[st.Run].Span]
			rs := t.runStyle(st.Run)
			thick := max(1/scale, rs.Size/14)
			if span.underline {
				y := r.Origin.Y + ln.Baseline + max(1, rs.Size*0.08)
				dst.FillRect(ggui.Rct(ggui.Pt(x+pc.X, y), ggui.Sz(pc.W, thick)), rs.Color)
			}
			if span.strike {
				// Through the middle of the lower case letters.
				y := r.Origin.Y + ln.Baseline - rs.Size*0.3
				dst.FillRect(ggui.Rct(ggui.Pt(x+pc.X, y), ggui.Sz(pc.W, thick)), rs.Color)
			}
		}
	}
}

// spanLink is the control a tappable span is: every box the span covers
// takes the pointer, the first one the focus.
type spanLink struct {
	ggui.Interactive
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
func (l *spanLink) HandlePointer(ev ggui.PointerEvent) bool { return l.Pointer(ev, l.tap) }

// HandleKey implements KeyHandler.
func (l *spanLink) HandleKey(ev ggui.KeyEvent) { l.Keyboard(ev, l.tap) }

// Describe implements Describer: a link that can be pressed and focused.
func (l *spanLink) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleLink, Name: l.SemanticName(), Actions: ggui.ActionPress | ggui.ActionFocus}
}

// paint registers the span's boxes in a paragraph at r.
func (l *spanLink) paint(dst *ggui.Canvas, r ggui.Rect, align float64) {
	p := &l.w.para
	var boxes []ggui.Rect
	for _, run := range p.Runs {
		if run.Span != l.span {
			continue
		}
		p.Rects(run.Start, run.End, r.Size.W, align, func(b ggui.Rect) {
			boxes = append(boxes, ggui.Rct(r.Origin.Add(b.Origin), b.Size))
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
		dst.HitCursor(b, ggui.CursorShapePointer)
	}
	if l.Focused && l.FocusVisible && dst != nil && dst.Image != nil {
		col := l.w.base.Color
		for i, run := range p.Runs {
			if run.Span == l.span {
				col = l.w.styles[i].Color
				break
			}
		}
		for _, b := range boxes {
			dst.StrokeRoundRect(ggui.Rct(b.Origin.Add(ggui.Pt(-2, -1)), ggui.Sz(b.Size.W+4, b.Size.H+2)), 2, 1, col)
		}
	}
}

// SelectionAreaWidget lets a drag select across the Selectable texts inside
// it, as a page in a browser does. Build one with SelectionArea.
type SelectionAreaWidget struct {
	child ggui.Widget

	// The Selectable texts inside, in the order they were painted, which
	// is the order a selection runs through them; painting gathers the
	// next frame's.
	texts, painting []*RichTextWidget

	// The selection runs from the anchor, where it began, to the focus,
	// where it ends, each a text and an offset in it.
	anchor, focus     *RichTextWidget
	anchorAt, focusAt int
	spans             bool // the selection spans more than one text
}

var selectionAreaKey = ggui.NewEnvKey[*SelectionAreaWidget]("selection area")

// SelectionArea lets the reader select across the Selectable texts inside
// child at once: a drag from one runs through those painted after it, ⌘A
// or Ctrl+A in one selects them all, and ⌘C or Ctrl+C copies what is
// selected of each, a line apart. A conversation of many paragraphs, each
// a text of its own, then copies as it reads.
//
//	widget.SelectionArea(widget.Column(
//		widget.Text(question).Selectable(),
//		widget.Text(answer).Selectable(),
//	))
func SelectionArea(child ggui.Widget) *SelectionAreaWidget { return &SelectionAreaWidget{child: child} }

// Layout implements Widget.
func (a *SelectionAreaWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return a.child.Layout(c, env.With(selectionAreaKey, a))
}

// Paint implements Widget. A press on the area between its texts clears
// the selection.
//
// A frame painted in part paints inert, around what changed, and keeps the
// texts the last whole frame found, as it keeps that frame's hit regions:
// gathered from it, they would be the few around a hovered row, and a
// click would clear the selection of those alone.
func (a *SelectionAreaWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if !dst.TakesInput() {
		dst.Paint(a.child, r)
		return
	}
	dst.HitPointer(r, a)
	a.painting = a.painting[:0]
	dst.Paint(a.child, r)
	a.texts, a.painting = a.painting, a.texts
	clear(a.painting)
}

// HandlePointer implements PointerHandler: a press that reaches the area,
// past every text, clears the selection and lets the press go on.
func (a *SelectionAreaWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind == ggui.PointerDown && ev.Button == ggui.MouseButtonLeft && !ev.Mods.Shift {
		a.clear(nil)
	}
	return false
}

// SelectedText returns what is selected in the area, each text's part a
// line apart, in the order the texts were painted.
func (a *SelectionAreaWidget) SelectedText() string {
	var parts []string
	for _, t := range a.texts {
		if t.ed.HasSelection() {
			parts = append(parts, t.ed.Selected())
		}
	}
	return strings.Join(parts, "\n")
}

// painted notes t, a Selectable text inside the area, as painted now.
func (a *SelectionAreaWidget) painted(t *RichTextWidget) { a.painting = append(a.painting, t) }

// pointer handles the pointer for t, a text inside the area: a press
// starts a selection in it, as a lone text's would, and a drag carries the
// selection into the texts it reaches.
func (a *SelectionAreaWidget) pointer(t *RichTextWidget, ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerDown:
		if ev.Button != ggui.MouseButtonLeft {
			return false
		}
		if ev.Mods.Shift && a.anchor != nil && slices.Contains(a.texts, a.anchor) && a.anchor != t {
			a.extend(t, t.offsetAt(ev.Pos))
			return true
		}
		a.clear(t)
		t.clicks.Press(&t.ed, ggui.ClockTime(), ev.Pos, t.offsetAt, ev.Mods.Shift)
		a.anchor, a.anchorAt = t, t.ed.Anchor
		a.focus, a.focusAt = t, t.ed.Caret
		return true
	case ggui.PointerDrag:
		u, at := a.textAt(ev.Pos)
		if u == nil || u == a.anchor || !slices.Contains(a.texts, a.anchor) {
			// Within the text it began in, the selection grows as a lone
			// text's does: by words after a double click.
			a.clear(t)
			t.clicks.Drag(&t.ed, ev.Pos, t.offsetAt)
			a.anchor, a.anchorAt, a.focus, a.focusAt = t, t.ed.Anchor, t, t.ed.Caret
			return true
		}
		a.extend(u, at)
		return true
	case ggui.PointerUp, ggui.PointerTap, ggui.PointerMove, ggui.PointerEnter, ggui.PointerExit:
		return true
	}
	return false
}

// extend moves the selection's focus to offset at of u: the anchor's text
// is selected from the anchor on toward u, the texts between wholly, and
// u up to at.
func (a *SelectionAreaWidget) extend(u *RichTextWidget, at int) {
	i, j := slices.Index(a.texts, a.anchor), slices.Index(a.texts, u)
	if i < 0 || j < 0 {
		return
	}
	a.focus, a.focusAt, a.spans = u, at, i != j
	lo, hi := min(i, j), max(i, j)
	// The texts it runs through are not the one the pointer reached, which
	// alone the frame would paint again.
	ggui.RepaintAll()
	for k, t := range a.texts {
		switch {
		case k == i && k == j:
			t.ed.MoveTo(a.anchorAt, false)
			t.ed.MoveTo(at, true)
		case k == i:
			t.ed.MoveTo(a.anchorAt, false)
			t.ed.MoveTo(gem.Pick(i < j, len(t.ed.Text), 0), true)
		case k == j:
			t.ed.MoveTo(gem.Pick(i < j, 0, len(t.ed.Text)), false)
			t.ed.MoveTo(at, true)
		case k > lo && k < hi:
			t.ed.SelectAll()
		default:
			t.ed.MoveTo(t.ed.Caret, false)
		}
	}
}

// selectAll selects every text in the area.
func (a *SelectionAreaWidget) selectAll() {
	for _, t := range a.texts {
		t.ed.SelectAll()
	}
	if len(a.texts) > 0 {
		first, last := a.texts[0], a.texts[len(a.texts)-1]
		a.anchor, a.anchorAt, a.focus, a.focusAt = first, 0, last, len(last.ed.Text)
		a.spans = len(a.texts) > 1
	}
}

// clear drops the selection of every text but keep. A text it drops one
// in is painted again with the frame, though the pointer reached another
// or none: a click in one paragraph clears the rest of a selection.
func (a *SelectionAreaWidget) clear(keep *RichTextWidget) {
	for _, t := range a.texts {
		if t != keep && t.ed.HasSelection() {
			t.ed.MoveTo(t.ed.Caret, false)
			ggui.RepaintAll()
		}
	}
	a.spans = false
}

// textAt returns the text inside the area at the window point at, and the
// offset in it nearest at. Between texts it is the one level with at, or
// the one above it, at its end; above every text, the first, at its start.
func (a *SelectionAreaWidget) textAt(at ggui.Point) (*RichTextWidget, int) {
	for _, t := range a.texts {
		if t.rect.Contains(at) {
			return t, t.offsetAt(at)
		}
	}
	for _, t := range a.texts {
		if at.Y >= t.rect.Origin.Y && at.Y < t.rect.Origin.Y+t.rect.Size.H {
			return t, t.offsetAt(at)
		}
	}
	var above *RichTextWidget
	for _, t := range a.texts {
		if t.rect.Origin.Y <= at.Y {
			above = t
		}
	}
	if above != nil {
		return above, len(above.ed.Text)
	}
	if len(a.texts) > 0 {
		return a.texts[0], 0
	}
	return nil, 0
}

// Find selects the match of needle n places on, from 0, among the area's
// texts painted from the window's y top down, in paint order, and reports
// where it is painted, in window coordinates: a page's find, for a page
// whose rows are laid out only near the window. Case is ignored. ok is
// false, and the selection is left as it was, when there are no more than
// n matches.
func (a *SelectionAreaWidget) Find(needle string, top float64, n int) (at ggui.Rect, ok bool) {
	if needle == "" || n < 0 {
		return ggui.Rect{}, false
	}
	for _, t := range a.texts {
		if t.rect.Origin.Y+t.rect.Size.H <= top {
			continue
		}
		text := t.ed.Text
		for i := 0; ; {
			lo, hi := textedit.IndexFold(text, needle, i)
			if lo < 0 {
				break
			}
			if n > 0 {
				n--
				i = hi
				continue
			}
			a.clear(t)
			t.ed.MoveTo(lo, false)
			t.ed.MoveTo(hi, true)
			a.anchor, a.anchorAt, a.focus, a.focusAt = t, lo, t, hi
			first := true
			t.para.Rects(lo, hi, t.rect.Size.W, t.alignment(), func(b ggui.Rect) {
				b.Origin = t.rect.Origin.Add(b.Origin)
				if first {
					at, first = b, false
				} else {
					at = at.Union(b)
				}
			})
			return at, true
		}
	}
	return ggui.Rect{}, false
}

// Selectable lets the reader select the paragraph's text and copy it: a
// drag selects from where it began, a double click a word, a triple click
// the line, a Shift-click extends; once a click has focused it ⌘A or
// Ctrl+A selects everything and ⌘C or Ctrl+C copies the selection. The
// selection is drawn in the Env's EditorStyle Selection color while the
// text has the focus. It stays out of the Tab order.
func (t *RichTextWidget) Selectable() *RichTextWidget {
	defer property.Watch(&t.props, &t.selectable)()
	t.selectable = true
	return t
}

// Selectable makes the text selectable and copyable, as
// RichTextWidget.Selectable does.
func (t *TextWidget) Selectable() *TextWidget {
	if t.rich == nil {
		t.rich = &selectableText{w: RichTextWidget{wrap: true}}
		t.rich.w.Selectable()
		t.props.Changed()
	}
	return t
}

// Selection returns the selected byte range of Text, lo <= hi; lo == hi
// when nothing is selected.
func (t *RichTextWidget) Selection() (lo, hi int) { return t.ed.Selection() }

// SelectedText returns the selected text.
func (t *RichTextWidget) SelectedText() string { return t.ed.Selected() }

// layoutRich lays a Selectable Text out as a one-span RichText in style,
// the Text's own merged onto the one it inherits.
func (t *TextWidget) layoutRich(c ggui.Constraints, env ggui.Env, style ggui.TextStyle) ggui.Size {
	r := &t.rich.w
	if len(r.cur) != 1 || r.cur[0].text != t.value {
		r.spans.Set([]TextSpan{Span(t.value)})
	}
	r.style = style
	r.wrap, r.ellipsis, r.maxLines = t.wrap, t.ellipsis, t.maxLines
	r.align, r.alignSet, r.role = t.align, t.alignSet, t.role
	return r.Layout(c, env)
}

// Describe implements Describer: text whose selection a screen reader can
// read and move.
func (t *RichTextWidget) Describe() ggui.Node {
	lo, hi := t.ed.Selection()
	return ggui.Node{
		Role:     roleOr(t.role, ggui.RoleText),
		Name:     t.para.Text,
		Actions:  ggui.ActionFocus | ggui.ActionSetSelection,
		SelStart: lo,
		SelEnd:   hi,
	}
}

// Act implements Actor: a screen reader moves the selection.
func (t *RichTextWidget) Act(a ggui.Action) bool {
	if a.Kind != ggui.ActionSetSelection || !t.selectable {
		return false
	}
	t.ed.MoveTo(a.SelStart, false)
	t.ed.MoveTo(a.SelEnd, true)
	return true
}

// TabStop implements TabStopper: selectable text is focused by a click,
// never by Tab.
func (t *RichTextWidget) TabStop() bool { return false }

// ConsumesKey implements KeyConsumer: Escape clears a selection, and the
// arrows move one; with nothing selected Escape passes on.
func (t *RichTextWidget) ConsumesKey(ev ggui.KeyEvent) bool {
	if ev.Kind != ggui.KeyPress {
		return false
	}
	switch ev.Key {
	case ggui.KeyEscape:
		return t.ed.HasSelection() || t.area != nil && t.area.spans
	case ggui.KeyArrowLeft, ggui.KeyArrowRight, ggui.KeyHome, ggui.KeyEnd:
		return ev.Mods.Shift
	}
	return false
}

// ClaimsChord implements ChordClaimer: select-all and copy stay with the
// focused text, ahead of a shortcut on the same chord.
func (t *RichTextWidget) ClaimsChord(ev ggui.KeyEvent) bool {
	return ev.Kind == ggui.KeyPress && t.selectable && ev.Mods.Cmd() && (ev.Key == ggui.KeyA || ev.Key == ggui.KeyC)
}

// HandleKey implements KeyHandler.
func (t *RichTextWidget) HandleKey(ev ggui.KeyEvent) {
	switch ev.Kind {
	case ggui.KeyFocus:
		t.focused = true
	case ggui.KeyBlur:
		t.focused = false
	case ggui.KeyPress:
		m := ev.Mods
		switch ev.Key {
		case ggui.KeyA:
			if m.Cmd() && t.area != nil {
				t.area.selectAll()
			} else if m.Cmd() {
				t.ed.SelectAll()
			}
		case ggui.KeyC:
			if m.Cmd() && t.area != nil && t.area.spans {
				ggui.CurrentClipboard().Write(t.area.SelectedText())
			} else if m.Cmd() && t.ed.HasSelection() {
				ggui.CurrentClipboard().Write(t.ed.Selected())
			}
		case ggui.KeyEscape:
			if t.area != nil {
				t.area.clear(nil)
			}
			t.ed.MoveTo(t.ed.Caret, false)
		case ggui.KeyArrowLeft, ggui.KeyArrowRight:
			if m.Shift {
				t.ed.MoveBy(gem.Pick(ev.Key == ggui.KeyArrowLeft, -1, 1), textedit.WordWise(m.Alt, m.Ctrl, m.Meta), true)
			}
		case ggui.KeyHome, ggui.KeyEnd:
			if m.Shift {
				t.ed.MoveTo(gem.Pick(ev.Key == ggui.KeyHome, 0, len(t.ed.Text)), true)
			}
		}
	}
}

// HandlePointer implements PointerHandler: a press places the selection's
// start, a drag extends it, and repeated clicks select a word, then the
// line.
func (t *RichTextWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if t.area != nil {
		return t.area.pointer(t, ev)
	}
	switch ev.Kind {
	case ggui.PointerDown:
		if ev.Button != ggui.MouseButtonLeft {
			return false
		}
		t.clicks.Press(&t.ed, ggui.ClockTime(), ev.Pos, t.offsetAt, ev.Mods.Shift)
		return true
	case ggui.PointerDrag:
		t.clicks.Drag(&t.ed, ev.Pos, t.offsetAt)
		return true
	case ggui.PointerUp, ggui.PointerTap, ggui.PointerMove, ggui.PointerEnter, ggui.PointerExit:
		return true
	}
	return false
}

// CaptureTouchDrag implements TouchDragCapturer: a touch drag that began
// on selectable text selects rather than scrolls.
func (t *RichTextWidget) CaptureTouchDrag() bool { return t.selectable }

// CaptureDrag implements ggui.DragCapturer: a selection dragged in text
// inside a drag source selects rather than drags the source.
func (t *RichTextWidget) CaptureDrag() bool { return t.selectable }

// offsetAt returns the text offset nearest the window point at.
func (t *RichTextWidget) offsetAt(at ggui.Point) int {
	return t.para.OffsetAt(ggui.Pt(at.X-t.rect.Origin.X, at.Y-t.rect.Origin.Y), t.rect.Size.W, t.alignment())
}

// Adopt implements Adopter: a rebuilt paragraph at the same place keeps
// the previous one's focus and, while the text is the same, its selection.
func (t *RichTextWidget) Adopt(prev any) {
	p, ok := prev.(*RichTextWidget)
	if !ok || p == t {
		return
	}
	t.focused = p.focused
	t.clicks = p.clicks
	if p.ed.Text == t.ed.Text {
		t.ed.Anchor, t.ed.Caret = p.ed.Anchor, p.ed.Caret
	}
}
