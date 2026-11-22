package widget

import (
	"fmt"
	"image/color"
	"slices"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/inspect"
	"github.com/ironpark/ggui/layout"
	"github.com/ironpark/ggui/property"
)

// BoxWidget paints a rectangle and lays an optional child inside its padding.
// Build one with Box.
type BoxWidget struct {
	widthProp   property.Value[float64]
	heightProp  property.Value[float64]
	props       property.Owner
	fill        shownColor
	deco        *boxDeco // shadows and gradient, made when one is first set
	radius      float64
	borderWidth float64
	borderColor shownColor

	padding    ggui.EdgeInsets
	width      float64 // 0 means "as small as the child allows"
	height     float64
	maxW, maxH float64 // 0 means no limit
	child      ggui.Widget

	childRect ggui.Rect // inside the padding, from the last Layout
}

// boxDeco is what few boxes paint: shadows, a gradient and a clip. A
// BoxWidget makes one only when one is set.
type boxDeco struct {
	shadows       []ggui.ShadowStyle
	gradient      ggui.Gradient      // as given; see BoxWidget.Gradient
	shownShadows  []ggui.ShadowStyle // shadows with their colors resolved
	shownGradient ggui.Gradient      // gradient with its colors resolved

	// paintClipped paints the child at childAt; it is made once, when
	// Clip is first called, so a clipped Paint makes no closure.
	paintClipped func(dst *ggui.Canvas)
	childAt      ggui.Rect
}

// shownColor is a color property as it was given and as it is painted: the
// given color itself, or an EnvColor's value in the Env of the last layout.
type shownColor struct{ given, shown color.Color }

// set gives the property c. A widget lays out again for an EnvColor, given
// or replaced, since only layout knows its Env.
func (s *shownColor) set(c color.Color, changed func()) {
	if c == s.given {
		return
	}
	if ggui.IsEnvColor(c) || ggui.IsEnvColor(s.given) {
		changed()
	}
	s.given, s.shown = c, c
}

func (s *shownColor) layout(env ggui.Env) { s.shown = ggui.ResolveColor(s.given, env) }

// decorate returns the box's boxDeco, making it on first use.
func (b *BoxWidget) decorate() *boxDeco {
	if b.deco == nil {
		b.deco = new(boxDeco)
	}
	return b.deco
}

// Gradient fills the box with g, over its Fill and under its border, with
// the same rounded corners. A zero Gradient removes it. EnvColor stops are
// resolved at layout, as Fill's color is.
func (b *BoxWidget) Gradient(g ggui.Gradient) *BoxWidget {
	var was ggui.Gradient
	if b.deco != nil {
		was = b.deco.gradient
	}
	if property.Equal(g, was) {
		return b
	}
	if g.Themed() || was.Themed() {
		b.props.Changed()
	}
	d := b.decorate()
	d.gradient, d.shownGradient = g, g
	return b
}

// Box wraps at most one child. With no child it is an empty rectangle, sized
// with Size, Width or Height.
func Box(child ...ggui.Widget) *BoxWidget {
	b := &BoxWidget{}
	switch len(child) {
	case 0:
	case 1:
		b.child = child[0]
	default:
		panic("ggui: Box takes at most one child; wrap several in a Column")
	}
	return b
}

// Fill sets the background color. Nil paints nothing.
func (b *BoxWidget) Fill(c color.Color) *BoxWidget {
	b.fill.set(c, b.props.Changed)
	return b
}

// Shadow replaces the outer shadow layers. Calling it without arguments clears
// them. Shadows paint in argument order and do not reserve layout space.
func (b *BoxWidget) Shadow(styles ...ggui.ShadowStyle) *BoxWidget {
	if b.deco == nil && len(styles) == 0 {
		return b
	}
	d := b.decorate()
	themed := func(s ggui.ShadowStyle) bool { return ggui.IsEnvColor(s.Color) }
	if !slices.Equal(d.shadows, styles) && (slices.ContainsFunc(d.shadows, themed) || slices.ContainsFunc(styles, themed)) {
		b.props.Changed()
	}
	d.shadows = append(d.shadows[:0], styles...)
	d.shownShadows = append(d.shownShadows[:0], styles...)
	return b
}

