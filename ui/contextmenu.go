package ui

import (
	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// ContextMenuWidget wraps content with a secondary-click action menu.
// Keep the widget alive or assign a Key when rebuilding it.
type ContextMenuWidget struct {
	ggui.Interactive
	content ggui.Widget
	menu    *MenuWidget
	rect    ggui.Rect
	at      *ggui.Point
	theme   *uitheme.Theme

	// entries makes a ContextMenuFunc's entries into lazy as the menu opens.
	nativeEntries func() []ggui.MenuItem
	entries       func() []ggui.Widget
	lazy          *lazyEntries

	// inner are where the context menus inside content were painted last,
	// whose secondary clicks are theirs and not this one's.
	inner []ggui.Rect
}

// ContextMenu opens entries at the right-click position. Tab focuses the
// wrapper; Shift+F10, Enter or Space opens it from the keyboard. Entries are
// those of Menu, with the same arrow navigation and submenus.
func ContextMenu(content ggui.Widget, entries ...ggui.Widget) *ContextMenuWidget {
	m := panelMenu()
	m.body.content = m.column(entries)
	return newContextMenu(content, m)
}

// panelMenu is a menu for a context menu: its items and the keys that move
// through them, shown in the context menu's popup, with no button of its
// own to open it nor popup under one. A list keeps a context menu for each
// of its rows, and those were tens of megabytes. Its panel is as wide as
// its entries rather than a dropdown's fixed width.
func panelMenu() *MenuWidget {
	m := newMenu()
	m.fit = true
	return m
}

// ContextMenuFunc is ContextMenu with entries made by entries as the menu
// opens rather than as it is built: for content rebuilt far more often than
// its menu opens, such as a row in a list that follows live state, whose
// entries would otherwise be made with every rebuild.
func ContextMenuFunc(content ggui.Widget, entries func() []ggui.Widget) *ContextMenuWidget {
	m := panelMenu()
	lazy := &lazyEntries{}
	m.body.content = lazy
	c := newContextMenu(content, m)
	c.entries, c.lazy = entries, lazy
	return c
}

// NativeContextMenu uses the platform's context menu with the same model as
// App.SetMenu and App.AddTray. Where unavailable (including probes), it uses
// the regular widget menu. Keyboard and accessibility invocation work as usual.
func NativeContextMenu(content ggui.Widget, entries ...ggui.MenuItem) *ContextMenuWidget {
	return NativeContextMenuFunc(content, func() []ggui.MenuItem { return entries })
}

// NativeContextMenuFunc builds its entries each time the menu opens.
func NativeContextMenuFunc(content ggui.Widget, entries func() []ggui.MenuItem) *ContextMenuWidget {
	c := ContextMenuFunc(content, func() []ggui.Widget { return menuEntries(entries()) })
	c.nativeEntries = entries
	return c
}

func newContextMenu(content ggui.Widget, m *MenuWidget) *ContextMenuWidget {
	c := &ContextMenuWidget{content: content, menu: m}
	c.Role = ggui.RoleMenu
	c.AutoKey()
	c.menu.keys = c
	c.menu.popup = ggui.Popup(content, menuSurface(c.menu.panel)).Gap(0).Keys(c).Owner(c)
	return c
}

// build makes a ContextMenuFunc's entries, once: when it opens, or when it
// is laid out open, as one rebuilt while its menu is open is.
func (c *ContextMenuWidget) build() {
	if c.entries == nil || c.lazy.col != nil {
		return
	}
	c.lazy.col = c.menu.column(c.entries())
}

// lazyEntries is a ContextMenuFunc's panel content, nothing until built.
type lazyEntries struct{ col ggui.Widget }

func (e *lazyEntries) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	if e.col == nil {
		return cs.Constrain(ggui.Size{})
	}
	return e.col.Layout(cs, env)
}

func (e *lazyEntries) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if e.col != nil {
		dst.Paint(e.col, r)
	}
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

func (c *ContextMenuWidget) open(at ggui.Point, first bool) {
	if c.IsInert() {
		return
	}
	if c.nativeEntries != nil {
		entries := c.nativeEntries()
		if w := ggui.UseWindow(); w != nil && w.ShowContextMenu(at, entries...) {
			return
		}
		// Rebuild the fallback from this invocation's snapshot.
		c.menu.items = nil
		c.lazy.col = c.menu.column(menuEntries(entries))
	}
	c.at = &at
	c.build()
	c.menu.reopen(first)
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
		c.open(c.rect.Origin.Add(ggui.Pt(0, c.rect.Size.H)), true)
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
	if c.Popup().IsOpen() {
		c.build()
	}
	c.theme = uitheme.Of(env)
	t := c.theme
	c.menu.chrome(t)
	return c.Popup().Layout(cs, env)
}

// Paint implements ggui.Widget.
func (c *ContextMenuWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	c.rect = r
	dst.Describe(r, c)
	if !c.IsInert() {
		dst.HitKey(r, c)
	}
	// A context menu inside another tells it where it is, in a frame whose
	// regions take input; one painted in part keeps the last whole frame's.
	if dst.TakesInput() {
		if outer, ok := dst.Enclosing().(*ContextMenuWidget); ok && !c.IsInert() {
			outer.inner = append(outer.inner, dst.Visible(r))
		}
		c.inner = c.inner[:0]
	}
	dst.Enclose(c, func() { dst.Paint(c.Popup(), r) })
	// Secondary clicks take precedence over child controls, though not over
	// a context menu inside this one, as a row's in a list that has its own;
	// other events pass on.
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
	for _, r := range c.inner {
		if r.Contains(ev.Pos) {
			return false
		}
	}
	switch ev.Kind {
	case ggui.PointerDown:
		c.open(ev.Pos, false)
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
		(c.Popup().IsOpen() && (navConsumes(ev, navVertical) || c.menu.consumes(ev)))
}

// HandleKey implements ggui.KeyHandler.
func (c *ContextMenuWidget) HandleKey(ev ggui.KeyEvent) {
	c.Keyboard(ev, nil)
	if c.IsInert() || ev.Kind != ggui.KeyPress {
		return
	}
	if !c.Popup().IsOpen() {
		if ggui.Activates(ev) || (ev.Key == ggui.KeyF10 && ev.Mods.Shift) {
			c.open(c.rect.Origin.Add(ggui.Pt(0, c.rect.Size.H)), true)
		}
		return
	}
	// The menu takes the keys that move through it and its submenus; an
	// Escape it leaves, with no submenu open, closes it.
	if !c.menu.key(ev) && ev.Key == ggui.KeyEscape {
		c.Popup().Hide()
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
