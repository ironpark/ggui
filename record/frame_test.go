package record

import (
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"math"
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

// Each FrameMac style's bar: its height, where the content goes, the
// buttons' centres and the separator, at scale 1 and 2.
func TestFrameStyles(t *testing.T) {
	for _, tc := range []struct {
		style      FrameStyle
		bar, btnY  float64
		separator  bool
		title      titleAt
		hiddenBase bool // TitlebarHidden makes it
	}{
		{FrameMac, 28, 14, true, titleCentred, false},
		{FrameMacTitleless, 28, 14, true, titleNone, false},
		{FrameMacUnified, 52, 26, false, titleLarge, false},
		{FrameMacCompact, 38, 19, false, titleLeading, false},
		{FrameMacExpanded, 66, 14, true, titleCentred, false},
		{FrameMacHidden, 0, 14, false, titleNone, false},
		{FrameMacUnified, 0, 14, false, titleNone, true},
	} {
		for _, s := range []float64{1, 2} {
			tour := Tour{Size: ggui.Sz(400, 300), Scale: s, Frame: tc.style, TitlebarHidden: tc.hiddenBase}
			l := tour.layout(resolve(nil))
			name := fmt.Sprintf("%v (hidden %v) at %g", tc.style, tc.hiddenBase, s)
			m, bar := int(48*s), int(tc.bar*s)
			if want := image.Pt(int(400*s)+2*m, int(300*s)+bar+2*m); l.size != want {
				t.Errorf("%s: size %v, want %v", name, l.size, want)
			}
			if want := image.Rect(m, m+bar, m+int(400*s), m+bar+int(300*s)); l.content != want {
				t.Errorf("%s: content at %v, want %v", name, l.content, want)
			}
			if want := image.Rect(m, m, m+int(400*s), m+bar+int(300*s)); l.window != want {
				t.Errorf("%s: window at %v, want %v", name, l.window, want)
			}
			c, _ := l.buttons()
			for i, x := range []float64{20, 40, 60} {
				if want := (pt{float64(m) + x*s, float64(m) + tc.btnY*s}); c[i] != want {
					t.Errorf("%s: button %d at %v, want %v", name, i, c[i], want)
				}
			}
			if l.separator != tc.separator || l.bar.title != tc.title {
				t.Errorf("%s: separator %v, title %d", name, l.separator, l.bar.title)
			}
			if l.bar.title == titleLeading || l.bar.title == titleLarge {
				if want := m + int(80*s); l.titleX() != want {
					t.Errorf("%s: title at x %d, want %d", name, l.titleX(), want)
				}
			}
		}
	}

	// The Separator option overrides the style's; a hidden bar has none.
	tour := Tour{Size: ggui.Sz(400, 300), Frame: FrameMacUnified}
	if l := tour.layout(resolve([]Option{Separator(true)})); !l.separator {
		t.Error("Separator(true) draws no separator under a unified toolbar")
	}
	tour.Frame = FrameMac
	if l := tour.layout(resolve([]Option{Separator(false)})); l.separator {
		t.Error("Separator(false) draws one under FrameMac's bar")
	}
	tour.Frame = FrameMacHidden
	if l := tour.layout(resolve([]Option{Separator(true)})); l.separator {
		t.Error("a separator under a hidden bar")
	}

	// ButtonInset moves the hidden style's buttons, the option before the
	// tour's, and no other style's.
	for _, tc := range []struct {
		frame FrameStyle
		tour  float64
		opts  []Option
		dx    float64
	}{
		{FrameMacHidden, 8, nil, 8},
		{FrameMacHidden, 8, []Option{ButtonInset(12)}, 12},
		{FrameMacHidden, 8, []Option{ButtonInset(0)}, 0},
		{FrameMac, 8, []Option{ButtonInset(12)}, 0},
	} {
		tour := Tour{Size: ggui.Sz(400, 300), Scale: 2, Frame: tc.frame, ButtonInset: tc.tour}
		l := tour.layout(resolve(append(tc.opts, Margin(0))))
		c, _ := l.buttons()
		if want := (pt{(20 + tc.dx) * 2, (14 + tc.dx) * 2}); c[0] != want {
			t.Errorf("%v, inset %g, %d options: the close button at %v, want %v", tc.frame, tc.tour, len(tc.opts), c[0], want)
		}
	}
}

// A unified toolbar sets its title after the buttons, and the subtitle
// under it; a compact one the title alone, after the buttons; an expanded
// one in the middle of the title row, over a plain toolbar row.
func TestFrameStyleTitles(t *testing.T) {
	for _, tc := range []struct {
		style    FrameStyle
		subtitle string
	}{
		{FrameMacUnified, "3 items"},
		{FrameMacUnified, ""},
		{FrameMacCompact, ""},
		{FrameMacExpanded, ""},
		{FrameMacTitleless, ""},
	} {
		tour := Tour{Size: ggui.Sz(300, 100), Scale: 2, Frame: tc.style}
		l := tour.layout(resolve(nil))
		w, h := l.content.Dx(), l.content.Dy()
		plain := newFramer(l, "", "", resolve(nil)).draw(fill(w, h, white), white, ggui.TextStyle{})
		titled := newFramer(l, "Notes", "", resolve(nil)).draw(fill(w, h, white), white, ggui.TextStyle{})
		diff := changed(plain, titled)
		bar := image.Rect(l.window.Min.X, l.window.Min.Y, l.window.Max.X, l.content.Min.Y)
		c, _ := l.buttons()
		switch tc.style {
		case FrameMacTitleless:
			if !diff.Empty() {
				t.Errorf("%v draws a title at %v", tc.style, diff)
			}
			continue
		case FrameMacExpanded:
			mid := (l.window.Min.X + l.window.Max.X) / 2
			row := image.Rect(bar.Min.X, bar.Min.Y, bar.Max.X, bar.Min.Y+56)
			if diff.Empty() || !diff.In(row) || diff.Min.X > mid || diff.Max.X < mid {
				t.Errorf("%v: the title at %v, not in the middle of the title row %v", tc.style, diff, row)
			}
			continue
		}
		if diff.Empty() || !diff.In(bar) {
			t.Fatalf("%v: the title at %v, not in the bar %v", tc.style, diff, bar)
		}
		if x := l.titleX(); diff.Min.X < x || diff.Min.X > x+6 {
			t.Errorf("%v: the title starts at x %d, not at %d after the buttons", tc.style, diff.Min.X, x)
		}
		// On the buttons' line, give or take the ink's descent.
		if mid := float64(diff.Min.Y+diff.Max.Y) / 2; math.Abs(mid-c[0].y) > 4 {
			t.Errorf("%v: the title's ink is centred at y %g, the buttons at %g", tc.style, mid, c[0].y)
		}
		if tc.subtitle == "" {
			continue
		}
		sub := newFramer(l, "Notes", tc.subtitle, resolve(nil)).draw(fill(w, h, white), white, ggui.TextStyle{})
		under := changed(titled, sub)
		if under.Empty() || !under.In(bar) {
			t.Fatalf("the subtitle at %v, not in the bar %v", under, bar)
		}
		if under.Max.Y <= int(c[0].y) {
			t.Errorf("the subtitle at %v is not under the buttons' line %g", under, c[0].y)
		}
	}
}

func luma(c color.RGBA) int { return (int(c.R)*2126 + int(c.G)*7152 + int(c.B)*722) / 10000 }

// The title bar is a shade of the theme's background, light or dark,
// and is drawn again when the background changes.
func TestFrameFollowsTheme(t *testing.T) {
	tour := Tour{Size: ggui.Sz(200, 100), Scale: 2, Frame: FrameMac}
	l := tour.layout(resolve(nil))
	f := newFramer(l, "", "", resolve(nil))
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
	plain := newFramer(l, "", "", resolve(nil)).draw(fill(600, 200, white), white, ggui.TextStyle{})
	titled := newFramer(l, "Notes", "", resolve(nil)).draw(fill(600, 200, white), white, ggui.TextStyle{})
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

// The request's frame and backdrop are read as ggui record writes them.
func TestParseFrameAndBackdrop(t *testing.T) {
	if c, err := parseBackdrop("#20242a"); err != nil || c != (color.NRGBA{R: 0x20, G: 0x24, B: 0x2a, A: 0xff}) {
		t.Errorf("#20242a: %v, %v", c, err)
	}
	if c, err := parseBackdrop("transparent"); err != nil || c != color.Transparent {
		t.Errorf("transparent: %v, %v", c, err)
	}
	for _, s := range []string{"20242a", "#2024", "#20242g", "+20242a"} {
		if _, err := parseBackdrop(s); err == nil {
			t.Errorf("%q read as a backdrop", s)
		}
	}
	for s, want := range map[string]FrameStyle{
		"none": FrameNone, "mac": FrameMac, "mac-titleless": FrameMacTitleless, "mac-unified": FrameMacUnified,
		"mac-compact": FrameMacCompact, "mac-expanded": FrameMacExpanded, "mac-hidden": FrameMacHidden,
	} {
		if f, err := parseFrame(s); err != nil || f != want || f.String() != s {
			t.Errorf("%s: %v, %v", s, f, err)
		}
	}
	if _, err := parseFrame("round"); err == nil {
		t.Error("a frame named round")
	}
}
