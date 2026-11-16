package ggui

import (
	"testing"
	"time"
)

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

// A boundary its parent measures twice a layout, each time under the
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
		pane := &held{child: around(100, 100, column(grows, sized(50, 10)))}
		p := NewWindowProbe(func() Widget { return stack(row(measuredAt(pane, tc.cons...), sized(50, 50))) }, Sz(200, 120))
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
			t.Fatalf("%s: the boundary gave up holding", tc.name)
		}
	}
}

// The boundary a disclosure asks is the nearest, and one that turned out
// to follow its content passes the request to the one around it.
func TestInvalidateAsksTheNearestBoundaryThatHolds(t *testing.T) {
	open := State(false)
	// A disclosure of a block 40 square, easing its height open.
	var m Motion
	r := FromFuncs(func(c Constraints, env Env) Size {
		return c.Constrain(Sz(40, 40*max(m.LayoutToggle(env, open.Get(), 100*time.Millisecond), 0)))
	}, func(*Canvas, Rect) {})
	inner := &held{child: column(r)}
	outer := &held{child: around(100, 100, stack(inner))}
	p := NewWindowProbe(func() Widget { return stack(row(outer, sized(50, 50))) }, Sz(200, 120))
	defer p.Close()
	p.Run(16*time.Millisecond, 10)
	// Two deep: the outer boundary, around the inner, is in none.
	if inner.hold.outer != &outer.hold || outer.hold.outer != nil {
		t.Fatalf("around the inner boundary %p, around that %p; want %p, then none", inner.hold.outer, outer.hold.outer, &outer.hold)
	}
	open.Set(true)
	p.Run(16*time.Millisecond, 30)
	if !inner.hold.sizedByContent || outer.hold.sizedByContent {
		t.Fatalf("sized by content: the inner boundary %v, the outer %v; want the inner alone", inner.hold.sizedByContent, outer.hold.sizedByContent)
	}
}
