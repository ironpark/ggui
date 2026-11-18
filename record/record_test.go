package record

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ggtest"
	"github.com/ironpark/ggui/widget"
)

// The tests that draw run in ggtest.Main's frame: a tour draws in it, as
// it does in any test that a renderer runs already.
func TestMain(m *testing.M) { ggtest.Main(m) }

func fill(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

var (
	red   = color.RGBA{R: 0xff, A: 0xff}
	blue  = color.RGBA{B: 0xff, A: 0xff}
	white = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
)

func TestChanged(t *testing.T) {
	a, b := fill(10, 8, white), fill(10, 8, white)
	if r := changed(a, b); !r.Empty() {
		t.Fatalf("same images changed %v", r)
	}
	b.SetRGBA(2, 3, red)
	b.SetRGBA(6, 5, red)
	if r, want := changed(a, b), image.Rect(2, 3, 7, 6); r != want {
		t.Fatalf("changed %v, want %v", r, want)
	}
	b = fill(10, 8, white)
	b.SetRGBA(9, 7, blue)
	if r, want := changed(a, b), image.Rect(9, 7, 10, 8); r != want {
		t.Fatalf("changed %v, want %v", r, want)
	}
}

// Kept at 10 of 15 a second, every other frame drawn goes, and the
// delays add up to the time drawn.
func TestPacer(t *testing.T) {
	p := newPacer(15, 10)
	kept := 0
	for range 15 {
		if p.next() {
			kept++
		}
	}
	if kept != 10 {
		t.Fatalf("kept %d of 15, want 10", kept)
	}
	sum := 0
	for k := range 10 {
		sum += p.centis(k+1) - p.centis(k)
	}
	if sum != 100 {
		t.Fatalf("delays add up to %d centiseconds, want 100", sum)
	}
	p = newPacer(15, 0)
	for i := range 15 {
		if !p.next() {
			t.Fatalf("frame %d dropped with no FPS asked", i)
		}
	}
}

// A recording with fewer colors than the palette holds keeps each
// exactly.
func TestPaletteKeepsFewColors(t *testing.T) {
	img := fill(20, 20, white)
	colors := []color.RGBA{red, blue, {R: 0x18, G: 0x18, B: 0x1b, A: 0xff}, {R: 0x3b, G: 0x82, B: 0xf6, A: 0xff}}
	for i, c := range colors {
		for x := range 5 {
			img.SetRGBA(x+5*i%20, i, c)
		}
	}
	h := new(histogram)
	h.add(img, img.Rect)
	p := h.palette(256)
	if len(p) != len(colors)+1 {
		t.Fatalf("palette of %d colors, want %d: %v", len(p), len(colors)+1, p)
	}
	for _, c := range append(colors, white) {
		if p[p.Index(c)] != color.Color(c) {
			t.Errorf("%v became %v", c, p[p.Index(c)])
		}
	}
}

// A gradient through more colors than the palette has is matched
// closely: the colors spread over it.
func TestPaletteGradient(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 256, 4))
	for x := range 256 {
		for y := range 4 {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(x / 2), B: 0x80, A: 0xff})
		}
	}
	h := new(histogram)
	h.add(img, img.Rect)
	p := h.palette(32)
	if len(p) != 32 {
		t.Fatalf("palette of %d colors, want 32", len(p))
	}
	m := newMapper(p, 0)
	worst := 0
	for x := range 256 {
		c := img.RGBAAt(x, 0)
		r, _, _, _ := p[m.index(c.R, c.G, c.B, x, 0)].RGBA()
		worst = max(worst, abs(int(r>>8)-int(c.R)))
	}
	if worst > 8 {
		t.Fatalf("a gradient of 256 steps in 32 colors is off by up to %d", worst)
	}
}

func abs(v int) int { return max(v, -v) }

// Past maxExact colors the histogram bins them, and still fills the
// palette.
func TestPaletteManyColors(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 512, 512))
	for y := range 512 {
		for x := range 512 {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x>>8 | y>>8<<1), A: 0xff})
		}
	}
	h := new(histogram)
	h.add(img, img.Rect)
	if h.bins == nil {
		t.Fatal("262144 colors were counted one by one")
	}
	if p := h.palette(256); len(p) != 256 {
		t.Fatalf("a palette of %d colors, want 256", len(p))
	}
}

