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

// ButtonWidget is a clickable box with a label. Build one with Button.
type ButtonWidget struct {
	label     *ggui.TextWidget
	box       *ggui.BoxWidget
	onTap     func()
	secondary bool
	disabled  bool
	padded    bool

	hovered, pressed bool
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
		dst.HitCursor(r, ebiten.CursorShapePointer)
	}
	b.box.Paint(dst, r)
}

// Adopt implements ggui.Adopter: hover and press carry across a rebuild.
func (b *ButtonWidget) Adopt(prev any) {
	if p, ok := prev.(*ButtonWidget); ok {
		b.hovered, b.pressed = p.hovered, p.pressed
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

// toggle is the shared body of Checkbox, Radio and Switch: a glyph, an
// optional label, hover and tap.
type toggle struct {
	label    *ggui.TextWidget
	disabled bool
	hovered  bool
	onTap    func()

	labelSize ggui.Size
	glyph     ggui.Size
	theme     ggui.Theme
}

func (g *toggle) layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	g.theme = env.Theme()
	size := g.glyph
	if g.label != nil {
		g.label.Color(pick[color.Color](g.disabled, g.theme.Muted, nil))
		g.labelSize = g.label.Layout(ggui.Loose(ggui.Sz(max(c.MaxW-size.W-controlGap, 0), c.MaxH)), env)
		size.W += controlGap + g.labelSize.W
		size.H = max(size.H, g.labelSize.H)
	}
	return c.Constrain(size)
}

// paint registers the region and paints the label, and returns the Rect the
// glyph should be drawn in.
func (g *toggle) paint(dst *ggui.Canvas, r ggui.Rect, handler ggui.PointerHandler) ggui.Rect {
	if !g.disabled {
		dst.HitPointer(r, handler)
		dst.HitCursor(r, ebiten.CursorShapePointer)
	}
	if g.label != nil {
		at := ggui.Pt(r.Origin.X+g.glyph.W+controlGap, r.Origin.Y+(r.Size.H-g.labelSize.H)/2)
		g.label.Paint(dst, ggui.Rct(at, g.labelSize))
	}
	return ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+(r.Size.H-g.glyph.H)/2), g.glyph)
}

func (g *toggle) state() *toggle { return g }

// Adopt implements ggui.Adopter: hover carries across a rebuild.
func (g *toggle) Adopt(prev any) {
	if p, ok := prev.(interface{ state() *toggle }); ok {
		g.hovered = p.state().hovered
	}
}

func (g *toggle) handle(ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerEnter, ggui.PointerMove:
		g.hovered = true
	case ggui.PointerExit:
		g.hovered = false
	case ggui.PointerTap:
		if ev.Button == ebiten.MouseButtonLeft && g.onTap != nil {
			g.onTap()
		}
	case ggui.PointerScroll:
		return false
	}
	return true
}

// CheckboxWidget is a box that is ticked while its signal is true. Build one
// with Checkbox.
type CheckboxWidget struct {
	toggle
	checked  *ggui.Signal[bool]
	onChange func(bool)
}

// Checkbox binds a tick box to checked; a click toggles it. label may be "".
func Checkbox(checked *ggui.Signal[bool], label string) *CheckboxWidget {
	c := &CheckboxWidget{checked: checked}
	c.glyph = ggui.Sz(controlSize, controlSize)
	if label != "" {
		c.label = ggui.Text(label)
	}
	c.onTap = func() {
		ggui.Toggle(checked)
		if c.onChange != nil {
			c.onChange(checked.Peek())
		}
	}
	return c
}

// Disabled greys the box out and ignores the pointer while v is true.
func (c *CheckboxWidget) Disabled(v bool) *CheckboxWidget { c.disabled = v; return c }

// OnChange fires with the new value after a click toggled it.
func (c *CheckboxWidget) OnChange(fn func(bool)) *CheckboxWidget { c.onChange = fn; return c }

// Layout implements Widget.
func (c *CheckboxWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	return c.layout(cs, env)
}