// Radius rounds the corners of the fill and border.
func (b *BoxWidget) Radius(r float64) *BoxWidget {
	b.radius = r
	return b
}

// Clip paints the child clipped to the inside of the box: within the
// border, with the corners rounded by Radius less the border's width, or
// square when there is no Radius. Content that runs to the box's edge, the
// last row of a table or an image, then keeps to its rounded corners. Input
// is clipped to that inside as a rectangle.
func (b *BoxWidget) Clip() *BoxWidget {
	d := b.decorate()
	if d.paintClipped == nil {
		d.paintClipped = func(dst *ggui.Canvas) { dst.Paint(b.child, d.childAt) }
	}
	return b
}

// Border draws a line of width w in color c just inside the edge.
func (b *BoxWidget) Border(w float64, c color.Color) *BoxWidget {
	b.borderWidth = w
	b.borderColor.set(c, b.props.Changed)
	return b
}

// Pad sets padding with the CSS shorthand Insets accepts.
func (b *BoxWidget) Pad(sides ...float64) *BoxWidget {
	defer property.Watch(&b.props, &b.padding)()
	b.padding = ggui.Insets(sides...)
	return b
}

// Padding sets per-side padding.
func (b *BoxWidget) Padding(e ggui.EdgeInsets) *BoxWidget {
	defer property.Watch(&b.props, &b.padding)()
	b.padding = e
	return b
}

// Size fixes both dimensions. Zero leaves that dimension to the child.
func (b *BoxWidget) Size(w, h float64) *BoxWidget { return b.Width(w).Height(h) }

// Width fixes the width. Zero leaves it to the child.
func (b *BoxWidget) Width(w float64) *BoxWidget {
	if b.widthProp.Set(w) {
		b.props.Changed()
	}
	b.width = w
	return b
}

// BindWidth follows a non-nil width reader without rebuilding the box.
func (b *BoxWidget) BindWidth(r ggui.Readable[float64]) *BoxWidget {
	if b.widthProp.Bind(r, "BindWidth") {
		b.props.Changed()
	}
	return b
}

// Height fixes the height. Zero leaves it to the child.
func (b *BoxWidget) Height(h float64) *BoxWidget {
	if b.heightProp.Set(h) {
		b.props.Changed()
	}
	b.height = h
	return b
}

// BindHeight follows a non-nil height reader without rebuilding the box.
func (b *BoxWidget) BindHeight(r ggui.Readable[float64]) *BoxWidget {
	if b.heightProp.Bind(r, "BindHeight") {
		b.props.Changed()
	}
	return b
}

// MaxWidth caps the width, padding included, so a form or a paragraph
// stops growing in a wide window; zero removes the cap. A parent that
// demands more, such as a stretching Column, still wins: put the box in an
// Align or Center to keep it narrow there. A fixed Width above the cap is
// cut to it.
func (b *BoxWidget) MaxWidth(w float64) *BoxWidget {
	defer property.Watch(&b.props, &b.maxW)()
	b.maxW = w
	return b
}

// MaxHeight caps the height the way MaxWidth caps the width.
func (b *BoxWidget) MaxHeight(h float64) *BoxWidget {
	defer property.Watch(&b.props, &b.maxH)()
	b.maxH = h
	return b
}

// Layout implements Widget.
func (b *BoxWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer b.props.Layout().EndLayout()
	b.fill.layout(env)
	b.borderColor.layout(env)
	if d := b.deco; d != nil {
		d.shownGradient = d.gradient.Resolve(env)
		for i, s := range d.shadows {
			d.shownShadows[i].Color = ggui.ResolveColor(s.Color, env)
		}
	}
	b.width, b.height = b.widthProp.Get(), b.heightProp.Get()
	var measure layout.MeasureFunc
	if b.child != nil {
		measure = func(_ int, c ggui.Constraints) layout.Measurement { return measureWidget(b.child, c, env) }
	}
	result := layout.Box(c, layout.BoxStyle{
		Padding: b.padding, Width: b.width, Height: b.height,
		MaxWidth: b.maxW, MaxHeight: b.maxH,
	}, measure)
	b.childRect = result.Child
	return result.Size
}

