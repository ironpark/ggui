package main

import (
	"math"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// dial is a rotary control over a float in [0, 1]. Interactive supplies
// hover, press, focus, identity across rebuilds and the semantics node; the
// dial adds the geometry, the drawing and what a drag or an arrow key does.
type dial struct {
	ggui.Interactive
	value  *ggui.StateValue[float64]
	needle *ggui.Sprung[float64] // eases toward value; Paint reads it
	theme  uitheme.Theme
	rect   ggui.Rect
}

func newDial(value *ggui.StateValue[float64], name string) *dial {
	d := &dial{value: value, needle: ggui.Spring(ggui.Untrack(value.Get))}
	d.Role = ggui.RoleSlider
	d.SetName(name)
	d.AutoKey()
	return d
}

// Layout resolves the theme now, because Paint receives only the canvas.
func (d *dial) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	d.Sync()
	d.theme = uitheme.From(env)
	s := min(c.MaxW, c.MaxH, 160)
	return c.Constrain(ggui.Sz(s, s))
}

// Paint registers the hit region through Hit and draws the ring, the arc
// the needle has swept and the needle itself at the spring's position.
func (d *dial) Paint(dst *ggui.Canvas, r ggui.Rect) {
	d.rect = r
	d.Hit(dst, r, d, ggui.CursorShapePointer)
	t := d.theme
	c := r.Center()
	radius := min(r.Size.W, r.Size.H)/2 - 4
	ring := t.Border
	if d.Hovered || d.Pressed {
		ring = t.Ring
	}
	dst.FillCircle(c, radius, t.Card)
	dst.StrokeRoundRect(ggui.Rct(ggui.Pt(c.X-radius, c.Y-radius), ggui.Sz(2*radius, 2*radius)), radius, 2, ring)
	// The spring moves a little every frame until it settles; the runtime
	// repaints each frame, so reading it here is enough to animate.
	angle := angleOf(d.needle.Get())
	tip := ggui.Pt(c.X+math.Cos(angle)*(radius-10), c.Y+math.Sin(angle)*(radius-10))
	dst.StrokeLine(c, tip, 3, t.Primary)
	dst.FillCircle(c, 5, t.Primary)
	d.FocusRing(dst, r, radius+4, t.Ring)
}

// angleOf maps [0, 1] onto the 270° sweep from 7 o'clock to 5 o'clock.
func angleOf(v float64) float64 { return (0.75 + 1.5*v) * math.Pi }

// set moves the value to where p points from the centre and retargets the
// needle; the spring, not the pointer, decides where it is drawn.
func (d *dial) set(p ggui.Point) {
	c := d.rect.Center()
	a := math.Atan2(p.Y-c.Y, p.X-c.X)/math.Pi - 0.75 // turns past 7 o'clock
	for a < 0 {
		a += 2
	}
	d.write(min(a/1.5, 1))
}

func (d *dial) write(v float64) {
	v = max(0, min(v, 1))
	d.value.Set(v)
	d.needle.Set(v)
}

// HandlePointer implements ggui.PointerHandler. Pointer keeps hover and
// press current; a press or a drag anywhere while pressed sets the value.
func (d *dial) HandlePointer(ev ggui.PointerEvent) bool {
	handled := d.Pointer(ev, nil)
	if ev.Kind == ggui.PointerDown || ev.Kind == ggui.PointerDrag {
		d.set(ev.Pos)
	}
	return handled
}

// HandleKey implements ggui.KeyHandler: arrows step the value.
func (d *dial) HandleKey(ev ggui.KeyEvent) {
	d.Keyboard(ev, nil)
	if ev.Kind != ggui.KeyPress {
		return
	}
	switch ev.Key {
	case ggui.KeyArrowLeft, ggui.KeyArrowDown:
		d.write(ggui.Untrack(d.value.Get) - 0.05)
	case ggui.KeyArrowRight, ggui.KeyArrowUp:
		d.write(ggui.Untrack(d.value.Get) + 0.05)
	}
}

// ConsumesKey implements ggui.KeyConsumer, so a bare-key shortcut on an
// arrow yields to a focused dial.
func (d *dial) ConsumesKey(ev ggui.KeyEvent) bool {
	switch ev.Key {
	case ggui.KeyArrowLeft, ggui.KeyArrowRight, ggui.KeyArrowUp, ggui.KeyArrowDown:
		return true
	}
	return false
}

// Describe implements ggui.Describer: assistive technology sees a slider.
func (d *dial) Describe() ggui.Node {
	return ggui.Node{
		Role:     ggui.RoleSlider,
		Name:     d.SemanticName(),
		Min:      0,
		Max:      1,
		Now:      ggui.Untrack(d.value.Get),
		Disabled: d.IsInert(),
		Actions:  ggui.ActionFocus,
	}
}
