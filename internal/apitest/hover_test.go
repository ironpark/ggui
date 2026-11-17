package apitest_test

import (
	"slices"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
	"github.com/ironpark/ggui/widget"
)

// A row holding a button hears the pointer over the button as over the
// row, and the button still shows its own hover: a tab whose close button
// shows while the tab is hovered.
func TestARowHearsHoverOverAButtonInsideIt(t *testing.T) {
	t.Parallel()
	for _, start := range []string{"the row", "outside"} {
		t.Run(start, func(t *testing.T) {
			t.Parallel()
			var hover []bool
			closeBtn := ui.ButtonOf(ui.Icon(icons.Plus), func() {}).Ghost().Name("Close")
			row := widget.Pointer(widget.Row(widget.Box().Size(100, 30), closeBtn)).
				OnHover(func(on bool) { hover = append(hover, on) })
			p := ggui.NewProbe(widget.Column(row, widget.Box().Size(100, 30)), ggui.Sz(200, 100))
			defer p.Close()
			x, ok := p.FindRole(ggui.RoleButton, "Close")
			if !ok {
				t.Fatal("no close button")
			}
			if start == "the row" {
				p.Move(ggui.Pt(10, 10)) // the row's own area
				if !slices.Equal(hover, []bool{true}) {
					t.Fatalf("over the row: hover %v, want [true]", hover)
				}
			}
			p.Move(x.Center()) // the button
			if !slices.Equal(hover, []bool{true}) || !closeBtn.Hovered {
				t.Fatalf("over the button: hover %v, button hovered %v; want [true] and hovered", hover, closeBtn.Hovered)
			}
			p.Move(ggui.Pt(10, 10))
			if !slices.Equal(hover, []bool{true}) || closeBtn.Hovered {
				t.Fatalf("back on the row: hover %v, button hovered %v; want [true] and not hovered", hover, closeBtn.Hovered)
			}
			p.Move(ggui.Pt(10, 80))
			if !slices.Equal(hover, []bool{true, false}) {
				t.Fatalf("away: hover %v, want [true false]", hover)
			}
		})
	}
}

// The reactive form: a tab builds its close button from a signal its
// hover sets. The button, once shown, keeps the tab hovered while the
// pointer is over it, so it does not vanish from under the pointer, and a
// click on it is its own.
func TestAButtonShownOnHoverStaysWhileThePointerIsOverIt(t *testing.T) {
	t.Parallel()
	hover := ggui.State(false)
	closed, opened := 0, 0
	var closeBtn *ui.ButtonWidget
	shown := ggui.View(hover, func(on bool) ggui.Widget {
		if !on {
			closeBtn = nil
			return widget.Box().Size(24, 24)
		}
		closeBtn = ui.ButtonOf(ui.Icon(icons.Plus), func() { closed++ }).Ghost().Name("Close")
		return closeBtn
	})
	tab := widget.Pointer(ui.ButtonOf(widget.Row(widget.Box().Size(80, 24), shown), func() { opened++ }).Ghost().Name("Tab")).
		OnHover(hover.Set)
	p := ggui.NewProbe(widget.Column(tab, widget.Box().Size(100, 40)), ggui.Sz(200, 120))
	defer p.Close()
	if _, ok := p.FindRole(ggui.RoleButton, "Close"); ok {
		t.Fatal("the close button shows before the tab is hovered")
	}
	p.Move(ggui.Pt(10, 10))
	p.Frame()
	x, ok := p.FindRole(ggui.RoleButton, "Close")
	if !ok {
		t.Fatal("the close button does not show over the tab")
	}
	p.Move(x.Center())
	for range 3 {
		p.Frame()
	}
	if _, ok := p.FindRole(ggui.RoleButton, "Close"); !ok || closeBtn == nil || !closeBtn.Hovered {
		t.Fatalf("over the close button: found %v, hovered %v", ok, closeBtn != nil && closeBtn.Hovered)
	}
	p.Click(x.Center())
	p.Frame()
	if closed != 1 || opened != 0 {
		t.Fatalf("closed %d, opened %d after clicking the close button; want 1 and 0", closed, opened)
	}
	p.Move(ggui.Pt(10, 100))
	p.Frame()
	if _, ok := p.FindRole(ggui.RoleButton, "Close"); ok {
		t.Fatal("the close button still shows after the pointer left the tab")
	}
}
