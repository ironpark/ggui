package ggui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui/inspect"
	"io/fs"
	"time"

	"github.com/ironpark/ggui/runtime"
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
//	// ggui.Untrack(on.Get) is now true
//
// Every input method paints a frame first, as the runtime would have before
// the event, and flushes effects afterwards. Effects that never settle
// panic with ErrCycle.
type Probe struct {
	frameLoop
	size   Size
	in     inputState
	canvas Canvas

	pointer    Point
	hasPointer bool

	overlay Overlay
	sinks   []func(*inspect.Frame)

	now     time.Time // the probe's clock once Advance has been called
	restore func()
}

// SetOverlay installs o over the probe's window, as App.SetOverlay does:
// it paints after every frame and is offered input first.
func (p *Probe) SetOverlay(o Overlay) { p.overlay = o }

// RenderTo draws every frame from now on into img, at one pixel per
// logical pixel, for a test that looks at what was painted. Without it a
// probe lays out and routes input but draws nothing.
func (p *Probe) RenderTo(img *ebiten.Image) { p.canvas.Image = img }

// OnInspect registers fn to receive every frame's inspect.Frame, as
// App.OnInspect does.
func (p *Probe) OnInspect(fn func(*inspect.Frame)) {
	p.sinks = append(p.sinks, fn)
	inspectorEnabled = true
}

// NewProbe creates a Probe that lays w out at size under the current theme.
// The widget is held by a root owner, so effects it creates are disposed by
// Close.
func NewProbe(w Widget, size Size) *Probe {
	return ProbeBuilder(func() Widget { return w }, size)
}

// ProbeBuilder creates a Probe from a Builder, as New does for an App: build
// runs once under a root owner, and
// setup functions run first.
//
//	p := ggui.ProbeBuilder(func() ggui.Widget { return ggui.Textf("%d", n) }, ggui.Sz(100, 20))
func ProbeBuilder(build Builder, size Size) *Probe {
	p := &Probe{size: size}
	p.build = build
	p.dialogs = &runtime.StubFilePicker{}
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
// reads a DerivedValue or a widget built by an effect before the first frame. It
// panics with ErrCycle when they never settle.
func (p *Probe) Flush() {
	if p.dispose == nil && !p.closed {
		p.start()
	}
	if err := p.settle(p.size); err != nil {
		panic(err)
	}
}

// Frame runs one frame: posted work, animations, effects, then layout when
// something moved and paint, collecting the hit regions the next event is
// routed to. It returns the root's size.
func (p *Probe) Frame() Size {
	if p.dispose == nil && !p.closed {
		p.start()
	}
	p.runFrame()
	p.runPosted()
	if err := p.tick(frame.set(p.clock())); err != nil {
		panic(err)
	}
	if p.root == nil {
		return Size{}
	}
	c := &p.canvas
	c.prev, c.hits = c.hits, nil
	f := c.fs()
	f.pointer, f.hasPointer, f.logical = p.pointer, p.hasPointer, p.size
	clear(f.trace)
	clear(f.traceWidgets)
	f.tracing = len(p.sinks) > 0
	f.trace, f.traceWidgets = f.trace[:0], f.traceWidgets[:0]
	f.focusBounds = Rect{}
	if p.in.focused != nil {
		f.focusBounds = p.in.focused.rect
	}
	c.nextFrame()
	c.resetSemantics()
	if err := p.settle(p.size); err != nil {
		panic(err)
	}
	if p.closed || p.root == nil {
		return Size{}
	}
	c.Paint(p.root, Rect{Size: p.rootSize})
	c.paintOverlays()
	p.in.regions = c.hits
	p.in.observers = c.inputObservers
	p.in.applyFocusRequest(c)
	p.publishSemantics(c, p.in.focused)
	if f.tracing {
		fr := inspectFrame(c, p.semantics())
		for _, fn := range p.sinks {
			fn(fr)
		}
	}
	if p.overlay != nil {
		p.overlay.Paint(c)
	}
	return p.rootSize
}

// clock is the raw time a probe's frame reads: the time Advance set, else
// the package clock. Frames set the frame clock to it exactly, so a probe
// steps by what the test asked for.
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
	p.in.dispatchOver(p.overlay, f)
	if err := p.settle(p.size); err != nil {
		panic(err)
	}
}

