// Package gem holds the small generic helpers ggui's packages share and
// an app built on them may as well: choosing a value inline, keeping one
// within bounds, reading an unbounded constraint, comparing two values of
// any type without the panic == can raise, and a typed sync.Pool.
// A helper earns a place only when it is too small to belong to any one
// package and more than one wants it.
package gem

import (
	"cmp"
	"math"
	"reflect"
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

// Same reports whether a and b are equal as == has it, and false where ==
// would panic: two slices, maps or funcs, or structs or arrays holding one,
// even inside an interface. It is how handlers, keys and styles held as
// any are compared, as a widget's identity is. Pointers, strings and
// numbers, which those mostly are, compare as they are; only another kind
// pays for the recover that guards a comparison which panics.
func Same(a, b any) bool {
	t := reflect.TypeOf(a)
	if t != reflect.TypeOf(b) {
		return false
	}
	if t == nil {
		return true
	}
	switch t.Kind() {
	case reflect.Pointer, reflect.UnsafePointer, reflect.Chan, reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return a == b
	}
	return sameGuarded(a, b)
}

// sameGuarded is a == b, false where the comparison panics.
func sameGuarded(a, b any) (same bool) {
	defer func() {
		if recover() != nil {
			same = false
		}
	}()
	return a == b
}
