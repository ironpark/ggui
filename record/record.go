// Package record draws scripted walks through an app, tours, into an
// animated GIF or numbered PNG frames, for a README or a release note. No
// screen is captured: the tour's tree runs in a Probe whose clock is
// virtual, and each frame is painted offscreen, so a recording is as fast
// as drawing, the same on every run, and needs no display.
//
// An app registers its tours, from an init function or main before
// ggui.Run:
//
//	record.Register(record.Tour{
//		Size:  ggui.Sz(800, 500),
//		Build: func() ggui.Widget { return ui.Build(m) },
//		Setup: func(p *ggui.Probe) { m = ui.NewModel(demoData()) },
//		Script: func(r *record.Recorder) {
//			r.Hold(time.Second)
//			r.Then(1500*time.Millisecond, func() { r.Probe().Tap("Settings") })
//		},
//	})
//
// and ggui record builds the app and runs it to draw one: ggui.Run sees
// that it was started to record, draws the tour and exits instead of
// opening a window. A program can also draw a tour itself with
// Tour.WriteGIF or Tour.WritePNGs. See docs/recording.md.
//
// Drawing needs the renderer, which a tour brings up with no window, on
// the main thread: call WriteGIF, WritePNGs and Main from the main
// goroutine, as ggui.Run is called, before any app runs. The renderer
// stays up for the rest of the process, which then cannot run an app; a
// process that records exits when it is done. A test run by ggtest.Main
// is already inside a frame of the renderer, and a tour draws in it.
package record

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/ironpark/ggfx"

	"github.com/ironpark/ggui"
)

// A Tour is a scripted walk through an app, drawn frame by frame with no
// screen captured.
type Tour struct {
	// Name is what ggui record NAME selects. "" is the default tour,
	// which ggui record draws when no name is given.
	Name string
	// Size is the logical size of the window drawn.
	Size ggui.Size
	// Scale is the device pixels drawn per logical pixel, as a HiDPI
	// display's: 2 draws a 400×300 tour into 800×600 pixels. Zero means 1.
	Scale float64
	// FPS is how many frames are drawn per second of tour time. Zero
	// means 15. The files written can keep fewer (see the FPS option).
	FPS int
	// Start is the time the tour's clock reads at its first frame. The
	// zero Time means noon on 1 January 2026, local time, so that two
	// runs draw the same frames.
	Start time.Time
	// Setup, if set, runs under the probe's root owner before the tree
	// is first built, as App.Setup does: themes, the environment and the
	// demo state the tour shows are set up here.
	Setup func(p *ggui.Probe)
	// Build returns the root of the tree drawn. It is required.
	Build ggui.Builder
	// Frame is what is drawn around the app: FrameNone, the default,
	// draws the content alone; FrameMac a macOS window around it. The
	// Frame option overrides it.
	Frame FrameStyle
	// Title is the window's title, which FrameMac sets in its title bar,
	// as the app's Config.Title would be.
	Title string
	// TitlebarHidden draws FrameMac as for a window whose Config sets
	// TitlebarHidden: no title bar, the content up to the window's top
	// edge and the window buttons over it.
	TitlebarHidden bool
	// Script walks through the app, a step at a time, with Hold and
	// Then. It is required. A panic in it, as Probe.Tap's when nothing
	// has the label, ends the recording with an error.
	Script func(r *Recorder)
}

// DefaultStart is the time a tour starts at when its Start is zero.
var DefaultStart = time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)

// defaultFPS is the frames drawn per second when Tour.FPS is zero.
const defaultFPS = 15

var (
	mu    sync.Mutex
	tours []Tour
)

