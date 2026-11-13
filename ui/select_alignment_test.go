package ui_test

import (
	"math"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestSelectAlignsSelectedRowWithTrigger(t *testing.T) {
	t.Parallel()
	selectBox := ui.Select(ggui.State("Beta")).Options([]string{"Alpha", "Beta", "Gamma"}).Name("Choice").MinWidth(180)
	p := ggui.NewProbe(ggui.Padding(ggui.Column(ggui.Box().Height(100), selectBox), 0, 50), ggui.Sz(400, 350))
	defer p.Close()
	p.Tap("Choice")
	p.Advance(time.Second)
	trigger, _ := p.Find("Choice")
	selected, ok := p.FindRole(ggui.RoleOption, "Beta")
	if !ok || math.Abs(selected.Rect.Center().Y-trigger.Rect.Center().Y) > 1e-6 {
		t.Fatalf("selected=%v trigger=%v", selected.Rect, trigger.Rect)
	}
	before := selectBox.Popup().Rect()
	p.Key("down")
	if selectBox.Popup().Rect() != before {
		t.Fatal("highlight movement repositioned popup")
	}
	p.Key("enter")
	if selectBox.Popup().IsOpen() {
		t.Fatal("choosing did not close aligned popup")
	}
}

func TestSelectAlignedCollisionAndOptOut(t *testing.T) {
	t.Parallel()
	for _, align := range []bool{true, false} {
		box := ui.Select(ggui.State("Beta")).Options([]string{"Alpha", "Beta", "Gamma"}).Name("Choice").AlignItemWithTrigger(align)
		p := ggui.NewProbe(ggui.Column(box), ggui.Sz(240, 220))
		p.Tap("Choice")
		p.Advance(time.Second)
		trigger, _ := p.Find("Choice")
		r := box.Popup().Rect()
		// An aligned list keeps the 10px Base UI's SelectPopup keeps from
		// the top and bottom of the window, any other the positioner's 5px.
		edge := 5.0
		if align {
			edge = 10
		}
		if r.Origin.Y < edge || r.Origin.Y+r.Size.H > 220-edge || r.Origin.X < 5 {
			t.Fatalf("align=%v: popup at %v, want it %vpx inside the window above and below and 5px at the side", align, r, edge)
		}
		if !align && r.Origin.Y < trigger.Rect.Origin.Y+trigger.Rect.Size.H+4 {
			t.Fatalf("opt-out overlapped trigger: %v", r)
		}
		// A short list stays fully available even when exact alignment would
		// put preceding rows above the window.
		if _, ok := p.FindRole(ggui.RoleOption, "Alpha"); !ok {
			t.Fatal("collision clipped a fitting list")
		}
		p.Close()
	}
}

func TestSelectAlignedOverflowAndReducedMotion(t *testing.T) {
	t.Parallel()
	values := []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine"}
	box := ui.Select(ggui.State("seven")).Options(values).Name("Choice")
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(ggui.Box().Height(60), box)), ggui.Sz(220, 160))
	defer p.Close()
	p.Tap("Choice")
	selected, ok := p.FindRole(ggui.RoleOption, "seven")
	trigger, _ := p.Find("Choice")
	if !ok || math.Abs(selected.Rect.Center().Y-trigger.Rect.Center().Y) > 1e-6 {
		t.Fatalf("overflow alignment: selected=%v trigger=%v", selected.Rect, trigger.Rect)
	}
	r := box.Popup().Rect()
	p.Scroll(r.Center(), ggui.Pt(0, 100))
	p.Frame()
	if _, ok := p.FindRole(ggui.RoleOption, "zero"); !ok {
		t.Fatal("scroll could not reach preceding options")
	}
}

func TestSelectAlignedRebuildKeepsScrollPosition(t *testing.T) {
	t.Parallel()
	revision := ggui.State(0)
	value := ggui.State("seven")
	var box *ui.SelectWidget[string]
	p := ggui.ProbeBuilder(func() ggui.Widget {
		_ = revision.Get()
		box = ui.Select(value).Options([]string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine"}).Name("Choice").Key("choice")
		return ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(ggui.Box().Height(60), box))
	}, ggui.Sz(220, 160))
	defer p.Close()
	p.Tap("Choice")
	p.Frame()
	p.Scroll(box.Popup().Rect().Center(), ggui.Pt(0, 100))
	p.Frame()
	if _, ok := p.FindRole(ggui.RoleOption, "zero"); !ok {
		t.Fatal("initial scroll failed")
	}
	revision.Set(1)
	p.Frame()
	if _, ok := p.FindRole(ggui.RoleOption, "zero"); !ok {
		t.Fatal("rebuilding realigned and discarded user's scroll position")
	}
}

func TestNativeSelectFallsBackWithoutNativeWindow(t *testing.T) {
	t.Parallel()
	value := ggui.State("Alpha")
	changes := 0
	box := ui.NativeSelect(value).Options([]string{"Alpha", "Beta"}).Name("Native choice").OnChange(func(string) { changes++ })
	p := ggui.NewProbe(ggui.Column(box), ggui.Sz(240, 220))
	defer p.Close()
	p.Tap("Native choice")
	if !box.Popup().IsOpen() {
		t.Fatal("native unavailable must open fallback")
	}
	p.Tap("Beta")
	if value.Get() != "Beta" || changes != 1 {
		t.Fatal("fallback selection did not update binding")
	}
	box.Disabled(true)
	if box.Act(ggui.Action{Kind: ggui.ActionExpand}) {
		t.Fatal("disabled native select accepted expansion")
	}
}
