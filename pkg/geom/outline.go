package geom

import "github.com/ironpark/ggui/pkg/layout"

// Outline is a path as its points: subpaths of lines and quadratic and
// cubic Bézier curves, as a vector renderer is given them. It answers what
// a renderer does not say: whether a point is inside and what it covers.
// The zero value is empty.
type Outline struct {
	ops []outlineOp

	// flat is the outline flattened into polygons, until it changes.
	flat  [][]Point
	stale bool
}

// outlineOp is one step of an Outline as it was given.
type outlineOp struct {
	kind opKind
	pts  [3]Point
}

type opKind uint8

const (
	opMove opKind = iota
	opLine
	opQuad
	opCubic
	opClose
)

func (o *Outline) add(kind opKind, pts ...Point) {
	op := outlineOp{kind: kind}
	copy(op.pts[:], pts)
	o.ops = append(o.ops, op)
	o.stale = true
}

// MoveTo starts a subpath at p.
func (o *Outline) MoveTo(p Point) { o.add(opMove, p) }

// LineTo adds a line to p.
func (o *Outline) LineTo(p Point) { o.add(opLine, p) }

// QuadTo adds a quadratic Bézier curve through control point c to p.
func (o *Outline) QuadTo(c, p Point) { o.add(opQuad, c, p) }

// CubicTo adds a cubic Bézier curve through control points c1 and c2 to p.
func (o *Outline) CubicTo(c1, c2, p Point) { o.add(opCubic, c1, c2, p) }

// Close closes the subpath, back to where it started.
func (o *Outline) Close() { o.add(opClose) }

// Reset empties the outline, keeping its storage.
func (o *Outline) Reset() {
	o.ops = o.ops[:0]
	o.stale = true
}

// CurveSteps is how many lines a curve is flattened into.
const CurveSteps = 16

// Polygons returns the outline flattened into closed polygons, rebuilt only
// after a change. A rebuild makes new slices, so ones handed out before stay
// as they were: a hit region tests against the outline it was painted with.
func (o *Outline) Polygons() [][]Point {
	if !o.stale && o.flat != nil {
		return o.flat
	}
	var out [][]Point
	var cur []Point
	var start, at Point
	flush := func() {
		if len(cur) > 1 {
			out = append(out, cur)
		}
		cur = nil
	}
	begin := func() {
		if cur == nil {
			cur = []Point{at}
		}
	}
	for _, op := range o.ops {
		switch op.kind {
		case opMove:
			flush()
			start, at = op.pts[0], op.pts[0]
			cur = []Point{at}
		case opLine:
			begin()
			at = op.pts[0]
			cur = append(cur, at)
		case opQuad, opCubic:
			begin()
			p0, c1, c2, end := at, op.pts[0], op.pts[1], op.pts[2]
			if op.kind == opQuad {
				// A quadratic is the cubic whose handles are two thirds
				// of the way from its ends to its control point.
				end = op.pts[1]
				c1 = layout.Pt(p0.X+2*(c1.X-p0.X)/3, p0.Y+2*(c1.Y-p0.Y)/3)
				c2 = layout.Pt(end.X+2*(op.pts[0].X-end.X)/3, end.Y+2*(op.pts[0].Y-end.Y)/3)
			}
			for i := 1; i <= CurveSteps; i++ {
				t := float64(i) / CurveSteps
				u := 1 - t
				a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
				cur = append(cur, layout.Pt(a*p0.X+b*c1.X+c*c2.X+d*end.X, a*p0.Y+b*c1.Y+c*c2.Y+d*end.Y))
			}
			at = end
		case opClose:
			flush()
			at = start
		}
	}
	flush()
	if out == nil {
		out = [][]Point{}
	}
	o.flat, o.stale = out, false
	return out
}

// Contains reports whether pt is inside the outline by the nonzero rule,
// with every subpath closed. Curves are followed to within a sixteenth of
// their length.
func (o *Outline) Contains(pt Point) bool { return PolygonsContain(o.Polygons(), pt) }

// PolygonsContain is Contains for an outline already flattened.
func PolygonsContain(polys [][]Point, pt Point) bool {
	winding := 0
	for _, poly := range polys {
		for i, a := range poly {
			b := poly[(i+1)%len(poly)]
			// The winding number counts the edges crossing the ray to the
			// right of pt, upwards as one and downwards as minus one.
			side := (b.X-a.X)*(pt.Y-a.Y) - (pt.X-a.X)*(b.Y-a.Y)
			switch {
			case a.Y <= pt.Y && b.Y > pt.Y && side > 0:
				winding++
			case a.Y > pt.Y && b.Y <= pt.Y && side < 0:
				winding--
			}
		}
	}
	return winding != 0
}

// Bounds returns the smallest Rect holding the outline, or the zero Rect
// for an empty one.
func (o *Outline) Bounds() Rect {
	return PointBounds(func(yield func(Point) bool) {
		for _, poly := range o.Polygons() {
			for _, q := range poly {
				if !yield(q) {
					return
				}
			}
		}
	})
}
