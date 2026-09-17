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
	"fmt"
	"image/color"
	"math"
	"time"

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

// sprint is the default option label.
func sprint[T any](v T) string { return fmt.Sprint(v) }

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

// setChanged stores v in s if it differs and then reports it to fn, which
// may be nil. It is the shape every control's OnChange follows.
func setChanged[T comparable](s *ggui.Signal[T], v T, fn func(T)) {
	if s.Peek() == v {
		return
	}
	s.Set(v)
	if fn != nil {
		fn(v)
	}
}

// stepIndex moves cur by dir through n items, wrapping around and skipping
// items enabled rejects; from -1 it starts at the near end. It returns cur
// when nothing is enabled.
func stepIndex(cur, dir, n int, enabled func(int) bool) int {
	i := cur
	for range n {
		if i < 0 {
			i = pick(dir < 0, n-1, 0)
		} else {
			i = (i + dir + n) % n
		}
		if enabled == nil || enabled(i) {
			return i
		}
	}
	return cur
}
