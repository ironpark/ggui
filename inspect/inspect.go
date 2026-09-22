// Package inspect is the model the widget inspector reads: one frame's
// painted widgets, what each one says about itself, and the accessibility
// tree beside them. It holds no widgets and draws nothing, so a viewer can
// live in the app's own window, as ggui's does, or in another process fed
// over a connection.
//
// A Frame is built by the app at the end of a painted frame and is valid
// until the next one begins: its slices are the app's own buffers. The
// cheap parts, every widget's name, geometry, place in the tree and box
// model, are filled as the frame paints. The parts that cost a call into
// the widget, its label and role, its configuration and the accessibility
// node it produced, are asked for through the Source when a viewer needs
// them, and remembered for the rest of the frame.
package inspect

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/geom"
)

type (
	Point = geom.Point
	Size  = geom.Size
	Rect  = geom.Rect
)

// Node is one widget as painted: its Rect and where it sits in the tree.
type Node struct {
	Name     string // the widget's type: Box for *ggui.BoxWidget
	Kind     string // a layout badge, flex or grid or scroll; empty for most
	Rect     Rect
	Clip     Rect // the clip in force when it painted
	Clipped  bool
	Depth    int
	Path     string // structural place: /root/child/grandchild by index
	ID       any    // the widget's explicit identity, or nil
	Instance any    // the widget itself when comparable, an identity of last resort
	Children int    // direct children painted under it

	// What the widget says about itself, filled by Frame.Describe.
	Label, Role string
	described   bool
}

// Box is a widget's box model: padding inside a border around content.
// Valid is false for a widget with none, where Content is its whole size.
type Box struct {
	Padding Insets
	Border  float64
	Content Size
	Valid   bool
}

// Insets are the four sides of padding.
type Insets struct{ Top, Right, Bottom, Left float64 }

// Field is one line of a details pane: a key and its value, or a section
// heading when Value is empty. Number marks a measurement.
type Field struct {
	Key, Value string
	Number     bool
}

// Fielder is a widget that describes its configuration to the inspector:
// the resolved colors, sizes and layout settings the Computed pane shows.
// Format measurements with Num and colors with Color.
type Fielder interface {
	InspectFields() []Field
}

// Tab is a details pane.
type Tab uint8

const (
	// Layout is geometry and place in the tree, plus the box model.
	Layout Tab = iota
	// Computed is what the widget's own InspectFields reports.
	Computed
	// Semantics is the accessibility node the widget produced.
	Semantics
)

// Source answers what a Frame does not hold: the parts that need the
// widget itself. The app implements it over its widgets; a remote viewer
// implements it over a connection. Every index is into Frame.Nodes.
type Source interface {
	// Describe is the widget's label and role.
	Describe(i int) (label, role string)
	// Fields is the widget's own configuration, for the Computed pane.
	Fields(i int) []Field
	// Semantic is the index in Frame.Sem of the node the widget produced,
	// or -1 for none.
	Semantic(i int) int
	// Box is the widget's box model, and false when it has none.
	Box(i int) (Box, bool)
}

// Frame is one painted frame: every widget in paint order, parents before
// children, and the accessibility tree it published.
type Frame struct {
	Nodes  []Node
	Sem    *a11y.SemTree // may be nil when nothing was published
	Source Source        // may be nil, in which case nodes stay as built
}

// Describe returns node i with its Label and Role filled in.
func (f *Frame) Describe(i int) *Node {
	n := &f.Nodes[i]
	if !n.described {
		n.described = true
		if f.Source != nil {
			n.Label, n.Role = f.Source.Describe(i)
		}
	}
	return n
}

// DescribeAll fills every node's Label and Role, for a viewer that will
// leave the process and cannot ask later.
func (f *Frame) DescribeAll() {
	for i := range f.Nodes {
		f.Describe(i)
	}
}

// Badge is what a tree row shows after the name: the layout kind, else
// the role.
func (f *Frame) Badge(i int) string {
	n := f.Describe(i)
	if n.Kind != "" {
		return n.Kind
	}
	return n.Role
}

// Matches reports whether node i's name, label or role contains filter,
// which must already be lower-cased.
func (f *Frame) Matches(i int, filter string) bool {
	n := f.Describe(i)
	return strings.Contains(strings.ToLower(n.Name+" "+n.Label+" "+n.Role), filter)
}

// Deepest is the innermost node under p that is not clipped away there,
// which is what a click would reach; -1 for none.
func Deepest(nodes []Node, p Point) int {
	found := -1
	for i := range nodes {
		n := &nodes[i]
		if n.Rect.Contains(p) && (!n.Clipped || n.Clip.Contains(p)) {
			found = i
		}
	}
	return found
}

// HasChildren reports whether node i painted anything under it.
func HasChildren(nodes []Node, i int) bool {
	return i+1 < len(nodes) && nodes[i+1].Depth > nodes[i].Depth
}

// Ancestors lists node i's parent, grandparent and so on up to a root.
func Ancestors(nodes []Node, i int) []int {
	var out []int
	need := nodes[i].Depth - 1
	for j := i - 1; j >= 0 && need >= 0; j-- {
		if nodes[j].Depth == need {
			out = append(out, j)
			need--
		}
	}
	return out
}

