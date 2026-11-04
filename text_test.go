package ggui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/textedit"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/text"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

func TestTextHeightFollowsLines(t *testing.T) {
	t.Parallel()
	one := Text("a").Layout(Loose(Sz(1000, 1000)), Env{})
	two := Text("a\nb").Layout(Loose(Sz(1000, 1000)), Env{})
	if one.H <= 0 || two.H <= one.H {
		t.Fatalf("heights %v and %v, want one line shorter than two", one.H, two.H)
	}
	if d := two.H - one.H - DefaultTextSize*1.2; d > 1e-9 || d < -1e-9 {
		t.Fatalf("second line added %v, want the line spacing %v", two.H-one.H, DefaultTextSize*1.2)
	}
}

func TestTextWrapsAtSpaces(t *testing.T) {
	t.Parallel()
	w := Text("aaa bbb ccc")
	wide := w.Layout(Loose(Sz(1000, 1000)), Env{})
	if len(w.lines) != 1 {
		t.Fatalf("wide: %d lines, want 1", len(w.lines))
	}
	narrow := w.Layout(Loose(Sz(wide.W*0.7, 1000)), Env{})
	if len(w.lines) != 2 || w.lines[0] != "aaa bbb" || w.lines[1] != "ccc" {
		t.Fatalf("narrow: lines = %q, want [aaa bbb, ccc]", w.lines)
	}
	if narrow.W > wide.W*0.7 || narrow.H <= wide.H {
		t.Fatalf("narrow = %+v, want no wider than %v and taller than %v", narrow, wide.W*0.7, wide.H)
	}
}

func TestTextBreaksLongWordsBetweenRunes(t *testing.T) {
	t.Parallel()
	w := Text(strings.Repeat("x", 40))
	full := w.Layout(Loose(Sz(1000, 1000)), Env{})
	w.Layout(Loose(Sz(full.W/4, 1000)), Env{})
	if len(w.lines) < 4 {
		t.Fatalf("%d lines, want at least 4", len(w.lines))
	}
	for _, l := range w.lines {
		if textlayout.Width(l, w.faceAt(1)) > full.W/4 {
			t.Fatalf("line %q is wider than the limit", l)
		}
	}
}

func TestTextNoWrapKeepsOneLine(t *testing.T) {
	t.Parallel()
	w := Text("aaa bbb ccc").NoWrap()
	w.Layout(Loose(Sz(5, 1000)), Env{})
	if len(w.lines) != 1 {
		t.Fatalf("%d lines, want 1", len(w.lines))
	}
}

func TestTextSizeScalesLayout(t *testing.T) {
	t.Parallel()
	small := Text("hello").Size(10).Layout(Loose(Sz(1000, 1000)), Env{})
	big := Text("hello").Size(20).Layout(Loose(Sz(1000, 1000)), Env{})
	if big.W <= small.W || big.H <= small.H {
		t.Fatalf("size 20 = %+v not larger than size 10 = %+v", big, small)
	}
}

func TestTextHeightFollowsInheritedLineHeight(t *testing.T) {
	t.Parallel()
	w := Text("first\nsecond\nthird")
	c := Loose(Sz(1000, 1000))
	env := Env{}.WithText(TextStyle{Size: 20, LineHeight: 1})
	before := w.Layout(c, env)
	after := w.Layout(c, env.WithText(TextStyle{LineHeight: 2}))
	// Font metrics can leave a rounding residual when the heights are subtracted.
	if after.W != before.W || math.Abs(after.H-before.H-40) > 1e-9 {
		t.Fatalf("line spacing: before=%v after=%v; want unchanged width and 40 more height", before, after)
	}
}

func TestLoadFontRejectsGarbage(t *testing.T) {
	t.Parallel()
	if _, err := LoadFont([]byte("not a font")); err == nil {
		t.Fatal("LoadFont(garbage) = nil error")
	}
}

func TestTextEllipsisCutsWhatDoesNotFit(t *testing.T) {
	t.Parallel()
	w := Text("aaa bbb ccc\nshort").NoWrap().Ellipsis()
	full := w.Layout(Loose(Sz(1000, 1000)), Env{})
	if fmt.Sprint(w.lines) != "[aaa bbb ccc short]" {
		t.Fatalf("lines that fit were changed: %q", w.lines)
	}
	got := w.Layout(Loose(Sz(full.W*0.6, 1000)), Env{})
	if len(w.lines) != 2 || !strings.HasSuffix(w.lines[0], "…") || strings.HasSuffix(w.lines[0], " …") || w.lines[1] != "short" {
		t.Fatalf("lines = %q, want the first cut with an ellipsis and the second whole", w.lines)
	}
	if got.W > full.W*0.6 {
		t.Fatalf("width %v, want no more than %v", got.W, full.W*0.6)
	}
	w.Layout(Loose(Sz(1, 1000)), Env{})
	if w.lines[0] != "…" {
		t.Fatalf("with no room the line is %q, want the ellipsis alone", w.lines[0])
	}
}

