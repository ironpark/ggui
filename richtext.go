package ggui

import (
	"image/color"
	"slices"
	"strings"

	"github.com/ironpark/ggui/internal/paragraph"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/textedit"
	"github.com/ironpark/ggui/internal/third_party/text"
)

// TextSpan is one stretch of a RichText in a style of its own. It is a
// description value: build one with Span, and each setter returns a
// changed copy.
type TextSpan struct {
	text       string
	style      TextStyle
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
	para      paragraph.Paragraph
	styles    []TextStyle   // each of para.Runs' style, resolved, for what the engine does not take
	base      TextStyle     // the paragraph's style, resolved
	dir       TextDirection // what para was laid out in
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
func (t *RichTextWidget) Text() string { return t.para.Text }

// alignment is the fraction lines are placed at.
func (t *RichTextWidget) alignment() float64 {
	return pick(t.alignSet, t.align, startAlign(t.dir))
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
	// A span's background, resolved here for the paint, which has no Env.
	for i := range t.cur {
		if t.cur[i].background != nil {
			t.cur[i].background = ResolveColor(t.cur[i].background, env)
		}
	}
	base := env.ResolveText(t.style)
	p := &t.para
	p.Runs, t.styles = p.Runs[:0], t.styles[:0]
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
		p.Runs = append(p.Runs, paragraph.Run{Start: start, End: end, Style: st.engine(), Span: i})
		t.styles = append(t.styles, st)
		keys = append(keys, runKey{start, end, st.Font, st.Size, st.LineHeight, st.Weight})
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
	p.Base, t.base = base.engine(), base
	t.dir, p.Lang = env.Direction(), env.Language()
	p.RTL = t.dir == DirRTL
	t.selection = env.EditorStyle().Selection
	// As Text's: the width only matters when something wraps or cuts to it.
	fits := t.wrap || t.ellipsis || t.maxLines > 0
	key := richKey{generation: fontGen(), text: text, maxW: pick(fits, c.MaxW, 0), wrap: t.wrap, ellipsis: t.ellipsis, maxLines: t.maxLines,
		dir: t.dir, lang: p.Lang, base: runKey{font: base.Font, size: base.Size, lineHeight: base.LineHeight, weight: base.Weight}}
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
				l.Role = RoleLink
				l.SetName(pick(sp.name != "", sp.name, sp.text))
				t.links = append(t.links, l)
			}
		}
	}
	return c.Constrain(p.Size)
}

// runStyle returns run i's resolved style, or the paragraph's for -1.
func (t *RichTextWidget) runStyle(i int) TextStyle {
	if i < 0 {
		return t.base
	}
	return t.styles[i]
}

// sameSpan compares spans by what they show, and their tap by presence.
func sameSpan(a, b TextSpan) bool {
	return a.text == b.text && sameAny(a.style, b.style) && a.underline == b.underline && a.strike == b.strike &&
		sameAny(a.background, b.background) && a.name == b.name && (a.onTap == nil) == (b.onTap == nil)
}

