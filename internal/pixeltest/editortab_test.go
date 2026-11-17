// Package pixeltest holds tests that read what widgets paint, under
// ggtest.Main.
package pixeltest

import (
	"image"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ggtest"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/widget"
)

func TestMain(m *testing.M) { ggtest.Main(m) }

// An editor tab's close button shows only while the pointer is over the
// tab, which it tints; the selected tab is raised all the while.
func TestEditorTabShowsItsCloseButtonOnHover(t *testing.T) {
	size := ggui.Sz(200, 40)
	p := ggui.NewProbe(widget.Row(
		ui.EditorTab("main.go", func() {}).OnClose(func() {}),
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
