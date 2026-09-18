package ggui

import (
	"testing"
	"time"
)

type form struct {
	Name string
	Age  int
}

func TestLensReadsAndWritesThrough(t *testing.T) {
	f := State(form{Name: "a", Age: 1})
	name := f.Lens(func(v form) string { return v.Name }, func(v form, s string) form { v.Name = s; return v })
	var b Binding[string] = name
	runs := 0
	dispose := Effect(func() { runs++; b.Get() })
	defer dispose()
	b.Set("bb")
	effects.flush()
	if got := f.Peek(); got.Name != "bb" || got.Age != 1 {
		t.Fatalf("whole = %+v", got)
	}
	if runs != 2 || b.Peek() != "bb" {
		t.Fatalf("runs = %d, peek = %q", runs, b.Peek())
	}
}

func TestTweenIsABinding(t *testing.T) {
	var b Binding[float64] = Tween(0.0, time.Millisecond)
	b.Set(1)
	if b.Peek() != 0 {
		t.Fatalf("tween jumped: %v", b.Peek())
	}
}

func TestForRetainEvictsOffscreenRows(t *testing.T) {
	var items []int
	for i := range 100 {
		items = append(items, i)
	}
	src := State(items)
	var f *ForWidget[int, int]
	off := State(0.0)
	dispose := Effect(func() {
		f = For(src, func(i int) int { return i }, func(Reader[int]) Widget { return Box().Size(10, 10) }).ItemExtent(10).Retain(2)
	})
	defer dispose()
	p := NewProbe(Scroll(f).Offset(off), Sz(10, 50))
	p.Frame()
	off.Set(500)
	p.Frame()
	if n := len(f.entries); n != 5+2 {
		t.Fatalf("mounted rows = %d, want 7", n)
	}
}

// independentWritable deliberately does not embed Signal: the helpers must
// depend only on the public contract, including generic argument inference.
type independentWritable[T any] struct {
	value   T
	writes  int
	updates int
}

func (w *independentWritable[T]) Get() T              { return w.value }
func (w *independentWritable[T]) Peek() T             { return w.value }
func (w *independentWritable[T]) Set(v T)             { w.value = v; w.writes++ }
func (w *independentWritable[T]) Update(fn func(T) T) { w.updates++; w.Set(fn(w.value)) }

func TestWritableHelpersOnSignalsLensesAndCustomValues(t *testing.T) {
	state := State(form{Name: "kept", Age: 1})
	age := state.Lens(func(f form) int { return f.Age }, func(f form, v int) form { f.Age = v; return f })
	custom := &independentWritable[int]{value: 1}
	for _, w := range []Writable[int]{State(1), age, custom} {
		Add(w, 2)
		Add(w, 3)
		if w.Peek() != 6 {
			t.Fatalf("accumulated value=%d", w.Peek())
		}
	}
	if state.Peek().Name != "kept" || custom.updates != 2 {
		t.Fatal("update did not use writable contract")
	}
	boolState := State(struct{ On bool }{})
	flag := boolState.Lens(func(v struct{ On bool }) bool { return v.On }, func(v struct{ On bool }, b bool) struct{ On bool } { v.On = b; return v })
	Toggle(flag)
	if !boolState.Peek().On {
		t.Fatal("toggle did not write through lens")
	}
	// Inference also works without an interface-typed intermediate variable.
	Add(custom, 1)
	Toggle(&independentWritable[bool]{})
}

func TestWritableSlicesCopyAndSkipNoopRemoval(t *testing.T) {
	type model struct {
		Items []int
		Other int
	}
	items := make([]int, 2, 8)
	items[0], items[1] = 1, 2
	state := State(model{Items: items, Other: 7})
	lens := state.Lens(func(m model) []int { return m.Items }, func(m model, v []int) model { m.Items = v; return m })
	runs := 0
	dispose := Effect(func() { state.Get(); runs++ })
	defer dispose()
	Remove(lens, func(v int) bool { return v == 9 })
	effects.flush()
	if runs != 1 {
		t.Fatal("no-op remove notified readers")
	}
	Append(lens, 3)
	effects.flush()
	if runs != 2 || items[:3][2] != 0 {
		t.Fatal("append reused backing storage or missed notification")
	}
	Remove(lens, func(v int) bool { return v == 2 })
	effects.flush()
	got := state.Peek()
	if runs != 3 || len(got.Items) != 2 || got.Items[0] != 1 || got.Items[1] != 3 || got.Other != 7 {
		t.Fatalf("result=%+v, runs=%d", got, runs)
	}
	custom := &independentWritable[[]int]{value: []int{1}}
	Remove(custom, func(int) bool { return false })
	if custom.writes != 0 {
		t.Fatal("no-op remove wrote custom value")
	}
	Append(custom, 2)
	if custom.updates != 1 || len(custom.value) != 2 {
		t.Fatal("append bypassed custom Update")
	}
}

func TestAnimatedBindingsAreNotWritable(t *testing.T) {
	for _, b := range []Binding[float64]{Tween(0.0, time.Second), Spring(0.0)} {
		if _, ok := b.(Writable[float64]); ok {
			t.Fatal("animated binding must not support immediate updates")
		}
	}
}

// Field is Lens with one closure instead of two: it reads and writes through
// a pointer into a copy of the whole.
func TestFieldLensReadsAndWritesThrough(t *testing.T) {
	type form struct {
		Name string
		Age  int
	}
	f := State(form{Name: "a", Age: 1})
	name := f.Field(func(v *form) *string { return &v.Name })

	seen := ""
	defer Watch(name, func(s string) { seen = s })()
	if name.Get() != "a" || name.Peek() != "a" {
		t.Fatalf("read %q/%q, want a", name.Get(), name.Peek())
	}

	name.Set("b")
	effects.flush()
	if got := f.Peek(); got.Name != "b" || got.Age != 1 {
		t.Fatalf("writing the part left the whole as %+v", got)
	}
	if seen != "b" {
		t.Fatalf("the watcher saw %q", seen)
	}

	name.Update(func(s string) string { return s + "!" })
	if got := f.Peek().Name; got != "b!" {
		t.Fatalf("Update through the field gave %q", got)
	}

	// Writing the whole is seen through the field.
	f.Set(form{Name: "c", Age: 2})
	effects.flush()
	if name.Peek() != "c" || seen != "c" {
		t.Fatalf("peek %q, watcher %q after the whole was replaced", name.Peek(), seen)
	}
}
