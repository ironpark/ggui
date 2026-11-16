package ggui

import (
	"slices"
	"testing"

	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/inspect"
)

// inspectFrameOf builds the frame for a canvas outside an App, publishing
// its semantics first.
func inspectFrameOf(c *Canvas) *inspect.Frame {
	return inspectFrame(c, buildSemTree(c, nil, nil, new(a11y.Builder)))
}

// inspectorCanvas paints w with the trace on, as a frame with the
// inspector open would.
func inspectorCanvas(w Widget, size Size) *Canvas {
	inspectorEnabled.Store(true)
	c := &Canvas{}
	c.fs().logical = size
	c.fs().tracing = true
	c.Paint(w, Rct(Point{}, w.Layout(Tight(size), rootEnv())))
	return c
}

func TestInspectorTracesPaintedWidgets(t *testing.T) {
	inspectorEnabled.Store(true)
	c := Canvas{}
	c.fs().tracing = true
	w := column(sized(10, 10), padded(sized(10, 10), 5))
	c.Paint(w, Rct(Pt(0, 0), w.Layout(Loose(Sz(100, 100)), Env{})))
	if len(c.fs().trace) != 4 || len(c.fs().traceWidgets) != 4 {
		t.Fatalf("%d traced widgets, want 4", len(c.fs().trace))
	}
	// A widget is named by its type, *pile here.
	if c.fs().trace[0].Name != "pile" || c.fs().trace[0].Depth != 0 || c.fs().trace[3].Depth != 2 {
		t.Fatalf("trace = %+v", c.fs().trace)
	}
	if c.fs().trace[3].Rect != Rct(Pt(5, 15), Sz(10, 10)) || c.fs().trace[3].Path != "/0/1/0" {
		t.Fatalf("innermost = %+v", c.fs().trace[3])
	}
}

// modelled is a block that describes a box model and its border to the
// inspector, as they are when it is asked.
type modelled struct {
	block
	pad    inspect.Insets
	border float64
}

func (m *modelled) InspectBox(size Size) inspect.Box {
	content := Sz(size.W-m.pad.Left-m.pad.Right, size.H-m.pad.Top-m.pad.Bottom)
	return inspect.Box{Padding: m.pad, Border: m.border, Content: content, Valid: true}
}

func (m *modelled) InspectFields() []inspect.Field {
	return []inspect.Field{{Key: "border", Value: inspect.Num(m.border) + " px", Number: true}}
}

// The frame answers what a widget says of itself, its box model and its
// fields, from the widget as it is when asked, not as it was traced.
func TestInspectFrameAnswersFromTheWidgets(t *testing.T) {
	w := &modelled{block: block{size: Sz(108, 50)}, pad: inspect.Insets{Top: 7, Right: 11, Bottom: 13, Left: 17}, border: 2}
	inspectorEnabled.Store(true)
	c := &Canvas{}
	c.fs().tracing = true
	c.Paint(w, Rct(Pt(20, 30), w.Layout(Loose(Sz(400, 400)), rootEnv())))
	fr := inspectFrameOf(c)
	info := fr.BoxOf(0)
	if !info.Valid || info.Padding != (inspect.Insets{Top: 7, Right: 11, Bottom: 13, Left: 17}) || info.Border != 2 || info.Content != Sz(80, 30) {
		t.Fatalf("incorrect box model: %+v", info)
	}
	if !slices.ContainsFunc(fr.Details(inspect.Computed, 0), func(f inspect.Field) bool { return f.Key == "border" && f.Value == "2 px" }) {
		t.Fatal("computed properties missed the widget's border")
	}
	// A widget that changes in place is read live, not from the trace.
	w.pad.Top = 9
	if fr.BoxOf(0).Padding.Top != 9 {
		t.Fatal("box model was frozen at trace time")
	}
	w.border = 3
	if !slices.ContainsFunc(fr.Details(inspect.Computed, 0), func(f inspect.Field) bool { return f.Key == "border" && f.Value == "3 px" }) {
		t.Fatal("computed properties were frozen at trace time")
	}
}

func TestInspectFrameFindsTheSemanticNode(t *testing.T) {
	t.Parallel()
	w := &twice{}
	w.Role = RoleButton
	w.SetName("Save document")
	c := inspectorCanvas(column(labelled("Heading"), w), Sz(800, 600))
	fr := inspectFrameOf(c)
	if label, role := fr.Source.Describe(2); label != "Save document" || role != "button" {
		t.Fatalf("described %q %q", label, role)
	}
	if got := fr.Source.Semantic(2); got < 0 || fr.Sem.Ref(got).Name != "Save document" {
		t.Fatalf("semantic node %d", got)
	}
	c.fs().traceWidgets[2] = nil
	if got := fr.Source.Semantic(2); got < 0 {
		t.Fatal("without the widget the node under its middle should stand in")
	}
}

// fakePanel records what the app asks of the registered inspector.
type fakePanel struct {
	frames, resets, releases int
	opts                     InspectorOptions
	closed                   bool
	painted                  bool
}

func (f *fakePanel) Paint(*Canvas)                    { f.painted = true }
func (f *fakePanel) Input(OverlayInput) bool          { return false }
func (f *fakePanel) Cursor(Point) (CursorShape, bool) { return 0, false }
func (f *fakePanel) Frame(*inspect.Frame)             { f.frames++ }
func (f *fakePanel) Apply(o InspectorOptions)         { f.opts = o }
func (f *fakePanel) Reset()                           { f.resets++ }
func (f *fakePanel) Release()                         { f.releases++ }
func (f *fakePanel) Closed() bool                     { return f.closed }