// Paint implements Widget.
func (c *CheckboxWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := c.theme
	box := c.paint(dst, r, c)
	on := c.checked.Peek()
	radius := t.Radius * 0.6
	switch {
	case c.disabled:
		dst.FillRoundRect(box, radius, t.Surface)
		dst.StrokeRoundRect(box, radius, 1, t.Border)
	case on:
		dst.FillRoundRect(box, radius, pick(c.hovered, t.AccentHover, t.Accent))
	default:
		dst.FillRoundRect(box, radius, t.Field)
		dst.StrokeRoundRect(box, radius, 1, pick(c.hovered, t.Accent, t.Border))
	}
	if on {
		at := func(x, y float64) ggui.Point {
			return ggui.Pt(box.Origin.X+x*box.Size.W, box.Origin.Y+y*box.Size.H)
		}
		col := pick(c.disabled, t.Muted, t.OnAccent)
		dst.StrokeLine(at(0.24, 0.52), at(0.43, 0.72), 2, col)
		dst.StrokeLine(at(0.41, 0.72), at(0.78, 0.30), 2, col)
	}
}

// HandlePointer implements PointerHandler.
func (c *CheckboxWidget) HandlePointer(ev ggui.PointerEvent) bool { return c.handle(ev) }

// RadioWidget is one option of a group that shares a signal. Build one with
// Radio.
type RadioWidget[T comparable] struct {
	toggle
	selected *ggui.Signal[T]
	value    T
}

// Radio creates a round option that is filled while selected holds value
// and selects it when clicked. Every Radio bound to the same signal is one
// group.
func Radio[T comparable](selected *ggui.Signal[T], value T, label string) *RadioWidget[T] {
	r := &RadioWidget[T]{selected: selected, value: value}
	r.glyph = ggui.Sz(controlSize, controlSize)
	if label != "" {
		r.label = ggui.Text(label)
	}
	r.onTap = func() { selected.Set(value) }
	return r
}

// Disabled greys the option out and ignores the pointer while v is true.
func (r *RadioWidget[T]) Disabled(v bool) *RadioWidget[T] { r.disabled = v; return r }

