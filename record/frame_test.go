package record

import (
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/ironpark/ggui"
)

func TestFrameLayout(t *testing.T) {
	for _, s := range []float64{1, 2} {
		tour := Tour{Size: ggui.Sz(400, 300), Scale: s, Frame: FrameMac}
		l := tour.layout(resolve(nil))
		m, bar := int(48*s), int(28*s)
		if want := image.Pt(int(400*s)+2*m, int(300*s)+bar+2*m); l.size != want {
			t.Errorf("scale %g: size %v, want %v", s, l.size, want)
		}
		if want := image.Rect(m, m+bar, m+int(400*s), m+bar+int(300*s)); l.content != want {
			t.Errorf("scale %g: content at %v, want %v", s, l.content, want)
		}
		if want := image.Rect(m, m, m+int(400*s), m+bar+int(300*s)); l.window != want {
			t.Errorf("scale %g: window at %v, want %v", s, l.window, want)
		}
		c, r := l.buttons()
		if r != 6*s {
			t.Errorf("scale %g: buttons of radius %g", s, r)
		}
		for i, x := range []float64{20, 40, 60} {
			want := pt{float64(m) + x*s, float64(m) + 14*s}
			if c[i] != want {
				t.Errorf("scale %g: button %d at %v, want %v", s, i, c[i], want)
			}
		}
		if l.radius != 16*s {
			t.Errorf("scale %g: radius %g", s, l.radius)
		}
		if p := l.device(ggui.Pt(10, 5)); p != (pt{float64(m) + 10*s, float64(m+bar) + 5*s}) {
			t.Errorf("scale %g: (10, 5) of the content is at %v", s, p)
		}
	}

	tour := Tour{Size: ggui.Sz(400, 300), Frame: FrameMac, TitlebarHidden: true}
	l := tour.layout(resolve([]Option{Margin(10), CornerRadius(10)}))
	if l.content != l.window || l.content.Min != image.Pt(10, 10) || l.size != image.Pt(420, 320) || l.radius != 10 {
		t.Errorf("a hidden title bar: %+v", l)
	}
	l = tour.layout(resolve([]Option{Frame(FrameNone)}))
	if l.size != image.Pt(400, 300) || l.content.Min != (image.Point{}) {
		t.Errorf("the Frame option does not override the tour's: %+v", l)
	}
}

func luma(c color.RGBA) int { return (int(c.R)*2126 + int(c.G)*7152 + int(c.B)*722) / 10000 }

// The title bar is a shade of the theme's background, light or dark,
// and is drawn again when the background changes.
func TestFrameFollowsTheme(t *testing.T) {
	tour := Tour{Size: ggui.Sz(200, 100), Scale: 2, Frame: FrameMac}
	l := tour.layout(resolve(nil))
	f := newFramer(l, "", resolve(nil))
	content := fill(400, 200, red)
	barAt := image.Pt(l.window.Max.X-40, l.window.Min.Y+28)
	for _, tc := range []struct {
		bg   color.RGBA
		dark bool
	}{
		{white, false},
		{color.RGBA{R: 0x1e, G: 0x1e, B: 0x1e, A: 0xff}, true},
		{white, false},
	} {
		img := f.draw(content, tc.bg, ggui.TextStyle{})
		bar := img.RGBAAt(barAt.X, barAt.Y)
		if dark := luma(bar) < 128; dark != tc.dark {
			t.Errorf("on %v the bar is %v", tc.bg, bar)
		}
		if bar == tc.bg {
			t.Errorf("the bar is the background, %v, not a shade of it", bar)
		}
		if got := img.RGBAAt(l.content.Min.X+100, l.content.Min.Y+100); got != red {
			t.Errorf("the content is %v", got)
		}
		// The corner is cut round: the content does not show there.
		if got := img.RGBAAt(l.content.Min.X, l.content.Max.Y-1); got == red {
			t.Errorf("the bottom-left corner is not round")
		}
		if got := img.RGBAAt(2, 2); got != color.RGBAModel.Convert(DefaultBackdrop) {
			t.Errorf("the backdrop is %v", got)
		}
		// The close button.
		c, _ := l.buttons()
		if got := img.RGBAAt(int(c[0].x), int(c[0].y)); got != (color.RGBA{R: 0xff, G: 0x5f, B: 0x57, A: 0xff}) {
			t.Errorf("the close button is %v", got)
		}
		// Below the window, the shadow darkens the backdrop.
		under := img.RGBAAt(l.window.Min.X+200, l.window.Max.Y+12)
		if luma(under) >= luma(color.RGBAModel.Convert(DefaultBackdrop).(color.RGBA))-8 {
			t.Errorf("no shadow under the window: %v", under)
		}
	}
}

