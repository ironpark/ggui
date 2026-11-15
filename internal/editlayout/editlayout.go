// Package editlayout is the layout behind widget.TextInput: its text cut
// into lines, wrapped to a width or broken at line breaks only, and each
// line holding right-to-left text laid out in visual order. It is kept by
// hard line, so that an edit lays out again only the lines it changed:
// wrapping all of a long text at each keystroke costs milliseconds.
//
// It measures with the face and style its caller hands it, and paints
// nothing.
package editlayout

import (
	"image/color"
	"sort"
	"strings"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/paragraph"
	"github.com/ironpark/ggui/internal/textedit"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/text"
)

// Key is what measuring the text depends on beside the text: the face and
// the language right-to-left text is shaped in. Generation is the caller's
// count of font changes, which a measurement made before one may not
// survive.
type Key struct {
	Generation uint64
	Font       *fontsys.Font
	Weight     fontsys.Weight
	Size       float64
	Tracking   float64
	Lang       string
}

// Mode is how a text is cut into lines.
type Mode uint8

const (
	OneLine   Mode = iota // the text is one line, which scrolls sideways
	WrapWidth             // at line breaks and to the width
	BreakOnly             // at line breaks only, as NoWrap has it
)

// Style is what a wrap depends on beside the text.
type Style struct {
	Key   Key
	Width float64 // when wrapping to it
	Mode  Mode
}

// spanKey is what a wrap depends on.
type spanKey struct {
	text  string
	style Style
}

// Wrap is a text's wrap: its lines, and for each the hard line it is on
// and its index there, which holds what else is known of it.
type Wrap struct {
	// Spans are the lines, as byte ranges of the text.
	Spans []textlayout.Span
	key   spanKey
	lines []lineRef
	rev   uint64 // the editor's name for key.text, or 0 if it gave none
}

// Text is the text r wraps.
func (r *Wrap) Text() string { return r.key.text }

// Rev is the name the editor gave r's text when it was wrapped, which its
// Changed knows again, or 0 if it gave none.
func (r *Wrap) Rev() uint64 { return r.rev }

type lineRef struct {
	w *lineWrap
	i int
}

// lineWraps holds the wraps of hard lines in one style by the line's text.
// Lines that neither of the wraps kept uses go once there are many more
// than those have.
type lineWraps struct {
	style  Style
	byText map[string]*lineWrap
	bytes  int // the keys' bytes
	gen    uint64
}

// lineWrap is a hard line's wrap, its spans relative to its start, and
// each span laid out in visual order, when first asked for.
type lineWrap struct {
	spans []textlayout.Span
	bidi  []*Line // noBidi for a span that needs no reordering
	used  uint64  // the gen of the last pruning that found it in use
}

// Cache wraps an editor's texts, and keeps what it measured for the next
// edit. Its zero value is ready once Face and Style are set.
type Cache struct {
	// Face is the face the text is measured in to wrap it to a width, at a
	// scale of 1.
	Face func() text.Face
	// Style is the style a line holding right-to-left text is laid out in.
	// It is asked only for a line not laid out before.
	Style func() paragraph.Style

	// wrapped caches the wraps of the two texts last asked for, which
	// layout, paint, the caret's blink and hit testing all ask for again:
	// two, since while the IME composes, paint wraps the text with the
	// composition in it and the rest the text without.
	wrapped [2]*Wrap
	// lines caches each hard line's wrap and visual layout by its text.
	lines lineWraps
}

// Wrap wraps s in style. ed is the editor s is the text of, or was: what
// it knows of its last edit spares comparing the text with the one before.
// The result is shared, and lasts until two other texts are wrapped, which
// reuse its memory: callers must not change it or keep it.
func (c *Cache) Wrap(s string, style Style, ed *textedit.Editor) *Wrap {
	key := spanKey{s, style}
	w := &c.wrapped
	if w[0] != nil && w[0].key == key {
		return w[0]
	}
	if w[1] != nil && w[1].key == key {
		w[0], w[1] = w[1], w[0]
		return w[0]
	}
	r := c.wrapText(key, ed, w[0], w[1])
	r.rev = ed.Rev(s)
	w[0], w[1] = r, w[0]
	return r
}

