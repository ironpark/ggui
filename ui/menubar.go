package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// MenubarWidget coordinates Menu triggers through one popup and one tab stop.
// Menus belong exclusively to the bar; do not also paint them independently.
type MenubarWidget struct {
	ggui.Interactive
	menus        []*MenuWidget
	active       int
	popup        *ggui.PopupWidget
	sizes        []ggui.Size
	rects        []ggui.Rect
	theme        ggui.Theme
	motion       pointerMotion
	compact      bool
	naturalWidth float64
}

// Menubar creates an application menu strip. Left/Right wrap top-level menus;
// Up/Down and Enter/Space open actions. Escape closes without invoking an action.
func Menubar(menus ...*MenuWidget) *MenubarWidget {
	b := &MenubarWidget{menus: menus, active: 0, sizes: make([]ggui.Size, len(menus)), rects: make([]ggui.Rect, len(menus))}
	b.Role, b.Name = ggui.RoleToolbar, "Menubar"
	b.AutoKey()
	b.popup = ggui.Popup(menubarAnchor{b}, menubarPanel{b}).Keys(b).Owner(b).Gap(4)
	for i, m := range menus {
		m.button.Ghost().Pad(4, 8)
		m.button.onTap = func() { b.toggle(i) }
		m.button.Expands(func() bool { return b.popup.IsOpen() && b.active == i }).Opens(menubarAction{b, i})
	}
	return b
}

// Named sets the accessible name of the menu strip.
func (b *MenubarWidget) Named(s string) *MenubarWidget { b.Name = s; return b }

// Compact paints the menu strip at its content width, even in a stretched layout.
func (b *MenubarWidget) Compact() *MenubarWidget { b.compact = true; return b }

// Popup returns the shared popup.
func (b *MenubarWidget) Popup() *ggui.PopupWidget { return b.popup }
func (b *MenubarWidget) open(i int) {
	if b.Inert || i < 0 || i >= len(b.menus) || b.menus[i].button.Inert {
		return
	}
	b.active = i
	b.menus[i].reopen()
	r := b.rects[i]
	b.popup.ShowAt(r.Origin.Add(ggui.Pt(0, r.Size.H)))
}
func (b *MenubarWidget) toggle(i int) {
	if b.popup.IsOpen() && i == b.active {
		b.popup.Hide()
	} else {
		b.open(i)
	}
}

// Layout implements ggui.Widget.
func (b *MenubarWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	b.theme = env.Theme()
	if len(b.menus) > 0 && b.menus[b.active].button.Inert {
		b.popup.Hide()
		b.active = stepIndex(b.active, 1, len(b.menus), func(i int) bool { return !b.menus[i].button.Inert })
	}
	return b.popup.Layout(c, env)
}

// Adopt retains the active menu and highlighted action across keyed rebuilds.
func (b *MenubarWidget) Adopt(prev any) {
	b.Interactive.Adopt(prev)
	if p, ok := prev.(*MenubarWidget); ok && p.active < len(b.menus) && p.active >= 0 && !b.menus[p.active].button.Inert {
		b.active = p.active
		b.motion = p.motion
		if p.popup.IsOpen() {
			b.open(b.active)
			current := p.menus[p.active].current
			if current >= 0 && current < len(b.menus[b.active].items) {
				b.menus[b.active].current = current
			}
		}
	}
}

// Paint implements ggui.Widget.
func (b *MenubarWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if b.compact {
		r.Size.W = min(r.Size.W, b.naturalWidth)
	}
	dst.Describe(r, b)
	dst.FillRoundRect(r, b.theme.Radius, b.theme.Card)
	dst.StrokeRoundRect(r, b.theme.Radius, 1, b.theme.Border)
	dst.Paint(b.popup, r)
}

// HandlePointer lets the bar serve as an input-state identity without consuming events.
func (b *MenubarWidget) HandlePointer(ggui.PointerEvent) bool { return false }

// ConsumesKey implements ggui.KeyConsumer.
func (b *MenubarWidget) ConsumesKey(e ggui.KeyEvent) bool {
	if e.Kind != ggui.KeyPress {
		return false
	}
	switch e.Key {
	case ebiten.KeyArrowLeft, ebiten.KeyArrowRight, ebiten.KeyArrowUp, ebiten.KeyArrowDown, ebiten.KeyHome, ebiten.KeyEnd:
		return true
	}
	return ggui.Activates(e)
}

