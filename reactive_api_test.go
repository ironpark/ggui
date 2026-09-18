package ggui

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDerivedIsLazyPureAndUntrackIsFresh(t *testing.T) {
	n := State(1)
	calls := 0
	m := Derived(func() int { calls++; return n.Get() * 2 })
	defer m.Dispose()
	if calls != 0 {
		t.Fatal("derived evaluated at construction")
	}
	if Untrack(m.Get) != 2 || calls != 1 {
		t.Fatal("first read")
	}
	n.Set(2)
	effects.flush()
	if calls != 1 {
		t.Fatal("unobserved derived evaluated during flush")
	}
	if Untrack(m.Get) != 4 || calls != 2 {
		t.Fatal("untracked read was stale")
	}
	bad := Derived(func() int { n.Set(9); return 0 })
	defer bad.Dispose()
	assertPanics(t, func() { bad.Get() })
	if n.Get() != 2 {
		t.Fatal("derived changed user state")
	}
	var cycle *DerivedValue[int]
	cycle = Derived(func() int { return cycle.Get() })
	defer cycle.Dispose()
	assertPanics(t, func() { cycle.Get() })
}

func assertPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected panic")
		}
	}()
	fn()
}

func TestPublicEffectMountOrderCleanupAndDispose(t *testing.T) {
	n, shown := State(0), State(true)
	var log []string
	p := ProbeBuilder(func() Widget {
		return If(shown, func() Widget {
			Effect(func() Cleanup { n.Get(); log = append(log, "effect"); return func() { log = append(log, "cleanup") } })
			return FromFuncs(func(c Constraints, env Env) Size { log = append(log, "layout"); return c.Constrain(Size{}) }, func(*Canvas, Rect) { log = append(log, "paint") })
		})
	}, Sz(20, 20))
	p.Frame()
	if !reflect.DeepEqual(log, []string{"layout", "effect", "paint"}) {
		t.Fatalf("first frame: %v", log)
	}
	log = nil
	n.Set(1)
	n.Set(2)
	p.Frame()
	if !slices.Equal(log, []string{"layout", "cleanup", "effect", "paint"}) && !slices.Equal(log, []string{"cleanup", "effect", "paint"}) {
		t.Fatalf("update: %v", log)
	}
	log = nil
	shown.Set(false)
	p.Frame()
	if !slices.Equal(log, []string{"cleanup"}) {
		t.Fatalf("unmount: %v", log)
	}
	p.Close()
	if !slices.Equal(log, []string{"cleanup"}) {
		t.Fatal("cleanup repeated")
	}
	assertPanics(t, func() { Effect(func() Cleanup { return nil }) })
}

func TestEffectWriteSettlesBeforePaint(t *testing.T) {
	n := State(0)
	painted := -1
	p := ProbeBuilder(func() Widget {
		Effect(func() Cleanup {
			if n.Get() == 0 {
				n.Set(1)
			}
			return nil
		})
		return View(n, func(v int) Widget {
			return FromFuncs(func(c Constraints, _ Env) Size { return c.Constrain(Size{}) }, func(*Canvas, Rect) { painted = v })
		})
	}, Sz(10, 10))
	defer p.Close()
	p.Frame()
	if painted != 1 {
		t.Fatalf("painted stale value %d", painted)
	}
}

func TestEffectDisposedBeforeFirstRunAndUntrackedCleanup(t *testing.T) {
	trigger, other := State(0), State(0)
	runs := 0
	p := ProbeBuilder(func() Widget {
		stop := Effect(func() Cleanup { t.Error("disposed effect ran"); return nil })
		stop()
		stop()
		Effect(func() Cleanup { trigger.Get(); runs++; return func() { other.Get() } })
		return Box()
	}, Sz(10, 10))
	defer p.Close()
	p.Frame()
	trigger.Set(1)
	p.Frame()
	other.Set(1)
	p.Frame()
	if runs != 2 {
		t.Fatalf("cleanup subscribed: %d", runs)
	}
}

func TestIfFactoryLifetimeAndConfiguration(t *testing.T) {
	shown := State(false)
	created, cleaned := 0, 0
	var local *StateValue[int]
	branch := If(shown, func() Widget {
		created++
		local = State(0)
		OnCleanup(func() { cleaned++ })
		return Textf("%d", local)
	})
	p := NewProbe(branch, Sz(100, 30))
	defer p.Close()
	p.Frame()
	if created != 0 {
		t.Fatal("inactive branch constructed")
	}
	shown.Set(true)
	p.Frame()
	local.Set(9)
	p.Frame()
	if created != 1 {
		t.Fatal("branch rebuilt on local write")
	}
	shown.Set(false)
	p.Frame()
	shown.Set(true)
	p.Frame()
	if created != 2 || cleaned != 1 || local.Get() != 0 {
		t.Fatal("branch lifetime incorrect")
	}
	assertPanics(t, func() { branch.Else(func() Widget { return Box() }) })
}

