package ggui

import (
	"fmt"
	"strconv"

	"github.com/ironpark/ggui/inspect"
)

// The widget inspector, as the root package knows it. The panel itself is
// ggui/inspect/panel, an Overlay that registers here when imported and ships
// only in builds tagged ggui_inspector. What is here compiles in every
// build and is small: the public surface Config.Inspector, App.Inspector
// and App.OnInspect rest on; the frame built for the panel from the trace
// Canvas.Paint keeps, with an inspect.Source that answers from the widgets;
// and what the built-in widgets say about themselves for the Computed
// pane, through inspect.Fielder.
//
// Nothing here costs a release build anything but bytes: Canvas.Paint
// keeps the trace only while inspectorEnabled is set, which a registered
// panel or an OnInspect handler does.

// inspectorEnabled is set once anything wants frames: a registered panel or
// an OnInspect handler. Canvas.Paint tests it before walking to the frame
// state, so an app with neither pays one branch per painted widget.
var inspectorEnabled bool

// InspectorPanel is what an inspector implementation provides: an Overlay
// that shows the frames it is handed. ggui/inspect/panel is one; importing it
// registers it, and App.Inspector then turns it on and off.
type InspectorPanel interface {
	Overlay
	// Frame hands over the frame just painted, for the next Paint.
	Frame(fr *inspect.Frame)
	// Apply changes the docking and outline settings.
	Apply(InspectorOptions)
	// Reset ends a session when the panel is turned off, forgetting its
	// selection and everything borrowed from the last frame.
	Reset()
	// Release frees what the panel holds outside Go's memory, such as
	// images, when the app closes.
	Release()
	// Closed reports that the panel asked to be turned off.
	Closed() bool
}

// newInspectorPanel makes the registered panel, or is nil when none is.
var newInspectorPanel func() InspectorPanel

// RegisterInspector installs the panel App.Inspector turns on. The
// ggui/inspect/panel package calls it when imported; an inspector of one's own
// calls it instead. nil unregisters.
func RegisterInspector(fn func() InspectorPanel) {
	newInspectorPanel = fn
	inspectorEnabled = inspectorEnabled || fn != nil
}

// inspectorPanel is the app's panel, made on first need from the
// registered constructor; nil when none is registered.
func (a *App) inspectorPanel() InspectorPanel {
	if a.panel == nil && newInspectorPanel != nil {
		a.panel = newInspectorPanel()
	}
	return a.panel
}

// OnInspect registers fn to receive every painted frame's inspect.Frame:
// the widgets as painted and the accessibility tree beside them, as the
// inspector's own panel sees them. It is for a viewer of one's own, such
// as one in another process fed over a connection. The frame is valid
// until fn returns and no longer, since the next frame reuses its buffers;
// a viewer that keeps it copies it, and one that leaves the process calls
// DescribeAll first. Unlike App.Inspector it needs no panel: the frames
// are built whether or not ggui/inspect/panel is imported.
func (a *App) OnInspect(fn func(*inspect.Frame)) {
	a.sinks = append(a.sinks, fn)
	inspectorEnabled = true
}

// publishInspect ends a traced frame: the panel is handed the frame when it
// is on, and every OnInspect handler receives it.
func (a *App) publishInspect(c *Canvas, sem *SemTree) {
	fr := inspectFrame(c, sem)
	if a.inspect {
		a.panel.Frame(fr)
	}
	for _, fn := range a.sinks {
		fn(fr)
	}
}

// InspectorDock is the edge the inspector's panel is docked to.
type InspectorDock uint8

const (
	InspectorBottom InspectorDock = iota // tree beside details on wide panels
	InspectorRight                       // tree above details
)

// InspectorOptions configures the inspector. The toolbar changes the same
// settings while it is open; dragging the panel edge adjusts its size.
// The zero value docks bottom and highlights only the selected widget.
type InspectorOptions struct {
	Dock         InspectorDock
	ShowOutlines bool
}

// inspectSource implements inspect.Source over one frame's widgets.
type inspectSource struct{ f *frameState }

func (s inspectSource) widget(i int) Widget {
	if i < 0 || i >= len(s.f.traceWidgets) {
		return nil
	}
	return s.f.traceWidgets[i]
}

