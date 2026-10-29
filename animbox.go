package ggui

import (
	"image/color"
	"math"
	"time"

	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
)

// AnimatedBoxWidget is a Box whose fill, border, corner radius, size and
// padding ease to each new value instead of jumping, whether the value is
// set again or comes from a bound reader. Build one with AnimatedBox.
//
// Theme colors are resolved before they blend, so a theme switch fades the
// box too. Under reduced motion every change lands at once. The motion
// lives in the widget: keep one box and change its values, rather than
// building a new box with the new values, which starts where it is told.
type AnimatedBoxWidget struct {
	props       property.Owner
	box         *BoxWidget // painted with the values of the moment
	duration    time.Duration
	ease        Easing
	collapsible bool // a size of zero is zero, eased like any other

	fill, border            property.Value[color.Color]
	width, height           property.Value[float64]
	radius, borderWidth     property.Value[float64]
	padding                 property.Value[EdgeInsets]
	fillTo, borderTo        follower[color.Color]
	widthTo, heightTo       follower[float64]
	radiusTo, borderWidthTo follower[float64]
	paddingTo               follower[EdgeInsets]
	size                    Size // as the last layout left it
}

// AnimatedBox wraps at most one child, as Box does, and animates its looks
// over 200ms with EaseOut until Duration and Easing say otherwise.
//
//	card := ggui.AnimatedBox(content).BindFill(tint).Radius(8).Pad(12)
func AnimatedBox(child ...Widget) *AnimatedBoxWidget {
	return &AnimatedBoxWidget{box: Box(child...), duration: 200 * time.Millisecond, ease: EaseOut}
}

// animSet assigns a literal and lays out again if it changed.
func animSet[T any](a *AnimatedBoxWidget, p *property.Value[T], v T) *AnimatedBoxWidget {
	property.Set(&a.props, p, v)
	return a
}

// animBind follows a reader and lays out again if it is a new one.
func animBind[T any](a *AnimatedBoxWidget, p *property.Value[T], r Readable[T], name string) *AnimatedBoxWidget {
	property.Bind(&a.props, p, r, name)
	return a
}

// Duration sets how long each change takes.
func (a *AnimatedBoxWidget) Duration(d time.Duration) *AnimatedBoxWidget {
	defer property.Watch(&a.props, &a.duration)()
	a.duration = d
	return a
}

// Easing sets the curve; see EaseLinear, EaseIn, EaseOut, EaseInOut.
func (a *AnimatedBoxWidget) Easing(e Easing) *AnimatedBoxWidget {
	a.ease = e
	return a
}

// Fill sets the background color. Nil paints nothing, and fades from or to
// transparent.
func (a *AnimatedBoxWidget) Fill(c color.Color) *AnimatedBoxWidget { return animSet(a, &a.fill, c) }

// BindFill follows a non-nil color reader, easing to each new color.
func (a *AnimatedBoxWidget) BindFill(r Readable[color.Color]) *AnimatedBoxWidget {
	return animBind(a, &a.fill, r, "BindFill")
}

// Border draws a line of width w in color c just inside the edge.
func (a *AnimatedBoxWidget) Border(w float64, c color.Color) *AnimatedBoxWidget {
	return animSet(animSet(a, &a.borderWidth, w), &a.border, c)
}

// BindBorderColor follows a non-nil reader for the border's color.
func (a *AnimatedBoxWidget) BindBorderColor(r Readable[color.Color]) *AnimatedBoxWidget {
	return animBind(a, &a.border, r, "BindBorderColor")
}

// Radius rounds the corners of the fill and border.
func (a *AnimatedBoxWidget) Radius(r float64) *AnimatedBoxWidget { return animSet(a, &a.radius, r) }

// BindRadius follows a non-nil corner radius reader.
func (a *AnimatedBoxWidget) BindRadius(r Readable[float64]) *AnimatedBoxWidget {
	return animBind(a, &a.radius, r, "BindRadius")
}

// Size fixes both dimensions. Zero leaves that dimension to the child.
func (a *AnimatedBoxWidget) Size(w, h float64) *AnimatedBoxWidget { return a.Width(w).Height(h) }

// Width fixes the width. Zero leaves it to the child: a change to zero
// lands at once, and one from zero eases from the size the child gave.
func (a *AnimatedBoxWidget) Width(w float64) *AnimatedBoxWidget { return animSet(a, &a.width, w) }

// BindWidth follows a non-nil width reader.
func (a *AnimatedBoxWidget) BindWidth(r Readable[float64]) *AnimatedBoxWidget {
	return animBind(a, &a.width, r, "BindWidth")
}