func TestTextMaxLinesEndsTheLastShownLine(t *testing.T) {
	t.Parallel()
	w := Text("aaa bbb ccc ddd").MaxLines(2)
	wide := w.Layout(Loose(Sz(1000, 1000)), Env{})
	if len(w.lines) != 1 || w.lines[0] != "aaa bbb ccc ddd" {
		t.Fatalf("text that fits was cut: %q", w.lines)
	}
	narrow := w.Layout(Loose(Sz(wide.W*0.3, 1000)), Env{})
	if len(w.lines) != 2 || !strings.HasSuffix(w.lines[1], "…") {
		t.Fatalf("lines = %q, want two, the second ending in an ellipsis", w.lines)
	}
	for _, l := range w.lines {
		if textlayout.Width(l, w.faceAt(1)) > wide.W*0.3 {
			t.Fatalf("line %q is wider than the limit", l)
		}
	}
	two := Text("a\nb").Layout(Loose(Sz(1000, 1000)), Env{})
	if narrow.H != two.H {
		t.Fatalf("height %v, want two lines' %v", narrow.H, two.H)
	}
	p := NewProbe(w, Sz(wide.W*0.3, 100))
	defer p.Close()
	if _, ok := p.Semantics().Find(RoleText, "aaa bbb ccc ddd"); !ok {
		t.Fatal("the semantics tree lost the text that was cut")
	}
}

func TestCanvasFitTextCutsToTheWidth(t *testing.T) {
	t.Parallel()
	c := &Canvas{scale: 2}
	s := "a fairly long label"
	full := c.TextWidth(s, nil, 14)
	if got := c.FitText(s, nil, 14, full); got != s {
		t.Fatalf("text that fits came back %q", got)
	}
	got := c.FitText(s, nil, 14, full/2)
	if !strings.HasSuffix(got, "…") || c.TextWidth(got, nil, 14) > full/2 {
		t.Fatalf("FitText = %q, %v wide; want an ellipsis within %v", got, c.TextWidth(got, nil, 14), full/2)
	}
}

// inked counts the pixels of img with any alpha, and those with a color
// other than gray.
func inked(img *image.RGBA) (ink, colored int) {
	for i := 0; i < len(img.Pix); i += 4 {
		r, g, b, a := img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]
		if a == 0 {
			continue
		}
		ink++
		if r != g || g != b {
			colored++
		}
	}
	return ink, colored
}

func TestRenderTextDrawsWhatTextMeasures(t *testing.T) {
	t.Parallel()
	font := MustFont(goregular.TTF).NoFallback()
	st := TextStyle{Font: font, Size: 20}
	img := RenderText("Hello, world", st, 0)
	want := textlayout.Width("Hello, world", font.face(20, 0))
	if w := float64(img.Bounds().Dx()); math.Abs(w-want) > 1 {
		t.Errorf("image is %v wide, Text measures %v", w, want)
	}
	if ink, colored := inked(img); ink == 0 || colored != 0 {
		t.Errorf("black text has %d pixels drawn, %d of them colored", ink, colored)
	}
	m := font.face(20, 0).Metrics()
	if h := float64(img.Bounds().Dy()); math.Abs(h-(m.HAscent+m.HDescent)) > 1 {
		t.Errorf("one line is %v high, want its ascent and descent, %v", h, m.HAscent+m.HDescent)
	}

	red := RenderText("Hello", TextStyle{Font: font, Size: 20, Color: color.RGBA{R: 0xff, A: 0xff}}, 0)
	if _, colored := inked(red); colored == 0 {
		t.Error("red text has no colored pixels")
	}

	one := RenderText("one two three four five", st, 0)
	wrapped := RenderText("one two three four five", st, 80)
	if wrapped.Bounds().Dx() > 80 || wrapped.Bounds().Dy() <= one.Bounds().Dy() {
		t.Errorf("wrapped to 80 the text is %v, on one line %v", wrapped.Bounds().Size(), one.Bounds().Size())
	}
	lines := RenderText("a\nb\nc", st, 0)
	if want := m.HAscent + 2*1.2*20 + m.HDescent; math.Abs(float64(lines.Bounds().Dy())-want) > 1 {
		t.Errorf("three lines are %v high, want %v", lines.Bounds().Dy(), want)
	}
}

