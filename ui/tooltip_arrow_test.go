package ui

import (
	"testing"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func TestTooltipArrowReservesGapAndFollowsCollision(t *testing.T) {
	t.Parallel()
	env := uitheme.Default().Apply(ggui.Env{})
	tip := Tooltip(ggui.Box().Size(20, 20), "Tooltip content")
	tip.Layout(ggui.Loose(ggui.Sz(300, 300)), env)
	with := tip.effect.Layout(ggui.Loose(ggui.Sz(320, 300)), env)
	tip.Arrow(false)
	without := tip.effect.Layout(ggui.Loose(ggui.Sz(320, 300)), env)
	if with.W != without.W || with.H != without.H+4 {
		t.Fatalf("arrow changed body geometry: with=%v without=%v", with, without)
	}
	tip.Arrow(true)
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(tip)), ggui.Sz(180, 180))
	defer p.Close()
	p.Move(ggui.Pt(10, 10))
	p.Frame()
	if !tip.below {
		t.Fatal("tooltip at top edge did not flip arrow/body below")
	}
	if tip.arrowX < tip.theme.RadiusSm+5 {
		t.Fatalf("arrow collided with rounded corner: x=%v", tip.arrowX)
	}
}

func TestTooltipArrowPointsDownForAbovePlacement(t *testing.T) {
	t.Parallel()
	tip := Tooltip(ggui.Box().Size(60, 20), "Tip")
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(ggui.Box().Height(100), tip)), ggui.Sz(180, 180))
	defer p.Close()
	p.Move(ggui.Pt(30, 110))
	p.Frame()
	if tip.below {
		t.Fatal("room above anchor should keep tooltip above")
	}
	if tip.arrowX <= 0 {
		t.Fatal("arrow position was not calculated")
	}
}

func TestTooltipEscapeAndDisabledKeepChildInteractive(t *testing.T) {
	t.Parallel()
	calls := 0
	disabled := ggui.State(false)
	tip := Tooltip(Button("Target", func() { calls++ }), "Help").BindDisabled(disabled)
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(tip)), ggui.Sz(180, 180))
	defer p.Close()
	p.Move(ggui.Pt(20, 10))
	p.Key("escape")
	p.Frame()
	if !tip.escaped {
		t.Fatal("Escape dismissal not retained")
	}
	p.Move(ggui.Pt(175, 175))
	p.Frame()
	if tip.escaped {
		t.Fatal("hover exit did not reset dismissal")
	}
	disabled.Set(true)
	p.Frame()
	p.Tap("Target")
	if calls != 1 || !tip.disabled {
		t.Fatal("disabled tooltip intercepted child")
	}
}
