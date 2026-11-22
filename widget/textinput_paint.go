package widget

import (
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/editlayout"
	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/text"
	"github.com/ironpark/ggui/pkg/gem"
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
	h := t.height()
	bl := t.bidiAt(t.wrap(shown), 0)
	var caretX, total float64
	if bl != nil {
		scale := t.measureScale()
		caretX, total = bl.X(caret)/scale, bl.Width()/scale
	} else {
		caretX, total = t.advance(shown[:caret]), t.advance(shown)
	}
	t.scroll = gem.Clamp(t.scroll, 0, max(total-r.Size.W+1, 0))
	if caretX-t.scroll > r.Size.W-1 {
		t.scroll = caretX - r.Size.W + 1
	}
	if caretX-t.scroll < 0 {
		t.scroll = caretX
	}
	x0 := r.Origin.X - t.scroll
	t.caretBounds = image.Rect(
		int(x0+caretX), int(r.Origin.Y),
		int(x0+caretX)+1, int(r.Origin.Y+h),
	)
	if dst == nil || dst.Image == nil {
		return
	}
	clip := dst.Clip(r)

	if t.Focused() && t.composition == "" && t.ed.HasSelection() {
		lo, hi := t.ed.Selection()
		t.spanX(shown, textlayout.Span{End: len(shown)}, bl, lo, hi, 0, func(a, b float64) {
			clip.FillRect(ggui.Rct(ggui.Pt(x0+a, r.Origin.Y), ggui.Sz(b-a, h)), t.selection)
		})
	}

	op := &text.DrawOptions{}
	op.GeoM.Translate(px(dst, x0), px(dst, r.Origin.Y))
	if shown == "" && t.placeholder != "" {
		op.ColorScale.ScaleWithColor(t.muted)
		fontsys.DrawText(clip.Image, placeholderLines(t.placeholder, false)[0], t.face(dst.Scale()), op)
	} else {
		t.drawText(clip, shown, textlayout.Span{End: len(shown)}, bl, t.textColors(shown), x0, r.Origin.Y, op)
	}

	if t.composition != "" {
		lo, _ := t.ed.Selection()
		y := r.Origin.Y + h - 1
		t.spanX(shown, textlayout.Span{End: len(shown)}, bl, lo, lo+len(t.composition), 0, func(a, b float64) {
			clip.FillRect(ggui.Rct(ggui.Pt(x0+a, y), ggui.Sz(b-a, 1)), t.resolved.Color)
		})
	}

	if t.Focused() {
		if now := ggui.FrameTime(); (now.Sub(t.blink)/caretBlink)%2 == 0 {
			clip.FillRect(ggui.Rct(ggui.Pt(x0+caretX, r.Origin.Y), ggui.Sz(1, h)), t.resolved.Color)
		}
		ggui.WakeAt(blinkWake(t.blink, ggui.FrameTime(), caretBlink))
	}
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
	t.scroll = gem.Clamp(t.scroll, 0, max(t.linesHeight(len(spans))-r.Size.H, 0))
	if caretY+h-t.scroll > r.Size.H {
		t.scroll = caretY + h - r.Size.H
	}
	if caretY-t.scroll < 0 {
		t.scroll = caretY
	}
	x0, y0 := r.Origin.X, r.Origin.Y-t.scroll
	t.caretBounds = image.Rect(
		int(x0+caretX), int(y0+caretY),
		int(x0+caretX)+1, int(y0+caretY+h),
	)
	if dst == nil || dst.Image == nil {
		return
	}
	clip := dst.Clip(r)
	sp := t.spacing()
	lineY := func(i int) float64 { return y0 + float64(i)*sp }
	// visibleRange returns the [first, last) lines of n that intersect r, in O(1),
	// so lines scrolled out of view are neither measured nor drawn.
	visibleRange := func(n int) (first, last int) {
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
		for first < last && lineY(first)+h < r.Origin.Y {
			first++
		}
		for last > first && lineY(last-1) > r.Origin.Y+r.Size.H {
			last--
		}
		return first, last
	}
	first, last := visibleRange(len(spans))

	// eachLine calls fn with the part of [lo, hi) that falls on each line
	// in view, as x offsets, extending to the line's end when the range
	// runs on.
	eachLine := func(lo, hi int, fn func(i int, a, b float64)) {
		if first >= last {
			return
		}
		start := max(first, textlayout.LineOf(spans, lo))
		for i := start; i < last; i++ {
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
			t.spanX(shown, sp, t.bidiAt(wrapped, i), lo, hi, runOn, func(a, b float64) { fn(i, a, b) })
		}
	}

	if t.Focused() && t.composition == "" && t.ed.HasSelection() {
		lo, hi := t.ed.Selection()
		eachLine(lo, hi, func(i int, a, b float64) {
			clip.FillRect(ggui.Rct(ggui.Pt(x0+a, lineY(i)), ggui.Sz(b-a, h)), t.selection)
		})
	}

	op := &text.DrawOptions{}
	if shown == "" && t.placeholder != "" {
		// The text drawer stacks no lines of its own: each line of the
		// placeholder goes on a line of the field, as typed text would.
		op.ColorScale.ScaleWithColor(t.muted)
		lines := placeholderLines(t.placeholder, true)
		pFirst, pLast := visibleRange(len(lines))
		for i := pFirst; i < pLast; i++ {
			op.GeoM.Reset()
			op.GeoM.Translate(px(dst, x0), px(dst, lineY(i)))
			fontsys.DrawText(clip.Image, lines[i], t.face(dst.Scale()), op)
		}
	} else {
		colors := t.textColors(shown)
		for i := first; i < last; i++ {
			t.drawText(clip, shown, spans[i], t.bidiAt(wrapped, i), colors, x0, lineY(i), op)
		}
	}

	if t.composition != "" {
		lo, _ := t.ed.Selection()
		eachLine(lo, lo+len(t.composition), func(i int, a, b float64) {
			clip.FillRect(ggui.Rct(ggui.Pt(x0+a, lineY(i)+h-1), ggui.Sz(b-a, 1)), t.resolved.Color)
		})
	}

	if t.Focused() {
		if now := ggui.FrameTime(); (now.Sub(t.blink)/caretBlink)%2 == 0 {
			clip.FillRect(ggui.Rct(ggui.Pt(x0+caretX, y0+caretY), ggui.Sz(1, h)), t.resolved.Color)
		}
		ggui.WakeAt(blinkWake(t.blink, ggui.FrameTime(), caretBlink))
	}
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

// spanX calls fn with the stretches, from the line's left edge, that the
// offsets lo to hi of s cover on the line sp of it, laid out as bl, or
// as it is when bl is nil: one stretch, or one per piece of a reordered
// line. runOn more is covered past the line's end.
func (t *TextInputWidget) spanX(s string, sp textlayout.Span, bl *editlayout.Line, lo, hi int, runOn float64, fn func(a, b float64)) {
	a, b := max(lo, sp.Start), min(hi, sp.End)
	if bl == nil {
		fn(t.advance(s[sp.Start:a]), t.advance(s[sp.Start:b])+runOn)
		return
	}
	scale := t.measureScale()
	bl.SpanX(a-sp.Start, b-sp.Start, func(x0, x1 float64) { fn(x0/scale, x1/scale) })
	if runOn > 0 {
		end := bl.Width() / scale
		fn(end, end+runOn)
	}
}

// drawText draws the line sp of s, laid out as bl or as it is when bl is
// nil, with its top left at x, y: in the style's colour, and the pieces
// colors colours each in its own, the line drawn again within where each
// piece stands, so it is shaped once, in either direction. op is reused.
func (t *TextInputWidget) drawText(dst *ggui.Canvas, s string, sp textlayout.Span, bl *editlayout.Line, colors []TextColor, x, y float64, op *text.DrawOptions) {
	line := s[sp.Start:sp.End]
	if len(colors) == 0 {
		t.drawLine(dst, line, bl, x, y, op, t.resolved.Color)
		return
	}
	t.pieces = colorPieces(t.pieces[:0], sp, colors)
	h, w := t.height(), t.lineWidth(s, sp, bl)
	for _, p := range t.pieces {
		col := t.resolved.Color
		if p.Color != nil {
			col = ggui.ResolveColor(p.Color, t.env)
		}
		t.spanX(s, sp, bl, p.Start, p.End, 0, func(a, b float64) {
			// A glyph may stand out past the line's ends, as an italic's
			// does: the clip there runs on.
			if a <= 0 {
				a -= h
			}
			if b >= w {
				b += h
			}
			t.drawLine(dst.Clip(ggui.Rct(ggui.Pt(x+a, y), ggui.Sz(b-a, h))), line, bl, x, y, op, col)
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
// top left at x, y, in col. op is reused.
func (t *TextInputWidget) drawLine(dst *ggui.Canvas, line string, bl *editlayout.Line, x, y float64, op *text.DrawOptions, col color.Color) {
	if bl == nil {
		op.GeoM.Reset()
		op.ColorScale.Reset()
		op.GeoM.Translate(px(dst, x), px(dst, y))
		op.ColorScale.ScaleWithColor(col)
		fontsys.DrawText(dst.Image, t.display(line), t.face(dst.Scale()), op)
		return
	}
	scale := sanitizeScale(dst.Scale())
	baseline := t.face(scale).Metrics().HAscent
	bl.Draw(dst.Image, &t.pieceOp, scale/t.measureScale(), baseline,
		func(int) color.Color { return col },
		func(op *text.DrawOptions) { op.GeoM.Translate(px(dst, x), px(dst, y)) })
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
