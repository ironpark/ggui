package record

import (
	"image"
	"image/color"
	"image/draw"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/widget"
)

// movingTour moves the cursor over a still app, clicks, and switches its
// background: changes small and large, over a frame.
func movingTour() Tour {
	bg := ggui.State(color.Color(white))
	return Tour{
		Size:  ggui.Sz(160, 100),
		Scale: 2,
		FPS:   20,
		Frame: FrameMac,
		Title: "Moving",
		Build: func() ggui.Widget {
			return ggui.View(bg, func(c color.Color) ggui.Widget {
				return widget.Box(widget.Column(widget.Text("one"), widget.Text("two"))).Fill(c).Padding(ggui.Insets(12))
			})
		},
		Setup: func(*ggui.Probe) {
			ggui.SetEnv(ggui.Untrack(ggui.UseEnv).With(ggui.BackgroundKey, color.Color(white)))
		},
		Script: func(r *Recorder) {
			r.Hold(100 * time.Millisecond)
			r.MoveTo(ggui.Pt(30, 20), 300*time.Millisecond)
			r.Click()
			r.MoveTo(ggui.Pt(140, 90), 400*time.Millisecond)
			r.Then(200*time.Millisecond, func() { bg.Set(color.RGBA{R: 0x20, G: 0x22, B: 0x28, A: 0xff}) })
		},
	}
}

// timeline decodes a GIF into what it shows each hundredth of a second.
func timeline(t *testing.T, path string) []*image.RGBA {
	t.Helper()
	g := readGIF(t, path)
	screen := image.NewRGBA(image.Rect(0, 0, g.Config.Width, g.Config.Height))
	var out []*image.RGBA
	for i, f := range g.Image {
		draw.Draw(screen, f.Bounds(), f, f.Bounds().Min, draw.Over)
		for range g.Delay[i] {
			out = append(out, screen)
		}
		next := image.NewRGBA(screen.Rect)
		copy(next.Pix, screen.Pix)
		screen = next
	}
	return out
}

// Level 1 writes a GIF that shows exactly what level 0's does, in fewer
// bytes.
func TestOptimizeShowsTheSame(t *testing.T) {
	dir := t.TempDir()
	plain, opt := filepath.Join(dir, "0.gif"), filepath.Join(dir, "1.gif")
	if err := movingTour().WriteGIF(plain, Optimize(0)); err != nil {
		t.Fatal(err)
	}
	if err := movingTour().WriteGIF(opt); err != nil {
		t.Fatal(err)
	}
	a, b := timeline(t, plain), timeline(t, opt)
	if len(a) != len(b) {
		t.Fatalf("%d hundredths of a second, and %d optimized", len(a), len(b))
	}
	for i := range a {
		if r := changed(a[i], b[i]); !r.Empty() {
			t.Fatalf("at %d hundredths, the optimized GIF differs in %v", i, r)
		}
	}
	sa, _ := os.Stat(plain)
	sb, _ := os.Stat(opt)
	if sb.Size() >= sa.Size() {
		t.Errorf("optimized: %d bytes, not fewer than %d", sb.Size(), sa.Size())
	}
	t.Logf("%d bytes at level 0, %d at level 1", sa.Size(), sb.Size())
}

// Level 2 splits frames whose changes are far apart: it shows what level
// 1 does but in the 20 ms the first part of a split frame shows, and is
// smaller.
func TestOptimizeSplits(t *testing.T) {
	dir := t.TempDir()
	one, two := filepath.Join(dir, "1.gif"), filepath.Join(dir, "2.gif")
	tour := func() Tour {
		col := ggui.State(color.Color(red))
		return Tour{
			Size:  ggui.Sz(160, 100),
			Scale: 2,
			FPS:   15,
			Build: func() ggui.Widget {
				return ggui.View(col, func(c color.Color) ggui.Widget {
					return widget.Column(widget.Box().Size(20, 20).Fill(c))
				})
			},
			Setup: func(*ggui.Probe) {
				ggui.SetEnv(ggui.Untrack(ggui.UseEnv).With(ggui.BackgroundKey, color.Color(white)))
			},
			Script: func(r *Recorder) {
				// The cursor in one corner and a change in the other,
				// each frame.
				r.MoveTo(ggui.Pt(150, 90), 0)
				r.Hold(100 * time.Millisecond)
				for i := range 8 {
					col.Set(color.RGBA{R: uint8(30 * i), G: 0x40, B: 0x80, A: 0xff})
					r.MoveTo(ggui.Pt(150-float64(i)*3, 90-float64(i)*2), 70*time.Millisecond)
				}
			},
		}
	}
	if err := tour().WriteGIF(one); err != nil {
		t.Fatal(err)
	}
	if err := tour().WriteGIF(two, Optimize(2)); err != nil {
		t.Fatal(err)
	}
	a := timeline(t, one)
	g := readGIF(t, two)
	b := timeline(t, two)
	if len(a) != len(b) {
		t.Fatalf("%d hundredths of a second, and %d split", len(a), len(b))
	}
	lead := make([]bool, len(b))
	at, splits := 0, 0
	for i, d := range g.Delay {
		if d == splitLead && i+1 < len(g.Delay) {
			lead[at], lead[at+1] = true, true
			splits++
		}
		at += d
	}
	for i := range a {
		if !lead[i] && !changed(a[i], b[i]).Empty() {
			t.Fatalf("at %d hundredths the split GIF differs", i)
		}
	}
	sa, _ := os.Stat(one)
	sb, _ := os.Stat(two)
	t.Logf("%d bytes at level 1, %d at level 2, %d frames split", sa.Size(), sb.Size(), splits)
	if splits == 0 || sb.Size() >= sa.Size() {
		t.Errorf("level 2 split %d frames into %d bytes, from %d", splits, sb.Size(), sa.Size())
	}
}

