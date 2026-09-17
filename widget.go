package ggui

import "github.com/hajimehoshi/ebiten/v2"

// Widget is the unit of composition. Layout asks it for a size under some
// Constraints; Paint then hands it the Rect its parent assigned: the origin the
// parent chose and the size Layout returned. Layout always runs before Paint in
// a frame, so a widget keeps only what the Rect cannot tell it, such as where
// its children go. Leaf widgets usually keep nothing at all.
type Widget interface {
	Layout(c Constraints) Size
	Paint(dst *ebiten.Image, r Rect)
}

// Builder turns state into a Widget tree. A component in this framework is a
// plain Go function of this shape; it re-runs when the signals it reads change.
type Builder func() Widget

// Children builds one Widget per item. It is the bridge from data to tree for
// any widget that takes children:
//
//	Column(Children(rows, func(r Row) Widget { return rowWidget(r) })...)
func Children[T any](items []T, build func(T) Widget) []Widget {
	out := make([]Widget, len(items))
	for i, item := range items {
		out[i] = build(item)
	}
	return out
}

type widgetFunc struct {
	layout func(Constraints) Size
	paint  func(*ebiten.Image, Rect)
}

func (w widgetFunc) Layout(c Constraints) Size       { return w.layout(c) }
func (w widgetFunc) Paint(dst *ebiten.Image, r Rect) { w.paint(dst, r) }

// FromFuncs builds a Widget from a layout and a paint function, for one-off
// widgets that do not deserve a named type.
func FromFuncs(layout func(Constraints) Size, paint func(*ebiten.Image, Rect)) Widget {
	return widgetFunc{layout: layout, paint: paint}
}
