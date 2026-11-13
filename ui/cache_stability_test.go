package ui_test

import (
	"strconv"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// A control that writes a property of its own after measuring with it has
// changed since it was laid out, and no Cached above it holds again: every
// frame lays out everything beside it. A multiline TextField did, setting
// its box's padding to the theme's and back each layout.
func TestControlsLeaveACachedParentHeld(t *testing.T) {
	t.Parallel()
	on := ggui.State(false)
	options := []string{"a", "b", "c"}
	for name, w := range map[string]ggui.Widget{
		"TextField":           ui.TextField(ggui.State("a")),
		"TextField multiline": ui.TextField(ggui.State("a")).Multiline(),
		"Button":              ui.Button("b", nil),
		"Button icon":         ui.Button("b", nil).Size(ui.ButtonIcon),
		"Toggle":              ui.Toggle(on, "t"),
		"Slider":              ui.Slider(ggui.State(1.0), 0, 10),
		"RangeSlider":         ui.RangeSlider(ggui.State([]float64{1, 5}), 0, 10),
		"Select":              ui.Select(ggui.State("a")).Options(options),
		"ComboboxMultiple":    ui.ComboboxMultiple(ggui.State([]string{"a"})).Options(options),
		"ToggleGroupMultiple": ui.ToggleGroupMultiple(ggui.State([]string{"a"})).Options(options),
		"Tabs":                ui.Tabs(ggui.State(0), ui.Tab("a", ggui.Text("x")), ui.Tab("b", ggui.Text("x"))),
		"Popover":             ui.Popover("p", ggui.Text("x")),
		"NavigationMenu":      ui.NavigationMenu(ui.NavigationMenuItem("a", ggui.Text("x")), ui.NavigationMenuLink("b", nil)),
		"CodeBlock":           ui.CodeBlock("a\nb\nc").LineNumbers(true),
		"Trace":               ui.Trace(ui.TraceStep{ID: "a", Title: "A"}),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sibling := &layoutCounter{}
			tick := ggui.State(0)
			root := ggui.Column(ggui.Cached(ggui.Column(w, sibling)), ggui.TextOf(ggui.Map[int, string](tick, strconv.Itoa)))
			p := ggui.NewProbe(root, ggui.Sz(600, 600))
			defer p.Close()
			for range 3 {
				p.Frame()
			}
			before := sibling.layouts
			for i := range 5 {
				tick.Set(i + 1)
				p.Frame()
			}
			if got := sibling.layouts - before; got != 0 {
				t.Fatalf("a sibling under the same Cached was laid out %d times in 5 frames that changed nothing in it", got)
			}
		})
	}
}