// A lossy GIF keeps every pixel within its limit of the exact one, and
// is smaller.
func TestLossy(t *testing.T) {
	dir := t.TempDir()
	exact, lossy := filepath.Join(dir, "exact.gif"), filepath.Join(dir, "lossy.gif")
	if err := movingTour().WriteGIF(exact); err != nil {
		t.Fatal(err)
	}
	if err := movingTour().WriteGIF(lossy, Lossy(40)); err != nil {
		t.Fatal(err)
	}
	a, b := timeline(t, exact), timeline(t, lossy)
	if len(a) != len(b) {
		t.Fatalf("%d hundredths of a second, and %d lossy", len(a), len(b))
	}
	worst := 0.0
	for i := range a {
		for p := 0; p < len(a[i].Pix); p += 4 {
			x, y := a[i].Pix[p:p+3], b[i].Pix[p:p+3]
			d := labDist(oklab(float64(x[0]), float64(x[1]), float64(x[2])), oklab(float64(y[0]), float64(y[1]), float64(y[2])))
			worst = max(worst, d)
		}
	}
	if worst > 0.040001 {
		t.Errorf("a pixel is %.4f from the exact one, past Lossy(40)'s 0.04", worst)
	}
	sa, _ := os.Stat(exact)
	sb, _ := os.Stat(lossy)
	if sb.Size() >= sa.Size() {
		t.Errorf("lossy: %d bytes, not fewer than %d", sb.Size(), sa.Size())
	}
	t.Logf("%d bytes exact, %d lossy", sa.Size(), sb.Size())
}

// The encoder decodes back to its input, with and without a transparent
// index, and writes what compress/lzw writes when it has no choice.
func TestLZWRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	const w, h = 97, 61
	want := make([]byte, w*h)
	under := make([]byte, w*h)
	for i := range want {
		want[i] = byte(rng.IntN(6))
		under[i] = want[i]
		if rng.IntN(3) == 0 {
			under[i] = byte(rng.IntN(6))
		}
	}
	for _, clear := range []int{-1, 7} {
		chosen := make([]byte, len(want))
		var e lzwEncoder
		var data []byte
		buf := byteBuf{&data}
		if err := e.encode(buf, 3, frameCodes{want: want, under: under, clear: clear, chosen: chosen}); err != nil {
			t.Fatal(err)
		}
		for i, c := range chosen {
			shown := c
			if int(c) == clear {
				shown = under[i]
			}
			if shown != want[i] {
				t.Fatalf("clear %d: pixel %d shows %d, want %d", clear, i, shown, want[i])
			}
		}
		got := decodeLZW(t, data, 3, len(want))
		if string(got) != string(chosen) {
			t.Fatalf("clear %d: decoded differently from what was chosen", clear)
		}
	}
}

type byteBuf struct{ b *[]byte }

func (b byteBuf) WriteByte(c byte) error { *b.b = append(*b.b, c); return nil }

// decodeLZW decodes data as a GIF's image data, with lit-bit literals,
// through image/gif.
func decodeLZW(t *testing.T, data []byte, lit, n int) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.gif")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	pal := make(color.Palette, 1<<lit)
	for i := range pal {
		pal[i] = color.RGBA{R: uint8(i), A: 0xff}
	}
	g := newGIFWriter(f, image.Pt(n, 1), pal, 1)
	if err := g.frame(image.Rect(0, 0, n, 1), lit, data, 10, -1); err != nil {
		t.Fatal(err)
	}
	if err := g.close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	img := readGIF(t, path).Image[0]
	return img.Pix[:n]
}

// The flat colors of a UI keep entries of their own in a palette too
// small for every color, rather than being averaged with a gradient's.
func TestPaletteKeepsFlatColors(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 512, 512))
	flat := []color.RGBA{{R: 0xf6, G: 0xf6, B: 0xf7, A: 0xff}, {R: 0x80, G: 0x90, B: 0x44, A: 0xff}, {R: 0x30, G: 0xa0, B: 0x40, A: 0xff}}
	for y := range 512 {
		for x := range 512 {
			c := color.RGBA{R: uint8(x / 2), G: uint8(y / 2), B: uint8((x + y) / 4), A: 0xff}
			switch {
			case y < 120:
				c = flat[0]
			case y < 140 && x < 40:
				c = flat[1]
			case y < 160 && x < 40:
				c = flat[2]
			}
			img.SetRGBA(x, y, c)
		}
	}
	h := new(histogram)
	h.add(img, img.Rect)
	p := h.palette(32)
	for _, c := range flat {
		if p[p.Index(c)] != color.Color(c) {
			t.Errorf("%v became %v", c, p[p.Index(c)])
		}
	}
}
