package ggui

import (
	"io/fs"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/inspect"
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
	size Size

	pointer    Point
	hasPointer bool

	sinks []func(*inspect.Frame)
}

// SetOverlay installs o over the probe's window, as App.SetOverlay does:
// it paints after every frame and is offered input first.
func (p *Probe) SetOverlay(o Overlay) { p.overlay = o }

// RenderTo draws every frame from now on into img, at one pixel per
// logical pixel, for a test that looks at what was painted. Without it a
// probe lays out and routes input but draws nothing.
func (p *Probe) RenderTo(img *ggfx.Image) { p.canvas.Image = img }

// SetScale makes the probe's canvas scale device pixels per logical pixel,
// as a HiDPI monitor makes a window's, for a test of what a widget sizes or
// rasterizes for the screen: Canvas.Scale and UseScale report it, and
// RenderTo draws at it. The default is 1. Call it before the first frame.
func (p *Probe) SetScale(scale float64) { p.canvas.scale = scale }

// Layers returns how many offscreen images the last frame drawn painted
// its layers into. Canvas.Layer, and the blurs, shape clips and transforms
// that paint through one, take an image each, though a layer opened after
// another has closed, at the same depth, paints into that one's image
// where it covers the area. It is for a test of when a widget pays for a
// layer, as a fade in progress does and an opacity of 1 or 0 must not. A
// probe that draws nothing, with no RenderTo or Draw, paints every layer
// into the Canvas it was opened on and takes none.
func (p *Probe) Layers() int {
	if f := p.canvas.frameState; f != nil {
		return f.layers.Len()
	}
	return 0
}

// WholeRepaints counts the times the input the probe handled asked for the
// whole window to be painted again, rather than only the regions its
// events reached: with RepaintAll, which a handler calls when what it did
// changes how widgets look beyond the region its event reached, as a click
// clearing a selection in the paragraphs it did not land in does, and as
// input does itself for each focus and blur it hands out. A probe paints
// every frame whole whatever was asked, so a test reads the count to tell
// that a window would have painted the frame after the input whole.
func (p *Probe) WholeRepaints() uint64 { return p.world().delivered }

// Draw runs one frame into img, as RenderTo followed by Frame would, and
// goes back to drawing nothing. The caller clears img and sizes the probe in
// logical pixels with Resize. Call it where GPU drawing is allowed: inside a
// ggfx FrameEvent, as the examples' offscreen renders do.
func (p *Probe) Draw(img *ggfx.Image) Size {
	p.RenderTo(img)
	defer p.RenderTo(nil)
	return p.Frame()
}

// OnInspect registers fn to receive every frame's inspect.Frame, as
// App.OnInspect does.
func (p *Probe) OnInspect(fn func(*inspect.Frame)) {
	p.sinks = append(p.sinks, fn)
	inspectorEnabled.Store(true)
}

// NewProbe creates a Probe that lays w out at size under the current root environment.
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
	p.init(p, nil, build)
	p.headless()
	return p
}

// Setup registers fn to run under the probe's root owner before the first
// build, as App.Setup does. Call it before the first frame.
func (p *Probe) Setup(fn func()) *Probe {
	p.setup = append(p.setup, fn)
	return p
}

// Close disposes the root owner and everything built under it.
func (p *Probe) Close() { p.close() }

// RequestClose asks to close as a window's close button does: the
// OnCloseRequest handlers run now, and the probe closes if they all agree.
// It reports whether it closed.
func (p *Probe) RequestClose() bool {
	if !p.closeAllowed() {
		return false
	}
	p.Close()
	return true
}

// Resize changes the viewport; the next frame lays the tree out again.
func (p *Probe) Resize(size Size) { p.size = size }

// Flush runs effects until they settle, without a frame, for a test that
// reads a DerivedValue or a widget built by an effect before the first frame. It
// panics with ErrCycle when they never settle.
func (p *Probe) Flush() {
	p.start()
	if err := p.settle(p.size); err != nil {
		panic(err)
	}
}

// Frame runs one frame: posted work, animations, effects, then layout when
// something moved and paint, collecting the hit regions the next event is
// routed to. It returns the root's size.
func (p *Probe) Frame() Size {
	p.start()
	p.beginFrame()
	if err := p.tick(true); err != nil {
		panic(err)
	}
	if p.root == nil {
		return Size{}
	}
	c := &p.canvas
	defer c.trimLayers()
	f := c.fs()
	f.pointer, f.hasPointer = p.pointer, p.hasPointer
	ok, err := p.paint(paintPass{size: p.size, tracing: len(p.sinks) > 0, hooks: probeHooks{p}})
	if err != nil {
		panic(err)
	}
	if !ok {
		return Size{}
	}
	return p.rootSize
}