func TestRenderTextDrawsEmojiInColor(t *testing.T) {
	useTestEmoji(t)
	img := RenderText("a👍b", TextStyle{Font: MustFont(goregular.TTF), Size: 24}, 0)
	if _, colored := inked(img); colored == 0 {
		t.Error("the emoji drew no colored pixels")
	}
}

func TestRenderTextIsSafeAcrossGoroutines(t *testing.T) {
	t.Parallel()
	font := MustFont(goregular.TTF)
	done := make(chan *image.RGBA)
	for range 8 {
		go func() { done <- RenderText("concurrent ascent", TextStyle{Font: font, Size: 16}, 0) }()
	}
	first := <-done
	for range 7 {
		if img := <-done; !img.Bounds().Eq(first.Bounds()) || string(img.Pix) != string(first.Pix) {
			t.Fatal("the same text renders differently on different goroutines")
		}
	}
}

// Each grapheme starts its own cell on a whole pixel, however far the
// font's advance is from the cell's width, so a long row does not drift.
func TestDrawTextCellsKeepsEveryGraphemeOnItsCell(t *testing.T) {
	t.Parallel()
	face := MustFont(gomono.TTF).NoFallback().face(13*2, 0)
	adv := textlayout.Width("M", face)
	cell := math.Round(adv) + 1 // well off the advance, so drift would show
	const x0 = 20.25
	s := "é" + "abcdefghijklmnopqrstuvwxyz0123456789"
	glyphs := text.AppendLazyGlyphs(nil, s, face, nil)
	shifts := cellShifts(nil, textedit.Bounds(s), glyphs, x0, cell)
	k, last := -1, -1
	for i, g := range glyphs {
		at := x0 + g.OriginX + shifts[i]
		if g.StartIndexInBytes < 3 { // é, its mark included
			if g.StartIndexInBytes == 0 {
				k = 0
			}
			if want := math.Round(x0) + g.OriginX - glyphs[0].OriginX; math.Abs(at-want) > 1e-9 {
				t.Fatalf("glyph %d of é at %v, want %v: the mark left its letter", i, at, want)
			}
			continue
		}
		if g.StartIndexInBytes != last {
			k++
			last = g.StartIndexInBytes
		}
		if want := math.Round(x0 + float64(k)*cell); at != want {
			t.Fatalf("grapheme %d at %v, want its cell at %v", k, at, want)
		}
	}
	if k != 36 {
		t.Fatalf("placed %d graphemes after é, want 36", k)
	}
	(&Canvas{}).DrawTextCells("no image, no panic", TextStyle{}, Pt(0, 0), 8)
}

// Shifts found from a run's bounds are those stepping through it grapheme
// by grapheme finds, whatever its clusters are.
func TestCellShiftsFollowTheRunsGraphemes(t *testing.T) {
	face := MustFont(gomono.TTF).NoFallback().face(13*2, 0)
	const x0, cell = 20.25, 17.0
	for _, s := range []string{"é" + "abc", "한글 ok", "á̂b", "👩‍💻x", "ffi fl", "\xffz", "❯ go test"} {
		glyphs := text.AppendLazyGlyphs(nil, s, face, nil)
		got := cellShifts(nil, textedit.Bounds(s), glyphs, x0, cell)
		k, next, shift := -1, 0, 0.0
		for i, g := range glyphs {
			if g.StartIndexInBytes >= next {
				for g.StartIndexInBytes >= next && next < len(s) {
					next = textedit.NextGrapheme(s, next)
					k++
				}
				shift = math.Round(x0+float64(k)*cell) - x0 - g.OriginX
			}
			if got[i] != shift {
				t.Errorf("%q glyph %d: shift %v, want %v", s, i, got[i], shift)
			}
		}
	}
}

func TestTextStylesCompareWithoutAllocating(t *testing.T) {
	a := TextStyle{Size: 13, Color: color.RGBA{1, 2, 3, 255}, LineHeight: 1.4, Weight: 500}
	b := a
	b.Color = color.NRGBA{1, 2, 3, 255}
	if !property.Equal(a, b) {
		t.Fatal("styles of colors drawing alike are not equal")
	}
	c := a
	c.Size = 14
	if property.Equal(a, c) {
		t.Fatal("styles of two sizes are equal")
	}
	if property.Equal(a, TextStyle{Size: 13, LineHeight: 1.4, Weight: 500}) {
		t.Fatal("a style with a color equals one without")
	}
	if n := testing.AllocsPerRun(100, func() { property.Equal(a, b) }); n != 0 {
		t.Fatalf("comparing two styles took %v allocations", n)
	}
}

