package paragraph

import (
	"image/color"
	"strings"
	"testing"

	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/pkg/layout"
)

// The benchmarks set text in the built-in font with the system's fonts
// behind it, as an App's text is, so the right-to-left ones measure
// whichever Hebrew and Arabic fonts the machine has. Compare runs on one
// machine only.

const (
	benchLatin = "The quick brown fox jumps over the lazy dog, again and again."
	benchMixed = "Reply from שלום עולם at 10:42 — مرحبا بالعالم, see you soon."
)

func benchStyle(size float64) Style {
	return Style{Font: fontsys.Builtin(), Size: size, Weight: 400, LineHeight: 1.2}
}

// benchParagraph is a paragraph of three runs in two sizes, its text the
// line repeated n times.
func benchParagraph(line string, n int, rtl bool) *Paragraph {
	body := strings.Repeat(line+" ", n)
	third := len(body) / 3
	for third < len(body) && body[third]&0xC0 == 0x80 {
		third++ // keep the cut on a rune boundary
	}
	return &Paragraph{
		Text: body,
		Runs: []Run{
			{Start: 0, End: third, Style: benchStyle(14)},
			{Start: third, End: 2 * third, Style: benchStyle(18), Span: 1},
			{Start: 2 * third, End: len(body), Style: benchStyle(14), Span: 2},
		},
		Base: benchStyle(14),
		RTL:  rtl,
	}
}

func benchLine(b *testing.B, s string, rtl bool) {
	styles := []LineStyle{{Start: 0, End: len(s), Style: benchStyle(14)}}
	LayoutLine(s, styles, rtl, "") // load the fonts and fill the caches first
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		LayoutLine(s, styles, rtl, "")
	}
}

func BenchmarkLayoutLineLatin(b *testing.B) { benchLine(b, benchLatin, false) }
func BenchmarkLayoutLineMixed(b *testing.B) { benchLine(b, benchMixed, false) }
func BenchmarkLayoutLineRTL(b *testing.B)   { benchLine(b, benchMixed, true) }

// BenchmarkCaretAt finds the caret across a laid-out mixed line, as a click
// or a drag along it does.
func BenchmarkCaretAt(b *testing.B) {
	styles := []LineStyle{{Start: 0, End: len(benchMixed), Style: benchStyle(14)}}
	ps := LayoutLine(benchMixed, styles, false, "")
	w := Width(ps)
	b.ReportAllocs()
	b.ResetTimer()
	x := 0.0
	for b.Loop() {
		CaretAt(benchMixed, ps, styles, x)
		x += 7
		if x > w {
			x = 0
		}
	}
}

func benchLayout(b *testing.B, line string, rtl bool) {
	p := benchParagraph(line, 20, rtl)
	p.Layout(320, true, 0, false)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		p.Layout(320, true, 0, false)
	}
}

// BenchmarkParagraphLayout wraps a paragraph of three runs to a width, as
// RichText does each time it is laid out at a new width.
func BenchmarkParagraphLayout(b *testing.B)      { benchLayout(b, benchLatin, false) }
func BenchmarkParagraphLayoutMixed(b *testing.B) { benchLayout(b, benchMixed, false) }
func BenchmarkParagraphLayoutRTL(b *testing.B)   { benchLayout(b, benchMixed, true) }

// BenchmarkParagraphLayoutCut wraps the same paragraph to four lines, the
// last cut with an ellipsis.
func BenchmarkParagraphLayoutCut(b *testing.B) {
	p := benchParagraph(benchLatin, 20, false)
	b.ReportAllocs()
	for b.Loop() {
		p.Layout(320, true, 4, true)
	}
}

// BenchmarkOffsetAt maps points across a laid-out paragraph to offsets, as
// selecting with the pointer does.
func BenchmarkOffsetAt(b *testing.B) {
	p := benchParagraph(benchMixed, 20, false)
	p.Layout(320, true, 0, false)
	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		at := layout.Pt(float64(i%320), float64(i%int(p.Size.H+1)))
		p.OffsetAt(at, 320, 0)
		i += 13
	}
}

// BenchmarkRender draws a wrapped paragraph on the CPU, as RenderText does.
func BenchmarkRender(b *testing.B) {
	s := strings.Repeat(benchLatin+" ", 6)
	st := benchStyle(14)
	b.ReportAllocs()
	for b.Loop() {
		Render(s, st, color.Black, 320)
	}
}
