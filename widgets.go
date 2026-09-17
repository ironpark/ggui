package ggui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/basicfont"
)

// Built-in widgets follow one shape: a constructor takes what the widget
// cannot do without (its text, its children), and chainable setters take the
// rest. Setters mutate and return the receiver, so
//
//	Box(Text("hi").Color(fg)).Pad(8).Fill(bg)
//
// reads as the tree it builds. Widget types end in Widget so the short names
// stay free for the constructors.

// defaultFace is the placeholder font until font loading lands.
var defaultFace = text.NewGoXFace(basicfont.Face7x13)

// EdgeInsets is padding on the four sides of a box.
type EdgeInsets struct {
	Top, Right, Bottom, Left float64
}

// All returns insets with the same value on every side.
func All(v float64) EdgeInsets { return Insets(v) }

// Insets builds EdgeInsets with CSS shorthand: one value for every side, two
// for vertical then horizontal, four for top, right, bottom, left.
func Insets(sides ...float64) EdgeInsets {
	switch len(sides) {
	case 0:
		return EdgeInsets{}
	case 1:
		return EdgeInsets{Top: sides[0], Right: sides[0], Bottom: sides[0], Left: sides[0]}
	case 2:
		return EdgeInsets{Top: sides[0], Right: sides[1], Bottom: sides[0], Left: sides[1]}
	case 4:
		return EdgeInsets{Top: sides[0], Right: sides[1], Bottom: sides[2], Left: sides[3]}
	default:
		panic("ggui: Insets takes 1, 2 or 4 values")
	}
}

// Shrink returns constraints with the insets removed from the maximums.
func (e EdgeInsets) Shrink(c Constraints) Constraints {
	return Constraints{
		MinW: max(c.MinW-e.horizontal(), 0),
		MinH: max(c.MinH-e.vertical(), 0),
		MaxW: max(c.MaxW-e.horizontal(), 0),
		MaxH: max(c.MaxH-e.vertical(), 0),
	}
}

// Inflate returns s grown by the insets.
func (e EdgeInsets) Inflate(s Size) Size {
	return Size{W: s.W + e.horizontal(), H: s.H + e.vertical()}
}

func (e EdgeInsets) horizontal() float64 { return e.Left + e.Right }
func (e EdgeInsets) vertical() float64   { return e.Top + e.Bottom }

// TextWidget draws a single line of text. Build one with Text.
type TextWidget struct {
	value string
	color color.Color
}

// Text draws s on a single line.
func Text(s string) *TextWidget { return &TextWidget{value: s} }

// Color sets the text color.
func (t *TextWidget) Color(c color.Color) *TextWidget { t.color = c; return t }

// Layout implements Widget.
func (t *TextWidget) Layout(c Constraints) Size {
	w, h := text.Measure(t.value, defaultFace, 0)
	return c.Constrain(Sz(w, h))
}

// Paint implements Widget.
func (t *TextWidget) Paint(dst *ebiten.Image, r Rect) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(r.Origin.X, r.Origin.Y)
	if t.color != nil {
		op.ColorScale.ScaleWithColor(t.color)
	}
	text.Draw(dst, t.value, defaultFace, op)
}

// BoxWidget paints a rectangle and lays an optional child inside its padding.
// Build one with Box.
type BoxWidget struct {
	fill    color.Color
	padding EdgeInsets
	width   float64 // 0 means "as small as the child allows"
	height  float64
	child   Widget

	childSize Size
}

