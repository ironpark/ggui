// Package property implements borrowed widget properties and their measurement
// revisions. It has no dependency on ggui, so core and themed widgets share it.
package property

import (
	"image/color"
	"math"
	"reflect"

	"github.com/ironpark/ggui/internal/reactive"
)

// Owner is the revision of a widget's explicit configuration. It is a
// layout source, just like a signal's version: a read records a layout
// dependency and a change schedules measurement, and neither creates a
// subscription, which keeps configuration free of owning effects.
type Owner struct {
	version uint64
	mounted bool
}

func (o *Owner) Version() uint64 { return o.version }

// LayoutVersion implements reactive.LayoutSource.
func (o *Owner) LayoutVersion() uint64 { return o.version }
func (o *Owner) Read() {
	o.mounted = true
	reactive.Record(o, o.version)
}
func (o *Owner) Changed() {
	o.version++
	if o.mounted && !reactive.InLayout() {
		reactive.RequestLayout()
	}
}

// Layout records the final revision as well as the initial one: a custom parent
// may configure a child several times before measuring it. Such writes dirty
// cached measurements through their revision without scheduling another frame.
func (o *Owner) Layout() func() {
	done := EnterLayout()
	o.Read()
	return func() { o.Read(); done() }
}
func EnterLayout() func() { return reactive.EnterLayout() }

// Equal compares configuration values, not reader identity. Colors compare by
// their rendered channels; equal NaNs must not turn a still frame into a loop.
func Equal[T any](a, b T) bool {
	if ca, ok := any(a).(color.Color); ok {
		if cb, ok := any(b).(color.Color); ok {
			ar, ag, ab, aa := ca.RGBA()
			br, bg, bb, ba := cb.RGBA()
			return ar == br && ag == bg && ab == bb && aa == ba
		}
	}
	switch av := any(a).(type) {
	case float64:
		bv, ok := any(b).(float64)
		return ok && (av == bv || math.IsNaN(av) && math.IsNaN(bv))
	case float32:
		bv, ok := any(b).(float32)
		return ok && (av == bv || math.IsNaN(float64(av)) && math.IsNaN(float64(bv)))
	}
	if Same(a, b) {
		return true
	}
	return reflect.DeepEqual(a, b)
}

// Watch checks a value setter's final result, including compound assignments.
// Call it with defer before changing the field. Bound properties use Value.
func Watch[T any](o *Owner, p *T) func() {
	before := *p
	return func() {
		if !Equal(before, *p) {
			o.Changed()
		}
	}
}

type Reader[T any] interface{ Get() T }

func Require(v any, name string) {
	if v == nil {
		panic("ggui: " + name + " requires a non-nil reader")
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if r.IsNil() {
			panic("ggui: " + name + " requires a non-nil reader")
		}
	}
}
func Same(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return reflect.ValueOf(a).Comparable() && reflect.ValueOf(b).Comparable() && a == b
}

// Value keeps a literal or a borrowed reader. Resolution participates in the
// caller's tracking scope and never publishes a widget-configuration change.
type Value[T any] struct {
	value  T
	reader Reader[T]
}

func (p *Value[T]) Get() T {
	if p.reader != nil {
		p.value = p.reader.Get()
	}
	return p.value
}
func (p *Value[T]) Current() T  { return p.value }
func (p *Value[T]) Bound() bool { return p.reader != nil }
func (p *Value[T]) Set(v T) bool {
	changed := p.reader != nil || !Equal(p.value, v)
	p.reader = nil
	p.value = v
	return changed
}
func (p *Value[T]) Bind(r Reader[T], name string) bool {
	Require(r, name)
	if Same(p.reader, r) {
		return false
	}
	p.reader = r
	return true
}
