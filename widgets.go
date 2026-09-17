package ggui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/basicfont"
)

// defaultFace is the placeholder font until font loading lands.
var defaultFace = text.NewGoXFace(basicfont.Face7x13)

// EdgeInsets is padding on the four sides of a box.
type EdgeInsets struct {
	Top, Right, Bottom, Left float64
}

// All returns insets with the same value on every side.
func All(v float64) EdgeInsets {
	return EdgeInsets{Top: v, Right: v, Bottom: v, Left: v}
}

func (e EdgeInsets) horizontal() float64 { return e.Left + e.Right }
func (e EdgeInsets) vertical() float64   { return e.Top + e.Bottom }

// Box paints a rectangle and lays a single optional child inside its padding.
type Box struct {
	Color   color.Color
	Padding EdgeInsets
	Width   float64 // 0 means "as small as the child allows"
	Height  float64
	Child   Widget

	size      Size
	childSize Size
}

// Layout implements Widget.
func (b *Box) Layout(c Constraints) Size {
	inner := Constraints{
		MaxW: c.MaxW - b.Padding.horizontal(),
		MaxH: c.MaxH - b.Padding.vertical(),
	}
	if b.Child != nil {
		b.childSize = b.Child.Layout(inner)
	} else {
		b.childSize = Size{}
	}

	want := Size{
		W: b.childSize.W + b.Padding.horizontal(),
		H: b.childSize.H + b.Padding.vertical(),
	}
	if b.Width > 0 {
		want.W = b.Width
	}
	if b.Height > 0 {
		want.H = b.Height
	}
	b.size = c.Constrain(want)
	return b.size
}

// Paint implements Widget.
func (b *Box) Paint(dst *ebiten.Image, at Point) {
	if b.Color != nil {
		vector.DrawFilledRect(dst,
			float32(at.X), float32(at.Y),
			float32(b.size.W), float32(b.size.H),
			b.Color, true)
	}
	if b.Child != nil {
		b.Child.Paint(dst, Point{at.X + b.Padding.Left, at.Y + b.Padding.Top})
	}
}

// Text draws a single line of text.
type Text struct {
	Value string
	Color color.Color

	size Size
}

// Layout implements Widget.
func (t *Text) Layout(c Constraints) Size {
	w, h := text.Measure(t.Value, defaultFace, 0)
	t.size = c.Constrain(Size{W: w, H: h})
	return t.size
}

// Paint implements Widget.
func (t *Text) Paint(dst *ebiten.Image, at Point) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(at.X, at.Y)
	if t.Color != nil {
		op.ColorScale.ScaleWithColor(t.Color)
	}
	text.Draw(dst, t.Value, defaultFace, op)
}

// Column stacks its children vertically, separated by Gap.
type Column struct {
	Gap      float64
	Children []Widget

	sizes []Size
	size  Size
}

// Layout implements Widget.
func (col *Column) Layout(c Constraints) Size {
	col.sizes = col.sizes[:0]
	var total Size
	remaining := c.MaxH
	for i, child := range col.Children {
		s := child.Layout(Constraints{MaxW: c.MaxW, MaxH: remaining})
		col.sizes = append(col.sizes, s)
		if s.W > total.W {
			total.W = s.W
		}
		total.H += s.H
		remaining -= s.H
		if i < len(col.Children)-1 {
			total.H += col.Gap
			remaining -= col.Gap
		}
	}
	col.size = c.Constrain(total)
	return col.size
}

// Paint implements Widget.
func (col *Column) Paint(dst *ebiten.Image, at Point) {
	y := at.Y
	for i, child := range col.Children {
		child.Paint(dst, Point{X: at.X, Y: y})
		y += col.sizes[i].H + col.Gap
	}
}

// Center places a single child in the middle of the space it is given.
type Center struct {
	Child Widget

	childSize Size
	size      Size
}

// Layout implements Widget.
func (c *Center) Layout(cs Constraints) Size {
	c.childSize = c.Child.Layout(Loose(Size{W: cs.MaxW, H: cs.MaxH}))
	c.size = Size{W: cs.MaxW, H: cs.MaxH}
	return c.size
}

// Paint implements Widget.
func (c *Center) Paint(dst *ebiten.Image, at Point) {
	c.Child.Paint(dst, Point{
		X: at.X + (c.size.W-c.childSize.W)/2,
		Y: at.Y + (c.size.H-c.childSize.H)/2,
	})
}

// List is a Column driven by data: it builds one child per item with Item, so
// the caller keeps its own slice instead of a []Widget. Children are rebuilt
// each layout pass, which keeps them in step with Items.
type List[T any] struct {
	Gap   float64
	Items []T
	Item  func(T) Widget

	col Column
}

// Layout implements Widget.
func (l *List[T]) Layout(c Constraints) Size {
	l.col.Gap = l.Gap
	l.col.Children = Children(l.Items, l.Item)
	return l.col.Layout(c)
}

// Paint implements Widget.
func (l *List[T]) Paint(dst *ebiten.Image, at Point) { l.col.Paint(dst, at) }