// Last is the wrap asked for last, or nil before any.
func (c *Cache) Last() *Wrap { return c.wrapped[0] }

// Held reports how many hard lines' wraps c keeps for the next edit, and
// the bytes of their text.
func (c *Cache) Held() (lines, bytes int) { return len(c.lines.byText), c.lines.bytes }

// wrapText wraps key's text from prev, the wrap made last, as far as it
// can: the lines before the first one the text changed in, and after the
// last, are prev's, those after moved by what the text grew or shrank by.
// An edit wraps again only the lines it changed, and a line whose text
// c.lines holds, as an undo brings back, not even those. The wrap is made
// in spare, the wrap before prev, which this one replaces, and in its
// memory: a keystroke in a long text allocates for the lines it changed
// only.
func (c *Cache) wrapText(key spanKey, ed *textedit.Editor, prev, spare *Wrap) *Wrap {
	s, style := key.text, key.style
	l := &c.lines
	if l.byText == nil || l.style != style {
		l.style, l.byText, l.bytes = style, map[string]*lineWrap{}, 0
	}
	r := spare
	if r == nil {
		r = new(Wrap)
	}
	r.key = key
	if style.Mode == OneLine {
		// The text is the line, and a new one at each keystroke: kept by
		// it, each would be a copy of the text for nothing.
		r.Spans = append(r.Spans[:0], textlayout.Span{Start: 0, End: len(s)})
		r.lines = append(r.lines[:0], lineRef{c.newLineWrap(s), 0})
		return r
	}
	from, to := 0, len(s) // the lines of s wrapped here
	head, tail := 0, -1   // prev's spans kept: those before head, and from tail on
	delta := 0            // how much longer s is than prev's text
	if prev != nil && prev.key.style == style {
		old := prev.key.text
		// Where the texts differ: what the editor's last edit changed,
		// when it made the one from the other, else found by comparing.
		a, cut, _, ok := ed.Changed(prev.rev, s)
		z := len(old) - a - cut
		if !ok {
			a = textedit.CommonPrefix(old, s)
			z = textedit.CommonSuffix(old[a:], s[a:])
		}
		delta = len(s) - len(old)
		from = strings.LastIndexByte(s[:a], '\n') + 1
		head = sort.Search(len(prev.Spans), func(k int) bool { return prev.Spans[k].Start >= from })
		if i := strings.IndexByte(s[len(s)-z:], '\n'); i >= 0 {
			to = len(s) - z + i // a line break both texts end with
			tail = sort.Search(len(prev.Spans), func(k int) bool { return prev.Spans[k].Start > to-delta })
		}
	}
	r.Spans, r.lines = r.Spans[:0], r.lines[:0]
	if head > 0 {
		r.Spans, r.lines = append(r.Spans, prev.Spans[:head]...), append(r.lines, prev.lines[:head]...)
	}
	for start, line := range textlayout.Lines(s[from:to]) {
		start += from
		lw := c.lineWrap(line)
		for i, sp := range lw.spans {
			r.Spans = append(r.Spans, textlayout.Span{Start: start + sp.Start, End: start + sp.End})
			r.lines = append(r.lines, lineRef{lw, i})
		}
	}
	if tail >= 0 {
		for _, sp := range prev.Spans[tail:] {
			r.Spans = append(r.Spans, textlayout.Span{Start: sp.Start + delta, End: sp.End + delta})
		}
		r.lines = append(r.lines, prev.lines[tail:]...)
	}
	c.prune(r)
	return r
}

// lineWrap returns line's wrap in c.lines' style, wrapping it when c.lines
// holds none.
func (c *Cache) lineWrap(line string) *lineWrap {
	l := &c.lines
	if lw := l.byText[line]; lw != nil {
		return lw
	}
	lw := c.newLineWrap(line)
	// The key is a copy: the line is cut from a text the next keystroke
	// replaces, which it would otherwise keep.
	l.byText[strings.Clone(line)] = lw
	l.bytes += len(line)
	return lw
}

