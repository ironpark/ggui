package ui

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

// Picking a tab fades its surface in where it stands while the one left
// fades out, as shadcn's triggers do under transition-all; no surface
// travels between the two. A line strip's bar does the same.
func TestTabsCrossfadeTheShownSegmentInPlace(t *testing.T) {
	t.Parallel()
	for _, line := range []bool{false, true} {
		selected := ggui.State(0)
		tabs := Tabs(selected, Tab("One", ggui.Text("1")), Tab("Two", ggui.Text("2")), Tab("Three", ggui.Text("3")))
		if line {
			tabs.Line()
		}
		p := ggui.NewProbe(ggui.Column(tabs), ggui.Sz(320, 200))
		p.Advance(0)
		p.Frame()
		if got := tabs.shown; len(got) != 3 || got[0] != 1 || got[1] != 0 || got[2] != 0 {
			t.Fatalf("line=%v: at rest the labels are shown %v, want [1 0 0]", line, got)
		}
		p.Tap("Three")
		p.Frame()
		p.Advance(tabs.theme.MotionFast / 2)
		p.Frame()
		if a, b, c := tabs.shown[0], tabs.shown[1], tabs.shown[2]; !(a > 0 && a < 1) || b != 0 || !(c > 0 && c < 1) {
			t.Fatalf("line=%v: halfway the labels are shown %v, want the first leaving, the last arriving and the middle untouched", line, tabs.shown)
		}
		p.Advance(time.Second)
		p.Frame()
		if got := tabs.shown; got[0] != 0 || got[1] != 0 || got[2] != 1 {
			t.Fatalf("line=%v: settled the labels are shown %v, want [0 0 1]", line, got)
		}
		p.Close()
	}
}

// A label's color follows how far it is into being the shown one, so it
// eases with its surface rather than switching ahead of it.
func TestTabLabelColorEasesTowardTheTextColor(t *testing.T) {
	t.Parallel()
	tabs := Tabs(ggui.State(0), Tab("One", nil), Tab("Two", nil))
	p := ggui.NewProbe(ggui.Column(tabs), ggui.Sz(320, 200))
	defer p.Close()
	p.Frame()
	th := tabs.theme
	rest := fade(th.Fg, .6)
	if got := tabs.labelColor(1, 0); !sameColor(got, rest) {
		t.Fatalf("resting label %v, want %v", got, rest)
	}
	if got := tabs.labelColor(1, 1); !sameColor(got, th.Fg) {
		t.Fatalf("shown label %v, want %v", got, th.Fg)
	}
	if got := tabs.labelColor(1, .5); !sameColor(got, mix(rest, th.Fg, .5)) {
		t.Fatalf("label halfway %v, want %v", got, mix(rest, th.Fg, .5))
	}
}
