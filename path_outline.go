package ggui

import "math"

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
		case pathQuad:
			begin()
			p0 := at
			for i := 1; i <= curveSteps; i++ {
				t := float64(i) / curveSteps
				u := 1 - t
				cur = append(cur, Pt(u*u*p0.X+2*u*t*op.pts[0].X+t*t*op.pts[1].X, u*u*p0.Y+2*u*t*op.pts[0].Y+t*t*op.pts[1].Y))
			}
			at = op.pts[1]
		case pathCubic:
			begin()
			p0 := at
			for i := 1; i <= curveSteps; i++ {
				t := float64(i) / curveSteps
				u := 1 - t
				a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
				cur = append(cur, Pt(a*p0.X+b*op.pts[0].X+c*op.pts[1].X+d*op.pts[2].X, a*p0.Y+b*op.pts[0].Y+c*op.pts[1].Y+d*op.pts[2].Y))
			}
			at = op.pts[2]
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
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for _, poly := range p.polygons() {
		for _, q := range poly {
			x0, y0, x1, y1 = min(x0, q.X), min(y0, q.Y), max(x1, q.X), max(y1, q.Y)
		}
	}
	if x0 > x1 {
		return Rect{}
	}
	return Rct(Pt(x0, y0), Sz(x1-x0, y1-y0))
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
