package ggui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

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
	want := lineWidth("Hello, world", font.face(20, 0))
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
