// Package property is what a widget's setters are written with: the X and
// BindX pairs docs/api-conventions.md describes. ggui's own widget and ui
// packages are written with it, and a widget in another module that wants
// setters behaving as theirs do uses it the same way.
//
// A widget keeps one [Owner], the revision of its configuration, reads it at
// the start of Layout, and changes it from its setters:
//
//	type Gauge struct {
//		props property.Owner
//		width float64                 // literal only
//		level property.Value[float64] // literal or reader
//	}
//
//	func (g *Gauge) Width(w float64) *Gauge {
//		defer property.Watch(&g.props, &g.width)()
//		g.width = w
//		return g
//	}
//
//	func (g *Gauge) Level(v float64) *Gauge { property.Set(&g.props, &g.level, v); return g }
//
//	func (g *Gauge) BindLevel(r ggui.Readable[float64]) *Gauge {
//		property.Bind(&g.props, &g.level, r, "Gauge.BindLevel")
//		return g
//	}
//
//	func (g *Gauge) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
//		defer g.props.Layout().EndLayout()
//		level := g.level.Get() // read in Layout: a change measures again
//		...
//	}
//
// A setter that changes a value asks for a layout and makes every cached
// measurement that read the Owner stale; one that assigns an equal value
// does nothing. No effect or subscription is created, so a widget needs no
// owner and nothing to dispose. A reader is borrowed: Value.Get reads it
// where it is called, and it is the caller's tracking, Layout's as a rule,
// that follows it.
//
// The package does not import ggui, so the kernel uses it as well.
package property

import (
	"image/color"
	"math"
	"reflect"
	"sync"

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

// Version is the number of changes made so far.
func (o *Owner) Version() uint64 { return o.version }

// LayoutVersion implements reactive.LayoutSource.
func (o *Owner) LayoutVersion() uint64 { return o.version }

// Read records, in the layout running now, that it depends on the widget's
// configuration. Layout and EndLayout call it; a widget calls it itself only
// when it enters layout some other way.
func (o *Owner) Read() {
	o.mounted = true
	reactive.Record(o, o.version)
}

// Changed marks the configuration changed: measurements that read it are
// stale, and a layout is asked for unless one is running or the widget has
// never been laid out. Watch, Set and Bind call it; a setter calls it itself
// for a change they cannot see, such as a replaced callback.
func (o *Owner) Changed() {
	o.version++
	if o.mounted && !reactive.InLayout() {
		reactive.RequestLayout()
	}
}

// Layout enters layout and records the initial revision; defer EndLayout on
// its result to record the final one too: a custom parent may configure a
// child several times before measuring it. Such writes dirty cached
// measurements through their revision without scheduling another frame.
//
//	defer w.props.Layout().EndLayout()
//
// It returns o rather than a closure so that it allocates nothing.
func (o *Owner) Layout() *Owner {
	reactive.EnterLayout()
	o.Read()
	return o
}

// EndLayout records the final revision and leaves the layout Layout entered.
func (o *Owner) EndLayout() {
	o.Read()
	reactive.LeaveLayout()
}

// Equal compares configuration values, not reader identity. Colors compare by
// their rendered channels; equal NaNs must not turn a still frame into a loop.
func Equal[T any](a, b T) bool {
	t := reflect.TypeFor[T]()
	if t.Kind() != reflect.Float64 && t.Kind() != reflect.Float32 && plain(t) {
		// == is DeepEqual for these, and the values, boxed only to be
		// compared, stay on the stack. A float alone is below, where a NaN
		// is the same as another.
		return any(a) == any(b)
	}
	// A type that registered what equal is, as a style holding a color
	// does, compares with no boxing or reflection.
	if eq, ok := equals.Load(t); ok {
		return eq.(func(a, b T) bool)(a, b)
	}
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

// equals are the comparisons RegisterEqual registered, by type.
var equals sync.Map // reflect.Type → func(a, b T) bool

// RegisterEqual makes Equal compare values of T with eq, for a type a
// property holds that is not plain but can say cheaply what equal is: a
// struct holding a color.Color, which Equal would otherwise box and
// compare by reflection on every layout.
func RegisterEqual[T any](eq func(a, b T) bool) {
	equals.Store(reflect.TypeFor[T](), eq)
}

// plainTypes caches plain by type: Equal runs for every property a layout
// sets, and Same's Comparable allocates as it walks a struct's fields.
var plainTypes sync.Map // reflect.Type → bool

var colorType = reflect.TypeFor[color.Color]()

// plain reports whether t's values are made of booleans, numbers and
// strings alone, in arrays and structs: values == compares as DeepEqual
// does, never panicking and never following a reference. A color is not,
// being compared by what it draws.
func plain(t reflect.Type) bool {
	if v, ok := plainTypes.Load(t); ok {
		return v.(bool)
	}
	var is bool
	switch t.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128, reflect.String:
		is = true
	case reflect.Array:
		is = plain(t.Elem())
	case reflect.Struct:
		is = true
		for field := range t.Fields() {
			if !plain(field.Type) {
				is = false
				break
			}
		}
	}
	if t.Implements(colorType) {
		is = false
	}
	plainTypes.Store(t, is)
	return is
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

// Reader is what a property follows: a ggui.Readable, or anything else with
// a Get. One whose Get reads no signal is not followed by itself; see
// ggui.Invalidate.
type Reader[T any] interface{ Get() T }

// Require panics when v, a reader or a callback a setter named name was
// given, is nil, a typed nil included.
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

// Same reports whether a and b are the same reader or callback: both nil,
// or comparable and equal. Two that cannot be compared, as functions cannot,
// are never the same.
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

// Get returns the reader's value now, or the literal. Call it in Layout or
// Paint, where the read is followed.
func (p *Value[T]) Get() T {
	if p.reader != nil {
		p.value = p.reader.Get()
	}
	return p.value
}

// Current returns the literal, or what Get last read from the reader,
// without reading it again.
func (p *Value[T]) Current() T { return p.value }

// Bound reports whether p follows a reader.
func (p *Value[T]) Bound() bool { return p.reader != nil }

// Set gives p the literal v, dropping any reader, and reports whether that
// changed it. A widget's setter calls the package's Set, which tells the
// Owner as well.
func (p *Value[T]) Set(v T) bool {
	changed := p.reader != nil || !Equal(p.value, v)
	p.reader = nil
	p.value = v
	return changed
}

// Bind has p follow r and reports whether r is a new reader. It panics,
// naming the setter name, when r is nil.
func (p *Value[T]) Bind(r Reader[T], name string) bool {
	Require(r, name)
	if Same(p.reader, r) {
		return false
	}
	p.reader = r
	return true
}

// Set gives p the literal v and, if that changed it, o: a widget's setter.
func Set[T any](o *Owner, p *Value[T], v T) bool {
	changed := p.Set(v)
	if changed {
		o.Changed()
	}
	return changed
}

// Bind has p follow r and, if that is a new reader, changes o: a widget's
// Bind setter. name is the setter's, for the panic on a nil reader.
func Bind[T any](o *Owner, p *Value[T], r Reader[T], name string) bool {
	changed := p.Bind(r, name)
	if changed {
		o.Changed()
	}
	return changed
}
