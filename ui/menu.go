package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// MenuWidget is a button that opens a list of actions. Build one with Menu
// or MenuOf, with MenuItem and MenuDivider as the entries.
type MenuWidget struct {
	button  *ButtonWidget
	popup   *ggui.PopupWidget
	panel   *ggui.BoxWidget
	items   []*MenuItemWidget
	current int // the item the keyboard is on while open, or -1
	theme   ggui.Theme
}

// Menu creates a secondary button labelled label that opens entries below
// it. Space or Enter opens the menu, the arrow keys move through the
// items, Enter runs one and Escape closes it.
func Menu(label string, entries ...ggui.Widget) *MenuWidget {
	m := &MenuWidget{current: -1}
	m.button = Button(label, m.toggle).Secondary()
	m.button.Role = ggui.RoleMenu
	m.init(entries)
	return m
}

// MenuOf is Menu with any content as the button's face.
func MenuOf(content ggui.Widget, entries ...ggui.Widget) *MenuWidget {
	m := &MenuWidget{current: -1}
	m.button = ButtonOf(content, m.toggle).Secondary()
	m.button.Role = ggui.RoleMenu
	m.init(entries)
	return m
}

// Label names a MenuOf for Probe.Find and the inspector.
func (m *MenuWidget) Label(s string) *MenuWidget { m.button.Name = s; return m }

// Disabled disables this menu's trigger and closes it.
func (m *MenuWidget) Disabled(v bool) *MenuWidget {
	m.button.Disabled(v)
	if v {
		m.popup.Hide()
	}
	return m
}

// Semantics implements ggui.Semantic through the button.
func (m *MenuWidget) Semantics() (ggui.Role, string) { return m.button.Semantics() }

// ConsumesKey implements ggui.KeyConsumer: Up and Down open and move.
func (m *MenuWidget) ConsumesKey(ev ggui.KeyEvent) bool {
	return ev.Kind == ggui.KeyPress && (ggui.Activates(ev) || ev.Key == ebiten.KeyArrowUp || ev.Key == ebiten.KeyArrowDown)
}

func (m *MenuWidget) init(entries []ggui.Widget) {
	for _, e := range entries {
		if it, ok := e.(*MenuItemWidget); ok {
			it.menu = m
			index := len(m.items)
			it.onHover = func() { m.current = index }
			m.items = append(m.items, it)
		}
	}
	m.panel = ggui.Box(ggui.Column(entries...).Align(ggui.AlignStretch)).Width(224)
	m.popup = ggui.Popup(m.button, m.panel).Keys(m).Owner(m.button)
	m.button.Expands(m.popup.IsOpen).Opens(m)
}

// Width sets the popup panel width, constrained to the available space.
func (m *MenuWidget) Width(w float64) *MenuWidget { m.panel.Width(max(w, 0)); return m }

// Popup returns the popup the entries open in.
func (m *MenuWidget) Popup() *ggui.PopupWidget { return m.popup }

func (m *MenuWidget) toggle() {
	if m.popup.IsOpen() {
		m.popup.Hide()
		return
	}
	m.current = -1
	m.popup.Show()
}

// Act implements ggui.Actor. The node is the button's, so the button hands
// the action on to the menu that owns it.
func (m *MenuWidget) Act(a ggui.Action) bool {
	if m.button.Inert {
		return false
	}
	switch a.Kind {
	case ggui.ActionExpand:
		m.current = -1
		m.popup.Show()
	case ggui.ActionCollapse:
		m.popup.Hide()
	default:
		return false
	}
	return true
}

// Layout implements Widget.
func (m *MenuWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	m.theme = t
	m.panel.Shadow(panelShadow(t)).Fill(t.Surface).Border(1, t.Border).Radius(t.Radius).Padding(t.PanelPad)
	return m.popup.Layout(c, env)
}

// Paint implements Widget.
func (m *MenuWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	for i, it := range m.items {
		it.active = i == m.current
	}
	dst.Paint(m.popup, r)
	if !m.button.Inert {
		// On top of the button's own region, so the arrow keys reach the
		// menu; everything else is passed on to the button.
		dst.HitKey(r, m)
	}
}

// HandleKey implements KeyHandler.
func (m *MenuWidget) HandleKey(ev ggui.KeyEvent) {
	if ev.Kind == ggui.KeyBlur {
		m.popup.Hide()
	}
	if ev.Kind != ggui.KeyPress || len(m.items) == 0 {
		m.button.HandleKey(ev)
		return
	}
	open := m.popup.IsOpen()
	switch ev.Key {
	case ebiten.KeyEscape:
		m.popup.Hide()
	case ebiten.KeyArrowUp, ebiten.KeyArrowDown:
		if !open {
			m.toggle()
		}
		m.step(pick(ev.Key == ebiten.KeyArrowUp, -1, 1))
	case ebiten.KeyEnter, ebiten.KeyNumpadEnter, ebiten.KeySpace:
		if open && m.current >= 0 {
			m.items[m.current].run()
			return
		}
		m.button.HandleKey(ev)
	default:
		m.button.HandleKey(ev)
	}
}

