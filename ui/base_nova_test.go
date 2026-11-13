package ui

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func TestBaseNovaControlGeometry(t *testing.T) {
	t.Parallel()
	for _, theme := range []uitheme.Theme{uitheme.Default(), uitheme.Dark(), (uitheme.Preset{}).Light(), (uitheme.Preset{}).Dark()} {
		env := theme.Apply(ggui.Env{})
		c := ggui.Loose(ggui.Sz(500, 200))
		for _, tc := range []struct {
			text, icon *ButtonSizeVariant
			height     float64
		}{
			{ButtonExtraSmall, ButtonIconExtraSmall, 24}, {ButtonSmall, ButtonIconSmall, 28}, {ButtonDefault, ButtonIcon, 32}, {ButtonLarge, ButtonIconLarge, 36},
		} {
			text := Button("Action", nil).Size(tc.text).Layout(c, env)
			icon := ButtonOf(Icon(icons.Plus), nil).Size(tc.icon).Layout(c, env)
			if text.H != tc.height || icon != ggui.Sz(tc.height, tc.height) {
				t.Errorf("%s: text=%v icon=%v, want height %v", tc.text, text, icon, tc.height)
			}
		}
		if size := TextField(ggui.State("")).Layout(c, env); size.H != 32 {
			t.Errorf("input height=%v", size.H)
		}
		if size := Badge("New").Layout(c, env); size.H != 20 {
			t.Errorf("badge height=%v", size.H)
		}
		if size := Switch(ggui.State(false), "").Layout(c, env); size != ggui.Sz(32, 18.4) {
			t.Errorf("switch=%v", size)
		}
		largeText := Button("Readable", nil).Layout(c, env.With(ggui.TextScaleKey, 2.0))
		if largeText.H <= 32 {
			t.Fatalf("text scaling clipped to default height: %v", largeText)
		}
	}
}

func TestBaseNovaModalCloseAction(t *testing.T) {
	t.Parallel()
	for _, sheet := range []bool{false, true} {
		open := ggui.State(true)
		var w ggui.Widget = Dialog(open, ggui.Text("Body"))
		if sheet {
			w = Sheet(open, ggui.Text("Body"))
		}
		p := ggui.NewProbe(w, ggui.Sz(800, 600))
		p.Frame()
		p.Tap("Close")
		if open.Get() {
			t.Errorf("sheet=%v: close action left modal open", sheet)
		}
		p.Close()
	}
}

func TestAccordionHeadersAreIndependentTabStops(t *testing.T) {
	t.Parallel()
	open := ggui.State([]string{})
	a := Accordion(open, AccordionItem("a", "Alpha", ggui.Text("A")), AccordionItem("b", "Disabled", ggui.Text("B")).Disabled(true), AccordionItem("c", "Charlie", ggui.Text("C")))
	p := ggui.NewProbe(a, ggui.Sz(400, 300))
	defer p.Close()
	p.Key("tab")
	p.Key("tab")
	if n, ok := p.Semantics().Focused(); !ok || n.Name != "Charlie" {
		t.Fatalf("Tab must skip disabled header: %+v", n)
	}
	p.Key("enter")
	if len(open.Get()) != 1 || open.Get()[0] != "c" {
		t.Fatal("Enter did not expand the focused header")
	}
	p.Advance(time.Second)
	p.Key("shift+tab")
	if n, ok := p.Semantics().Focused(); !ok || n.Name != "Alpha" {
		t.Fatalf("Shift-Tab did not return to Alpha: %+v", n)
	}
}

func TestSliderKeyboardRangeAndLargeSteps(t *testing.T) {
	t.Parallel()
	value := ggui.State(50.0)
	slider := Slider(value, 0, 100).Step(1)
	p := ggui.NewProbe(slider, ggui.Sz(200, 32))
	defer p.Close()
	p.Key("tab")
	for _, step := range []struct {
		key  string
		want float64
	}{{"home", 0}, {"end", 100}, {"shift+left", 90}, {"pagedown", 80}, {"pageup", 90}} {
		p.Key(step.key)
		if value.Get() != step.want {
			t.Fatalf("%s: got %v want %v", step.key, value.Get(), step.want)
		}
	}
}