// Baseline implements Baseliner: the child's, below the top padding.
func (b *BoxWidget) Baseline() (float64, bool) { return baselineAt(b.child, b.padding.Top) }

// Paint implements Widget.
func (b *BoxWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if b.deco != nil {
		for _, s := range b.deco.shownShadows {
			dst.Shadow(r, b.radius, s)
		}
	}
	dst.FillRoundRect(r, b.radius, b.fill.shown)
	if b.deco != nil {
		dst.FillGradientRoundRect(r, b.radius, b.deco.shownGradient)
	}
	if b.borderWidth > 0 {
		dst.StrokeRoundRect(r, b.radius, b.borderWidth, b.borderColor.shown)
	}
	if b.child == nil {
		return
	}
	at := ggui.Rct(r.Origin.Add(b.childRect.Origin), b.childRect.Size)
	if b.deco == nil || b.deco.paintClipped == nil {
		dst.Paint(b.child, at)
		return
	}
	bw := b.borderWidth
	inner := ggui.Rct(r.Origin.Add(ggui.Pt(bw, bw)), ggui.Sz(max(r.Size.W-2*bw, 0), max(r.Size.H-2*bw, 0)))
	b.deco.childAt = at
	dst.ClipRoundRect(inner, max(min(b.radius-bw, inner.Size.W/2, inner.Size.H/2), 0), b.deco.paintClipped)
}

// InspectFields implements inspect.Fielder.
func (b *BoxWidget) InspectFields() []inspect.Field {
	out := []inspect.Field{{Key: "Box style"}, {Key: "background", Value: inspect.Color(b.fill.shown)}, {Key: "border", Value: inspect.Num(b.borderWidth) + " px " + inspect.Color(b.borderColor.shown)}, {Key: "radius", Value: inspect.Num(b.radius) + " px", Number: true}, {Key: "padding", Value: fmt.Sprintf("%s %s %s %s", inspect.Num(b.padding.Top), inspect.Num(b.padding.Right), inspect.Num(b.padding.Bottom), inspect.Num(b.padding.Left)), Number: true}}
	if b.width > 0 {
		out = append(out, inspect.Field{Key: "fixed width", Value: inspect.Num(b.width) + " px", Number: true})
	}
	if b.height > 0 {
		out = append(out, inspect.Field{Key: "fixed height", Value: inspect.Num(b.height) + " px", Number: true})
	}
	if b.maxW > 0 {
		out = append(out, inspect.Field{Key: "max width", Value: inspect.Num(b.maxW) + " px", Number: true})
	}
	if b.maxH > 0 {
		out = append(out, inspect.Field{Key: "max height", Value: inspect.Num(b.maxH) + " px", Number: true})
	}
	return out
}

// InspectBox implements inspect.Boxer: the box model as painted at size,
// its padding and border around the child, or around the space left when
// it has none.
func (b *BoxWidget) InspectBox(size ggui.Size) inspect.Box {
	content := b.childRect.Size
	if b.child == nil {
		content = ggui.Sz(max(size.W-b.padding.Horizontal(), 0), max(size.H-b.padding.Vertical(), 0))
	}
	p := b.padding
	return inspect.Box{Padding: inspect.Insets{Top: p.Top, Right: p.Right, Bottom: p.Bottom, Left: p.Left}, Border: b.borderWidth, Content: content, Valid: true}
}

// Padding surrounds child with empty space, using the CSS shorthand Insets
// accepts: Padding(w, 8), Padding(w, 4, 12) or Padding(w, 1, 2, 3, 4). It is
// a Box with no fill, so the rest of Box's setters stay available.
func Padding(child ggui.Widget, sides ...float64) *BoxWidget { return Box(child).Pad(sides...) }
