package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// ButtonWidget is a clickable box with a label. Build one with Button.
type ButtonWidget struct {
	label     *ggui.TextWidget
	box       *ggui.BoxWidget
	onTap     func()
	secondary bool
	disabled  bool
	padded    bool

	hovered, pressed bool
	focus            focusState
	theme            ggui.Theme
}

// Button creates a primary button: Accent background, OnAccent label.
func Button(label string, onTap func()) *ButtonWidget {
	b := &ButtonWidget{onTap: onTap, label: ggui.Text(label).NoWrap()}
	b.box = ggui.Box(b.label)
	return b
}

// ButtonOf creates a button around any content instead of a text label.
func ButtonOf(child ggui.Widget, onTap func()) *ButtonWidget {
	return &ButtonWidget{onTap: onTap, box: ggui.Box(child)}
}

// Secondary makes the button quiet: Surface background with a border and
// the normal text color, for actions that are not the main one.
func (b *ButtonWidget) Secondary() *ButtonWidget { b.secondary = true; return b }

// Disabled greys the button out and ignores the pointer while v is true.
func (b *ButtonWidget) Disabled(v bool) *ButtonWidget { b.disabled = v; return b }

// Pad overrides the theme's padding, with the shorthand Insets accepts.
func (b *ButtonWidget) Pad(sides ...float64) *ButtonWidget {
	b.box.Pad(sides...)
	b.padded = true
	return b
}

// Layout implements Widget.
func (b *ButtonWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	b.theme = t
	if !b.padded {
		b.box.Pad(t.Space*0.75, t.Space*2)
	}
	b.box.Radius(t.Radius)
	if b.label != nil {
		switch {
		case b.disabled:
			b.label.Color(t.Muted)
		case b.secondary:
			b.label.Color(t.Fg)
		default:
			b.label.Color(t.OnAccent)
		}
	}
	return b.box.Layout(c, env)
}

// Paint implements Widget.
func (b *ButtonWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := b.theme
	var fill color.Color
	b.box.Border(0, nil)
	switch {
	case b.disabled:
		fill = t.Surface
	case b.secondary:
		fill = pick(b.hovered, t.Border, t.Surface)
		b.box.Border(1, t.Border)
	default:
		fill = pick(b.hovered, t.AccentHover, t.Accent)
	}
	if b.pressed && !b.disabled {
		fill = tint(fill, pressTint)
	}
	b.box.Fill(fill)
	if !b.disabled {
		dst.HitPointer(r, b)
		dst.HitKey(r, b)
		dst.HitCursor(r, ebiten.CursorShapePointer)
	}
	dst.Paint(b.box, r)
	b.focus.paintRing(dst, r, t.Radius, t)
}

// HandleKey implements KeyHandler: Space or Enter presses the button.
func (b *ButtonWidget) HandleKey(ev ggui.KeyEvent) {
	b.focus.handle(ev)
	if activates(ev) && b.onTap != nil {
		b.onTap()
	}
}

// Adopt implements ggui.Adopter: hover and press carry across a rebuild.
func (b *ButtonWidget) Adopt(prev any) {
	if p, ok := prev.(*ButtonWidget); ok {
		b.hovered, b.pressed, b.focus = p.hovered, p.pressed, p.focus
	}
}

// HandlePointer implements PointerHandler.
func (b *ButtonWidget) HandlePointer(ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerEnter, ggui.PointerMove:
		b.hovered = true
	case ggui.PointerExit:
		b.hovered, b.pressed = false, false
	case ggui.PointerDown:
		b.pressed = ev.Button == ebiten.MouseButtonLeft
	case ggui.PointerUp:
		b.pressed = false
	case ggui.PointerTap:
		if ev.Button == ebiten.MouseButtonLeft && b.onTap != nil {
			b.onTap()
		}
	case ggui.PointerScroll:
		return false
	}
	return true
}
