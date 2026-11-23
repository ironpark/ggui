package ggui

import (
	"image"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/framestats"
)

// frameOn runs one frame into screen: all of runFrameEvent that needs no
// native window, which a WindowProbe runs as well, so that a test sees the
// frames a window paints and asks for, not ones like them. It reports
// whether the next frame must run without waiting for input and, when it
// need not, the instant a widget asked to be painted again at, or zero.
// The cursor the frame leaves is w.cursor.
func (w *Window) frameOn(screen *ggfx.Image, scale float64, kept bool, setDamage func([]image.Rectangle)) (next bool, wake time.Time, err error) {
	// Handlers dispatched below reach this window through UseHost.
	w.beginFrame()
	f := w.takeInput()
	cf := w.canvas.fs()
	moved := !cf.hasPointer || cf.pointer != f.pos
	cf.pointer, cf.hasPointer = f.pos, true
	before := w.world().delivered
	w.dispatchInput(f)
	touched, placed := w.input.takeTouched()
	switch {
	case w.world().delivered != before:
		w.dirty.Or(uint32(dirtyInput))
	case !placed:
		w.dirty.Or(uint32(dirtyEvent))
	default:
		// What a pointer event reached is painted again, and what lies
		// beside it left as it was, unless a signal was written too.
		w.addRepaints(touched)
	}
	if moved && cf.pointerRead {
		w.pointerMoved(cf)
	}
	if w.closed {
		return false, time.Time{}, nil
	}
	if err := w.tick(false); err != nil {
		return false, time.Time{}, err
	}
	w.draw(screen, scale, kept, setDamage)
	if w.frameErr != nil {
		return false, time.Time{}, w.frameErr
	}
	w.cursor = w.cursorAt(f.pos)
	if w.closed {
		return false, time.Time{}, nil
	}
	if w.endFrame(f) {
		return true, time.Time{}, nil
	}
	return false, w.world().frame.takeWake(), nil
}

// draw paints the tree to screen, which is in physical pixels at scale
// pixels per logical pixel, so that a HiDPI monitor gets a sharp image while
// widgets keep working in logical pixels.
func (w *Window) draw(screen *ggfx.Image, scale float64, kept bool, setDamage func([]image.Rectangle)) {
	dirtyBits := w.takeDirty(scale)
	asked := w.takeRepaints()
	pointer := w.pointerAsked
	w.pointerAsked = false
	if w.skipFrame(kept, dirtyBits, asked) {
		return
	}
	target, kept := w.paintTarget(screen, kept)
	contained := w.takeContained()
	defer w.frameLoop.endFrameFree(len(contained) > 0, false)
	// within is where a frame whose changes a scroll contains paints, in
	// pixels; empty, the frame paints all of it.
	var within image.Rectangle
	if wanted := len(w.motion) > 0 || len(asked) > 0; dirtyBits != 0 {
		if wanted || len(contained) > 0 {
			refusePartialDirty(dirtyBits)
		}
	} else if len(contained) > 0 {
		within = w.containedDamage(target, kept, contained, asked)
	} else if wanted && w.drawPartial(screen, target, kept, asked, pointer, setDamage) {
		return
	}
	w.drawWhole(screen, target, within, setDamage)
}

// takeDirty takes what marked the window dirty since the last frame, a
// change of scale and semantics someone now wants among it.
func (w *Window) takeDirty(scale float64) dirtyReason {
	dirtyBits := dirtyReason(w.dirty.Swap(0))
	if scale > 0 {
		if scale != w.canvas.scale {
			dirtyBits |= dirtyScale
			w.canvas.scale = scale
		}
	}
	if w.canvas.fs().semOff && (w.performs.Load() > 0 || w.semWanted.Load() || w.ax.Listening()) {
		dirtyBits |= dirtyOther
	}
	return dirtyBits
}

