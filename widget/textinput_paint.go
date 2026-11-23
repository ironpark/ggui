package widget

import (
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/gem"
	"github.com/ironpark/ggui/internal/editlayout"
	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/text"
)

// caretBlink is how long the caret shows and hides for.
const caretBlink = 530 * time.Millisecond

// blinkWake returns when a caret that toggles every period since start next
// changes, for WakeAt.
func blinkWake(start, now time.Time, period time.Duration) time.Time {
	n := now.Sub(start)/period + 1
	return start.Add(n * period)
}

// px converts a logical length or coordinate to dst's Image pixels, as
// Canvas.Px does, in the float64 a GeoM takes.
func px(dst *ggui.Canvas, v float64) float64 { return v * dst.Scale() }

// inputView is the part of the editor's text a paint shows: the lines
// first to last of shown, as wrapped breaks it, drawn in clip, line i's
// top left at x0 and i spacings below y0, each line h tall, and the
// caret's top left at caret.
type inputView struct {
	clip        *ggui.Canvas
	shown       string
	wrapped     *editlayout.Wrap
	first, last int
	x0, y0      float64
	spacing, h  float64
	caret       ggui.Point
}

// top is where line i of v starts down the canvas.
func (v *inputView) top(i int) float64 { return v.y0 + float64(i)*v.spacing }

// caretBounds is the caret's box in whole pixels, for the native IME.
func (v *inputView) caretBounds() image.Rectangle {
	return image.Rect(int(v.caret.X), int(v.caret.Y), int(v.caret.X)+1, int(v.caret.Y+v.h))
}

// Paint implements Widget.
func (t *TextInputWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t.Sync()
	// Where it is being painted is recorded first: describing it reports
	// where its characters are, which is measured from here.
	t.rect = r
	if dst != nil {
		t.scale = dst.Scale()
	}
	// A disabled editor takes no input but is still read out.
	dst.Describe(r, t)
	if !t.IsDisabled() {
		dst.HitPointer(r, t)
		dst.HitKey(r, t)
		dst.HitCursor(r, ggui.CursorShapeText)
	}
	if t.multiline {
		t.paintLines(dst, r)
		return
	}

	shown, caret := t.rendered()
	v := inputView{shown: shown, last: 1, y0: r.Origin.Y, h: t.height()}
	v.wrapped = t.wrap(shown)
	bl := t.bidiAt(v.wrapped, 0)
	var caretX, total float64
	if bl != nil {
		scale := t.measureScale()
		caretX, total = bl.X(caret)/scale, bl.Width()/scale
	} else {
		caretX, total = t.advance(shown[:caret]), t.advance(shown)
	}
	t.caretScroll(caretX, caretX, r.Size.W-1, total-r.Size.W+1)
	v.x0 = r.Origin.X - t.scroll
	v.caret = ggui.Pt(v.x0+caretX, v.y0)
	t.caretBounds = v.caretBounds()
	if dst == nil || dst.Image == nil {
		return
	}
	v.clip = dst.Clip(r)
	t.paintSelection(&v)

	op := &text.DrawOptions{}
	op.GeoM.Translate(px(dst, v.x0), px(dst, v.y0))
	if shown == "" && t.placeholder != "" {
		op.ColorScale.ScaleWithColor(t.muted)
		fontsys.DrawText(v.clip.Image, placeholderLines(t.placeholder, false)[0], t.face(dst.Scale()), op)
	} else {
		t.drawText(&v, 0, t.textColors(shown), op)
	}

	t.paintComposition(&v)
	t.paintCaret(&v)
}

