package ggui

import "testing"

// Peek reads a value without subscribing the computation that reads it.
func TestPeekDoesNotSubscribe(t *testing.T) {
	s := State(1)
	d := Map(s, func(v int) int { return v * 2 })
	runs := 0
	dispose := Root(func() {
		Effect(func() Cleanup {
			runs++
			Peek(s)
			Peek(d)
			return nil
		})
	})
	defer dispose()
	p := NewProbe(Box(), Sz(10, 10))
	defer p.Close()
	p.Frame()
	s.Set(2)
	p.Frame()
	if runs != 1 {
		t.Fatalf("the effect ran %d times after a peeked value changed, want 1", runs)
	}
	if Peek(d) != 4 {
		t.Fatalf("Peek gave %d, want the latest value 4", Peek(d))
	}
}

// Not and Or follow the flags they combine.
func TestBooleanCombinators(t *testing.T) {
	a, b := State(false), State(true)
	not, or := Not(a), Or(a, b)
	check := func(wantNot, wantOr bool) {
		t.Helper()
		if Peek(not) != wantNot || Peek(or) != wantOr {
			t.Fatalf("Not %v Or %v; want %v %v", Peek(not), Peek(or), wantNot, wantOr)
		}
	}
	check(true, true)
	a.Set(true)
	check(false, true)
	a.Set(false)
	b.Set(false)
	check(true, false)
	if Peek(Or()) {
		t.Fatal("Or of nothing should be false")
	}
}

// Responsive lays out the arrangement its width calls for, and the two may
// share a widget.
func TestResponsivePicksAnArrangementByWidth(t *testing.T) {
	shared := Box().Size(40, 20)
	wide := Row(shared, Box().Size(40, 20))
	narrow := Column(shared, Box().Size(40, 20))
	r := Responsive(100, wide, narrow)
	if got := r.Layout(Loose(Sz(200, 200)), Env{}); got != Sz(80, 20) {
		t.Fatalf("wide: %v, want the row's 80x20", got)
	}
	if got := r.Layout(Loose(Sz(90, 200)), Env{}); got != Sz(40, 40) {
		t.Fatalf("narrow: %v, want the column's 40x40", got)
	}
}

// Stretch is Align(AlignStretch).
func TestStretchIsAlignStretch(t *testing.T) {
	if Column().Stretch().align != AlignStretch || Row().Stretch().align != AlignStretch {
		t.Fatal("Stretch did not set AlignStretch")
	}
}
