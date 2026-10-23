package ggui

import (
	"math"
	"testing"
)

func TestCurvesMapEndpoints(t *testing.T) {
	t.Parallel()
	curves := map[string]Easing{
		"InOutCubic": EaseInOutCubic, "InBack": EaseInBack, "OutBack": EaseOutBack, "InOutBack": EaseInOutBack,
		"OutElastic": EaseOutElastic, "InElastic": EaseInElastic, "OutBounce": EaseOutBounce, "InBounce": EaseInBounce,
		"Standard": EaseStandard, "StandardAccelerate": EaseStandardAccelerate, "StandardDecelerate": EaseStandardDecelerate,
		"Emphasized": EaseEmphasized, "EmphasizedAccelerate": EaseEmphasizedAccelerate, "EmphasizedDecelerate": EaseEmphasizedDecelerate,
		"CubicBezier": CubicBezier(0.25, 0.1, 0.25, 1), "Reverse": Reverse(EaseIn), "Interval": Interval(0.2, 0.8, EaseOut),
	}
	for name, e := range curves {
		if math.Abs(e(0)) > 1e-9 || math.Abs(e(1)-1) > 1e-9 {
			t.Errorf("%s maps 0 to %v and 1 to %v", name, e(0), e(1))
		}
	}
}

func TestCubicBezierMatchesCSS(t *testing.T) {
	t.Parallel()
	linear := CubicBezier(0, 0, 1, 1)
	for _, x := range []float64{0.1, 0.33, 0.5, 0.9} {
		if math.Abs(linear(x)-x) > 1e-5 {
			t.Fatalf("cubic-bezier(0,0,1,1)(%v) = %v", x, linear(x))
		}
	}
	if v := CubicBezier(0.42, 0, 0.58, 1)(0.5); math.Abs(v-0.5) > 1e-5 {
		t.Fatalf("ease-in-out at 0.5 = %v", v)
	}
	// CSS "ease" at half time is about 0.8024.
	if v := CubicBezier(0.25, 0.1, 0.25, 1)(0.5); math.Abs(v-0.8024) > 1e-3 {
		t.Fatalf("ease at 0.5 = %v", v)
	}
	prev := 0.0
	for i := 1; i <= 100; i++ {
		v := EaseStandard(float64(i) / 100)
		if v < prev-1e-9 {
			t.Fatalf("EaseStandard is not monotonic at %v", float64(i)/100)
		}
		prev = v
	}
}

func TestCurveShapes(t *testing.T) {
	t.Parallel()
	over := false
	for i := 1; i < 100; i++ {
		over = over || EaseOutBack(float64(i)/100) > 1
	}
	if !over {
		t.Fatal("EaseOutBack never overshoots")
	}
	if EaseInBack(0.2) >= 0 {
		t.Fatal("EaseInBack does not pull back")
	}
	if v := EaseEmphasized(1.0 / 6); math.Abs(v-0.4) > 1e-6 {
		t.Fatalf("EaseEmphasized at its joint = %v, want 0.4", v)
	}
	for _, x := range []float64{0.1, 0.4, 0.7} {
		if math.Abs(Reverse(EaseIn)(x)-EaseOut(x)) > 1e-9 {
			t.Fatalf("Reverse(EaseIn) differs from EaseOut at %v", x)
		}
		if math.Abs(EaseInBounce(x)-(1-EaseOutBounce(1-x))) > 1e-9 {
			t.Fatal("EaseInBounce is not EaseOutBounce reversed")
		}
	}
}

func TestIntervalRunsInItsStretch(t *testing.T) {
	t.Parallel()
	e := Interval(0.25, 0.75, nil)
	for x, want := range map[float64]float64{0: 0, 0.25: 0, 0.5: 0.5, 0.75: 1, 1: 1} {
		if got := e(x); math.Abs(got-want) > 1e-9 {
			t.Fatalf("Interval(0.25, 0.75)(%v) = %v, want %v", x, got, want)
		}
	}
	step := Interval(0.5, 0.5, EaseOut)
	if step(0.4) != 0 || step(0.5) != 1 {
		t.Fatal("an empty interval should step at its point")
	}
}
