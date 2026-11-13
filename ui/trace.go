package ui

import (
	"maps"
	"slices"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// TraceStatus describes an execution step. The caller owns its lifecycle.
// Pending rows fade, running captions pulse unless motion is reduced, and
// failed/error captions use the destructive color in every row variant.
type TraceStatus string

const (
	TracePending   TraceStatus = "pending"
	TraceRunning   TraceStatus = "running"
	TraceCompleted TraceStatus = "completed"
	TraceFailed    TraceStatus = "failed"
	TraceError     TraceStatus = "error"
	TraceSkipped   TraceStatus = "skipped"
)

// TraceStep describes one execution step. IDs must be nonempty and unique
// throughout a trace, including descendants. They preserve disclosure state and
// focus when steps are updated or reordered. Duration <= 0 hides the duration.
// Details accepts arbitrary widgets, including a CodeBlock or selectable text.
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
// focusable disclosures. It does not start work or measure execution time.
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
			row = &traceRow{id: step.ID, text: ggui.Text("")}
			row.body.row = row
			open := ggui.Bind(func() bool { return t.expanded.Get()[row.id] }, func(v bool) {
				if ggui.Untrack(t.expanded.Get)[row.id] == v {
					return
				}
				t.expanded.Set(withKey(ggui.Untrack(t.expanded.Get), row.id, v))
				if t.onToggle != nil {
					t.onToggle(row.id, v)
				}
			})
			row.disclosure = Collapsible(open, step.Title, &row.body)
			row.disclosure.Key(row)
			// A wrapped title keeps status and duration readable in narrow views.
			row.disclosure.title = ggui.Text("")
			row.action = ButtonOf(row.text, func() {
				if row.step.OnActivate != nil {
					row.step.OnActivate()
				}
			}).Ghost()
			row.action.Key(struct{ row *traceRow }{row})
			t.rows[step.ID] = row
		}
		row.step = step
		row.body.children.rows = t.build(step.Children, seen)
		row.disclosure.SetName(step.Title)
		row.disclosure.title.Content(row.label())
		row.text.Content(row.label())
		row.action.Name(step.Title)
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

type traceList struct {
	name  string
	rows  []*traceRow
	sizes []ggui.Size
	gap   float64
}

func (l *traceList) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	l.gap = uitheme.Of(env).ControlGap
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

type traceRow struct {
	id          string
	step        TraceStep
	disclosure  *CollapsibleWidget
	text        *ggui.TextWidget
	action      *ButtonWidget
	body        traceBody
	theme       *uitheme.Theme
	headerTheme uitheme.Theme
	reduced     bool
	textSize    ggui.Size
}

func (r *traceRow) expandable() bool { return r.step.Details != nil || len(r.step.Children) > 0 }

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

func (r *traceRow) label() string {
	s := r.step.Title
	if meta := r.metadata(); meta != "" {
		s += " · " + meta
	}
	if r.step.Description != "" {
		s += "\n" + r.step.Description
	}
	return s
}

func (r *traceRow) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	r.theme = uitheme.Of(env)
	r.reduced = env.ReducedMotion()
	r.headerTheme = *r.theme
	r.text.Style(r.theme.Text).Color(r.theme.Fg)
	if r.expandable() {
		size := r.disclosure.Layout(cs, env)
		// Only the disclosure header takes status colors. Details and child
		// rows have already inherited the original theme unchanged.
		r.disclosure.theme = &r.headerTheme
		return size
	}
	if r.step.OnActivate != nil {
		return r.action.Layout(cs, env)
	}
	pad := r.theme.FieldPad
	r.textSize = r.text.Layout(ggui.Loose(ggui.Sz(max(0, cs.MaxW-pad.Left-pad.Right), cs.MaxH)), env)
	return cs.Constrain(ggui.Sz(r.textSize.W+pad.Left+pad.Right, r.textSize.H+pad.Top+pad.Bottom))
}

func (r *traceRow) Paint(dst *ggui.Canvas, rect ggui.Rect) {
	header := rect
	if r.expandable() {
		header.Size.H = r.disclosure.headerH
	}
	alpha := 1.0
	visible := dst.Visible(header)
	if r.step.Status == TracePending {
		alpha = .5
	} else if r.step.Status == TraceRunning && !r.reduced && dst.TakesInput() && visible.Size.W > 0 && visible.Size.H > 0 {
		alpha = pulse(.6)
	}
	foreground, icon := r.theme.Fg, r.theme.MutedFg
	if r.step.Status == TraceFailed || r.step.Status == TraceError {
		foreground, icon = r.theme.Destructive, r.theme.Destructive
	}
	foreground, icon = fade(foreground, alpha), fade(icon, alpha)
	// Text.Color is paint-only for concrete colors. These overrides follow
	// layout, so neither Collapsible nor Button can restore the base foreground.
	r.text.Color(foreground)
	r.disclosure.title.Color(foreground)
	r.headerTheme.Fg, r.headerTheme.MutedFg = foreground, icon
	dst.Node(rect, ggui.Node{Role: ggui.RoleListItem, Name: r.step.Title, Description: r.description()}, func(dst *ggui.Canvas) {
		if r.step.Status != "" {
			dst.Leaf(header, ggui.Node{Role: ggui.RoleStatus, Name: r.step.Title + ": " + string(r.step.Status)})
		}
		if r.expandable() {
			dst.Paint(r.disclosure, rect)
			return
		}
		if r.step.OnActivate != nil {
			dst.Paint(r.action, rect)
			return
		}
		dst.Paint(r.text, ggui.Rct(rect.Origin.Add(ggui.Pt(r.theme.FieldPad.Left, r.theme.FieldPad.Top)), r.textSize))
	})
}

type traceBody struct {
	row                      *traceRow
	children                 traceList
	detailSize, childrenSize ggui.Size
	indent, gap              float64
}

func (b *traceBody) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	b.indent, b.gap = t.ControlSize, t.ControlGap
	inner := ggui.Constraints{MinW: max(0, cs.MinW-b.indent), MaxW: max(0, cs.MaxW-b.indent), MaxH: ggui.Unbounded}
	b.detailSize, b.childrenSize = ggui.Size{}, ggui.Size{}
	if b.row.step.Details != nil {
		b.detailSize = b.row.step.Details.Layout(inner, env)
	}
	if len(b.children.rows) > 0 {
		b.childrenSize = b.children.Layout(inner, env)
	}
	gap := 0.0
	if b.detailSize.H > 0 && b.childrenSize.H > 0 {
		gap = b.gap
	}
	return cs.Constrain(ggui.Sz(b.indent+max(b.detailSize.W, b.childrenSize.W), b.detailSize.H+b.childrenSize.H+gap+b.gap))
}

func (b *traceBody) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.FillRect(ggui.Rct(r.Origin.Add(ggui.Pt(b.indent/2, 0)), ggui.Sz(1, r.Size.H)), b.row.theme.Border)
	origin := r.Origin.Add(ggui.Pt(b.indent, 0))
	width := max(0, r.Size.W-b.indent)
	if b.row.step.Details != nil {
		dst.Paint(b.row.step.Details, ggui.Rct(origin, ggui.Sz(width, b.detailSize.H)))
		origin.Y += b.detailSize.H + b.gap
	}
	if len(b.children.rows) > 0 {
		dst.Paint(&b.children, ggui.Rct(origin, ggui.Sz(width, b.childrenSize.H)))
	}
}
