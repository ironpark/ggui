package ggui

import (
	"image"
	"slices"
	"sync"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
)

// A frame that only time asks for, because a widget read the clock, eased
// a value, or asked to be painted again at an instant, changes only where
// those widgets paint: a caret blinking, a dot breathing, a hover fading.
// Where the frame is painted into what still holds the last one, a screen
// its driver keeps (Metal's does) or the backbuffer a BackdropBlur keeps,
// such a frame is painted over it within those regions alone, rather than
// the whole window again for a few pixels. Anything else that asks for a frame, input, a
// signal written, a layout, a resize, paints it whole.
//
// Canvas.Paint keeps, for each widget, the bounds of what it and its
// subtree drew. A widget that read the time while painting, in its own
// Paint rather than a child's, leaves its bounds as a motion region; the
// next frame, if time is all that asks for it, is painted within them.
// There, a widget whose Rect with damageReach around it misses the region
// is not painted at all, and nothing painted registers input, semantics
// or shortcuts: those of the last whole frame stand, as nothing they
// depend on has changed.

// damageReach is how far beyond its Rect a widget may draw and still be
// painted again within a region it does not itself reach: a shadow, a
// halo, a focus ring.
const damageReach = 48

// motionSlack is how far around a motion region the next frame repaints,
// for a motion that moves a little further each frame.
const motionSlack = 4

// mostMotionRegions is how many separate regions a frame is painted in at
// most; past it they are painted as the one region around them all.
const mostMotionRegions = 6

// paintScope is a widget being painted: the bounds its subtree drew, and
// the clock's marks when it started and those its children made.
type paintScope struct {
	bounds      Rect
	start, kids uint64
	placed      bool // drawn in window coordinates, not under a Transform or a shape clip
}

// openScope begins a widget's scope at r.
func (f *frameState) openScope(c *Canvas, r Rect, marks uint64) {
	f.scopes = append(f.scopes, paintScope{bounds: r, start: marks, placed: c.space == nil})
}

// closeScope ends the widget's scope: a widget that read the time itself
// leaves its bounds as a motion region, and its bounds and marks go to its
// parent's.
func (f *frameState) closeScope(w Widget, marks uint64) {
	s := f.scopes[len(f.scopes)-1]
	f.scopes = f.scopes[:len(f.scopes)-1]
	total := marks - s.start
	if total-s.kids > 0 {
		if s.placed && !s.bounds.Empty() {
			f.motion = append(f.motion, s.bounds)
			f.movers = append(f.movers, widgetName(w))
		} else {
			f.motionLost = true
			f.movers = append(f.movers, widgetName(w)+" (unplaced)")
		}
	}
	if n := len(f.scopes); n > 0 {
		p := &f.scopes[n-1]
		p.kids += total
		p.bounds = cover(p.bounds, s.bounds)
	}
}

// drew adds r, with extra around it, to the bounds of the widget painting.
func (f *frameState) drew(r Rect, extra float64) {
	if n := len(f.scopes); n > 0 && !r.Empty() {
		s := &f.scopes[n-1]
		s.bounds = cover(s.bounds, grow(r, extra))
	}
}

// skips reports whether a partial frame leaves a widget at r unpainted:
// one outside a Transform or a shape clip whose Rect, with damageReach
// around it, misses the region being painted.
func (f *frameState) skips(c *Canvas, r Rect) bool {
	return f.partial && c.space == nil && !r.Empty() && !overlaps(grow(r, damageReach), f.damage)
}

// motionRegions are a frame's motion regions as the next frame paints
// them: grown by motionSlack, cut to the window, and merged where they
// overlap, or into one when there are many.
func motionRegions(motion []Rect, window Rect) []Rect {
	out := make([]Rect, 0, len(motion))
	for _, r := range motion {
		if r = grow(r, motionSlack).Intersect(window); !r.Empty() {
			out = append(out, r)
		}
	}
	for merged := true; merged; {
		merged = false
		for i := 0; i < len(out) && !merged; i++ {
			for j := i + 1; j < len(out); j++ {
				if overlaps(grow(out[i], motionSlack), out[j]) {
					out[i] = cover(out[i], out[j])
					out = slices.Delete(out, j, j+1)
					merged = true
					break
				}
			}
		}
	}
	if len(out) > mostMotionRegions {
		all := out[0]
		for _, r := range out[1:] {
			all = cover(all, r)
		}
		out = append(out[:0], all)
	}
	return out
}

