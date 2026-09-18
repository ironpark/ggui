package ggui

import (
	"errors"
	"strings"
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
		r := recover()
		err, ok := r.(error)
		if !ok || !errors.Is(err, ErrCycle) {
			t.Fatalf("recovered %v, want an error matching ErrCycle", r)
		}
		// The message says how much of the app is caught in the cycle, so
		// that one stuck effect among many is not read as all of them.
		if !strings.Contains(err.Error(), "never settled") {
			t.Fatalf("the error does not say what did not settle: %v", err)
		}
	}()
	p.Frame()
	t.Fatal("frame returned")
}

// Without the debug build there are no creation sites to print, and the
// error says where to get them.
func TestCycleSaysHowToNameTheEffects(t *testing.T) {
	n := State(0)
	p := ProbeBuilder(func() Widget { return Box() }, Sz(10, 10)).Setup(func() {
		Effect(func() { n.Set(n.Get() + 1) })
	})
	defer p.Close()
	defer func() {
		err, _ := recover().(error)
		if err == nil {
			t.Fatal("no cycle reported")
		}
		msg := err.Error()
		if effectOrigin() == "" {
			if !strings.Contains(msg, "ggui_debug") {
				t.Fatalf("the error does not say how to name the effects: %v", msg)
			}
			return
		}
		if !strings.Contains(msg, "loop_test.go:") {
			t.Fatalf("the debug build did not name the effect's creation site: %v", msg)
		}
	}()
	p.Frame()
	t.Fatal("frame returned")
}

func TestProbeLayoutFollowsSetupAndThemeChanges(t *testing.T) {
	old := theme.Peek()
	defer SetTheme(old)
	dark := State(true)
	var seen Theme
	w := FromFuncs(func(c Constraints, env Env) Size {
		seen = env.Theme()
		return c.Constrain(Sz(10, 10))
	}, func(*Canvas, Rect) {})
	p := NewProbe(Cached(w), Sz(20, 20)).Setup(func() {
		BindTheme(dark, DarkTheme(), DefaultTheme())
	})
	defer p.Close()
	p.Frame()
	if seen.Bg != DarkTheme().Bg {
		t.Fatal("first layout did not see the theme set by Setup")
	}
	dark.Set(false)
	p.Frame()
	if seen.Bg != DefaultTheme().Bg {
		t.Fatal("cached layout did not follow the theme change")
	}
}

func TestPostDefersRepostedWorkToNextFrame(t *testing.T) {
	p := NewProbe(Box(), Sz(10, 10))
	defer p.Close()
	var seen []int
	var again func()
	again = func() {
		seen = append(seen, 1)
		// Bound the callback so a regression fails instead of hanging tests.
		if len(seen) < 10 {
			p.Post(again)
		}
	}
	p.Post(again)
	p.Post(func() { seen = append(seen, 2) })
	p.Frame()
	if len(seen) != 2 || seen[0] != 1 || seen[1] != 2 {
		t.Fatalf("first frame ran %v, want [1 2]", seen)
	}
	p.Frame()
	if len(seen) != 3 || seen[2] != 1 {
		t.Fatalf("second frame ran %v, want [1 2 1]", seen)
	}
}

func TestPostedCloseSkipsRemainingWork(t *testing.T) {
	p := NewProbe(Box(), Sz(10, 10))
	defer p.Close()
	p.Post(p.Close)
	p.Post(func() { t.Error("posted work ran after Close") })
	p.Frame()
}

func TestCloseRejectsLatePostedWork(t *testing.T) {
	p := NewProbe(Box(), Sz(10, 10))
	defer p.Close()
	p.Frame()
	start, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		<-start
		for range 100 {
			p.Post(func() { t.Error("work ran after Close") })
			p.Announce("late", Polite)
		}
	}()
	close(start)
	p.Close()
	<-done
	if len(p.posted) != 0 || len(p.notices) != 0 {
		t.Fatal("closed probe retained work or announcements")
	}
}
