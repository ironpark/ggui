package layout

import (
	"math"
)

// WrapStyle lays children left to right, starting another line at MaxW.
// Align places children vertically within a line; baseline and stretch are
// treated as start. Gap separates children, RunGap separates lines.
type WrapStyle struct {
	Gap, RunGap float64
	Align       CrossAlign
}

// Wrap measures count children once and packs them into lines.
func (s *Scratch) Wrap(c Constraints, style WrapStyle, count int, measure MeasureFunc) Result {
	s.reset(count)
	for i := range count {
		s.measured[i] = measure(i, c.Loosen())
		s.rects[i].Size = s.measured[i].Size
	}
	var x, y, lineH, width float64
	start := 0
	place := func(end int) {
		for i := start; i < end; i++ {
			s.rects[i].Origin.Y = y + (lineH-s.measured[i].Size.H)*style.Align.Fraction()
		}
	}
	for i, m := range s.measured {
		if m.Absent {
			s.rects[i].Origin.X = x
			continue
		}
		if i > start && x+m.Size.W > c.MaxW {
			place(i)
			width = max(width, x-style.Gap)
			y += lineH + style.RunGap
			x, lineH, start = 0, 0, i
		}
		s.rects[i].Origin.X = x
		x += m.Size.W + style.Gap
		lineH = max(lineH, m.Size.H)
	}
	place(count)
	if count > 0 {
		width = max(width, x-style.Gap)
		y += lineH
	}
	return s.result(c.Constrain(Size{W: width, H: y}))
}

// GridStyle makes equal-width columns. Rows take their tallest child's height.
// Columns below one are treated as one. Grid reserves a cell even for an absent
// child. An unbounded width uses the widest child's natural width for each cell.
type GridStyle struct {
	Columns     int
	Gap, RowGap float64
}

// GridResult is a grid's Result and the width given to each of its cells.
type GridResult struct {
	Result
	CellWidth float64
}

// Grid measures count children into equal-width columns.
func (s *Scratch) Grid(c Constraints, style GridStyle, count int, measure MeasureFunc) GridResult {
	s.reset(count)
	cols := max(style.Columns, 1)
	cellW := 0.0
	inf := math.Inf(1)
	if math.IsInf(c.MaxW, 1) {
		for i := range count {
			m := measure(i, Constraints{MaxW: inf, MaxH: inf})
			cellW = max(cellW, m.Size.W)
		}
	} else {
		cellW = max((c.MaxW-style.Gap*float64(cols-1))/float64(cols), 0)
	}
	cell := Constraints{MinW: cellW, MaxW: cellW, MaxH: inf}
	y := 0.0
	for row := 0; row*cols < count; row++ {
		rowH := 0.0
		for col := 0; col < cols && row*cols+col < count; col++ {
			i := row*cols + col
			m := measure(i, cell)
			s.measured[i] = m
			s.rects[i] = Rect{Origin: Point{X: float64(col) * (cellW + style.Gap), Y: y}, Size: m.Size}
			rowH = max(rowH, m.Size.H)
		}
		y += rowH
		if (row+1)*cols < count {
			y += style.RowGap
		}
	}
	usedCols := min(cols, count)
	width := float64(usedCols)*cellW + style.Gap*float64(max(usedCols-1, 0))
	return GridResult{Result: s.result(c.Constrain(Size{W: width, H: y})), CellWidth: cellW}
}

// Stack overlays children at the origin. Expand fills bounded axes; otherwise
// the container hugs its largest child. The input order is retained.
func (s *Scratch) Stack(c Constraints, expand bool, count int, measure MeasureFunc) Result {
	s.reset(count)
	var size Size
	for i := range count {
		m := measure(i, c.Loosen())
		s.measured[i] = m
		s.rects[i].Size = m.Size
		size.W = max(size.W, m.Size.W)
		size.H = max(size.H, m.Size.H)
	}
	if expand {
		size = Size{W: bounded(c.MaxW, size.W), H: bounded(c.MaxH, size.H)}
	}
	return s.result(c.Constrain(size))
}

