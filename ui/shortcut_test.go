package ui

import (
	"testing"

	"github.com/ironpark/ggui"
)

// A button's Shortcut presses it while it is on screen and enabled, and a
// Tooltip around it shows the chord as the platform writes it.
func TestButtonShortcutPressesAndLabelsTheTooltip(t *testing.T) {
	t.Parallel()
	disabled := ggui.State(false)
	undos := 0
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return Tooltip(Button("Undo", func() { undos++ }).Shortcut("cmd+z").BindDisabled(disabled), "Undo last change")
	}, ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
	p.Key("cmd+z")
	if undos != 1 {
		t.Fatalf("cmd+z pressed the button %d times, want 1", undos)
	}
	p.Key("tab") // focus opens the tooltip
	p.Frame()
	label := ggui.MustChord("cmd+z").Label()
	shown := map[string]bool{}
	for _, n := range p.Semantics().All() {
		shown[n.Name] = true
	}
	if !shown[label] || !shown["Undo last change"] {
		t.Errorf("the tooltip shows %v, want its text and %q", shown, label)
	}
	disabled.Set(true)
	p.Frame()
	p.Key("cmd+z")
	if undos != 1 {
		t.Fatalf("cmd+z pressed a disabled button: %d presses", undos)
	}
}

func TestShortcutHintsWriteChordsForThePlatform(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"cmd+shift+z": ggui.MustChord("cmd+shift+z").Label(),
		"⌘N":          "⌘N",
		"Ctrl + S":    "Ctrl + S",
		"":            "",
	} {
		if got := shortcutLabel(in); got != want {
			t.Errorf("shortcutLabel(%q) = %q, want %q", in, got, want)
		}
	}
	tip := Tooltip(ggui.Box(), "Save").Shortcut("cmd+s")
	tip.Layout(ggui.Loose(ggui.Sz(100, 100)), ggui.Env{})
	if tip.keys == nil {
		t.Fatal("Tooltip.Shortcut added no hint")
	}
}

// A tab's Header replaces its text in the strip and a click on it still
// picks the tab by its label; focus on the strip shows only the shown
// tab's Tooltip.
func TestTabsHeaderAndTooltip(t *testing.T) {
	t.Parallel()
	sel := ggui.State(0)
	badge := Badge("3")
	tabs := Tabs(sel,
		Tab("Build", ggui.Text("build page")).Header(ggui.Row(ggui.Text("Build"), badge).Gap(6)).Tooltip("Compile the project"),
		Tab("Test", ggui.Text("test page")).Tooltip("Run the tests"),
	)
	p := ggui.NewProbe(ggui.Column(tabs), ggui.Sz(400, 200))
	defer p.Close()
	p.Frame()
	names := func() map[string]bool {
		shown := map[string]bool{}
		for _, n := range p.Semantics().All() {
			shown[n.Name] = true
		}
		return shown
	}
	if badge.size == (ggui.Size{}) || tabs.labelSize[0].W <= tabs.labelSize[1].W {
		t.Fatalf("the badge is %v and the tabs %v wide; want the custom header, badge included", badge.size, tabs.labelSize)
	}
	p.Tap("Test")
	if ggui.Untrack(sel.Get) != 1 {
		t.Fatalf("selected %d after clicking Test", ggui.Untrack(sel.Get))
	}
	p.Type(ggui.Mods{}, ggui.KeyTab) // focus the strip
	p.Frame()
	if n := names(); !n["Run the tests"] || n["Compile the project"] {
		t.Fatalf("with focus on the strip, the tips shown are %v; want only the shown tab's", n)
	}
}
