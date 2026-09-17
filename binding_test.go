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
