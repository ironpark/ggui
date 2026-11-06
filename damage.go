package ggui

import (
	"image"
	"runtime"
	"slices"
	"strings"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/framestats"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/region"
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
		p.bounds = p.bounds.Cover(s.bounds)
	}
}

// drew adds r, with extra around it, to the bounds of the widget painting.
func (f *frameState) drew(r Rect, extra float64) {
	if n := len(f.scopes); n > 0 && !r.Empty() {
		s := &f.scopes[n-1]
		s.bounds = s.bounds.Cover(r.Grow(extra))
	}
}

// skips reports whether a partial frame leaves a widget at r unpainted:
// one outside a Transform or a shape clip whose Rect, with damageReach
// around it, misses the region being painted.
func (f *frameState) skips(c *Canvas, r Rect) bool {
	return f.partial && c.space == nil && !r.Empty() && !r.Grow(damageReach).Overlaps(f.damage)
}

// motionRegions are a frame's motion regions as the next frame paints
// them: grown by motionSlack, cut to the window, and merged where they
// overlap, or into one when there are many.
func motionRegions(motion []Rect, window Rect) []Rect {
	return region.Merge(motion, window, motionSlack, mostMotionRegions)
}

// motionState is what a frame that only time asks for repaints. scopes
// are the widgets being painted, motion the regions of those that read the
// time, and motionLost that one did where it cannot be placed. partial is
// set while such a frame paints within damage.
type motionState struct {
	scopes     []paintScope
	motion     []Rect
	motionLost bool
	movers     []string // the widgets behind motion, by type
	partial    bool
	damage     Rect
	overlaid   bool // an overlay was painted this frame
}

// startMotion clears what the last frame noted of its motion.
func (f *motionState) startMotion() {
	f.scopes, f.motion, f.movers = f.scopes[:0], f.motion[:0], f.movers[:0]
	f.motionLost, f.overlaid = false, false
}

// noteMotion decides, after a frame painted into target, whether the next
// may be painted in part, and where.
func (w *Window) noteMotion(target *ggfx.Image) {
	f := w.canvas.fs()
	w.motion = motionRegions(f.motion, Rct(Point{}, f.logical))
	w.motionLost = f.motionLost
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
	framestats.SetMovers(f.movers)
	w.motionGen = reactive.StateGen()
	w.motionSize = target.Bounds().Size()
	w.freeSeen, w.fontSeen = reactive.FreeGen(), fontGen()
}

// A scroll that moves its content changes what its viewport shows and
// nothing else: its writes and layout requests are made in
// reactive.Contained, and it notes its viewport with noteContained. A
// frame for which nothing else changed, no write or layout outside a
// Contained, is laid out and painted as a whole frame is, every widget
// registering its regions, semantics and shortcuts, but into the part of
// the screen the viewports cover alone: a widget elsewhere draws into an
// image it misses, which draws nothing, and what the screen kept of the
// last frame shows there.

// mostContained is the share of the window past which a frame a scroll
// contains is painted whole: one pass either way, so higher than
// mostPartial.
const mostContained = 0.85

// noteContained records r, in window coordinates, as all that a change
// just made shows in, for the frame running on this goroutine.
func noteContained(r Rect) {
	if l, ok := reactive.RunningLoop().(*frameLoop); ok {
		l.contained = append(l.contained, r)
		l.watchFree()
	}
}

// mostFreeWriters is how many writes outside a scroll a frame keeps the
// writers of.
const mostFreeWriters = 16

// watchFree has the loop note who writes outside Contained until a frame
// with nothing contained, to name what keeps a scroll's frame whole.
func (l *frameLoop) watchFree() {
	if l.watchingFree {
		return
	}
	l.watchingFree = true
	reactive.WatchFree(func() {
		if len(l.freeWriters) < mostFreeWriters {
			l.freeWriters = append(l.freeWriters, freeWriter())
		}
	})
}

// endFrameFree ends a frame's watch for writes outside a scroll: they are
// counted when refused says the frame was painted whole for them, and the
// watch stops after a frame with nothing contained.
func (l *frameLoop) endFrameFree(contained, refused bool) {
	if refused {
		for _, pc := range l.freeWriters {
			noteDirtier(dirtyFree, pc)
		}
	}
	l.freeWriters = l.freeWriters[:0]
	if !contained && l.watchingFree {
		l.watchingFree = false
		reactive.WatchFree(nil)
	}
}

