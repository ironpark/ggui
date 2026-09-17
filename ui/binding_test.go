package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestDisabledWhenFollowsSignalWithoutRebuild(t *testing.T) {
	busy := ggui.State(false)
	taps := 0
	b := ui.Button("x", func() { taps++ }).DisabledWhen(busy)
	p := ggui.NewProbe(b, ggui.Sz(100, 40))
	p.Click(ggui.Pt(10, 10))
	busy.Set(true)
	p.Click(ggui.Pt(10, 10))
	if taps != 1 || !b.Inert {
		t.Fatalf("taps = %d, inert = %v; want 1 and true", taps, b.Inert)
	}
}

func TestSliderCommitsOnRelease(t *testing.T) {
	v := ggui.State(0.0)
	changes, commits := 0, 0
	s := ui.Slider(v, 0, 100).OnChange(func(float64) { changes++ }).OnCommit(func(float64) { commits++ })
	p := ggui.NewProbe(s, ggui.Sz(200, 20))
	p.Press(ggui.Pt(50, 10))
	p.Move(ggui.Pt(80, 10))
	p.Move(ggui.Pt(120, 10))
	p.Release(ggui.Pt(120, 10))
	if changes < 2 || commits != 1 {
		t.Fatalf("changes = %d, commits = %d; want several and 1", changes, commits)
	}
}

func TestControlBindsToLens(t *testing.T) {
	type prefs struct{ Dark bool }
	pv := ggui.State(prefs{})
	dark := pv.Lens(func(p prefs) bool { return p.Dark }, func(p prefs, v bool) prefs { p.Dark = v; return p })
	p := ggui.NewProbe(ui.Switch(dark, ""), ggui.Sz(100, 30))
	p.Click(ggui.Pt(5, 5))
	if !pv.Peek().Dark {
		t.Fatal("switch did not write through the lens")
	}
}
