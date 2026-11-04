package kinetics

import (
	"math"
	"testing"
	"time"

	"github.com/ironpark/ggui/pkg/layout"
)

func TestRubberFollowsInsideAndResistsPast(t *testing.T) {
	t.Parallel()
	const lo, hi, extent = 0.0, 1000.0, 400.0
	if got := Rubber(500, 30, lo, hi, extent, false); got != 530 {
		t.Fatalf("inside the range moved to %v, want 530", got)
	}
	// Past an end, half the movement at the end itself.
	if got := Rubber(1000, 40, lo, hi, extent, false); got != 1020 {
		t.Fatalf("pulled from the end to %v, want half of 40 past it", got)
	}
	// Further past, less of it; momentum less again.
	near, far := Rubber(1010, 10, lo, hi, extent, false)-1010, Rubber(1200, 10, lo, hi, extent, false)-1200
	if !(far < near && near < 10) {
		t.Fatalf("resistance does not grow past the end: %v near, %v far", near, far)
	}
	if fling := Rubber(1010, 10, lo, hi, extent, true) - 1010; fling >= near {
		t.Fatalf("momentum met %v, a finger %v: want less for momentum", fling, near)
	}
	// A whole extent past, it moves no further; back toward the range,
	// freely.
	if got := Rubber(hi+extent, 50, lo, hi, extent, false); got != hi+extent {
		t.Fatalf("an extent past the end moved to %v", got)
	}
	if got := Rubber(-100, 30, lo, hi, extent, false); got != -70 {
		t.Fatalf("moving back from before the start went to %v, want -70", got)
	}
}

func TestStretchGrowsToHalfAgain(t *testing.T) {
	t.Parallel()
	for over, want := range map[float64]float64{0: 1, 100: 1.125, -200: 1.25, 400: 1.5, 4000: 1.5} {
		if got := Stretch(over, 400); math.Abs(got-want) > 1e-12 {
			t.Errorf("Stretch(%v, 400) = %v, want %v", over, got, want)
		}
	}
}

func TestSnapTarget(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                      string
		pos, end, step, from, dir float64
		paging                    bool
		want                      float64
	}{
		{"nearest row", 130, 1000, 50, 0, 1, false, 150},
		{"the end is a stop", 980, 990, 100, 0, 1, false, 990},
		{"clamped to the end", 1040, 1000, 100, 0, 1, false, 1000},
		{"page back under a sixth", 350, 2000, 400, 400, -1, true, 400},
		{"next page past a sixth", 500, 2000, 400, 400, 1, true, 800},
		{"previous page past a sixth", 300, 2000, 400, 400, -1, true, 0},
	} {
		if got := SnapTarget(tc.pos, tc.end, tc.step, tc.from, tc.dir, tc.paging); got != tc.want {
			t.Errorf("%s: SnapTarget = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A fling covers the same distance and stops at the same time whatever
// steps it is advanced in.
func TestFlingIsIndependentOfTheStep(t *testing.T) {
	t.Parallel()
	run := func(step float64) (total layout.Point, steps int) {
		v := layout.Pt(300, -1000)
		for Flinging(v) {
			travel, next, done := Fling(v, step)
			total, v = total.Add(travel), next
			steps++
			if done {
				break
			}
		}
		return total, steps
	}
	a, na := run(1.0 / 60)
	b, nb := run(1.0 / 144)
	if math.Abs(a.X-b.X) > 1e-6 || math.Abs(a.Y-b.Y) > 1e-6 {
		t.Fatalf("60Hz travelled %v, 144Hz %v", a, b)
	}
	if na >= nb || na == 0 {
		t.Fatalf("steps %d at 60Hz and %d at 144Hz", na, nb)
	}
	// The whole distance is the speed over the friction, less what the
	// stopping speed would have gone on to cover.
	want := (1000 - FlingMinSpeed*1000/math.Hypot(300, 1000)) / FlingFriction
	if math.Abs(-a.Y-want) > 1e-6 {
		t.Fatalf("travelled %v vertically, want %v", -a.Y, want)
	}
}

func TestTrackerFiltersAndReverses(t *testing.T) {
	t.Parallel()
	now := time.Unix(100, 0)
	tr := Tracker{Last: now}
	for i := 1; i <= 20; i++ {
		tr.Sample(now.Add(time.Duration(i)*16*time.Millisecond), layout.Pt(0, -16*float64(i)), 100*time.Millisecond)
	}
	if v := tr.Velocity.Y; math.Abs(v+1000) > 20 {
		t.Fatalf("a steady 1000px/s upward drag tracked at %v", v)
	}
	// A reversal drops the old momentum at once.
	at := tr.Last.Add(16 * time.Millisecond)
	tr.Sample(at, tr.Pos.Add(layout.Pt(0, 10)), 100*time.Millisecond)
	if tr.Velocity.Y <= 0 {
		t.Fatalf("velocity kept the old direction: %v", tr.Velocity)
	}
	// A pause longer than the gap drops it altogether.
	tr.Sample(at.Add(time.Second), tr.Pos.Add(layout.Pt(0, 10)), 100*time.Millisecond)
	if tr.Velocity != (layout.Point{}) {
		t.Fatalf("velocity after a pause: %v", tr.Velocity)
	}
}

func TestBoostLeavesSlowGesturesAlone(t *testing.T) {
	t.Parallel()
	slow := Tracker{Velocity: layout.Pt(0, 120)}
	slow.Boost()
	if slow.Velocity.Y != 120 {
		t.Fatalf("a slow gesture was boosted to %v", slow.Velocity)
	}
	brisk := Tracker{Velocity: layout.Pt(0, 2000)}
	brisk.Boost()
	if brisk.Velocity.Y != 3600 {
		t.Fatalf("a brisk flick boosted to %v, want 1.8 times", brisk.Velocity)
	}
	capped := Tracker{Velocity: layout.Pt(0, 7000)}
	capped.Boost()
	if capped.Velocity.Y != FlingMaxSpeed {
		t.Fatalf("a boost passed the cap: %v", capped.Velocity)
	}
}

var sink float64

func BenchmarkCubicBezier(b *testing.B) {
	e := CubicBezier(0.25, 0.1, 0.25, 1)
	t := 0.0
	for b.Loop() {
		sink += e(t)
		t += 0.0137
		if t > 1 {
			t = 0
		}
	}
}

func BenchmarkEaseEmphasized(b *testing.B) {
	t := 0.0
	for b.Loop() {
		sink += EaseEmphasized(t)
		t += 0.0137
		if t > 1 {
			t = 0
		}
	}
}

func BenchmarkRubber(b *testing.B) {
	raw := 900.0
	for b.Loop() {
		raw = Rubber(raw, 3, 0, 1000, 400, false)
		if raw > 1300 {
			raw = 900
		}
	}
	sink += raw
}

func BenchmarkFlingStep(b *testing.B) {
	v := layout.Pt(300, -4000)
	for b.Loop() {
		travel, next, done := Fling(v, 1.0/120)
		sink += travel.Y
		if v = next; done || !Flinging(v) {
			v = layout.Pt(300, -4000)
		}
	}
}

func BenchmarkTrackerSample(b *testing.B) {
	now := time.Unix(100, 0)
	tr := Tracker{Last: now}
	y := 0.0
	for b.Loop() {
		now = now.Add(8 * time.Millisecond)
		y -= 8
		tr.Sample(now, layout.Pt(0, y), 100*time.Millisecond)
	}
	sink += tr.Velocity.Y
}
