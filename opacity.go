package ggui

import "github.com/ironpark/ggui/internal/property"

// OpacityWidget paints its child as a whole at an opacity: overlapping parts
// of the child do not show through each other, as they would if each were
// faded on its own. Build one with Opacity.
type OpacityWidget struct {
	props   property.Owner
	child   Widget
	opacity property.Value[float64]
	shown   float64
}

// Opacity paints child at opacity, from 0, invisible, to 1, as it is. An
// invisible child is still laid out and still takes input, as Flutter's
// Opacity does; wrap it in Inert as well to stop that.
func Opacity(child Widget, opacity float64) *OpacityWidget {
	o := &OpacityWidget{child: child}
	o.opacity.Set(opacity)
	return o
}

// Opacity sets the opacity, clamped to [0, 1].
func (o *OpacityWidget) Opacity(v float64) *OpacityWidget {
	if o.opacity.Set(v) {
		o.props.Changed()
	}
	return o
}

// BindOpacity follows a non-nil opacity reader, a Tween say.
func (o *OpacityWidget) BindOpacity(r Readable[float64]) *OpacityWidget {
	if o.opacity.Bind(r, "BindOpacity") {
		o.props.Changed()
	}
	return o
}

// Baseline implements Baseliner: the child's.
func (o *OpacityWidget) Baseline() (float64, bool) { return baselineOf(o.child) }

// Layout implements Widget.
func (o *OpacityWidget) Layout(c Constraints, env Env) Size {
	defer o.props.Layout()()
	o.shown = clamp(o.opacity.Get(), 0, 1)
	return o.child.Layout(c, env)
}

// Paint implements Widget. A fully opaque child paints directly, with no
// layer.
func (o *OpacityWidget) Paint(dst *Canvas, r Rect) {
	if o.shown >= 1 {
		dst.Paint(o.child, r)
		return
	}
	dst.Layer(LayerOptions{Fade: 1 - o.shown}, func(layer *Canvas) { layer.Paint(o.child, r) })
}