// skipFrame reports, counting the frame skipped, whether nothing changed
// since the last frame, which the screen kept.
func (w *Window) skipFrame(kept bool, dirtyBits dirtyReason, asked []Rect) bool {
	if !kept || dirtyBits != 0 || len(asked) != 0 || len(w.motion) != 0 || w.motionLost || len(w.frameLoop.contained) != 0 ||
		w.stateChanged() {
		return false
	}
	// Nothing changed: work was posted that wrote nothing, say. The
	// screen keeps the last frame, and untouched, nothing is presented.
	framestats.CountSkipped()
	w.drew = FrameSkipped
	return true
}

// drawWhole paints the whole tree into target, or the part of it within
// when that is not empty, and presents it on screen.
func (w *Window) drawWhole(screen, target *ggfx.Image, within image.Rectangle, setDamage func([]image.Rectangle)) {
	d := &w.drawing
	*d = windowDraw{w: w, screen: screen, target: target, dst: target, setDamage: setDamage}
	w.drew = FrameWhole
	defer d.release()
	if !within.Empty() {
		d.dst = target.SubImage(within).(*ggfx.Image)
	}
	if within.Empty() {
		// All of it changes: the screen need not start from a copy of the last frame.
		d.damage(screen.Bounds())
	} else {
		d.damage(within)
	}
	if w.root == nil {
		d.dst.Fill(w.background())
		return
	}
	defer w.canvas.trimLayers()
	w.aim(d.dst)
	defer w.presentBackdrop(screen, target)
	logical := w.logicalSize(screen)
	if Untrack(w.viewport.Get) != logical {
		w.viewport.Set(logical)
	}
	d.tracing = w.inspect || len(w.sinks) > 0
	_, err := w.paint(paintPass{
		size:    logical,
		tracing: d.tracing,
		semOff:  !w.semWanted.Load() && w.performs.Load() == 0 && !d.tracing && !w.ax.Listening(),
		hooks:   d,
	})
	if !d.filled {
		// The loop closed or lost its root as it settled: what is
		// presented is the background, not what the screen kept.
		d.dst.Fill(w.background())
	}
	if err != nil {
		w.frameErr = err
		return
	}
	w.noteMotion(target)
}

// windowDraw is what a Window's whole frame keeps while it paints, the
// paint's hooks among it: kept on the Window, a frame allocates none of it.
type windowDraw struct {
	w                   *Window
	screen, target, dst *ggfx.Image // dst is where the frame paints: target, or the part a scroll contains
	setDamage           func([]image.Rectangle)
	tracing             bool
	// filled is whether the background was filled: once the settle has
	// run, which may change the theme it comes from, or turn a frame a
	// scroll contains into a whole one.
	filled bool
}

// damage tells the host, when it asked, that r is what the frame changes.
func (d *windowDraw) damage(r image.Rectangle) {
	if d.setDamage != nil {
		d.w.damagePx = append(d.w.damagePx[:0], r)
		d.setDamage(d.w.damagePx)
	}
}

// release lets go of what the frame held. The damage last given stays in
// the Window's damagePx, which the host may read until the next frame.
func (d *windowDraw) release() { *d = windowDraw{} }

// settled implements paintHooks.
func (d *windowDraw) settled() {
	w := d.w
	unheld := w.frameLoop.unheld
	w.frameLoop.unheld = false
	if d.dst != d.target && (w.freeChanged() || unheld) {
		// The layout changed more than the boundaries held: paint all of
		// it.
		if unheld {
			framestats.Refuse("sized by content")
		} else {
			framestats.Refuse("state changed")
			w.frameLoop.endFrameFree(true, true)
		}
		d.dst = d.target
		w.aim(d.dst)
		d.damage(d.screen.Bounds())
	}
	d.dst.Fill(w.background())
	d.filled = true
}

// painted implements paintHooks.
func (d *windowDraw) painted() {
	w := d.w
	// Counted once the settle had its chance to paint it whole.
	if d.dst != d.target {
		framestats.CountContained()
		w.drew = FrameContained
	}
	w.ax.Publish(w.semantics(), w.takeAnnouncements())
	if d.tracing {
		w.publishInspect(&w.canvas, w.semantics())
	}
}
