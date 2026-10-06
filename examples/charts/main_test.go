package main

import (
	"testing"

	"github.com/ironpark/ggui"
)

// TestCharts drives the catalog headlessly: Tap finds controls by their
// accessible label, and the signals record what they did.
func TestCharts(t *testing.T) {
	t.Parallel()
	m := newModel("chart-area-default", false)
	p := ggui.ProbeBuilder(m.build, ggui.Sz(740, 600))
	defer p.Close()

	p.Tap("Replay")
	p.Tap("Replay")
	if got := ggui.Peek(m.Generation); got != 2 {
		t.Fatalf("generation after two replays = %d, want 2", got)
	}
	p.Tap("Dark theme")
	if !ggui.Peek(m.Dark) {
		t.Fatal("dark theme switch did not flip the signal")
	}
	if _, ok := p.Find("Chart example"); !ok {
		t.Fatal("chart example select is not in the tree")
	}
}
