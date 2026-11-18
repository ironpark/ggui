package record

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/ironpark/ggui"
)

// mouse is the pointer the Recorder's steps move, and how it is drawn.
type mouse struct {
	draw   bool // the Cursor option: draw it at all
	shown  bool // HideCursor and ShowCursor
	placed bool // a pointer step has put it somewhere
	at     ggui.Point
	down   bool      // a button is held: drawn smaller
	size   ggui.Size // the tour's, where it starts
	rings  []ring
}

// ring is the ring a click leaves, from the frame it was made at.
type ring struct {
	at    ggui.Point
	frame int
}

// ringTime is how long a click's ring takes to fade.
const ringTime = 400 * time.Millisecond

// How long the pointer steps take where the script gives no duration.
const (
	pressTime  = 100 * time.Millisecond // a button held for a click
	settleTime = 100 * time.Millisecond // after a click, before the next step
	keyTime    = 120 * time.Millisecond // after each chord Key presses
)

// drawPointer draws the clicks' rings and the cursor over img, where the
// pointer is, in the shape the app asks for there.
func (r *Recorder) drawPointer(img *image.RGBA, dark bool) {
	m := &r.mouse
	col := color.NRGBA{A: 0xff}
	if dark {
		col = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	}
	n := r.framesIn(ringTime)
	keep := m.rings[:0]
	for _, rg := range m.rings {
		age := r.frames - rg.frame
		if age >= n {
			continue
		}
		keep = append(keep, rg)
		if m.draw {
			drawRing(img, r.lay.device(rg.at), float64(age)/float64(n), r.lay.scale, col)
		}
	}
	m.rings = keep
	if !m.draw || !m.shown || !m.placed {
		return
	}
	sp := r.cursors.get(kindOf(r.p.Cursor()), m.down)
	sp.draw(img, r.lay.device(m.at))
}

// framesIn returns how many frames d takes, rounded up, as Hold draws.
func (r *Recorder) framesIn(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	return int((int64(d)*int64(r.fps) + int64(time.Second) - 1) / int64(time.Second))
}

// shootFor draws the frames d takes, at least one.
func (r *Recorder) shootFor(d time.Duration) {
	for range max(r.framesIn(d), 1) {
		r.shoot()
	}
}

// Pointer returns where the pointer the steps move is, in logical pixels
// of the window: the middle of it before the first pointer step.
func (r *Recorder) Pointer() ggui.Point {
	if !r.mouse.placed {
		return ggui.Pt(r.mouse.size.W/2, r.mouse.size.H/2)
	}
	return r.mouse.at
}

// moveTo puts the pointer at p and tells the app.
func (r *Recorder) moveTo(p ggui.Point) {
	r.mouse.at, r.mouse.placed = p, true
	r.p.Move(p)
}

// HideCursor stops drawing the cursor from the next frame; the pointer
// steps still move it. ShowCursor draws it again.
func (r *Recorder) HideCursor() { r.mouse.shown = false }

// ShowCursor draws the cursor again after HideCursor. The cursor shows
// from the first pointer step.
func (r *Recorder) ShowCursor() { r.mouse.shown = true }

// MoveTo glides the pointer to p, in logical pixels of the window, over
// d: along a slight curve, easing in and out, a frame at a time, telling
// the app where it is at each so that what it hovers lights up as it
// passes. Before the first pointer step it starts in the middle of the
// window. A d of 0 puts it there at once, drawing nothing.
func (r *Recorder) MoveTo(p ggui.Point, d time.Duration) {
	from := r.Pointer()
	n := r.framesIn(d)
	if n == 0 {
		r.moveTo(p)
		return
	}
	for k := 1; k <= n; k++ {
		r.moveTo(glide(from, p, float64(k)/float64(n)))
		r.shoot()
	}
}

// glide returns where the pointer is t of the way from a to b: eased in
// and out, along a quadratic curve that bows to one side by a tenth of
// the distance, at most 40 logical pixels. Along the line from a to b it
// gets on as the easing does, and it lands on b exactly.
func glide(a, b ggui.Point, t float64) ggui.Point {
	if t >= 1 {
		return b
	}
	if t <= 0 {
		return a
	}
	e := easeInOut(t)
	dx, dy := b.X-a.X, b.Y-a.Y
	dist := math.Hypot(dx, dy)
	if dist == 0 {
		return b
	}
	bow := min(dist*0.1, 40) / dist
	cx, cy := a.X+dx/2-dy*bow, a.Y+dy/2+dx*bow
	u := 1 - e
	return ggui.Pt(u*u*a.X+2*u*e*cx+e*e*b.X, u*u*a.Y+2*u*e*cy+e*e*b.Y)
}

