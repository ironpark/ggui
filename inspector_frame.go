//go:build ggui_inspector

package ggui

import "github.com/ironpark/ggui/inspect"

// The frame the inspector reads is built here from the trace Canvas.Paint
// kept and the accessibility tree the frame published. The nodes are the
// trace's own buffer, so nothing is copied; what needs a widget is answered
// through inspectSource when a viewer asks.

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
