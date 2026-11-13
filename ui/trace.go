package ui

import (
	"image/color"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// TraceStatus describes an execution step. The caller owns its lifecycle.
// A pending row fades, a highlight moves across a running row's title unless
// motion is reduced, and a failed or error row is drawn in the destructive
// color in every row variant. The status is not printed: it reaches assistive
// technology through the row's description and a status node.
type TraceStatus string

// The statuses of a step.
const (
	TracePending   TraceStatus = "pending"   // not started: the row fades
	TraceRunning   TraceStatus = "running"   // under way: the title shimmers
	TraceCompleted TraceStatus = "completed" // finished
	TraceFailed    TraceStatus = "failed"    // finished badly: drawn in the destructive color
	TraceError     TraceStatus = "error"     // as TraceFailed
	TraceSkipped   TraceStatus = "skipped"   // passed over
)

// TraceStep describes one execution step. IDs must be nonempty and unique
// throughout a trace, including descendants. They preserve disclosure state and
// focus when steps are updated or reordered. A row is one line: the Title, then
// the Description and the Duration in a fainter run beside it. Duration <= 0
// hides the duration. Details accepts arbitrary widgets, including a CodeBlock
// or selectable text.
type TraceStep struct {
	ID          string
	Title       string
	Description string
	Status      TraceStatus
	Duration    time.Duration
	Details     ggui.Widget
	Children    []TraceStep
	// OnActivate turns a leaf without Details or Children into a button. Put
	// actions in Details for expandable rows, whose header remains a disclosure.
	OnActivate func()
}

// TraceWidget presents execution steps as nested lists with independently
// focusable disclosures: bare muted lines that turn to the foreground under
// the pointer, each with its chevron after the title, and the details of an
// open one indented behind a rule. It does not start work or measure
// execution time.
type TraceWidget struct {
	props    property.Owner
	steps    property.Value[[]TraceStep]
	expanded ggui.Binding[map[string]bool]
	name     string
	onToggle func(string, bool)
	rows     map[string]*traceRow
	list     traceList
}

// Trace creates a hierarchy of execution steps. Details and descendants start
// collapsed. Expanded or BindExpanded controls the open step IDs. Tab reaches
// each disclosure and its visible detail controls; Enter and Space toggle it.
// Wrap the trace in ggui.Scroll when the execution history needs a viewport.
func Trace(steps ...TraceStep) *TraceWidget {
	t := &TraceWidget{name: "Execution trace", expanded: ggui.State(map[string]bool{}), rows: make(map[string]*traceRow)}
	t.steps.Set(cloneTraceSteps(steps))
	return t
}

// Steps replaces the hierarchy and detaches any BindSteps reader.
func (t *TraceWidget) Steps(steps []TraceStep) *TraceWidget {
	property.Set(&t.props, &t.steps, cloneTraceSteps(steps))
	return t
}

// BindSteps follows execution updates without rebuilding the trace. Treat each
// published slice and its descendants as immutable, as for other GGUI readers.
func (t *TraceWidget) BindSteps(steps ggui.Readable[[]TraceStep]) *TraceWidget {
	property.Bind(&t.props, &t.steps, steps, "BindSteps")
	return t
}

// Name sets the accessible name of the execution list.
func (t *TraceWidget) Name(name string) *TraceWidget {
	defer property.Watch(&t.props, &t.name)()
	t.name = name
	return t
}

// BindExpanded binds open step IDs. User actions publish a fresh map, leaving
// the caller's previous map untouched. Layout never writes the binding.
func (t *TraceWidget) BindExpanded(open ggui.Binding[map[string]bool]) *TraceWidget {
	property.Require(open, "BindExpanded")
	if !property.Same(t.expanded, open) {
		t.expanded = open
		t.props.Changed()
	}
	return t
}

// Expanded detaches the external binding and opens the supplied IDs.
func (t *TraceWidget) Expanded(open map[string]bool) *TraceWidget {
	return t.BindExpanded(ggui.State(maps.Clone(open)))
}

// OnToggle runs after a user expands or collapses a step.
func (t *TraceWidget) OnToggle(fn func(id string, open bool)) *TraceWidget { t.onToggle = fn; return t }

func cloneTraceSteps(steps []TraceStep) []TraceStep {
	out := slices.Clone(steps)
	for i := range out {
		out[i].Children = cloneTraceSteps(out[i].Children)
	}
	return out
}

func (t *TraceWidget) build(steps []TraceStep, seen map[string]bool) []*traceRow {
	rows := make([]*traceRow, 0, len(steps))
	for _, step := range steps {
		if step.ID == "" || seen[step.ID] {
			panic("ui: Trace requires nonempty, globally unique step IDs")
		}
		seen[step.ID] = true
		row := t.rows[step.ID]
		if row == nil {
			row = &traceRow{id: step.ID, trace: t, title: ggui.Text("").NoWrap().Ellipsis(), run: ggui.Text("").NoWrap().Ellipsis()}
			row.body.row = row
			row.SetKey(row)
			t.rows[step.ID] = row
		}
		row.step = step
		row.body.children.rows = t.build(step.Children, seen)
		row.Role = pick(row.expandable(), ggui.RoleDisclosure, ggui.RoleButton)
		row.SetName(step.Title)
		// Said once a layout: a running step paints every frame.
		row.described, row.statusName = row.description(), step.Title+": "+string(step.Status)
		row.shown = row.caption()
		row.title.Content(step.Title)
		row.run.Content(row.shown)
		rows = append(rows, row)
	}
	return rows
}

// Layout implements Widget.
func (t *TraceWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	defer t.props.Layout().EndLayout()
	seen := make(map[string]bool)
	t.list.rows = t.build(t.steps.Get(), seen)
	for id := range t.rows {
		if !seen[id] {
			delete(t.rows, id)
		}
	}
	t.list.name = t.name
	return t.list.Layout(cs, env)
}

// Paint implements Widget.
func (t *TraceWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { t.list.Paint(dst, r) }

// setOpen opens or closes the step id for a user's action, publishing a
// fresh map and telling OnToggle.
func (t *TraceWidget) setOpen(id string, open bool) {
	now := ggui.Untrack(t.expanded.Get)
	if now[id] == open {
		return
	}
	t.expanded.Set(withKey(now, id, open))
	if t.onToggle != nil {
		t.onToggle(id, open)
	}
}

type traceList struct {
	name  string
	rows  []*traceRow
	sizes []ggui.Size
	gap   float64
}

func (l *traceList) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	l.gap = uitheme.Of(env).Space
	l.sizes = l.sizes[:0]
	var width, height float64
	for i, row := range l.rows {
		if i > 0 {
			height += l.gap
		}
		size := row.Layout(ggui.Constraints{MinW: cs.MinW, MaxW: cs.MaxW, MaxH: ggui.Unbounded}, env)
		l.sizes = append(l.sizes, size)
		width, height = max(width, size.W), height+size.H
	}
	return cs.Constrain(ggui.Sz(width, height))
}

func (l *traceList) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Clip(r).Node(r, ggui.Node{Role: ggui.RoleList, Name: l.name, Max: float64(len(l.rows))}, func(dst *ggui.Canvas) {
		y := r.Origin.Y
		for i, row := range l.rows {
			dst.Paint(row, ggui.Rct(ggui.Pt(r.Origin.X, y), ggui.Sz(r.Size.W, l.sizes[i].H)))
			y += l.sizes[i].H + l.gap
		}
	})
}