// paintLines is Paint for a Multiline editor: wrapped lines that scroll
// vertically to keep the caret in view.
func (t *TextInputWidget) paintLines(dst *ggui.Canvas, r ggui.Rect) {
	t.width = r.Size.W
	shown, caret := t.rendered()
	wrapped := t.wrap(shown)
	spans := wrapped.Spans
	li := textlayout.LineOf(spans, caret)
	caretX := t.xAt(wrapped, li, caret)
	caretY := float64(li) * t.spacing()
	h := t.height()
	t.caretScroll(caretY, caretY+h, r.Size.H, t.linesHeight(len(spans))-r.Size.H)
	v := inputView{shown: shown, wrapped: wrapped, x0: r.Origin.X, y0: r.Origin.Y - t.scroll, spacing: t.spacing(), h: h}
	v.caret = ggui.Pt(v.x0+caretX, v.y0+caretY)
	t.caretBounds = v.caretBounds()
	if dst == nil || dst.Image == nil {
		return
	}
	v.clip = dst.Clip(r)
	v.first, v.last = t.visibleLines(&v, len(spans), r)
	t.paintSelection(&v)

	op := &text.DrawOptions{}
	if shown == "" && t.placeholder != "" {
		// The text drawer stacks no lines of its own: each line of the
		// placeholder goes on a line of the field, as typed text would.
		op.ColorScale.ScaleWithColor(t.muted)
		lines := placeholderLines(t.placeholder, true)
		pFirst, pLast := t.visibleLines(&v, len(lines), r)
		for i := pFirst; i < pLast; i++ {
			op.GeoM.Reset()
			op.GeoM.Translate(px(dst, v.x0), px(dst, v.top(i)))
			fontsys.DrawText(v.clip.Image, lines[i], t.face(dst.Scale()), op)
		}
	} else {
		colors := t.textColors(shown)
		for i := v.first; i < v.last; i++ {
			t.drawText(&v, i, colors, op)
		}
	}

	t.paintComposition(&v)
	t.paintCaret(&v)
}

// caretScroll clamps the scroll to between zero and limit, then moves it
// as little as keeps the caret's stretch, lo to hi along the scroll, in a
// window view long.
func (t *TextInputWidget) caretScroll(lo, hi, view, limit float64) {
	t.scroll = gem.Clamp(t.scroll, 0, max(limit, 0))
	if hi-t.scroll > view {
		t.scroll = hi - view
	}
	if lo-t.scroll < 0 {
		t.scroll = lo
	}
}

// visibleLines returns the [first, last) lines of n, spaced as v's, that
// intersect r, in O(1), so lines scrolled out of view are neither
// measured nor drawn.
func (t *TextInputWidget) visibleLines(v *inputView, n int, r ggui.Rect) (first, last int) {
	sp, h := v.spacing, v.h
	if n <= 0 || !(sp > 0) {
		return 0, max(n, 0)
	}
	lo := math.Floor((t.scroll - h) / sp)
	hi := math.Ceil((t.scroll+r.Size.H)/sp) + 1
	if lo >= float64(n) {
		return n, n
	}
	if lo > 0 {
		first = int(lo)
	}
	if hi <= float64(first) {
		return first, first
	}
	if hi >= float64(n) {
		last = n
	} else {
		last = int(hi)
	}
	for first < last && v.top(first)+h < r.Origin.Y {
		first++
	}
	for last > first && v.top(last-1) > r.Origin.Y+r.Size.H {
		last--
	}
	return first, last
}

// eachLine calls fn with the part of [lo, hi) that falls on each line of
// v in view, as x offsets, extending to the line's end when the range
// runs on.
func (t *TextInputWidget) eachLine(v *inputView, lo, hi int, fn func(i int, a, b float64)) {
	if v.first >= v.last {
		return
	}
	spans := v.wrapped.Spans
	start := max(v.first, textlayout.LineOf(spans, lo))
	for i := start; i < v.last; i++ {
		sp := spans[i]
		if sp.Start > hi {
			break
		}
		if lo > sp.End {
			continue
		}
		// A range that runs on past the line takes a space's width at
		// its end, which shows the line break in it.
		runOn := 0.0
		if hi > sp.End && i+1 < len(spans) {
			runOn = t.advance(" ")
		}
		t.spanX(shownLine{text: v.shown, span: sp, bidi: t.bidiAt(v.wrapped, i)}, lo, hi, runOn, func(a, b float64) { fn(i, a, b) })
	}
}

