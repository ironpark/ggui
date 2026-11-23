package widget

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"math"
	"os"
	"runtime"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggfx/svg"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/property"
)

// ParseSVG parses an SVG document from XML bytes, rejecting documents without
// a finite positive viewBox.
func ParseSVG(data []byte) (*svg.Document, error) {
	doc, err := svg.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ggui: parse SVG: %w", err)
	}
	x, y, w, h := doc.ViewBox()
	if w <= 0 || h <= 0 || math.IsNaN(x+y+w+h) || math.IsInf(x+y+w+h, 0) {
		return nil, errors.New("ggui: parse SVG: finite positive viewBox required")
	}
	return doc, nil
}

// LoadSVGFile reads and parses an SVG document from path.
func LoadSVGFile(path string) (*svg.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ggui: load SVG: %w", err)
	}
	return ParseSVG(data)
}

type svgDrive uint8

const (
	svgDriveNone svgDrive = iota
	svgDriveTime
	svgDriveProgress
)

// svgSlotState is retained across widget rebuilds via Canvas.Retain. Playhead
// lives here when no SVGController is attached; surface ALWAYS lives here so
// multiple widgets sharing one SVGController render safely at different sizes.
type svgSlotState struct {
	doc        *svg.Document
	ph         *svgPlayhead
	surface    *svgSurface
	overrides  svg.Overrides
	nodesRev   uint64
	firedEpoch uint64
}

var svgSlot = ggui.NewSlot[*svgSlotState]("svg state")

// SVGWidget renders an svg.Document with optional SMIL/CSS animation playback
// and programmatic per-node control via SVGController. Build one with SVG.
type SVGWidget struct {
	props  property.Owner
	doc    *svg.Document
	width  float64
	height float64
	fit    ggui.ImageFit
	alt    string
	color  property.Value[color.Color]

	ctrl   *SVGController
	id     any
	onDone func()

	autoPlay     bool
	autoPlaySet  bool
	speed        float64
	loop         int
	alternate    bool
	maxFPS       int
	reducedAt    time.Duration
	reducedAtSet bool

	drive    svgDrive
	timeProp property.Value[time.Duration]
	progProp property.Value[float64]

	// Captured in Layout, read in Paint without triggering layout re-measure.
	env     ggui.Env
	reduced bool
	ink     color.Color
	mounted bool

	// Retained across paints on this widget instance and across rebuilds via svgSlot.
	slot *svgSlotState

	// Optional test override for documents whose Timeline/Animated cannot yet
	// be populated by stubbed ggfx/svg backends. Nil in production.
	testTimeline *svg.Timeline
}

// SVG creates a widget displaying doc. A nil doc lays out to zero (or its
// explicit Size) and paints nothing. Animated documents auto-play by default
// unless AutoPlay(false) is set.
func SVG(doc *svg.Document) *SVGWidget {
	return &SVGWidget{
		doc:   doc,
		id:    reactive.AutoID(),
		speed: 1,
	}
}

// Size fixes both dimensions. A zero dimension follows the other, keeping the
// aspect ratio, or the viewBox when both are zero.
func (w *SVGWidget) Size(width, height float64) *SVGWidget {
	defer property.Watch(&w.props, &w.height)()
	defer property.Watch(&w.props, &w.width)()
	w.width, w.height = width, height
	return w
}

// Width fixes the width; the height follows the aspect ratio.
func (w *SVGWidget) Width(v float64) *SVGWidget {
	defer property.Watch(&w.props, &w.width)()
	w.width = v
	return w
}

// Height fixes the height; the width follows the aspect ratio.
func (w *SVGWidget) Height(v float64) *SVGWidget {
	defer property.Watch(&w.props, &w.height)()
	w.height = v
	return w
}

// Fit sets how the SVG is placed when the box has another shape; the default
// is ggui.FitContain.
func (w *SVGWidget) Fit(f ggui.ImageFit) *SVGWidget {
	defer property.Watch(&w.props, &w.fit)()
	w.fit = f
	return w
}

// Alt describes the SVG for a screen reader. Without one, the SVG is
// decorative and stays out of the accessibility tree.
func (w *SVGWidget) Alt(s string) *SVGWidget {
	defer property.Watch(&w.props, &w.alt)()
	w.alt = s
	return w
}