func TestDirectionalityMirrorsRowsAndColumns(t *testing.T) {
	t.Parallel()
	rtl := Env{}.With(DirectionKey, DirRTL)
	if rtl.Direction() != DirRTL || (Env{}).Direction() != DirLTR || DirRTL.String() != "rtl" {
		t.Fatal("Env.Direction does not follow DirectionKey")
	}
	row := Row(Box().Size(10, 10), Box().Size(20, 10))
	size := row.Layout(Loose(Sz(100, 100)), rtl)
	if row.rects[0].Origin.X != size.W-10 || row.rects[1].Origin.X != 0 {
		t.Fatalf("RTL row offsets %v in %v, want the first child rightmost", row.rects, size)
	}
	col := Column(Box().Size(10, 10), Box().Size(30, 10))
	col.Layout(Loose(Sz(100, 100)), rtl)
	if col.rects[0].Origin.X != 20 || col.rects[1].Origin.X != 0 {
		t.Fatalf("RTL column offsets %v, want AlignStart on the right", col.rects)
	}
	ltr := Row(Box().Size(10, 10), Box().Size(20, 10))
	ltr.Layout(Loose(Sz(100, 100)), Env{})
	if ltr.rects[0].Origin.X != 0 {
		t.Fatalf("LTR row moved: %v", ltr.rects)
	}
}

func TestTextUnderRTLStartsAtTheRight(t *testing.T) {
	t.Parallel()
	rtl := Env{}.With(DirectionKey, DirRTL)
	w := Text("שלום world")
	w.Layout(Loose(Sz(300, 100)), rtl)
	if len(w.bidi.lines) != 1 || w.bidi.lines[0] == nil {
		t.Fatal("RTL text was not reordered")
	}
	if a := pick(w.alignSet, w.align, startAlign(w.bidi.dir)); a != 1 {
		t.Fatalf("default alignment under RTL is %v, want 1", a)
	}
	w.Align(0)
	if a := pick(w.alignSet, w.align, startAlign(w.bidi.dir)); a != 0 {
		t.Fatalf("explicit Align(0) gave %v", a)
	}
	// The Latin word comes first in the reordered line: it is on the left.
	ps := w.bidi.lines[0]
	if ps[0].RTL || w.lines[0][ps[0].Start:ps[0].End] != "world" {
		t.Fatalf("pieces %+v, want world on the left", ps)
	}

	r := RichText(Span("ab"))
	r.Layout(Tight(Sz(200, 20)), rtl)
	var box Rect
	r.para.Rects(0, 2, 200, r.alignment(), func(b Rect) { box = b })
	if math.Abs(box.Origin.X+box.Size.W-200) > 0.01 {
		t.Fatalf("RTL rich text covers %v, want it at the right edge", box)
	}
	// The reordered lines, a selection, an underline and a rotated line
	// all paint into an image.
	img := ggfx.NewImage(300, 120)
	defer img.Deallocate()
	sel := RichText(Span("שלום "), Span("bold").Weight(WeightBold).Underline().OnTap(func() {})).Selectable()
	rot := Text("مرحبا 😀 abc")
	p := NewProbe(Directionality(DirRTL, Column(Text("مرحبا بالعالم 123 😀"), sel, FromFuncs(
		func(c Constraints, env Env) Size { return rot.Layout(c, env) },
		func(dst *Canvas, r Rect) { rot.PaintRotated(dst, r, 0.3) },
	))), Sz(300, 120))
	defer p.Close()
	p.RenderTo(img)
	p.Frame()
	node, ok := p.Semantics().Find(RoleText, "שלום bold")
	if !ok || node.Rect.Origin.X+node.Rect.Size.W < 299 {
		t.Fatalf("selectable RTL text at %v, want it at the right of the column", node.Rect)
	}
	p.Click(Pt(node.Rect.Origin.X+node.Rect.Size.W-2, node.Rect.Origin.Y+node.Rect.Size.H/2))
	p.Key("cmd+a")
	p.Frame()
	if sel.SelectedText() != "שלום bold" {
		t.Fatalf("select all in RTL selected %q", sel.SelectedText())
	}
}
