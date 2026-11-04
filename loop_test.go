package ggui

import (
	"errors"
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui/internal/reactive"
)

func TestCloseDisposesEverythingBuilt(t *testing.T) {
	t.Parallel()
	n := State(0)
	runs := 0
	p := ProbeBuilder(func() Widget {
		reactive.Observe(func() { n.Get(); runs++ })
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
	if !reactive.Flush() {
		t.Fatal("flush did not settle")
	}
	if runs != 2 {
		t.Fatalf("runs = %d after Close, want 2: no effect may outlive Close", runs)
	}
}

func TestAdvanceStepsAnimations(t *testing.T) {
	t.Parallel()
	tw := Tween(0.0, time.Second).Easing(EaseLinear)
	p := NewProbe(Box(), Sz(10, 10))
	defer p.Close()
	p.Frame()
	tw.Set(100)
	p.Advance(0) // the first step takes the start time
	p.Advance(250 * time.Millisecond)
	if got := Untrack(tw.Get); got != 25 {
		t.Fatalf("value after 250ms = %v, want 25", got)
	}
	if Now() != p.world().frame.pinned {
		t.Fatal("Now does not read the probe's clock")
	}
}

func TestPostRunsBeforeTheFrame(t *testing.T) {
	t.Parallel()
	n := State(0)
	var seen []int
	p := ProbeBuilder(func() Widget { return Reactive(func() Widget { seen = append(seen, n.Get()); return Box() }) }, Sz(10, 10))
	defer p.Close()
	p.Frame()
	p.Post(func() { n.Set(1) })
	p.Frame()
	if len(seen) != 2 || seen[1] != 1 {
		t.Fatalf("builder saw %v, want [0 1]", seen)
	}
}

// Work posted before the first frame, as from main before App.Run, waits
// for Setup and the first build rather than being dropped.
func TestPostBeforeStartRunsAfterSetupAndBuild(t *testing.T) {
	t.Parallel()
	var order []string
	n := State(0)
	p := ProbeBuilder(func() Widget {
		order = append(order, "build")
		return Reactive(func() Widget { n.Get(); return Box() })
	}, Sz(10, 10)).Setup(func() { order = append(order, "setup") })
	defer p.Close()
	p.Post(func() { order = append(order, "post"); n.Set(1) })
	p.Frame()
	p.Frame()
	if want := []string{"setup", "build", "post"}; !slices.Equal(order, want) {
		t.Fatalf("ran %v, want %v, each once", order, want)
	}
}

func TestCycleIsReported(t *testing.T) {
	t.Parallel()
	n := State(0)
	p := ProbeBuilder(func() Widget { return Box() }, Sz(10, 10)).Setup(func() {
		reactive.Observe(func() { n.Set(n.Get() + 1) })
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
	t.Parallel()
	n := State(0)
	p := ProbeBuilder(func() Widget { return Box() }, Sz(10, 10)).Setup(func() {
		reactive.Observe(func() { n.Set(n.Get() + 1) })
	})
	defer p.Close()
	defer func() {
		err, _ := recover().(error)
		if err == nil {
			t.Fatal("no cycle reported")
		}
		msg := err.Error()
		if !reactive.Debug {
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

func TestProbeLayoutFollowsSetupAndEnvironmentChanges(t *testing.T) {
	t.Parallel()
	old := Untrack(UseEnv)
	defer SetEnv(old)
	dark := State(true)
	var seen float64
	w := FromFuncs(func(c Constraints, env Env) Size {
		seen = env.Spacing()
		return c.Constrain(Sz(10, 10))
	}, func(*Canvas, Rect) {})
	p := NewProbe(Cached(w), Sz(20, 20)).Setup(func() {
		Watch(dark, func(v bool) { SetEnv(Env{}.With(SpacingKey, pick(v, 12.0, 8.0))) })
	})
	defer p.Close()
	p.Frame()
	if seen != 12 {
		t.Fatal("first layout did not see the environment set by Setup")
	}
	dark.Set(false)
	p.Frame()
	if seen != 8 {
		t.Fatal("cached layout did not follow the environment change")
	}
}

func TestPostDefersRepostedWorkToNextFrame(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	p := NewProbe(Box(), Sz(10, 10))
	defer p.Close()
	p.Post(p.Close)
	p.Post(func() { t.Error("posted work ran after Close") })
	p.Frame()
}

func TestCloseRejectsLatePostedWork(t *testing.T) {
	t.Parallel()
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

// The pattern UIThread exists for: a component starts work on a goroutine
// and the result reaches its state on the UI goroutine, with no App handed
// down from main.
func TestUIThreadCarriesAResultBackIntoAComponent(t *testing.T) {
	t.Parallel()
	done := make(chan struct{})
	var text *StateValue[string]
	p := ProbeBuilder(func() Widget {
		return Component(func() Widget {
			loaded := State("loading")
			text = loaded
			post := UIThread()
			go func() {
				post(func() { loaded.Set("done") })
				close(done)
			}()
			return Reactive(func() Widget { return Text(loaded.Get()) })
		})
	}, Sz(100, 20))
	defer p.Close()

	p.Frame() // first layout runs setup, which starts the goroutine
	<-done
	if got := Untrack(text.Get); got != "loading" {
		t.Fatalf("the posted write landed before a frame ran it: %q", got)
	}
	p.Frame()
	if got := Untrack(text.Get); got != "done" {
		t.Fatalf("after a frame the value is %q, want done", got)
	}
}

func TestUIThreadOutsideAFrameIsRefused(t *testing.T) {
	t.Parallel()
	p := ProbeBuilder(func() Widget { return Box() }, Sz(10, 10))
	p.Frame()
	p.Close()
	defer func() {
		if recover() == nil {
			t.Fatal("UIThread outside a frame did not say so")
		}
	}()
	UIThread()
}

func TestWorkPostedAfterCloseIsDropped(t *testing.T) {
	t.Parallel()
	var post func(func())
	p := ProbeBuilder(func() Widget { post = UIThread(); return Box() }, Sz(10, 10))
	p.Frame()
	p.Close()
	ran := false
	post(func() { ran = true })
	if ran {
		t.Fatal("work posted to a closed app ran immediately")
	}
}

// stamp paints its label twice in different colors, as a shimmer does.
type stamp struct{ label *TextWidget }

func (s *stamp) Layout(c Constraints, env Env) Size { return s.label.Layout(c, env) }
func (s *stamp) Paint(dst *Canvas, r Rect) {
	s.label.Color(color.White)
	dst.Paint(s.label, r)
	s.label.Color(color.Black)
	dst.Paint(s.label, r)
}

func TestPaintTimeWritesScheduleNoLayout(t *testing.T) {
	t.Parallel()
	p := NewProbe(&stamp{label: Text("hi")}, Sz(100, 20))
	defer p.Close()
	p.Frame()
	p.Frame()
	// The layout generation is this goroutine's, so nothing another test
	// does moves it.
	before := reactive.LayoutGen()
	p.Frame()
	if reactive.LayoutGen() != before {
		t.Fatal("configuring a child while painting scheduled a layout")
	}
}

func TestCloseRequestCanKeepTheWindowOpen(t *testing.T) {
	t.Parallel()
	asking := State(false)
	p := NewProbe(Text("doc"), Sz(100, 20))
	defer p.Close()
	var order []string
	p.OnCloseRequest(func() bool { order = append(order, "save"); asking.Set(true); return false })
	p.OnCloseRequest(func() bool { order = append(order, "second"); return true })
	p.Frame()
	if p.RequestClose() {
		t.Fatal("closed although a handler kept the window open")
	}
	if !Untrack(asking.Get) || len(order) != 1 {
		t.Fatalf("handlers ran %v; the first false must stop the rest", order)
	}
	p.Frame() // still usable: the question is up
	p.Close()
	if p.RequestClose() {
		t.Fatal("a closed probe closed again")
	}
}

func TestCloseRequestWithAgreeingHandlersCloses(t *testing.T) {
	t.Parallel()
	p := NewProbe(Text("doc"), Sz(100, 20))
	defer p.Close()
	p.OnCloseRequest(func() bool { return true })
	p.Frame()
	if !p.RequestClose() {
		t.Fatal("handlers agreed but the probe stayed open")
	}
}

func TestNowIsFrozenForTheFrame(t *testing.T) {
	t.Parallel()
	f := &activeWorld().frame
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

// A paced look asks for its next frame at the next step, and does not keep
// the frame reading the time, which would ask for every refresh.
func TestPacedAsksForTheNextStepAlone(t *testing.T) {
	f := &activeWorld().frame
	start := time.Date(2026, 1, 1, 0, 0, 0, 10_000_000, time.UTC) // 10 ms past a second
	f.begin(start)
	now := Paced(50 * time.Millisecond)
	if !now.Equal(f.peek()) {
		t.Fatalf("Paced read %v, the frame is at %v", now, f.peek())
	}
	if f.timeRead() {
		t.Fatal("a paced look marked the frame as reading the time")
	}
	if got, want := f.takeWake(), start.Truncate(50*time.Millisecond).Add(50*time.Millisecond); !got.Equal(want) {
		t.Fatalf("woke at %v, want %v", got, want)
	}
}

func TestInnerEffectDisposedWhenOuterReruns(t *testing.T) {
	t.Parallel()
	outerDep, innerDep := State(0), State(0)
	innerRuns := 0
	dispose := reactive.Observe(func() {
		outerDep.Get()
		reactive.Observe(func() { innerDep.Get(); innerRuns++ })
	})
	defer dispose()
	if innerRuns != 1 {
		t.Fatalf("innerRuns = %d after setup, want 1", innerRuns)
	}
	innerDep.Set(1)
	reactive.Flush()
	if innerRuns != 2 {
		t.Fatalf("innerRuns = %d after inner change, want 2", innerRuns)
	}
	outerDep.Set(1) // outer re-runs: old inner disposed, one new inner created
	reactive.Flush()
	innerDep.Set(2)
	reactive.Flush()
	if innerRuns != 4 {
		t.Fatalf("innerRuns = %d, want 4 (not 5: the stale inner must not fire)", innerRuns)
	}
}

func TestDerivedInsideBuilderDoesNotAccumulate(t *testing.T) {
	t.Parallel()
	rebuild, n := State(0), State(1)
	computes := 0
	dispose := reactive.Observe(func() {
		rebuild.Get()
		m := Derived(func() int { computes++; return n.Get() * 2 })
		reactive.Observe(func() { m.Get() })
	})
	defer dispose()
	rebuild.Set(1)
	rebuild.Set(2)
	reactive.Flush()
	computes = 0
	n.Set(5)
	reactive.Flush()
	if computes != 1 {
		t.Fatalf("computes = %d after one change, want 1 live Derived", computes)
	}
}

func TestOnCleanupRunsBeforeRerunAndOnDispose(t *testing.T) {
	t.Parallel()
	dep := State(0)
	var log []string
	dispose := reactive.Observe(func() {
		v := dep.Get()
		log = append(log, "run")
		OnCleanup(func() { log = append(log, "cleanup") })
		_ = v
	})
	dep.Set(1)
	reactive.Flush()
	dispose()
	dispose() // idempotent
	want := []string{"run", "cleanup", "run", "cleanup"}
	if len(log) != len(want) {
		t.Fatalf("log = %v, want %v", log, want)
	}
	for i := range want {
		if log[i] != want[i] {
			t.Fatalf("log = %v, want %v", log, want)
		}
	}
}

func TestOnCleanupOutsideEffectPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("OnCleanup outside an Effect did not panic")
		}
	}()
	OnCleanup(func() {})
}

func TestUntrackAndPeekDoNotSubscribe(t *testing.T) {
	t.Parallel()
	a, b := State(0), State(0)
	runs := 0
	dispose := reactive.Observe(func() {
		runs++
		Untrack(func() struct {
		} {
			a.Get()
			return struct {
			}{}
		})
		Untrack(b.Get)
	})
	defer dispose()
	a.Set(1)
	b.Set(1)
	reactive.Flush()
	if runs != 1 {
		t.Fatalf("runs = %d, want 1: untracked reads must not subscribe", runs)
	}
}

func TestFlushSkipsEffectsDisposedEarlierInThePass(t *testing.T) {
	t.Parallel()
	dep := State(0)
	staleRuns := 0
	dispose := reactive.Observe(func() {
		dep.Get()
		gen := Untrack(dep.Get)
		reactive.Observe(func() {
			dep.Get()
			if gen != Untrack(dep.Get) {
				staleRuns++ // a child from an older parent run fired
			}
		})
	})
	defer dispose()
	dep.Set(1)
	reactive.Flush()
	if staleRuns != 0 {
		t.Fatalf("a disposed child effect ran %d times", staleRuns)
	}
}

func TestReactiveRebuildsWithoutParent(t *testing.T) {
	t.Parallel()
	leaf := State("a")
	parentBuilds, leafBuilds := 0, 0
	var root Widget
	dispose := reactive.Observe(func() {
		parentBuilds++
		root = Column(Reactive(func() Widget { leafBuilds++; return Text(leaf.Get()) }))
	})
	defer dispose()
	leaf.Set("b")
	reactive.Flush()
	if parentBuilds != 1 || leafBuilds != 2 {
		t.Fatalf("parentBuilds = %d, leafBuilds = %d; want 1 and 2", parentBuilds, leafBuilds)
	}
	if got := root.(*ColumnWidget).children[0].(*ComponentWidget).child.(*TextWidget).value; got != "b" {
		t.Fatalf("leaf shows %q, want b", got)
	}
}

func TestComponentRunsSetupOnceAndKeepsState(t *testing.T) {
	t.Parallel()
	setups, builds := 0, 0
	var local *StateValue[int]
	var c *ComponentWidget
	dispose := reactive.Observe(func() {
		c = Component(func() Widget {
			setups++
			local = State(0)
			return Reactive(func() Widget { builds++; return Box().Size(float64(local.Get()), 1) })
		})
	})
	defer dispose()
	c.Layout(Loose(Sz(100, 100)), Env{}) // mounts
	local.Set(7)
	reactive.Flush()
	if setups != 1 || builds != 2 {
		t.Fatalf("setups = %d, builds = %d; want 1 and 2", setups, builds)
	}
	if got := c.Layout(Loose(Sz(100, 100)), Env{}); got.W != 7 {
		t.Fatalf("component laid out at %v wide, want 7 from its own state", got.W)
	}
}

func TestComponentSetupDoesNotSubscribeParent(t *testing.T) {
	t.Parallel()
	dep := State(0)
	parentBuilds := 0
	var c *ComponentWidget
	dispose := reactive.Observe(func() {
		parentBuilds++
		c = Component(func() Widget {
			dep.Get() // read during setup
			return Reactive(func() Widget { return Box() })
		})
	})
	defer dispose()
	c.Layout(Loose(Sz(100, 100)), Env{})
	dep.Set(1)
	reactive.Flush()
	if parentBuilds != 1 {
		t.Fatalf("parent rebuilt %d times on a setup read, want 1", parentBuilds)
	}
}

func TestComponentDisposedWithParent(t *testing.T) {
	t.Parallel()
	parentDep, leaf := State(0), State(0)
	setups, builds := 0, 0
	var c *ComponentWidget
	dispose := reactive.Observe(func() {
		parentDep.Get()
		c = Component(func() Widget {
			setups++
			return Reactive(func() Widget { builds++; leaf.Get(); return Box() })
		})
	})
	defer dispose()
	c.Layout(Loose(Sz(100, 100)), Env{})
	parentDep.Set(1)
	reactive.Flush()
	c.Layout(Loose(Sz(100, 100)), Env{})
	builds = 0
	leaf.Set(1)
	reactive.Flush()
	if setups != 2 || builds != 1 {
		t.Fatalf("setups = %d, builds = %d; want 2 setups and 1 live builder", setups, builds)
	}
}

func TestChildCleanupCanDisposeSibling(t *testing.T) {
	t.Parallel()
	var stopSibling func()
	var cleaned []int
	dispose := Root(func() {
		reactive.Observe(func() { OnCleanup(func() { cleaned = append(cleaned, 0); stopSibling() }) })
		stopSibling = reactive.Observe(func() { OnCleanup(func() { cleaned = append(cleaned, 1) }) })
		reactive.Observe(func() { OnCleanup(func() { cleaned = append(cleaned, 2) }) })
	})
	defer dispose()
	dispose()
	if len(cleaned) != 3 || cleaned[0] != 0 || cleaned[1] != 1 || cleaned[2] != 2 {
		t.Fatalf("cleanup skipped or repeated a sibling: %v", cleaned)
	}
}

func TestEffectRemovalPreservesExecutionOrder(t *testing.T) {
	t.Parallel()
	n := State(0)
	var seen []int
	var stops []func()
	dispose := Root(func() {
		for i := range 5 {
			stops = append(stops, reactive.Observe(func() { n.Get(); seen = append(seen, i) }))
		}
	})
	defer dispose()
	stops[1]()
	stops[3]()
	seen = nil
	n.Set(1)
	if !reactive.Flush() {
		t.Fatal("effects did not settle")
	}
	if len(seen) != 3 || seen[0] != 0 || seen[1] != 2 || seen[2] != 4 {
		t.Fatalf("effect order = %v, want [0 2 4]", seen)
	}
}

// mountProps holds a slice, so == cannot compare it, and declares its own
// equality instead.
type mountProps struct {
	Title string
	Tags  []string
}

func (p mountProps) Equal(o mountProps) bool {
	return p.Title == o.Title && slices.Equal(p.Tags, o.Tags)
}

func TestEqualDerivedPropsDoNotRebuild(t *testing.T) {
	t.Parallel()
	tick := State(0)
	builds := 0
	p := ProbeBuilder(func() Widget {
		props := Derived(func() mountProps { tick.Get(); return mountProps{Title: "a", Tags: []string{"x"}} })
		return View(props, func(value mountProps) Widget { builds++; return Text(value.Title) })
	}, Sz(200, 200))
	defer p.Close()
	p.Frame()
	for range 3 {
		tick.Update(func(n int) int { return n + 1 })
		p.Frame()
	}
	if builds != 1 {
		t.Fatalf("equal props rebuilt %d times", builds)
	}
}

// A type without an Equal method and without comparable fields notifies on
// every write, as it always has: there is nothing to compare it with.
func TestSignalWithoutEqualityNotifiesEveryWrite(t *testing.T) {
	t.Parallel()
	s := State([]int{1})
	runs := 0
	defer reactive.Observe(func() { s.Get(); runs++ })()
	s.Set([]int{1})
	reactive.Flush()
	if runs != 2 {
		t.Fatalf("effect ran %d times, want a notification for an uncomparable write", runs)
	}
}