// traceRow is one step: a line as wide as its text, which is a disclosure
// when the step has details or children, a button when it has an action and
// plain text otherwise, and below it the body an open disclosure reveals.
type traceRow struct {
	ggui.Interactive
	described, statusName string // the row's description and status, as a screen reader hears them
	shown                 string // the run beside the title: the step's description and duration
	id                    string
	step                  TraceStep
	trace                 *TraceWidget
	title, run            *ggui.TextWidget
	body                  traceBody
	reveal                ggui.Motion
	progress              float64
	laidOpen              bool // open, as the last layout read it
	theme                 *uitheme.Theme
	env                   ggui.Env
	reduced               bool
	gap, chevron          float64 // between the title and the run, and the chevron's box with its gap
	titleSize, runSize    ggui.Size
	line, bodySize        ggui.Size
	header                ggui.Rect   // where the line was last painted
	ink                   color.Color // the title's color, as last painted
	shimmering            bool        // whether the last paint moved a highlight across the title
}

func (r *traceRow) expandable() bool { return r.step.Details != nil || len(r.step.Children) > 0 }

// actionable reports whether the line takes input: to open, or to act.
func (r *traceRow) actionable() bool { return r.expandable() || r.step.OnActivate != nil }

func (r *traceRow) open() bool { return r.trace.expanded.Get()[r.id] }

func (r *traceRow) metadata() string {
	description := string(r.step.Status)
	if r.step.Duration > 0 {
		if description != "" {
			description += " · "
		}
		description += r.step.Duration.String()
	}
	return description
}

func (r *traceRow) description() string {
	s := r.metadata()
	if r.step.Description != "" {
		if s != "" {
			s += " · "
		}
		s += r.step.Description
	}
	return s
}

// caption is what the fainter run beside the title shows: the description,
// then the duration. The status is told by the row's look instead.
func (r *traceRow) caption() string {
	s := r.step.Description
	if r.step.Duration > 0 {
		if s != "" {
			s += " · "
		}
		s += r.step.Duration.String()
	}
	return s
}

