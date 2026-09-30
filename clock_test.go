package ggui

import (
	"testing"
	"time"
)

func TestNowIsFrozenForTheFrame(t *testing.T) {
	f := &activeWorld().frame
	defer f.reset()
	f.reset()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	raw := t0
	defer SetClock(func() time.Time { return raw })()

	f.begin(raw)
	first := Now()
	raw = raw.Add(3 * time.Millisecond) // time passes while the frame paints
	if got := Now(); got != first {
		t.Fatalf("Now moved mid-frame from %v to %v: widgets in one frame must agree", first, got)
	}
	if got := f.begin(raw); got != first.Add(3*time.Millisecond) {
		t.Fatalf("next frame at %v, want %v", got, first.Add(3*time.Millisecond))
	}
}

func TestFrameClockCapsALongPause(t *testing.T) {
	t.Parallel()
	var f frameClock
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f.begin(t0)
	// The window was hidden for five seconds with no frame, and the frame
	// that comes back must not jump.
	if got := f.begin(t0.Add(5 * time.Second)); got != t0.Add(maxFrameStep) {
		t.Fatalf("after a pause the clock is at %v, want %v", got, t0.Add(maxFrameStep))
	}
	if got := f.begin(t0.Add(5*time.Second + 16*time.Millisecond)); got != t0.Add(maxFrameStep+16*time.Millisecond) {
		t.Fatalf("the frame after the pause is at %v, want a normal step on", got)
	}
}

func TestFrameClockNeverGoesBackwards(t *testing.T) {
	t.Parallel()
	var f frameClock
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f.begin(t0.Add(time.Second))
	if got := f.begin(t0); got != t0.Add(time.Second) {
		t.Fatalf("a clock that went back moved Now to %v", got)
	}
}

func TestProbeAdvanceStepsPastTheCap(t *testing.T) {
	t.Parallel()
	tw := Tween(0.0, time.Second).Easing(EaseLinear)
	p := NewProbe(Reactive(func() Widget { return Box().Size(tw.Get(), 1) }), Sz(200, 20))
	defer p.Close()
	p.Frame()
	tw.Set(100)
	p.Advance(0)                      // the first step takes the tween's start
	p.Advance(250 * time.Millisecond) // far more than maxFrameStep
	if got := Untrack(tw.Get); got != 25 {
		t.Fatalf("tween at %v after advancing 250ms, want 25: a probe steps by exactly what it asked for", got)
	}
}

// clockReader reads Now while it paints, as a spinner does.
type clockReader struct{}

func (clockReader) Layout(c Constraints, _ Env) Size { return c.Constrain(Sz(20, 20)) }
func (clockReader) Paint(*Canvas, Rect)              { Now() }

func TestTimeReadOutsideTheClipDoesNotAnimate(t *testing.T) {
	t.Parallel()
	offset := State(0.0)
	p := NewProbe(Scroll(Column(Box().Size(100, 500), clockReader{})).BindOffset(offset), Sz(100, 100))
	defer p.Close()
	animating := func() bool {
		f := &p.world().frame
		f.mu.Lock()
		f.read = false
		f.mu.Unlock()
		p.Frame()
		return f.timeRead()
	}
	if animating() {
		t.Fatal("a clock read below the viewport asked for another frame")
	}
	offset.Set(450)
	if !animating() {
		t.Fatal("the clock read scrolled into view did not ask for another frame")
	}
}
