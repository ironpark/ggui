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
				// A line is muted until the pointer is over it, and a failed
				// one destructive whether it is or not.
				want := theme.MutedFg
				if status == TraceFailed || status == TraceError {
					want = theme.Destructive
				}
				if status == TracePending {
					want = fade(want, .6)
				}
				if !sameColor(row.ink, want) {
					t.Errorf("%s %s foreground = %v, want %v", id, status, row.ink, want)
				}
				if _, ok := p.Semantics().Find(ggui.RoleStatus, row.step.Title+": "+string(status)); !ok {
					t.Errorf("%s lacks accessible status", id)
				}
			}
			if !sameColor(detailsForeground, theme.Fg) {
				t.Error("parent status leaked into detail content")
			}
			p.Close()
		}
	}
}

// A line that does something turns to the foreground under the pointer; a
// plain one, and a failed one, keep their color.
func TestTraceLineTurnsForegroundUnderThePointer(t *testing.T) {
	t.Parallel()
	theme := uitheme.Default()
	trace := Trace(
		TraceStep{ID: "plain", Title: "Plain"},
		TraceStep{ID: "open", Title: "Open", Details: ggui.Text("Details")},
		TraceStep{ID: "failed", Title: "Failed", Status: TraceFailed, OnActivate: func() {}},
	)
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, uitheme.With(theme, ggui.Column(trace))), ggui.Sz(300, 300))
	p.Frame()
	defer p.Close()
	for id, want := range map[string]color.Color{"plain": theme.MutedFg, "open": theme.Fg, "failed": theme.Destructive} {
		row := trace.rows[id]
		p.Move(row.header.Origin.Add(ggui.Pt(4, 4)))
		p.Frame()
		if !sameColor(row.ink, want) {
			t.Errorf("%s under the pointer = %v, want %v", id, row.ink, want)
		}
	}
}

// The line shows the description and the duration beside the title, never
// the status word, and is no wider than what it shows.
func TestTraceLineShowsDescriptionAndDurationNotStatus(t *testing.T) {
	t.Parallel()
	trace := Trace(
		TraceStep{ID: "a", Title: "Search", Description: "12 results", Status: TraceCompleted, Duration: 1500 * time.Millisecond, Details: ggui.Text("Details")},
		TraceStep{ID: "b", Title: "Plan", Status: TraceRunning},
	)
	p := ggui.NewProbe(ggui.Column(trace), ggui.Sz(400, 300))
	defer p.Close()
	p.Frame()
	a, b := trace.rows["a"], trace.rows["b"]
	if a.shown != "12 results · 1.5s" || b.shown != "" {
		t.Fatalf("runs = %q, %q", a.shown, b.shown)
	}
	if a.line.W >= 300 || a.line.H > 24 {
		t.Fatalf("line = %v, want a bare line as wide as its text", a.line)
	}
	// The chevron follows the text instead of leading it.
	if a.line.W <= a.titleSize.W+a.runSize.W+a.gap || a.chevron == 0 || b.chevron != 0 {
		t.Fatalf("chevron box = %v and %v in a line of %v", a.chevron, b.chevron, a.line)
	}
}

func TestTraceRunningShimmerStopsForReducedMotionAndCompletion(t *testing.T) {
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
			for range 3 {
				p.Step(500 * time.Millisecond)
			}
			if trace.rows["run"].shimmering == reduced {
				t.Errorf("%s reduced=%v shimmering=%v", variant, reduced, trace.rows["run"].shimmering)
			}
			step.Status = TraceCompleted
			steps.Set([]TraceStep{step})
			for range 3 {
				frame = p.Step(100 * time.Millisecond)
			}
			if frame.Next || !frame.Wake.IsZero() || trace.rows["run"].shimmering {
				t.Errorf("%s completed trace still animates: %+v", variant, frame)
			}
			p.Close()
		}
	}
}
