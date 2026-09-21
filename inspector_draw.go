//go:build ggui_inspector

package ggui

import (
	"image"
	"image/color"

	"sync"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/gomono"
)

// Every place the inspector reaches past ggui's public API to draw itself.
//
// It cannot draw the way a widget does. App.Draw leaves frameState.tracing
// on while the inspector paints, and Canvas.Inert keeps the frame's trace
// rather than detaching it, so a label painted through dst.Paint(Text(...))
// would be appended to the very trace the inspector is rendering. An
// overlay has to draw imperatively, and ggui exposes no imperative text,
// no *Font-to-text.Face, no inverse of Canvas.Px and no path fill.
//
// Collecting those reaches here keeps the rest of the inspector on public
// calls, and makes this file the exact list of what ggui would have to
// export were the inspector ever moved into a package of its own.

var inspectFontOnce sync.Once
var inspectFont *Font

// inspectorFace is the inspector's own monospaced face, sized in physical
// pixels. It falls back to ggui's default font if gomono fails to load.
func inspectorFace(dst *Canvas) text.Face {
	inspectFontOnce.Do(func() { inspectFont, _ = LoadFont(gomono.TTF) })
	if inspectFont == nil {
		return fallbackFont().face(12 * dst.Scale())
	}
	return inspectFont.face(12 * dst.Scale())
}

// textWidth measures s in logical pixels.
func textWidth(dst *Canvas, face text.Face, s string) float64 { return dst.dp(lineWidth(s, face)) }

// drawLine draws one unwrapped line with its top-left at the logical (x, y).
func drawLine(dst *Canvas, face text.Face, s string, x, y float64, col color.Color) {
	if dst == nil || dst.Image == nil {
		return
	}
	op := &text.DrawOptions{}
	op.ColorScale.ScaleWithColor(col)
	op.GeoM.Translate(dst.px(x), dst.px(y))
	drawText(dst.Image, s, face, op)
}

// fillPath fills a path already built in physical pixels; Canvas exposes
// only the rounded-rect and circle shapes, and the disclosure arrows are
// neither.
func fillPath(dst *Canvas, p *vector.Path, col color.Color) {
	vector.FillPath(dst.Image, p, &vector.FillOptions{}, pathOptions(col))
}

// panelBounds is r in the physical pixels the panel's cached image is
// allocated in.
func panelBounds(dst *Canvas, r Rect) image.Rectangle { return dst.physical(r) }
