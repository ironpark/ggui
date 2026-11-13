package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// Right to left, Left is the arrow that goes on to the next option and
// Right the one that goes back, as Base UI's CompositeRoot swaps them; Up
// and Down keep their sense, and Home and End are not the group's keys:
// RadioGroup passes enableHomeAndEndKeys={false}.
func TestRadiosArrowsFollowTheReadingDirection(t *testing.T) {
	t.Parallel()
	plan := ggui.State("free")
	root := ggui.Directionality(ggui.DirRTL, ggui.Column(
		ui.Radios(plan).Options([]string{"free", "pro", "team"}).Name("Plan"),
	))
	p := ggui.NewProbe(root, ggui.Sz(300, 300))
	defer p.Close()
	p.Key("tab")
	for _, step := range []struct{ key, want string }{
		{"left", "pro"},
		{"left", "team"},
		{"left", "free"}, // around the end
		{"right", "team"},
		{"up", "pro"},
		{"down", "team"},
		{"home", "team"},
		{"end", "team"},
	} {
		p.Key(step.key)
		p.Frame()
		if plan.Get() != step.want || focusedName(t, p) != step.want {
			t.Fatalf("%s: value=%q focus=%q, want %q", step.key, plan.Get(), focusedName(t, p), step.want)
		}
	}
}