// Baseline implements Baseliner: the first line's baseline.
func (t *RichTextWidget) Baseline() (float64, bool) {
	if len(t.para.Lines) == 0 {
		return 0, false
	}
	return t.para.Lines[0].Baseline, true
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
	case p.Text != "" && !dst.named(r):
		dst.Node(r, Node{Role: roleOr(t.role, RoleText), Name: p.Text}, described)
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
		p.Rects(lo, hi, r.Size.W, align, func(b Rect) {
			dst.FillRect(Rct(r.Origin.Add(b.Origin), b.Size), t.selection)
		})
	}
	scale := dst.Scale()
	for i := range p.Lines {
		ln := &p.Lines[i]
		// A line wholly outside the clip is not shaped and drawn to be
		// clipped, with a line's height to spare for what reaches past it.
		top, h := ln.Baseline-ln.Ascent, ln.Ascent+ln.Descent
		if !dst.visiblePaintBounds(Rct(Pt(r.Origin.X, r.Origin.Y+top), Sz(r.Size.W, h)), h) {
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
			bg := Rct(Pt(x+pc.X-pad, r.Origin.Y+ln.Baseline-rs.Size*0.92), Sz(pc.W+2*pad, rs.Size*1.18))
			dst.FillRoundRect(bg, rs.Size*0.2, t.cur[p.Runs[st.Run].Span].background)
		}
		col := func(style int) color.Color { return t.runStyle(ln.Styles[style].Run).Color }
		paragraph.Draw(dst.Image, &t.pieceOp, ln.S, ln.Pieces, ln.Styles, scale, dst.px(ln.Baseline), col, func(op *text.DrawOptions) {
			op.GeoM.Translate(dst.px(x), dst.px(r.Origin.Y))
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
				dst.FillRect(Rct(Pt(x+pc.X, y), Sz(pc.W, thick)), rs.Color)
			}
			if span.strike {
				// Through the middle of the lower case letters.
				y := r.Origin.Y + ln.Baseline - rs.Size*0.3
				dst.FillRect(Rct(Pt(x+pc.X, y), Sz(pc.W, thick)), rs.Color)
			}
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
	for _, run := range p.Runs {
		if run.Span != l.span {
			continue
		}
		p.Rects(run.Start, run.End, r.Size.W, align, func(b Rect) {
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
		col := l.w.base.Color
		for i, run := range p.Runs {
			if run.Span == l.span {
				col = l.w.styles[i].Color
				break
			}
		}
		for _, b := range boxes {
			dst.StrokeRoundRect(Rct(b.Origin.Add(Pt(-2, -1)), Sz(b.Size.W+4, b.Size.H+2)), 2, 1, col)
		}
	}
}

// SelectionAreaWidget lets a drag select across the Selectable texts inside
// it, as a page in a browser does. Build one with SelectionArea.
type SelectionAreaWidget struct {
	child Widget

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

var selectionAreaKey = NewEnvKey[*SelectionAreaWidget]("selection area")

// SelectionArea lets the reader select across the Selectable texts inside
// child at once: a drag from one runs through those painted after it, ⌘A
// or Ctrl+A in one selects them all, and ⌘C or Ctrl+C copies what is
// selected of each, a line apart. A conversation of many paragraphs, each
// a text of its own, then copies as it reads.
//
//	ggui.SelectionArea(ggui.Column(
//		ggui.Text(question).Selectable(),
//		ggui.Text(answer).Selectable(),
//	))
func SelectionArea(child Widget) *SelectionAreaWidget { return &SelectionAreaWidget{child: child} }

// Layout implements Widget.
func (a *SelectionAreaWidget) Layout(c Constraints, env Env) Size {
	return a.child.Layout(c, env.With(selectionAreaKey, a))
}

// Paint implements Widget. A press on the area between its texts clears
// the selection.
func (a *SelectionAreaWidget) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, a)
	a.painting = a.painting[:0]
	dst.Paint(a.child, r)
	a.texts, a.painting = a.painting, a.texts
	clear(a.painting)
}

// HandlePointer implements PointerHandler: a press that reaches the area,
// past every text, clears the selection and lets the press go on.
func (a *SelectionAreaWidget) HandlePointer(ev PointerEvent) bool {
	if ev.Kind == PointerDown && ev.Button == MouseButtonLeft && !ev.Mods.Shift {
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
func (a *SelectionAreaWidget) pointer(t *RichTextWidget, ev PointerEvent) bool {
	switch ev.Kind {
	case PointerDown:
		if ev.Button != MouseButtonLeft {
			return false
		}
		if ev.Mods.Shift && a.anchor != nil && slices.Contains(a.texts, a.anchor) && a.anchor != t {
			a.extend(t, t.offsetAt(ev.Pos))
			return true
		}
		a.clear(t)
		t.clicks.press(&t.ed, ev.Pos, t.offsetAt, ev.Mods.Shift)
		a.anchor, a.anchorAt = t, t.ed.Anchor
		a.focus, a.focusAt = t, t.ed.Caret
		return true
	case PointerDrag:
		u, at := a.textAt(ev.Pos)
		if u == nil || u == a.anchor || !slices.Contains(a.texts, a.anchor) {
			// Within the text it began in, the selection grows as a lone
			// text's does: by words after a double click.
			a.clear(t)
			t.clicks.drag(&t.ed, ev.Pos, t.offsetAt)
			a.anchor, a.anchorAt, a.focus, a.focusAt = t, t.ed.Anchor, t, t.ed.Caret
			return true
		}
		a.extend(u, at)
		return true
	case PointerUp, PointerTap, PointerMove, PointerEnter, PointerExit:
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
	delivered.Add(1)
	for k, t := range a.texts {
		switch {
		case k == i && k == j:
			t.ed.MoveTo(a.anchorAt, false)
			t.ed.MoveTo(at, true)
		case k == i:
			t.ed.MoveTo(a.anchorAt, false)
			t.ed.MoveTo(pick(i < j, len(t.ed.Text), 0), true)
		case k == j:
			t.ed.MoveTo(pick(i < j, 0, len(t.ed.Text)), false)
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
			delivered.Add(1)
		}
	}
	a.spans = false
}

// textAt returns the text inside the area at the window point at, and the
// offset in it nearest at. Between texts it is the one level with at, or
// the one above it, at its end; above every text, the first, at its start.
func (a *SelectionAreaWidget) textAt(at Point) (*RichTextWidget, int) {
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
func (a *SelectionAreaWidget) Find(needle string, top float64, n int) (at Rect, ok bool) {
	if needle == "" || n < 0 {
		return Rect{}, false
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
			t.para.Rects(lo, hi, t.rect.Size.W, t.alignment(), func(b Rect) {
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
	return Rect{}, false
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
		t.rich, t.richAt = RichText().Selectable(), new(richAt)
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
func (t *TextWidget) layoutRich(c Constraints, env Env, style TextStyle) Size {
	r := t.rich
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
func (t *RichTextWidget) Describe() Node {
	lo, hi := t.ed.Selection()
	return Node{
		Role:     roleOr(t.role, RoleText),
		Name:     t.para.Text,
		Actions:  ActionFocus | ActionSetSelection,
		SelStart: lo,
		SelEnd:   hi,
	}
}

// Act implements Actor: a screen reader moves the selection.
func (t *RichTextWidget) Act(a Action) bool {
	if a.Kind != ActionSetSelection || !t.selectable {
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
func (t *RichTextWidget) ConsumesKey(ev KeyEvent) bool {
	if ev.Kind != KeyPress {
		return false
	}
	switch ev.Key {
	case KeyEscape:
		return t.ed.HasSelection() || t.area != nil && t.area.spans
	case KeyArrowLeft, KeyArrowRight, KeyHome, KeyEnd:
		return ev.Mods.Shift
	}
	return false
}

// ClaimsChord implements ChordClaimer: select-all and copy stay with the
// focused text, ahead of a shortcut on the same chord.
func (t *RichTextWidget) ClaimsChord(ev KeyEvent) bool {
	return ev.Kind == KeyPress && t.selectable && ev.Mods.Cmd() && (ev.Key == KeyA || ev.Key == KeyC)
}

// HandleKey implements KeyHandler.
func (t *RichTextWidget) HandleKey(ev KeyEvent) {
	switch ev.Kind {
	case KeyFocus:
		t.focused = true
	case KeyBlur:
		t.focused = false
	case KeyPress:
		m := ev.Mods
		switch ev.Key {
		case KeyA:
			if m.Cmd() && t.area != nil {
				t.area.selectAll()
			} else if m.Cmd() {
				t.ed.SelectAll()
			}
		case KeyC:
			if m.Cmd() && t.area != nil && t.area.spans {
				currentClipboard().Write(t.area.SelectedText())
			} else if m.Cmd() && t.ed.HasSelection() {
				currentClipboard().Write(t.ed.Selected())
			}
		case KeyEscape:
			if t.area != nil {
				t.area.clear(nil)
			}
			t.ed.MoveTo(t.ed.Caret, false)
		case KeyArrowLeft, KeyArrowRight:
			if m.Shift {
				t.ed.MoveBy(pick(ev.Key == KeyArrowLeft, -1, 1), wordWise(m), true)
			}
		case KeyHome, KeyEnd:
			if m.Shift {
				t.ed.MoveTo(pick(ev.Key == KeyHome, 0, len(t.ed.Text)), true)
			}
		}
	}
}

// HandlePointer implements PointerHandler: a press places the selection's
// start, a drag extends it, and repeated clicks select a word, then the
// line.
func (t *RichTextWidget) HandlePointer(ev PointerEvent) bool {
	if t.area != nil {
		return t.area.pointer(t, ev)
	}
	switch ev.Kind {
	case PointerDown:
		if ev.Button != MouseButtonLeft {
			return false
		}
		t.clicks.press(&t.ed, ev.Pos, t.offsetAt, ev.Mods.Shift)
		return true
	case PointerDrag:
		t.clicks.drag(&t.ed, ev.Pos, t.offsetAt)
		return true
	case PointerUp, PointerTap, PointerMove, PointerEnter, PointerExit:
		return true
	}
	return false
}

// CaptureTouchDrag implements TouchDragCapturer: a touch drag that began
// on selectable text selects rather than scrolls.
func (t *RichTextWidget) CaptureTouchDrag() bool { return t.selectable }

// offsetAt returns the text offset nearest the window point at.
func (t *RichTextWidget) offsetAt(at Point) int {
	return t.para.OffsetAt(Pt(at.X-t.rect.Origin.X, at.Y-t.rect.Origin.Y), t.rect.Size.W, t.alignment())
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