// Collapsible makes a width or height of zero mean zero, eased to and from
// like any other size, rather than leaving that dimension to the child: a
// gap that opens from nothing and closes back to it.
func (a *AnimatedBoxWidget) Collapsible() *AnimatedBoxWidget {
	defer property.Watch(&a.props, &a.collapsible)()
	a.collapsible = true
	return a
}

// Height fixes the height. Zero leaves it to the child, as in Width.
func (a *AnimatedBoxWidget) Height(h float64) *AnimatedBoxWidget { return animSet(a, &a.height, h) }

// BindHeight follows a non-nil height reader.
func (a *AnimatedBoxWidget) BindHeight(r Readable[float64]) *AnimatedBoxWidget {
	return animBind(a, &a.height, r, "BindHeight")
}

// Pad sets padding with the CSS shorthand Insets accepts.
func (a *AnimatedBoxWidget) Pad(sides ...float64) *AnimatedBoxWidget {
	return animSet(a, &a.padding, Insets(sides...))
}

// Padding sets per-side padding.
func (a *AnimatedBoxWidget) Padding(e EdgeInsets) *AnimatedBoxWidget {
	return animSet(a, &a.padding, e)
}

// BindPadding follows a non-nil padding reader.
func (a *AnimatedBoxWidget) BindPadding(r Readable[EdgeInsets]) *AnimatedBoxWidget {
	return animBind(a, &a.padding, r, "BindPadding")
}

// Layout implements Widget.
func (a *AnimatedBoxWidget) Layout(c Constraints, env Env) Size {
	defer a.props.Layout().EndLayout()
	d := env.Motion(a.duration)
	b := a.box
	b.Fill(a.fillTo.follow(ResolveColor(a.fill.Get(), env), d, a.ease, LerpColor))
	b.Border(a.borderWidthTo.follow(a.borderWidth.Get(), d, a.ease, LerpFloat),
		a.borderTo.follow(ResolveColor(a.border.Get(), env), d, a.ease, LerpColor))
	b.Radius(a.radiusTo.follow(a.radius.Get(), d, a.ease, LerpFloat))
	b.Width(a.boxSize(followSize(&a.widthTo, a.width.Get(), a.size.W, d, a.ease, a.collapsible)))
	b.Height(a.boxSize(followSize(&a.heightTo, a.height.Get(), a.size.H, d, a.ease, a.collapsible)))
	b.Padding(a.paddingTo.follow(a.padding.Get(), d, a.ease, LerpInsets))
	a.size = b.Layout(c, env)
	return a.size
}

// Paint implements Widget.
func (a *AnimatedBoxWidget) Paint(dst *Canvas, r Rect) { a.box.Paint(dst, r) }

// Baseline implements Baseliner: the child's, below the top padding.
func (a *AnimatedBoxWidget) Baseline() (float64, bool) { return a.box.Baseline() }

// follower eases one property of a widget toward the value its layout
// asks for. Its tween is made at the first layout, in that frame's world,
// and owned by nothing: it stops when it settles.
type follower[T any] struct{ tw *Tweened[T] }

// follow heads for target over d and returns the value of the moment; a
// layout that calls it reads the tween, so it lays out again as it moves.
func (f *follower[T]) follow(target T, d time.Duration, e Easing, lerp func(a, b T, t float64) T) T {
	if d <= 0 {
		if f.tw != nil && f.tw.running {
			f.tw.Jump(target)
		}
		f.tw = nil
		return target
	}
	if f.tw == nil {
		reactive.WithOwner(nil, func() { f.tw = TweenWith(target, d, lerp) })
		return target
	}
	if !property.Equal(f.tw.Target(), target) {
		f.tw.Duration(d).Easing(e).Set(target)
	}
	return f.tw.Get()
}

// boxSize is the size the inner Box is given for v: where zero is zero,
// one too small to see, as a Box takes zero to mean its child's size.
func (a *AnimatedBoxWidget) boxSize(v float64) float64 {
	if a.collapsible && v <= 0 {
		return math.SmallestNonzeroFloat64
	}
	return v
}

// followSize eases a width or height, where zero is no number on the way
// but "as the child sizes it": a move to zero lands at once, and a move
// away from it starts at shown, the size the box last had. A collapsible
// box's zero is a number like any other.
func followSize(f *follower[float64], target, shown float64, d time.Duration, e Easing, collapsible bool) float64 {
	if collapsible {
		return f.follow(target, d, e, LerpFloat)
	}
	if target <= 0 {
		d = 0
	} else if f.tw == nil && shown > 0 && d > 0 {
		reactive.WithOwner(nil, func() { f.tw = TweenWith(shown, d, LerpFloat) })
	}
	return f.follow(target, d, e, LerpFloat)
}
