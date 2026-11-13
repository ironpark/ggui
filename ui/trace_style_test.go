package ui

import (
	"image/color"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func TestTraceStatusColorsEveryVariantWithoutTintingDetails(t *testing.T) {
	t.Parallel()
	for _, theme := range []uitheme.Theme{uitheme.Default(), uitheme.Dark()} {
		for _, status := range []TraceStatus{TracePending, TraceRunning, TraceFailed, TraceError, TraceCompleted} {
			var detailsForeground color.Color
			details := ggui.FromFuncs(func(cs ggui.Constraints, env ggui.Env) ggui.Size {
				detailsForeground = uitheme.Of(env).Fg
				return cs.Constrain(ggui.Sz(100, 20))
			}, func(*ggui.Canvas, ggui.Rect) {})
			trace := Trace(
				TraceStep{ID: "plain", Title: "Plain", Status: status},
				TraceStep{ID: "action", Title: "Action", Status: status, OnActivate: func() {}},
				TraceStep{ID: "disclosure", Title: "Disclosure", Status: status, Details: details},
			).Expanded(map[string]bool{"disclosure": true})
			p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, uitheme.With(theme, ggui.Column(trace))), ggui.Sz(300, 300))
			p.Frame()
			for id, row := range trace.rows {
				want := theme.Fg
				if status == TraceFailed || status == TraceError {
					want = theme.Destructive
				}
				if status == TracePending {
					want = fade(want, .5)
				}
				if !sameColor(row.headerTheme.Fg, want) {
					t.Errorf("%s %s foreground = %v, want %v", id, status, row.headerTheme.Fg, want)
				}
				if _, ok := p.Semantics().Find(ggui.RoleStatus, row.step.Title+": "+string(status)); !ok {
					t.Errorf("%s lacks accessible status", id)
				}
			}
			if trace.rows["disclosure"].disclosure.theme != &trace.rows["disclosure"].headerTheme {
				t.Error("disclosure restored the base header theme")
			}
			if !sameColor(detailsForeground, theme.Fg) {
				t.Error("parent status leaked into detail content")
			}
			p.Close()
		}
	}
}

func TestTraceRunningPulseStopsForReducedMotionAndCompletion(t *testing.T) {
	// WindowProbe's wake assertions run without parallel probes.
	for _, reduced := range []bool{false, true} {
		for _, variant := range []string{"plain", "action", "disclosure"} {
			step := TraceStep{ID: "run", Title: "Run", Status: TraceRunning}
			if variant == "action" {
				step.OnActivate = func() {}
			}
			if variant == "disclosure" {
				step.Details = ggui.Text("Details")
			}
			steps := ggui.State([]TraceStep{step})
			trace := Trace().BindSteps(steps)
			p := ggui.NewWindowProbe(func() ggui.Widget {
				return ggui.Provide(ggui.ReducedMotionKey, reduced, ggui.Column(trace))
			}, ggui.Sz(300, 150))
			frame := p.Step(0)
			if frame.Wake.IsZero() != reduced {
				t.Errorf("%s reduced=%v wake=%v", variant, reduced, frame.Wake)
			}
			before := trace.rows["run"].headerTheme.Fg
			changed := false
			for range 3 {
				p.Step(500 * time.Millisecond)
				changed = changed || !sameColor(before, trace.rows["run"].headerTheme.Fg)
			}
			if changed == reduced {
				t.Errorf("%s reduced=%v pulse changed=%v", variant, reduced, changed)
			}
			step.Status = TraceCompleted
			steps.Set([]TraceStep{step})
			for range 3 {
				frame = p.Step(100 * time.Millisecond)
			}
			if frame.Next || !frame.Wake.IsZero() {
				t.Errorf("%s completed trace still animates: %+v", variant, frame)
			}
			p.Close()
		}
	}
}
