package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// shadcn's checkbox, radio and switch are hit 12px to either side of the
// glyph and 8px above and below it (after:-inset-x-3 after:-inset-y-2),
// with no change to their size.
func TestChoiceControlsAreHitAroundTheGlyph(t *testing.T) {
	t.Parallel()
	agree, pick, on := ggui.State(false), ggui.State("a"), ggui.State(false)
	root := ggui.Box(ggui.Column(
		ui.Checkbox(agree, "").Name("Agree"),
		ui.Radio(pick, "b", "").Name("Second"),
		ui.Switch(on, "").Name("Airplane"),
	).Gap(40).Align(ggui.AlignStart)).Padding(ggui.Insets(40))
	p := ggui.NewProbe(root, ggui.Sz(320, 400))
	defer p.Close()
	for _, c := range []struct {
		role ggui.Role
		name string
		size ggui.Size
		on   func() bool
	}{
		{ggui.RoleCheckbox, "Agree", ggui.Sz(16, 16), agree.Get},
		{ggui.RoleRadio, "Second", ggui.Sz(16, 16), func() bool { return pick.Get() == "b" }},
		{ggui.RoleSwitch, "Airplane", ggui.Sz(32, 18.4), on.Get},
	} {
		f, ok := p.FindRole(c.role, c.name)
		if !ok {
			t.Fatalf("no %v named %q", c.role, c.name)
		}
		if f.Rect.Size != c.size {
			t.Errorf("%s is %v, want %v: the reach must not change its size", c.name, f.Rect.Size, c.size)
		}
		left, top := f.Rect.Origin.X, f.Rect.Origin.Y
		p.Click(ggui.Pt(left-13, top+4))
		p.Click(ggui.Pt(left+4, top-9))
		if c.on() {
			t.Errorf("%s was toggled from past its reach", c.name)
		}
		p.Click(ggui.Pt(left-11, top-7))
		if !c.on() {
			t.Errorf("%s was not toggled from 11px left of and 7px above its glyph", c.name)
		}
		p.Click(ggui.Pt(left+f.Rect.Size.W+11, top+f.Rect.Size.H+7))
		if c.role != ggui.RoleRadio && c.on() {
			t.Errorf("%s was not toggled back from the far corner of its reach", c.name)
		}
	}
}

// A labelled checkbox reaches around its glyph, not around its label.
func TestChoiceReachStaysWithTheGlyph(t *testing.T) {
	t.Parallel()
	agree := ggui.State(false)
	root := ggui.Box(ggui.Column(ui.Checkbox(agree, "Agree to the terms")).Align(ggui.AlignStart)).Padding(ggui.Insets(40))
	p := ggui.NewProbe(root, ggui.Sz(320, 200))
	defer p.Close()
	f, _ := p.FindRole(ggui.RoleCheckbox, "Agree to the terms")
	p.Click(ggui.Pt(f.Rect.Origin.X+f.Rect.Size.W+6, f.Rect.Center().Y))
	p.Click(ggui.Pt(f.Rect.Origin.X+60, f.Rect.Origin.Y-5))
	if agree.Get() {
		t.Fatal("the reach runs along the label")
	}
	p.Click(ggui.Pt(f.Rect.Origin.X+60, f.Rect.Center().Y))
	if !agree.Get() {
		t.Fatal("the label no longer toggles")
	}
}

// A slider's thumb is taken from 8px around it (after:-inset-2), past the
// slider's own edges, and shows its halo there.
func TestSliderThumbIsHitAroundIt(t *testing.T) {
	t.Parallel()
	v := ggui.State(0.0)
	s := ui.Slider(v, 0, 100).Name("Volume")
	p := ggui.NewProbe(ggui.Box(s).Padding(ggui.Insets(40)), ggui.Sz(320, 120))
	defer p.Close()
	f, _ := p.FindRole(ggui.RoleSlider, "Volume")
	if f.Rect.Size.H != 20 {
		t.Fatalf("the slider is %v tall, want 20", f.Rect.Size.H)
	}
	above := ggui.Pt(f.Rect.Origin.X+6, f.Rect.Origin.Y-3)
	p.Move(above)
	if !s.Hovered {
		t.Fatal("the thumb is not hovered 3px above the slider")
	}
	p.Move(ggui.Pt(f.Rect.Origin.X+60, f.Rect.Origin.Y-3))
	if s.Hovered {
		t.Fatal("the track is hit above the slider away from the thumb")
	}
	p.Press(ggui.Pt(f.Rect.Origin.X-6, f.Rect.Center().Y))
	p.Move(ggui.Pt(f.Rect.Origin.X+f.Rect.Size.W/2, f.Rect.Center().Y))
	p.Release(ggui.Pt(f.Rect.Origin.X+f.Rect.Size.W/2, f.Rect.Center().Y))
	if got := v.Get(); got != 50 {
		t.Fatalf("dragging the thumb from left of the slider to its middle gave %v, want 50", got)
	}
}
