package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestSelectPopupStaysWithinTriggerWidth(t *testing.T) {
	value := ggui.State("Alpha")
	selectBox := ui.Select(value).Options([]string{"Alpha", "Beta"}).Name("Choose option")
	p := ggui.NewProbe(ggui.Box(selectBox).Width(180), ggui.Sz(900, 400))
	defer p.Close()
	p.Tap("Choose option")
	trigger, ok := p.Find("Choose option")
	if !ok {
		t.Fatal("missing trigger")
	}
	option, ok := p.Find("Beta")
	if !ok {
		t.Fatal("missing option")
	}
	if option.Rect.Size.W > trigger.Rect.Size.W {
		t.Fatalf("popup expanded to viewport: option %v, trigger %v", option.Rect, trigger.Rect)
	}
}

func TestSelectOverflowScrollsWithoutCollapsingOptions(t *testing.T) {
	options := []string{"amber", "blue", "cyan", "green", "lime", "orange", "pink", "purple", "red", "rose", "teal", "yellow"}
	for _, above := range []bool{false, true} {
		value := ggui.State("lime")
		selectBox := ui.Select(value).Options(options).Name("Color")
		var root ggui.Widget = ggui.Column(selectBox).Align(ggui.AlignStretch)
		if above {
			root = ggui.Column(ggui.Box().Height(180), selectBox).Align(ggui.AlignStretch)
		}
		p := ggui.NewProbe(root, ggui.Sz(240, 220))
		p.Tap("Color")
		p.Advance(time.Second)
		visible := p.FindAll(ggui.RoleOption)
		if len(visible) == 0 || len(visible) >= len(options) {
			t.Fatalf("above=%v: visible count = %d", above, len(visible))
		}
		for i := 1; i < len(visible); i++ {
			prev, next := visible[i-1].Rect, visible[i].Rect
			if next.Origin.Y+1e-6 < prev.Origin.Y+prev.Size.H {
				t.Fatalf("overlapping options: %v %v", prev, next)
			}
		}
		if _, ok := p.Find("lime"); !ok {
			t.Fatal("selected option should be visible on opening")
		}
		popup := selectBox.Popup().Rect()
		p.Scroll(ggui.Pt(popup.Origin.X+20, popup.Origin.Y+20), ggui.Pt(0, -100))
		if _, ok := p.Find("yellow"); !ok {
			t.Fatal("last option missing after scrolling")
		}
		p.Frame()
		if _, ok := p.Find("yellow"); !ok {
			t.Fatal("popup measurement reset scroll position")
		}
		p.Close()
	}
}

func TestSelectKeyboardRevealsOverflowOption(t *testing.T) {
	options := []string{"one", "two", "three", "four", "five", "six", "seven", "eight"}
	value := ggui.State("one")
	selectBox := ui.Select(value).Options(options).Name("Number")
	p := ggui.NewProbe(ggui.Column(selectBox), ggui.Sz(180, 150))
	defer p.Close()
	p.Tap("Number")
	p.Advance(time.Second)
	for range len(options) - 1 {
		p.Type(ggui.Mods{}, ggui.KeyArrowDown)
	}
	if _, ok := p.Find("eight"); !ok {
		t.Fatal("keyboard-highlighted option is offscreen")
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if ggui.Untrack(value.Get) != "eight" {
		t.Fatal("keyboard did not select last option")
	}
}

func TestSelectionControlsOwnOptionSnapshots(t *testing.T) {
	for _, kind := range []string{"select", "combobox", "toggle group"} {
		t.Run(kind, func(t *testing.T) {
			options := []string{"Alpha", "Beta"}
			value := ggui.State("Beta")
			var control ggui.Widget
			if kind == "select" {
				control = ui.Select(value).Options(options).Name("Choice")
			} else if kind == "combobox" {
				control = ui.Combobox(value).Options(options).Name("Choice")
			} else {
				control = ui.ToggleGroup(value).Options(options)
			}
			options[0] = "Mutated"
			p := ggui.NewProbe(ggui.Column(control), ggui.Sz(300, 300))
			defer p.Close()
			if kind != "toggle group" {
				p.Tap("Choice")
			}
			p.Tap("Alpha")
			if got := ggui.Untrack(value.Get); got != "Alpha" {
				t.Fatalf("picked %q from caller-mutated options; want Alpha", got)
			}
		})
	}
}