func (r *traceRow) activate() {
	if r.expandable() {
		r.trace.setOpen(r.id, !ggui.Untrack(r.open))
	} else if r.step.OnActivate != nil {
		r.step.OnActivate()
	}
}

// Describe implements ggui.Describer: a disclosure reports whether its
// content is showing, and a row with an action is a button.
func (r *traceRow) Describe() ggui.Node {
	if !r.expandable() {
		return ggui.Node{Role: ggui.RoleButton, Name: r.step.Title, Actions: ggui.ActionPress | ggui.ActionFocus}
	}
	open := ggui.Untrack(r.open)
	return ggui.Node{
		Role:     ggui.RoleDisclosure,
		Name:     r.step.Title,
		Expanded: ggui.Expandable(open),
		Actions:  ggui.ActionPress | ggui.ActionFocus | pick(open, ggui.ActionCollapse, ggui.ActionExpand),
	}
}

// Act implements ggui.Actor: expanding and collapsing say which way to go.
func (r *traceRow) Act(a ggui.Action) bool {
	if !r.expandable() {
		return false
	}
	switch a.Kind {
	case ggui.ActionExpand:
		r.trace.setOpen(r.id, true)
	case ggui.ActionCollapse:
		r.trace.setOpen(r.id, false)
	default:
		return false
	}
	return true
}

// HandleKey implements KeyHandler: Space or Enter opens, closes or acts.
func (r *traceRow) HandleKey(ev ggui.KeyEvent) { r.Keyboard(ev, r.activate) }

// HandlePointer implements PointerHandler.
func (r *traceRow) HandlePointer(ev ggui.PointerEvent) bool { return r.Pointer(ev, r.activate) }

func (r *traceRow) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	r.Sync()
	t := uitheme.Of(env)
	r.theme, r.env, r.reduced = t, env, env.ReducedMotion()
	expandable := r.expandable()
	r.gap, r.chevron = t.Space, 0
	if expandable {
		r.chevron = t.IconSize + t.Space/2
	}
	// The run keeps its width and the title gives way, down to a third of
	// the line, so neither a long description nor a long title hides the other.
	avail := max(0, cs.MaxW-r.chevron)
	r.title.Style(t.Text)
	r.run.Style(t.Text)
	r.runSize = ggui.Size{}
	if r.shown != "" {
		r.runSize = r.run.Layout(ggui.Loose(ggui.Sz(max(0, avail*2/3-r.gap), ggui.Unbounded)), env)
		avail = max(0, avail-r.runSize.W-r.gap)
	}
	r.titleSize = r.title.Layout(ggui.Loose(ggui.Sz(avail, ggui.Unbounded)), env)
	r.line = ggui.Sz(r.titleSize.W+r.chevron, max(r.titleSize.H, r.runSize.H))
	if r.shown != "" {
		r.line.W += r.gap + r.runSize.W
	}
	if expandable {
		r.line.H = max(r.line.H, t.IconSize)
	}
	r.line.W = min(r.line.W, cs.MaxW)
	open := expandable && r.open()
	r.laidOpen = open
	r.progress = r.reveal.LayoutToggle(env, open, t.MotionSlow)
	r.bodySize = ggui.Size{}
	if expandable && (r.progress > 0 || open) {
		r.bodySize = r.body.Layout(ggui.Constraints{MinW: cs.MinW, MaxW: cs.MaxW, MaxH: ggui.Unbounded}, env)
	}
	return cs.Constrain(ggui.Sz(max(r.line.W, r.bodySize.W), r.line.H+r.bodySize.H*r.progress))
}

// Shared hover timing for trace lines, independent of their opening motion.
var traceHoverSlot = ggui.NewSlot[*ggui.Motion]("trace hover")

