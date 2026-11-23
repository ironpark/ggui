package widget

import (
	"image/color"
	"math"
	"time"

	"github.com/ironpark/ggfx/svg"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/gem"
	"github.com/ironpark/ggui/property"
)

// svgPlayhead tracks playback state for either an SVGController (shared by all
// widgets attached to it) or a standalone SVGWidget (retained via svgSlot).
type svgPlayhead struct {
	init      bool
	playing   bool
	dir       int           // +1 forward, -1 reverse (0 treated as +1)
	pos       time.Duration // current playhead position
	pass      int           // completed passes in the current run
	doneEpoch uint64        // monotonically incremented each time a finite run completes
	anchor    time.Time     // last quantized frame timestamp while playing

	// Shared-controller offscreen coordination: if one widget on a shared
	// controller paints offscreen earlier in the same frame than a visible
	// widget, the visible widget restores savedAnchor when clearedAt == now.
	visAt       time.Time
	savedAnchor time.Time
	clearedAt   time.Time
}

// markDone records a finite playback completion, advancing doneEpoch so every
// attached widget can fire its OnDone callback once.
func (p *svgPlayhead) markDone() {
	p.doneEpoch++
}

// markOffscreen clears the clock anchor unless another visible widget on the
// same playhead already painted in this frame.
func (p *svgPlayhead) markOffscreen(now time.Time) {
	if p.visAt == now {
		return
	}
	if !p.anchor.IsZero() {
		p.savedAnchor = p.anchor
		p.clearedAt = now
		p.anchor = time.Time{}
	}
}

// markVisible restores a clock anchor cleared earlier in the same frame by an
// offscreen sibling sharing this playhead, and marks this frame visible.
func (p *svgPlayhead) markVisible(now time.Time) {
	if p.anchor.IsZero() && p.clearedAt == now && !p.savedAnchor.IsZero() {
		p.anchor = p.savedAnchor
	}
	p.visAt = now
	p.savedAnchor = time.Time{}
	p.clearedAt = time.Time{}
}

// advance steps p forward or backward by dt >= 0 according to tl, loop and
// alternate. It reports true when a finite run reaches its end.
func (p *svgPlayhead) advance(dt time.Duration, tl svg.Timeline, loop int, alternate bool) (finished bool) {
	if dt <= 0 {
		return false
	}
	if p.dir == 0 {
		p.dir = 1
	}
	span := tl.Span

	// Authored infinite timeline (loop == 0 && tl.Infinite): play indefinitely
	// when moving forward, or stop at 0 when reversing.
	if loop == 0 && tl.Infinite {
		if p.dir > 0 {
			p.pos += dt
			if tl.Loop > 0 && p.pos >= tl.LoopStart {
				p.pos = tl.LoopStart + (p.pos-tl.LoopStart)%tl.Loop
			}
			return false
		}
		if dt >= p.pos {
			p.pos = 0
			p.playing = false
			return true
		}
		p.pos -= dt
		return false
	}

	if span <= 0 {
		p.pos = 0
		p.playing = false
		return false
	}

	// Infinite looping over [0, span] (loop < 0).
	if loop < 0 {
		cycle := span
		if alternate {
			cycle = 2 * span
		}
		rem := dt % cycle
		for rem > 0 {
			if p.dir > 0 {
				toEnd := span - p.pos
				if rem < toEnd {
					p.pos += rem
					rem = 0
				} else {
					rem -= toEnd
					if alternate {
						p.pos = span
						p.dir = -1
					} else {
						p.pos = 0
					}
				}
			} else {
				toStart := p.pos
				if rem < toStart {
					p.pos -= rem
					rem = 0
				} else {
					rem -= toStart
					if alternate {
						p.pos = 0
						p.dir = 1
					} else {
						p.pos = span
					}
				}
			}
		}
		return false
	}

	// Finite run: loop > 0 passes, or loop == 0 -> 1 authored finite pass.
	totalPasses := loop
	if totalPasses == 0 {
		totalPasses = 1
	}
	rem := dt
	for rem > 0 {
		// Fast-forward full passes if starting from an endpoint.
		if (p.pos == 0 && p.dir > 0) || (p.pos == span && p.dir < 0) {
			full := int(rem / span)
			left := totalPasses - p.pass
			if full >= left {
				// Consume all remaining passes and land on the final endpoint.
				finalDir := p.dir
				if alternate && (left-1)%2 == 1 {
					finalDir = -finalDir
				}
				p.pass = totalPasses
				if finalDir > 0 {
					p.pos = span
				} else {
					p.pos = 0
				}
				p.playing = false
				return true
			}
			if full > 0 {
				p.pass += full
				rem -= time.Duration(full) * span
				if alternate && full%2 == 1 {
					p.dir = -p.dir
					if p.dir > 0 {
						p.pos = 0
					} else {
						p.pos = span
					}
				}
				if rem == 0 {
					return false
				}
			}
		}
		if p.dir > 0 {
			toEnd := span - p.pos
			if rem < toEnd {
				p.pos += rem
				rem = 0
			} else {
				rem -= toEnd
				p.pass++
				if p.pass >= totalPasses {
					p.pos = span
					p.playing = false
					return true
				}
				if alternate {
					p.pos = span
					p.dir = -1
				} else {
					p.pos = 0
				}
			}
		} else {
			toStart := p.pos
			if rem < toStart {
				p.pos -= rem
				rem = 0
			} else {
				rem -= toStart
				p.pass++
				if p.pass >= totalPasses {
					p.pos = 0
					p.playing = false
					return true
				}
				if alternate {
					p.pos = 0
					p.dir = 1
				} else {
					p.pos = span
				}
			}
		}
	}
	return false
}

