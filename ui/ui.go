// Package ui is the standard control set for ggui: Button, Checkbox, Radio,
// Switch, Slider, TextField and Divider. Each binds to a StateValue the way
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
	"cmp"
	"fmt"
	"image/color"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/fn"
)

const (
	sliderKnob    = 8
	switchWidth   = 36
	switchHeight  = 20
	defaultStripe = 160 // a slider's width when nothing bounds it
)

// squareGlyph is the box a checkbox or radio draws itself in.
func squareGlyph(t ggui.Theme) ggui.Size { return ggui.Sz(t.ControlSize, t.ControlSize) }

// sprint is the default option label.
func sprint[T any](v T) string { return fmt.Sprint(v) }

// pick, bounded and clamp come from internal/fn, shared with ggui: see the
// package comment there for why clamp's contradicting-bounds case matters.
func pick[T any](cond bool, a, b T) T { return fn.Pick(cond, a, b) }

func bounded(v, fallback float64) float64 { return fn.Bounded(v, fallback) }

func clamp[T cmp.Ordered](v, lo, hi T) T { return fn.Clamp(v, lo, hi) }

// channels returns c with its color scaled by f and its alpha by fa.
func channels(c color.Color, f, fa float64) color.Color {
	if c == nil {
		return nil
	}
	r, g, b, a := c.RGBA()
	scale := func(v uint32, by float64) uint8 { return uint8(clamp(float64(v>>8)*by, 0, 255)) }
	return color.RGBA{scale(r, f), scale(g, f), scale(b, f), scale(a, fa)}
}

// tint darkens (f < 1) or lightens (f > 1) an opaque color.
func tint(c color.Color, f float64) color.Color { return channels(c, f, 1) }

// fade dims a color toward nothing. Every channel goes, alpha included,
// because a premultiplied color stays premultiplied only if they all do.
func fade(c color.Color, f float64) color.Color { return channels(c, f, f) }

// edgeFade paints a 12px gradient from col at one edge of r to transparent.
// (dx, dy) points inward from the opaque edge: (1, 0) fades from the left,
// (-1, 0) from the right, (0, -1) from the bottom.
func edgeFade(dst *ggui.Canvas, r ggui.Rect, dx, dy float64, col color.Color, strength float64) {
	const width = 12
	for i := 0; i < width; i++ {
		alpha := strength * (1 - float64(i)/width)
		strip := ggui.Rct(r.Origin, ggui.Sz(r.Size.W, 1.0))
		switch {
		case dx > 0:
			strip = ggui.Rct(r.Origin.Add(ggui.Pt(float64(i), 0.0)), ggui.Sz(1.0, r.Size.H))
		case dx < 0:
			strip = ggui.Rct(r.Origin.Add(ggui.Pt(r.Size.W-float64(i)-1, 0.0)), ggui.Sz(1.0, r.Size.H))
		case dy > 0:
			strip = ggui.Rct(r.Origin.Add(ggui.Pt(0.0, float64(i))), ggui.Sz(r.Size.W, 1.0))
		default:
			strip = ggui.Rct(r.Origin.Add(ggui.Pt(0.0, r.Size.H-float64(i)-1)), ggui.Sz(r.Size.W, 1.0))
		}
		dst.FillRect(strip, fade(col, alpha))
	}
}

// setChanged stores v in s if it differs and then reports it to fn, which
// may be nil. It is the shape every control's OnChange follows.
func setChanged[T comparable](s ggui.Binding[T], v T, fn func(T)) {
	if ggui.Untrack(s.Get) == v {
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

// pointerMotion reports whether the pointer actually moved. PointerMove is
// synthesized every frame, so hover competing with a keyboard highlight has to
// ignore a pointer that is standing still.
type pointerMotion struct {
	last ggui.Point
	seen bool
}

// moved reports whether the pointer is somewhere new; a first sighting counts.
func (m *pointerMotion) moved(p ggui.Point) bool {
	was, seen := m.last, m.seen
	m.last, m.seen = p, true
	return !seen || was != p
}

// drifted is moved without the first sighting, so a menu opened from the
// keyboard is not stolen by a pointer that has not actually moved.
func (m *pointerMotion) drifted(p ggui.Point) bool {
	seen := m.seen
	return m.moved(p) && seen
}

// hoverPick is the pointer body a group's items share: the hovered index
// follows the pointer, a left click picks, and the wheel passes through to
// whatever scrolls behind. A tab strip, a toggle group and a sidebar all
// track which of their rows the pointer is on this way.
func hoverPick(ev ggui.PointerEvent, i int, hover *int, pick func()) bool {
	switch ev.Kind {
	case ggui.PointerEnter, ggui.PointerMove:
		*hover = i
	case ggui.PointerExit:
		if *hover == i {
			*hover = -1
		}
	case ggui.PointerTap:
		if ev.Button == ggui.MouseButtonLeft {
			pick()
		}
	case ggui.PointerScroll:
		return false
	}
	return true
}

func mix(a, b color.Color, amount float64) color.Color {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	blend := func(x, y uint32) uint8 { return uint8((float64(x)*(1-amount) + float64(y)*amount) / 257) }
	return color.RGBA{blend(ar, br), blend(ag, bg), blend(ab, bb), blend(aa, ba)}
}