// withPanel registers a fake panel for the test and restores whatever was
// registered before.
func withPanel(t *testing.T) *fakePanel {
	t.Helper()
	old := newInspectorPanel
	t.Cleanup(func() { newInspectorPanel = old })
	panel := &fakePanel{}
	RegisterInspector(func() InspectorPanel { return panel })
	return panel
}

func TestInspectorDrivesTheRegisteredPanel(t *testing.T) {
	panel := withPanel(t)
	a := &Window{}
	a.SetInspector(InspectorOptions{Dock: InspectorRight})
	if panel.opts.Dock != InspectorRight {
		t.Fatal("options set before the panel opened were lost")
	}
	a.Inspector(true)
	if !a.inspect || a.overlay != a.panel || a.panel != InspectorPanel(panel) {
		t.Fatal("turning the inspector on did not install the registered panel as the overlay")
	}
	c := inspectorCanvas(around(100, 20, labelled("hello")), Sz(800, 600))
	a.publishInspect(c, buildSemTree(c, nil, nil, new(a11y.Builder)))
	if panel.frames != 1 {
		t.Fatal("the open panel was not handed the frame")
	}
	panel.closed = true
	a.dispatchInput(frameInput{pos: Pt(1, 1)})
	if a.inspect || a.overlay != nil || panel.resets != 1 {
		t.Fatal("a panel that closed itself was not turned off")
	}
	a.publishInspect(c, buildSemTree(c, nil, nil, new(a11y.Builder)))
	if panel.frames != 1 {
		t.Fatal("a closed panel was handed a frame")
	}
	a.Close()
	if panel.releases != 1 {
		t.Fatal("closing the app did not release the panel")
	}
}

func TestInspectorWithoutARegisteredPanelStaysOff(t *testing.T) {
	t.Parallel()
	old := newInspectorPanel
	t.Cleanup(func() { newInspectorPanel = old })
	newInspectorPanel = nil
	a := &Window{}
	a.Inspector(true)
	a.SetInspector(InspectorOptions{})
	if a.inspect || a.overlay != nil || a.panel != nil {
		t.Fatal("Inspector(true) did something with no panel registered")
	}
}

// OnInspect receives the frame the panel would read, with the panel closed:
// tracing turns on for it alone, and the frame answers through its Source.
func TestOnInspectReceivesFramesWithThePanelClosed(t *testing.T) {
	withPanel(t)
	a := &Window{}
	var frames int
	var names []string
	a.OnInspect(func(fr *inspect.Frame) {
		frames++
		names = names[:0]
		for i := range fr.Nodes {
			names = append(names, fr.Nodes[i].Name)
		}
		if label, _ := fr.Source.Describe(1); label != "hello" {
			t.Errorf("label = %q", label)
		}
	})
	c := inspectorCanvas(around(100, 20, labelled("hello")), Sz(800, 600))
	a.publishInspect(c, buildSemTree(c, nil, nil, new(a11y.Builder)))
	if frames != 1 || !slices.Equal(names, []string{"block", "label"}) {
		t.Fatalf("frames = %d, names = %v", frames, names)
	}
	if a.overlay != nil {
		t.Fatal("observing frames should not install the panel")
	}
	a.Inspector(true)
	a.Inspector(false)
	if len(a.sinks) != 1 {
		t.Fatal("toggling the panel dropped the OnInspect handler")
	}
}

func TestProbePublishesFramesAndPaintsTheOverlay(t *testing.T) {
	t.Parallel()
	p := NewProbe(around(100, 20, labelled("hello")), Sz(300, 200))
	defer p.Close()
	var names []string
	p.OnInspect(func(fr *inspect.Frame) {
		names = names[:0]
		for i := range fr.Nodes {
			names = append(names, fr.Nodes[i].Name)
		}
	})
	o := &recorder{rect: Rct(Pt(0, 0), Sz(50, 50))}
	p.SetOverlay(o)
	p.Frame()
	if !slices.Equal(names, []string{"block", "label"}) || o.paints != 1 {
		t.Fatalf("names = %v, paints = %d", names, o.paints)
	}
	p.Move(Pt(10, 10))
	if p.Cursor() != CursorShapeCrosshair {
		t.Fatal("the overlay's cursor was not reported")
	}
}

// A View that rebuilt says how often, in its badge, its Computed pane and
// the filter.
func TestInspectorCountsRebuilds(t *testing.T) {
	n := State(0)
	view := View(n, func(v int) *block { return sized(10, 10) })
	p := NewProbe(view, Sz(50, 50))
	defer p.Close()
	p.Frame()
	for i := range 3 {
		n.Set(i + 1)
		p.Frame()
	}
	fr := inspectFrameOf(inspectorCanvas(view, Sz(50, 50)))
	if got := fr.Badge(0); got != "rebuilt 3" {
		t.Fatalf("badge %q, want %q", got, "rebuilt 3")
	}
	if !fr.Matches(0, "rebuilt") {
		t.Error("the filter does not find the rebuilt view")
	}
	if !slices.ContainsFunc(fr.Details(inspect.Computed, 0), func(f inspect.Field) bool { return f.Key == "rebuilt" && f.Value == "3 times" }) {
		t.Errorf("Computed pane %+v, want 3 rebuilds", fr.Details(inspect.Computed, 0))
	}
}
