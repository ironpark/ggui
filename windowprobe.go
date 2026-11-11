package ggui

import (
	"image"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/runtime"
	"github.com/ironpark/ggwd"
)

// WindowProbe runs a widget tree through a Window's own frames with no
// native window, for a test about what a window does with no input coming:
// whether a frame is skipped, painted in part or within a scroll's
// viewport, or painted whole, and whether the window asks for the next
// one. A Probe cannot tell: it paints every frame whole and runs one
// whenever it is told, so a widget that moves on only because the window
// asks for its next frame, as a disclosure easing open does, passes under
// a Probe and stands still in a window.
//
// Each frame is the one a window's FrameEvent runs, on a screen its driver
// keeps from frame to frame, as Metal's is, at one pixel per logical pixel.
// Input queued with Move, Press and the rest reaches it as the platform's
// events would, gathered until the frame; a signal written between frames
// is a write the window sees at its next one.
//
//	p := ggui.NewWindowProbe(func() ggui.Widget { return ui.Collapsible(open, "More", body) }, ggui.Sz(300, 200))
//	defer p.Close()
//	p.Step(0)
//	open.Set(true)
//	frames := p.Run(16*time.Millisecond, 60)
//	// frames[len(frames)-1].Next is false: the window came to rest
//
// What marks a frame whole includes an event handed to any widget in the
// process, as the one UI thread of an App would hand it; a test asserting
// how frames are painted runs without t.Parallel, beside no other probe.
type WindowProbe struct {
	w      *Window
	screen *ggfx.Image
	damage []Rect      // what the frame being run gave setDamage last
	frames int         // run so far
	last   WindowFrame // what the last frame did, for Idle
}

// FramePaint is how a window painted a frame.
type FramePaint uint8

// The ways a frame is painted; see damage.go.
const (
	// FrameSkipped painted nothing: nothing changed, and the screen keeps
	// the last frame.
	FrameSkipped FramePaint = iota
	// FramePartial painted over the last frame where widgets that read the
	// clock, widgets that read the pointer as it moved, or regions
	// RepaintRect asked for, are, and registered nothing.
	FramePartial
	// FrameContained laid out and painted the tree as a whole frame does,
	// into the viewports of scrolls that held every change alone.
	FrameContained
	// FrameWhole painted all of the window.
	FrameWhole
)

func (p FramePaint) String() string {
	switch p {
	case FrameSkipped:
		return "skipped"
	case FramePartial:
		return "partial"
	case FrameContained:
		return "contained"
	case FrameWhole:
		return "whole"
	}
	return "FramePaint(?)"
}

// WindowFrame is what one of a WindowProbe's frames did.
type WindowFrame struct {
	// At is the instant the frame's widgets read.
	At time.Time
	// Painted is how the frame was painted.
	Painted FramePaint
	// Damage is the part of the window the frame painted, in logical
	// pixels: all of it for a whole frame, nothing for a skipped one.
	Damage []Rect
	// Next is whether the window asked for the next frame, with no input
	// coming. A window at rest asks for none.
	Next bool
	// Wake is the instant a widget asked to be painted again at, as a
	// caret blinking does, when the window asked for no next frame; zero
	// when none did.
	Wake time.Time
}

// NewWindowProbe makes a window of size, not open, that shows the tree
// build makes. build runs at the first frame, under a root owner of its
// own, after the functions Window().Setup registered.
func NewWindowProbe(build Builder, size Size) *WindowProbe {
	rt := reactive.NewRuntime()
	worldOf(rt) // made here, on the goroutine that made the probe
	w := newWindow(nil, rt, Config{Width: int(size.W), Height: int(size.H)}, build)
	w.dialogs = &runtime.StubFilePicker{}
	w.clipboard = &runtime.MemoryClipboard{}
	return &WindowProbe{w: w, screen: ggfx.NewImage(int(size.W), int(size.H))}
}

// Window returns the window the probe runs, for what a test asks of it
// directly: Post, Setup, OnFrame, RepaintRect, Shortcut.
func (p *WindowProbe) Window() *Window { return p.w }

// Screen returns the image the window paints its frames into, which keeps
// the last frame as a driver's screen does: a frame painted in part or
// within a boundary paints over what it holds. A test reads it with
// ReadPixels inside a frame of the renderer, as gfxtest.MainInFrame runs
// one, to tell that such a frame shows what one painted whole would.
func (p *WindowProbe) Screen() *ggfx.Image { return p.screen }

// Close closes the window and disposes everything built under it.
func (p *WindowProbe) Close() {
	p.w.Close()
	p.screen.Deallocate()
}

