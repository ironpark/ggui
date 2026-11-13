package ui_test

import (
	"math"
	"slices"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func rangeProbe(t *testing.T, w ggui.Widget) *ggui.Probe {
	t.Helper()
	p := ggui.NewProbe(w, ggui.Sz(224, 40))
	t.Cleanup(p.Close)
	p.Frame()
	return p
}

func rangePoint(v float64) ggui.Point { return ggui.Pt(6+2.12*v, 18) }

func TestRangeSliderThumbsHaveIndependentFocusAndBounds(t *testing.T) {
	t.Parallel()
	values := ggui.State([]float64{20, 50, 80})
	s := ui.RangeSlider(values, 0, 100).Name("Band").ThumbNames("Low", "Mid", "High").Step(5).MinGap(10)
	p := rangeProbe(t, s)
	nodes := rangeNodes(p)
	if len(nodes) != 3 {
		t.Fatalf("got %d thumbs: %s", len(nodes), p.Semantics())
	}
	for i, want := range []struct{ lo, hi, now float64 }{{0, 40, 20}, {30, 70, 50}, {60, 100, 80}} {
		if n := nodes[i]; n.Min != want.lo || n.Max != want.hi || n.Now != want.now {
			t.Fatalf("thumb %d: %+v", i, n)
		}
	}
	p.Key("tab", "right")
	if !slices.Equal(ggui.Peek(values), []float64{25, 50, 80}) {
		t.Fatalf("first thumb: %v", ggui.Peek(values))
	}
	p.Key("tab", "end")
	if n, ok := p.Semantics().Focused(); !ok || n.Name != "Mid" {
		t.Fatalf("focus = %+v", n)
	}
	if !slices.Equal(ggui.Peek(values), []float64{25, 70, 80}) {
		t.Fatalf("second thumb crossed: %v", ggui.Peek(values))
	}
	p.Key("tab", "home")
	if !slices.Equal(ggui.Peek(values), []float64{25, 70, 80}) {
		t.Fatalf("third thumb crossed: %v", ggui.Peek(values))
	}
	p.Key("shift+tab", "home")
	if !slices.Equal(ggui.Peek(values), []float64{25, 35, 80}) {
		t.Fatalf("reverse tab or Home: %v", ggui.Peek(values))
	}
}

func TestRangeSliderTrackSelectsNearestAndCommitsOnceOutside(t *testing.T) {
	t.Parallel()
	values := ggui.State([]float64{20, 80})
	changes, commits := 0, 0
	s := ui.RangeSlider(values, 0, 100).Step(5).MinGap(15).Name("Range").OnChange(func(v []float64) { changes++; v[0] = -100 }).OnCommit(func(v []float64) { commits++; v[1] = -100 })
	p := rangeProbe(t, s)
	p.Press(rangePoint(65))
	p.Move(rangePoint(10))
	if !slices.Equal(ggui.Peek(values), []float64{20, 35}) || commits != 0 {
		t.Fatalf("drag = %v, commits %d", ggui.Peek(values), commits)
	}
	p.Release(rangePoint(150))
	if !slices.Equal(ggui.Peek(values), []float64{20, 100}) || commits != 1 || changes != 3 {
		t.Fatalf("release=%v changes=%d commits=%d", ggui.Peek(values), changes, commits)
	}
	if n, ok := p.Semantics().Focused(); !ok || n.Name != "Range 2" {
		t.Fatalf("track focus=%+v", n)
	}
}

func TestRangeSliderThumbDragSurvivesRebuild(t *testing.T) {
	t.Parallel()
	values := ggui.State([]float64{20, 80})
	commits := 0
	p := rangeProbe(t, ggui.Reactive(func() ggui.Widget {
		values.Get()
		return ui.RangeSlider(values, 0, 100).Step(5).MinGap(10).OnCommit(func([]float64) { commits++ })
	}))
	p.Press(rangePoint(20))
	p.Move(rangePoint(35))
	p.Move(rangePoint(60))
	p.Release(rangePoint(90))
	if !slices.Equal(ggui.Peek(values), []float64{70, 80}) || commits != 1 {
		t.Fatalf("rebuild drag=%v commits=%d", ggui.Peek(values), commits)
	}
}

func TestRangeSliderControlledBindingCanRejectEdits(t *testing.T) {
	t.Parallel()
	stored := []float64{20, 80}
	var requested, committed []float64
	s := ui.RangeSlider(ggui.Controlled(stored, func(v []float64) { requested = v }), 0, 100).ThumbNames("Low", "High").Step(5).OnCommit(func(v []float64) { committed = v })
	p := rangeProbe(t, s)
	act(t, p, ggui.RoleSlider, "Low", ggui.Action{Kind: ggui.ActionSetValue, Num: 43})
	if !slices.Equal(requested, []float64{45, 80}) || !slices.Equal(committed, stored) {
		t.Fatalf("requested=%v committed=%v", requested, committed)
	}
	if n, _ := p.Semantics().Find(ggui.RoleSlider, "Low"); n.Now != 20 {
		t.Fatalf("rejected edit painted %v", n.Now)
	}
	requested[1] = -10
	committed[0] = -10
	if !slices.Equal(stored, []float64{20, 80}) {
		t.Fatalf("callback aliased source: %v", stored)
	}
}

func TestRangeSliderDisabledEmptyAndChangingThumbCount(t *testing.T) {
	t.Parallel()
	values := ggui.State([]float64{})
	disabled := ggui.State(false)
	s := ui.RangeSlider(values, 0, 100).BindDisabled(disabled)
	p := rangeProbe(t, s)
	p.Click(rangePoint(50))
	if len(rangeNodes(p)) != 0 {
		t.Fatal("empty range has thumbs")
	}
	values.Set([]float64{20, 80})
	disabled.Set(true)
	p.Click(rangePoint(50))
	act(t, p, ggui.RoleSlider, "Value 1", ggui.Action{Kind: ggui.ActionIncrement})
	if !slices.Equal(ggui.Peek(values), []float64{20, 80}) {
		t.Fatal("disabled edit accepted")
	}
	for _, n := range rangeNodes(p) {
		if !n.Disabled {
			t.Fatal("enabled semantics")
		}
	}
	disabled.Set(false)
	values.Set([]float64{40})
	p.Key("tab", "right")
	if got := ggui.Peek(values); len(got) != 1 || got[0] != 41 {
		t.Fatalf("new count edit: %v", got)
	}
}

func TestRangeSliderImpossibleGapAndInvalidExternalValues(t *testing.T) {
	t.Parallel()
	values := ggui.State([]float64{math.NaN(), -5, math.Inf(1)})
	p := rangeProbe(t, ui.RangeSlider(values, 100, 0).MinGap(90))
	nodes := rangeNodes(p)
	for i, n := range nodes {
		if n.Now != float64(i)*50 {
			t.Fatalf("normalized thumb %d=%v", i, n.Now)
		}
	}
	if !math.IsNaN(ggui.Peek(values)[0]) {
		t.Fatal("display wrote source")
	}
	act(t, p, ggui.RoleSlider, "Value 2", ggui.Action{Kind: ggui.ActionSetValue, Num: 90})
	if !slices.Equal(ggui.Peek(values), []float64{0, 50, 100}) {
		t.Fatalf("gap infeasible: %v", ggui.Peek(values))
	}
}

func TestRangeSliderStepAndNonMultipleGap(t *testing.T) {
	t.Parallel()
	values := ggui.State([]float64{20, 50})
	p := rangeProbe(t, ui.RangeSlider(values, 0, 100).Step(10).MinGap(15))
	act(t, p, ggui.RoleSlider, "Value 1", ggui.Action{Kind: ggui.ActionSetValue, Num: 50})
	if !slices.Equal(ggui.Peek(values), []float64{30, 50}) {
		t.Fatalf("gap or step: %v", ggui.Peek(values))
	}
	act(t, p, ggui.RoleSlider, "Value 2", ggui.Action{Kind: ggui.ActionSetValue, Num: -100})
	if !slices.Equal(ggui.Peek(values), []float64{30, 50}) {
		t.Fatalf("gap or step: %v", ggui.Peek(values))
	}
}

func rangeNodes(p *ggui.Probe) []ggui.SemNode {
	var nodes []ggui.SemNode
	for _, n := range p.Semantics().Nodes(ggui.RoleSlider) {
		nodes = append(nodes, n)
	}
	return nodes
}
