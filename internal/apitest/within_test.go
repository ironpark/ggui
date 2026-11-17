package apitest_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/widget"
)

// A pane hears every press inside it before what it lands on, and takes
// none of them: a press anywhere in a pane makes it the current one.
func TestWithinHearsEveryPressInAPane(t *testing.T) {
	t.Parallel()
	downs, taps := 0, 0
	pane := widget.Within(widget.Column(
		ui.Button("Run", func() { taps++ }),
		widget.Box().Size(100, 50),
	)).OnDown(func(ggui.PointerEvent) { downs++ })
	p := ggui.NewProbe(widget.Row(pane, ui.Button("Other", func() {})).Align(widget.AlignStart), ggui.Sz(300, 120))
	defer p.Close()
	run, ok := p.FindRole(ggui.RoleButton, "Run")
	if !ok {
		t.Fatal("no Run button")
	}
	p.Tap("Run")
	if downs != 1 || taps != 1 {
		t.Fatalf("a click on Run: downs %d, taps %d; want it heard and not taken", downs, taps)
	}
	p.Click(ggui.Pt(20, run.Rect.Origin.Y+run.Rect.Size.H+20)) // the pane's bare area
	if downs != 2 {
		t.Fatalf("a click on the pane's own area: downs %d, want 2", downs)
	}
	p.Tap("Other")
	if downs != 2 {
		t.Fatalf("a click beside the pane: downs %d, want still 2", downs)
	}
	p.Scroll(ggui.Pt(20, 20), ggui.Pt(0, -3))
	if downs != 2 {
		t.Fatalf("a wheel over the pane: downs %d, want still 2", downs)
	}
}

// A popup the pane opens is painted over it and inside nothing: a press on
// its content is not the pane's.
func TestWithinDoesNotHearAPressOnAPopupItOpened(t *testing.T) {
	t.Parallel()
	downs, picked := 0, 0
	open := ggui.State(false)
	menu := widget.Popup(
		ui.Button("Run", func() { open.Set(true) }),
		ui.Button("Pick", func() { picked++ }),
	).BindOpen(open).Modal(false)
	pane := widget.Within(widget.Column(menu, widget.Box().Size(200, 150))).
		OnDown(func(ggui.PointerEvent) { downs++ })
	p := ggui.NewProbe(widget.Column(pane), ggui.Sz(300, 300))
	defer p.Close()
	p.Tap("Run")
	p.Frame()
	if !open.Get() || downs != 1 {
		t.Fatalf("open %v, downs %d after Run", open.Get(), downs)
	}
	pick, ok := p.FindRole(ggui.RoleButton, "Pick")
	if !ok {
		t.Fatal("the popup did not open")
	}
	p.Click(pick.Center())
	if picked != 1 || downs != 1 {
		t.Fatalf("a click on the popup: picked %d, downs %d; want 1 and still 1", picked, downs)
	}
}

// The pattern for what hover reveals: the close button is built always,
// at no opacity while neither the pointer nor the focus is in the tab, so
// it stays in the Tab order and the accessibility tree; hovering or
// tabbing into the tab shows it.
func TestWithinShowsAButtonOnHoverOrFocus(t *testing.T) {
	t.Parallel()
	hover, focus := ggui.State(false), ggui.State(false)
	shown := ggui.Combine(hover, focus, func(h, f bool) float64 {
		if h || f {
			return 1
		}
		return 0
	})
	closed := 0
	tab := widget.Within(widget.Row(
		widget.Box().Size(80, 24),
		widget.Opacity(ui.Button("Close", func() { closed++ }), 0).BindOpacity(shown),
	)).OnHover(hover.Set).OnFocus(focus.Set)
	p := ggui.NewProbe(widget.Column(tab, ui.Button("Other", func() {})).Align(widget.AlignStart), ggui.Sz(300, 120))
	defer p.Close()
	if _, ok := p.Semantics().Find(ggui.RoleButton, "Close"); !ok {
		t.Fatal("the hidden close button is not in the accessibility tree")
	}
	x, _ := p.FindRole(ggui.RoleButton, "Close")
	p.Move(ggui.Pt(10, 10))
	if !hover.Get() || shown.Get() != 1 {
		t.Fatalf("over the tab: hover %v, shown %v", hover.Get(), shown.Get())
	}
	p.Move(x.Center())
	p.Click(x.Center())
	if closed != 1 || !hover.Get() {
		t.Fatalf("closed %d, hover %v over the close button", closed, hover.Get())
	}
	p.Move(ggui.Pt(290, 110))
	p.Tap("Other")
	if hover.Get() || focus.Get() || shown.Get() != 0 {
		t.Fatalf("away: hover %v, focus %v, shown %v", hover.Get(), focus.Get(), shown.Get())
	}
	p.Key("tab") // round to the close button
	if !focus.Get() || shown.Get() != 1 {
		t.Fatalf("tabbed in: focus %v, shown %v", focus.Get(), shown.Get())
	}
}