// Describe implements inspect.Source.
func (s inspectSource) Describe(i int) (label, role string) {
	w := s.widget(i)
	if w == nil {
		return "", ""
	}
	return inspectLabel(w), string(nodeOf(w).Role)
}

// Fields implements inspect.Source: the widget's own fields, then how it
// is being interacted with.
func (s inspectSource) Fields(i int) []inspect.Field {
	w := s.widget(i)
	var out []inspect.Field
	if f, ok := w.(inspect.Fielder); ok {
		out = f.InspectFields()
	}
	if w, ok := w.(interface{ state() *Interactive }); ok {
		st := w.state()
		out = append(out, inspect.Field{Key: "Interaction"}, inspect.Field{Key: "hovered", Value: fmtBool(st.Hovered)}, inspect.Field{Key: "pressed", Value: fmtBool(st.Pressed)}, inspect.Field{Key: "focused", Value: fmtBool(st.Focused)}, inspect.Field{Key: "disabled", Value: fmtBool(st.IsInert())})
	}
	return out
}

func fmtBool(b bool) string { return pick(b, "true", "false") }

// Semantic implements inspect.Source: the node the widget described, found
// by the widget itself, else by matching bounds and name, else, for a
// widget that described nothing, whatever is under its middle.
func (s inspectSource) Semantic(i int) int {
	if i < 0 || i >= len(s.f.trace) {
		return -1
	}
	e, w := &s.f.trace[i], s.widget(i)
	sem := s.f.sem
	if w == nil {
		for j := len(sem) - 1; j >= 0; j-- {
			mid := Pt(e.Rect.Origin.X+e.Rect.Size.W/2, e.Rect.Origin.Y+e.Rect.Size.H/2)
			if !sem[j].node.Offscreen && sem[j].rect.Contains(mid) {
				return j
			}
		}
		return -1
	}
	for j := range sem {
		if sameAny(sem[j].handler, w) {
			return j
		}
	}
	n := nodeOf(w)
	for j := range sem {
		if sem[j].full == e.Rect && sem[j].node.Role == n.Role && sem[j].node.Name == n.Name {
			return j
		}
	}
	if _, ok := w.(*TextWidget); ok {
		label := inspectLabel(w)
		for j := range sem {
			if sem[j].full == e.Rect && sem[j].node.Name == label {
				return j
			}
		}
	}
	return -1
}

// Box implements inspect.Source.
func (s inspectSource) Box(i int) (inspect.Box, bool) {
	if b, ok := s.widget(i).(*BoxWidget); ok {
		return b.inspectBox(s.f.trace[i].Rect.Size), true
	}
	return inspect.Box{}, false
}

// inspectBox is a Box's box model as painted at size: its padding and
// border around the child, or around the space left when it has none.
func (b *BoxWidget) inspectBox(size Size) inspect.Box {
	content := b.childSize
	if b.child == nil {
		content = Sz(max(size.W-b.padding.horizontal(), 0), max(size.H-b.padding.vertical(), 0))
	}
	p := b.padding
	return inspect.Box{Padding: inspect.Insets{Top: p.Top, Right: p.Right, Bottom: p.Bottom, Left: p.Left}, Border: b.borderWidth, Content: content, Valid: true}
}

// inspectLabel is what a widget is called: a Text's contents, else its
// accessible name.
func inspectLabel(w Widget) string {
	if t, ok := w.(*TextWidget); ok {
		return t.value
	}
	return nodeOf(w).Name
}

// inspectFrame is the frame the inspector reads for c, with sem as the
// accessibility tree published for it.
func inspectFrame(c *Canvas, sem *SemTree) *inspect.Frame {
	f := c.fs()
	return &inspect.Frame{Nodes: f.trace, Sem: sem, Source: inspectSource{f}}
}