// Color sets the root currentColor for the SVG. Nil inherits the ambient text
// color from Env. EnvColor values are resolved during Paint.
func (w *SVGWidget) Color(c color.Color) *SVGWidget {
	property.Set(&w.props, &w.color, c)
	return w
}

// BindColor follows a non-nil color reader for the SVG's root currentColor.
func (w *SVGWidget) BindColor(r ggui.Readable[color.Color]) *SVGWidget {
	property.Bind(&w.props, &w.color, r, "SVG.BindColor")
	return w
}

// Controller attaches c for playback transport and programmatic node control.
func (w *SVGWidget) Controller(c *SVGController) *SVGWidget {
	if w.ctrl != c {
		w.ctrl = c
		w.props.Changed()
	}
	return w
}

// Key gives the SVG an explicit identity so retained playback state and its
// GPU surface follow the widget across rebuilds even when its Rect moves.
func (w *SVGWidget) Key(k any) *SVGWidget {
	w.id = k
	return w
}

// OnDone registers fn to be posted to the host when a finite playback run
// completes. Calling OnDone again replaces the previous callback; nil clears.
func (w *SVGWidget) OnDone(fn func()) *SVGWidget {
	w.onDone = fn
	return w
}

// AutoPlay controls whether an animated SVG starts playing automatically on
// first paint. Unset, it defaults to doc.Animated().
func (w *SVGWidget) AutoPlay(v bool) *SVGWidget {
	if !w.autoPlaySet || w.autoPlay != v {
		w.autoPlay = v
		w.autoPlaySet = true
		w.props.Changed()
	}
	return w
}

// Speed sets the playback rate multiplier (default 1). Negative or NaN values
// clamp to 0.
func (w *SVGWidget) Speed(rate float64) *SVGWidget {
	if math.IsNaN(rate) || rate < 0 {
		rate = 0
	}
	defer property.Watch(&w.props, &w.speed)()
	w.speed = rate
	return w
}

// Loop sets how many passes playback runs: 0 uses the SVG's authored timeline,
// n > 0 plays n passes of Timeline().Span, and n < 0 loops forever.
func (w *SVGWidget) Loop(n int) *SVGWidget {
	defer property.Watch(&w.props, &w.loop)()
	w.loop = n
	return w
}

// Alternate makes repeated passes reverse direction at each end (ping-pong).
func (w *SVGWidget) Alternate(v bool) *SVGWidget {
	defer property.Watch(&w.props, &w.alternate)()
	w.alternate = v
	return w
}

// MaxFPS caps animation stepping and rasterization rate in frames per second.
// Zero (or negative) uses the default of 60 FPS.
func (w *SVGWidget) MaxFPS(fps int) *SVGWidget {
	if fps < 0 {
		fps = 0
	}
	defer property.Watch(&w.props, &w.maxFPS)()
	w.maxFPS = fps
	return w
}

// ReducedMotionAt sets the static timestamp shown when the environment
// requests reduced motion.
func (w *SVGWidget) ReducedMotionAt(t time.Duration) *SVGWidget {
	if t < 0 {
		t = 0
	}
	if !w.reducedAtSet || w.reducedAt != t {
		w.reducedAt = t
		w.reducedAtSet = true
		w.props.Changed()
	}
	return w
}

// Time drives the SVG at explicit timestamp t, dropping any BindTime or
// BindProgress reader.
func (w *SVGWidget) Time(t time.Duration) *SVGWidget {
	changed := w.drive != svgDriveTime
	w.drive = svgDriveTime
	w.progProp.Set(0)
	if w.timeProp.Set(t) {
		changed = true
	}
	if w.ctrl != nil && w.mounted {
		w.ctrl.releasedBound = false
	}
	if changed {
		w.props.Changed()
	}
	return w
}

// BindTime follows a non-nil duration reader for the SVG timestamp during
// Paint. Any transport call on an attached SVGController takes playback back.
func (w *SVGWidget) BindTime(r ggui.Readable[time.Duration]) *SVGWidget {
	changed := w.timeProp.Bind(r, "SVG.BindTime")
	w.progProp.Set(0)
	if w.drive != svgDriveTime {
		w.drive = svgDriveTime
		changed = true
	}
	if w.ctrl != nil && w.mounted {
		w.ctrl.releasedBound = false
	}
	if changed {
		w.props.Changed()
	}
	return w
}