// evalTime returns the document timestamp corresponding to p's current position.
func (p *svgPlayhead) evalTime(tl svg.Timeline) time.Duration {
	t := max(p.pos, 0)
	if tl.Infinite && tl.Loop > 0 && t >= tl.LoopStart {
		t = tl.LoopStart + (t-tl.LoopStart)%tl.Loop
	}
	return t
}

func progressToTime(p float64, span time.Duration) time.Duration {
	if math.IsNaN(p) || span <= 0 {
		return 0
	}
	p = gem.Clamp(p, 0, 1)
	return time.Duration(math.Round(p * float64(span)))
}

// SVGController drives playback and per-node programmatic overrides for one or
// more SVG widgets. The zero value is ready to use; attach it with
// SVGWidget.Controller.
//
//	var ctrl widget.SVGController
//	ctrl.Node("needle").BindRotate(angle)
//	widget.SVG(gaugeDoc).Controller(&ctrl)
//
// Controller properties and node bindings are read during Paint rather than
// Layout, so ticking a Tween or Signal bound to a node never triggers layout
// re-measurement.
type SVGController struct {
	props property.Owner
	doc   *svg.Document
	tl    svg.Timeline

	attached        bool
	releasedBound   bool // true once a transport method took playhead from BindTime/BindProgress
	hasPendingProg  bool
	pendingProg     float64
	pendingStartEnd bool // Reverse called before attach while pos == 0

	ph    svgPlayhead
	nodes map[string]*SVGNode
}

// attach binds doc and its summarized timeline to c on Paint.
func (c *SVGController) attach(doc *svg.Document, tl svg.Timeline) {
	if c.doc != nil && c.doc != doc {
		c.ph = svgPlayhead{dir: 1}
		c.releasedBound = false
		c.hasPendingProg = false
		c.pendingProg = 0
		c.pendingStartEnd = false
	}
	c.doc = doc
	c.tl = tl
	c.attached = true
	if c.hasPendingProg {
		c.ph.pos = progressToTime(c.pendingProg, tl.Span)
		c.hasPendingProg = false
	}
	if c.pendingStartEnd {
		if tl.Span > 0 && c.ph.pos <= 0 {
			c.ph.pos = tl.Span
		}
		c.pendingStartEnd = false
	}
}

// Play starts or resumes forward playback. If a finite document is already at
// its end, Play restarts from zero.
func (c *SVGController) Play() {
	c.releasedBound = true
	c.pendingStartEnd = false
	c.ph.init = true
	c.ph.dir = 1
	c.ph.anchor = time.Time{}
	c.ph.pass = 0
	if !c.ph.playing && c.tl.Span > 0 && !c.tl.Infinite && c.ph.pos >= c.tl.Span {
		c.ph.pos = 0
	}
	c.ph.playing = true
	c.props.Changed()
}