// Box wraps at most one child. With no child it is an empty rectangle, sized
// with Size, Width or Height.
func Box(child ...Widget) *BoxWidget {
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
func (b *BoxWidget) Fill(c color.Color) *BoxWidget { b.fill = c; return b }

// Pad sets padding with the CSS shorthand Insets accepts.
func (b *BoxWidget) Pad(sides ...float64) *BoxWidget { b.padding = Insets(sides...); return b }

// Padding sets per-side padding.
func (b *BoxWidget) Padding(e EdgeInsets) *BoxWidget { b.padding = e; return b }

// Size fixes both dimensions. Zero leaves that dimension to the child.
func (b *BoxWidget) Size(w, h float64) *BoxWidget { b.width, b.height = w, h; return b }

// Width fixes the width. Zero leaves it to the child.
func (b *BoxWidget) Width(w float64) *BoxWidget { b.width = w; return b }

// Height fixes the height. Zero leaves it to the child.
func (b *BoxWidget) Height(h float64) *BoxWidget { b.height = h; return b }

// Layout implements Widget.
func (b *BoxWidget) Layout(c Constraints) Size {
	b.childSize = Size{}
	if b.child != nil {
		b.childSize = b.child.Layout(Loose(b.padding.Shrink(c).Max()))
	}
	want := b.padding.Inflate(b.childSize)
	if b.width > 0 {
		want.W = b.width
	}
	if b.height > 0 {
		want.H = b.height
	}
	return c.Constrain(want)
}

// Paint implements Widget.
func (b *BoxWidget) Paint(dst *ebiten.Image, r Rect) {
	if b.fill != nil {
		vector.DrawFilledRect(dst,
			float32(r.Origin.X), float32(r.Origin.Y),
			float32(r.Size.W), float32(r.Size.H),
			b.fill, true)
	}
	if b.child != nil {
		b.child.Paint(dst, Rect{
			Origin: r.Origin.Add(Pt(b.padding.Left, b.padding.Top)),
			Size:   b.childSize,
		})
	}
}

// flow lays children out along one axis, separated by gap. Column and Row
// are the two orientations of it.
type flow struct {
	horizontal bool
	gap        float64
	children   []Widget

	sizes []Size
}

func (f *flow) layout(c Constraints) Size {
	f.sizes = f.sizes[:0]
	var total Size
	remaining := c.Max()
	for i, child := range f.children {
		s := child.Layout(Loose(remaining))
		f.sizes = append(f.sizes, s)
		gap := f.gap
		if i == len(f.children)-1 {
			gap = 0
		}
		if f.horizontal {
			total.W += s.W + gap
			total.H = max(total.H, s.H)
			remaining.W -= s.W + gap
		} else {
			total.H += s.H + gap
			total.W = max(total.W, s.W)
			remaining.H -= s.H + gap
		}
	}
	return c.Constrain(total)
}

func (f *flow) paint(dst *ebiten.Image, r Rect) {
	at := r.Origin
	for i, child := range f.children {
		child.Paint(dst, Rct(at, f.sizes[i]))
		if f.horizontal {
			at.X += f.sizes[i].W + f.gap
		} else {
			at.Y += f.sizes[i].H + f.gap
		}
	}
}

// ColumnWidget stacks its children vertically. Build one with Column.
type ColumnWidget struct{ flow }

// Column stacks children top to bottom.
func Column(children ...Widget) *ColumnWidget {
	return &ColumnWidget{flow{children: children}}
}

// Gap sets the space between consecutive children.
func (col *ColumnWidget) Gap(v float64) *ColumnWidget { col.gap = v; return col }

// Layout implements Widget.
func (col *ColumnWidget) Layout(c Constraints) Size { return col.layout(c) }

// Paint implements Widget.
func (col *ColumnWidget) Paint(dst *ebiten.Image, r Rect) { col.paint(dst, r) }

// RowWidget lines its children up horizontally. Build one with Row.
type RowWidget struct{ flow }

// Row lines children up left to right.
func Row(children ...Widget) *RowWidget {
	return &RowWidget{flow{horizontal: true, children: children}}
}

// Gap sets the space between consecutive children.
func (row *RowWidget) Gap(v float64) *RowWidget { row.gap = v; return row }

// Layout implements Widget.
func (row *RowWidget) Layout(c Constraints) Size { return row.layout(c) }

// Paint implements Widget.
func (row *RowWidget) Paint(dst *ebiten.Image, r Rect) { row.paint(dst, r) }

// PaddingWidget surrounds one child with empty space. Build one with Padding.
type PaddingWidget struct {
	insets EdgeInsets
	child  Widget

	childSize Size
}

// Padding surrounds child with the CSS shorthand Insets accepts:
// Padding(w, 8), Padding(w, 4, 12) or Padding(w, 1, 2, 3, 4).
func Padding(child Widget, sides ...float64) *PaddingWidget {
	return &PaddingWidget{child: child, insets: Insets(sides...)}
}

// Insets sets per-side padding from an EdgeInsets.
func (p *PaddingWidget) Insets(e EdgeInsets) *PaddingWidget { p.insets = e; return p }

// Layout implements Widget.
func (p *PaddingWidget) Layout(c Constraints) Size {
	p.childSize = p.child.Layout(p.insets.Shrink(c))
	return c.Constrain(p.insets.Inflate(p.childSize))
}

// Paint implements Widget.
func (p *PaddingWidget) Paint(dst *ebiten.Image, r Rect) {
	p.child.Paint(dst, Rct(r.Origin.Add(Pt(p.insets.Left, p.insets.Top)), p.childSize))
}

// StackWidget layers its children on top of each other, first at the bottom.
// Build one with Stack.
type StackWidget struct {
	expand   bool
	children []Widget

	sizes []Size
}

// Stack layers children in order, all anchored at the top-left corner. It is
// as large as its largest child unless Expand is set.
func Stack(children ...Widget) *StackWidget {
	return &StackWidget{children: children}
}

// Expand makes the stack fill the space it is given instead of hugging its
// largest child.
func (st *StackWidget) Expand() *StackWidget { st.expand = true; return st }

// Layout implements Widget.
func (st *StackWidget) Layout(c Constraints) Size {
	st.sizes = st.sizes[:0]
	var total Size
	for _, child := range st.children {
		s := child.Layout(Loose(c.Max()))
		st.sizes = append(st.sizes, s)
		total.W = max(total.W, s.W)
		total.H = max(total.H, s.H)
	}
	if st.expand {
		total = c.Max()
	}
	return c.Constrain(total)
}

// Paint implements Widget.
func (st *StackWidget) Paint(dst *ebiten.Image, r Rect) {
	for i, child := range st.children {
		child.Paint(dst, Rct(r.Origin, st.sizes[i]))
	}
}

// AlignWidget fills the space it is given and places one child within it.
// Build one with Align or Center.
type AlignWidget struct {
	x, y  float64
	child Widget

	childSize Size
}

// Align fills the available space and places child in it, centered until At
// or one of Left, Right, Top, Bottom moves it.
func Align(child Widget) *AlignWidget { return &AlignWidget{x: 0.5, y: 0.5, child: child} }

// Center is Align with the child in the middle.
func Center(child Widget) *AlignWidget { return Align(child) }

// At places the child at a fraction of the free space on each axis: (0, 0)
// is the top-left corner, (1, 1) the bottom-right, (0.5, 0.5) the center.
func (a *AlignWidget) At(x, y float64) *AlignWidget { a.x, a.y = x, y; return a }

// Left snaps the child to the left edge.
func (a *AlignWidget) Left() *AlignWidget { a.x = 0; return a }

// Right snaps the child to the right edge.
func (a *AlignWidget) Right() *AlignWidget { a.x = 1; return a }

// Top snaps the child to the top edge.
func (a *AlignWidget) Top() *AlignWidget { a.y = 0; return a }

// Bottom snaps the child to the bottom edge.
func (a *AlignWidget) Bottom() *AlignWidget { a.y = 1; return a }

// Layout implements Widget.
func (a *AlignWidget) Layout(c Constraints) Size {
	a.childSize = a.child.Layout(Loose(c.Max()))
	return c.Max()
}

// Paint implements Widget.
func (a *AlignWidget) Paint(dst *ebiten.Image, r Rect) {
	a.child.Paint(dst, Rct(
		r.Origin.Add(Pt((r.Size.W-a.childSize.W)*a.x, (r.Size.H-a.childSize.H)*a.y)),
		a.childSize,
	))
}

// ListWidget is a Column driven by data: one child per item, built by the
// item function. Children are rebuilt each layout pass, so the list stays in
// step with the slice it was given. Build one with List.
type ListWidget[T any] struct {
	items []T
	item  func(T) Widget

	col ColumnWidget
}

// List builds one child per item and stacks them like a Column.
func List[T any](items []T, item func(T) Widget) *ListWidget[T] {
	return &ListWidget[T]{items: items, item: item}
}

// Gap sets the space between consecutive items.
func (l *ListWidget[T]) Gap(v float64) *ListWidget[T] { l.col.gap = v; return l }

// Layout implements Widget.
func (l *ListWidget[T]) Layout(c Constraints) Size {
	l.col.children = Children(l.items, l.item)
	return l.col.Layout(c)
}

// Paint implements Widget.
func (l *ListWidget[T]) Paint(dst *ebiten.Image, r Rect) { l.col.Paint(dst, r) }