// OnFrame registers fn to run once per Frame, before posted work and
// input, as App.OnFrame does.
func (p *Probe) OnFrame(fn func()) { p.frame = append(p.frame, fn) }

// OnKey registers a global key handler, as App.OnKey does.
func (p *Probe) OnKey(fn func(KeyEvent) bool) { p.in.shortcuts = append(p.in.shortcuts, fn) }

// Shortcut registers a chord, as App.Shortcut does.
func (p *Probe) Shortcut(chord string, fn func()) *ShortcutHandle { return p.in.addShortcut(chord, fn) }

// OnDrop registers a handler for drops no zone took, as App.OnDrop does.
func (p *Probe) OnDrop(fn func(DropEvent)) { p.in.drops = append(p.in.drops, fn) }

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
// its label, a field by its Name setting or placeholder.
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
	p.dispatch(frameInput{pos: pos, down: []MouseButton{MouseButtonLeft}})
}

// Release lets the left button go at pos.
func (p *Probe) Release(pos Point) {
	p.dispatch(frameInput{pos: pos, up: []MouseButton{MouseButtonLeft}})
}

// Click presses and releases the left button at pos.
func (p *Probe) Click(pos Point) {
	p.ClickButton(pos, MouseButtonLeft)
}

// ClickButton presses and releases the specified mouse button at pos.
func (p *Probe) ClickButton(pos Point, button MouseButton) {
	p.dispatch(frameInput{pos: pos, down: []MouseButton{button}})
	p.dispatch(frameInput{pos: pos, up: []MouseButton{button}})
}

// Scroll turns the wheel by delta over pos.
func (p *Probe) Scroll(pos, delta Point) { p.dispatch(frameInput{pos: pos, wheel: delta}) }

// Type presses keys, one frame each, with mods held.
func (p *Probe) Type(mods Mods, keys ...KeyboardKey) {
	for _, k := range keys {
		p.dispatch(frameInput{keys: []KeyboardKey{k}, mods: mods})
	}
}

// Text delivers typed characters to the focused widget, as a platform
// without an IME would.
func (p *Probe) Text(s string) { p.dispatch(frameInput{text: s}) }

// DragOver drags files over the window at pos without dropping them, as
// the desktop reports while a drag is in progress; the zone under pos hears
// of it through OnDropHover. DragEnd, Drop or DropPaths ends it.
func (p *Probe) DragOver(pos Point) { p.dispatch(frameInput{pos: pos, drag: true, dragAt: pos}) }

// DragEnd abandons a drag started with DragOver, as letting go outside the
// window does.
func (p *Probe) DragEnd() { p.dispatch(frameInput{pos: p.pointer}) }

// Drop drops the root entries of fsys onto the window at pos, as the desktop
// would. A testing/fstest.MapFS is the easiest fsys to drop; the files it
// carries have no Path and are read through DroppedFile.Open.
func (p *Probe) Drop(pos Point, fsys fs.FS) {
	p.dispatch(frameInput{pos: pos, drop: droppedFiles(fsys)})
}

// DropPaths drops real files or directories onto the window at pos. Each
// file carries its absolute path and reads from its own directory.
func (p *Probe) DropPaths(pos Point, paths ...string) {
	p.dispatch(frameInput{pos: pos, drop: droppedPaths(paths)})
}

// Cursor returns the cursor shape the last event left the pointer with.
func (p *Probe) Cursor() CursorShape { return p.in.cursorOver(p.overlay, p.pointer) }

// Focused reports whether some region holds keyboard focus.
func (p *Probe) Focused() bool { return p.in.focused != nil }