// Reverse starts or resumes backward playback toward zero. If currently at
// zero, Reverse starts from the document's span.
func (c *SVGController) Reverse() {
	c.releasedBound = true
	c.ph.init = true
	c.ph.dir = -1
	c.ph.anchor = time.Time{}
	c.ph.pass = 0
	if c.ph.pos <= 0 {
		if c.tl.Span > 0 {
			c.ph.pos = c.tl.Span
			c.pendingStartEnd = false
		} else {
			c.pendingStartEnd = true
		}
	} else {
		c.pendingStartEnd = false
	}
	c.ph.playing = true
	c.props.Changed()
}

// Pause freezes playback at the current timestamp.
func (c *SVGController) Pause() {
	c.releasedBound = true
	c.pendingStartEnd = false
	c.ph.init = true
	c.ph.playing = false
	c.ph.anchor = time.Time{}
	c.props.Changed()
}

// Stop pauses playback and rewinds the playhead to zero without firing OnDone.
func (c *SVGController) Stop() {
	c.releasedBound = true
	c.hasPendingProg = false
	c.pendingProg = 0
	c.pendingStartEnd = false
	c.ph.init = true
	c.ph.playing = false
	c.ph.pos = 0
	c.ph.pass = 0
	c.ph.anchor = time.Time{}
	c.props.Changed()
}

// Seek moves the playhead to t while preserving the current play/pause state.
func (c *SVGController) Seek(t time.Duration) {
	if t < 0 {
		t = 0
	}
	c.releasedBound = true
	c.hasPendingProg = false
	c.pendingProg = 0
	c.pendingStartEnd = false
	c.ph.init = true
	c.ph.pos = t
	c.ph.anchor = time.Time{}
	c.props.Changed()
}

// SeekProgress moves the playhead to normalized progress p in [0, 1] while
// preserving the current play/pause state. Before a document is attached, p is
// retained as pending progress and applied on attach.
func (c *SVGController) SeekProgress(p float64) {
	if math.IsNaN(p) {
		p = 0
	}
	p = gem.Clamp(p, 0, 1)
	c.releasedBound = true
	c.pendingStartEnd = false
	c.ph.init = true
	c.ph.anchor = time.Time{}
	if c.attached {
		c.hasPendingProg = false
		c.ph.pos = progressToTime(p, c.tl.Span)
	} else {
		c.hasPendingProg = true
		c.pendingProg = p
	}
	c.props.Changed()
}

// Playing reports whether playback is currently running.
func (c *SVGController) Playing() bool { return c.ph.playing }

// Time returns the current playhead timestamp.
func (c *SVGController) Time() time.Duration { return c.ph.evalTime(c.tl) }

// Progress returns the normalized playhead progress in [0, 1], or 0 when the
// document has no finite span (unless SeekProgress was called before attach).
func (c *SVGController) Progress() float64 {
	if c.hasPendingProg {
		return c.pendingProg
	}
	if c.tl.Span <= 0 {
		return 0
	}
	return gem.Clamp(float64(c.Time())/float64(c.tl.Span), 0, 1)
}

// Duration returns the attached document's Timeline.Span, or 0 before attach.
func (c *SVGController) Duration() time.Duration { return c.tl.Span }

// Node returns the programmatic control handle for the SVG element with the
// given id, creating one on demand. Calling Node with the same id returns the
// same handle.
func (c *SVGController) Node(id string) *SVGNode {
	if c.nodes == nil {
		c.nodes = make(map[string]*SVGNode)
	}
	if n, ok := c.nodes[id]; ok {
		return n
	}
	n := &SVGNode{
		ctrl:  c,
		id:    id,
		pivot: svg.PivotBox(0.5, 0.5),
	}
	c.nodes[id] = n
	return n
}

// ResetNodes clears all programmatic overrides on every node handle.
func (c *SVGController) ResetNodes() {
	if len(c.nodes) == 0 {
		return
	}
	changed := false
	for _, n := range c.nodes {
		if n.resetFields() {
			changed = true
		}
	}
	if changed {
		c.props.Changed()
	}
}

