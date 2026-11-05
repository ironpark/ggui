package ggui

import (
	"image/color"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/textedit"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/text"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

func TestTextWeightDrawsTheHeavierFace(t *testing.T) {
	t.Parallel()
	family := MustFont(goregular.TTF).WithWeight(WeightBold, MustFont(gobold.TTF))
	loose := Loose(Sz(1000, 1000))
	regular := Text("Weighty words").Font(family).Layout(loose, Env{})
	bold := Text("Weighty words").Font(family).Weight(WeightBold).Layout(loose, Env{})
	if bold.W <= regular.W {
		t.Fatalf("bold is %v wide, regular %v: want the bold face, which is wider", bold.W, regular.W)
	}
	inherited := Styled(Text("Weighty words")).Font(family).Weight(WeightBold)
	if got := inherited.Layout(loose, Env{}); got.W != bold.W {
		t.Fatalf("inherited bold is %v wide, want %v", got.W, bold.W)
	}
	// The built-in font has no bold, so it draws regular.
	if plain, heavy := Text("Weighty words").Layout(loose, Env{}), Text("Weighty words").Weight(WeightBold).Layout(loose, Env{}); plain != heavy {
		t.Fatalf("the built-in font drew bold %v against regular %v, want the same face", heavy, plain)
	}
}

func TestTextRecolorIsPaintOnly(t *testing.T) {
	t.Parallel()
	w := Text("x")
	w.Layout(Loose(Sz(100, 100)), Env{})
	v := w.props.Version()
	w.Color(color.RGBA{R: 255, A: 255})
	if w.props.Version() == v {
		t.Fatal("setting a color over the inherited one did not lay the text out again")
	}
	v = w.props.Version()
	w.Color(color.RGBA{G: 255, A: 255})
	if w.props.Version() != v {
		t.Fatal("swapping one color for another laid the text out again")
	}
	w.Color(nil)
	if w.props.Version() == v {
		t.Fatal("going back to the inherited color did not lay the text out again")
	}
}

func TestDefaultMonoFontFollowsItsSetting(t *testing.T) {
	t.Parallel()
	mono := MustFont(gomono.TTF)
	p := NewProbe(Text("x"), Sz(10, 10)).Setup(func() { SetDefaultMonoFont(mono) })
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

func TestAProbeDrawsInTheBuiltInFont(t *testing.T) {
	t.Parallel()
	p := NewProbe(Text("x"), Sz(10, 10))
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
	data, err := os.ReadFile("testdata/emoji-subset.ttf")
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
func TestEmojiSwitchInvalidatesCachedMeasurement(t *testing.T) {
	t.Parallel()
	useTestEmoji(t)
	w := Text("👩🏽‍💻")
	c := Cached(w)
	constraints := Loose(Sz(1000, 100))
	env := Env{}
	c.Layout(constraints, env)
	old := w.wrapped.generation
	SetEmojiFont(nil)
	c.Layout(constraints, env)
	if w.wrapped.generation == old {
		t.Fatal("cached text did not remeasure after font switch")
	}
}

func TestEmojiPointerCaretAndBackspace(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"👍🏽", "🇰🇷", "👩🏽‍💻", "👨‍👩‍👧‍👦", "1️⃣", "🏳️‍🌈", "❤️"} {
		t.Run(s, func(t *testing.T) {
			// A subtest runs on a goroutine of its own, whose settings are
			// its own.
			useTestEmoji(t)
			useFakeIME(t)
			value := State(s)
			w := TextInput(value)
			w.Layout(Tight(Sz(300, 30)), rootEnv())
			for x := 0.0; x <= w.advance(s)+5; x += .5 {
				i := w.indexOnLine(w.wrap(s), 0, x)
				if i != 0 && i != len(s) {
					t.Fatalf("click at x=%v placed caret inside emoji at byte %d/%d", x, i, len(s))
				}
			}
			w.rect = Rct(Point{}, Sz(300, 30))
			w.HandlePointer(PointerEvent{Kind: PointerDown, Button: MouseButtonLeft, Pos: Pt(299, 10)})
			w.HandleKey(KeyEvent{Kind: KeyPress, Key: KeyBackspace})
			if Untrack(value.Get) != "" {
				t.Fatalf("backspace left emoji fragments: %q", Untrack(value.Get))
			}
		})
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
