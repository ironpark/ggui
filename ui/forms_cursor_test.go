package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// shadcn's form controls keep the arrow under the pointer: none of them
// sets cursor-pointer, and the rows of a select are cursor-default.
func TestFormControlsKeepTheDefaultCursor(t *testing.T) {
	t.Parallel()
	fruit := ggui.State("Apple")
	root := ggui.Column(
		ui.Checkbox(ggui.State(false), "Agree"),
		ui.Radio(ggui.State("a"), "b", "Second"),
		ui.Switch(ggui.State(false), "Airplane"),
		ui.Slider(ggui.State(40.0), 0, 100).Name("Volume"),
		ui.RangeSlider(ggui.State([]float64{20, 60}), 0, 100).Name("Price"),
		ui.Select(fruit).Options([]string{"Apple", "Banana"}).Name("Fruit").AlignItemWithTrigger(false),
	).Gap(8)
	p := ggui.NewProbe(root, ggui.Sz(320, 400))
	defer p.Close()
	for _, c := range []struct {
		role ggui.Role
		name string
	}{
		{ggui.RoleCheckbox, "Agree"}, {ggui.RoleRadio, "Second"}, {ggui.RoleSwitch, "Airplane"},
		{ggui.RoleSlider, "Volume"}, {ggui.RoleSelect, "Fruit"},
	} {
		f, ok := p.FindRole(c.role, c.name)
		if !ok {
			t.Fatalf("no %v named %q", c.role, c.name)
		}
		p.Move(f.Rect.Center())
		if got := p.Cursor(); got != ggui.CursorShapeDefault {
			t.Errorf("cursor = %v over %q, want default", got, c.name)
		}
	}
	for _, thumb := range p.FindAll(ggui.RoleSlider) {
		p.Move(thumb.Rect.Center())
		if got := p.Cursor(); got != ggui.CursorShapeDefault {
			t.Errorf("cursor = %v over the slider thumb at %v, want default", got, thumb.Rect)
		}
	}
	p.Tap("Fruit")
	option, ok := p.FindRole(ggui.RoleOption, "Banana")
	if !ok {
		t.Fatal("the open select lists no Banana")
	}
	p.Move(option.Rect.Center())
	if got := p.Cursor(); got != ggui.CursorShapeDefault {
		t.Errorf("cursor = %v over a select option, want default", got)
	}
}