// Step moves the window's clock forward by d and runs one frame, as the
// window does when its FrameEvent comes, and reports what the frame did.
// The first call fixes the clock at the current time; from then on frames
// see only the time Step adds. As in a window, a frame moves the clock at
// most 100ms past the last, however long d is, but for the frame the last
// one's Wake asked for, which may move as far as that instant. A closed
// window runs no frame and reports the zero WindowFrame. Step panics with
// the error a window's frame would have ended App.Run with.
func (p *WindowProbe) Step(d time.Duration) WindowFrame {
	w := p.w
	if w.frameErr != nil {
		panic(w.frameErr)
	}
	if w.closed {
		return WindowFrame{}
	}
	if w.dispose == nil {
		w.start()
	}
	clk := &w.world().frame
	clk.pinned = clk.raw().Add(d)
	p.damage = p.damage[:0]
	// The first frame finds nothing on the screen to keep.
	next, wake, err := w.frameOn(p.screen, 1, p.frames > 0, p.setDamage)
	p.frames++
	if err != nil {
		panic(err)
	}
	fr := WindowFrame{At: FrameTime(), Painted: w.drew, Next: next, Wake: wake}
	if fr.Painted != FrameSkipped {
		fr.Damage = append([]Rect(nil), p.damage...)
	}
	p.last = fr
	return fr
}

// Run runs a frame d after the last, as the event that changed something
// would bring one, then more, each d after the last, while the window asks
// for the next, and returns what each did: the last asks for none when the
// window came to rest within most frames. It does not follow Wake.
func (p *WindowProbe) Run(d time.Duration, most int) []WindowFrame {
	var out []WindowFrame
	for range most {
		fr := p.Step(d)
		out = append(out, fr)
		if !fr.Next {
			break
		}
	}
	return out
}

// Idle runs the frames a window with no input runs over span: a refresh d
// after a frame that asks for the next, at the Wake of one that asks for
// none, or a millisecond on when that has passed, and none after one that
// asks for neither. It returns what each did; a window at rest returns
// none.
func (p *WindowProbe) Idle(d, span time.Duration) []WindowFrame {
	var out []WindowFrame
	last := p.last
	for passed := time.Duration(0); ; {
		step := d
		if !last.Next {
			if last.Wake.IsZero() {
				break
			}
			step = max(last.Wake.Sub(last.At), time.Millisecond)
		}
		if passed += step; passed > span {
			break
		}
		last = p.Step(step)
		out = append(out, last)
	}
	return out
}

// setDamage keeps what a frame says it changed, in logical pixels. A frame
// a scroll was to contain may say all of the window after, painting it
// whole: the last word stands.
func (p *WindowProbe) setDamage(rs []image.Rectangle) {
	c := &p.w.canvas
	p.damage = p.damage[:0]
	for _, r := range rs {
		p.damage = append(p.damage, Rct(Pt(c.dp(float64(r.Min.X)), c.dp(float64(r.Min.Y))), Sz(c.dp(float64(r.Dx())), c.dp(float64(r.Dy())))))
	}
}

// Find returns the first region the last frame registered with label,
// without running a frame.
func (p *WindowProbe) Find(label string) (Found, bool) {
	for _, f := range foundIn(&p.w.input, "") {
		if f.Label == label {
			return f, true
		}
	}
	return Found{}, false
}

// Move puts the pointer at pos, for the next frame.
func (p *WindowProbe) Move(pos Point) {
	p.handle(ggwd.MouseMoveEvent{X: pos.X, Y: pos.Y})
}

// Press pushes the left button down at pos, for the next frame.
func (p *WindowProbe) Press(pos Point) {
	p.handle(ggwd.MouseButtonEvent{Button: MouseButtonLeft, Pressed: true, X: pos.X, Y: pos.Y})
}

// Release lets the left button go at pos, for the next frame.
func (p *WindowProbe) Release(pos Point) {
	p.handle(ggwd.MouseButtonEvent{Button: MouseButtonLeft, X: pos.X, Y: pos.Y})
}

// Click presses and releases the left button at pos, both for the next
// frame, as a quick click between two frames comes.
func (p *WindowProbe) Click(pos Point) {
	p.Press(pos)
	p.Release(pos)
}

// Scroll turns the wheel by delta, in lines, over pos, for the next frame.
func (p *WindowProbe) Scroll(pos, delta Point) {
	p.Move(pos)
	p.handle(ggwd.ScrollEvent{X: delta.X, Y: delta.Y})
}

// Key presses and releases each chord, written as ParseChord reads it,
// all for the next frame. It panics on a malformed chord.
func (p *WindowProbe) Key(chords ...string) {
	for _, s := range chords {
		c := MustChord(s)
		p.handle(ggwd.KeyEvent{Key: c.Key, Pressed: true, Modifiers: c.held()})
		p.handle(ggwd.KeyEvent{Key: c.Key, Modifiers: c.held()})
	}
}

// Text delivers typed characters to the focused widget, for the next
// frame, as a platform without an IME would.
func (p *WindowProbe) Text(s string) { p.handle(ggwd.TextEvent{Text: s}) }

// handle hands ev to the window as its native window would.
func (p *WindowProbe) handle(ev ggwd.Event) {
	if err := p.w.handle(ev); err != nil {
		panic(err)
	}
}
