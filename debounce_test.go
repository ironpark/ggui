package ggui

import (
	"slices"
	"testing"
	"time"
)

func TestDebouncedWaitsForAPauseByTheProbeClock(t *testing.T) {
	t.Parallel()
	query := State("a")
	var q Readable[string]
	p := ProbeBuilder(func() Widget { return sized(0, 0) }, Sz(10, 10)).Setup(func() {
		q = Debounced(query, 250*time.Millisecond)
	})
	defer p.Close()
	p.Frame()
	if got := Peek(q); got != "a" {
		t.Fatalf("first value = %q, want it at once", got)
	}
	// Each keystroke is a frame of its own, as typing into a probe is.
	query.Set("ab")
	p.Frame()
	p.Advance(200 * time.Millisecond)
	query.Set("abc")
	p.Frame()
	p.Advance(200 * time.Millisecond)
	if got := Peek(q); got != "a" {
		t.Fatalf("value while typing = %q, want the first", got)
	}
	p.Advance(60 * time.Millisecond)
	if got := Peek(q); got != "abc" {
		t.Fatalf("value after the pause = %q, want abc", got)
	}
}

func TestDebouncedIsPendingUntilItsWaitEnds(t *testing.T) {
	t.Parallel()
	query := State("a")
	var q *DebouncedValue[string]
	var runs int
	p := ProbeBuilder(func() Widget { return sized(0, 0) }, Sz(10, 10)).Setup(func() {
		q = Debounced(query, 250*time.Millisecond)
		Watch[string](q, func(string) { runs++ })
	})
	defer p.Close()
	p.Frame()
	if Untrack(q.Pending) {
		t.Fatal("pending before any change")
	}
	runs = 0

	// Typing that comes back to where it started: pending from the first
	// change until the wait ends, and nothing new comes out.
	query.Set("ab")
	p.Frame()
	if !Untrack(q.Pending) {
		t.Fatal("not pending after a change")
	}
	p.Advance(100 * time.Millisecond)
	query.Set("a")
	p.Frame()
	p.Advance(200 * time.Millisecond)
	if !Untrack(q.Pending) {
		t.Fatal("not pending before the wait ends")
	}
	p.Advance(60 * time.Millisecond)
	if Untrack(q.Pending) {
		t.Fatal("still pending after the wait ended")
	}
	if got := Peek[string](q); got != "a" || runs != 0 {
		t.Fatalf("value = %q after %d runs, want a unchanged", got, runs)
	}

	// A change that sticks: the value comes out as pending ends.
	query.Set("abc")
	p.Frame()
	p.Advance(250 * time.Millisecond)
	if Untrack(q.Pending) || Peek[string](q) != "abc" || runs != 1 {
		t.Fatalf("pending %v, value %q, %d runs; want abc once, nothing pending", Untrack(q.Pending), Peek[string](q), runs)
	}
}

func TestDebouncedPendingTracks(t *testing.T) {
	t.Parallel()
	query := State(0)
	var seen []bool
	p := ProbeBuilder(func() Widget { return sized(0, 0) }, Sz(10, 10)).Setup(func() {
		q := Debounced(query, 100*time.Millisecond)
		Effect(func() Cleanup { seen = append(seen, q.Pending()); return nil })
	})
	defer p.Close()
	p.Frame()
	query.Set(1)
	p.Frame()
	p.Advance(100 * time.Millisecond)
	if want := []bool{false, true, false}; !slices.Equal(seen, want) {
		t.Fatalf("an effect reading Pending saw %v, want %v", seen, want)
	}
}

func TestDebouncedStopsWithItsOwner(t *testing.T) {
	t.Parallel()
	query := State(1)
	show := State(true)
	var q *DebouncedValue[int]
	p := ProbeBuilder(func() Widget {
		return If(show, func() Widget {
			return Component(func() Widget {
				q = Debounced(query, 100*time.Millisecond)
				return sized(0, 0)
			})
		})
	}, Sz(10, 10))
	defer p.Close()
	p.Frame()
	query.Set(2)
	p.Frame()
	if !Untrack(q.Pending) {
		t.Fatal("not pending after a change")
	}
	show.Set(false)
	p.Frame()
	if Untrack(q.Pending) {
		t.Fatal("a disposed Debounced is still pending")
	}
	p.Advance(time.Second)
	if got := Peek[int](q); got != 1 {
		t.Fatalf("disposed Debounced took %d", got)
	}
}

func TestDebouncedPostsWhenNoFrameComes(t *testing.T) {
	t.Parallel()
	query := State(1)
	var q Readable[int]
	p := ProbeBuilder(func() Widget { return sized(0, 0) }, Sz(10, 10)).Setup(func() {
		q = Debounced(query, 10*time.Millisecond)
	})
	defer p.Close()
	p.Frame()
	query.Set(2)
	p.Frame()
	// With the probe's clock on real time, the process's timer posts the
	// check, and the frame that runs it finds the time up.
	deadline := time.Now().Add(5 * time.Second)
	for !p.hasPosted() {
		if time.Now().After(deadline) {
			t.Fatal("no check was posted")
		}
		time.Sleep(time.Millisecond)
	}
	p.Frame()
	if got := Peek(q); got != 2 {
		t.Fatalf("value = %d, want 2", got)
	}
}
