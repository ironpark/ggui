package layout

import (
	"math"
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
func (s *Scratch) Flow(c Constraints, f FlowStyle, items []Item, measure MeasureFunc) Result {
	n := len(items)
	s.reset(n)
	mainMax, crossMax := f.main(c.Max()), f.cross(c.Max())
	crossMin := f.stretched(crossMax, 0)
	gaps := f.Gap * float64(max(n-1, 0))
	used, totalFlex, present := gaps, 0.0, n
	flexible := !math.IsInf(mainMax, 1)
	for i, item := range items {
		if item.Weight > 0 && flexible {
			totalFlex += item.Weight
			continue
		}
		m := measure(i, f.constraints(0, max(mainMax-used, 0), crossMin, crossMax))
		s.measured[i] = m
		used += f.main(m.Size)
		if m.Absent {
			present--
		}
	}
	if present < n {
		settled := f.Gap * float64(max(present-1, 0))
		used += settled - gaps
		gaps = settled
	}
	free := max(mainMax-used, 0)
	for i, item := range items {
		if item.Weight > 0 && flexible {
			extent := free * item.Weight / totalFlex
			minimum := extent
			if item.Loose {
				minimum = 0
			}
			s.measured[i] = measure(i, f.constraints(minimum, extent, crossMin, crossMax))
		}
	}
	content, crossUsed := gaps, 0.0
	for _, m := range s.measured {
		content += f.main(m.Size)
		crossUsed = max(crossUsed, f.cross(m.Size))
	}
	baseline := f.Axis == Horizontal && f.Align == AlignBaseline
	above, below := 0.0, 0.0
	if baseline {
		for _, m := range s.measured {
			b := baselineOrBottom(m)
			if !m.Absent {
				above = max(above, b)
				below = max(below, m.Size.H-b)
			}
		}
		crossUsed = above + below
	}
	mainTotal := content
	if totalFlex > 0 || f.Justify != JustifyStart {
		mainTotal = bounded(mainMax, content)
	}
	size := c.Constrain(f.size(mainTotal, f.stretched(crossMax, crossUsed)))
	lead, between := 0.0, f.Gap
	if slack := max(f.main(size)-content, 0); present > 0 {
		switch f.Justify {
		case JustifyCenter:
			lead = slack / 2
		case JustifyEnd:
			lead = slack
		case SpaceBetween:
			if present > 1 {
				between += slack / float64(present-1)
			}
		case SpaceAround:
			lead = slack / float64(present) / 2
			between += slack / float64(present)
		case SpaceEvenly:
			lead = slack / float64(present+1)
			between += slack / float64(present+1)
		}
	}
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
