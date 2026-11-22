package layout

// Constraints bound the size a child may choose. Bounds are nonnegative;
// positive infinity is permitted for maximums. Minimums win if bounds conflict.
type Constraints struct {
	MinW, MinH float64
	MaxW, MaxH float64
}

// Tight returns constraints that permit exactly one size.
func Tight(s Size) Constraints {
	return Constraints{MinW: s.W, MinH: s.H, MaxW: s.W, MaxH: s.H}
}

// Loose returns constraints that permit anything up to s.
func Loose(s Size) Constraints {
	return Constraints{MaxW: s.W, MaxH: s.H}
}

// Max returns the largest size c permits.
func (c Constraints) Max() Size { return Size{W: c.MaxW, H: c.MaxH} }

// Loosen returns c with its minimums dropped: anything up to c.Max() passes.
func (c Constraints) Loosen() Constraints { return Loose(c.Max()) }

// Constrain clamps s so that it satisfies c.
func (c Constraints) Constrain(s Size) Size {
	return Size{
		W: clamp(s.W, c.MinW, c.MaxW),
		H: clamp(s.H, c.MinH, c.MaxH),
	}
}

func clamp(v, lo, hi float64) float64 {
	if v > hi {
		v = hi
	}
	if v < lo {
		v = lo
	}
	return v
}
