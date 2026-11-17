package ggui

import (
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ironpark/ggui/inspect"
	"github.com/ironpark/ggui/pkg/gem"
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
var inspectorEnabled atomic.Bool

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
	if fn != nil {
		inspectorEnabled.Store(true)
	}
}

// inspectorPanel is the app's panel, made on first need from the
// registered constructor; nil when none is registered.
func (w *Window) inspectorPanel() InspectorPanel {
	if w.panel == nil && newInspectorPanel != nil {
		w.panel = newInspectorPanel()
	}
	return w.panel
}

// OnInspect registers fn to receive every painted frame's inspect.Frame:
// the widgets as painted and the accessibility tree beside them, as the
// inspector's own panel sees them. It is for a viewer of one's own, such
// as one in another process fed over a connection. The frame is valid
// until fn returns and no longer, since the next frame reuses its buffers;
// a viewer that keeps it copies it, and one that leaves the process calls
// DescribeAll first. Unlike App.Inspector it needs no panel: the frames
// are built whether or not ggui/inspect/panel is imported.
func (w *Window) OnInspect(fn func(*inspect.Frame)) {
	w.sinks = append(w.sinks, fn)
	inspectorEnabled.Store(true)
}

// publishInspect ends a traced frame: the panel is handed the frame when it
// is on, and every OnInspect handler receives it.
func (w *Window) publishInspect(c *Canvas, sem *SemTree) {
	fr := inspectFrame(c, sem)
	if w.inspect {
		w.panel.Frame(fr)
	}
	for _, fn := range w.sinks {
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
	return s.region(w, out)
}

// region appends to out the hit region w registered, the topmost if it
// registered several: what it takes, and the chain of regions it was
// painted inside, the nearest first, by the type of their handlers.
func (s inspectSource) region(w Widget, out []inspect.Field) []inspect.Field {
	if w == nil {
		return out
	}
	hits := s.f.hits
	at := -1
	for i := len(hits) - 1; i >= 0 && at < 0; i-- {
		r := &hits[i]
		if r.pointer != nil && gem.Same(r.pointer, w) || r.key != nil && gem.Same(r.key, w) || r.within && gem.Same(s.withinOf(i), w) {
			at = i
		}
	}
	if at < 0 {
		return out
	}
	out = append(out, inspect.Field{Key: "Input"}, inspect.Field{Key: "takes", Value: regionTakes(&hits[at])})
	var chain []string
	for p := hits[at].parent; p != 0 && int(p) <= len(hits); p = hits[p-1].parent {
		chain = append(chain, s.regionName(int(p-1)))
	}
	if len(chain) == 0 {
		return append(out, inspect.Field{Key: "inside", Value: "nothing"})
	}
	return append(out, inspect.Field{Key: "inside", Value: strings.Join(chain, " › ")})
}

// withinOf is the within handler of the region at i of the frame's.
func (s inspectSource) withinOf(i int) any {
	for _, e := range s.f.withins {
		if int(e.at) == i {
			return e.h
		}
	}
	return nil
}

// regionName names the region at i by the type of its handler.
func (s inspectSource) regionName(i int) string {
	r := &s.f.hits[i]
	for _, h := range [...]any{r.pointer, r.key, s.withinOf(i), r.drag} {
		if h != nil {
			return handlerName(h)
		}
	}
	return "cursor"
}

// regionTakes lists what a region takes.
func regionTakes(r *hitRegion) string {
	var kinds []string
	for _, k := range [...]struct {
		on   bool
		name string
	}{{r.pointer != nil, "pointer"}, {r.key != nil, "keys"}, {r.within, "within"}, {r.drag != nil, "drag"}, {r.shaped, "cursor"}} {
		if k.on {
			kinds = append(kinds, k.name)
		}
	}
	return strings.Join(kinds, ", ")
}

// handlerName is a handler's type, as widgetName writes a widget's.
func handlerName(h any) string {
	if w, ok := h.(Widget); ok {
		return widgetName(w)
	}
	t := reflect.TypeOf(h)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

func fmtBool(b bool) string { return gem.Pick(b, "true", "false") }

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
		if gem.Same(sem[j].handler, w) {
			return j
		}
	}
	n := nodeOf(w)
	for j := range sem {
		if sem[j].full == e.Rect && sem[j].node.Role == n.Role && sem[j].node.Name == n.Name {
			return j
		}
	}
	if _, ok := w.(inspect.Labeler); ok {
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
	if b, ok := s.widget(i).(inspect.Boxer); ok {
		return b.InspectBox(s.f.trace[i].Rect.Size), true
	}
	return inspect.Box{}, false
}

// inspectLabel is what a widget is called: what it says it shows, as a
// Text its contents, else its accessible name.
func inspectLabel(w Widget) string {
	if l, ok := w.(inspect.Labeler); ok {
		return l.InspectLabel()
	}
	return nodeOf(w).Name
}

// inspectFrame is the frame the inspector reads for c, with sem as the
// accessibility tree published for it.
func inspectFrame(c *Canvas, sem *SemTree) *inspect.Frame {
	f := c.fs()
	return &inspect.Frame{Nodes: f.trace, Sem: sem, Source: inspectSource{f}}
}

// inspectKind is the layout badge a tree row shows after a widget's name.
func inspectKind(w Widget) string {
	if k, ok := w.(inspect.Kinder); ok {
		return k.InspectKind()
	}
	return ""
}

// InspectKind implements inspect.Kinder: how often a Reactive or View
// rebuilt, once it has.
func (c *ComponentWidget) InspectKind() string {
	if c.builds > 1 {
		return "rebuilt " + strconv.Itoa(c.builds-1)
	}
	return ""
}

// InspectFields implements inspect.Fielder: how often a Reactive or View
// rebuilt, the way to spot one that rebuilds far more than what it shows
// changes.
func (c *ComponentWidget) InspectFields() []inspect.Field {
	if c.builds == 0 {
		return nil
	}
	out := []inspect.Field{{Key: "Rebuilds"}, {Key: "rebuilt", Value: strconv.Itoa(c.builds-1) + " times", Number: true}}
	if c.origin != "" {
		out = append(out, inspect.Field{Key: "made at", Value: c.origin})
	}
	return out
}

// traceState is every widget painted this frame, in paint order, while
// the inspector or a sink reads it.
type traceState struct {
	trace        []inspect.Node // every Paint call
	traceWidgets []Widget       // the widget behind each trace node
	tracing      bool
	depth        int
	traceParent  int // index plus one of the widget currently painting
	traceRoots   int
}

// reset begins a paint's trace, taken when tracing.
func (t *traceState) reset(tracing bool) {
	clear(t.trace)
	clear(t.traceWidgets)
	t.trace, t.traceWidgets = t.trace[:0], t.traceWidgets[:0]
	t.tracing, t.traceParent, t.traceRoots = tracing, 0, 0
}

// traceEnter records w, painting into r through c, as the next node of the
// frame's trace, and returns what traceLeave takes once w has painted.
func (f *frameState) traceEnter(c *Canvas, w Widget, r Rect) (parent int) {
	parent = f.traceParent
	path := "/" + strconv.Itoa(f.traceRoots)
	if parent > 0 {
		p := &f.trace[parent-1]
		path = p.Path + "/" + strconv.Itoa(p.Children)
		p.Children++
	} else {
		f.traceRoots++
	}
	f.trace = append(f.trace, inspect.Node{
		Rect: r, Depth: f.depth, Name: widgetName(w), Kind: inspectKind(w),
		ID: inspectComparable(idOf(w)), Instance: inspectComparable(w),
		Path: path, Clip: c.clip, Clipped: c.clipped,
	})
	f.traceWidgets = append(f.traceWidgets, w)
	f.traceParent = len(f.trace)
	f.depth++
	return parent
}

// traceLeave ends the node traceEnter began.
func (f *traceState) traceLeave(parent int) { f.depth--; f.traceParent = parent }

var widgetNames sync.Map // reflect.Type -> string; names never change

// inspectComparable keeps a widget id only when it can be compared, so an
// inspect.Node stays usable as a map key however a widget was identified.
func inspectComparable(v any) any {
	if v != nil && reflect.ValueOf(v).Comparable() {
		return v
	}
	return nil
}

// widgetName is a widget's type for the inspector: Box for *widget.BoxWidget.
func widgetName(w Widget) string {
	t := reflect.TypeOf(w)
	if name, ok := widgetNames.Load(t); ok {
		return name.(string)
	}
	original := t
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	name := t.Name()
	if i := strings.Index(name, "["); i >= 0 {
		name = name[:i]
	}
	name = strings.TrimSuffix(name, "Widget")
	widgetNames.Store(original, name)
	return name
}