// paintSelection fills the selection behind the text on the lines of v in
// view, while the editor has the focus and nothing is being composed.
func (t *TextInputWidget) paintSelection(v *inputView) {
	if !t.Focused() || t.composition != "" || !t.ed.HasSelection() {
		return
	}
	lo, hi := t.ed.Selection()
	t.eachLine(v, lo, hi, func(i int, a, b float64) {
		v.clip.FillRect(ggui.Rct(ggui.Pt(v.x0+a, v.top(i)), ggui.Sz(b-a, v.h)), t.selection)
	})
}

// paintComposition underlines the text the IME is composing.
func (t *TextInputWidget) paintComposition(v *inputView) {
	if t.composition == "" {
		return
	}
	lo, _ := t.ed.Selection()
	t.eachLine(v, lo, lo+len(t.composition), func(i int, a, b float64) {
		v.clip.FillRect(ggui.Rct(ggui.Pt(v.x0+a, v.top(i)+v.h-1), ggui.Sz(b-a, 1)), t.resolved.Color)
	})
}

// paintCaret draws the caret while the editor has the focus, in the half
// of each blink it shows in, and wakes the frame for the blink's next turn.
func (t *TextInputWidget) paintCaret(v *inputView) {
	if !t.Focused() {
		return
	}
	if now := ggui.FrameTime(); (now.Sub(t.blink)/caretBlink)%2 == 0 {
		v.clip.FillRect(ggui.Rct(v.caret, ggui.Sz(1, v.h)), t.resolved.Color)
	}
	ggui.WakeAt(blinkWake(t.blink, ggui.FrameTime(), caretBlink))
}

// PaintCustom replaces the standard text drawing, for segmented editors such
// as Input OTP. The callback returns the logical caret bounds used by the native
// IME. The caller owns hit regions and semantics and forwards keyboard events
// and HandleTick to this editor. Layout must have run first.
func (t *TextInputWidget) PaintCustom(dst *ggui.Canvas, r ggui.Rect, paint func(*ggui.Canvas, ggui.Rect, TextInputState) ggui.Rect) {
	t.rect = r
	if dst != nil {
		t.scale = dst.Scale()
	}
	caret := paint(dst, r, t.EditingState())
	t.caretBounds = image.Rect(int(caret.Origin.X), int(caret.Origin.Y), int(caret.Origin.X+caret.Size.W), int(caret.Origin.Y+caret.Size.H))
}

// placeholderLines splits a placeholder into the lines a field shows: one
// per line of a multiline field, and one line with spaces for its breaks
// in a single-line field, which has room for no other.
func placeholderLines(s string, multiline bool) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if multiline {
		return strings.Split(s, "\n")
	}
	return []string{strings.ReplaceAll(s, "\n", " ")}
}

type shownLine struct {
	text string
	span textlayout.Span
	bidi *editlayout.Line
}

// spanX calls fn with the stretches, from the line's left edge, that the
// offsets lo to hi of the text cover on the line's span of it, laid out as
// its bidi layout, or as it is when that is nil: one stretch, or one per
// piece of a reordered line. runOn more is covered past the line's end.
func (t *TextInputWidget) spanX(line shownLine, lo, hi int, runOn float64, fn func(a, b float64)) {
	sp, bl := line.span, line.bidi
	a, b := max(lo, sp.Start), min(hi, sp.End)
	if bl == nil {
		fn(t.advance(line.text[sp.Start:a]), t.advance(line.text[sp.Start:b])+runOn)
		return
	}
	scale := t.measureScale()
	bl.SpanX(a-sp.Start, b-sp.Start, func(x0, x1 float64) { fn(x0/scale, x1/scale) })
	if runOn > 0 {
		end := bl.Width() / scale
		fn(end, end+runOn)
	}
}