// Progress drives the SVG at normalized progress p in [0, 1] of Timeline.Span,
// dropping any BindTime or BindProgress reader.
func (w *SVGWidget) Progress(p float64) *SVGWidget {
	changed := w.drive != svgDriveProgress
	w.drive = svgDriveProgress
	w.timeProp.Set(0)
	if w.progProp.Set(p) {
		changed = true
	}
	if w.ctrl != nil && w.mounted {
		w.ctrl.releasedBound = false
	}
	if changed {
		w.props.Changed()
	}
	return w
}

// BindProgress follows a non-nil progress reader in [0, 1] during Paint. Any
// transport call on an attached SVGController takes playback back.
func (w *SVGWidget) BindProgress(r ggui.Readable[float64]) *SVGWidget {
	changed := w.progProp.Bind(r, "SVG.BindProgress")
	w.timeProp.Set(0)
	if w.drive != svgDriveProgress {
		w.drive = svgDriveProgress
		changed = true
	}
	if w.ctrl != nil && w.mounted {
		w.ctrl.releasedBound = false
	}
	if changed {
		w.props.Changed()
	}
	return w
}

func (w *SVGWidget) natural() ggui.Size {
	if w.doc == nil {
		return ggui.Size{}
	}
	_, _, vw, vh := w.doc.ViewBox()
	if vw <= 0 || vh <= 0 || math.IsNaN(vw+vh) || math.IsInf(vw+vh, 0) {
		return ggui.Size{}
	}
	return ggui.Sz(vw, vh)
}

func (w *SVGWidget) timeline() svg.Timeline {
	if w.testTimeline != nil {
		return *w.testTimeline
	}
	if w.doc == nil {
		return svg.Timeline{}
	}
	return w.doc.Timeline()
}

func (w *SVGWidget) animated() bool {
	if w.testTimeline != nil {
		return w.testTimeline.Span > 0 || w.testTimeline.Infinite
	}
	if w.doc == nil {
		return false
	}
	return w.doc.Animated()
}

// Layout implements ggui.Widget. It reads only layout sizing properties and Env
// metadata; bound colors, timestamps, and controller node properties are read
// during Paint so animations cause zero layout re-measurement.
func (w *SVGWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer w.props.Layout().EndLayout()
	w.env = env
	w.reduced = env.ReducedMotion()
	w.ink = env.ResolveText(ggui.TextStyle{}).Color
	w.mounted = true

	return aspectFit(w.natural(), w.width, w.height, c)
}

// Paint implements ggui.Widget.
func (w *SVGWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	var postDone func()
	defer func() {
		if postDone != nil {
			postDone()
		}
	}()

	if dst != nil && w.alt != "" {
		dst.Leaf(r, ggui.Node{Role: ggui.RoleImage, Name: w.alt})
	}
	if w.doc == nil || r.Size.W <= 0 || r.Size.H <= 0 ||
		math.IsNaN(r.Origin.X+r.Origin.Y+r.Size.W+r.Size.H) ||
		math.IsInf(r.Origin.X+r.Origin.Y+r.Size.W+r.Size.H, 0) {
		return
	}

	w.retainSlot(dst, r)
	tl := w.timeline()
	ph := w.playhead(tl)

	// Offscreen pause: reset clock anchor, avoid waking the frame clock, skip
	// rasterization, while keeping the retained slot alive.
	frameNow := ggui.FrameTime()
	if dst != nil && dst.Visible(r).Empty() {
		ph.markOffscreen(frameNow)
		return
	}
	ph.markVisible(frameNow)
	w.advancePlayhead(ph, tl, frameNow)

	postDone = w.deliverDone(ph)

	frame := w.resolveFrame(ph.evalTime(tl))

	// Headless probes without RenderTo still run clock, state and semantics
	// above, but skip GPU rasterization and texture allocation.
	if dst == nil || dst.Image == nil {
		return
	}
	w.paintFrame(dst, r, frame)
}

// retainSlot adopts the state a widget showing the same document at the
// same anchor retained, or starts its own, and retains it for the next
// paint.
func (w *SVGWidget) retainSlot(dst *ggui.Canvas, r ggui.Rect) {
	atAnchor := ggui.Anchor{Rect: r, ID: w.id}
	if w.slot == nil {
		if prev, ok := dst.Retained(atAnchor, svgSlot); ok && prev != nil && prev.doc == w.doc {
			w.slot = prev
		} else {
			w.slot = &svgSlotState{doc: w.doc}
		}
	}
	dst.Retain(atAnchor, svgSlot, w.slot)
}