// easeInOut is the cubic ease in and out.
func easeInOut(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	f := -2*t + 2
	return 1 - f*f*f/2
}

// glideTime is how long the pointer takes to the next control, from how
// far it is: as a hand moves, longer for further, within reason.
func glideTime(a, b ggui.Point) time.Duration {
	d := math.Hypot(b.X-a.X, b.Y-a.Y)
	return min(max(300*time.Millisecond+time.Duration(d*1.2)*time.Millisecond, 400*time.Millisecond), 900*time.Millisecond)
}

// find returns the middle of what label names, as Probe.Find finds it,
// and ends the tour with an error naming the label when nothing has it.
func (r *Recorder) find(label string) ggui.Point {
	f, ok := r.p.Find(label)
	if !ok {
		panic(fmt.Errorf("nothing is labelled %q for the pointer to go to", label))
	}
	return f.Center()
}

// MoveToLabel glides the pointer over d to the middle of what is
// labelled label, as Probe.Find finds it: a button by its text, a field
// by its name or placeholder. The tour ends with an error naming the
// label when nothing has it.
func (r *Recorder) MoveToLabel(label string, d time.Duration) {
	r.MoveTo(r.find(label), d)
}

// ClickLabel glides the pointer to what is labelled label, taking longer
// the further it goes, and clicks it.
func (r *Recorder) ClickLabel(label string) {
	p := r.find(label)
	r.MoveTo(p, glideTime(r.Pointer(), p))
	r.Click()
}

// press pushes button down where the pointer is, leaving a ring.
func (r *Recorder) press() {
	at := r.Pointer()
	r.mouse.placed, r.mouse.at = true, at
	r.mouse.down = true
	r.mouse.rings = append(r.mouse.rings, ring{at: at, frame: r.frames})
	r.p.Press(at)
}

func (r *Recorder) release() {
	r.mouse.down = false
	r.p.Release(r.Pointer())
}

// Click clicks the left button where the pointer is: it presses, holds
// for a few frames, in which the app shows what it does while pressed
// and the cursor is drawn a little smaller, and lets go; a ring fades
// out around the click.
func (r *Recorder) Click() {
	r.press()
	r.shootFor(pressTime)
	r.release()
	r.shootFor(settleTime)
}

// DoubleClick clicks twice where the pointer is, quickly enough to count
// as a double click.
func (r *Recorder) DoubleClick() {
	r.press()
	r.shoot()
	r.release()
	r.shoot()
	r.press()
	r.shoot()
	r.release()
	r.shootFor(settleTime)
}

// RightClick clicks the secondary button where the pointer is, as for a
// context menu.
func (r *Recorder) RightClick() {
	at := r.Pointer()
	r.mouse.placed, r.mouse.at = true, at
	r.mouse.rings = append(r.mouse.rings, ring{at: at, frame: r.frames})
	r.mouse.down = true
	r.p.ClickButton(at, ggui.MouseButtonRight)
	r.shootFor(pressTime)
	r.mouse.down = false
	r.shootFor(settleTime)
}

// Drag presses the left button where the pointer is, glides to to over
// d, and lets go there.
func (r *Recorder) Drag(to ggui.Point, d time.Duration) {
	r.press()
	r.shoot()
	r.MoveTo(to, d)
	r.release()
	r.shootFor(settleTime)
}

// Scroll turns the wheel by delta, in the units Probe.Scroll takes,
// over the pointer, spread over the frames d takes.
func (r *Recorder) Scroll(delta ggui.Point, d time.Duration) {
	at := r.Pointer()
	r.mouse.placed, r.mouse.at = true, at
	n := max(r.framesIn(d), 1)
	for k := 1; k <= n; k++ {
		f0, f1 := float64(k-1)/float64(n), float64(k)/float64(n)
		r.p.Scroll(at, ggui.Pt(delta.X*f1-delta.X*f0, delta.Y*f1-delta.Y*f0))
		r.shoot()
	}
}

// Type types text a character at a time into what has the keyboard
// focus, perChar apart, so that it appears as it would being typed.
// Characters closer together than a frame land in the same frame.
func (r *Recorder) Type(text string, perChar time.Duration) {
	start := r.frames
	i := 0
	for _, c := range text {
		i++
		r.p.Text(string(c))
		due := start + r.framesIn(time.Duration(i)*perChar)
		for r.frames < due {
			r.shoot()
		}
	}
	if r.frames == start && i > 0 {
		r.shoot()
	}
}

// Key presses each chord, written as ggui.ParseChord reads it ("enter",
// "cmd+a", "shift+tab"), as Probe.Key does, holding what follows each
// for a beat.
func (r *Recorder) Key(chords ...string) {
	for _, c := range chords {
		r.p.Key(c)
		r.shootFor(keyTime)
	}
}
