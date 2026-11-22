package layout

import (
	"math"
	"slices"
)

// Justify distributes slack along the main axis. Values other than JustifyStart
// make a flow fill its bounded main axis.
type Justify int

const (
	JustifyStart Justify = iota
	JustifyCenter
	JustifyEnd
	SpaceBetween
	SpaceAround
	SpaceEvenly
)

// CrossAlign positions children across the main axis.
type CrossAlign int

const (
	AlignStart CrossAlign = iota
	AlignCenter
	AlignEnd
	// AlignStretch gives every child the flow's cross size. When that is
	// bounded each child is measured at it; when it is not, the flow is as
	// big as its biggest child and the smaller ones are measured again at
	// that size, as CSS stretches the items of a flex line with an auto
	// cross size.
	AlignStretch
	AlignBaseline // horizontal flows only; vertical flows treat it as AlignStart
)

// FlowStyle configures a Row or Column. Gaps are resolved pixel values.
// The zero value is a vertical, start-aligned flow that hugs its children.
type FlowStyle struct {
	Axis      Axis
	Gap       float64
	Justify   Justify
	Align     CrossAlign
	Direction Direction
}

func (f FlowStyle) main(s Size) float64  { return f.Axis.Main(s) }
func (f FlowStyle) cross(s Size) float64 { return f.Axis.Cross(s) }
func (f FlowStyle) size(main, cross float64) Size {
	return f.Axis.Size(main, cross)
}
func (f FlowStyle) constraints(mainMin, mainMax, crossMin, crossMax float64) Constraints {
	return f.Axis.Constraints(mainMin, mainMax, crossMin, crossMax)
}
func (f FlowStyle) stretched(crossMax, fallback float64) float64 {
	return f.Align.Stretched(crossMax, fallback)
}

// Fraction is where a child sits in the free cross space: 0 at the start,
// .5 centered, 1 at the end. Stretch and baseline report 0.
func (a CrossAlign) Fraction() float64 {
	switch a {
	case AlignCenter:
		return .5
	case AlignEnd:
		return 1
	}
	return 0
}

// Stretched returns a bounded crossMax under AlignStretch, else natural: the
// cross size a stretched flow, or each of its children, takes.
func (a CrossAlign) Stretched(crossMax, natural float64) float64 {
	if a == AlignStretch {
		return bounded(crossMax, natural)
	}
	return natural
}

// baselineOrBottom is where m sits on a shared baseline: its own, or its
// bottom edge when it has none.
func baselineOrBottom(m Measurement) float64 {
	if m.HasBaseline {
		return m.Baseline
	}
	return m.Size.H
}

// Flow measures rigid children in input order against remaining space, then
// weighted children against their shares. It preserves input order in Result.
// It implements ggui's constrained flow semantics, including no flex-shrink.
// Under AlignStretch with an unbounded cross axis, it then measures each
// present child smaller across than the biggest once more, tight across at
// that size and with the main-axis bounds it was first given.
func (s *Scratch) Flow(c Constraints, f FlowStyle, items []Item, measure MeasureFunc) Result {
	n := len(items)
	s.reset(n)
	s.mainBounds = slices.Grow(s.mainBounds[:0], n)[:n]
	b := flowBounds{mainMax: f.main(c.Max()), crossMax: f.cross(c.Max())}
	b.crossMin = f.stretched(b.crossMax, 0)
	space := s.measureRigid(f, items, measure, b)
	s.measureFlexible(f, items, measure, space, b)
	if f.Align == AlignStretch && math.IsInf(b.crossMax, 1) {
		s.stretchUnbounded(f, f.cross(Size{W: c.MinW, H: c.MinH}), measure)
	}
	return s.place(f, c, b, space)
}

// flowBounds are the main- and cross-axis limits Flow measures
// against.
type flowBounds struct{ mainMax, crossMin, crossMax float64 }

// flowSpace is what measuring the rigid children left for placing: the
// free main-axis space the flexible children share, and the gaps and
// count the absent ones left.
type flowSpace struct {
	free, totalFlex float64
	gaps            float64
	present         int
	flexible        bool
}