// A framed tour written as PNG frames with a clear backdrop keeps it
// clear; as a GIF it is drawn over white.
func TestFrameTransparentBackdrop(t *testing.T) {
	tour := boxTour("clear")
	tour.Frame = FrameMac
	tour.Title = "Box"
	dir := t.TempDir()
	if err := tour.WritePNGs(dir, Backdrop(color.Transparent)); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(dir, "0000.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Errorf("the backdrop's corner has alpha %d", a)
	}
	b := img.Bounds()
	if b.Dx() != 60+96 || b.Dy() != 40+28+96 {
		t.Errorf("framed frames of %v", b)
	}
	if _, _, _, a := img.At(b.Dx()/2, b.Dy()-48+10).RGBA(); a == 0 || a == 0xffff {
		t.Errorf("the shadow below the window has alpha %d", a)
	}

	path := filepath.Join(t.TempDir(), "clear.gif")
	if err := tour.WriteGIF(path, Backdrop(color.Transparent)); err != nil {
		t.Fatal(err)
	}
	g := readGIF(t, path)
	if c := g.Image[0].At(0, 0); !near(color.RGBAModel.Convert(c).(color.RGBA), white, 2) {
		t.Errorf("the GIF's backdrop is %v, not white", c)
	}
}

// The title is set in the bar, between the buttons and the right edge.
func TestFrameTitle(t *testing.T) {
	tour := Tour{Size: ggui.Sz(300, 100), Scale: 2, Frame: FrameMac}
	l := tour.layout(resolve(nil))
	plain := newFramer(l, "", resolve(nil)).draw(fill(600, 200, white), white, ggui.TextStyle{})
	titled := newFramer(l, "Notes", resolve(nil)).draw(fill(600, 200, white), white, ggui.TextStyle{})
	diff := changed(plain, titled)
	if diff.Empty() {
		t.Fatal("no title drawn")
	}
	mid := (l.window.Min.X + l.window.Max.X) / 2
	if !diff.In(image.Rect(l.window.Min.X, l.window.Min.Y, l.window.Max.X, l.content.Min.Y)) ||
		diff.Min.X > mid || diff.Max.X < mid {
		t.Errorf("the title is drawn at %v, not in the middle of the bar %v", diff, l.window)
	}
}

// A framed tour draws the frame around what it drew before, unchanged.
func TestFrameKeepsContent(t *testing.T) {
	var plain, framed []*image.RGBA
	keep := func(to *[]*image.RGBA) func(*image.RGBA) error {
		return func(img *image.RGBA) error {
			c := image.NewRGBA(img.Rect)
			copy(c.Pix, img.Pix)
			*to = append(*to, c)
			return nil
		}
	}
	tour := boxTour("keep")
	tour.Scale = 2
	if err := tour.play(resolve(nil), keep(&plain)); err != nil {
		t.Fatal(err)
	}
	tour = boxTour("keep")
	tour.Scale = 2
	tour.Frame = FrameMac
	o := resolve(nil)
	if err := tour.play(o, keep(&framed)); err != nil {
		t.Fatal(err)
	}
	l := tour.layout(o)
	for i := range plain {
		for y := 20; y < 60; y++ {
			for x := 20; x < 100; x++ {
				if a, b := plain[i].RGBAAt(x, y), framed[i].RGBAAt(l.content.Min.X+x, l.content.Min.Y+y); a != b {
					t.Fatalf("frame %d at (%d, %d): %v framed, %v not", i, x, y, b, a)
				}
			}
		}
	}
}

func readGIF(t *testing.T, path string) *gif.GIF {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	g, err := gif.DecodeAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
