package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestSliderOrientationAndDirection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		vertical, rtl bool
	}{{"horizontal", false, false}, {"rtl", false, true}, {"vertical", true, false}, {"vertical-rtl", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			value := ggui.State(20.0)
			s := ui.Slider(value, 0, 100).Step(5).LargeStep(50).Name("Value")
			if tc.vertical {
				s.Vertical()
			}
			var w ggui.Widget = s
			if tc.rtl {
				w = ggui.Directionality(ggui.DirRTL, w)
			}
			p := ggui.NewProbe(w, ggui.Sz(224, 224))
			defer p.Close()
			pos := ggui.Pt(6+212*.25, 10)
			want := 25.0
			if tc.rtl {
				want = 75
			}
			if tc.vertical {
				pos = ggui.Pt(10, 6+212*.25)
				want = 75
			}
			p.Click(pos)
			if ggui.Peek(value) != want {
				t.Fatalf("pointer: %v want %v", ggui.Peek(value), want)
			}
			p.Key("left")
			if tc.rtl && !tc.vertical {
				want += 5
			} else {
				want -= 5
			}
			if ggui.Peek(value) != want {
				t.Fatalf("left: %v want %v", ggui.Peek(value), want)
			}
			p.Key("up")
			want += 5
			if ggui.Peek(value) != want {
				t.Fatalf("up: %v want %v", ggui.Peek(value), want)
			}
			p.Key("home", "shift+up")
			if ggui.Peek(value) != 50 {
				t.Fatal("large step or Home")
			}
			p.Key("end", "pagedown")
			if ggui.Peek(value) != 50 {
				t.Fatal("PageDown or End")
			}
		})
	}
}

func TestRangeSliderOrientationAndDirection(t *testing.T) {
	t.Parallel()
	for _, vertical := range []bool{false, true} {
		name := "rtl"
		if vertical {
			name = "vertical-rtl"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			values := ggui.State([]float64{20, 80})
			s := ui.RangeSlider(values, 0, 100).Step(5).LargeStep(50).MinGap(10).ThumbNames("Low", "High")
			if vertical {
				s.Vertical()
			}
			p := ggui.NewProbe(ggui.Directionality(ggui.DirRTL, s), ggui.Sz(224, 224))
			defer p.Close()
			low, _ := p.Semantics().Find(ggui.RoleSlider, "Low")
			high, _ := p.Semantics().Find(ggui.RoleSlider, "High")
			if (!vertical && low.Rect.Origin.X <= high.Rect.Origin.X) || (vertical && low.Rect.Origin.Y <= high.Rect.Origin.Y) {
				t.Fatalf("thumbs not reversed: low=%v high=%v", low.Rect, high.Rect)
			}
			p.Key("tab", "left")
			want := 25.0
			if vertical {
				want = 15
			}
			if ggui.Peek(values)[0] != want {
				t.Fatalf("left=%v want %v", ggui.Peek(values), want)
			}
			p.Key("up")
			if ggui.Peek(values)[0] != want+5 {
				t.Fatalf("up=%v", ggui.Peek(values))
			}
			pos := ggui.Pt(6+212*.65, 10)
			if vertical {
				pos = ggui.Pt(10, 6+212*.65)
			}
			p.Press(pos)
			p.Release(pos)
			if ggui.Peek(values)[0] != 35 {
				t.Fatalf("pointer=%v", ggui.Peek(values))
			}
			p.Key("shift+up")
			if ggui.Peek(values)[0] != 70 {
				t.Fatalf("large step gap=%v", ggui.Peek(values))
			}
		})
	}
}

