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
	m.init(entries)
	return m
}

// MenuOf is Menu with any content as the button's face.
func MenuOf(content ggui.Widget, entries ...ggui.Widget) *MenuWidget {
	m := &MenuWidget{current: -1}
	m.button = ButtonOf(content, m.toggle).Secondary()
	m.init(entries)
	return m
}

func (m *MenuWidget) init(entries []ggui.Widget) {
	for _, e := range entries {
		if it, ok := e.(*MenuItemWidget); ok {
			it.menu = m
			m.items = append(m.items, it)
		}
	}
	m.panel = ggui.Box(ggui.Column(entries...).Align(ggui.AlignStretch))
	m.popup = ggui.Popup(m.button, m.panel).Keys(m)
}

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

// Layout implements Widget.
func (m *MenuWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	m.theme = t
	m.panel.Fill(t.Surface).Border(1, t.Border).Radius(t.Radius).Padding(t.PanelPad)
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
	text   *ggui.TextWidget
	onTap  func()
	menu   *MenuWidget
	active bool

	pad      ggui.EdgeInsets
	textSize ggui.Size
	theme    ggui.Theme
	popup    *ggui.PopupWidget
}

// MenuItem creates an entry that runs onTap and closes the menu.
func MenuItem(label string, onTap func()) *MenuItemWidget {
	return &MenuItemWidget{text: ggui.Text(label).NoWrap(), onTap: onTap}
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
	it.textSize = it.text.Layout(it.pad.Shrink(c).Loosen(), env)
	return c.Constrain(it.pad.Inflate(it.textSize))
}

// Paint implements Widget.
func (it *MenuItemWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := it.theme
	if !it.Inert {
		dst.HitPointer(r, it)
		dst.HitCursor(r, ebiten.CursorShapePointer)
		if it.Hovered || it.active {
			dst.FillRoundRect(r, t.Radius*0.75, t.Selection)
		}
	}
	dst.Paint(it.text, ggui.Rct(ggui.Pt(r.Origin.X+it.pad.Left, r.Origin.Y+(r.Size.H-it.textSize.H)/2), it.textSize))
}

// HandlePointer implements PointerHandler.
func (it *MenuItemWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if (ev.Kind == ggui.PointerEnter || ev.Kind == ggui.PointerMove) && it.menu != nil {
		it.menu.current = -1
	}
	return it.Pointer(ev, it.run)
}