// resolveInto resolves all node properties and reactive bindings against env
// into *dst, bumping *rev only when a resolved NodeOverride actually changes.
// It reports whether *dst contains any active overrides.
func (c *SVGController) resolveInto(dst *svg.Overrides, rev *uint64, env ggui.Env) bool {
	if len(c.nodes) == 0 {
		if len(*dst) > 0 {
			clear(*dst)
			*rev++
		}
		return false
	}

	changed := false
	activeCount := 0
	for id, n := range c.nodes {
		if id == "" {
			continue
		}
		ov := n.resolve(env)
		if ov.IsZero() {
			if *dst != nil {
				if _, existed := (*dst)[id]; existed {
					delete(*dst, id)
					changed = true
				}
			}
			continue
		}
		activeCount++
		if *dst == nil {
			*dst = make(svg.Overrides, len(c.nodes))
		}
		if prev, ok := (*dst)[id]; !ok || !ov.Equal(prev) {
			(*dst)[id] = ov
			changed = true
		}
	}

	// Remove any keys in *dst that are no longer in c.nodes.
	if len(*dst) > activeCount {
		for id := range *dst {
			if n, ok := c.nodes[id]; !ok || n == nil {
				delete(*dst, id)
				changed = true
			}
		}
	}

	if changed {
		*rev++
	}
	return activeCount > 0
}

type svgScaleMode uint8

const (
	svgScaleUnset svgScaleMode = iota
	svgScaleUniform
	svgScaleXY
)

// SVGNode configures programmatic transform, paint, opacity, visibility and
// stroke-trim overrides for a single named element in an SVG document. Obtain
// one with SVGController.Node(id).
//
// Bound properties (Bind*) are read during Paint so reactive updates never
// trigger layout re-measurement.
type SVGNode struct {
	ctrl *SVGController
	id   string

	hasOffset bool
	offset    property.Value[ggui.Point]

	hasRotate bool
	rotate    property.Value[float64]

	scaleMode    svgScaleMode
	scaleUniform property.Value[float64]
	scaleXY      property.Value[ggui.Point]

	hasPivot bool
	pivot    svg.Pivot

	hasOpacity bool
	opacity    property.Value[float64]

	hasVisible bool
	visible    property.Value[bool]

	hasFill bool
	fill    property.Value[color.Color]

	hasStroke bool
	stroke    property.Value[color.Color]

	hasStrokeWidth bool
	strokeWidth    property.Value[float64]

	hasTrimStart bool
	trimStart    property.Value[float64]

	hasTrimEnd bool
	trimEnd    property.Value[float64]
}

// Translate sets a parent-space translation (dx, dy) on the node.
func (n *SVGNode) Translate(dx, dy float64) *SVGNode { return n.Offset(ggui.Pt(dx, dy)) }

// Offset sets a parent-space translation p on the node, dropping any BindOffset reader.
func (n *SVGNode) Offset(p ggui.Point) *SVGNode {
	changed := !n.hasOffset
	n.hasOffset = true
	if n.offset.Set(p) || changed {
		n.ctrl.props.Changed()
	}
	return n
}

// BindOffset follows a non-nil reader for parent-space translation.
func (n *SVGNode) BindOffset(r ggui.Readable[ggui.Point]) *SVGNode {
	changed := n.offset.Bind(r, "SVGNode.BindOffset")
	if !n.hasOffset {
		n.hasOffset = true
		changed = true
	}
	if changed {
		n.ctrl.props.Changed()
	}
	return n
}

// Rotate sets clockwise rotation in radians around Origin.
func (n *SVGNode) Rotate(rad float64) *SVGNode {
	changed := !n.hasRotate
	n.hasRotate = true
	if n.rotate.Set(rad) || changed {
		n.ctrl.props.Changed()
	}
	return n
}

// BindRotate follows a non-nil reader for clockwise rotation in radians.
func (n *SVGNode) BindRotate(r ggui.Readable[float64]) *SVGNode {
	changed := n.rotate.Bind(r, "SVGNode.BindRotate")
	if !n.hasRotate {
		n.hasRotate = true
		changed = true
	}
	if changed {
		n.ctrl.props.Changed()
	}
	return n
}

