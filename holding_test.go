package ggui

import (
	"testing"
	"time"
)

// A Contain laid out and painted at rest, inside another, allocates
// nothing: the Env it hands down is the one it handed down last time.
func TestAContainAtRestAllocatesNothing(t *testing.T) {
	c := Contain(Box(Contain(Box().Size(10, 10))).Size(50, 50))
	run := func() {
		size := c.Layout(Tight(Sz(50, 50)), Env{})
		c.Paint(nil, Rct(Point{}, size))
	}
	run()
	if n := testing.AllocsPerRun(100, run); n != 0 {
		t.Fatalf("a Contain at rest allocates %v times a frame", n)
	}
}

// measuredAt lays child out under each of cons in turn, as a parent that
// measures a child before it settles on a size does, and paints it at
// the last.
func measuredAt(child Widget, cons ...Constraints) Widget {
	var size Size
	return FromFuncs(func(c Constraints, env Env) Size {
		for _, k := range cons {
			size = child.Layout(k, env)
		}
		return c.Constrain(size)
	}, func(dst *Canvas, r Rect) { dst.Paint(child, Rct(r.Origin, size)) })
}

// A Contain its parent measures twice a layout, each time under the
// constraints of the last, is checked against each and holds; one
// measured under more constraints than it keeps cannot be checked, and
// its frames are painted whole, though it stays a boundary: the easing
// still comes to rest.
func TestAContainMeasuredMoreThanOnceIsCheckedUnderEach(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cons  []Constraints
		holds bool
	}{
		{"twice", []Constraints{Loose(Sz(100, 100)), Tight(Sz(100, 100))}, true},
		{"past what it keeps", []Constraints{
			Tight(Sz(100, 100)), Tight(Sz(100, 101)), Tight(Sz(100, 102)), Tight(Sz(100, 103)), Tight(Sz(100, 104)),
		}, false},
	} {
		// The child grows a step at each of its layouts.
		steps := 6 * len(tc.cons)
		grows, laid, painted := easing(steps)
		// As large as either constraint allows, whatever grows inside.
		pane := Contain(Box(Column(grows, Box().Size(50, 10))).Size(100, 100))
		p := NewWindowProbe(func() Widget { return topLeft(Row(measuredAt(pane, tc.cons...), Box().Size(50, 50))) }, Sz(200, 120))
		p.Step(0)
		frames := p.Run(0, 18)
		p.Close()
		if *laid < steps || *painted != *laid || len(frames) < 4 || frames[len(frames)-1].Next {
			t.Fatalf("%s: laid out %d times, painted the %dth, over %v", tc.name, *laid, *painted, frames)
		}
		for i, fr := range frames {
			if held := fr.Painted == FrameContained; held != tc.holds {
				t.Fatalf("%s: frame %d was painted %v: %v", tc.name, i, fr.Painted, frames)
			}
		}
		if pane.hold.sizedByContent {
			t.Fatalf("%s: the Contain gave up holding", tc.name)
		}
	}
}

// The Contain a disclosure asks is the nearest, and one that turned out
// to follow its content passes the request to the one around it.
func TestInvalidateAsksTheNearestBoundaryThatHolds(t *testing.T) {
	open := State(false)
	r := Reveal(open, Box().Size(40, 40)).Duration(100 * time.Millisecond)
	inner := Contain(Column(r))
	outer := Contain(Box(topLeft(inner)).Size(100, 100))
	p := NewWindowProbe(func() Widget { return topLeft(Row(outer, Box().Size(50, 50))) }, Sz(200, 120))
	defer p.Close()
	p.Run(16*time.Millisecond, 10)
	if inner.hold.outer != &outer.hold || inner.hold.depth != 2 {
		t.Fatalf("the inner Contain is %d deep, around it %p, want 2 and %p", inner.hold.depth, inner.hold.outer, &outer.hold)
	}
	open.Set(true)
	p.Run(16*time.Millisecond, 30)
	if !inner.hold.sizedByContent || outer.hold.sizedByContent {
		t.Fatalf("sized by content: the inner Contain %v, the outer %v; want the inner alone", inner.hold.sizedByContent, outer.hold.sizedByContent)
	}
}
