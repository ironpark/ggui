package ggui

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"
)

type inspectField struct {
	key, value string
	number     bool
}

// Input runs before the next paint, so these are the most recently painted
// trace and semantics. Serialize only when requested, resolving selection
// again so a click followed by Copy does not copy the previous element.
func (in *inspector) copySelection() {
	dst := in.copySource
	if dst == nil {
		return
	}
	sel := in.find(dst.frameTrace())
	if sel < 0 {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", dst.frameTrace()[sel].name)
	for _, tab := range []inspectTab{inspectTabLayout, inspectTabComputed, inspectTabSemantics} {
		for _, f := range inspectDetails(tab, dst, sel) {
			if f.value == "" {
				fmt.Fprintf(&b, "\n%s\n", f.key)
			} else {
				fmt.Fprintf(&b, "%s: %s\n", f.key, f.value)
			}
		}
	}
	currentClipboard().Write(b.String())
	in.copied = true
}

type inspectBox struct {
	padding EdgeInsets
	border  float64
	content Size
	valid   bool
}

func inspectLabel(e *traceEntry) string {
	if t, ok := e.widget.(*TextWidget); ok {
		return t.value
	}
	return nodeOf(e.widget).Name
}
func inspectKind(e *traceEntry) string {
	switch e.widget.(type) {
	case *RowWidget, *ColumnWidget:
		return "flex"
	case *WrapWidget:
		return "wrap"
	case *GridWidget:
		return "grid"
	case *ScrollWidget:
		return "scroll"
	}
	return string(nodeOf(e.widget).Role)
}
func inspectSemantic(dst *Canvas, e *traceEntry) int {
	if e.widget == nil {
		return semanticAt(dst, Pt(e.rect.Origin.X+e.rect.Size.W/2, e.rect.Origin.Y+e.rect.Size.H/2))
	}
	for i := range dst.frameSem() {
		if sameAny(dst.frameSem()[i].handler, e.widget) {
			return i
		}
	}
	n := nodeOf(e.widget)
	for i := range dst.frameSem() {
		s := &dst.frameSem()[i]
		if s.full == e.rect && s.node.Role == n.Role && s.node.Name == n.Name {
			return i
		}
	}
	if _, ok := e.widget.(*TextWidget); ok {
		for i := range dst.frameSem() {
			if dst.frameSem()[i].full == e.rect && dst.frameSem()[i].node.Name == inspectLabel(e) {
				return i
			}
		}
	}
	return -1
}
func inspectedBox(tr []traceEntry, sel int) inspectBox {
	e := &tr[sel]
	box, ok := e.widget.(*BoxWidget)
	// Standard controls paint their decorated Box as a child. Use that
	// exact, same-bounds box rather than inventing padding for a control.
	if !ok {
		for i := sel + 1; i < len(tr) && tr[i].depth > e.depth; i++ {
			if b, yes := tr[i].widget.(*BoxWidget); yes && tr[i].rect == e.rect {
				box, ok = b, true
				break
			}
		}
	}
	if !ok {
		return inspectBox{content: e.rect.Size}
	}
	content := box.childSize
	if box.child == nil {
		content = Sz(max(e.rect.Size.W-box.padding.horizontal(), 0), max(e.rect.Size.H-box.padding.vertical(), 0))
	}
	return inspectBox{box.padding, box.borderWidth, content, true}
}
func inspectColor(c color.Color) string {
	if c == nil {
		return "none"
	}
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	if n.A == 255 {
		return fmt.Sprintf("#%02x%02x%02x", n.R, n.G, n.B)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", n.R, n.G, n.B, n.A)
}
func flowFields(f *flow) []inspectField {
	justify := []string{"start", "center", "end", "space-between", "space-around", "space-evenly"}
	align := []string{"start", "center", "end", "stretch"}
	out := []inspectField{{"direction", pick(f.horizontal, "row", "column"), false}, {"justify", justify[clamp(int(f.justify), 0, len(justify)-1)], false}, {"align", align[clamp(int(f.align), 0, len(align)-1)], false}}
	if f.space > 0 {
		out = append(out, inspectField{"gap token", num(f.space) + " × theme.space", false})
	} else {
		out = append(out, inspectField{"gap", num(f.gap) + " px", true})
	}
	return out
}

// inspectDetails lists the fields of one details pane for the selection.
func inspectDetails(tab inspectTab, dst *Canvas, sel int) []inspectField {
	if sel < 0 {
		return nil
	}
	e := &dst.frameTrace()[sel]
	if tab == inspectTabSemantics {
		return semanticFields(dst, e)
	}
	if tab == inspectTabComputed {
		return computedFields(e)
	}
	parent := "root"
	if a := ancestors(dst.frameTrace(), sel); len(a) > 0 {
		parent = dst.frameTrace()[a[0]].name
	}
	kids := 0
	for i := sel + 1; i < len(dst.frameTrace()) && dst.frameTrace()[i].depth > e.depth; i++ {
		if dst.frameTrace()[i].depth == e.depth+1 {
			kids++
		}
	}
	out := []inspectField{{key: "Geometry"}, {"x", num(e.rect.Origin.X) + " px", true}, {"y", num(e.rect.Origin.Y) + " px", true}, {"width", num(e.rect.Size.W) + " px", true}, {"height", num(e.rect.Size.H) + " px", true}, {key: "Hierarchy"}, {"parent", parent, false}, {"children", strconv.Itoa(kids), true}, {"depth", strconv.Itoa(e.depth), true}}
	if e.clipped {
		out = append(out, inspectField{"visible size", num(e.rect.Intersect(e.clip).Size.W) + " × " + num(e.rect.Intersect(e.clip).Size.H), true})
	}
	if e.id != nil {
		out = append(out, inspectField{"identity", fmt.Sprint(e.id), false})
	}
	return out
}
func computedFields(e *traceEntry) []inspectField {
	out := []inspectField{{key: "Widget"}, {"type", e.name, false}, {"width", num(e.rect.Size.W) + " px", true}, {"height", num(e.rect.Size.H) + " px", true}}
	if name := inspectLabel(e); name != "" {
		out = append(out, inspectField{"label", name, false})
	}
	switch w := e.widget.(type) {
	case *BoxWidget:
		out = append(out, inspectField{key: "Box style"}, inspectField{"background", inspectColor(w.fill), false}, inspectField{"border", num(w.borderWidth) + " px " + inspectColor(w.borderColor), false}, inspectField{"radius", num(w.radius) + " px", true}, inspectField{"padding", fmt.Sprintf("%s %s %s %s", num(w.padding.Top), num(w.padding.Right), num(w.padding.Bottom), num(w.padding.Left)), true})
		if w.width > 0 {
			out = append(out, inspectField{"fixed width", num(w.width) + " px", true})
		}
		if w.height > 0 {
			out = append(out, inspectField{"fixed height", num(w.height) + " px", true})
		}
	case *TextWidget:
		out = append(out, inspectField{key: "Typography"}, inspectField{"font size", num(w.resolved.Size) + " px", true}, inspectField{"line height", num(w.resolved.LineHeight), true}, inspectField{"color", inspectColor(w.resolved.Color), false}, inspectField{"wrap", strconv.FormatBool(w.wrap), false}, inspectField{"lines", strconv.Itoa(len(w.lines)), true})
	case *RowWidget:
		out = append(out, inspectField{key: "Flex layout"})
		out = append(out, flowFields(&w.flow)...)
	case *ColumnWidget:
		out = append(out, inspectField{key: "Flex layout"})
		out = append(out, flowFields(&w.flow)...)
	case *GridWidget:
		out = append(out, inspectField{key: "Grid layout"}, inspectField{"columns", strconv.Itoa(w.cols), true}, inspectField{"column width", num(w.cellW) + " px", true}, inspectField{"column gap", num(w.gap) + " px", true}, inspectField{"row gap", num(w.rowGap) + " px", true})
	case *WrapWidget:
		out = append(out, inspectField{key: "Wrapping layout"}, inspectField{"gap", num(w.gap) + " px", true}, inspectField{"run gap", num(w.runGap) + " px", true})
	case *ScrollWidget:
		out = append(out, inspectField{key: "Scroll"}, inspectField{"axis", pick(w.horizontal, "horizontal", "vertical"), false}, inspectField{"offset", num(w.position()) + " px", true}, inspectField{"content", num(w.childSize.W) + " × " + num(w.childSize.H), true})
	case *FlexWidget:
		out = append(out, inspectField{"flex", num(w.flex), true})
	}
	if w, ok := e.widget.(interface{ state() *Interactive }); ok {
		s := w.state()
		out = append(out, inspectField{key: "Interaction"}, inspectField{"hovered", strconv.FormatBool(s.Hovered), false}, inspectField{"pressed", strconv.FormatBool(s.Pressed), false}, inspectField{"focused", strconv.FormatBool(s.Focused), false}, inspectField{"disabled", strconv.FormatBool(s.Inert), false})
	}
	return out
}

// semanticFields describes the widget's own accessibility node and ancestry.
func semanticFields(dst *Canvas, e *traceEntry) []inspectField {
	found := inspectSemantic(dst, e)
	if found < 0 {
		return []inspectField{{key: "Node"}, {"role", "none", false}}
	}
	n := &dst.frameSem()[found].node
	out := []inspectField{{key: "Node"}, {"role", string(n.Role), false}}
	add := func(k, v string) {
		if v != "" {
			out = append(out, inspectField{k, v, false})
		}
	}
	add("name", n.Name)
	add("description", n.Description)
	add("value", n.Value)
	switch n.Checked {
	case TriOff:
		add("checked", "false")
	case TriOn:
		add("checked", "true")
	case TriMixed:
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
		out = append(out, inspectField{"range", num(n.Min) + " – " + num(n.Max), true}, inspectField{"now", num(n.Now), true})
	}
	if chain := semanticChainFrom(dst, found); len(chain) > 1 {
		out = append(out, inspectField{key: "Path"})
		for _, c := range chain[:len(chain)-1] {
			out = append(out, inspectField{"", strings.TrimLeft(c, " "), false})
		}
	}
	return out
}

// num rounds logical pixel measurements to two decimal places.
func num(v float64) string { return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64) }

// semanticAt returns the innermost accessibility node painted under p, or
// -1: the last one described there, since parents describe before children.
func semanticAt(dst *Canvas, p Point) int {
	for i, v := range slices.Backward(dst.frameSem()) {
		if !v.node.Offscreen && v.rect.Contains(p) {
			return i
		}
	}
	return -1
}

// semanticChain describes the accessibility node under p and everything it
// is inside of, outermost first and indented, which is what a screen reader
// walks: the single role and label the inspector used to show could not say
// that a tab is inside a strip or an option inside its combobox.
func semanticChain(dst *Canvas, p Point) []string {
	found := semanticAt(dst, p)
	if found < 0 {
		return nil
	}
	return semanticChainFrom(dst, found)
}

func semanticChainFrom(dst *Canvas, found int) []string {
	var chain []int
	for i := found; i >= 0; i = dst.frameSem()[i].parent - 1 {
		chain = append(chain, i)
	}
	lines := make([]string, 0, len(chain))
	for i, c := range slices.Backward(chain) {
		e := &dst.frameSem()[c]
		n := SemNode{Node: e.node}
		lines = append(lines, fmt.Sprintf("%s%s %q%s",
			strings.Repeat("  ", len(chain)-1-i), e.node.Role, e.node.Name, n.flags()))
	}
	return lines
}