func TestScaler(t *testing.T) {
	// Blocks of 2×2 halve into single pixels of their color.
	img := fill(4, 4, white)
	for y := range 2 {
		for x := range 2 {
			img.SetRGBA(x, y, red)
			img.SetRGBA(x+2, y+2, blue)
		}
	}
	s := newScaler(4, 4, 2)
	out := s.scale(img)
	if out.Rect.Dx() != 2 || out.Rect.Dy() != 2 {
		t.Fatalf("scaled to %v", out.Rect)
	}
	if out.RGBAAt(0, 0) != red || out.RGBAAt(1, 1) != blue || out.RGBAAt(1, 0) != white {
		t.Fatalf("scaled pixels %v %v %v", out.RGBAAt(0, 0), out.RGBAAt(1, 1), out.RGBAAt(1, 0))
	}
	// Three columns into two: the middle one is shared half and half.
	img = fill(3, 1, white)
	img.SetRGBA(1, 0, color.RGBA{A: 0xff})
	out = newScaler(3, 1, 2).scale(img)
	if got := out.RGBAAt(0, 0).R; got < 167 || got > 172 {
		t.Fatalf("a third of black over white reads %d", got)
	}
	if newScaler(10, 10, 0) != nil || newScaler(10, 10, 20) != nil {
		t.Fatal("a scaler that keeps the size was made")
	}
}

