// Package fn holds the few tiny helpers that both ggui and ggui/ui need.
// The ui package builds on ggui, so it cannot reach ggui's unexported
// helpers, and each had grown its own copy. The copies had drifted: ui's
// clamp was min(max(v, lo), hi), which returns hi when the bounds
// contradict, while ggui's returned lo. One definition settles it.
//
// Nothing here is part of the public API. A helper earns a place only when
// two packages want it and it is too small to belong to either.
package fn

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
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
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