// playhead returns the playhead this paint moves, the controller's or the
// slot's own, started on first use: playing at once when it auto-plays
// and nothing drives its time.
func (w *SVGWidget) playhead(tl svg.Timeline) *svgPlayhead {
	var ph *svgPlayhead
	if w.ctrl != nil {
		// Mark controller mounted without recording in Layout.
		w.ctrl.props.Read()
		w.ctrl.attach(w.doc, tl)
		ph = &w.ctrl.ph
	} else {
		if w.slot.ph == nil {
			w.slot.ph = &svgPlayhead{dir: 1}
		}
		ph = w.slot.ph
	}
	if w.slot.firedEpoch > ph.doneEpoch {
		w.slot.firedEpoch = ph.doneEpoch
	}

	if !ph.init {
		ph.init = true
		if ph.dir == 0 {
			ph.dir = 1
		}
		auto := w.animated()
		if w.autoPlaySet {
			auto = w.autoPlay
		}
		if auto && w.drive == svgDriveNone && (w.ctrl == nil || !w.ctrl.releasedBound) {
			ph.playing = true
		}
	}
	return ph
}

// advancePlayhead advances or evaluates ph for this frame: it sets it to
// the bound time or progress, rests it where reduced motion asks, or moves
// it on by the time played.
func (w *SVGWidget) advancePlayhead(ph *svgPlayhead, tl svg.Timeline, now time.Time) {
	switch {
	case w.drive != svgDriveNone && (w.ctrl == nil || !w.ctrl.releasedBound):
		if w.drive == svgDriveTime {
			t := max(w.timeProp.Get(), 0)
			ph.pos = t
		} else {
			ph.pos = progressToTime(w.progProp.Get(), tl.Span)
		}
		ph.playing = false
		ph.anchor = time.Time{}

	case w.reduced:
		w.restReduced(ph, tl)

	case ph.playing:
		w.stepPlaying(ph, tl, now)
	}
}

// restReduced rests ph where reduced motion holds it: at ReducedMotionAt
// when set, else, when it was playing, at the start of an endless run or
// at the end a finite one plays toward, which completes the run.
func (w *SVGWidget) restReduced(ph *svgPlayhead, tl svg.Timeline) {
	ph.anchor = time.Time{}
	wasPlaying := ph.playing
	if w.reducedAtSet {
		ph.pos = w.reducedAt
		ph.playing = false
		if wasPlaying && !tl.Infinite && w.loop >= 0 && w.animated() {
			ph.markDone()
		}
	} else if wasPlaying {
		if tl.Infinite || w.loop < 0 {
			ph.pos = 0
			ph.playing = false
		} else {
			if ph.dir < 0 {
				ph.pos = 0
			} else {
				ph.pos = tl.Span
			}
			ph.playing = false
			if w.animated() {
				ph.markDone()
			}
		}
	}
}

// stepPlaying moves a playing ph on by the frame time since its anchor,
// quantized to MaxFPS and scaled by Speed, and paces the next frame. A
// timeline with nothing to play stops it.
func (w *SVGWidget) stepPlaying(ph *svgPlayhead, tl svg.Timeline, now time.Time) {
	if tl.Span <= 0 && !tl.Infinite {
		ph.playing = false
		ph.anchor = time.Time{}
		return
	}
	fps := w.maxFPS
	if fps <= 0 {
		fps = 60
	}
	step := time.Second / time.Duration(fps)
	q := now.Truncate(step)
	if !ph.anchor.IsZero() && q.After(ph.anchor) && w.speed > 0 {
		dt := time.Duration(float64(q.Sub(ph.anchor)) * w.speed)
		if ph.advance(dt, tl, w.loop, w.alternate) {
			ph.markDone()
		}
	}
	ph.anchor = q
	if ph.playing && w.speed > 0 {
		ggui.Paced(step)
	}
}

// svgFrame is what a paint resolves before it rasterizes: the document
// time, the colour currentColor takes, and whether controller overrides
// apply.
type svgFrame struct {
	t         time.Duration
	color     color.Color
	overrides bool
}