func TestEachPositionKeyIndexAndEmptyLifetime(t *testing.T) {
	type item struct {
		ID    int
		Label string
	}
	source := State([]item{{1, "a"}, {2, "b"}})
	var rows []EachItem[item]
	created, cleaned, empty := 0, 0, 0
	keyed := EachKeyed(source, func(v item) int { return v.ID }, func(row EachItem[item]) Widget {
		rows = append(rows, row)
		created++
		OnCleanup(func() { cleaned++ })
		return Textf("%v", row.Value)
	}).Else(func() Widget { empty++; return Text("empty") })
	p := NewProbe(keyed, Sz(200, 100))
	defer p.Close()
	p.Frame()
	source.Set([]item{{2, "B"}, {1, "A"}})
	p.Frame()
	if created != 2 || rows[0].Index.Get() != 1 || rows[0].Value.Get().Label != "A" {
		t.Fatal("key/index reuse incorrect")
	}
	source.Set(nil)
	p.Frame()
	if cleaned != 2 || empty != 1 {
		t.Fatal("rows not disposed or empty not mounted")
	}
	source.Set(nil)
	p.Frame()
	if empty != 1 {
		t.Fatal("empty branch remounted")
	}
	values := State([]string{"x", "x"})
	var positional []EachItem[string]
	q := NewProbe(Each(values, func(row EachItem[string]) Widget { positional = append(positional, row); return TextOf(row.Value) }), Sz(100, 50))
	defer q.Close()
	q.Frame()
	values.Set([]string{"y"})
	q.Frame()
	if len(positional) != 2 || positional[0].Value.Get() != "y" {
		t.Fatal("positional list not reused")
	}
}

func TestDerivedEqualityAndMeasurementTracking(t *testing.T) {
	n := State(1)
	runs := 0
	p := ProbeBuilder(func() Widget {
		v := Derived(func() []int { return []int{n.Get() % 2} }).WithEqual(slices.Equal[[]int])
		return View(v, func([]int) Widget { runs++; return Box() })
	}, Sz(10, 10))
	defer p.Close()
	p.Frame()
	n.Set(3)
	p.Frame()
	if runs != 1 {
		t.Fatal("equal derived value propagated")
	}
}

func TestPublicEffectCycleReportsItsOrigin(t *testing.T) {
	n := State(0)
	p := ProbeBuilder(func() Widget {
		Effect(func() Cleanup { n.Set(n.Get() + 1); return nil })
		return Box()
	}, Sz(10, 10))
	defer p.Close()
	defer func() {
		err, ok := recover().(error)
		if !ok || !errors.Is(err, ErrCycle) {
			t.Fatalf("expected ErrCycle, got %v", err)
		}
		if effectOrigin() != "" && !strings.Contains(err.Error(), "reactive_api_test.go:") {
			t.Fatalf("missing origin: %v", err)
		}
	}()
	p.Frame()
}

func TestEffectCanCloseAppBeforePaint(t *testing.T) {
	var p *Probe
	painted := false
	cleaned := 0
	p = ProbeBuilder(func() Widget {
		Effect(func() Cleanup { p.Close(); return func() { cleaned++ } })
		return FromFuncs(func(c Constraints, _ Env) Size { return c.Constrain(Size{}) }, func(*Canvas, Rect) { painted = true })
	}, Sz(10, 10))
	p.Frame()
	if painted {
		t.Fatal("closed app painted")
	}
	if cleaned != 1 {
		t.Fatalf("cleanup after close: %d", cleaned)
	}
}

func TestEachEmptyBranchLaidOutAfterExitTransition(t *testing.T) {
	items := State([]int{1})
	cleaned := 0
	p := ProbeBuilder(func() Widget {
		return EachKeyed(items, func(n int) int { return n }, func(EachItem[int]) Widget {
			OnCleanup(func() { cleaned++ })
			return Box().Size(10, 10)
		}).Transition(func(w Widget) *TransitionWidget { return Transition(w).Fade().Duration(time.Millisecond) }).
			Else(func() Widget { return Column(Text("empty"), Text("list")) })
	}, Sz(100, 100))
	defer p.Close()
	p.Frame()
	items.Set(nil)
	p.Frame()
	p.Advance(2 * time.Millisecond)
	if cleaned != 1 {
		t.Fatalf("exit cleanup count=%d", cleaned)
	}
	if _, ok := p.Semantics().Find(RoleText, "empty"); !ok {
		t.Fatal("empty branch not painted")
	}
}