func TestSliderDefaultAndExplicitLargeSteps(t *testing.T) {
	t.Parallel()
	scalar := ggui.State(20.0)
	rangeValue := ggui.State([]float64{20, 180})
	for _, w := range []ggui.Widget{ui.Slider(scalar, 0, 200), ui.RangeSlider(rangeValue, 0, 200)} {
		p := ggui.NewProbe(w, ggui.Sz(224, 40))
		p.Key("tab", "right", "shift+right")
		p.Close()
	}
	if ggui.Peek(scalar) != 31 || ggui.Peek(rangeValue)[0] != 31 {
		t.Fatalf("defaults scalar=%v range=%v", ggui.Peek(scalar), ggui.Peek(rangeValue))
	}
}

func TestSliderDisableDuringDragCancelsPressAndCommit(t *testing.T) {
	t.Parallel()
	disabled := ggui.State(false)
	value := ggui.State(0.0)
	commits := 0
	s := ui.Slider(value, 0, 100).BindDisabled(disabled).OnCommit(func(float64) { commits++ })
	p := ggui.NewProbe(ggui.Column(s), ggui.Sz(224, 40))
	defer p.Close()
	p.Press(ggui.Pt(50, 10))
	before := ggui.Peek(value)
	disabled.Set(true)
	p.Frame()
	p.Release(ggui.Pt(218, 10))
	disabled.Set(false)
	p.Frame()
	if s.Pressed || commits != 0 || ggui.Peek(value) != before {
		t.Fatalf("cancel: pressed=%v commits=%d value=%v", s.Pressed, commits, ggui.Peek(value))
	}
	// A new gesture must still be usable after the cancelled capture.
	p.Press(ggui.Pt(60, 10))
	p.Release(ggui.Pt(218, 10))
	if s.Pressed || commits != 1 || ggui.Peek(value) != 100 {
		t.Fatalf("next gesture: pressed=%v commits=%d value=%v", s.Pressed, commits, ggui.Peek(value))
	}
	// Direct handlers also cancel when there has been no intervening Layout.
	s.HandlePointer(ggui.PointerEvent{Kind: ggui.PointerDown, Button: ggui.MouseButtonLeft, Pos: ggui.Pt(60, 10)})
	s.Disabled(true)
	s.HandlePointer(ggui.PointerEvent{Kind: ggui.PointerUp, Button: ggui.MouseButtonLeft, Pos: ggui.Pt(218, 10)})
	if s.Pressed || commits != 1 {
		t.Fatalf("direct inert release: pressed=%v commits=%d", s.Pressed, commits)
	}
}

func TestSliderPointerReleasePreservesOffStepEndpoints(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		vertical, rtl bool
	}{{"horizontal", false, false}, {"rtl", false, true}, {"vertical", true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			value := ggui.State(0.0)
			var committed []float64
			s := ui.Slider(value, 0, 10).Step(3).OnCommit(func(v float64) { committed = append(committed, v) })
			if tc.vertical {
				s.Vertical()
			}
			var w ggui.Widget = s
			if tc.rtl {
				w = ggui.Directionality(ggui.DirRTL, w)
			}
			p := ggui.NewProbe(w, ggui.Sz(224, 224))
			defer p.Close()
			low, high, middle := ggui.Pt(6, 10), ggui.Pt(218, 10), ggui.Pt(112, 10)
			if tc.rtl {
				low, high = high, low
			}
			if tc.vertical {
				low, high, middle = ggui.Pt(10, 218), ggui.Pt(10, 6), ggui.Pt(10, 112)
			}
			p.Press(middle)
			if ggui.Peek(value) != 6 {
				t.Fatal("interior value did not snap")
			}
			p.Release(high) // deliberately no intervening Move event
			if ggui.Peek(value) != 10 || len(committed) != 1 || committed[0] != 10 {
				t.Fatalf("maximum value=%v commits=%v", ggui.Peek(value), committed)
			}
			p.Press(middle)
			p.Release(low)
			if ggui.Peek(value) != 0 || len(committed) != 2 || committed[1] != 0 {
				t.Fatalf("minimum value=%v commits=%v", ggui.Peek(value), committed)
			}
		})
	}
}
