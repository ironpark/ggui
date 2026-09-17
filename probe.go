package ggui

import "github.com/hajimehoshi/ebiten/v2"

// Probe drives a widget tree without a window, for tests: it runs the same
// per-frame steps as App (flush effects, lay out, paint, dispatch input)
// against a Canvas that draws nothing, so hit regions, focus, keys and the
// signals a widget writes can all be checked headlessly.
//
//	p := ggui.NewProbe(ui.Checkbox(on, "x"), ggui.Sz(200, 30))
//	p.Click(ggui.Pt(5, 5))
//	// on.Peek() is now true
//
// Every input method paints a frame first, as the runtime would have before
// the event, and flushes effects afterwards.
type Probe struct {
	root Widget
	size Size
	in   inputState
	env  Env

	pointer    Point
	hasPointer bool
}

// NewProbe creates a Probe that lays w out at size under the current theme.
func NewProbe(w Widget, size Size) *Probe {
	return &Probe{root: w, size: size, env: rootEnv()}
}

// Frame flushes effects, then lays out and paints the tree, collecting the
// hit regions the next event is routed to. It returns the root's size.
func (p *Probe) Frame() Size {
	effects.flush()
	c := Canvas{prev: p.in.regions, pointer: p.pointer, hasPointer: p.hasPointer}
	s := p.root.Layout(Tight(p.size), p.env)
	c.Paint(p.root, Rect{Size: s})
	c.paintOverlays()
	p.in.regions = c.hits
	return s
}

func (p *Probe) dispatch(f frameInput) {
	p.pointer, p.hasPointer = f.pos, true
	p.Frame()
	p.in.dispatch(f)
	effects.flush()
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
