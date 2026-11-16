package apitest_test

import (
	"image/color"
	"slices"
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

// The tests here assert how a window paints its frames, which an event
// handed to any widget in the process changes: they run without
// t.Parallel; see ggui.WindowProbe.

// pane is a layout boundary of our own, as ggui.Contain is one: it lays
// its child out under the constraints it is given and clips it to its own
// Rect, and a change of size made inside it is held there.
type pane struct {
	child ggui.Widget
	size  ggui.Size
	hold  ggui.LayoutBoundary
}

func (p *pane) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	inner, check := p.hold.Begin(env)
	p.size = p.child.Layout(c, inner)
	size := c.Constrain(p.size)
	baseline, based := ggui.BaselineOf(p.child)
	p.hold.End(check, c, size, baseline, based)
	return size
}

func (p *pane) Paint(dst *ggui.Canvas, r ggui.Rect) {
	p.hold.Paint(dst, r)
	dst.Clip(r).Paint(p.child, ggui.Rct(r.Origin, p.size))
}

// grower is a block that grows toward the height its target asks, ten
// pixels a layout, calling Invalidate with the Env it was laid out under
// for the next step until it is there.
type grower struct {
	target *ggui.StateValue[float64]
	h      float64
}

func (g *grower) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	if to := g.target.Get(); g.h < to {
		g.h = min(g.h+10, to)
		if g.h < to {
			ggui.Invalidate(env)
		}
	}
	return c.Constrain(ggui.Sz(60, g.h))
}

func (g *grower) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.FillRect(r, gray) }

var gray = color.RGBA{0x80, 0x80, 0x80, 0xff}

// growing runs the window build makes to rest, sets the grower's target
// to 60, and returns the frames that run until the window rests again.
func growing(t *testing.T, build func(ggui.Widget) ggui.Widget) []ggui.WindowFrame {
	t.Helper()
	g := &grower{target: ggui.State(0.0)}
	p := ggui.NewWindowProbe(func() ggui.Widget {
		return ggui.Align(build(ggui.Column(g, ggui.Box().Size(60, 10).Fill(gray)))).At(0, 0)
	}, ggui.Sz(240, 160))
	defer p.Close()
	p.Step(0)
	p.Run(16*time.Millisecond, 10)
	g.target.Set(60)
	frames := p.Run(16*time.Millisecond, 30)
	if len(frames) < 6 || frames[len(frames)-1].Next || g.h != 60 {
		t.Fatalf("grew to %v over %v, want to 60, then rest", g.h, frames)
	}
	return frames
}

// A boundary of our own holds the steps of a size growing inside it: each
// frame after the write that starts it is painted within the boundary
// alone, as one inside a Contain is, frame for frame.
func TestABoundaryOfOurOwnHoldsAChangeOfSizeInside(t *testing.T) {
	fixed := func(boundary func(ggui.Widget) ggui.Widget) func(ggui.Widget) ggui.Widget {
		return func(w ggui.Widget) ggui.Widget {
			return ggui.Row(boundary(ggui.Box(w).Size(100, 120)), ggui.Box().Size(100, 120).Fill(gray))
		}
	}
	ours := growing(t, fixed(func(w ggui.Widget) ggui.Widget { return &pane{child: w} }))
	if ours[0].Painted != ggui.FrameWhole {
		t.Fatalf("the write was painted %v, want whole: %v", ours[0].Painted, ours)
	}
	paintedWithin(t, ours[1:], ggui.Rct(ggui.Pt(0, 0), ggui.Sz(100, 120)))
	contain := growing(t, fixed(func(w ggui.Widget) ggui.Widget { return ggui.Contain(w) }))
	if !sameFrames(ours, contain) {
		t.Fatalf("our boundary's frames %v, a Contain's %v", ours, contain)
	}
}

// A boundary of our own whose size follows its content holds nothing: the
// first frame it was to hold is painted whole, and so is every one after;
// with a fixed one around it, that one holds them. A Contain does the
// same, frame for frame.
func TestABoundaryOfOurOwnSizedByItsContentDefersToTheOneAroundIt(t *testing.T) {
	var runs [][]ggui.WindowFrame
	for _, tc := range []struct {
		name     string
		boundary func(ggui.Widget) ggui.Widget
	}{
		{"ours", func(w ggui.Widget) ggui.Widget { return &pane{child: w} }},
		{"Contain", func(w ggui.Widget) ggui.Widget { return ggui.Contain(w) }},
	} {
		loose := func(w ggui.Widget) ggui.Widget {
			return ggui.Column(tc.boundary(w), ggui.Box().Size(100, 30).Fill(gray))
		}
		alone := growing(t, loose)
		for i, fr := range alone {
			if fr.Painted != ggui.FrameWhole {
				t.Fatalf("%s alone: frame %d was painted %v, want whole: %v", tc.name, i, fr.Painted, alone)
			}
		}

		held := growing(t, func(w ggui.Widget) ggui.Widget {
			return ggui.Row(tc.boundary(ggui.Box(loose(w)).Size(120, 140)), ggui.Box().Size(100, 100).Fill(gray))
		})
		// The write, and the frame that finds the inner one sized by its
		// content; then the outer holds every step.
		whole := 0
		for whole < len(held) && held[whole].Painted == ggui.FrameWhole {
			whole++
		}
		if whole != 2 {
			t.Fatalf("%s: %d frames painted whole before the outer held, want 2: %v", tc.name, whole, held)
		}
		paintedWithin(t, held[whole:], ggui.Rct(ggui.Pt(0, 0), ggui.Sz(120, 140)))
		runs = append(runs, alone, held)
	}
	if !sameFrames(runs[0], runs[2]) || !sameFrames(runs[1], runs[3]) {
		t.Fatalf("our boundary's frames %v and %v, a Contain's %v and %v", runs[0], runs[1], runs[2], runs[3])
	}
}

