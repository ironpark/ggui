package ggui

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// paintInspector draws the frame's trace: an outline for every widget,
// colored by depth, and a highlight for the deepest widget under the
// cursor, named and followed by the accessibility node there with every
// node it is inside of.
func paintInspector(dst *Canvas) {
	palette := []color.Color{
		color.RGBA{0xe5, 0x39, 0x35, 0xff}, color.RGBA{0xfb, 0x8c, 0x00, 0xff},
		color.RGBA{0x43, 0xa0, 0x47, 0xff}, color.RGBA{0x1e, 0x88, 0xe5, 0xff},
		color.RGBA{0x8e, 0x24, 0xaa, 0xff}, color.RGBA{0x00, 0x89, 0x7b, 0xff},
	}
	var hit *traceEntry
	p, ok := dst.Pointer()
	for i := range dst.trace {
		e := &dst.trace[i]
		dst.StrokeRoundRect(e.rect, 0, 1, palette[e.depth%len(palette)])
		if ok && e.rect.Contains(p) && (hit == nil || e.depth >= hit.depth) {
			hit = e
		}
	}
	if hit == nil {
		return
	}
	dst.FillRect(hit.rect, color.RGBA{0x1e, 0x88, 0xe5, 0x40})
	lines := append([]string{fmt.Sprintf("%s  %g×%g  @ %g,%g", hit.name, hit.rect.Size.W, hit.rect.Size.H, hit.rect.Origin.X, hit.rect.Origin.Y)},
		semanticChain(dst, p)...)
	face := fallbackFont().face(11 * dst.Scale())
	var w float64
	for _, line := range lines {
		w = max(w, dst.dp(text.Advance(line, face)))
	}
	m := face.Metrics()
	lh := dst.dp(m.HAscent + m.HDescent)
	h := lh * float64(len(lines))
	const pad = 4
	at := Pt(hit.rect.Origin.X, hit.rect.Origin.Y+hit.rect.Size.H+2)
	if size := dst.Size(); size != (Size{}) {
		at.X = clamp(at.X, 0, max(size.W-w-2*pad, 0))
		if at.Y+h+2*pad > size.H {
			at.Y = max(hit.rect.Origin.Y-h-2*pad-2, 0)
		}
	}
	dst.FillRoundRect(Rct(at, Sz(w+2*pad, h+2*pad)), 3, color.RGBA{0x1f, 0x23, 0x28, 0xe6})
	op := &text.DrawOptions{}
	op.ColorScale.ScaleWithColor(color.White)
	for i, line := range lines {
		op.GeoM.Reset()
		op.GeoM.Translate(dst.px(at.X+pad), dst.px(at.Y+pad+float64(i)*lh))
		text.Draw(dst.Image, line, face, op)
	}
}

// semanticChain describes the accessibility node under p and everything it
// is inside of, outermost first and indented, which is what a screen reader
// walks: the single role and label the inspector used to show could not say
// that a tab is inside a strip or an option inside its combobox.
func semanticChain(dst *Canvas, p Point) []string {
	found := -1
	for i := len(dst.sem) - 1; i >= 0; i-- {
		if !dst.sem[i].node.Offscreen && dst.sem[i].rect.Contains(p) {
			found = i
			break
		}
	}
	if found < 0 {
		return nil
	}
	var chain []int
	for i := found; i >= 0; i = dst.sem[i].parent - 1 {
		chain = append(chain, i)
	}
	lines := make([]string, 0, len(chain))
	for i := len(chain) - 1; i >= 0; i-- {
		e := &dst.sem[chain[i]]
		n := SemNode{Node: e.node}
		lines = append(lines, fmt.Sprintf("%s%s %q%s",
			strings.Repeat("  ", len(chain)-1-i), e.node.Role, e.node.Name, n.flags()))
	}
	return lines
}
