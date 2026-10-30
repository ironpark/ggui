package ggui

import (
	"slices"
	"sync"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
)

// A frame that only time asks for, because a widget read the clock, eased
// a value, or asked to be painted again at an instant, changes only where
// those widgets paint: a caret blinking, a dot breathing, a hover fading.
// The window keeps the last frame in its backbuffer and paints such a
// frame there alone, within those regions, rather than the whole window
// again for a few pixels. Anything else that asks for a frame, input, a
// signal written, posted work, a layout, a resize, paints it whole.
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

// motionFrames is how many frames the backbuffer is kept after the last
// one that had a motion region.
const motionFrames = 120

// startMotion clears what the last frame noted of its motion.
func (f *frameState) startMotion() {
	f.scopes, f.motion, f.movers = f.scopes[:0], f.motion[:0], f.movers[:0]
	f.motionLost, f.overlaid = false, false
}

// noteMotion decides, after a frame painted into target, whether the next
// may be painted in part, and where.
func (w *Window) noteMotion(screen, target *ggfx.Image) {
	f := w.canvas.fs()
	if len(f.motion) > 0 {
		w.motionTTL = motionFrames
	} else if w.motionTTL > 0 {
		w.motionTTL--
	}
	w.motion = motionRegions(f.motion, Rct(Point{}, f.logical))
	// Painted in part, the frame keeps the input, semantics and overlays of
	// the last whole one: none may have been painted, nor anything that
	// reads every frame or traces it.
	switch {
	case len(w.motion) == 0:
		w.motionWhy = ""
	case f.motionLost:
		w.motionWhy = "unplaced"
	case f.overlaid:
		w.motionWhy = "overlay"
	case target == screen:
		w.motionWhy = "no backbuffer"
	case w.overlay != nil || w.inspect || len(w.sinks) > 0 || len(w.frame) > 0:
		w.motionWhy = "inspected"
	default:
		w.motionWhy = "ok"
	}
	w.motionOK = w.motionWhy == "ok"
	noteMovers(f.movers)
	w.motionGen = reactive.StateGen()
	w.motionSize = target.Bounds().Size()
}

// drawPartial paints a frame that only time asked for within the last
// frame's motion regions, onto the last frame in target, and reports
// false, painting nothing, when the frame must be painted whole.
func (w *Window) drawPartial(screen, target *ggfx.Image) bool {
	if !w.motionOK {
		refusePartial(w.motionWhy)
		return false
	}
	if target == screen || target.Bounds().Size() != w.motionSize || w.closed || w.root == nil {
		refusePartial("resized")
		return false
	}
	changed := func() bool { return reactive.StateGen() != w.motionGen || layoutGen() != w.seenGen }
	logical := Sz(w.canvas.dp(float64(screen.Bounds().Dx())), w.canvas.dp(float64(screen.Bounds().Dy())))
	if changed() || Untrack(w.viewport.Get) != logical {
		refusePartial("state changed")
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
	c.window, c.onScreen = true, false
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
		for _, d := range w.motion {
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
	c.trimLayers()
	partialFrames.Add(1)
	w.noteMotion(screen, target)
	w.presentBackdrop(screen, target)
	return true
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
// "resized", "no backbuffer".
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