// HandleKey implements ggui.KeyHandler.
func (b *MenubarWidget) HandleKey(e ggui.KeyEvent) {
	b.Keyboard(e, nil)
	if b.Inert || e.Kind != ggui.KeyPress || len(b.menus) == 0 {
		return
	}
	switch e.Key {
	case ebiten.KeyEscape:
		b.popup.Hide()
	case ebiten.KeyArrowLeft, ebiten.KeyArrowRight:
		b.active = stepIndex(b.active, pick(e.Key == ebiten.KeyArrowLeft, -1, 1), len(b.menus), func(i int) bool { return !b.menus[i].button.Inert })
		if b.popup.IsOpen() {
			b.open(b.active)
		}
	case ebiten.KeyArrowDown, ebiten.KeyArrowUp:
		if !b.popup.IsOpen() {
			b.open(b.active)
			if e.Key == ebiten.KeyArrowUp {
				b.menus[b.active].jump(-1)
			}
		} else {
			b.menus[b.active].step(pick(e.Key == ebiten.KeyArrowUp, -1, 1))
		}
	case ebiten.KeyHome, ebiten.KeyEnd:
		if b.popup.IsOpen() {
			b.menus[b.active].jump(pick(e.Key == ebiten.KeyHome, 1, -1))
		} else {
			b.active = stepIndex(-1, pick(e.Key == ebiten.KeyHome, 1, -1), len(b.menus), func(i int) bool { return !b.menus[i].button.Inert })
			if b.active < 0 {
				b.active = 0
			}
		}
	default:
		if ggui.Activates(e) {
			if !b.popup.IsOpen() {
				b.open(b.active)
			} else {
				b.menus[b.active].activate()
			}
		}
	}
}

type menubarAction struct {
	b *MenubarWidget
	i int
}

func (a menubarAction) Act(e ggui.Action) bool {
	switch e.Kind {
	case ggui.ActionExpand:
		a.b.open(a.i)
	case ggui.ActionCollapse:
		a.b.popup.Hide()
	default:
		return false
	}
	return true
}

type menubarAnchor struct{ b *MenubarWidget }

func (a menubarAnchor) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	w, h := 6.0, 0.0
	for i, m := range a.b.menus {
		a.b.sizes[i] = m.button.Layout(c.Loosen(), env)
		w += a.b.sizes[i].W
		h = max(h, a.b.sizes[i].H)
	}
	a.b.naturalWidth = w
	return c.Constrain(ggui.Sz(w, h+6))
}
func (a menubarAnchor) Paint(dst *ggui.Canvas, r ggui.Rect) {
	b := a.b
	x := r.Origin.X + 3
	for i, m := range b.menus {
		rr := ggui.Rct(ggui.Pt(x, r.Origin.Y+3), ggui.Sz(b.sizes[i].W, max(0, r.Size.H-6)))
		b.rects[i] = rr
		m.button.box.Fill(nil).Border(0, nil)
		if (b.popup.IsOpen() && b.active == i) || m.button.Hovered {
			m.button.box.Fill(b.theme.Muted)
		}
		dst.Describe(rr, m.button)
		dst.Paint(m.button.box, rr)
		if !m.button.Inert {
			dst.HitPointer(rr, menubarTarget{b, i})
			dst.HitCursor(rr, ebiten.CursorShapePointer)
		}
		if b.Focused && b.FocusVisible && i == b.active {
			dst.StrokeRoundRect(rr, b.theme.Radius, 2, b.theme.Ring)
		}
		x += rr.Size.W
	}
	if len(b.menus) > 0 {
		dst.HitKey(r, b)
	}
}

type menubarPanel struct{ b *MenubarWidget }

func (p menubarPanel) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	b := p.b
	if len(b.menus) == 0 {
		return ggui.Size{}
	}
	t := env.Theme()
	return b.menus[b.active].chrome(t).Layout(c, env)
}
func (p menubarPanel) Paint(dst *ggui.Canvas, r ggui.Rect) {
	b := p.b
	if len(b.menus) == 0 {
		return
	}
	m := b.menus[b.active]
	for i, it := range m.items {
		it.active = i == m.current
	}
	dst.Paint(m.panel, r)
	for i, rr := range b.rects {
		if !b.menus[i].button.Inert {
			dst.HitPointer(rr, menubarTarget{b, i})
		}
	}
}

type menubarTarget struct {
	b *MenubarWidget
	i int
}

func (t menubarTarget) HandlePointer(e ggui.PointerEvent) bool {
	if t.b.menus[t.i].button.Inert {
		return false
	}
	switch e.Kind {
	case ggui.PointerEnter:
		t.b.menus[t.i].button.Hovered = true
		return true
	case ggui.PointerExit:
		t.b.menus[t.i].button.Hovered = false
		return true
	case ggui.PointerMove:
		if t.b.motion.drifted(e.Pos) && t.b.popup.IsOpen() && t.b.active != t.i {
			t.b.open(t.i)
		}
		return true
	case ggui.PointerDown:
		if e.Button == ebiten.MouseButtonLeft {
			t.b.toggle(t.i)
		}
		return true
	case ggui.PointerUp, ggui.PointerTap:
		return true
	}
	return false
}

func (t menubarTarget) Semantics() (ggui.Role, string) { return t.b.menus[t.i].button.Semantics() }