// Scale sets uniform scale around Origin.
func (n *SVGNode) Scale(s float64) *SVGNode {
	changed := n.scaleMode != svgScaleUniform
	n.scaleMode = svgScaleUniform
	n.scaleXY.Set(ggui.Point{})
	if n.scaleUniform.Set(s) || changed {
		n.ctrl.props.Changed()
	}
	return n
}

// BindScale follows a non-nil reader for uniform scale.
func (n *SVGNode) BindScale(r ggui.Readable[float64]) *SVGNode {
	changed := n.scaleUniform.Bind(r, "SVGNode.BindScale")
	n.scaleXY.Set(ggui.Point{})
	if n.scaleMode != svgScaleUniform {
		n.scaleMode = svgScaleUniform
		changed = true
	}
	if changed {
		n.ctrl.props.Changed()
	}
	return n
}

// ScaleXY sets non-uniform scale (sx, sy) around Origin.
func (n *SVGNode) ScaleXY(sx, sy float64) *SVGNode {
	changed := n.scaleMode != svgScaleXY
	n.scaleMode = svgScaleXY
	n.scaleUniform.Set(0)
	if n.scaleXY.Set(ggui.Pt(sx, sy)) || changed {
		n.ctrl.props.Changed()
	}
	return n
}

// BindScaleXY follows a non-nil reader for non-uniform scale (X=sx, Y=sy).
func (n *SVGNode) BindScaleXY(r ggui.Readable[ggui.Point]) *SVGNode {
	changed := n.scaleXY.Bind(r, "SVGNode.BindScaleXY")
	n.scaleUniform.Set(0)
	if n.scaleMode != svgScaleXY {
		n.scaleMode = svgScaleXY
		changed = true
	}
	if changed {
		n.ctrl.props.Changed()
	}
	return n
}

// Origin sets the transform origin as fractions of the node's local bounding
// box. The default is (0.5, 0.5), the center.
func (n *SVGNode) Origin(fx, fy float64) *SVGNode {
	p := svg.PivotBox(fx, fy)
	if !n.hasPivot || !n.pivot.Equal(p) {
		n.hasPivot = true
		n.pivot = p
		n.ctrl.props.Changed()
	}
	return n
}

// OriginAt sets the transform origin to an explicit point (x, y) in the node's
// local user space.
func (n *SVGNode) OriginAt(x, y float64) *SVGNode {
	p := svg.PivotAt(x, y)
	if !n.hasPivot || !n.pivot.Equal(p) {
		n.hasPivot = true
		n.pivot = p
		n.ctrl.props.Changed()
	}
	return n
}

// Opacity multiplies the node's evaluated opacity by a.
func (n *SVGNode) Opacity(a float64) *SVGNode {
	changed := !n.hasOpacity
	n.hasOpacity = true
	if n.opacity.Set(a) || changed {
		n.ctrl.props.Changed()
	}
	return n
}

// BindOpacity follows a non-nil reader for the node's opacity multiplier.
func (n *SVGNode) BindOpacity(r ggui.Readable[float64]) *SVGNode {
	changed := n.opacity.Bind(r, "SVGNode.BindOpacity")
	if !n.hasOpacity {
		n.hasOpacity = true
		changed = true
	}
	if changed {
		n.ctrl.props.Changed()
	}
	return n
}

// Visible sets whether the node is visible.
func (n *SVGNode) Visible(v bool) *SVGNode {
	changed := !n.hasVisible
	n.hasVisible = true
	if n.visible.Set(v) || changed {
		n.ctrl.props.Changed()
	}
	return n
}

// BindVisible follows a non-nil reader for the node's visibility.
func (n *SVGNode) BindVisible(r ggui.Readable[bool]) *SVGNode {
	changed := n.visible.Bind(r, "SVGNode.BindVisible")
	if !n.hasVisible {
		n.hasVisible = true
		changed = true
	}
	if changed {
		n.ctrl.props.Changed()
	}
	return n
}

// Fill overrides the node's fill color. A nil color disables the fill
// (svg.NoPaint). EnvColor values are resolved during Paint.
func (n *SVGNode) Fill(c color.Color) *SVGNode {
	changed := !n.hasFill
	n.hasFill = true
	if n.fill.Set(c) || changed {
		n.ctrl.props.Changed()
	}
	return n
}

