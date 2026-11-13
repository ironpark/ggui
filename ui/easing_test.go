package ui

import (
	"math"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

var easingTestSlot = ggui.NewSlot[*ggui.Motion]("easing test")

// A Tailwind transition-* utility runs on cubic-bezier(0.4, 0, 0.2, 1)
// unless an ease-* utility says otherwise: a control's states move on the
// theme's Ease, which is that curve until a theme sets another.
func TestControlTransitionsMoveOnTheThemesCurve(t *testing.T) {
	for _, c := range []struct {
		name  string
		theme uitheme.Theme
		curve ggui.Easing
	}{
		{"default", uitheme.Default(), ggui.CubicBezier(0.4, 0, 0.2, 1)},
		{"unset", func() uitheme.Theme { th := uitheme.Default(); th.Ease = uitheme.Curve{}; return th }(), ggui.CubicBezier(0.4, 0, 0.2, 1)},
		{"custom", func() uitheme.Theme {
			th := uitheme.Default()
			th.Ease = uitheme.Curve{X1: 0, Y1: 0, X2: 1, Y2: 1}
			return th
		}(), ggui.EaseLinear},
	} {
		th := c.theme
		target, got := 0.0, 0.0
		w := ggui.FromFuncs(
			func(cs ggui.Constraints, _ ggui.Env) ggui.Size { return cs.Constrain(ggui.Sz(10, 10)) },
			func(dst *ggui.Canvas, r ggui.Rect) {
				got = ease(dst, &th, ggui.Anchor{Rect: r}, easingTestSlot, target, 200*time.Millisecond)
			})
		p := ggui.NewProbe(w, ggui.Sz(100, 100))
		p.Advance(0)
		target = 1
		p.Advance(0)
		p.Advance(50 * time.Millisecond)
		if want := c.curve(0.25); math.Abs(got-want) > 1e-6 {
			t.Errorf("%s: a quarter of the way in time is %v of the way there, want %v", c.name, got, want)
		}
		p.Advance(50 * time.Millisecond)
		if want := c.curve(0.5); math.Abs(got-want) > 1e-6 {
			t.Errorf("%s: halfway in time is %v of the way there, want %v", c.name, got, want)
		}
		p.Close()
	}
}

// tw-animate-css runs animate-accordion-down on CSS's ease-out keyword,
// cubic-bezier(0, 0, 0.58, 1), over 200ms.
func TestAnAccordionPanelOpensOnCSSEaseOut(t *testing.T) {
	open := ggui.State([]string{})
	a := Accordion(open, AccordionItem("a", "Alpha", ggui.Text("Content")))
	p := ggui.NewProbe(a, ggui.Sz(300, 300))
	defer p.Close()
	p.Advance(0)
	open.Set([]string{"a"})
	p.Advance(0)
	p.Advance(100 * time.Millisecond)
	want := ggui.CubicBezier(0, 0, 0.58, 1)(0.5)
	if got := a.headers[0].progress; math.Abs(got-want) > 1e-6 {
		t.Fatalf("halfway through its 200ms the panel shows %v of its height, want %v", got, want)
	}
}

func TestThemeCurvesAreMadeOnce(t *testing.T) {
	c := uitheme.Curve{X1: 0.3, Y1: 0.1, X2: 0.7, Y2: 0.9}
	c.Easing()
	if n := testing.AllocsPerRun(100, func() { c.Easing() }); n != 0 {
		t.Fatalf("asking a Curve for its Easing allocates %v times", n)
	}
	if got, want := c.Easing()(0.3), ggui.CubicBezier(0.3, 0.1, 0.7, 0.9)(0.3); got != want {
		t.Fatalf("Curve easing = %v, want cubic-bezier's %v", got, want)
	}
}
