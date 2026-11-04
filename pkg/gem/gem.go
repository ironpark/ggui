// Package gem holds the small generic helpers ggui's packages share and
// an app built on them may as well: choosing a value inline, keeping one
// within bounds, reading an unbounded constraint, and a typed sync.Pool.
// A helper earns a place only when it is too small to belong to any one
// package and more than one wants it.
package gem

import (
	"cmp"
	"math"
)

// Pick returns a when cond holds, else b. It is the expression form of an
// if/else, for a value chosen inline in a builder chain.
func Pick[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

// Clamp keeps v within [lo, hi]. lo wins when the bounds contradict, so a
// caller that computed an empty range gets the low end rather than a value
// that depends on which comparison ran first.
func Clamp[T cmp.Ordered](v, lo, hi T) T {
	if v > hi {
		v = hi
	}
	if v < lo {
		v = lo
	}
	return v
}

// Bounded returns v, or fallback when v is an unbounded (+Inf) constraint.
func Bounded(v, fallback float64) float64 {
	if math.IsInf(v, 1) {
		return fallback
	}
	return v
}
