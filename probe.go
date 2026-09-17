package ggui

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// Probe drives a widget tree without a window, for tests: it runs the same
// per-frame steps as App (posted work, input, animations, effects, layout
// when something moved, paint) against a Canvas that draws nothing, so hit
// regions, focus, keys and the signals a widget writes can all be checked
// headlessly.
//
//	p := ggui.NewProbe(ui.Checkbox(on, "x"), ggui.Sz(200, 30))
//	defer p.Close()
//	p.Click(ggui.Pt(5, 5))
//	// on.Peek() is now true
//
// Every input method paints a frame first, as the runtime would have before
// the event, and flushes effects afterwards. Effects that never settle
// panic with ErrCycle.
type Probe struct {
	frameLoop
	size   Size
	in     inputState
	env    Env
	canvas Canvas

	pointer    Point
	hasPointer bool

	now     time.Time // the probe's clock once Advance has been called
	restore func()
}

// NewProbe creates a Probe that lays w out at size under the current theme.
// The widget is held by a root owner, so effects it creates are disposed by
// Close.
func NewProbe(w Widget, size Size) *Probe {
	return ProbeBuilder(func() Widget { return w }, size)
}

// ProbeBuilder creates a Probe from a Builder, as New does for an App: build
// runs under a root owner and again whenever a signal it read changes, and
// setup functions run first.
//
//	p := ggui.ProbeBuilder(func() ggui.Widget { return ggui.Textf("%d", n) }, ggui.Sz(100, 20))
func ProbeBuilder(build Builder, size Size) *Probe {
	p := &Probe{size: size, env: rootEnv()}
	p.build = build
	return p
}

// Setup registers fn to run under the probe's root owner before the first
// build, as App.Setup does. Call it before the first frame.
func (p *Probe) Setup(fn func()) *Probe {
	p.setup = append(p.setup, fn)
	return p
}

// Post queues fn to run before the next frame's input, as App.Post does.
func (p *Probe) Post(fn func()) { p.post(fn) }

// Close disposes the root owner and everything built under it, and puts
// back the clock Advance replaced.
func (p *Probe) Close() {
	p.close()
	if p.restore != nil {
		p.restore()
		p.restore = nil
	}
}

// Resize changes the viewport; the next frame lays the tree out again.
func (p *Probe) Resize(size Size) { p.size = size }

// Flush runs effects until they settle, without a frame, for a test that
// reads a Memo or a widget built by an effect before the first frame. It
// panics with ErrCycle when they never settle.
func (p *Probe) Flush() {
	if p.dispose == nil && !p.closed {
		p.start()
	}
	if !effects.flush() {
		panic(ErrCycle)
	}
}

// Frame runs one frame: posted work, animations, effects, then layout when
// something moved and paint, collecting the hit regions the next event is
// routed to. It returns the root's size.
func (p *Probe) Frame() Size {
	if p.dispose == nil && !p.closed {
		p.start()
	}
	p.runPosted()
	if err := p.tick(p.clock()); err != nil {
		panic(err)
	}
	if p.root == nil {
		return Size{}
	}
	c := &p.canvas
	c.prev, c.hits = c.hits, nil
	c.pointer, c.hasPointer, c.logical = p.pointer, p.hasPointer, p.size
	c.nextFrame()
	c.resetSemantics()
	if p.needsLayout(p.size) {
		p.rootSize = p.root.Layout(Tight(p.size), p.env)
	}
	c.Paint(p.root, Rect{Size: p.rootSize})
	c.paintOverlays()
	p.publishSemantics(c, p.in.focused)
	p.in.regions = c.hits
	return p.rootSize
}

// clock is what the probe's frames read: the time Advance set, else the
// package clock.
func (p *Probe) clock() time.Time {
	if p.now.IsZero() {
		return clock()
	}
	return p.now
}

// Advance moves the probe's clock forward by d and runs a frame, so
// animations step by exactly d. The first call fixes the clock at the
// current time and installs it with SetClock, so widgets that read Now
// see the same time; Close restores the previous clock.
//
//	p.Advance(150 * time.Millisecond) // a 300ms tween is now halfway
func (p *Probe) Advance(d time.Duration) {
	if p.now.IsZero() {
		p.now = clock()
		p.restore = SetClock(func() time.Time { return p.now })
	}
	p.now = p.now.Add(d)
	p.Frame()
}

