package ggui

import "github.com/hajimehoshi/ebiten/v2"

// Widget is the unit of composition: it is asked for a size under some
// Constraints, then asked to paint itself at an origin. Layout is always
// called before Paint for a given frame.
type Widget interface {
	Layout(c Constraints) Size
	Paint(dst *ebiten.Image, at Point)
}

// Builder turns state into a Widget tree. A component in this framework is a
// plain Go function of this shape; it re-runs when the signals it reads change.
type Builder func() Widget

// Children builds one Widget per item. It is the bridge from data to tree:
//
//	&Column{Children: Children(rows, func(r Row) Widget { return rowWidget(r) })}
func Children[T any](items []T, build func(T) Widget) []Widget {
	out := make([]Widget, len(items))
	for i, item := range items {
		out[i] = build(item)
	}
	return out
}

// LayoutFunc/PaintFunc let a widget be written inline without a named type.
type widgetFunc struct {
	layout func(Constraints) Size
	paint  func(*ebiten.Image, Point)
}

func (w widgetFunc) Layout(c Constraints) Size         { return w.layout(c) }
func (w widgetFunc) Paint(dst *ebiten.Image, at Point) { w.paint(dst, at) }

// FromFuncs builds a Widget from a layout and a paint function.
func FromFuncs(layout func(Constraints) Size, paint func(*ebiten.Image, Point)) Widget {
	return widgetFunc{layout: layout, paint: paint}
}