// Advance moves the probe's clock forward by d and runs a frame, so
// animations step by exactly d. The first call fixes the probe's clock at
// the current time; from then on its frames, and Now and every other
// reading of the time in them, see the time Advance set. Other probes and
// the App keep their own clocks.
//
//	p.Advance(150 * time.Millisecond) // a 300ms tween is now halfway
func (p *Probe) Advance(d time.Duration) {
	p.advanceClock(d)
	p.Frame()
}

func (p *Probe) dispatch(f frameInput) {
	p.pointer, p.hasPointer = f.pos, true
	p.Frame()
	p.deliver(f)
	if err := p.settle(p.size); err != nil {
		panic(err)
	}
}

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
	return foundIn(&p.input, role)
}

// foundIn returns every region in routes to with role, in paint order, or
// every region with a role when role is empty.
func foundIn(in *inputState, role Role) []Found {
	var out []Found
	for i := range in.regions {
		r := &in.regions[i]
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

// MoveWith is Move with mods held, as ⌘ over a link.
func (p *Probe) MoveWith(pos Point, mods Mods) { p.dispatch(frameInput{pos: pos, mods: mods}) }

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

// ClickWith clicks at pos with mods held, as a Shift-click or ⌘-click.
func (p *Probe) ClickWith(pos Point, mods Mods) {
	p.dispatch(frameInput{pos: pos, down: []MouseButton{MouseButtonLeft}, mods: mods})
	p.dispatch(frameInput{pos: pos, up: []MouseButton{MouseButtonLeft}, mods: mods})
}

// ClickButton presses and releases the specified mouse button at pos.
func (p *Probe) ClickButton(pos Point, button MouseButton) {
	p.dispatch(frameInput{pos: pos, down: []MouseButton{button}})
	p.dispatch(frameInput{pos: pos, up: []MouseButton{button}})
}

// TouchDown puts a finger down at pos, as a touch screen reports its first
// one: it presses there as the left button does. TouchMove and TouchUp
// follow it, and a finger that moves far enough pans the scroll under it,
// which takes the gesture from the press.
func (p *Probe) TouchDown(pos Point) {
	p.dispatch(frameInput{touch: true, pos: pos, down: []MouseButton{MouseButtonLeft}})
}

// TouchMove moves the finger TouchDown put down to pos.
func (p *Probe) TouchMove(pos Point) { p.dispatch(frameInput{touch: true, pos: pos}) }

// TouchUp lifts the finger at pos.
func (p *Probe) TouchUp(pos Point) {
	p.dispatch(frameInput{touch: true, pos: pos, up: []MouseButton{MouseButtonLeft}})
}

// Scroll turns the wheel by delta over pos.
func (p *Probe) Scroll(pos, delta Point) { p.dispatch(frameInput{pos: pos, wheel: delta}) }

// Type presses keys, one frame each, with mods held.
func (p *Probe) Type(mods Mods, keys ...KeyboardKey) {
	for _, k := range keys {
		p.dispatch(frameInput{keys: []KeyboardKey{k}, mods: mods})
	}
}

// Key presses each chord, written as ParseChord reads it, one frame each.
// "cmd+v" is ⌘V on macOS and Ctrl+V elsewhere, so a test that presses a
// shortcut with it passes on every platform. It panics on a malformed
// chord.
//
//	p.Key("cmd+a", "cmd+c")
func (p *Probe) Key(chords ...string) {
	for _, s := range chords {
		c := MustChord(s)
		p.Type(c.held(), c.Key)
	}
}

// Text delivers typed characters to the focused widget, as a platform
// without an IME would.
func (p *Probe) Text(s string) { p.dispatch(frameInput{text: s}) }

// TypeText presses key with mods held and delivers the text it typed, in
// one frame, as a platform without an IME does a key that types.
func (p *Probe) TypeText(mods Mods, key KeyboardKey, text string) {
	p.dispatch(frameInput{keys: []KeyboardKey{key}, text: text, mods: mods})
}

// KeyRelease lets go of each key, one frame each, with mods held.
func (p *Probe) KeyRelease(mods Mods, keys ...KeyboardKey) {
	for _, k := range keys {
		p.dispatch(frameInput{released: []KeyboardKey{k}, mods: mods})
	}
}

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
func (p *Probe) Cursor() CursorShape { return p.cursorAt(p.pointer) }

// Focused reports whether some region holds keyboard focus.
func (p *Probe) Focused() bool { return p.input.focused != nil }

// probeHooks are a Probe's paintHooks: the sinks see each frame.
type probeHooks struct{ p *Probe }

func (probeHooks) settled() {}

func (h probeHooks) painted() {
	p := h.p
	if len(p.sinks) == 0 {
		return
	}
	fr := inspectFrame(&p.canvas, p.semantics())
	for _, fn := range p.sinks {
		fn(fr)
	}
}
