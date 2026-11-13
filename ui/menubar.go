package ui

import (
	"image/color"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// MenubarWidget coordinates Menu triggers through one popup and one tab stop.
// Menus belong exclusively to the bar; do not also paint them independently.
type MenubarWidget struct {
	props property.Owner
	ggui.Interactive
	menus        []*MenuWidget
	active       int
	popup        *ggui.PopupWidget
	sizes        []ggui.Size
	rects        []ggui.Rect
	theme        *uitheme.Theme
	motion       pointerMotion
	compact      bool
	naturalWidth float64
	env          ggui.Env

	// Moving from one open menu to another is two popups in shadcn: the
	// one left plays its exit where it was while the one moved to plays
	// its entrance. The bar has one popup, so it replays the entrance in
	// swap and paints the menu left as a ghost on its way out.
	swap     *ggui.TransitionWidget
	ghost    *ggui.TransitionWidget
	arrive   ggui.Motion
	depart   ggui.Motion
	arriving bool
	shown    int       // the menu the popup was last opened with
	left     int       // the menu on its way out, or -1
	leftAt   ggui.Rect // where it was
}

// Menubar creates an application menu strip. Left/Right wrap top-level menus;
// Up/Down and Enter/Space open actions. Escape closes without invoking an action.
func Menubar(menus ...*MenuWidget) *MenubarWidget {
	b := &MenubarWidget{menus: menus, active: 0, left: -1, sizes: make([]ggui.Size, len(menus)), rects: make([]ggui.Rect, len(menus))}
	b.Role = ggui.RoleToolbar
	b.AutoKey()
	b.swap = ggui.PopIn(menuSurface(menubarPanel{b}))
	b.ghost = ggui.PopIn(menuSurface(menubarGhost{b}))
	b.popup = ggui.Popup(menubarAnchor{b}, b.swap).Keys(b).Owner(b)
	for i, m := range menus {
		m.fit, m.checkLeft, m.keys = true, true, b
		m.button.Ghost()
		m.button.onTap = func() { b.toggle(i, false) }
		m.button.Expands(func() bool { return b.popup.IsOpen() && b.active == i }).Opens(menubarAction{b, i})
	}
	return b
}

// Name sets the accessible name of the menu strip.
func (b *MenubarWidget) Name(s string) *MenubarWidget { b.SetName(s); return b }

// Compact paints the menu strip at its content width, even in a stretched layout.
func (b *MenubarWidget) Compact() *MenubarWidget {
	defer property.Watch(&b.props, &b.compact)()
	b.compact = true
	return b
}

// Popup returns the shared popup.
func (b *MenubarWidget) Popup() *ggui.PopupWidget { return b.popup }
func (b *MenubarWidget) open(i int, first bool) {
	if b.IsInert() || i < 0 || i >= len(b.menus) || b.menus[i].button.IsInert() {
		return
	}
	if b.popup.IsOpen() && b.shown != i && b.shown >= 0 && b.shown < len(b.menus) {
		// The menu left goes out where it is as the other comes in.
		now, d := ggui.FrameTime(), b.env.Motion(b.env.PopupDuration())
		b.left, b.leftAt = b.shown, b.popup.Rect()
		b.menus[b.left].closeSub()
		b.depart, b.arrive, b.arriving = ggui.Motion{}, ggui.Motion{}, true
		b.depart.MoveTo(1, now, 0)
		b.depart.MoveTo(0, now, d)
		b.arrive.MoveTo(0, now, 0)
		b.arrive.MoveTo(1, now, d)
	}
	b.active, b.shown = i, i
	b.menus[i].reopen(first)
	r := b.rects[i]
	// The panel hangs a little left of its trigger, clear of the bar.
	b.popup.ShowAt(r.Origin.Add(ggui.Pt(-b.inset(), r.Size.H)))
}
func (b *MenubarWidget) toggle(i int, first bool) {
	if b.popup.IsOpen() && i == b.active {
		b.popup.Hide()
	} else {
		b.open(i, first)
	}
}

// Layout implements ggui.Widget.
func (b *MenubarWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer b.props.Layout().EndLayout()
	for _, m := range b.menus {
		m.button.Sync()
	}
	b.theme, b.env = uitheme.Of(env), env
	t := b.theme
	b.popup.Gap(t.Space)
	for _, m := range b.menus {
		m.button.Pad(t.Space/4, t.Space*.75)
	}
	if len(b.menus) > 0 && b.menus[b.active].button.IsInert() {
		b.popup.Hide()
		b.active = stepIndex(b.active, 1, len(b.menus), b.enabled)
	}
	return b.popup.Layout(c, env)
}

// Adopt retains the active menu and highlighted action across keyed rebuilds.
func (b *MenubarWidget) Adopt(prev any) {
	b.Interactive.Adopt(prev)
	if p, ok := prev.(*MenubarWidget); ok && p.active < len(b.menus) && p.active >= 0 && !b.menus[p.active].button.IsInert() {
		b.active = p.active
		b.motion = p.motion
		if p.popup.IsOpen() {
			b.open(b.active, false)
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
	// Only a border: the bar takes the surface it is on.
	dst.StrokeRoundRect(r, b.theme.Radius, b.theme.BorderWidth, b.theme.Border)
	now, side := ggui.FrameTime(), b.popup.PlacedSide()
	if b.left >= 0 {
		out := b.depart.Value(now)
		if out <= 0 || !b.popup.IsOpen() || b.left >= len(b.menus) {
			b.left = -1
		} else {
			// Under the popup, which is painted after it, and inert.
			dst.Overlay(func(dst *ggui.Canvas) {
				b.ghost.PopFrom(side, 0).Progress(out, true)
				b.ghost.Layout(ggui.Tight(b.leftAt.Size), b.env)
				dst.Inert().Paint(b.ghost, b.leftAt)
			})
		}
	}
	in := 1.0
	if b.arriving {
		in = b.arrive.Value(now)
		b.arriving = in < 1
	}
	b.swap.PopFrom(side, 0).Progress(in, false)
	dst.Paint(b.popup, r)
}

// HandlePointer lets the bar serve as an input-state identity without consuming events.
func (b *MenubarWidget) HandlePointer(ggui.PointerEvent) bool { return false }

// ConsumesKey implements ggui.KeyConsumer.
func (b *MenubarWidget) ConsumesKey(e ggui.KeyEvent) bool {
	if e.Kind != ggui.KeyPress {
		return false
	}
	return navConsumes(e, navBoth) || ggui.Activates(e) || b.popup.IsOpen() && b.menus[b.active].consumes(e)
}

// inset is the space between the bar's edge and its triggers: the border
// and the padding inside it.
func (b *MenubarWidget) inset() float64 { return b.theme.BorderWidth + b.theme.Space*.375 }

// gap is the space between two triggers.
func (b *MenubarWidget) gap() float64 { return b.theme.Space / 4 }

// enabled reports whether menu i can take the keyboard.
func (b *MenubarWidget) enabled(i int) bool { return !b.menus[i].button.IsInert() }

// HandleKey implements ggui.KeyHandler.
func (b *MenubarWidget) HandleKey(e ggui.KeyEvent) {
	b.Keyboard(e, nil)
	if e.Kind == ggui.KeyText && !b.IsInert() && len(b.menus) > 0 && b.popup.IsOpen() {
		b.menus[b.active].typeahead(e.Text)
		return
	}
	if b.IsInert() || e.Kind != ggui.KeyPress || len(b.menus) == 0 {
		return
	}
	open := b.popup.IsOpen()
	along := navMoveOf(e.Key, navHorizontal) // Left, Right, Home or End
	ends := along == navFirst || along == navLast
	vertical := along == navNone && navMoveOf(e.Key, navVertical) != navNone // Up or Down
	switch {
	case open && b.menus[b.active].key(e):
		// The open menu moved or ran an item; Left, Right and Escape
		// belong to a submenu of it before they belong to the bar.
	case e.Key == ggui.KeyEscape:
		b.popup.Hide()
	case vertical && !open:
		// Down opens the menu on its first item. Up does nothing: Base UI
		// opens a menu of a horizontal bar on the cross-axis key that
		// points into it only.
		if e.Key == ggui.KeyArrowDown {
			b.open(b.active, true)
		}
	case open && (vertical || ends):
		// The menu had nothing to move to.
	case along != navNone:
		// Left and Right move along the bar, taking an open menu along;
		// Home and End, with the menu closed, go to its ends. Active is
		// never -1, which stays put only when every menu is disabled.
		i, _ := navIndex(e, navHorizontal, b.active, len(b.menus), b.enabled)
		b.active = max(i, 0)
		if open {
			b.open(b.active, true)
		}
	default:
		if ggui.Activates(e) && !open {
			b.open(b.active, true)
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
		a.b.open(a.i, true)
	case ggui.ActionCollapse:
		a.b.popup.Hide()
	default:
		return false
	}
	return true
}

type menubarAnchor struct{ b *MenubarWidget }

func (a menubarAnchor) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	b := a.b
	w, h := 2*b.inset(), 0.0
	for i, m := range b.menus {
		b.sizes[i] = m.button.Layout(c.Loosen(), env)
		w += b.sizes[i].W
		h = max(h, b.sizes[i].H)
	}
	w += b.gap() * float64(max(len(b.menus)-1, 0))
	b.naturalWidth = w
	return c.Constrain(ggui.Sz(w, h+2*b.inset()))
}
func (a menubarAnchor) Paint(dst *ggui.Canvas, r ggui.Rect) {
	b := a.b
	inset, radius := b.inset(), b.theme.Radius*.6
	x := r.Origin.X + inset
	for i, m := range b.menus {
		rr := ggui.Rct(ggui.Pt(x, r.Origin.Y+inset), ggui.Sz(b.sizes[i].W, max(0, r.Size.H-2*inset)))
		b.rects[i] = rr
		lit := (b.popup.IsOpen() && b.active == i) || m.button.Hovered
		m.button.box.Radius(radius).Fill(pick[color.Color](lit, b.theme.Muted, nil)).Border(0, nil)
		dst.Describe(rr, m.button)
		dst.Paint(m.button.box, rr)
		if !m.button.IsInert() {
			dst.HitPointer(rr, menubarTarget{b, i})
		}
		if b.Focused && b.FocusVisible && i == b.active {
			dst.StrokeRoundRect(rr, radius, b.theme.BorderWidth, b.theme.Ring)
			fieldHalo(dst, b.theme, rr, radius, b.theme.Ring)
		}
		x += rr.Size.W + b.gap()
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
	t := uitheme.Of(env)
	return b.menus[b.active].chrome(t).Layout(c, env)
}
func (p menubarPanel) Paint(dst *ggui.Canvas, r ggui.Rect) {
	b := p.b
	if len(b.menus) == 0 {
		return
	}
	dst.Paint(b.menus[b.active].panel, r)
	for i, rr := range b.rects {
		if !b.menus[i].button.IsInert() {
			dst.HitPointer(rr, menubarTarget{b, i})
		}
	}
}

// menubarGhost is the panel of the menu the bar has just left.
type menubarGhost struct{ b *MenubarWidget }

func (g menubarGhost) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	b := g.b
	if b.left < 0 || b.left >= len(b.menus) {
		return ggui.Size{}
	}
	return b.menus[b.left].chrome(uitheme.Of(env)).Layout(c, env)
}

func (g menubarGhost) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if b := g.b; b.left >= 0 && b.left < len(b.menus) {
		dst.Paint(b.menus[b.left].panel, r)
	}
}

type menubarTarget struct {
	b *MenubarWidget
	i int
}

func (t menubarTarget) HandlePointer(e ggui.PointerEvent) bool {
	if t.b.menus[t.i].button.IsInert() {
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
			t.b.open(t.i, false)
		}
		return true
	case ggui.PointerDown:
		if e.Button == ggui.MouseButtonLeft {
			t.b.toggle(t.i, false)
		}
		return true
	case ggui.PointerUp, ggui.PointerTap:
		return true
	}
	return false
}

func (t menubarTarget) Semantics() (ggui.Role, string) { return t.b.menus[t.i].button.Semantics() }

func (b *MenubarWidget) name() string {
	return pick(b.SemanticName() != "", b.SemanticName(), "Menubar")
}

// Semantics implements ggui.Semantic, including the built-in fallback name.
func (b *MenubarWidget) Semantics() (ggui.Role, string) { return b.Role, b.name() }
