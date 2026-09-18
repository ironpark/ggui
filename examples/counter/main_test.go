package main

import (
	"testing"

	"github.com/ironpark/ggui"
)

// TestCounter drives the app headlessly: Tap finds buttons by their
// label, Type sends keys through the same shortcut table as the window.
func TestCounter(t *testing.T) {
	m := newModel()
	p := ggui.ProbeBuilder(m.build, ggui.Sz(480, 320))
	defer p.Close()
	m.shortcuts(p)

	p.Tap("-")
	p.Tap("+")
	p.Tap("+")
	if got := ggui.Untrack(m.Count.Get); got != 1 {
		t.Fatalf("count after taps = %d, want 1", got)
	}

	// The last tap left "+" focused, so Space presses that button rather
	// than the bare-key shortcut; either way it adds the step.
	p.Type(ggui.Mods{}, ggui.KeyArrowUp, ggui.KeyArrowUp) // step 3
	p.Type(ggui.Mods{}, ggui.KeySpace)
	if got := ggui.Untrack(m.Count.Get); got != 4 {
		t.Fatalf("count after space with step 3 = %d, want 4", got)
	}

	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyArrowDown, ggui.KeyArrowDown) // clamps at 1
	if got := ggui.Untrack(m.Step.Get); got != 1 {
		t.Fatalf("step = %d, want 1", got)
	}
}