// Register adds t to the tours ggui record can draw of this program. Call
// it from an init function or from main before ggui.Run. It panics when a
// tour of the same name is registered already, or when t has no Build or
// Script.
func Register(t Tour) {
	if err := t.check(); err != nil {
		panic(err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, o := range tours {
		if o.Name == t.Name {
			panic(fmt.Sprintf("record: a tour named %q is registered already", t.Name))
		}
	}
	tours = append(tours, t)
}

// Lookup returns the registered tour called name. The name "" finds the
// default tour, or the one tour registered when there is only one.
func Lookup(name string) (Tour, bool) {
	mu.Lock()
	defer mu.Unlock()
	for _, t := range tours {
		if t.Name == name {
			return t, true
		}
	}
	if name == "" && len(tours) == 1 {
		return tours[0], true
	}
	return Tour{}, false
}

// Names returns the names of the registered tours, sorted.
func Names() []string {
	mu.Lock()
	defer mu.Unlock()
	names := make([]string, len(tours))
	for i, t := range tours {
		names[i] = t.Name
	}
	slices.Sort(names)
	return names
}

func (t Tour) check() error {
	switch {
	case t.Build == nil:
		return fmt.Errorf("record: tour %q has no Build", t.Name)
	case t.Script == nil:
		return fmt.Errorf("record: tour %q has no Script", t.Name)
	case t.Size.W <= 0 || t.Size.H <= 0:
		return fmt.Errorf("record: tour %q has no Size", t.Name)
	case t.Scale < 0 || t.FPS < 0:
		return fmt.Errorf("record: tour %q has a negative Scale or FPS", t.Name)
	}
	return nil
}

func (t Tour) fps() int {
	if t.FPS == 0 {
		return defaultFPS
	}
	return t.FPS
}

func (t Tour) scale() float64 {
	if t.Scale == 0 {
		return 1
	}
	return t.Scale
}

// pixels returns the size the tour is drawn at, in device pixels.
func (t Tour) pixels() (w, h int) {
	s := t.scale()
	return max(int(math.Ceil(t.Size.W*s)), 1), max(int(math.Ceil(t.Size.H*s)), 1)
}

// A Recorder runs a tour's Script: Hold and Then draw its frames, and
// Probe drives the app between them.
type Recorder struct {
	p    *ggui.Probe
	dev  device
	step time.Duration
	fps  int
	bg   color.Color // the Background option's, or nil for the Env's
	img  *ggfx.Image
	pix  *image.RGBA
	sink func(*image.RGBA) error

	lay   layout
	frame *framer // nil for FrameNone

	frames int
	err    error
}

// Probe returns the probe the tour runs in, to click, type, scroll and
// find with. Its input methods run frames that are not drawn; what they
// change shows in the frames Hold draws next.
func (r *Recorder) Probe() *ggui.Probe { return r.p }

// Hold shows what is there for d: it draws a frame for each 1/FPS of d,
// rounded up, moving the clock on by as much before each, so that
// animations play as they would on a screen.
func (r *Recorder) Hold(d time.Duration) {
	// Counted in whole frames: 1/FPS is not a whole number of
	// nanoseconds, and a second must be FPS frames, not one more.
	n := (int64(d)*int64(r.fps) + int64(time.Second) - 1) / int64(time.Second)
	for range n {
		r.shoot()
	}
}

// Then does step, lays the tree out again, and holds what follows from it
// for d: Then(d, step) is a step of a tour and how long it is watched.
func (r *Recorder) Then(d time.Duration, step func()) {
	step()
	r.p.Frame()
	r.Hold(d)
}

// Elapsed returns the tour time drawn so far.
func (r *Recorder) Elapsed() time.Duration {
	return time.Duration(r.frames) * time.Second / time.Duration(r.fps)
}

// abort unwinds a Script when a frame could not be written.
type abort struct{ err error }

// shoot moves the clock on a frame and draws it to the sink.
func (r *Recorder) shoot() {
	if r.err != nil {
		panic(abort{r.err})
	}
	r.p.Advance(r.step)
	env := ggui.Untrack(ggui.UseEnv)
	bg := r.bg
	if bg == nil {
		bg = env.Background()
	}
	err := r.dev.frame(func() error {
		r.img.Fill(opaque(bg))
		r.p.Draw(r.img)
		r.img.ReadPixels(r.pix.Pix)
		return nil
	})
	if err == nil {
		// What was drawn over an opaque background is opaque; the alpha
		// a driver leaves can be a bit off it.
		for i := 3; i < len(r.pix.Pix); i += 4 {
			r.pix.Pix[i] = 0xff
		}
		out := r.pix
		if r.frame != nil {
			out = r.frame.draw(r.pix, bg, env.Text())
		}
		err = r.sink(out)
	}
	if err != nil {
		r.err = err
		panic(abort{err})
	}
	r.frames++
}

// opaque returns c over white, as a GIF or a screen shows it.
func opaque(c color.Color) color.Color {
	r, g, b, a := c.RGBA()
	if a == 0xffff {
		return c
	}
	w := 0xffff - a
	return color.RGBA64{R: uint16(r + w), G: uint16(g + w), B: uint16(b + w), A: 0xffff}
}

// play runs t's script, handing every frame drawn to sink, at the tour's
// size in device pixels and FPS.
func (t Tour) play(o *settings, sink func(*image.RGBA) error) (err error) {
	if err := t.check(); err != nil {
		return err
	}
	dev, err := openDevice()
	if err != nil {
		return err
	}
	w, h := t.pixels()
	start := t.Start
	if start.IsZero() {
		start = DefaultStart
	}
	r := &Recorder{
		dev:  dev,
		step: time.Second / time.Duration(t.fps()),
		fps:  t.fps(),
		bg:   o.background,
		pix:  image.NewRGBA(image.Rect(0, 0, w, h)),
		sink: sink,
		lay:  t.layout(o),
	}
	if r.lay.style == FrameMac {
		r.frame = newFramer(r.lay, t.Title, o)
	}
	r.img = ggfx.NewImage(w, h)
	defer r.img.Deallocate()

	p := ggui.ProbeBuilder(t.Build, t.Size)
	r.p = p
	p.SetScale(t.scale())
	p.Setup(func() {
		// Setup runs with the probe's loop current, so the clock set is
		// the probe's own.
		ggui.SetClock(func() time.Time { return start })
		if t.Setup != nil {
			t.Setup(p)
		}
	})
	defer p.Close()

	defer func() {
		v := recover()
		if v == nil {
			return
		}
		if a, ok := v.(abort); ok {
			err = a.err
			return
		}
		if e, ok := v.(error); ok {
			err = fmt.Errorf("record: tour %q: %w", t.Name, e)
			return
		}
		err = fmt.Errorf("record: tour %q: %v", t.Name, v)
	}()
	p.Frame()
	t.Script(r)
	if r.frames == 0 {
		return fmt.Errorf("record: tour %q drew no frames: its Script never holds", t.Name)
	}
	return r.err
}