// The GIF sink keeps the changed rectangles alone, merges frames that
// changed nothing into the one before, and decodes to what it was given.
func TestGIFSink(t *testing.T) {
	o := resolve(nil)
	s, err := newGIFSink(40, 30, o, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	frames := []*image.RGBA{fill(40, 30, white), fill(40, 30, white), fill(40, 30, white)}
	frames[1].SetRGBA(5, 5, red)
	frames[2].SetRGBA(5, 5, red)
	for y := 20; y < 25; y++ {
		for x := 30; x < 38; x++ {
			frames[2].SetRGBA(x, y, color.RGBA{R: uint8(x * 6), G: uint8(y * 9), B: 0x40, A: 0xff})
		}
	}
	for _, f := range []*image.RGBA{frames[0], frames[0], frames[1], frames[2], frames[2], frames[2]} {
		if err := s.add(f); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	res, err := s.encode(&buf, o)
	if err != nil {
		t.Fatal(err)
	}
	if res.frames != 3 || res.duration != 600*time.Millisecond {
		t.Fatalf("wrote %d frames of %v, want 3 of 600ms", res.frames, res.duration)
	}
	g, err := gif.DecodeAll(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) != 3 || g.Delay[0] != 20 || g.Delay[1] != 10 || g.Delay[2] != 30 {
		t.Fatalf("decoded %d frames, delays %v", len(g.Image), g.Delay)
	}
	if g.LoopCount != 0 {
		t.Errorf("loop count %d, want 0 (for ever)", g.LoopCount)
	}
	if r := g.Image[1].Rect; r != image.Rect(5, 5, 6, 6) {
		t.Errorf("the second frame covers %v", r)
	}
	if r := g.Image[2].Rect; r != image.Rect(30, 20, 38, 25) {
		t.Errorf("the third frame covers %v", r)
	}
	// Composed, the last frame is the image given.
	out := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for _, f := range g.Image {
		for y := f.Rect.Min.Y; y < f.Rect.Max.Y; y++ {
			for x := f.Rect.Min.X; x < f.Rect.Max.X; x++ {
				out.Set(x, y, f.At(x, y))
			}
		}
	}
	for y := range 30 {
		for x := range 40 {
			if !near(out.RGBAAt(x, y), frames[2].RGBAAt(x, y), 2) {
				t.Fatalf("(%d,%d) decoded %v, was %v", x, y, out.RGBAAt(x, y), frames[2].RGBAAt(x, y))
			}
		}
	}
}

func TestGIFLoopAndColors(t *testing.T) {
	o := resolve([]Option{Loop(1), Colors(2)})
	s, err := newGIFSink(4, 4, o, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	if err := s.add(fill(4, 4, red)); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := s.encode(&buf, o); err != nil {
		t.Fatal(err)
	}
	g, err := gif.DecodeAll(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if g.LoopCount != -1 {
		t.Errorf("played once, loop count %d, want -1", g.LoopCount)
	}
	if n := len(g.Config.ColorModel.(color.Palette)); n != 2 {
		t.Errorf("a palette of %d colors", n)
	}
}

func near(a, b color.RGBA, tol int) bool {
	d := func(x, y uint8) bool { return abs(int(x)-int(y)) <= tol }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B)
}

// The request and the report are what ggui record writes and reads; its
// test checks the same strings (cmd/ggui/cmd_record_test.go).
const (
	goldenRequest = `{"v":1,"tour":"intro","format":"gif","out":"/tmp/tour.gif","fps":10,"width":640,"colors":128,"dither":0.25,"loop":2,"frame":"mac","backdrop":"#20242a","no_cursor":true,"report":"/tmp/report.json"}`
	goldenReport  = `{"v":1,"tours":["","intro"],"frames":42,"duration_ms":4200,"width":640,"height":400}`
)

func TestProtocolGolden(t *testing.T) {
	req := request{V: 1, Tour: "intro", Format: "gif", Out: "/tmp/tour.gif", FPS: 10, Width: 640, Colors: 128, Dither: 0.25, Loop: 2, Frame: "mac", Backdrop: "#20242a", NoCursor: true, Report: "/tmp/report.json"}
	if got, _ := json.Marshal(req); string(got) != goldenRequest {
		t.Errorf("request\n%s\nwant\n%s", got, goldenRequest)
	}
	var back request
	if err := json.Unmarshal([]byte(goldenRequest), &back); err != nil || back != req {
		t.Errorf("read back %+v, %v", back, err)
	}
	rep := report{V: 1, Tours: []string{"", "intro"}, Frames: 42, DurationMS: 4200, Width: 640, Height: 400}
	if got, _ := json.Marshal(rep); string(got) != goldenReport {
		t.Errorf("report\n%s\nwant\n%s", got, goldenReport)
	}
}

// boxTour shows a box that turns from red to blue, under a line of text,
// at 10 frames a second: 3 frames red, then 3 blue.
func boxTour(name string) Tour {
	col := ggui.State(color.Color(red))
	return Tour{
		Name: name,
		Size: ggui.Sz(60, 40),
		FPS:  10,
		Build: func() ggui.Widget {
			return widget.Column(
				ggui.View(col, func(c color.Color) ggui.Widget { return widget.Box().Size(60, 20).Fill(c) }),
				widget.Text("hello"),
			)
		},
		Setup: func(*ggui.Probe) {
			ggui.SetEnv(ggui.Untrack(ggui.UseEnv).With(ggui.BackgroundKey, color.Color(white)))
		},
		Script: func(r *Recorder) {
			r.Hold(300 * time.Millisecond)
			r.Then(300*time.Millisecond, func() { col.Set(blue) })
		},
	}
}

func TestWriteGIF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tour.gif")
	res, err := boxTour("box").writeGIF(path, resolve(nil))
	if err != nil {
		t.Fatal(err)
	}
	if res.duration != 600*time.Millisecond || res.w != 60 || res.h != 40 {
		t.Fatalf("wrote %+v", res)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	g, err := gif.DecodeAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if g.Config.Width != 60 || g.Config.Height != 40 || len(g.Image) != 2 {
		t.Fatalf("a GIF of %d×%d with %d frames, want 60×40 with 2", g.Config.Width, g.Config.Height, len(g.Image))
	}
	if g.Delay[0] != 30 || g.Delay[1] != 30 {
		t.Errorf("delays %v, want 30 and 30", g.Delay)
	}
	first := g.Image[0]
	if !near(rgba(first.At(30, 10)), red, 8) {
		t.Errorf("the box starts %v", first.At(30, 10))
	}
	if !near(rgba(first.At(59, 39)), white, 8) {
		t.Errorf("the background is %v", first.At(59, 39))
	}
	// The text drew something dark below the box.
	dark := false
	for y := 20; y < 40; y++ {
		for x := range 60 {
			if c := rgba(first.At(x, y)); c.R < 0x80 {
				dark = true
			}
		}
	}
	if !dark {
		t.Error("no text was drawn")
	}
	second := g.Image[1]
	if !second.Rect.In(image.Rect(0, 0, 60, 20)) {
		t.Errorf("the second frame covers %v, more than the box", second.Rect)
	}
	if !near(rgba(second.At(30, 10)), blue, 8) {
		t.Errorf("the box turns %v", second.At(30, 10))
	}
}

func TestWritePNGs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "frames")
	res, err := boxTour("box").writePNGs(dir, resolve([]Option{FPS(5), Width(30), Background(blue)}))
	if err != nil {
		t.Fatal(err)
	}
	if res.frames != 3 || res.w != 30 || res.h != 20 {
		t.Fatalf("wrote %+v, want 3 frames of 30×20", res)
	}
	names, _ := filepath.Glob(filepath.Join(dir, "*.png"))
	if len(names) != 3 || filepath.Base(names[0]) != "0000.png" {
		t.Fatalf("wrote %v", names)
	}
	var imgs []image.Image
	for _, n := range names {
		data, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		imgs = append(imgs, img)
	}
	if b := imgs[0].Bounds(); b.Dx() != 30 || b.Dy() != 20 {
		t.Fatalf("a frame of %v", b)
	}
	if !near(rgba(imgs[0].At(15, 5)), red, 8) || !near(rgba(imgs[2].At(15, 5)), blue, 8) {
		t.Errorf("the box is %v, then %v", imgs[0].At(15, 5), imgs[2].At(15, 5))
	}
	// The Background option replaced the Env's white.
	if !near(rgba(imgs[0].At(29, 19)), blue, 8) {
		t.Errorf("the background is %v", imgs[0].At(29, 19))
	}
}

func rgba(c color.Color) color.RGBA {
	r, g, b, a := c.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

// A script that fails ends the recording with its error, and nothing is
// left at the path.
func TestScriptPanic(t *testing.T) {
	tour := boxTour("")
	tour.Script = func(r *Recorder) {
		r.Hold(100 * time.Millisecond)
		r.Probe().Tap("no such button")
	}
	path := filepath.Join(t.TempDir(), "tour.gif")
	err := tour.WriteGIF(path)
	if err == nil || !strings.Contains(err.Error(), "no such button") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("a file was left behind")
	}
}

func TestServe(t *testing.T) {
	Register(boxTour("served"))
	dir := t.TempDir()
	rep := filepath.Join(dir, "report.json")
	readReport := func() report {
		t.Helper()
		data, err := os.ReadFile(rep)
		if err != nil {
			t.Fatal(err)
		}
		var r report
		if err := json.Unmarshal(data, &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	ask := func(r request) (int, error) {
		r.V = protocol
		data, _ := json.Marshal(r)
		var out bytes.Buffer
		return serveTo(string(data), &out)
	}

	if code, err := ask(request{Format: "list", Report: rep}); code != 0 || err != nil {
		t.Fatalf("list: %d %v", code, err)
	}
	if r := readReport(); len(r.Tours) != 1 || r.Tours[0] != "served" {
		t.Fatalf("listed %q", r.Tours)
	}

	code, err := ask(request{Tour: "nope", Format: "gif", Out: filepath.Join(dir, "x.gif")})
	if code != 2 || err == nil || !strings.Contains(err.Error(), `no tour named "nope"; it has "served"`) {
		t.Fatalf("an unknown tour: %d %v", code, err)
	}

	// The one tour registered is the default.
	out := filepath.Join(dir, "tour.gif")
	if code, err := ask(request{Format: "gif", Out: out, Report: rep, FPS: 5}); code != 0 || err != nil {
		t.Fatalf("gif: %d %v", code, err)
	}
	if r := readReport(); r.Frames != 2 || r.DurationMS != 600 || r.Width != 60 {
		t.Fatalf("reported %+v", r)
	}

	// Framed, with the default margin and title bar.
	if code, err := ask(request{Format: "gif", Out: out, Report: rep, Frame: "mac", Backdrop: "#ffffff"}); code != 0 || err != nil {
		t.Fatalf("framed: %d %v", code, err)
	}
	if r := readReport(); r.Width != 60+96 || r.Height != 40+28+96 {
		t.Fatalf("framed: reported %+v", r)
	}
	if code, err := ask(request{Format: "gif", Out: out, Frame: "round"}); code != 2 || err == nil {
		t.Fatalf("a frame named round: %d %v", code, err)
	}

	if code, err := serveTo(`{"v":99}`, &bytes.Buffer{}); code != 2 || err == nil || !strings.Contains(err.Error(), "version 99") {
		t.Fatalf("another version: %d %v", code, err)
	}
}
