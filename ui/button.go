package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// ButtonWidget is a clickable box with a label. Build one with Button.
type ButtonWidget struct {
	interactive
	label     *ggui.TextWidget
	box       *ggui.BoxWidget
	onTap     func()
	secondary bool
	padded    bool
	theme     ggui.Theme
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
		b.box.Padding(t.ButtonPad)
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
	if b.pressed && b.hovered && !b.disabled {
		fill = tint(fill, pressTint)
	}
	b.box.Fill(fill)
	b.hit(dst, r, b, ebiten.CursorShapePointer)
	dst.Paint(b.box, r)
	b.focus.paintRing(dst, r, t.Radius, t)
}

// HandleKey implements KeyHandler: Space or Enter presses the button.
func (b *ButtonWidget) HandleKey(ev ggui.KeyEvent) { b.key(ev, b.onTap) }

// HandlePointer implements PointerHandler.
func (b *ButtonWidget) HandlePointer(ev ggui.PointerEvent) bool { return b.pointer(ev, b.onTap) }