// freeWriter returns the call site that wrote, past the reactive package
// and ggui's state types.
func freeWriter() uintptr {
	var pcs [16]uintptr
	n := runtime.Callers(3, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	var last uintptr
	for {
		f, more := frames.Next()
		last = f.PC
		switch {
		case strings.HasPrefix(f.Function, "github.com/ironpark/ggui/internal/reactive."),
			strings.HasPrefix(f.Function, "github.com/ironpark/ggui.") && (strings.Contains(f.Function, "StateValue[") || strings.Contains(f.Function, "Binding[")):
		default:
			return f.PC
		}
		if !more {
			return last
		}
	}
}

// takeContained returns the regions noted since the last frame.
func (w *Window) takeContained() []Rect {
	rs := w.frameLoop.contained
	w.frameLoop.contained = w.frameLoop.contained[:0]
	return rs
}

// containedDamage is the part of the screen, in pixels, a frame the
// regions contain paints, or empty, saying why, when it must be painted
// whole.
func (w *Window) containedDamage(target *ggfx.Image, kept bool, contained, asked []Rect) image.Rectangle {
	why := w.partialRefusal(target)
	switch {
	case !kept:
		why = "not kept"
	case why != "":
	case reactive.FreeGen() != w.freeSeen || fontGen() != w.fontSeen:
		why = "state changed"
		w.frameLoop.endFrameFree(true, true)
	}
	if why != "" {
		framestats.Refuse(why)
		return image.Rectangle{}
	}
	window := Rct(Point{}, w.logicalSize(target))
	all := region.CoverAll(contained, w.motion, asked)
	all = all.Intersect(window)
	if all.Empty() || all.Size.W*all.Size.H > mostContained*window.Size.W*window.Size.H {
		framestats.Refuse("too much changed")
		return image.Rectangle{}
	}
	return w.canvas.physical(all).Intersect(target.Bounds())
}

// drawPartial paints a frame that only time and RepaintRect asked for over
// the last frame, which target holds when kept, within what changes now,
// and reports false, painting nothing, when the frame must be painted
// whole. It gives setDamage, when there is one, what changed.
func (w *Window) drawPartial(screen, target *ggfx.Image, kept bool, asked []Rect, setDamage func([]image.Rectangle)) bool {
	if why := w.partialRefusal(target); why != "" {
		framestats.Refuse(why)
		return false
	}
	changed := func() bool { return reactive.StateGen() != w.motionGen || layoutGen() != w.seenGen }
	logical := w.logicalSize(screen)
	if changed() || Untrack(w.viewport.Get) != logical {
		framestats.Refuse("state changed")
		return false
	}
	window := Rct(Point{}, logical)
	now := w.motion
	if len(asked) > 0 {
		now = motionRegions(slices.Concat(w.motion, asked), window)
	}
	damage, why := repaintFor(kept, now, window)
	if why != "" {
		framestats.Refuse(why)
		return false
	}
	setRunning(&w.frameLoop)
	if err := w.settle(logical); err != nil {
		w.frameErr = err
		return true
	}
	if changed() || w.closed || w.root == nil {
		framestats.Refuse("state changed")
		return false
	}
	c := &w.canvas
	readable := screen.CanBeSource()
	w.aim(target, target, screen, readable)
	f := w.beginPaint(c)
	defer endPaint(f)
	// Painted inert, nothing registers input, requests focus or a reveal,
	// observes input or listens for a shortcut: what the last whole frame
	// registered stands.
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
	// The layers are trimmed by whole frames alone: one painted in part
	// uses those of the regions it paints, and freeing the rest would make
	// the next whole frame allocate them again.
	framestats.CountPartial()
	if setDamage != nil {
		px := make([]image.Rectangle, len(now))
		for i, r := range now {
			px[i] = c.physical(r)
		}
		setDamage(px)
	}
	w.noteMotion(target)
	w.presentBackdrop(screen, target, readable)
	return true
}

// partialRefusal is why the next frame cannot be painted over the last one
// into target, which noteMotion judged, or "" when it may be.
func (w *Window) partialRefusal(target *ggfx.Image) string {
	switch {
	case w.motionWhy != "ok":
		return w.motionWhy
	case target.Bounds().Size() != w.motionSize || w.closed || w.root == nil:
		return "resized"
	}
	return ""
}

// logicalSize is the size of img in logical pixels.
func (w *Window) logicalSize(img *ggfx.Image) Size {
	b := img.Bounds()
	return Sz(w.canvas.dp(float64(b.Dx())), w.canvas.dp(float64(b.Dy())))
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

// dirtyReason records what marked the window dirty, keeping partial
// redraw from taking the frame.
type dirtyReason uint32

const (
	dirtyRepaint dirtyReason = 1 << iota
	dirtySignal
	dirtyLayout
	dirtyEvent
	dirtyAnim
	dirtyCallback
	dirtyInput
	dirtyScale
	dirtyOther
	// dirtyFree is never a dirty bit: it names, for a frame a scroll
	// contained that was painted whole for it, a write made outside.
	dirtyFree
)

var dirtyReasons = []struct {
	flag dirtyReason
	name string
}{
	{dirtyRepaint, "dirty(repaint)"},
	{dirtySignal, "dirty(signal)"},
	{dirtyLayout, "dirty(layout)"},
	{dirtyEvent, "dirty(event)"},
	{dirtyAnim, "dirty(anim)"},
	{dirtyCallback, "dirty(callback)"},
	{dirtyInput, "dirty(input)"},
	{dirtyScale, "dirty(scale)"},
	{dirtyOther, "dirty(other)"},
	{dirtyFree, "state changed"},
}

// name is how a report names d.
func (d dirtyReason) name() string {
	for _, r := range dirtyReasons {
		if r.flag == d {
			return r.name
		}
	}
	return "dirty"
}

// noteDirtier counts pc, a function's entry or a call site, as having
// marked a frame dirty for why.
func noteDirtier(why dirtyReason, pc uintptr) { framestats.Dirtied(why.name(), pc) }

// noteCaller counts the caller of the function calling it.
func noteCaller(why dirtyReason) {
	var pc [1]uintptr
	if runtime.Callers(3, pc[:]) == 1 {
		noteDirtier(why, pc[0])
	}
}

// callStack is the call stack past a function that recorded it, kept
// unresolved: what in an app or a widget package started an animation,
// say. Turning program counters into frames is what cost an animation's
// start most of its time, and a report needs only the animations still
// running when a frame is counted, so outside resolves it then, once.
type callStack struct {
	pcs [12]uintptr
	n   int
	pc  uintptr // the call site outside this package, once resolved
}

// callerStack records the stack past the function calling it.
func callerStack() (s callStack) {
	s.n = runtime.Callers(3, s.pcs[:])
	return s
}

// outside returns the first call site in s outside this package,
// resolving it on the first call.
func (s *callStack) outside() uintptr {
	if s.pc != 0 || s.n == 0 {
		return s.pc
	}
	frames := runtime.CallersFrames(s.pcs[:s.n])
	for {
		f, more := frames.Next()
		if !strings.HasPrefix(f.Function, "github.com/ironpark/ggui.") || !more {
			s.pc = f.PC
			return s.pc
		}
	}
}

// refusePartialDirty counts a frame painted whole for each of bits.
func refusePartialDirty(bits dirtyReason) {
	for _, d := range dirtyReasons {
		if bits&d.flag != 0 {
			framestats.Refuse(d.name)
		}
	}
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
	w.addRepaintLocked(r)
	w.repaintMu.Unlock()
	w.wakeFrame()
}

// addRepaints asks for rs, grown by damageReach for what a widget draws
// around its region, to be painted again in the frame being run.
func (w *Window) addRepaints(rs []Rect) {
	if len(rs) == 0 {
		return
	}
	w.repaintMu.Lock()
	defer w.repaintMu.Unlock()
	for _, r := range rs {
		if !r.Empty() {
			w.addRepaintLocked(r.Grow(damageReach))
		}
	}
}

// addRepaintLocked keeps r among the asked regions; past mostRepaints of
// them, they become the one region around them all.
func (w *Window) addRepaintLocked(r Rect) {
	if len(w.repaints) >= mostRepaints {
		r = r.Cover(region.CoverAll(w.repaints))
		w.repaints = w.repaints[:0]
	}
	w.repaints = append(w.repaints, r)
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
