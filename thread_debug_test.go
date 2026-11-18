//go:build ggui_debug

package ggui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// offThread runs fn on a goroutine of its own, as a worker does, and
// returns what it panicked with, or nil.
func offThread(fn func()) any {
	caught := make(chan any, 1)
	go func() {
		defer func() { caught <- recover() }()
		fn()
	}()
	return <-caught
}

func wantOffThreadPanic(t *testing.T, what string, v any) {
	t.Helper()
	if v == nil {
		t.Fatalf("%s from a worker was not reported", what)
	}
	if msg, _ := v.(string); !strings.Contains(msg, "hand the result back with App.Post") {
		t.Fatalf("%s from a worker: the panic does not say what to do instead: %v", what, v)
	}
}

// A probe's frames arm the check, for the state the test made before them
// and the state its tree made.
func TestProbeReportsAWorkerWritingState(t *testing.T) {
	t.Parallel()
	before := State(0)
	var owned *StateValue[int]
	p := ProbeBuilder(func() Widget { return sized(0, 0) }, Sz(10, 10)).Setup(func() {
		owned = State(0)
	})
	defer p.Close()
	p.Frame()
	wantOffThreadPanic(t, "a Set of state made before the probe", offThread(func() { before.Set(1) }))
	wantOffThreadPanic(t, "a Set of state the tree made", offThread(func() { owned.Set(1) }))
	wantOffThreadPanic(t, "a Get", offThread(func() { Peek[int](owned) }))
	if Peek[int](before) != 0 || Peek[int](owned) != 0 {
		t.Fatal("a reported write landed")
	}
}

func TestWindowProbeReportsAWorkerWritingState(t *testing.T) {
	t.Parallel()
	s := State(0)
	p := NewWindowProbe(func() Widget { return sized(10, 10) }, Sz(20, 20))
	defer p.Close()
	p.Step(0)
	wantOffThreadPanic(t, "a Set", offThread(func() { s.Set(1) }))
}

// A worker that hands its result back with Post, as Async and Resource
// do, writes on the probe's goroutine, and nothing is reported.
func TestProbeLetsAWorkerPostItsResult(t *testing.T) {
	t.Parallel()
	query := State("a")
	got := State("")
	var res *ResourceValue[string]
	p := ProbeBuilder(func() Widget { return sized(0, 0) }, Sz(10, 10)).Setup(func() {
		res = Resource(query.Get, func(_ context.Context, q string) (string, error) {
			return strings.ToUpper(q), nil
		})
	})
	defer p.Close()
	p.Frame()
	if v := offThread(func() {
		// The work happens here; the write is posted.
		v := "done"
		p.Post(func() { got.Set(v) })
	}); v != nil {
		t.Fatalf("a worker posting was reported: %v", v)
	}
	p.Frame()
	if Peek[string](got) != "done" {
		t.Fatal("the posted write did not land")
	}
	deadline := time.Now().Add(5 * time.Second)
	for Peek[AsyncState[string]](res).Status != Ready {
		if time.Now().After(deadline) {
			t.Fatal("the resource never came back")
		}
		time.Sleep(time.Millisecond)
		p.Frame()
	}
	if v := Peek[AsyncState[string]](res).Value; v != "A" {
		t.Fatalf("resource = %q", v)
	}
}

// Probes running beside one another each check against their own
// goroutine, so none trips another, whatever each writes between and in
// its frames.
func TestParallelProbesDoNotTripEachOther(t *testing.T) {
	t.Parallel()
	for i := range 4 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			n := State(0)
			var doubled *DerivedValue[int]
			p := ProbeBuilder(func() Widget { return sized(0, 0) }, Sz(10, 10)).Setup(func() {
				doubled = Derived(func() int { return 2 * n.Get() })
				Effect(func() Cleanup { n.Get(); return nil })
			})
			defer p.Close()
			for k := range 200 {
				n.Set(k)
				p.Post(func() { Add[int](n, 0) })
				p.Frame()
				if got := Peek[int](doubled); got != 2*k {
					t.Fatalf("doubled = %d, want %d", got, 2*k)
				}
			}
		})
	}
}

// What a test makes and writes before its probe's first frame, or between
// one probe's close and the next one's frames, is the test goroutine's,
// and so is the probe's goroutine.
func TestStateMadeBeforeTheFirstFrameIsTheTestGoroutines(t *testing.T) {
	t.Parallel()
	s := State(1)
	s.Set(2)
	p := ProbeBuilder(func() Widget { return sized(0, 0) }, Sz(10, 10)).Setup(func() {
		s.Set(3)
	})
	s.Set(4)
	p.Frame()
	s.Set(5)
	p.Close()

	// With no frame left to race, a goroutine may hand the state on.
	if v := offThread(func() { s.Set(6) }); v != nil {
		t.Fatalf("a write after the probe closed was reported: %v", v)
	}

	q := NewProbe(sized(0, 0), Sz(10, 10))
	defer q.Close()
	s.Set(7)
	q.Frame()
	s.Set(8)
	wantOffThreadPanic(t, "a Set once a new probe ran", offThread(func() { s.Set(9) }))
}