// cover is the Rect around a and b, either of which may be empty and is
// then left out.
func cover(a, b Rect) Rect {
	switch {
	case a.Empty():
		return b
	case b.Empty():
		return a
	}
	return a.Union(b)
}

// overlaps reports whether a and b share any area.
func overlaps(a, b Rect) bool { return !a.Intersect(b).Empty() }

// startMotion clears what the last frame noted of its motion.
func (f *frameState) startMotion() {
	f.scopes, f.motion, f.movers = f.scopes[:0], f.motion[:0], f.movers[:0]
	f.motionLost, f.overlaid = false, false
}

// noteMotion decides, after a frame painted into target, whether the next
// may be painted in part, and where.
func (w *Window) noteMotion(target *ggfx.Image) {
	f := w.canvas.fs()
	w.motion = motionRegions(f.motion, Rct(Point{}, f.logical))
	// Painted in part, the frame keeps the input, semantics and overlays of
	// the last whole one: none may have been painted, nor anything that
	// reads every frame or traces it.
	switch {
	case f.motionLost:
		w.motionWhy = "unplaced"
	case f.overlaid:
		w.motionWhy = "overlay"
	case w.overlay != nil || w.inspect || len(w.sinks) > 0 || len(w.frame) > 0:
		w.motionWhy = "inspected"
	default:
		w.motionWhy = "ok"
	}
	noteMovers(f.movers)
	w.motionGen = reactive.StateGen()
	w.motionSize = target.Bounds().Size()
}

// drawPartial paints a frame that only time and RepaintRect asked for over
// the last frame, which target holds when kept, within what changes now,
// and reports false, painting nothing, when the frame must be painted
// whole. It gives setDamage, when there is one, what changed.
func (w *Window) drawPartial(screen, target *ggfx.Image, kept bool, asked []Rect, setDamage func([]image.Rectangle)) bool {
	if w.motionWhy != "ok" {
		refusePartial(w.motionWhy)
		return false
	}
	if target.Bounds().Size() != w.motionSize || w.closed || w.root == nil {
		refusePartial("resized")
		return false
	}
	changed := func() bool { return reactive.StateGen() != w.motionGen || layoutGen() != w.seenGen }
	logical := Sz(w.canvas.dp(float64(screen.Bounds().Dx())), w.canvas.dp(float64(screen.Bounds().Dy())))
	if changed() || Untrack(w.viewport.Get) != logical {
		refusePartial("state changed")
		return false
	}
	window := Rct(Point{}, logical)
	now := w.motion
	if len(asked) > 0 {
		now = motionRegions(append(append(make([]Rect, 0, len(w.motion)+len(asked)), w.motion...), asked...), window)
	}
	damage, why := repaintFor(kept, now, window)
	if why != "" {
		refusePartial(why)
		return false
	}
	setRunning(&w.frameLoop)
	if err := w.settle(logical); err != nil {
		w.frameErr = err
		return true
	}
	if changed() || w.closed || w.root == nil {
		refusePartial("state changed")
		return false
	}
	c := &w.canvas
	c.Image = target
	c.window, c.onScreen = true, target == screen
	f := c.fs()
	f.startMotion()
	f.backdrop = false
	// Nothing painted registers anything, but what slips past the inert
	// Canvas is let go after.
	observers, shortcuts, reveals := len(c.inputObservers), len(c.shortcuts), len(c.revealRequests)
	background := w.background()
	f.partial = true
	func() {
		defer property.EnterLayout()()
		for _, d := range damage {
			f.damage = d
			at := c.Clip(d)
			at.inert = true
			at.Image.Fill(background)
			at.Paint(w.root, Rect{Size: w.rootSize})
			at.paintOverlays()
		}
	}()
	f.partial = false
	c.inputObservers, c.shortcuts, c.revealRequests = c.inputObservers[:observers], c.shortcuts[:shortcuts], c.revealRequests[:reveals]
	c.focusRequest, c.focusClaim = nil, nil
	// The layers are trimmed by whole frames alone: one painted in part
	// uses those of the regions it paints, and freeing the rest would make
	// the next whole frame allocate them again.
	partialFrames.Add(1)
	if setDamage != nil {
		px := make([]image.Rectangle, len(now))
		for i, r := range now {
			px[i] = c.physical(r)
		}
		setDamage(px)
	}
	w.noteMotion(target)
	w.presentBackdrop(screen, target)
	return true
}

