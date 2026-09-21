package ui

import (
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
	c.Role = ggui.RoleMenu
	c.AutoKey()
	c.menu.popup = ggui.Popup(content, c.menu.panel).Gap(0).Keys(c).Owner(c)
	return c
}

// Name sets the accessible name of the context-menu target.
func (c *ContextMenuWidget) Name(name string) *ContextMenuWidget { c.SetName(name); return c }

// Disabled disables menu invocation while leaving the wrapped content usable.
func (c *ContextMenuWidget) Disabled(v bool) *ContextMenuWidget {
	c.SetInert(v)
	if v {
		c.menu.popup.Hide()
	}
	return c
}

// Popup returns the menu's popup.
func (c *ContextMenuWidget) Popup() *ggui.PopupWidget { return c.menu.popup }

func (c *ContextMenuWidget) open(at ggui.Point) {
	if c.IsInert() {
		return
	}
	c.at = &at
	c.menu.reopen()
	c.menu.popup.ShowAt(at)
}

// Describe exposes menu invocation to accessibility clients.
func (c *ContextMenuWidget) Describe() ggui.Node {
	return ggui.Node{Role: c.Role, Name: c.name(), Disabled: c.IsInert(),
		Expanded: ggui.Expandable(c.Popup().IsOpen()),
		Actions:  ggui.ActionFocus | ggui.ActionPress | ggui.ActionExpand | ggui.ActionCollapse}
}

// Act implements ggui.Actor.
func (c *ContextMenuWidget) Act(a ggui.Action) bool {
	if c.IsInert() {
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
	if c.IsInert() {
		c.Popup().Hide()
	}
	c.theme = env.Theme()
	t := c.theme
	c.menu.chrome(t)
	return c.Popup().Layout(cs, env)
}

// Paint implements ggui.Widget.
func (c *ContextMenuWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	c.rect = r
	for i, it := range c.menu.items {
		it.active = i == c.menu.current
	}
	dst.Describe(r, c)
	if !c.IsInert() {
		dst.HitKey(r, c)
	}
	dst.Paint(c.Popup(), r)
	// Secondary clicks take precedence over child controls; other events pass on.
	if !c.IsInert() {
		dst.HitPointer(r, c)
	}
	c.FocusRing(dst, r, c.theme.Radius, c.theme.Ring)
}

// HandlePointer implements ggui.PointerHandler.
func (c *ContextMenuWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if c.IsInert() || ev.Button != ggui.MouseButtonRight {
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
	if c.IsInert() || ev.Kind != ggui.KeyPress {
		return false
	}
	return ggui.Activates(ev) || (ev.Key == ggui.KeyF10 && ev.Mods.Shift) ||
		(c.Popup().IsOpen() && (ev.Key == ggui.KeyArrowDown || ev.Key == ggui.KeyArrowUp || ev.Key == ggui.KeyHome || ev.Key == ggui.KeyEnd))
}

// HandleKey implements ggui.KeyHandler.
func (c *ContextMenuWidget) HandleKey(ev ggui.KeyEvent) {
	c.Keyboard(ev, nil)
	if c.IsInert() || ev.Kind != ggui.KeyPress {
		return
	}
	if !c.Popup().IsOpen() {
		if ggui.Activates(ev) || (ev.Key == ggui.KeyF10 && ev.Mods.Shift) {
			c.open(c.rect.Origin.Add(ggui.Pt(0, c.rect.Size.H)))
		}
		return
	}
	switch ev.Key {
	case ggui.KeyEscape:
		c.Popup().Hide()
	case ggui.KeyArrowDown:
		c.menu.step(1)
	case ggui.KeyArrowUp:
		c.menu.step(-1)
	case ggui.KeyHome:
		c.menu.jump(1)
	case ggui.KeyEnd:
		c.menu.jump(-1)
	default:
		if ggui.Activates(ev) {
			c.menu.activate()
		}
	}
}

// Adopt retains an open menu and its position across keyed rebuilds.
func (c *ContextMenuWidget) Adopt(prev any) {
	c.Interactive.Adopt(prev)
	if p, ok := prev.(*ContextMenuWidget); ok && !c.IsInert() {
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

// BindDisabled follows r for Disabled without rebuilding the control.
func (c *ContextMenuWidget) BindDisabled(r ggui.Readable[bool]) *ContextMenuWidget {
	c.BindInert(r)
	return c
}

func (c *ContextMenuWidget) name() string {
	return pick(c.SemanticName() != "", c.SemanticName(), "Context menu")
}

// Semantics implements ggui.Semantic, including the built-in fallback name.
func (c *ContextMenuWidget) Semantics() (ggui.Role, string) { return c.Role, c.name() }