// step moves the keyboard highlight by dir, skipping disabled items.
func (m *MenuWidget) step(dir int) {
	m.current = stepIndex(m.current, dir, len(m.items), func(i int) bool { return !m.items[i].Inert })
}

// Adopt implements ggui.Adopter: an open menu carries across a rebuild.
func (m *MenuWidget) Adopt(prev any) {
	if p, ok := prev.(*MenuWidget); ok {
		m.current = p.current
		m.button.Adopt(p.button)
		if p.popup.IsOpen() {
			m.popup.Show()
		}
	}
}

// MenuItemWidget is one action in a Menu. Build one with MenuItem.
type MenuItemWidget struct {
	ggui.Interactive
	text         *ggui.TextWidget
	onTap        func()
	menu         *MenuWidget
	active       bool
	shortcut     *ggui.TextWidget
	shortcutSize ggui.Size
	onHover      func()
	lastPointer  ggui.Point
	hasPointer   bool

	pad      ggui.EdgeInsets
	textSize ggui.Size
	theme    ggui.Theme
	popup    *ggui.PopupWidget
}

// MenuItem creates an entry that runs onTap and closes the menu.
func MenuItem(label string, onTap func()) *MenuItemWidget {
	it := &MenuItemWidget{text: ggui.Text(label).NoWrap(), onTap: onTap}
	it.Role, it.Name = ggui.RoleMenuItem, label
	it.AutoKey()
	return it
}

// Shortcut displays a right-aligned key hint; it does not register a shortcut.
func (it *MenuItemWidget) Shortcut(s string) *MenuItemWidget {
	it.shortcut = ggui.Caption(s).NoWrap()
	return it
}

// Disabled greys the item out and ignores it while v is true.
func (it *MenuItemWidget) Disabled(v bool) *MenuItemWidget { it.Inert = v; return it }

// DisabledWhen follows r for Disabled without a rebuild.
func (it *MenuItemWidget) DisabledWhen(r ggui.Reader[bool]) *MenuItemWidget {
	it.InertWhen(r)
	return it
}

// MenuDivider is a line between groups of items.
func MenuDivider() ggui.Widget { return ggui.Padding(Divider(), 4, 0) }

func (it *MenuItemWidget) run() {
	if it.Inert {
		return
	}
	if it.onTap != nil {
		it.onTap()
	}
	if it.popup != nil {
		it.popup.Hide()
	}
}

// Layout implements Widget.
func (it *MenuItemWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	it.Sync()
	t := env.Theme()
	it.theme = t
	it.popup, _ = ggui.PopupOf(env)
	it.pad = t.ItemPad
	it.text.Color(pick(it.Inert, t.Muted, t.Fg))
	inner := it.pad.Shrink(c).Loosen()
	gap := 0.0
	if it.shortcut != nil {
		it.shortcut.Color(t.Muted)
		it.shortcutSize = it.shortcut.Layout(inner, env)
		gap = 24
	}
	inner.MaxW = max(0, inner.MaxW-it.shortcutSize.W-gap)
	it.textSize = it.text.Layout(inner, env)
	return c.Constrain(it.pad.Inflate(ggui.Sz(it.textSize.W+it.shortcutSize.W+gap, max(it.textSize.H, it.shortcutSize.H))))
}

// Paint implements Widget.
func (it *MenuItemWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := it.theme
	// A disabled item takes no input but is still part of the menu.
	dst.Describe(r, it)
	if !it.Inert {
		dst.HitPointer(r, it)
		dst.HitCursor(r, ebiten.CursorShapePointer)
		if it.active || (it.onHover == nil && it.Hovered) {
			dst.FillRoundRect(r, t.Radius*0.75, mutedSurface(t))
		}
	}
	if it.shortcut != nil {
		dst.Paint(it.shortcut, ggui.Rct(ggui.Pt(r.Origin.X+r.Size.W-it.pad.Right-it.shortcutSize.W, r.Origin.Y+(r.Size.H-it.shortcutSize.H)/2), it.shortcutSize))
	}
	dst.Clip(r).Paint(it.text, ggui.Rct(ggui.Pt(r.Origin.X+it.pad.Left, r.Origin.Y+(r.Size.H-it.textSize.H)/2), it.textSize))
}

// HandlePointer implements PointerHandler.
func (it *MenuItemWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind == ggui.PointerMove {
		if (!it.hasPointer || it.lastPointer != ev.Pos) && !it.Inert && it.onHover != nil {
			it.onHover()
		}
		it.lastPointer, it.hasPointer = ev.Pos, true
	}
	return it.Pointer(ev, it.run)
}
