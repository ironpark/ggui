package ggui

import (
	"testing"
	"time"
)

func TestCloseDisposesEverythingBuilt(t *testing.T) {
	n := State(0)
	runs := 0
	p := ProbeBuilder(func() Widget {
		Effect(func() { n.Get(); runs++ })
		return Box()
	}, Sz(10, 10)).Setup(func() {
		Watch(n, func(int) { runs++ })
	})
	p.Frame()
	if runs != 2 {
		t.Fatalf("runs = %d, want 2 after the first frame", runs)
	}
	p.Close()
	n.Set(1)
	if !effects.flush() {
		t.Fatal("flush did not settle")
	}
	if runs != 2 {
		t.Fatalf("runs = %d after Close, want 2: no effect may outlive Close", runs)
	}
}

func TestAdvanceStepsAnimations(t *testing.T) {
	tw := Tween(0.0, time.Second).Easing(EaseLinear)
	p := NewProbe(Box(), Sz(10, 10))
	defer p.Close()
	p.Frame()
	tw.Set(100)
	p.Advance(0) // the first step takes the start time
	p.Advance(250 * time.Millisecond)
	if got := tw.Peek(); got != 25 {
		t.Fatalf("value after 250ms = %v, want 25", got)
	}
	if Now() != p.now {
		t.Fatal("Now does not read the probe's clock")
	}
}

func TestPostRunsBeforeTheFrame(t *testing.T) {
	n := State(0)
	var seen []int
	p := ProbeBuilder(func() Widget { seen = append(seen, n.Get()); return Box() }, Sz(10, 10))
	defer p.Close()
	p.Frame()
	p.Post(func() { n.Set(1) })
	p.Frame()
	if len(seen) != 2 || seen[1] != 1 {
		t.Fatalf("builder saw %v, want [0 1]", seen)
	}
}

func TestCycleIsReported(t *testing.T) {
	n := State(0)
	p := ProbeBuilder(func() Widget { return Box() }, Sz(10, 10)).Setup(func() {
		Effect(func() { n.Set(n.Get() + 1) })
	})
	defer p.Close()
	defer func() {
		if r := recover(); r != ErrCycle {
			t.Fatalf("recovered %v, want ErrCycle", r)
		}
	}()
	p.Frame()
	t.Fatal("frame returned")
}