// BoxOf is the box model the Layout pane draws for node i. A control paints
// its decorated Box as a child of the same bounds, so that box stands in
// for it rather than inventing padding. It is read from the widget each
// time, since a widget can change in place without being painted again.
func (f *Frame) BoxOf(i int) Box {
	n := &f.Nodes[i]
	if f.Source != nil {
		if box, ok := f.Source.Box(i); ok {
			return box
		}
		for j := i + 1; j < len(f.Nodes) && f.Nodes[j].Depth > n.Depth; j++ {
			if box, ok := f.Source.Box(j); ok && f.Nodes[j].Rect == n.Rect {
				return box
			}
		}
	}
	return Box{Content: n.Rect.Size}
}

// Details is the fields of tab for node i.
func (f *Frame) Details(tab Tab, i int) []Field {
	if i < 0 || i >= len(f.Nodes) {
		return nil
	}
	switch tab {
	case Computed:
		return f.computed(i)
	case Semantics:
		return f.semantic(i)
	}
	return f.layout(i)
}

func (f *Frame) layout(i int) []Field {
	n := &f.Nodes[i]
	parent := "root"
	if a := Ancestors(f.Nodes, i); len(a) > 0 {
		parent = f.Nodes[a[0]].Name
	}
	kids := 0
	for j := i + 1; j < len(f.Nodes) && f.Nodes[j].Depth > n.Depth; j++ {
		if f.Nodes[j].Depth == n.Depth+1 {
			kids++
		}
	}
	out := []Field{{Key: "Geometry"}, {"x", Num(n.Rect.Origin.X) + " px", true}, {"y", Num(n.Rect.Origin.Y) + " px", true}, {"width", Num(n.Rect.Size.W) + " px", true}, {"height", Num(n.Rect.Size.H) + " px", true}, {Key: "Hierarchy"}, {"parent", parent, false}, {"children", strconv.Itoa(kids), true}, {"depth", strconv.Itoa(n.Depth), true}}
	if n.Clipped {
		visible := n.Rect.Intersect(n.Clip).Size
		out = append(out, Field{"visible size", Num(visible.W) + " × " + Num(visible.H), true})
	}
	if n.ID != nil {
		out = append(out, Field{"identity", fmt.Sprint(n.ID), false})
	}
	return out
}

func (f *Frame) computed(i int) []Field {
	n := f.Describe(i)
	out := []Field{{Key: "Widget"}, {"type", n.Name, false}, {"width", Num(n.Rect.Size.W) + " px", true}, {"height", Num(n.Rect.Size.H) + " px", true}}
	if n.Label != "" {
		out = append(out, Field{"label", n.Label, false})
	}
	if f.Source != nil {
		out = append(out, f.Source.Fields(i)...)
	}
	return out
}

func (f *Frame) semantic(i int) []Field {
	found := -1
	if f.Source != nil {
		found = f.Source.Semantic(i)
	}
	if found < 0 || f.Sem == nil || found >= f.Sem.Len() {
		return []Field{{Key: "Node"}, {"role", "none", false}}
	}
	n := &f.Sem.Ref(found).Node
	out := []Field{{Key: "Node"}, {"role", string(n.Role), false}}
	add := func(k, v string) {
		if v != "" {
			out = append(out, Field{k, v, false})
		}
	}
	add("name", n.Name)
	add("description", n.Description)
	add("value", n.Value)
	switch n.Checked {
	case a11y.TriOff:
		add("checked", "false")
	case a11y.TriOn:
		add("checked", "true")
	case a11y.TriMixed:
		add("checked", "mixed")
	}
	if n.Expanded != nil {
		add("expanded", strconv.FormatBool(*n.Expanded))
	}
	if n.Selected {
		add("selected", "true")
	}
	if n.Disabled {
		add("disabled", "true")
	}
	if n.Offscreen {
		add("offscreen", "true")
	}
	if n.Min != 0 || n.Max != 0 || n.Now != 0 {
		out = append(out, Field{"range", Num(n.Min) + " – " + Num(n.Max), true}, Field{"now", Num(n.Now), true})
	}
	if chain := f.SemanticChain(found); len(chain) > 1 {
		out = append(out, Field{Key: "Path"})
		for _, c := range chain[:len(chain)-1] {
			out = append(out, Field{"", strings.TrimLeft(c, " "), false})
		}
	}
	return out
}

// SemanticAt is the innermost accessibility node painted under p, or -1:
// the last one described there, since parents describe before children.
func (f *Frame) SemanticAt(p Point) int {
	if f.Sem == nil {
		return -1
	}
	for i := f.Sem.Len() - 1; i >= 0; i-- {
		if n := f.Sem.Ref(i); !n.Offscreen && n.Rect.Contains(p) {
			return i
		}
	}
	return -1
}

// SemanticChain describes accessibility node i and everything it is inside
// of, outermost first and indented, which is what a screen reader walks.
func (f *Frame) SemanticChain(i int) []string {
	if f.Sem == nil || i < 0 {
		return nil
	}
	var chain []int
	for ; i >= 0; i = f.Sem.Ref(i).Parent {
		chain = append(chain, i)
	}
	lines := make([]string, 0, len(chain))
	for at, c := range slices.Backward(chain) {
		n := f.Sem.Ref(c)
		lines = append(lines, fmt.Sprintf("%s%s %q%s", strings.Repeat("  ", len(chain)-1-at), n.Role, n.Name, n.Flags()))
	}
	return lines
}

// Num formats a measurement in logical pixels to two decimal places.
func Num(v float64) string { return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64) }

// Color formats c as CSS hex, with alpha only when it is not opaque, and
// "none" for nil.
func Color(c color.Color) string {
	if c == nil {
		return "none"
	}
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	if n.A == 255 {
		return fmt.Sprintf("#%02x%02x%02x", n.R, n.G, n.B)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", n.R, n.G, n.B, n.A)
}
