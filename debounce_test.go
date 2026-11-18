package ggui

import (
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

func TestDebouncedStopsWithItsOwner(t *testing.T) {
	t.Parallel()
	query := State(1)
	show := State(true)
	var q Readable[int]
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
	show.Set(false)
	p.Frame()
	p.Advance(time.Second)
	if got := Peek(q); got != 1 {
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
