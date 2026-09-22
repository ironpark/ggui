package ggui

import (
	"fmt"
	"strconv"

	"github.com/ironpark/ggui/inspect"
)

// What the built-in widgets tell the inspector about themselves. Each one
// implements inspect.Fielder for the Computed pane; a custom widget does
// the same to be inspected beyond its name and bounds. These compile in
// every build, being small; the panel that reads them does not.

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
		return append(out, inspect.Field{Key: "gap token", Value: inspect.Num(f.space) + " × theme.space"})
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