// measureRigid measures each rigid child against the remaining main
// space, in input order, and returns the space the flexible children
// share.
func (s *Scratch) measureRigid(f FlowStyle, items []Item, measure MeasureFunc, b flowBounds) flowSpace {
	space := flowSpace{gaps: f.Gap * float64(max(len(items)-1, 0)), present: len(items), flexible: !math.IsInf(b.mainMax, 1)}
	used := space.gaps
	for i, item := range items {
		if item.Weight > 0 && space.flexible {
			space.totalFlex += item.Weight
			continue
		}
		s.mainBounds[i] = [2]float64{0, max(b.mainMax-used, 0)}
		m := measure(i, f.constraints(0, s.mainBounds[i][1], b.crossMin, b.crossMax))
		s.measured[i] = m
		used += f.main(m.Size)
		if m.Absent {
			space.present--
			settled := f.Gap * float64(max(space.present-1, 0))
			used += settled - space.gaps
			space.gaps = settled
		}
	}
	space.free = max(b.mainMax-used, 0)
	return space
}

// measureFlexible measures each weighted child against its share of the
// free space.
func (s *Scratch) measureFlexible(f FlowStyle, items []Item, measure MeasureFunc, space flowSpace, b flowBounds) {
	for i, item := range items {
		if item.Weight > 0 && space.flexible {
			extent := space.free * item.Weight / space.totalFlex
			minimum := extent
			if item.Loose {
				minimum = 0
			}
			s.mainBounds[i] = [2]float64{minimum, extent}
			s.measured[i] = measure(i, f.constraints(minimum, extent, b.crossMin, b.crossMax))
		}
	}
}

// place positions the measured children and returns the result.
func (s *Scratch) place(f FlowStyle, c Constraints, b flowBounds, space flowSpace) Result {
	content, crossUsed := s.extent(f, space.gaps)
	baseline := f.Axis == Horizontal && f.Align == AlignBaseline
	above := 0.0
	if baseline {
		var below float64
		above, below = s.baselineExtents()
		crossUsed = above + below
	}
	mainTotal := content
	if space.totalFlex > 0 || f.Justify != JustifyStart {
		mainTotal = bounded(b.mainMax, content)
	}
	size := c.Constrain(f.size(mainTotal, f.stretched(b.crossMax, crossUsed)))
	lead, between := s.leading(f, size, content, space)
	pos := lead
	for i, m := range s.measured {
		crossOff := (f.cross(size) - f.cross(m.Size)) * f.Align.Fraction()
		if baseline {
			crossOff = above - baselineOrBottom(m)
		}
		off := f.Axis.Point(pos, crossOff)
		if f.Direction == RTL {
			off.X = size.W - off.X - m.Size.W
		}
		s.rects[i] = Rect{Origin: off, Size: m.Size}
		if !m.Absent {
			pos += f.main(m.Size) + between
		}
	}
	return s.result(size)
}

// extent returns the content's main-axis length, gaps included, and the
// cross-axis size the children need.
func (s *Scratch) extent(f FlowStyle, gaps float64) (content, crossUsed float64) {
	content = gaps
	for _, m := range s.measured {
		content += f.main(m.Size)
		crossUsed = max(crossUsed, f.cross(m.Size))
	}
	return content, crossUsed
}

// baselineExtents returns how far the shared baseline sits above the
// children's tops and below their bottoms.
func (s *Scratch) baselineExtents() (above, below float64) {
	for _, m := range s.measured {
		b := baselineOrBottom(m)
		if !m.Absent {
			above = max(above, b)
			below = max(below, m.Size.H-b)
		}
	}
	return above, below
}

// leading returns where the first child starts and the gap between
// children, for the justification.
func (s *Scratch) leading(f FlowStyle, size Size, content float64, space flowSpace) (lead, between float64) {
	between = f.Gap
	if slack := max(f.main(size)-content, 0); space.present > 0 {
		switch f.Justify {
		case JustifyCenter:
			lead = slack / 2
		case JustifyEnd:
			lead = slack
		case SpaceBetween:
			if space.present > 1 {
				between += slack / float64(space.present-1)
			}
		case SpaceAround:
			lead = slack / float64(space.present) / 2
			between += slack / float64(space.present)
		case SpaceEvenly:
			lead = slack / float64(space.present+1)
			between += slack / float64(space.present+1)
		}
	}
	return lead, between
}

// stretchUnbounded measures again, tight across at the biggest present
// child's cross size (or floor, when that is bigger), every present child
// that came out smaller, keeping the main-axis bounds it was measured with.
func (s *Scratch) stretchUnbounded(f FlowStyle, floor float64, measure MeasureFunc) {
	cross := floor
	for _, m := range s.measured {
		if !m.Absent {
			cross = max(cross, f.cross(m.Size))
		}
	}
	for i, m := range s.measured {
		if m.Absent || f.cross(m.Size) >= cross {
			continue
		}
		b := s.mainBounds[i]
		s.measured[i] = measure(i, f.constraints(b[0], b[1], cross, cross))
	}
}
