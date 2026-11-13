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
	if tip.placed != ggui.PopupBottom {
		t.Fatal("tooltip at top edge did not flip arrow/body below")
	}
	if tip.arrowAt < tip.theme.RadiusSm+5 {
		t.Fatalf("arrow collided with rounded corner: x=%v", tip.arrowAt)
	}
}

func TestTooltipArrowPointsDownForAbovePlacement(t *testing.T) {
	t.Parallel()
	tip := Tooltip(ggui.Box().Size(60, 20), "Tip")
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(ggui.Box().Height(100), tip)), ggui.Sz(180, 180))
	defer p.Close()
	p.Move(ggui.Pt(30, 110))
	p.Frame()
	if tip.placed != ggui.PopupTop {
		t.Fatal("room above anchor should keep tooltip above")
	}
	if tip.arrowAt <= 0 {
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

func TestTooltipSideMovesTheBodyAndTheArrow(t *testing.T) {
	t.Parallel()
	anchor := ggui.Rct(ggui.Pt(170, 140), ggui.Sz(60, 20))
	for _, tc := range []struct {
		name       string
		side       ggui.PopupSide
		horizontal bool
		check      func(r ggui.Rect) bool
	}{
		// The surface reaches the anchor: the arrow stands in the gap.
		{"top", ggui.PopupTop, false, func(r ggui.Rect) bool { return r.Origin.Y+r.Size.H == anchor.Origin.Y }},
		{"bottom", ggui.PopupBottom, false, func(r ggui.Rect) bool { return r.Origin.Y == anchor.Origin.Y+anchor.Size.H }},
		{"left", ggui.PopupLeft, true, func(r ggui.Rect) bool { return r.Origin.X+r.Size.W == anchor.Origin.X }},
		{"right", ggui.PopupRight, true, func(r ggui.Rect) bool { return r.Origin.X == anchor.Origin.X+anchor.Size.W }},
	} {
		tip := Tooltip(ggui.Box().Size(60, 20), "Tip").Side(tc.side)
		p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Center(tip)), ggui.Sz(400, 300))
		p.Move(anchor.Center())
		p.Frame()
		node, ok := p.Semantics().Find(ggui.RoleText, "Tip")
		if !ok || tip.placed != tc.side {
			t.Fatalf("%s: tip shown=%v on side %v", tc.name, ok, tip.placed)
		}
		// The arrow widens the surface along the axis it points on, and
		// points at the middle of the anchor from the middle of the body.
		surface := tip.effect.Layout(ggui.Loose(ggui.Sz(320, 300)), tip.env)
		body := tip.box.Layout(ggui.Loose(ggui.Sz(320, 300)), tip.env)
		if wide := surface.W == body.W+4; wide != tc.horizontal || (surface.H == body.H+4) == tc.horizontal {
			t.Errorf("%s: surface %v around body %v", tc.name, surface, body)
		}
		extent := pick(tc.horizontal, body.H, body.W)
		if d := tip.arrowAt - extent/2; d > .01 || d < -.01 {
			t.Errorf("%s: arrow at %v along an edge of %v", tc.name, tip.arrowAt, extent)
		}
		// The text is inside the body, which stops 4px short of the anchor.
		grown := ggui.Rct(node.Rect.Origin.Sub(ggui.Pt(tip.pad.Left, tip.pad.Top)), body)
		switch tc.side {
		case ggui.PopupBottom:
			grown.Origin.Y -= 4
		case ggui.PopupRight:
			grown.Origin.X -= 4
		}
		grown.Size = surface
		if !tc.check(grown) {
			t.Errorf("%s: surface at %v does not meet the anchor at %v", tc.name, grown, anchor)
		}
		p.Close()
	}
}

func TestTooltipShortcutSitsCloserThanAWord(t *testing.T) {
	t.Parallel()
	env := uitheme.Default().Apply(ggui.Env{})
	tip := Tooltip(ggui.Box().Size(20, 20), "Save").Shortcut("mod+s")
	tip.Layout(ggui.Loose(ggui.Sz(300, 300)), env)
	if tip.pad.Right != 6 || tip.pad.Left != 12 {
		t.Fatalf("a tip with a key hint is padded %v, want 6 after the hint and 12 before the text", tip.pad)
	}
	whole := tip.box.Layout(ggui.Loose(ggui.Sz(320, 300)), env)
	text := tip.tip.Layout(ggui.Loose(ggui.Sz(320, 300)), env)
	keys := tip.keys.Layout(ggui.Loose(ggui.Sz(320, 300)), env)
	if gap := whole.W - text.W - keys.W - tip.pad.Left - tip.pad.Right; gap != 6 {
		t.Fatalf("the key hint is %v after the text, want 6", gap)
	}
}