// InspectFields implements inspect.Fielder.
func (b *BoxWidget) InspectFields() []inspect.Field {
	out := []inspect.Field{{Key: "Box style"}, {Key: "background", Value: inspect.Color(b.fill)}, {Key: "border", Value: inspect.Num(b.borderWidth) + " px " + inspect.Color(b.borderColor)}, {Key: "radius", Value: inspect.Num(b.radius) + " px", Number: true}, {Key: "padding", Value: fmt.Sprintf("%s %s %s %s", inspect.Num(b.padding.Top), inspect.Num(b.padding.Right), inspect.Num(b.padding.Bottom), inspect.Num(b.padding.Left)), Number: true}}
	if b.width > 0 {
		out = append(out, inspect.Field{Key: "fixed width", Value: inspect.Num(b.width) + " px", Number: true})
	}
	if b.height > 0 {
		out = append(out, inspect.Field{Key: "fixed height", Value: inspect.Num(b.height) + " px", Number: true})
	}
	return out
}

// InspectFields implements inspect.Fielder.
func (t *TextWidget) InspectFields() []inspect.Field {
	return []inspect.Field{{Key: "Typography"}, {Key: "font size", Value: inspect.Num(t.resolved.Size) + " px", Number: true}, {Key: "line height", Value: inspect.Num(t.resolved.LineHeight), Number: true}, {Key: "color", Value: inspect.Color(t.resolved.Color)}, {Key: "wrap", Value: strconv.FormatBool(t.wrap)}, {Key: "lines", Value: strconv.Itoa(len(t.lines)), Number: true}}
}

// InspectFields implements inspect.Fielder.
func (r *RowWidget) InspectFields() []inspect.Field { return r.flow.inspectFields() }

// InspectFields implements inspect.Fielder.
func (c *ColumnWidget) InspectFields() []inspect.Field { return c.flow.inspectFields() }

func (f *flow) inspectFields() []inspect.Field {
	justify := []string{"start", "center", "end", "space-between", "space-around", "space-evenly"}
	align := []string{"start", "center", "end", "stretch"}
	out := []inspect.Field{{Key: "Flex layout"}, {Key: "direction", Value: pick(f.horizontal, "row", "column")}, {Key: "justify", Value: justify[clamp(int(f.justify), 0, len(justify)-1)]}, {Key: "align", Value: align[clamp(int(f.align), 0, len(align)-1)]}}
	if f.space > 0 {
		return append(out, inspect.Field{Key: "gap token", Value: inspect.Num(f.space) + " × env.spacing"})
	}
	return append(out, inspect.Field{Key: "gap", Value: inspect.Num(f.gap) + " px", Number: true})
}

// InspectFields implements inspect.Fielder.
func (g *GridWidget) InspectFields() []inspect.Field {
	return []inspect.Field{{Key: "Grid layout"}, {Key: "columns", Value: strconv.Itoa(g.cols), Number: true}, {Key: "column width", Value: inspect.Num(g.cellW) + " px", Number: true}, {Key: "column gap", Value: inspect.Num(g.gap) + " px", Number: true}, {Key: "row gap", Value: inspect.Num(g.rowGap) + " px", Number: true}}
}

// InspectFields implements inspect.Fielder.
func (w *WrapWidget) InspectFields() []inspect.Field {
	return []inspect.Field{{Key: "Wrapping layout"}, {Key: "gap", Value: inspect.Num(w.gap) + " px", Number: true}, {Key: "run gap", Value: inspect.Num(w.runGap) + " px", Number: true}}
}

// InspectFields implements inspect.Fielder.
func (s *ScrollWidget) InspectFields() []inspect.Field {
	return []inspect.Field{{Key: "Scroll"}, {Key: "axis", Value: pick(s.horizontal, "horizontal", "vertical")}, {Key: "offset", Value: inspect.Num(s.position()) + " px", Number: true}, {Key: "content", Value: inspect.Num(s.childSize.W) + " × " + inspect.Num(s.childSize.H), Number: true}}
}

// InspectFields implements inspect.Fielder.
func (f *FlexWidget) InspectFields() []inspect.Field {
	return []inspect.Field{{Key: "flex", Value: inspect.Num(f.flex), Number: true}}
}

// inspectKind is the layout badge a tree row shows after a widget's name.
func inspectKind(w Widget) string {
	switch w.(type) {
	case *RowWidget, *ColumnWidget:
		return "flex"
	case *WrapWidget:
		return "wrap"
	case *GridWidget:
		return "grid"
	case *ScrollWidget:
		return "scroll"
	}
	return ""
}
