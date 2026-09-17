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
	return ev.Kind == ggui.KeyPress && (ev.Key == ebiten.KeySpace || ev.Key == ebiten.KeyEnter || ev.Key == ebiten.KeyNumpadEnter)
}

// paintRing outlines r in the accent color when focus is visible.
func (f *focusState) paintRing(dst *ggui.Canvas, r ggui.Rect, radius float64, t ggui.Theme) {
	if !f.focused || !f.ring {
		return
	}
	ring := ggui.Rct(r.Origin.Add(ggui.Pt(-2, -2)), ggui.Sz(r.Size.W+4, r.Size.H+4))
	dst.StrokeRoundRect(ring, radius+2, 2, t.Accent)
}

// control is a widget that takes both pointer and keyboard input.
type control interface {
	ggui.PointerHandler
	ggui.KeyHandler
}

// interactive is the input state every control shares: whether it takes
// input, hover, press and keyboard focus. A control embeds it and calls
// hit, pointer and key from its Paint and handlers; Adopt then carries the
// state across a rebuild for free.
type interactive struct {
	disabled         bool
	hovered, pressed bool
	focus            focusState
}

func (s *interactive) state() *interactive { return s }

// hit registers r for pointer, keys and the cursor, unless disabled.
func (s *interactive) hit(dst *ggui.Canvas, r ggui.Rect, h control, cursor ebiten.CursorShapeType) {
	if s.disabled {
		return
	}
	dst.HitPointer(r, h)
	dst.HitKey(r, h)
	dst.HitCursor(r, cursor)
}

// pointer tracks hover and press and calls onTap on a left click. Press
// is cleared by PointerUp, not by leaving, so a drag that strays outside
// keeps going. Scroll is left to whatever is behind.
func (s *interactive) pointer(ev ggui.PointerEvent, onTap func()) bool {
	switch ev.Kind {
	case ggui.PointerEnter, ggui.PointerMove:
		s.hovered = true
	case ggui.PointerExit:
		s.hovered = false
	case ggui.PointerDown:
		s.pressed = ev.Button == ebiten.MouseButtonLeft
	case ggui.PointerUp:
		s.pressed = false
	case ggui.PointerTap:
		if ev.Button == ebiten.MouseButtonLeft && onTap != nil {
			onTap()
		}
	case ggui.PointerScroll:
		return false
	}
	return true
}

// key tracks focus and calls onActivate for Space or Enter.
func (s *interactive) key(ev ggui.KeyEvent, onActivate func()) {
	s.focus.handle(ev)
	if activates(ev) && onActivate != nil {
		onActivate()
	}
}

// Adopt implements ggui.Adopter: hover, press and focus carry across a
// rebuild. Disabled is the new widget's to decide.
func (s *interactive) Adopt(prev any) {
	if p, ok := prev.(interface{ state() *interactive }); ok {
		q := p.state()
		s.hovered, s.pressed, s.focus = q.hovered, q.pressed, q.focus
	}
}

// motion eases a value retained on the Canvas toward target and returns
// where it is now. The Motion lives in the Canvas under r and slot rather
// than in the widget, so a control rebuilt every frame keeps animating
// without an Adopt.
// Each widget declares its own slot, a `new(byte)` var, per animated value.
func motion(dst *ggui.Canvas, r ggui.Rect, slot any, target float64) float64 {
	now := ggui.Now()
	m, ok := dst.Retained(r, slot).(*ggui.Motion)
	if !ok {
		m = new(ggui.Motion)
	}
	m.MoveTo(target, now, knobDuration)
	dst.Retain(r, slot, m)
	return m.Value(now)
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