// paintedWithin fails t unless every one of frames was painted within
// area alone.
func paintedWithin(t *testing.T, frames []ggui.WindowFrame, area ggui.Rect) {
	t.Helper()
	for i, fr := range frames {
		if fr.Painted != ggui.FrameContained || len(fr.Damage) == 0 {
			t.Fatalf("frame %d was painted %v, want within %v: %v", i, fr.Painted, area, frames)
		}
		for _, d := range fr.Damage {
			if d.Empty() || d.Intersect(area) != d {
				t.Fatalf("frame %d painted %v, want within %v", i, fr.Damage, area)
			}
		}
	}
}

// shifter is a scroller of our own: a boundary that shows its child moved
// up by its offset, which the wheel moves ten pixels a line. A move changes
// what the boundary shows and nothing else, so it is made in Contained,
// and asks with Relayout, under the Env of the last layout, for the layout
// that places the child anew: the boundary's own, which no boundary around
// holds, though the move holds it here.
type shifter struct {
	child  ggui.Widget
	offset float64
	env    ggui.Env
	size   ggui.Size
	hold   ggui.LayoutBoundary
	moves  []bool // whether each move was held
}

func (s *shifter) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	s.env = env
	inner, check := s.hold.Begin(env)
	s.size = s.child.Layout(c.Loosen(), inner)
	size := c.Constrain(ggui.Sz(100, 60))
	s.hold.End(check, c, size, 0, false)
	return size
}

func (s *shifter) Paint(dst *ggui.Canvas, r ggui.Rect) {
	s.hold.Paint(dst, r)
	dst.HitPointer(r, s)
	dst.Clip(r).Paint(s.child, ggui.Rct(r.Origin.Add(ggui.Pt(0, -s.offset)), s.size))
}

func (s *shifter) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind != ggui.PointerScroll {
		return false
	}
	s.moves = append(s.moves, s.hold.Contained(func() {
		s.offset -= 10 * ev.Scroll.Y
		ggui.Relayout(s.env)
	}))
	return true
}

func TestAScrollerOfOurOwnMovesWithinItself(t *testing.T) {
	s := &shifter{child: ggui.Column(ggui.Box().Size(100, 50).Fill(gray), ggui.Box().Size(100, 50))}
	p := ggui.NewWindowProbe(func() ggui.Widget {
		return ggui.Align(ggui.Row(ggui.Box().Size(50, 60).Fill(gray), s)).At(0, 0)
	}, ggui.Sz(240, 160))
	defer p.Close()
	if s.hold.Placed() {
		t.Fatal("a boundary never painted is placed")
	}
	p.Step(0)
	p.Run(16*time.Millisecond, 10)
	if !s.hold.Placed() {
		t.Fatal("a boundary painted outside any Transform is not placed")
	}
	// The frame of each move is painted within the boundary, and around
	// the pointer that brought it, as a Scroll's is.
	for range 3 {
		p.Scroll(ggui.Pt(80, 30), ggui.Pt(0, -1))
		if fr := p.Step(16 * time.Millisecond); fr.Painted != ggui.FrameContained {
			t.Fatalf("a move was painted %v, want within the boundary", fr.Painted)
		}
	}
	if s.offset != 30 || !slices.Equal(s.moves, []bool{true, true, true}) {
		t.Fatalf("moved to %v, held %v; want to 30, every move held", s.offset, s.moves)
	}

	// The same move made outside Contained, as a change that shows
	// anywhere, paints the frame whole.
	p.Window().Post(func() {
		s.offset = 0
		ggui.Relayout(s.env)
	})
	if fr := p.Step(16 * time.Millisecond); fr.Painted != ggui.FrameWhole {
		t.Fatalf("a move outside Contained was painted %v, want whole", fr.Painted)
	}
}

// sameFrames reports whether two windows painted their frames alike: as
// many, each the same way, over the same parts of the window.
func sameFrames(a, b []ggui.WindowFrame) bool {
	return slices.EqualFunc(a, b, func(x, y ggui.WindowFrame) bool {
		return x.Painted == y.Painted && x.Next == y.Next && slices.Equal(x.Damage, y.Damage)
	})
}