// drawText draws line i of v: in the style's colour, and the pieces colors
// colours each in its own, the line drawn again within where each piece
// stands, so it is shaped once, in either direction. op is reused.
func (t *TextInputWidget) drawText(v *inputView, i int, colors []TextColor, op *text.DrawOptions) {
	s, sp, bl := v.shown, v.wrapped.Spans[i], t.bidiAt(v.wrapped, i)
	origin := ggui.Pt(v.x0, v.top(i))
	line := s[sp.Start:sp.End]
	if len(colors) == 0 {
		t.drawLine(v.clip, line, bl, origin, op, t.resolved.Color)
		return
	}
	t.pieces = colorPieces(t.pieces[:0], sp, colors)
	h, w := t.height(), t.lineWidth(s, sp, bl)
	for _, p := range t.pieces {
		col := t.resolved.Color
		if p.Color != nil {
			col = ggui.ResolveColor(p.Color, t.env)
		}
		t.spanX(shownLine{text: s, span: sp, bidi: bl}, p.Start, p.End, 0, func(a, b float64) {
			// A glyph may stand out past the line's ends, as an italic's
			// does: the clip there runs on.
			if a <= 0 {
				a -= h
			}
			if b >= w {
				b += h
			}
			t.drawLine(v.clip.Clip(ggui.Rct(ggui.Pt(origin.X+a, origin.Y), ggui.Sz(b-a, h))), line, bl, origin, op, col)
		})
	}
}

// lineWidth is how wide the line sp of s is laid out as bl, or as it is
// when bl is nil.
func (t *TextInputWidget) lineWidth(s string, sp textlayout.Span, bl *editlayout.Line) float64 {
	if bl != nil {
		return bl.Width() / t.measureScale()
	}
	return t.advance(s[sp.Start:sp.End])
}

// drawLine draws line, laid out as bl or as it is when bl is nil, with its
// top left at origin, in col. op is reused.
func (t *TextInputWidget) drawLine(dst *ggui.Canvas, line string, bl *editlayout.Line, origin ggui.Point, op *text.DrawOptions, col color.Color) {
	if bl == nil {
		op.GeoM.Reset()
		op.ColorScale.Reset()
		op.GeoM.Translate(px(dst, origin.X), px(dst, origin.Y))
		op.ColorScale.ScaleWithColor(col)
		fontsys.DrawText(dst.Image, t.display(line), t.face(dst.Scale()), op)
		return
	}
	scale := sanitizeScale(dst.Scale())
	baseline := t.face(scale).Metrics().HAscent
	bl.Draw(dst.Image, &t.pieceOp, scale/t.measureScale(), baseline,
		func(int) color.Color { return col },
		func(op *text.DrawOptions) { op.GeoM.Translate(px(dst, origin.X), px(dst, origin.Y)) })
}

// textColors is what Colors says of shown, nil for none or a password.
func (t *TextInputWidget) textColors(shown string) []TextColor {
	if t.colors == nil || t.password || shown == "" {
		return nil
	}
	return t.colors(shown)
}

// colorPieces appends to out the pieces of the line sp that colors
// colours, those between them with no Color: in order, none empty,
// covering the line. A stretch out of order, overlapping the one before,
// is left out.
func colorPieces(out []TextColor, sp textlayout.Span, colors []TextColor) []TextColor {
	add := func(a, b int, c color.Color) {
		if a < b {
			out = append(out, TextColor{Start: a, End: b, Color: c})
		}
	}
	lo, hi := 0, len(colors)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if colors[mid].End <= sp.Start {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	at := sp.Start
	for _, c := range colors[lo:] {
		if c.Start >= sp.End {
			break
		}
		a, b := max(c.Start, sp.Start), min(c.End, sp.End)
		if b <= a || a < at {
			continue
		}
		add(at, a, nil)
		add(a, b, c.Color)
		at = b
	}
	add(at, sp.End, nil)
	return out
}