// mostPartial is the share of the window past which a frame is painted
// whole: there, painting in part saves little, and paints more than once
// where regions are painted one by one.
const mostPartial = 0.6

// repaintFor is what must be painted over the last frame for it to show
// this one: now, what changes, already merged. When that is all of it, it
// says why instead: the last frame is not kept, or most of window changes.
func repaintFor(kept bool, now []Rect, window Rect) ([]Rect, string) {
	if !kept {
		return nil, "not kept"
	}
	area := 0.0
	for _, r := range now {
		area += r.Size.W * r.Size.H
	}
	if area > mostPartial*window.Size.W*window.Size.H {
		return nil, "too much changed"
	}
	return now, ""
}

// What moved and why frames were painted whole, for MotionReport.
var (
	motionMu        sync.Mutex
	lastMovers      []string
	partialRefusals = map[string]int64{}
)

func noteMovers(movers []string) {
	motionMu.Lock()
	lastMovers = append(lastMovers[:0], movers...)
	motionMu.Unlock()
}

// refusePartial counts a frame only time asked for that was painted whole,
// by why: "" when nothing had moved is not counted.
func refusePartial(why string) {
	if why == "" {
		return
	}
	motionMu.Lock()
	partialRefusals[why]++
	motionMu.Unlock()
}

// MotionReport is what moved in the last frame a window painted, by
// widget type, and how many frames since the program started could have
// been painted in part but were painted whole, by why: "dirty" when
// something else asked for them, "state changed", "overlay",
// "unplaced" when what moved was under a transform, "inspected",
// "resized", "not kept" when the last frame was not, "too much
// changed".
type MotionReport struct {
	Movers   []string
	Refusals map[string]int64
}

// ReadMotionReport reports what moves and what keeps frames whole. It is
// safe to call from any goroutine.
func ReadMotionReport() MotionReport {
	motionMu.Lock()
	defer motionMu.Unlock()
	r := MotionReport{Movers: slices.Clone(lastMovers), Refusals: make(map[string]int64, len(partialRefusals))}
	for k, v := range partialRefusals {
		r.Refusals[k] = v
	}
	return r
}

// RepaintRect asks for r, in the window's logical coordinates, to be
// painted again in the next frame, for a widget whose look changed with
// no signal written, as a terminal's does as output comes: if nothing else
// changed by then, the frame paints r alone, and what lies beside it is
// left as it was. Safe to call from any goroutine.
func (w *Window) RepaintRect(r Rect) {
	if r.Empty() {
		return
	}
	w.repaintMu.Lock()
	if len(w.repaints) >= mostRepaints {
		// Past so many, the one region around them all.
		for _, o := range w.repaints {
			r = cover(r, o)
		}
		w.repaints = w.repaints[:0]
	}
	w.repaints = append(w.repaints, r)
	w.repaintMu.Unlock()
	w.wakeFrame()
}

// mostRepaints is how many asked regions a frame keeps apart.
const mostRepaints = 32

// takeRepaints returns the regions asked for since the last frame.
func (w *Window) takeRepaints() []Rect {
	w.repaintMu.Lock()
	defer w.repaintMu.Unlock()
	out := slices.Clone(w.repaints)
	w.repaints = w.repaints[:0]
	return out
}

// WindowRect returns where r, in the coordinates dst paints in, lies in
// the window, for a widget to hand to RepaintRect later, and false where
// that cannot be told: under a Transform or a shape clip.
func (c *Canvas) WindowRect(r Rect) (Rect, bool) {
	if c == nil || c.space != nil {
		return Rect{}, false
	}
	return r, true
}
