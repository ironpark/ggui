package ggui

import (
	"image"
	"image/color"
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/textedit"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/text"
	"github.com/ironpark/ggui/property"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

func TestDefaultMonoFontFollowsItsSetting(t *testing.T) {
	t.Parallel()
	mono := MustFont(gomono.TTF)
	p := NewProbe(labelled("x"), Sz(10, 10)).Setup(func() { SetDefaultMonoFont(mono) })
	defer p.Close()
	p.Frame()
	var monoWidth, textWidth float64
	p.Post(func() {
		monoWidth = textlayout.Width("iiii", DefaultMonoFont().face(14, 0))
		textWidth = textlayout.Width("iiii", fallbackFont().face(14, 0))
	})
	p.Frame()
	if want := textlayout.Width("iiii", mono.face(14, 0)); monoWidth != want || textWidth == want {
		t.Fatalf("DefaultMonoFont drew %v wide and the text font %v, want Go Mono's %v for the first only", monoWidth, textWidth, want)
	}
}

func TestDefaultMonoFontItalicFace(t *testing.T) {
	t.Parallel()
	// In a Probe with no mono font set, DefaultMonoFont resolves to builtin, not system mono italic.
	p1 := NewProbe(labelled("x"), Sz(10, 10))
	defer p1.Close()
	var probeItalic *Font
	p1.Post(func() { probeItalic = DefaultMonoFont().italicFace() })
	p1.Frame()
	if probeItalic != builtinFont() {
		t.Fatalf("Probe DefaultMonoFont().italicFace() = %p, want builtinFont (%p)", probeItalic, builtinFont())
	}

	// With SetDefaultMonoFont using a custom WithItalic, DefaultMonoFont().italicFace() returns that italic.
	italic := MustFont(gobold.TTF)
	mono := MustFont(gomono.TTF).WithItalic(italic)
	p2 := NewProbe(labelled("x"), Sz(10, 10)).Setup(func() { SetDefaultMonoFont(mono) })
	defer p2.Close()
	var got *Font
	p2.Post(func() { got = DefaultMonoFont().italicFace() })
	p2.Frame()
	if got != italic {
		t.Fatalf("DefaultMonoFont().italicFace() = %p, want custom italic (%p)", got, italic)
	}
}

func TestAProbeDrawsInTheBuiltInFont(t *testing.T) {
	t.Parallel()
	p := NewProbe(labelled("x"), Sz(10, 10))
	defer p.Close()
	var got *Font
	p.Post(func() { got = fallbackFont() })
	p.Frame()
	if got != builtinFont() {
		t.Fatal("a Probe drew in a font other than the built-in one, so tests would vary by machine")
	}
}

func TestDrawTextStyleKeepsTheWeight(t *testing.T) {
	t.Parallel()
	family := MustFont(goregular.TTF).WithWeight(WeightBold, MustFont(gobold.TTF))
	c := &Canvas{}
	c.scale = 2
	regular := c.TextStyleWidth("Weighty words", TextStyle{Font: family, Size: 14})
	bold := c.TextStyleWidth("Weighty words", TextStyle{Font: family, Size: 14, Weight: WeightBold})
	if bold <= regular {
		t.Fatalf("bold measured %v, regular %v: want the bold face, which is wider", bold, regular)
	}
	if got := c.TextWidth("Weighty words", family, 14); got != regular {
		t.Fatalf("TextWidth %v, TextStyleWidth %v: want the same regular face", got, regular)
	}
	// Mono faces keep one advance across weights, which a grid relies on.
	mono := MustFont(gomono.TTF)
	if a, b := c.TextStyleWidth("M", TextStyle{Font: mono, Size: 13}), c.TextStyleWidth("M", TextStyle{Font: mono, Size: 13, Weight: WeightBold}); a != b {
		t.Fatalf("a mono face without bold measured %v and %v", a, b)
	}
	c.DrawTextStyle("no image, no panic", TextStyle{}, Pt(0, 0))
}

func useTestEmoji(t *testing.T) *Font {
	t.Helper()
	data, err := os.ReadFile("assets/testdata/emoji-subset.ttf")
	if err != nil {
		t.Fatal(err)
	}
	f := MustFont(data)
	w := activeWorld()
	old := w.emoji
	SetEmojiFont(f)
	t.Cleanup(func() { setEmoji(w, old) })
	return f
}
func TestEmojiSequencesStayInOneColorGlyph(t *testing.T) {
	t.Parallel()
	useTestEmoji(t)
	face := fallbackFont().face(20, 0)
	for _, s := range []string{"👍", "👍🏽", "🇰🇷", "👩🏽‍💻", "👨‍👩‍👧‍👦", "1️⃣", "🏳️‍🌈", "❤️", "🫩"} {
		t.Run(s, func(t *testing.T) {
			for run, f := range fontsys.TextRuns(s, face) {
				if run != s {
					t.Fatalf("split sequence: %q", run)
				}
				glyphs := text.AppendLazyGlyphs(nil, run, f, nil)
				if len(glyphs) != 1 || !glyphs[0].Colored() {
					t.Fatalf("%q: expected one color glyph, got %d", s, len(glyphs))
				}
			}
			if textlayout.Width(s, face) <= 0 {
				t.Fatal("zero width")
			}
		})
	}
}
func TestEmojiPresentationAndTextShaping(t *testing.T) {
	t.Parallel()
	useTestEmoji(t)
	face := fallbackFont().face(20, 0)
	ef := face.(*fontsys.EmojiFace)
	for _, s := range []string{"AV 123 # * © ♥︎", "plain text"} {
		if got, want := textlayout.Width(s, face), text.AdvanceAt(s, len(s), ef.Face); got != want {
			t.Fatalf("ordinary text width changed: %q %v != %v", s, got, want)
		}
	}
	for _, s := range []string{"👍🏽", "🇰🇷", "👩🏽‍💻", "1️⃣", "🏳️‍🌈", "❤️"} {
		source := strings.Repeat(s, 3)
		if got := textlayout.Wrap(source, face, 1); len(got) != 3 {
			t.Fatalf("split emoji clusters: %q", got)
		} else {
			for _, line := range got {
				if line != s {
					t.Fatalf("broken cluster: %q", line)
				}
			}
		}
		spans := textlayout.WrapSpans(source, face, 1)
		if len(spans) != 3 {
			t.Fatalf("editor wrap: %+v", spans)
		}
		for _, span := range spans {
			if source[span.Start:span.End] != s {
				t.Fatalf("editor broke cluster: %+v", span)
			}
		}
		e := textedit.Editor{Text: source, Caret: len(source), Anchor: len(source)}
		e.Backspace(false)
		if e.Text != strings.Repeat(s, 2) {
			t.Fatalf("backspace broke %q: %q", s, e.Text)
		}
	}
}

func TestKoreanFallsBackToASystemFont(t *testing.T) {
	t.Parallel()
	if !systemFontDrawsKorean() {
		t.Skip("no system font draws Korean on this machine")
	}
	face := fallbackFont().face(14, 0)
	if ef, ok := face.(*fontsys.EmojiFace); ok {
		face = ef.Face
	}
	if _, ok := face.(*text.MultiFace); !ok {
		t.Fatalf("default face is %T, want a MultiFace with system fallbacks", face)
	}
	if textlayout.Width("한글", face) <= 0 {
		t.Fatal("Korean measures as zero width: no glyphs")
	}
	bare := MustFont(goregular.TTF).NoFallback()
	if bare.face(14, 0).(*text.GoTextFace) == nil || textlayout.Width("한글", bare.face(14, 0)) >= textlayout.Width("한글", face) {
		t.Fatal("NoFallback still draws Korean, or the fallback did not change the measurement")
	}
}

// systemFontDrawsKorean reports whether a system font has glyphs for
// Korean. A machine can have system fonts and none of them for Korean, as
// a Linux one with DejaVu Sans alone does, where the fallback draws the
// same .notdef boxes as the built-in font.
func systemFontDrawsKorean() bool {
	for _, f := range SystemFonts() {
		glyphs := text.AppendGlyphs(nil, "한글", f.face(14, 0), nil)
		if len(glyphs) > 0 && !slices.ContainsFunc(glyphs, func(g text.Glyph) bool { return g.GID == 0 }) {
			return true
		}
	}
	return false
}

// TestGlyphsNoListedFontHasComeFromAnyFontThatDoes draws a rune none of
// SystemFonts has, which the default font finds in whichever system font
// has it.
func TestGlyphsNoListedFontHasComeFromAnyFontThatDoes(t *testing.T) {
	t.Parallel()
	drawn := func(s string, face text.Face) bool {
		glyphs := text.AppendGlyphs(nil, s, face, nil)
		return len(glyphs) > 0 && !slices.ContainsFunc(glyphs, func(g text.Glyph) bool { return g.GID == 0 })
	}
	var s string
	for _, r := range []rune{'𓀀', 'ᚠ', 'Ꭰ', 'ⴰ', 'ሀ', 'ก', 'ა', 'अ'} {
		if fontsys.SystemFallback(r) == nil || slices.ContainsFunc(SystemFonts(), func(f *Font) bool { return drawn(string(r), f.face(14, 0)) }) {
			continue
		}
		s = string(r)
		break
	}
	if s == "" {
		t.Skip("no system font has a rune that the listed ones lack")
	}
	face := fallbackFont().face(14, 0)
	if ef, ok := face.(*fontsys.EmojiFace); ok {
		face = ef.Face
	}
	if !drawn(s, face) {
		t.Fatalf("%q draws .notdef, though %s has it", s, fontsys.SystemFallback([]rune(s)[0]).Meta().Metadata().Family)
	}
	if !drawn("a"+s+"b", face) || textlayout.Width("a"+s+"b", face) <= textlayout.Width("ab", face) {
		t.Fatalf("%q between Latin letters is not drawn", s)
	}
}

func TestLoadFontRejectsGarbage(t *testing.T) {
	t.Parallel()
	if _, err := LoadFont([]byte("not a font")); err == nil {
		t.Fatal("LoadFont(garbage) = nil error")
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
