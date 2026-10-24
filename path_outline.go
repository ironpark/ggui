package ggui

// pathOp is one step of a Path as it was given, kept beside the vector path
// so that a Path can answer where it is: ggfx's own path does not say.
type pathOp struct {
	kind pathKind
	pts  [3]Point
}

type pathKind uint8

const (
	pathMove pathKind = iota
	pathLine
	pathQuad
	pathCubic
	pathClose
)

// record appends a step and invalidates every cache.
func (p *Path) record(kind pathKind, pts ...Point) {
	op := pathOp{kind: kind}
	copy(op.pts[:], pts)
	p.outline = append(p.outline, op)
	p.revision++
}

// curveSteps is how many lines a curve is flattened into for hit testing.
const curveSteps = 16

// polygons returns the path flattened into closed polygons, rebuilt only
// after a change. A rebuild makes new slices, so one handed out before stays
// as it was: a hit region tests against the path it was painted with.
func (p *Path) polygons() [][]Point {
	if p.flatRev == p.revision && p.flat != nil {
		return p.flat
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
	for _, op := range p.outline {
		switch op.kind {
		case pathMove:
			flush()
			start, at = op.pts[0], op.pts[0]
			cur = []Point{at}
		case pathLine:
			begin()
			at = op.pts[0]
			cur = append(cur, at)
		case pathQuad, pathCubic:
			begin()
			p0, c1, c2, end := at, op.pts[0], op.pts[1], op.pts[2]
			if op.kind == pathQuad {
				// A quadratic is the cubic whose handles are two thirds
				// of the way from its ends to its control point.
				end = op.pts[1]
				c1 = Pt(p0.X+2*(c1.X-p0.X)/3, p0.Y+2*(c1.Y-p0.Y)/3)
				c2 = Pt(end.X+2*(op.pts[0].X-end.X)/3, end.Y+2*(op.pts[0].Y-end.Y)/3)
			}
			for i := 1; i <= curveSteps; i++ {
				t := float64(i) / curveSteps
				u := 1 - t
				a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
				cur = append(cur, Pt(a*p0.X+b*c1.X+c*c2.X+d*end.X, a*p0.Y+b*c1.Y+c*c2.Y+d*end.Y))
			}
			at = end
		case pathClose:
			flush()
			at = start
		}
	}
	flush()
	if out == nil {
		out = [][]Point{}
	}
	p.flat, p.flatRev = out, p.revision
	return out
}

// Contains reports whether pt is inside the path as FillPath fills it: by
// the nonzero rule, with every subpath closed. Curves are followed to within
// a sixteenth of their length.
func (p *Path) Contains(pt Point) bool {
	if p == nil {
		return false
	}
	return polygonsContain(p.polygons(), pt)
}

// polygonsContain is Contains for a path already flattened.
func polygonsContain(polys [][]Point, pt Point) bool {
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

// Bounds returns the smallest Rect holding the path's outline, or the zero
// Rect for an empty path.
func (p *Path) Bounds() Rect {
	if p == nil {
		return Rect{}
	}
	return pointBounds(func(yield func(Point) bool) {
		for _, poly := range p.polygons() {
			for _, q := range poly {
				if !yield(q) {
					return
				}
			}
		}
	})
}

// ovalPath is the ellipse inscribed in r, as four cubic arcs.
func ovalPath(r Rect) *Path {
	const k = 0.5522847498307936 // 4/3·(√2−1), a quarter circle's handle
	cx, cy := r.Origin.X+r.Size.W/2, r.Origin.Y+r.Size.H/2
	rx, ry := r.Size.W/2, r.Size.H/2
	p := &Path{}
	p.MoveTo(cx+rx, cy)
	p.CubicTo(cx+rx, cy+k*ry, cx+k*rx, cy+ry, cx, cy+ry)
	p.CubicTo(cx-k*rx, cy+ry, cx-rx, cy+k*ry, cx-rx, cy)
	p.CubicTo(cx-rx, cy-k*ry, cx-k*rx, cy-ry, cx, cy-ry)
	p.CubicTo(cx+k*rx, cy-ry, cx+rx, cy-k*ry, cx+rx, cy)
	p.Close()
	return p
}
