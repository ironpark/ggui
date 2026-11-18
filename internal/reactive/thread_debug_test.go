//go:build ggui_debug

package reactive

import (
	"strings"
	"testing"
)

// offThread runs fn on a goroutine of its own and returns what it panicked
// with, or nil.
func offThread(fn func()) any {
	caught := make(chan any, 1)
	go func() {
		defer func() { caught <- recover() }()
		fn()
	}()
	return <-caught
}

func TestOffThreadWriteIsReported(t *testing.T) {
	rt := NewRuntime()
	MarkUIThread(rt)
	t.Cleanup(func() { UnmarkUIThread(rt) })

	s := State(0) // made before the frame, in the goroutine's Base
	var inTree *StateValue[int]
	dispose := rt.Root(func() { inTree = State(0) })
	defer dispose()
	s.Set(1) // the UI goroutine: nothing to report
	if Untrack(s.Get) != 1 {
		t.Fatalf("a write on the UI goroutine did not land")
	}

	for _, v := range []*StateValue[int]{s, inTree} {
		got := offThread(func() { v.Set(2) })
		if got == nil {
			t.Fatal("a Set from another goroutine was not reported")
		}
		if msg, _ := got.(string); !strings.Contains(msg, "App.Post") {
			t.Fatalf("the panic does not say what to do instead: %v", got)
		}
	}
	if offThread(func() { Untrack(s.Get) }) == nil {
		t.Fatal("a Get from another goroutine was not reported")
	}
	// What the worker makes for itself is its own.
	if v := offThread(func() { State(0).Set(1) }); v != nil {
		t.Fatalf("a worker's own state was reported: %v", v)
	}
}

func TestWriteBeforeAnyFrameIsAllowed(t *testing.T) {
	s := State(0)
	if v := offThread(func() { s.Set(1) }); v != nil {
		t.Fatalf("a write before the first frame was reported: %v", v)
	}
}

func TestUnmarkedRuntimeIsNotChecked(t *testing.T) {
	rt := NewRuntime()
	s := State(0)
	MarkUIThread(rt)
	UnmarkUIThread(rt)
	if v := offThread(func() { s.Set(1) }); v != nil {
		t.Fatalf("a write after the frames ended was reported: %v", v)
	}
}

func TestRuntimesAreCheckedApart(t *testing.T) {
	// Another goroutine's runtime, marked by its own frames, does not
	// make this goroutine's state its own.
	var theirs *StateValue[int]
	done := make(chan *Runtime)
	go func() {
		rt := NewRuntime()
		MarkUIThread(rt)
		rt.Root(func() { theirs = State(0) })
		done <- rt
	}()
	rt := <-done
	defer UnmarkUIThread(rt)
	mine := State(0)
	mine.Set(1)
	if v := offThread(func() { theirs.Set(1) }); v == nil {
		t.Fatal("a write to another runtime's state was not reported")
	}
}
