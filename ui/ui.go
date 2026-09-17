// Package ui is the standard control set for ggui: Button, Checkbox, Radio,
// Switch, Slider, TextField and Divider. Each binds to a Signal the way
// Svelte's bind: does, takes its look from the Theme in its Env at layout
// time, and keeps hover and press state in the widget itself, so nothing
// rebuilds for a hover. Reading a bound signal happens in Paint, which runs
// every frame, so a control shows the signal's current value without
// subscribing.
//
// The package uses only ggui's public API, so a control set of your own can
// be built the same way.
package ui

import (
	"image/color"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

const (
	controlSize   = 18 // checkbox and radio glyphs
	controlGap    = 8  // between a glyph and its label
	sliderKnob    = 8
	switchWidth   = 36
	switchHeight  = 20
	pressTint     = 0.85
	knobDuration  = 150 * time.Millisecond
	defaultStripe = 160 // a slider's width when nothing bounds it
)

func pick[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

func bounded(v, fallback float64) float64 {
	if math.IsInf(v, 1) {
		return fallback
	}
	return v
}

func clamp(v, lo, hi float64) float64 { return min(max(v, lo), hi) }

// tint darkens (f < 1) or lightens (f > 1) an opaque color.
func tint(c color.Color, f float64) color.Color {
	if c == nil {
		return nil
	}
	r, g, b, a := c.RGBA()
	scale := func(v uint32) uint8 { return uint8(clamp(float64(v>>8)*f, 0, 255)) }
	return color.RGBA{scale(r), scale(g), scale(b), uint8(a >> 8)}
}

// focusState is the keyboard focus a control holds, and whether to show it:
// a ring is drawn only for focus that arrived by keyboard.
type focusState struct {
	focused bool
	ring    bool
}

func (f *focusState) handle(ev ggui.KeyEvent) {
	switch ev.Kind {
	case ggui.KeyFocus:
		f.focused, f.ring = true, ev.Key == ebiten.KeyTab
	case ggui.KeyBlur:
		f.focused, f.ring = false, false
	}
}

// activates reports whether ev is the key press that triggers a control.
func activates(ev ggui.KeyEvent) bool {
	return ev.Kind == ggui.KeyPress && (ev.Key == ebiten.KeySpace || ev.Key == ebiten.KeyEnter)
}

// paintRing outlines r in the accent color when focus is visible.
func (f *focusState) paintRing(dst *ggui.Canvas, r ggui.Rect, radius float64, t ggui.Theme) {
	if !f.focused || !f.ring {
		return
	}
	ring := ggui.Rct(r.Origin.Add(ggui.Pt(-2, -2)), ggui.Sz(r.Size.W+4, r.Size.H+4))
	dst.StrokeRoundRect(ring, radius+2, 2, t.Accent)
}
