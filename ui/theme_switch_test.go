package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestThemeSwitchPointerKeyboardAndDisabled(t *testing.T) {
	dark, disabled := ggui.State(false), ggui.State(false)
	var changes []bool
	s := ui.ThemeSwitch(dark).Name("Appearance").BindDisabled(disabled).
		OnChange(func(v bool) { changes = append(changes, v) })
	p := ggui.NewProbe(s, ggui.Sz(64, 36))
	defer p.Close()
	p.Tap("Appearance")
	if !ggui.Untrack(dark.Get) || s.Describe().Checked != ggui.Tri(true) {
		t.Fatal("tap did not enable dark mode or update accessibility")
	}
	p.Type(ggui.Mods{}, ggui.KeySpace)
	if ggui.Untrack(dark.Get) || len(changes) != 2 || !changes[0] || changes[1] {
		t.Fatalf("keyboard toggle: dark=%v changes=%v", ggui.Untrack(dark.Get), changes)
	}
	disabled.Set(true)
	p.Frame()
	p.Click(ggui.Pt(32, 18))
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if ggui.Untrack(dark.Get) || len(changes) != 2 || !s.Describe().Disabled {
		t.Fatal("disabled theme switch accepted input")
	}
}

func TestThemeSwitchRebuildAndReducedMotion(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		t.Run(map[bool]string{false: "animated", true: "reduced"}[reduced], func(t *testing.T) {
			dark := ggui.State(false)
			p := ggui.ProbeBuilder(func() ggui.Widget {
				dark.Get()
				return ggui.Provide(ggui.ReducedMotionKey, reduced, ui.ThemeSwitch(dark))
			}, ggui.Sz(64, 36))
			defer p.Close()
			p.Tap("Dark mode")
			p.Advance(80 * time.Millisecond)
			p.Type(ggui.Mods{}, ggui.KeyEnter)
			p.Advance(time.Second)
			if ggui.Untrack(dark.Get) {
				t.Fatal("rebuilt switch lost keyboard focus during transition")
			}
			dark.Set(true)
			p.Frame()
			p.Tap("Dark mode")
			if ggui.Untrack(dark.Get) {
				t.Fatal("switch did not follow an external mode change")
			}
		})
	}
}
