// Package layout computes constrained box layouts without widgets, rendering,
// signals, fonts or a frame loop. Callers supply child measurements and consume
// parent-relative rectangles. Geometry is expressed in logical pixels.
package layout

import (
	"math"
	"slices"
)

// Axis is the main axis of a flow.
type Axis uint8

const (
	Vertical Axis = iota
	Horizontal
)

// Main returns the extent of s along a.
func (a Axis) Main(s Size) float64 {
	if a == Horizontal {
		return s.W
	}
	return s.H
}

// Cross returns the extent of s across a.
func (a Axis) Cross(s Size) float64 {
	if a == Horizontal {
		return s.H
	}
	return s.W
}

// Size returns the Size with main along a and cross across it.
func (a Axis) Size(main, cross float64) Size {
	if a == Horizontal {
		return Size{W: main, H: cross}
	}
	return Size{W: cross, H: main}
}

// Point returns the Point at main along a and cross across it.
func (a Axis) Point(main, cross float64) Point {
	if a == Horizontal {
		return Point{X: main, Y: cross}
	}
	return Point{X: cross, Y: main}
}

// Constraints returns bounds given along and across a.
func (a Axis) Constraints(mainMin, mainMax, crossMin, crossMax float64) Constraints {
	if a == Horizontal {
		return Constraints{MinW: mainMin, MaxW: mainMax, MinH: crossMin, MaxH: crossMax}
	}
	return Constraints{MinH: mainMin, MaxH: mainMax, MinW: crossMin, MaxW: crossMax}
}

// Direction is a flow's reading direction. It mirrors horizontal positions.
type Direction uint8

const (
	LTR Direction = iota
	RTL
)

// String returns "ltr" or "rtl".
func (d Direction) String() string {
	if d == RTL {
		return "rtl"
	}
	return "ltr"
}

// Item describes a child's allocation in a flow. A positive Weight shares
// remaining bounded space; Loose permits the child to use less than its share.
// Weighted children reserve a slot and must not report Absent on a bounded axis.
// On an unbounded main axis, weighted children are measured like rigid children.
type Item struct {
	Weight float64
	Loose  bool
}

// Measurement is a child's response to constraints. Baseline is measured from
// its top. Absent removes its adjacent gap; a zero-sized present child keeps it.
// An absent child should return a zero Size and no baseline.
type Measurement struct {
	Size        Size
	Baseline    float64
	HasBaseline bool
	Absent      bool
}

// MeasureFunc measures child index under c. It must return a size satisfying c,
// except that absent children may return zero. Flow, Wrap, Stack, Box and Align
// measure each child once; Grid measures twice when its width is unbounded.
// Measurement may update caller-owned state, but must not mutate the active
// layout's inputs or reenter the same Scratch.
type MeasureFunc func(index int, c Constraints) Measurement

// Result describes the container and its children, in input order. Children
// contains parent-relative rectangles, including entries for absent children.
// Its slice is borrowed from Scratch until the next call using that Scratch.
// Copy the slice if it must outlive that call. Scalar fields remain valid.
// Baseline is the first present child baseline translated by its position.
type Result struct {
	Size        Size
	Baseline    float64
	HasBaseline bool
	Children    []Rect
}

// Scratch reuses working storage. Its zero value is ready to use. Use one per
// container for recursive layout; it is not safe for concurrent or reentrant use.
// Do not copy a Scratch after first use. Child counts must be nonnegative.
// It does not cache measurements or retain callbacks or caller objects.
type Scratch struct {
	measured []Measurement
	rects    []Rect
}

func (s *Scratch) reset(n int) {
	// Every algorithm writes each measurement; rects are cleared because
	// some fill only part of one.
	s.measured = slices.Grow(s.measured[:0], n)[:n]
	s.rects = slices.Grow(s.rects[:0], n)[:n]
	clear(s.rects)
}

func (s *Scratch) result(size Size) Result {
	r := Result{Size: size, Children: s.rects}
	for i, m := range s.measured {
		if !m.Absent && m.HasBaseline {
			r.Baseline, r.HasBaseline = m.Baseline+s.rects[i].Origin.Y, true
			break
		}
	}
	return r
}
func bounded(v, fallback float64) float64 {
	if math.IsInf(v, 1) {
		return fallback
	}
	return v
}