// newLineWrap wraps line in c.lines' style.
func (c *Cache) newLineWrap(line string) *lineWrap {
	lw := &lineWrap{spans: []textlayout.Span{{Start: 0, End: len(line)}}}
	if c.lines.style.Mode == WrapWidth {
		lw.spans = textlayout.WrapSpans(line, c.Face(), c.lines.style.Width)
	}
	return lw
}

// prune lets go of the lines neither r nor the wrap before it uses, once
// there are many more than they have, or many more bytes, as a long line
// typed in leaves a copy of itself at each keystroke: an edit's leftovers.
func (c *Cache) prune(r *Wrap) {
	l := &c.lines
	if len(l.byText) <= 2*len(r.lines)+64 && l.bytes <= 2*len(r.key.text)+4096 {
		return
	}
	l.gen++
	for _, w := range []*Wrap{r, c.wrapped[0]} {
		if w != nil && w.key.style == l.style {
			for _, ref := range w.lines {
				ref.w.used = l.gen
			}
		}
	}
	for k, lw := range l.byText {
		if lw.used != l.gen {
			delete(l.byText, k)
			l.bytes -= len(k)
		}
	}
}

// Line is a line holding right-to-left text, laid out in visual order by
// paragraph as Text lays out such a line: the caret, the selection, a
// click and the drawing all go by its pieces. A line of left-to-right text
// alone is measured and drawn as it is.
type Line struct {
	s      string
	pieces []paragraph.Piece
	styles []paragraph.LineStyle
}

// X is where the caret at offset b of the line stands.
func (l *Line) X(b int) float64 { return paragraph.CaretX(l.s, l.pieces, l.styles, b) }

// At is the offset of the line nearest x.
func (l *Line) At(x float64) int { return paragraph.CaretAt(l.s, l.pieces, l.styles, x) }

// Width is the line's width.
func (l *Line) Width() float64 { return paragraph.Width(l.pieces) }

// SpanX calls fn with the stretches, from the line's left edge, that the
// offsets lo to hi of the line cover: one per piece they reach.
func (l *Line) SpanX(lo, hi int, fn func(a, b float64)) {
	paragraph.SpanX(l.s, l.pieces, l.styles, lo, hi, fn)
}

// Draw draws the line into dst at scale, its baseline that far below the
// top, as paragraph.Draw does: op is reused for each piece, col gives its
// color and place moves it to where the line is.
func (l *Line) Draw(dst *ggfx.Image, op *text.DrawOptions, scale, baseline float64, col func(int) color.Color, place func(*text.DrawOptions)) {
	paragraph.Draw(dst, op, l.s, l.pieces, l.styles, scale, baseline, col, place)
}

// noBidi marks a line that needs no reordering.
var noBidi = new(Line)

// Bidi returns line i of r laid out in visual order, or nil when it holds
// no right-to-left text. The layout is kept with the line's wrap, so a
// line an edit did not change is not laid out again.
func (c *Cache) Bidi(r *Wrap, i int) *Line {
	ref := r.lines[i]
	lw := ref.w
	if lw.bidi == nil {
		lw.bidi = make([]*Line, len(lw.spans))
	}
	bl := lw.bidi[ref.i]
	if bl == nil {
		sp := r.Spans[i]
		bl = c.layoutBidi(r.key.text[sp.Start:sp.End], r.key.style.Key.Lang)
		lw.bidi[ref.i] = bl
	}
	if bl == noBidi {
		return nil
	}
	return bl
}

// layoutBidi lays line out in visual order, shaped in lang, or returns
// noBidi when it holds no right-to-left text.
func (c *Cache) layoutBidi(line, lang string) *Line {
	if !paragraph.NeedsBidi(line, false) {
		return noBidi
	}
	line = strings.Clone(line) // it outlives the text it was cut from
	styles := []paragraph.LineStyle{{Start: 0, End: len(line), Style: c.Style()}}
	return &Line{s: line, pieces: paragraph.LayoutLine(line, styles, false, lang), styles: styles}
}