// BindFill follows a non-nil reader for the node's fill color.
func (n *SVGNode) BindFill(r ggui.Readable[color.Color]) *SVGNode {
	changed := n.fill.Bind(r, "SVGNode.BindFill")
	if !n.hasFill {
		n.hasFill = true
		changed = true
	}
	if changed {
		n.ctrl.props.Changed()
	}
	return n
}

// ClearFill removes any fill override so the SVG's authored fill is used.
func (n *SVGNode) ClearFill() *SVGNode {
	if !n.hasFill && !n.fill.Bound() {
		return n
	}
	n.hasFill = false
	n.fill.Set(nil)
	n.ctrl.props.Changed()
	return n
}

// Stroke overrides the node's stroke color. A nil color disables the stroke
// (svg.NoPaint). EnvColor values are resolved during Paint.
func (n *SVGNode) Stroke(c color.Color) *SVGNode {
	changed := !n.hasStroke
	n.hasStroke = true
	if n.stroke.Set(c) || changed {
		n.ctrl.props.Changed()
	}
	return n
}

// BindStroke follows a non-nil reader for the node's stroke color.
func (n *SVGNode) BindStroke(r ggui.Readable[color.Color]) *SVGNode {
	changed := n.stroke.Bind(r, "SVGNode.BindStroke")
	if !n.hasStroke {
		n.hasStroke = true
		changed = true
	}
	if changed {
		n.ctrl.props.Changed()
	}
	return n
}

// ClearStroke removes any stroke override so the SVG's authored stroke is used.
func (n *SVGNode) ClearStroke() *SVGNode {
	if !n.hasStroke && !n.stroke.Bound() {
		return n
	}
	n.hasStroke = false
	n.stroke.Set(nil)
	n.ctrl.props.Changed()
	return n
}

// StrokeWidth overrides the node's stroke-width.
func (n *SVGNode) StrokeWidth(v float64) *SVGNode {
	changed := !n.hasStrokeWidth
	n.hasStrokeWidth = true
	if n.strokeWidth.Set(v) || changed {
		n.ctrl.props.Changed()
	}
	return n
}

// BindStrokeWidth follows a non-nil reader for the node's stroke-width.
func (n *SVGNode) BindStrokeWidth(r ggui.Readable[float64]) *SVGNode {
	changed := n.strokeWidth.Bind(r, "SVGNode.BindStrokeWidth")
	if !n.hasStrokeWidth {
		n.hasStrokeWidth = true
		changed = true
	}
	if changed {
		n.ctrl.props.Changed()
	}
	return n
}

// Trim sets the normalized arc-length stroke trim window [start, end].
func (n *SVGNode) Trim(start, end float64) *SVGNode {
	changed := !n.hasTrimStart || !n.hasTrimEnd
	n.hasTrimStart, n.hasTrimEnd = true, true
	if n.trimStart.Set(start) {
		changed = true
	}
	if n.trimEnd.Set(end) {
		changed = true
	}
	if changed {
		n.ctrl.props.Changed()
	}
	return n
}

// TrimStart sets the start of the normalized stroke trim window (end defaults to 1).
func (n *SVGNode) TrimStart(v float64) *SVGNode {
	changed := !n.hasTrimStart
	n.hasTrimStart = true
	if n.trimStart.Set(v) || changed {
		n.ctrl.props.Changed()
	}
	return n
}

// BindTrimStart follows a non-nil reader for the start of the stroke trim window.
func (n *SVGNode) BindTrimStart(r ggui.Readable[float64]) *SVGNode {
	changed := n.trimStart.Bind(r, "SVGNode.BindTrimStart")
	if !n.hasTrimStart {
		n.hasTrimStart = true
		changed = true
	}
	if changed {
		n.ctrl.props.Changed()
	}
	return n
}

// TrimEnd sets the end of the normalized stroke trim window (start defaults to 0).
func (n *SVGNode) TrimEnd(v float64) *SVGNode {
	changed := !n.hasTrimEnd
	n.hasTrimEnd = true
	if n.trimEnd.Set(v) || changed {
		n.ctrl.props.Changed()
	}
	return n
}