// SingleResult describes a container with at most one child. It owns all its
// data and needs no Scratch or heap-backed result slice.
type SingleResult struct {
	Size        Size
	Child       Rect
	HasChild    bool
	Baseline    float64
	HasBaseline bool
}

func singleResult(size Size, child Rect, m Measurement) SingleResult {
	return SingleResult{Size: size, Child: child, HasChild: true,
		Baseline: m.Baseline + child.Origin.Y, HasBaseline: m.HasBaseline && !m.Absent}
}

// Align fills bounded axes and positions one child by fractions of free space:
// (0,0) is top-left, (.5,.5) center, (1,1) bottom-right. x and y are physical
// fractions independent of reading direction. Unbounded axes hug the child.
func Align(c Constraints, x, y float64, measure MeasureFunc) SingleResult {
	m := measure(0, c.Loosen())
	size := c.Constrain(Size{W: bounded(c.MaxW, m.Size.W), H: bounded(c.MaxH, m.Size.H)})
	child := Rect{Origin: Point{X: (size.W - m.Size.W) * x, Y: (size.H - m.Size.H) * y}, Size: m.Size}
	return singleResult(size, child, m)
}

// Insets is padding on the four sides of a box.
type Insets struct{ Top, Right, Bottom, Left float64 }

// Horizontal returns the left and right padding together.
func (e Insets) Horizontal() float64 { return e.Left + e.Right }

// Vertical returns the top and bottom padding together.
func (e Insets) Vertical() float64 { return e.Top + e.Bottom }

// Shrink removes padding from the constraint bounds, flooring at zero.
func (e Insets) Shrink(c Constraints) Constraints {
	return Constraints{MinW: max(c.MinW-e.Horizontal(), 0), MinH: max(c.MinH-e.Vertical(), 0), MaxW: max(c.MaxW-e.Horizontal(), 0), MaxH: max(c.MaxH-e.Vertical(), 0)}
}

// Inflate grows size by the padding.
func (e Insets) Inflate(size Size) Size {
	return Size{W: size.W + e.Horizontal(), H: size.H + e.Vertical()}
}

// BoxStyle sets padding and optional fixed dimensions or maximums, including
// padding. Zero dimensions/caps are unset. Parent minimums take precedence.
type BoxStyle struct {
	Padding                            Insets
	Width, Height, MaxWidth, MaxHeight float64
}

// Box measures one child, or no child when measure is nil. Its rectangle starts
// inside the padding. Decoration and clipping are the caller's responsibility.
func Box(c Constraints, style BoxStyle, measure MeasureFunc) SingleResult {
	pad := style.Padding
	width, height := capped(style.Width, style.MaxWidth), capped(style.Height, style.MaxHeight)
	inner := pad.Shrink(c).Loosen()
	if style.MaxWidth > 0 {
		inner.MaxW = min(inner.MaxW, max(style.MaxWidth-pad.Horizontal(), 0))
	}
	if style.MaxHeight > 0 {
		inner.MaxH = min(inner.MaxH, max(style.MaxHeight-pad.Vertical(), 0))
	}
	if width > 0 {
		w := max(clamp(width, c.MinW, c.MaxW)-pad.Horizontal(), 0)
		inner.MinW, inner.MaxW = w, w
	}
	if height > 0 {
		h := max(clamp(height, c.MinH, c.MaxH)-pad.Vertical(), 0)
		inner.MinH, inner.MaxH = h, h
	}
	var m Measurement
	if measure != nil {
		m = measure(0, inner)
	}
	want := pad.Inflate(m.Size)
	if width > 0 {
		want.W = width
	}
	if height > 0 {
		want.H = height
	}
	size := c.Constrain(want)
	if measure == nil {
		return SingleResult{Size: size}
	}
	return singleResult(size, Rect{Origin: Point{X: pad.Left, Y: pad.Top}, Size: m.Size}, m)
}
func capped(v, limit float64) float64 {
	if v > 0 && limit > 0 {
		return min(v, limit)
	}
	return v
}
