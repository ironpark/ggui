package pixeltest

import (
	"image/color"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ggtest"
	"github.com/ironpark/ggui/widget"
)

// A Scroll's faded end eases out of what it fades, from the content as it
// is to the background, alike down every row: no step where the fade
// begins, as there was along its top and bottom rows, where a line
// running into it showed cut.
func TestFadedEdgesEaseOutAlikeDownTheirHeight(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		size := ggui.Sz(200, 30)
		content := widget.Box().Size(1000, 30).Fill(color.NRGBA{0xff, 0xff, 0xff, 0x52})
		s := widget.Scroll(content).Horizontal().FadeEdges(28)
		p := ggui.NewProbe(widget.Box(s).Fill(color.NRGBA{25, 25, 25, 255}), size)
		img := ggtest.Render(p, size, scale)
		p.Close()
		b := img.Bounds()
		fade := int(28 * scale)
		start := b.Max.X - fade
		at := func(x, y int) int { return int(img.RGBAAt(x, y).R) }
		middle := b.Dy() / 2
		for y := b.Min.Y; y < b.Max.Y; y++ {
			before := at(start-1, y)
			if d := before - at(start, y); d > 2 || d < 0 {
				t.Errorf("scale %v row %d: %d before the fade, %d where it starts", scale, y, before, at(start, y))
			}
			for x := start; x < b.Max.X; x++ {
				if at(x, y) > at(x-1, y) {
					t.Errorf("scale %v row %d: brightens at %d", scale, y, x)
				}
				if d := at(x, y) - at(x, middle); d > 1 || d < -1 {
					t.Errorf("scale %v row %d at %d: %d, the middle row %d", scale, y, x, at(x, y), at(x, middle))
					break
				}
			}
			if last := at(b.Max.X-1, y); last > 27 {
				t.Errorf("scale %v row %d: the end is %d, not the background's 25", scale, y, last)
			}
		}
	}
}
