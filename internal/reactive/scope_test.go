package reactive

import (
	"testing"

	"github.com/ironpark/ggui/internal/goid"
)

// A layout recorder sees every read, tracked or not, and a nested Measure
// sees only its own and then hands the outer recorder back.
func TestMeasureRecordsReadsAndNests(t *testing.T) {
	t.Parallel()
	a, b := State(1), State(2)
	var outer, inner []LayoutSource
	if Recording() || Recorder() != nil {
		t.Fatal("a recorder is installed before any Measure")
	}
	Record(a, 0) // with no recorder: nothing to report to
	Measure(func(src LayoutSource, _ uint64) { outer = append(outer, src) }, func() {
		if !Recording() || Recorder() == nil {
			t.Error("Measure did not install its recorder")
		}
		a.Get()
		Measure(func(src LayoutSource, _ uint64) { inner = append(inner, src) }, func() {
			Untrack(b.Get)
		})
		Record(b, 7)
	})
	if Recording() {
		t.Fatal("the recorder outlived Measure")
	}
	if len(outer) != 2 || outer[0] != a || outer[1] != b {
		t.Fatalf("the outer layout recorded %v, want a then b", outer)
	}
	if len(inner) != 1 || inner[0] != b {
		t.Fatalf("the inner layout recorded %v, want the untracked read of b", inner)
	}
}

func TestMeasureRestoresTheRecorderAfterAPanic(t *testing.T) {
	t.Parallel()
	panicOf(func() {
		Measure(func(LayoutSource, uint64) {}, func() { panic("layout failed") })
	})
	if Recording() {
		t.Fatal("a panicking layout left its recorder installed")
	}
}

func TestStateVersionAdvancesOnlyOnChange(t *testing.T) {
	t.Parallel()
	s := State("x")
	v := s.LayoutVersion()
	s.Set("x")
	if s.LayoutVersion() != v {
		t.Fatal("an equal write advanced the layout version")
	}
	s.Set("y")
	if s.LayoutVersion() != v+1 {
		t.Fatalf("a write moved the version from %d to %d, want one step", v, s.LayoutVersion())
	}
}

func TestLayoutDepthNests(t *testing.T) {
	t.Parallel()
	if InLayout() {
		t.Fatal("in layout before entering it")
	}
	outer := EnterLayout()
	inner := EnterLayout()
	inner()
	if !InLayout() {
		t.Fatal("leaving the inner layout left the outer one too")
	}
	outer()
	if InLayout() {
		t.Fatal("still in layout after leaving both")
	}
}

// Layout and state generations are the running goroutine's: a write or a
// layout request made on another goroutine moves neither.
func TestGenerationsBelongToTheirGoroutine(t *testing.T) {
	skipSharedGoroutines(t)
	t.Parallel()
	state, layout := StateGen(), LayoutGen()
	RequestLayout()
	if LayoutGen() == layout || StateGen() != state {
		t.Fatal("RequestLayout must advance the layout generation and only that")
	}
	layout = LayoutGen()
	done := make(chan struct{})
	go func() {
		State(0).Set(1)
		RequestLayout()
		close(done)
	}()
	<-done
	if LayoutGen() != layout || StateGen() != state {
		t.Fatal("another goroutine's write moved this goroutine's generations")
	}
}

func TestRunningLoopIsRememberedUntilCleared(t *testing.T) {
	skipSharedGoroutines(t)
	t.Parallel()
	a, b := new(int), new(int)
	if RunningLoop() != nil {
		t.Fatal("a fresh goroutine has a running loop")
	}
	SetRunningLoop(a)
	ClearRunningLoop(b)
	if RunningLoop() != a {
		t.Fatal("clearing another loop forgot the running one")
	}
	done := make(chan any)
	go func() { done <- RunningLoop() }()
	if <-done != nil {
		t.Fatal("the running loop leaked to another goroutine")
	}
	ClearRunningLoop(a)
	if RunningLoop() != nil {
		t.Fatal("ClearRunningLoop kept the loop")
	}
}

// skipSharedGoroutines skips a test that tells goroutines apart where they
// all share one scope, as in WebAssembly.
func skipSharedGoroutines(t *testing.T) {
	t.Helper()
	if goid.Shared() {
		t.Skip("every goroutine shares one scope here")
	}
}
