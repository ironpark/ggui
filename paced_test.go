package ggui

import (
	"testing"
	"time"
)

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
