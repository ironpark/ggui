//go:build ggui_debug

package ggui

import (
	"strings"
	"testing"
)

func TestOffThreadWriteIsReported(t *testing.T) {
	markUIThread()
	t.Cleanup(unmarkUIThread)

	s := State(0)
	s.Set(1) // the UI goroutine: nothing to report
	if s.Peek() != 1 {
		t.Fatalf("a write on the UI goroutine did not land")
	}

	caught := make(chan any, 1)
	go func() {
		defer func() { caught <- recover() }()
		s.Set(2)
	}()
	v := <-caught
	if v == nil {
		t.Fatal("a Set from another goroutine was not reported")
	}
	if msg, _ := v.(string); !strings.Contains(msg, "App.Post") {
		t.Fatalf("the panic does not say what to do instead: %v", v)
	}
}

func TestWriteBeforeAnyFrameIsAllowed(t *testing.T) {
	unmarkUIThread()
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		State(0).Set(1)
	}()
	if v := <-done; v != nil {
		t.Fatalf("a write before the first frame was reported: %v", v)
	}
}
