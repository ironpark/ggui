package ggui

import (
	"math"
	"testing"
)

func closeTo(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// Under AlignBaseline a large title and a small caption share a baseline,
// and the Row is as tall as the tallest ascent plus the tallest descent.
func TestRowAlignBaseline(t *testing.T) {
	title, caption := Text("Title").Size(28), Text("caption").Size(12)
	row := Row(title, caption).Align(AlignBaseline)
	size := row.Layout(Loose(Sz(400, 400)), Env{})

	tb, ok1 := title.Baseline()
	cb, ok2 := caption.Baseline()
	if !ok1 || !ok2 || tb <= cb {
		t.Fatalf("baselines: title %v %v, caption %v %v", tb, ok1, cb, ok2)
	}
	if !closeTo(row.offsets[0].Y+tb, row.offsets[1].Y+cb) {
		t.Fatalf("baselines differ: %v vs %v", row.offsets[0].Y+tb, row.offsets[1].Y+cb)
	}
	rb, ok := row.Baseline()
	if !ok || !closeTo(rb, tb) {
		t.Fatalf("row baseline = %v %v, want %v", rb, ok, tb)
	}
	below := max(row.sizes[0].H-tb, row.sizes[1].H-cb)
	if !closeTo(size.H, tb+below) {
		t.Fatalf("row height = %v, want %v", size.H, tb+below)
	}
}

// A child with no baseline of its own rests on the shared baseline by its
// bottom edge.
func TestRowAlignBaselineWithoutBaseline(t *testing.T) {
	title, box := Text("Title").Size(28), Box().Size(10, 10)
	row := Row(title, box).Align(AlignBaseline)
	row.Layout(Loose(Sz(400, 400)), Env{})
	tb, _ := title.Baseline()
	if !closeTo(row.offsets[1].Y+10, row.offsets[0].Y+tb) {
		t.Fatalf("box bottom %v, baseline %v", row.offsets[1].Y+10, row.offsets[0].Y+tb)
	}
}

// Containers pass their first child's baseline up, moved by where the
// child went.
func TestBaselinePropagates(t *testing.T) {
	col := Column(Box().Size(10, 5), Padding(Text("x"), 3)).Gap(2)
	col.Layout(Loose(Sz(100, 100)), Env{})
	cb, ok := col.Baseline()
	inner, _ := col.children[1].(Baseliner).Baseline()
	if !ok || !closeTo(cb, 5+2+inner) {
		t.Fatalf("column baseline = %v %v, want %v", cb, ok, 5+2+inner)
	}
	if _, ok := Box().Size(4, 4).Baseline(); ok {
		t.Fatal("an empty box has a baseline")
	}
}
