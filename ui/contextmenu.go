package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// ContextMenuWidget wraps content with a secondary-click action menu.
// Keep the widget alive or assign a Key when rebuilding it.
type ContextMenuWidget struct {
	ggui.Interactive
	content ggui.Widget
	menu    *MenuWidget
	rect    ggui.Rect
	at      *ggui.Point
	theme   ggui.Theme
}

// ContextMenu opens entries at the right-click position. Tab focuses the
// wrapper; Shift+F10, Enter or Space opens it from the keyboard. Entries are
// MenuItem and MenuDivider, with the same arrow navigation as Menu.
func ContextMenu(content ggui.Widget, entries ...ggui.Widget) *ContextMenuWidget {
	c := &ContextMenuWidget{content: content, menu: Menu("", entries...)}
	c.Role, c.Name = ggui.RoleMenu, "Context menu"
	c.AutoKey()
	c.menu.popup = ggui.Popup(contextAnchor{c}, c.menu.panel).Gap(0).Keys(c).Owner(c)
	return c
}

// Named sets the accessible name of the context-menu target.
func (c *ContextMenuWidget) Named(name string) *ContextMenuWidget { c.Name = name; return c }

// Disabled disables menu invocation while leaving the wrapped content usable.
func (c *ContextMenuWidget) Disabled(v bool) *ContextMenuWidget {
	c.Inert = v
	if v {
		c.menu.popup.Hide()
	}
	return c
}

// Popup returns the menu's popup.
func (c *ContextMenuWidget) Popup() *ggui.PopupWidget { return c.menu.popup }

func (c *ContextMenuWidget) open(at ggui.Point) {
	if c.Inert {
		return
	}
	c.at = &at
	for _, it := range c.menu.items {
		it.Sync()
	}
	c.menu.current = -1
	c.menu.step(1)
	c.menu.popup.ShowAt(at)
}

// Describe exposes menu invocation to accessibility clients.
func (c *ContextMenuWidget) Describe() ggui.Node {
	return ggui.Node{Role: c.Role, Name: c.Name, Disabled: c.Inert,
		Expanded: ggui.Expandable(c.Popup().IsOpen()),
		Actions:  ggui.ActionFocus | ggui.ActionPress | ggui.ActionExpand | ggui.ActionCollapse}
}

// Act implements ggui.Actor.
func (c *ContextMenuWidget) Act(a ggui.Action) bool {
	if c.Inert {
		return false
	}
	switch a.Kind {
	case ggui.ActionPress, ggui.ActionExpand:
		c.open(c.rect.Origin.Add(ggui.Pt(0, c.rect.Size.H)))
	case ggui.ActionCollapse:
		c.Popup().Hide()
	default:
		return false
	}
	return true
}

// Layout implements ggui.Widget.
func (c *ContextMenuWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	c.Sync()
	if c.Inert {
		c.Popup().Hide()
	}
	c.theme = env.Theme()
	t := c.theme
	c.menu.panel.Shadow(panelShadow(t)).Fill(t.Surface).Border(1, t.Border).Radius(t.Radius).Padding(t.PanelPad)
	return c.Popup().Layout(cs, env)
}

// Paint implements ggui.Widget.
func (c *ContextMenuWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	c.rect = r
	for i, it := range c.menu.items {
		it.active = i == c.menu.current
	}
	dst.Describe(r, c)
	if !c.Inert {
		dst.HitKey(r, c)
	}
	dst.Paint(c.Popup(), r)
	// Secondary clicks take precedence over child controls; other events pass on.
	if !c.Inert {
		dst.HitPointer(r, c)
	}
	c.FocusRing(dst, r, c.theme.Radius, focusColor(c.theme))
}

// HandlePointer implements ggui.PointerHandler.
func (c *ContextMenuWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if c.Inert || ev.Button != ebiten.MouseButtonRight {
		return false
	}
	switch ev.Kind {
	case ggui.PointerDown:
		c.open(ev.Pos)
		return true
	case ggui.PointerUp, ggui.PointerTap:
		return true
	}
	return false
}

// ConsumesKey implements ggui.KeyConsumer.
func (c *ContextMenuWidget) ConsumesKey(ev ggui.KeyEvent) bool {
	if c.Inert || ev.Kind != ggui.KeyPress {
		return false
	}
	return ggui.Activates(ev) || (ev.Key == ebiten.KeyF10 && ev.Mods.Shift) ||
		(c.Popup().IsOpen() && (ev.Key == ebiten.KeyArrowDown || ev.Key == ebiten.KeyArrowUp || ev.Key == ebiten.KeyHome || ev.Key == ebiten.KeyEnd))
}

// HandleKey implements ggui.KeyHandler.
func (c *ContextMenuWidget) HandleKey(ev ggui.KeyEvent) {
	c.Keyboard(ev, nil)
	if c.Inert || ev.Kind != ggui.KeyPress {
		return
	}
	if !c.Popup().IsOpen() {
		if ggui.Activates(ev) || (ev.Key == ebiten.KeyF10 && ev.Mods.Shift) {
			c.open(c.rect.Origin.Add(ggui.Pt(0, c.rect.Size.H)))
		}
		return
	}
	switch ev.Key {
	case ebiten.KeyEscape:
		c.Popup().Hide()
	case ebiten.KeyArrowDown:
		c.menu.step(1)
	case ebiten.KeyArrowUp:
		c.menu.step(-1)
	case ebiten.KeyHome:
		c.menu.current = -1
		c.menu.step(1)
	case ebiten.KeyEnd:
		c.menu.current = -1
		c.menu.step(-1)
	default:
		if ggui.Activates(ev) && c.menu.current >= 0 {
			c.menu.items[c.menu.current].run()
		}
	}
}

// Adopt retains an open menu and its position across keyed rebuilds.
func (c *ContextMenuWidget) Adopt(prev any) {
	c.Interactive.Adopt(prev)
	if p, ok := prev.(*ContextMenuWidget); ok && !c.Inert {
		c.menu.current = p.menu.current
		if c.menu.current >= len(c.menu.items) {
			c.menu.current = -1
		}
		if p.Popup().IsOpen() && p.at != nil {
			c.at = p.at
			c.Popup().ShowAt(*c.at)
		}
	}
}

type contextAnchor struct{ c *ContextMenuWidget }

func (a contextAnchor) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	return a.c.content.Layout(cs, env)
}
func (a contextAnchor) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(a.c.content, r) }