// resolveFrame resolves, in Paint, the root colour and every controller
// node binding for the frame at document time t.
func (w *SVGWidget) resolveFrame(t time.Duration) svgFrame {
	f := svgFrame{t: t, color: resolveSVGColor(w.color.Get(), w.env)}
	if f.color == nil {
		f.color = resolveSVGColor(w.ink, w.env)
	}
	if f.color == nil {
		f.color = color.Black
	}
	if w.ctrl != nil {
		f.overrides = w.ctrl.resolveInto(&w.slot.overrides, &w.slot.nodesRev, w.env)
	} else if len(w.slot.overrides) > 0 {
		clear(w.slot.overrides)
		w.slot.nodesRev++
	}
	return f
}

// paintFrame rasterizes frame at the pixel size its fit in r takes and
// draws it there, clipped to r.
func (w *SVGWidget) paintFrame(dst *ggui.Canvas, r ggui.Rect, frame svgFrame) {
	nat := w.natural()
	if nat.W <= 0 || nat.H <= 0 {
		return
	}
	at := w.fit.Place(nat, r)
	scale := dst.Scale()
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		scale = 1
	}
	pw, phPx := svgPixelSize(at.Size, scale)
	if pw <= 0 || phPx <= 0 {
		return
	}

	vx, vy, vw, vh := w.doc.ViewBox()
	var geoM ggfx.GeoM
	geoM.Translate(-vx, -vy)
	geoM.Scale(float64(pw)/vw, float64(phPx)/vh)

	img := w.texture(pw, phPx, geoM, frame)
	if img == nil {
		return
	}
	target := dst
	if at != r {
		target = dst.Clip(r)
	}
	target.DrawImage(img, at, ggui.ImageOptions{Fit: ggui.FitFill})
	runtime.KeepAlive(w.slot.surface)
	runtime.KeepAlive(w.doc)
}

// texture returns frame rasterized pw by phPx pixels through geoM: from
// the shared cache for a static document, else in the slot's own surface.
func (w *SVGWidget) texture(pw, phPx int, geoM ggfx.GeoM, frame svgFrame) *ggfx.Image {
	isStatic := !w.animated() && !frame.overrides
	if isStatic {
		var colKey uint32
		if w.doc.UsesCurrentColor() {
			colKey = packRGBA8(frame.color)
		}
		if w.slot.surface != nil {
			w.slot.surface.releaseTexture()
		}
		img, _ := svgTextures.getStatic(w.doc, pw, phPx, colKey, frame.color, geoM)
		return img
	}
	if w.slot.surface == nil {
		w.slot.surface = newSVGSurface(w.doc)
	}
	key := svgKey{
		doc:   w.doc,
		w:     int32(pw),
		h:     int32(phPx),
		t:     frame.t,
		color: packRGBA8(frame.color),
		nodes: w.slot.nodesRev,
	}
	opts := svg.DrawOptions{
		Time:         frame.t,
		CurrentColor: frame.color,
		Overrides:    w.slot.overrides,
	}
	return w.slot.surface.render(key, geoM, &opts)
}

func (w *SVGWidget) deliverDone(ph *svgPlayhead) func() {
	if w.slot.firedEpoch >= ph.doneEpoch {
		return nil
	}
	w.slot.firedEpoch = ph.doneEpoch
	fn := w.onDone
	if fn == nil {
		return nil
	}
	if host, ok := ggui.LookupHost(); ok && host != nil {
		host.Post(fn)
		return nil
	}
	return fn
}

func svgPixelSize(sz ggui.Size, scale float64) (int, int) {
	if sz.W <= 0 || sz.H <= 0 || math.IsNaN(sz.W+sz.H) || math.IsInf(sz.W+sz.H, 0) {
		return 0, 0
	}
	// Subtract a tiny epsilon before Ceil so IEEE 754 rounding from
	// Fit.Place (e.g. 100*(14/100) == 14.000000000000002) stays at 14px.
	pwF := max(1, math.Ceil(sz.W*scale-1e-9))
	phF := max(1, math.Ceil(sz.H*scale-1e-9))
	if math.IsNaN(pwF+phF) || math.IsInf(pwF+phF, 0) {
		return 0, 0
	}
	if pwF > svgMaxSide || phF > svgMaxSide {
		k := svgMaxSide / max(pwF, phF)
		pwF = math.Round(pwF * k)
		phF = math.Round(phF * k)
	}
	return max(1, int(pwF)), max(1, int(phF))
}