// Layout implements Widget.
func (r *RadioWidget[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size { return r.layout(c, env) }

// Paint implements Widget.
func (r *RadioWidget[T]) Paint(dst *ggui.Canvas, rect ggui.Rect) {
	t := r.theme
	box := r.paint(dst, rect, r)
	center := ggui.Pt(box.Origin.X+box.Size.W/2, box.Origin.Y+box.Size.H/2)
	radius := box.Size.W / 2
	on := r.selected.Peek() == r.value
	switch {
	case r.disabled:
		dst.FillCircle(center, radius, t.Border)
		dst.FillCircle(center, radius-1, t.Surface)
		if on {
			dst.FillCircle(center, radius*0.4, t.Muted)
		}
	case on:
		dst.FillCircle(center, radius, pick(r.hovered, t.AccentHover, t.Accent))
		dst.FillCircle(center, radius*0.4, t.OnAccent)
	default:
		dst.FillCircle(center, radius, pick(r.hovered, t.Accent, t.Border))
		dst.FillCircle(center, radius-1, t.Field)
	}
}

// HandlePointer implements PointerHandler.
func (r *RadioWidget[T]) HandlePointer(ev ggui.PointerEvent) bool { return r.handle(ev) }

// SwitchWidget is a sliding on/off toggle. Build one with Switch.
type SwitchWidget struct {
	toggle
	on   *ggui.Signal[bool]
	knob ggui.Motion
}

// Switch binds a toggle to on; a click flips it and the knob slides over.
func Switch(on *ggui.Signal[bool], label string) *SwitchWidget {
	s := &SwitchWidget{on: on}
	s.glyph = ggui.Sz(switchWidth, switchHeight)
	if label != "" {
		s.label = ggui.Text(label)
	}
	s.onTap = func() { ggui.Toggle(on) }
	return s
}

// Disabled greys the switch out and ignores the pointer while v is true.
func (s *SwitchWidget) Disabled(v bool) *SwitchWidget { s.disabled = v; return s }

// Layout implements Widget.
func (s *SwitchWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size { return s.layout(c, env) }

// Paint implements Widget.
func (s *SwitchWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := s.theme
	box := s.paint(dst, r, s)
	now := time.Now()
	on := s.on.Peek()
	s.knob.MoveTo(pick(on, 1.0, 0.0), now, knobDuration)
	k := s.knob.Value(now)
	track := pick(on, pick(s.hovered, t.AccentHover, t.Accent), pick(s.hovered, t.Muted, t.Border))
	if s.disabled {
		track = t.Border
	}
	dst.FillRoundRect(box, box.Size.H/2, track)
	radius := box.Size.H/2 - 2
	x := box.Origin.X + 2 + radius + k*(box.Size.W-4-2*radius)
	dst.FillCircle(ggui.Pt(x, box.Origin.Y+box.Size.H/2), radius, pick(s.disabled, t.Surface, t.Field))
}

// Adopt implements ggui.Adopter: the knob keeps sliding across a rebuild,
// which matters because flipping a switch often rebuilds what holds it.
func (s *SwitchWidget) Adopt(prev any) {
	s.toggle.Adopt(prev)
	if p, ok := prev.(*SwitchWidget); ok {
		s.knob = p.knob
	}
}

// HandlePointer implements PointerHandler.
func (s *SwitchWidget) HandlePointer(ev ggui.PointerEvent) bool { return s.handle(ev) }

// SliderWidget picks a number in a range by dragging a knob. Build one with
// Slider.
type SliderWidget struct {
	value    *ggui.Signal[float64]
	min, max float64
	step     float64
	disabled bool

	hovered, pressed bool
	theme            ggui.Theme
	rect             ggui.Rect
}

// Slider binds a horizontal slider to value, clamped to [lo, hi]. It fills
// the width it is given.
func Slider(value *ggui.Signal[float64], lo, hi float64) *SliderWidget {
	return &SliderWidget{value: value, min: lo, max: hi}
}

// Step snaps the value to multiples of s from the range's start.
func (s *SliderWidget) Step(step float64) *SliderWidget { s.step = step; return s }

// Disabled greys the slider out and ignores the pointer while v is true.
func (s *SliderWidget) Disabled(v bool) *SliderWidget { s.disabled = v; return s }

// Layout implements Widget.
func (s *SliderWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	s.theme = env.Theme()
	return c.Constrain(ggui.Sz(bounded(c.MaxW, defaultStripe), sliderKnob*2+4))
}

// fraction returns where value sits in the range, 0 to 1.
func (s *SliderWidget) fraction() float64 {
	if s.max == s.min {
		return 0
	}
	return clamp((s.value.Peek()-s.min)/(s.max-s.min), 0, 1)
}

// setFromX moves the value to where logical x falls on the track.
func (s *SliderWidget) setFromX(x float64) {
	track := s.rect.Size.W - 2*sliderKnob
	if track <= 0 {
		return
	}
	f := clamp((x-s.rect.Origin.X-sliderKnob)/track, 0, 1)
	v := s.min + f*(s.max-s.min)
	if s.step > 0 {
		v = s.min + math.Round((v-s.min)/s.step)*s.step
	}
	s.value.Set(clamp(v, min(s.min, s.max), max(s.min, s.max)))
}

// Paint implements Widget.
func (s *SliderWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := s.theme
	s.rect = r
	if !s.disabled {
		dst.HitPointer(r, s)
		dst.HitCursor(r, ebiten.CursorShapePointer)
	}
	cy := r.Origin.Y + r.Size.H/2
	x0, x1 := r.Origin.X+sliderKnob, r.Origin.X+r.Size.W-sliderKnob
	kx := x0 + (x1-x0)*s.fraction()
	dst.FillRoundRect(ggui.Rct(ggui.Pt(x0, cy-2), ggui.Sz(x1-x0, 4)), 2, t.Border)
	accent := pick(s.disabled, t.Muted, pick(s.hovered || s.pressed, t.AccentHover, t.Accent))
	dst.FillRoundRect(ggui.Rct(ggui.Pt(x0, cy-2), ggui.Sz(kx-x0, 4)), 2, accent)
	radius := float64(sliderKnob)
	if s.hovered || s.pressed {
		radius++
	}
	dst.FillCircle(ggui.Pt(kx, cy), radius, accent)
	dst.FillCircle(ggui.Pt(kx, cy), radius-3, t.Field)
}

// Adopt implements ggui.Adopter: a drag in progress carries across a rebuild.
func (s *SliderWidget) Adopt(prev any) {
	if p, ok := prev.(*SliderWidget); ok {
		s.hovered, s.pressed = p.hovered, p.pressed
	}
}

// HandlePointer implements PointerHandler.
func (s *SliderWidget) HandlePointer(ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerEnter, ggui.PointerMove:
		s.hovered = true
	case ggui.PointerExit:
		s.hovered = false
	case ggui.PointerDown:
		if ev.Button != ebiten.MouseButtonLeft {
			return false
		}
		s.pressed = true
		s.setFromX(ev.Pos.X)
	case ggui.PointerDrag:
		if s.pressed {
			s.setFromX(ev.Pos.X)
		}
	case ggui.PointerUp:
		s.pressed = false
	case ggui.PointerScroll:
		return false
	}
	return true
}

// TextFieldWidget is a TextInput in a themed box: Field background, a
// border that turns Accent while focused, the theme's padding and radius.
// Build one with TextField.
type TextFieldWidget struct {
	input *ggui.TextInputWidget
	box   *ggui.BoxWidget
	theme ggui.Theme
}

// TextField creates a text field bound to value.
func TextField(value *ggui.Signal[string]) *TextFieldWidget {
	f := &TextFieldWidget{input: ggui.TextInput(value)}
	f.box = ggui.Box(f.input)
	return f
}

// Placeholder sets the muted text shown while the value is empty.
func (f *TextFieldWidget) Placeholder(s string) *TextFieldWidget { f.input.Placeholder(s); return f }

// Password masks every rune with a bullet.
func (f *TextFieldWidget) Password() *TextFieldWidget { f.input.Password(); return f }

// MinWidth sets the width the field asks for when its parent leaves the
// width to it.
func (f *TextFieldWidget) MinWidth(w float64) *TextFieldWidget { f.input.MinWidth(w); return f }

// OnSubmit fires with the value when Enter is pressed.
func (f *TextFieldWidget) OnSubmit(fn func(string)) *TextFieldWidget { f.input.OnSubmit(fn); return f }

// OnChange fires with the value after every edit.
func (f *TextFieldWidget) OnChange(fn func(string)) *TextFieldWidget { f.input.OnChange(fn); return f }

// Input returns the editor inside, for Focused and the editor's own setters.
func (f *TextFieldWidget) Input() *ggui.TextInputWidget { return f.input }

// Layout implements Widget.
func (f *TextFieldWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	f.theme = t
	f.box.Pad(t.Space*0.75, t.Space).Radius(t.Radius).Fill(t.Field)
	return f.box.Layout(c, env)
}

// Paint implements Widget.
func (f *TextFieldWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	// The whole box, padding included, focuses and clicks into the editor.
	dst.HitPointer(r, f.input)
	dst.HitKey(r, f.input)
	dst.HitCursor(r, ebiten.CursorShapeText)
	f.box.Border(1, pick(f.input.Focused(), f.theme.Accent, f.theme.Border))
	f.box.Paint(dst, r)
}

// DividerWidget is a one-pixel line in the theme's Border color. Build one
// with Divider.
type DividerWidget struct {
	vertical bool
	theme    ggui.Theme
}

// Divider creates a horizontal rule that fills the width it is given.
func Divider() *DividerWidget { return &DividerWidget{} }

// Vertical turns the rule to fill the height it is given instead.
func (d *DividerWidget) Vertical() *DividerWidget { d.vertical = true; return d }

// Layout implements Widget.
func (d *DividerWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	d.theme = env.Theme()
	if d.vertical {
		return c.Constrain(ggui.Sz(1, bounded(c.MaxH, 0)))
	}
	return c.Constrain(ggui.Sz(bounded(c.MaxW, 0), 1))
}

// Paint implements Widget.
func (d *DividerWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.FillRect(r, d.theme.Border) }