func (p *Probe) dispatch(f frameInput) {
	p.pointer, p.hasPointer = f.pos, true
	p.Frame()
	p.in.dispatch(f)
	if !effects.flush() {
		panic(ErrCycle)
	}
}

// OnKey registers a global key handler, as App.OnKey does.
func (p *Probe) OnKey(fn func(KeyEvent) bool) { p.in.shortcuts = append(p.in.shortcuts, fn) }

// Shortcut registers a chord, as App.Shortcut does.
func (p *Probe) Shortcut(chord string, fn func()) *ShortcutHandle { return p.in.addShortcut(chord, fn) }

// Semantics runs a frame and returns the accessibility tree it published:
// every element on screen, nested as the widgets described it, including
// the ones Probe.Find cannot see because they take no input. A test asserts
// its shape with String.
func (p *Probe) Semantics() *SemTree {
	p.Frame()
	return p.semantics()
}

// Found is a region a Find located: where it is and what it is.
type Found struct {
	Rect  Rect
	Role  Role
	Label string
}

// Center returns the middle of the region, where a click lands.
func (f Found) Center() Point {
	return Pt(f.Rect.Origin.X+f.Rect.Size.W/2, f.Rect.Origin.Y+f.Rect.Size.H/2)
}

// Find returns the first region painted with label, after running a
// frame so the regions are current: a button by its text, a checkbox by
// its label, a field by its Label or placeholder.
func (p *Probe) Find(label string) (Found, bool) {
	return p.FindRole("", label)
}

// FindRole returns the first region painted with role and label; either
// may be empty to match any.
func (p *Probe) FindRole(role Role, label string) (Found, bool) {
	all := p.FindAll(role)
	for _, f := range all {
		if label == "" || f.Label == label {
			return f, true
		}
	}
	return Found{}, false
}

// FindAll returns every region painted with role, in paint order, or every
// region with a role when role is empty.
func (p *Probe) FindAll(role Role) []Found {
	p.Frame()
	var out []Found
	for i := range p.in.regions {
		r := &p.in.regions[i]
		if r.role == "" || (role != "" && r.role != role) {
			continue
		}
		out = append(out, Found{Rect: r.rect, Role: r.role, Label: r.label})
	}
	return out
}

// Tap clicks the middle of the first region labelled label, and panics
// when there is none.
func (p *Probe) Tap(label string) {
	f, ok := p.Find(label)
	if !ok {
		panic("ggui: Probe.Tap: nothing labelled " + label)
	}
	p.Click(f.Center())
}

// Move puts the pointer at pos with no buttons held, which drives hover,
// cursor shape and, after Press, dragging.
func (p *Probe) Move(pos Point) { p.dispatch(frameInput{pos: pos}) }

// Press pushes the left button down at pos.
func (p *Probe) Press(pos Point) {
	p.dispatch(frameInput{pos: pos, down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
}

// Release lets the left button go at pos.
func (p *Probe) Release(pos Point) {
	p.dispatch(frameInput{pos: pos, up: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
}

// Click presses and releases the left button at pos.
func (p *Probe) Click(pos Point) {
	p.Press(pos)
	p.Release(pos)
}

// Scroll turns the wheel by delta over pos.
func (p *Probe) Scroll(pos, delta Point) { p.dispatch(frameInput{pos: pos, wheel: delta}) }

// Type presses keys, one frame each, with mods held.
func (p *Probe) Type(mods Mods, keys ...ebiten.Key) {
	for _, k := range keys {
		p.dispatch(frameInput{keys: []ebiten.Key{k}, mods: mods})
	}
}

// Text delivers typed characters to the focused widget, as a platform
// without an IME would.
func (p *Probe) Text(s string) { p.dispatch(frameInput{text: s}) }

// Cursor returns the cursor shape the last event left the pointer with.
func (p *Probe) Cursor() ebiten.CursorShapeType { return p.in.cursor }

// Focused reports whether some region holds keyboard focus.
func (p *Probe) Focused() bool { return p.in.focused != nil }