func (r *traceRow) Paint(dst *ggui.Canvas, rect ggui.Rect) {
	t := r.theme
	expandable, actionable := r.expandable(), r.actionable()
	open := expandable && ggui.Untrack(r.open)
	// The body eases by itself; this is for a binding that is no signal,
	// whose change asks for no layout of its own.
	if open != r.laidOpen {
		ggui.Invalidate(r.env)
	}
	header := ggui.Rct(rect.Origin, ggui.Sz(min(r.line.W, rect.Size.W), min(r.line.H, rect.Size.H)))
	r.header = header
	hover := 0.0
	if actionable {
		hover = dst.Ease(r.Anchor(header), traceHoverSlot, pick(r.Hovered, 1.0, 0.0), r.env.Motion(t.MotionFast))
	}
	// Muted until the pointer is over a line that does something; the run
	// beside the title stays fainter either way.
	ink, faint := t.MutedFg, fade(t.MutedFg, .7)
	if r.step.Status == TraceFailed || r.step.Status == TraceError {
		ink = t.Destructive
	} else if hover > 0 {
		ink = mix(t.MutedFg, t.Fg, hover)
	}
	if r.step.Status == TracePending {
		ink, faint = fade(ink, .6), fade(faint, .6)
	}
	r.ink = ink
	// Text.Color is paint-only for concrete colors.
	r.title.Color(ink)
	r.run.Color(faint)
	dst.Node(rect, ggui.Node{Role: ggui.RoleListItem, Name: r.step.Title, Description: r.described}, func(dst *ggui.Canvas) {
		if r.step.Status != "" {
			dst.Leaf(header, ggui.Node{Role: ggui.RoleStatus, Name: r.statusName})
		}
		if actionable {
			r.Hit(dst, header, r, ggui.CursorShapePointer)
		}
		x := header.Origin.X
		titleRect := ggui.Rct(ggui.Pt(x, header.Origin.Y+(r.line.H-r.titleSize.H)/2), r.titleSize)
		dst.Paint(r.title, titleRect)
		// A highlight moves across a running step's title, and only while
		// it is in view: a row scrolled away asks for no frames. A frame
		// painted in part is painted inert, so inertness cannot be asked.
		r.shimmering = false
		if r.step.Status == TraceRunning && !r.reduced {
			if visible := dst.Visible(header); visible.Size.W > 0 && visible.Size.H > 0 {
				r.shimmering = true
				paintShimmer(dst, r.title, titleRect, ink, t.Fg)
			}
		}
		x += r.titleSize.W
		if r.shown != "" {
			x += r.gap
			dst.Paint(r.run, ggui.Rct(ggui.Pt(x, header.Origin.Y+(r.line.H-r.runSize.H)/2), r.runSize))
			x += r.runSize.W
		}
		if expandable {
			// The chevron turns from pointing right to pointing down with
			// the body it reveals.
			center := ggui.Pt(x+r.chevron-t.IconSize/2, header.Origin.Y+r.line.H/2)
			paintIconAt(dst, r.env, icons.ChevronRight, center, t.IconSize, ink, r.progress*math.Pi/2)
		}
		if r.Focused && r.FocusVisible {
			fieldHalo(dst, t, header, t.RadiusSm, t.Ring)
		}
		if expandable {
			paintDisclosure(dst, &r.body, ggui.Pt(rect.Origin.X, rect.Origin.Y+r.line.H), rect.Size.W, r.bodySize, r.progress, open)
		}
	})
}

// traceBody is what an open step shows under its line: the details, then
// the child steps, indented behind a rule that drops from the line's start.
type traceBody struct {
	row                      *traceRow
	children                 traceList
	detailSize, childrenSize ggui.Size
	rail, indent, trail, gap float64
	top, bottom              float64
}

func (b *traceBody) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	// The rule sits under the middle of a line's icon, and the content two
	// spaces past it.
	b.rail = max(0, t.IconSize/2-t.BorderWidth)
	b.indent, b.trail = b.rail+t.BorderWidth+2*t.Space, t.Space/2
	b.gap, b.top, b.bottom = t.Space, t.Space, t.Space/2
	side := b.indent + b.trail
	inner := ggui.Constraints{MinW: max(0, cs.MinW-side), MaxW: max(0, cs.MaxW-side), MaxH: ggui.Unbounded}
	b.detailSize, b.childrenSize = ggui.Size{}, ggui.Size{}
	if b.row.step.Details != nil {
		// Details read as supporting text. The theme they resolve is the
		// trace's own, whatever the status of the step above them.
		b.detailSize = b.row.step.Details.Layout(inner, env.WithText(ggui.TextStyle{Color: t.MutedFg, LineHeight: 1.625}))
	}
	if len(b.children.rows) > 0 {
		b.childrenSize = b.children.Layout(inner, env)
	}
	gap := 0.0
	if b.detailSize.H > 0 && b.childrenSize.H > 0 {
		gap = b.gap
	}
	return cs.Constrain(ggui.Sz(side+max(b.detailSize.W, b.childrenSize.W), b.top+b.detailSize.H+gap+b.childrenSize.H+b.bottom))
}

func (b *traceBody) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := b.row.theme
	dst.FillRect(ggui.Rct(r.Origin.Add(ggui.Pt(b.rail, b.top)), ggui.Sz(t.BorderWidth, max(0, r.Size.H-b.top-b.bottom))), t.Border)
	origin := r.Origin.Add(ggui.Pt(b.indent, b.top))
	width := max(0, r.Size.W-b.indent-b.trail)
	if b.row.step.Details != nil {
		dst.Paint(b.row.step.Details, ggui.Rct(origin, ggui.Sz(width, b.detailSize.H)))
		origin.Y += b.detailSize.H + b.gap
	}
	if len(b.children.rows) > 0 {
		dst.Paint(&b.children, ggui.Rct(origin, ggui.Sz(width, b.childrenSize.H)))
	}
}
