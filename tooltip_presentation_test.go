package ggui

import (
	"testing"
	"time"
)

func TestTooltipKeyboardFocusAndNarrowViewport(t *testing.T) {
	input := TextInput(State("")).Name("Editor")
	tip := Tooltip(input, "A longer explanation that must fit within a narrow window.")
	p := NewProbe(tip, Sz(100, 200))
	defer p.Close()
	p.Advance(0)
	p.Type(Mods{}, KeyTab)
	p.Frame()
	p.Advance(time.Second)
	if _, ok := p.Semantics().Find(RoleText, "A longer explanation that must fit within a narrow window."); !ok {
		t.Fatal("keyboard focus does not reveal tooltip")
	}
	size := tip.effect.Layout(Loose(Sz(100, Unbounded)), tip.env)
	if size.W > 100 {
		t.Fatalf("tooltip overflows: %v", size)
	}
}