// BindTrimEnd follows a non-nil reader for the end of the stroke trim window.
func (n *SVGNode) BindTrimEnd(r ggui.Readable[float64]) *SVGNode {
	changed := n.trimEnd.Bind(r, "SVGNode.BindTrimEnd")
	if !n.hasTrimEnd {
		n.hasTrimEnd = true
		changed = true
	}
	if changed {
		n.ctrl.props.Changed()
	}
	return n
}

// Reset clears all overrides and bindings on this node, restoring defaults.
func (n *SVGNode) Reset() *SVGNode {
	if n.resetFields() {
		n.ctrl.props.Changed()
	}
	return n
}

func (n *SVGNode) resetFields() bool {
	had := n.hasOffset || n.hasRotate || n.scaleMode != svgScaleUnset || n.hasPivot ||
		n.hasOpacity || n.hasVisible || n.hasFill || n.hasStroke ||
		n.hasStrokeWidth || n.hasTrimStart || n.hasTrimEnd

	n.hasOffset = false
	n.offset.Set(ggui.Point{})
	n.hasRotate = false
	n.rotate.Set(0)
	n.scaleMode = svgScaleUnset
	n.scaleUniform.Set(0)
	n.scaleXY.Set(ggui.Point{})
	n.hasPivot = false
	n.pivot = svg.PivotBox(0.5, 0.5)
	n.hasOpacity = false
	n.opacity.Set(0)
	n.hasVisible = false
	n.visible.Set(false)
	n.hasFill = false
	n.fill.Set(nil)
	n.hasStroke = false
	n.stroke.Set(nil)
	n.hasStrokeWidth = false
	n.strokeWidth.Set(0)
	n.hasTrimStart = false
	n.trimStart.Set(0)
	n.hasTrimEnd = false
	n.trimEnd.Set(0)
	return had
}

// resolve evaluates all configured literals and bound readers against env and
// returns the corresponding svg.NodeOverride.
func (n *SVGNode) resolve(env ggui.Env) svg.NodeOverride {
	var ov svg.NodeOverride
	if n.hasOffset {
		p := n.offset.Get()
		ov = ov.Translate(p.X, p.Y)
	}
	if n.hasRotate {
		ov = ov.Rotate(n.rotate.Get())
	}
	switch n.scaleMode {
	case svgScaleUniform:
		s := n.scaleUniform.Get()
		ov = ov.Scale(s, s)
	case svgScaleXY:
		p := n.scaleXY.Get()
		ov = ov.Scale(p.X, p.Y)
	}
	if n.hasRotate || n.scaleMode != svgScaleUnset || n.hasPivot {
		ov = ov.Pivot(n.pivot)
	}
	if n.hasOpacity {
		ov = ov.Opacity(n.opacity.Get())
	}
	if n.hasVisible {
		ov = ov.Visible(n.visible.Get())
	}
	if n.hasFill {
		ov = ov.Fill(toSVGPaint(n.fill.Get(), env))
	}
	if n.hasStroke {
		ov = ov.Stroke(toSVGPaint(n.stroke.Get(), env))
	}
	if n.hasStrokeWidth {
		ov = ov.StrokeWidth(n.strokeWidth.Get())
	}
	if n.hasTrimStart || n.hasTrimEnd {
		start := 0.0
		if n.hasTrimStart {
			start = n.trimStart.Get()
		}
		end := 1.0
		if n.hasTrimEnd {
			end = n.trimEnd.Get()
		}
		ov = ov.Trim(start, end, 0)
	}
	return ov
}

// resolveSVGColor resolves c against env, treating both untyped nil and
// typed-nil color.Color implementations as nil without panicking.
func resolveSVGColor(c color.Color, env ggui.Env) color.Color {
	if property.Equal(c, color.Color(nil)) {
		return nil
	}
	c = ggui.ResolveColor(c, env)
	if property.Equal(c, color.Color(nil)) {
		return nil
	}
	return c
}

func toSVGPaint(c color.Color, env ggui.Env) svg.Paint {
	c = resolveSVGColor(c, env)
	if c == nil {
		return svg.NoPaint()
	}
	return svg.ColorPaint(c)
}
