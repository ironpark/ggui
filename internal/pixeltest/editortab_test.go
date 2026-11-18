// Package pixeltest holds tests that read what widgets paint, under
// ggtest.Main.
package pixeltest

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ggtest"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

func TestMain(m *testing.M) { ggtest.Main(m) }

// An editor tab's close button shows only while the pointer is over the
// tab, which it tints; the selected tab is raised all the while.
func TestEditorTabShowsItsCloseButtonOnHover(t *testing.T) {
	size := ggui.Sz(260, 40)
	p := ggui.NewProbe(widget.Row(
		// Wide enough that its label ends well before its close button.
		ui.EditorTab("main.go", func() {}).OnClose(func() {}).MinWidth(120),
		ui.EditorTab("go.mod", func() {}).Selected(true),
	), size)
	defer p.Close()
	away := ggtest.Render(p, size, 1)
	closeAt, ok := p.Find("Close main.go")
	if !ok {
		t.Fatal("no close button")
	}
	tabAt, _ := p.Find("main.go")
	p.Move(tabAt.Center())
	ggtest.Render(p, size, 1) // the tint sets out from the first paint under the pointer
	p.Advance(time.Second)
	over := ggtest.Render(p, size, 1)
	icon := closeAt.Rect.Grow(-3) // the icon, inside the button's padding
	if !plain(away, icon) {
		t.Fatal("the close button shows with the pointer away")
	}
	if plain(over, icon) {
		t.Fatal("the close button did not show with the pointer over the tab")
	}
	if !changed(away, over, ggui.Rct(tabAt.Rect.Origin, ggui.Sz(6, tabAt.Rect.Size.H))) {
		t.Fatal("the tab under the pointer is not tinted")
	}
	empty := ggui.NewProbe(widget.Box(), size)
	defer empty.Close()
	selected, _ := p.Find("go.mod")
	if !changed(over, ggtest.Render(empty, size, 1), ggui.Rct(selected.Rect.Origin, ggui.Sz(6, selected.Rect.Size.H))) {
		t.Fatal("the selected tab is not raised")
	}
}

// A tab's tint under the pointer eases in and out, and holds as the
// pointer crosses from its label to its close button: the button under
// the pointer draws no hover of its own over the tab's, to come and go.
func TestEditorTabTintEasesAndHoldsOverItsCloseButton(t *testing.T) {
	size := ggui.Sz(200, 40)
	// Dark, as the tab's wash of the foreground shows best on it.
	p := ggui.NewProbe(theme.With(theme.Dark(), widget.Box(widget.Row(ui.EditorTab("main.go", func() {}).OnClose(func() {}))).Fill(color.NRGBA{25, 25, 25, 255})), size)
	defer p.Close()
	p.Advance(0)
	tab, _ := p.Find("main.go")
	closeAt, _ := p.Find("Close main.go")
	at := func() int {
		c := ggtest.Render(p, size, 1).RGBAAt(int(tab.Rect.Origin.X)+6, int(tab.Rect.Origin.Y)+3)
		return int(c.R) + int(c.G) + int(c.B)
	}
	rest := at()
	p.Move(tab.Center())
	// The tint sets out from the first paint under the pointer, at rest
	// still: it does not jump part of the way and ease the rest.
	if first := at(); first != rest {
		t.Fatalf("the first paint under the pointer: %d, not %d at rest", first, rest)
	}
	p.Advance(30 * time.Millisecond)
	part := at()
	p.Advance(time.Second)
	full := at()
	if !(rest < part && part < full) {
		t.Fatalf("at rest %d, a moment in %d, in full %d: the tint does not ease in", rest, part, full)
	}
	label := tab.Center()
	for i := range 10 {
		x := label.X + (closeAt.Center().X-label.X)*float64(i+1)/10
		p.Move(ggui.Pt(x, label.Y))
		p.Advance(16 * time.Millisecond)
		if got := at(); got != full {
			t.Fatalf("crossing to the close button, at %v: %d, not %d", x, got, full)
		}
	}
	p.Move(ggui.Pt(size.W-2, size.H-2))
	at()
	p.Advance(30 * time.Millisecond)
	if got := at(); got <= rest || got >= full {
		t.Fatalf("a moment after the pointer left: %d, between %d and %d", got, rest, full)
	}
	p.Advance(time.Second)
	if got := at(); got != rest {
		t.Fatalf("after the pointer left: %d, not %d", got, rest)
	}
}

// plain reports whether img is one color all over r, as a fill is and an
// icon drawn over it is not.
func plain(img *image.RGBA, r ggui.Rect) bool {
	first := img.RGBAAt(int(r.Origin.X), int(r.Origin.Y))
	for y := int(r.Origin.Y); y < int(r.Origin.Y+r.Size.H); y++ {
		for x := int(r.Origin.X); x < int(r.Origin.X+r.Size.W); x++ {
			if img.RGBAAt(x, y) != first {
				return false
			}
		}
	}
	return true
}

// changed reports whether a and b differ inside r.
func changed(a, b *image.RGBA, r ggui.Rect) bool {
	for y := int(r.Origin.Y); y < int(r.Origin.Y+r.Size.H); y++ {
		for x := int(r.Origin.X); x < int(r.Origin.X+r.Size.W); x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				return true
			}
		}
	}
	return false
}

// clipTo paints its child only within clip, as a frame painted in part
// does.
type clipTo struct {
	child ggui.Widget
	clip  ggui.Rect
}

func (c *clipTo) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size { return c.child.Layout(cs, env) }
func (c *clipTo) Paint(dst *ggui.Canvas, r ggui.Rect)                { dst.Clip(c.clip).Paint(c.child, r) }

// A group's label keeps to the Scroll's window, not to the part of a frame
// painted: repainting a tab far from the label, as the pointer over it
// does, draws there what a whole frame does, not the label come to it.
func TestTabGroupLabelKeepsToTheWindowInAFramePaintedInPart(t *testing.T) {
	size := ggui.Sz(400, 40)
	bar := func() ggui.Widget {
		var tabs []ggui.Widget
		for _, n := range []string{"one", "two", "three", "four"} {
			tabs = append(tabs, ui.EditorTab(n, func() {}))
		}
		g := ui.TabGroup(tabs...).Label(widget.Text("group")).Tint(color.NRGBA{0x6a, 0xa7, 0xff, 0xff})
		return widget.Box(widget.Scroll(widget.Row(g)).Horizontal().Bar(color.Transparent)).Fill(color.NRGBA{25, 25, 25, 255})
	}
	whole := ggui.NewProbe(theme.With(theme.Dark(), bar()), size)
	defer whole.Close()
	part := ggui.Rct(ggui.Pt(250, 0), ggui.Sz(150, 40))
	clipped := ggui.NewProbe(theme.With(theme.Dark(), &clipTo{child: bar(), clip: part}), size)
	defer clipped.Close()
	if changed(ggtest.Render(whole, size, 1), ggtest.Render(clipped, size, 1), part) {
		t.Fatal("the part painted differs from the whole frame's: the label followed the clip")
	}
}
