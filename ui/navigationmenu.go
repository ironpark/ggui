package ui

import (
	"slices"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// NavigationMenuItemWidget is a navigation link or dropdown trigger. Build it
// with NavigationMenuLink, NavigationMenuLinkOf, or NavigationMenuItem.
// Links can also be used inside arbitrary dropdown layouts.
type NavigationMenuItemWidget struct {
	*ButtonWidget
	content      ggui.Widget
	panel        *ggui.BoxWidget
	owner        *NavigationMenuWidget
	popup        *ggui.PopupWidget
	onNavigate   func()
	width        float64
	activeProp   property.Value[bool]
	closeOnClick bool
	rect         ggui.Rect
}

// NavigationMenuLink creates a real navigation link with a native navigation
// callback (for routing or opening a URL). CloseOnClick optionally dismisses its popup.
func NavigationMenuLink(label string, onNavigate func()) *NavigationMenuItemWidget {
	item := &NavigationMenuItemWidget{onNavigate: onNavigate}
	item.ButtonWidget = Button(label, item.activate).Ghost()
	item.Role = ggui.RoleLink
	return item
}

// NavigationMenuLinkOf uses custom link content, such as a title and description.
// Supply its accessible label with Name.
func NavigationMenuLinkOf(content ggui.Widget, onNavigate func()) *NavigationMenuItemWidget {
	item := &NavigationMenuItemWidget{onNavigate: onNavigate}
	item.ButtonWidget = ButtonOf(content, item.activate).Ghost()
	item.Role = ggui.RoleLink
	return item
}

// NavigationMenuItem creates a dropdown with arbitrary content. Compose its
// contents from NavigationMenuLink, text, columns, or other native widgets.
func NavigationMenuItem(label string, content ggui.Widget) *NavigationMenuItemWidget {
	item := &NavigationMenuItemWidget{content: content, width: 320}
	item.ButtonWidget = Button(label, item.activate).Ghost()
	item.box = ggui.Box(&buttonContent{child: &navigationMenuFace{item: item}})
	item.panel = ggui.Box(ggui.Scroll(content))
	item.Expands(item.isOpen).Opens(item)
	return item
}

// Name sets the link or trigger's accessible name.
func (i *NavigationMenuItemWidget) Name(name string) *NavigationMenuItemWidget {
	i.ButtonWidget.Name(name)
	return i
}

// BindName follows the link or trigger's accessible name.
func (i *NavigationMenuItemWidget) BindName(r ggui.Readable[string]) *NavigationMenuItemWidget {
	i.ButtonWidget.BindName(r)
	return i
}

// Key gives this link or trigger a stable input identity.
func (i *NavigationMenuItemWidget) Key(k any) *NavigationMenuItemWidget {
	i.ButtonWidget.Key(k)
	return i
}

// Disabled disables this entry and closes its dropdown if open.
func (i *NavigationMenuItemWidget) Disabled(v bool) *NavigationMenuItemWidget {
	i.ButtonWidget.Disabled(v)
	if v && i.isOpen() {
		i.owner.popup.Hide()
	}
	return i
}

// BindDisabled follows a disabled-state reader.
func (i *NavigationMenuItemWidget) BindDisabled(v ggui.Readable[bool]) *NavigationMenuItemWidget {
	i.ButtonWidget.BindDisabled(v)
	return i
}

// Active marks a link as the current destination, visually and semantically.
func (i *NavigationMenuItemWidget) Active(v bool) *NavigationMenuItemWidget {
	property.Set(&i.ButtonWidget.props, &i.activeProp, v)
	i.selected = v
	return i
}

// BindActive follows whether this link is the current destination.
func (i *NavigationMenuItemWidget) BindActive(r ggui.Readable[bool]) *NavigationMenuItemWidget {
	property.Bind(&i.ButtonWidget.props, &i.activeProp, r, "BindActive")
	return i
}

// Width sets this entry's dropdown width (320px by default).
func (i *NavigationMenuItemWidget) Width(w float64) *NavigationMenuItemWidget {
	defer property.Watch(&i.ButtonWidget.props, &i.width)()
	i.width = max(w, 0)
	return i
}

// CloseOnClick closes the enclosing dropdown when this link is activated.
// The default is false, matching navigation links that manage their own route.
func (i *NavigationMenuItemWidget) CloseOnClick(v bool) *NavigationMenuItemWidget {
	i.closeOnClick = v
	return i
}

// Trigger returns the button for visual customization.
func (i *NavigationMenuItemWidget) Trigger() *ButtonWidget { return i.ButtonWidget }

func (i *NavigationMenuItemWidget) isOpen() bool {
	return i.owner != nil && i.owner.popup.IsOpen() && i.owner.current() == i
}
func (i *NavigationMenuItemWidget) disabled() bool {
	return i.IsInert() || i.owner != nil && i.owner.IsInert()
}
func (i *NavigationMenuItemWidget) activate() {
	if i.disabled() {
		return
	}
	if i.panel != nil {
		if i.owner != nil {
			if i.isOpen() {
				i.owner.popup.Hide()
			} else {
				i.owner.open(i)
			}
		}
		return
	}
	if i.closeOnClick {
		if i.popup != nil {
			i.popup.Hide()
		}
		if i.owner != nil {
			i.owner.popup.Hide()
		}
	}
	if i.onNavigate != nil {
		i.onNavigate()
	}
}

// Act implements accessible link activation and dropdown expand/collapse.
func (i *NavigationMenuItemWidget) Act(a ggui.Action) bool {
	if i.disabled() {
		return false
	}
	switch a.Kind {
	case ggui.ActionPress:
		i.activate()
	case ggui.ActionExpand:
		if i.owner == nil || i.panel == nil {
			return false
		}
		i.owner.open(i)
	case ggui.ActionCollapse:
		if i.owner == nil || i.panel == nil {
			return false
		}
		if i.isOpen() {
			i.owner.popup.Hide()
		}
	default:
		return false
	}
	return true
}

// Describe exposes native link/button semantics, current destination and
// expanded state, including a disabled parent navigation group.
func (i *NavigationMenuItemWidget) Describe() ggui.Node {
	node := i.ButtonWidget.Describe()
	node.Disabled = i.disabled()
	return node
}

// Layout implements ggui.Widget.
func (i *NavigationMenuItemWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	i.popup, _ = ggui.PopupOf(env)
	i.selected = i.activeProp.Get()
	return i.ButtonWidget.Layout(c, env)
}

// Paint implements ggui.Widget.
func (i *NavigationMenuItemWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	i.rect = r
	// Register the entry itself as the sole focus target. Painting the nested
	// button would create an additional Tab stop with its own key handler.
	t := i.theme
	fill := i.style.Fill
	if i.selected || i.Hovered || i.isOpen() {
		fill = i.style.Hover
	}
	label := i.style.Label
	if i.disabled() {
		fill, label = t.Disabled(fill), t.Disabled(label)
	}
	if i.label != nil {
		i.label.Color(label)
	}
	i.box.Fill(fill).Border(0, nil)
	if i.style.Border != nil {
		i.box.Border(t.BorderWidth, i.style.Border)
	}
	dst.Describe(r, i)
	if !i.disabled() {
		dst.HitPointer(r, i)
		dst.HitKey(r, i)
		dst.HitCursor(r, pick(i.panel == nil, ggui.CursorShapePointer, ggui.CursorShapeDefault))
	}
	dst.Paint(i.box, r)
	controlFocusRing(dst, &i.Interactive, t, r, t.Radius, t.Ring)
}

// ConsumesKey reserves horizontal navigation and dropdown opening keys.
func (i *NavigationMenuItemWidget) ConsumesKey(e ggui.KeyEvent) bool {
	return ggui.Activates(e) || i.owner != nil && (navConsumes(e, navHorizontal) || e.Kind == ggui.KeyPress && i.panel != nil && (e.Key == ggui.KeyArrowDown || e.Key == ggui.KeyArrowUp))
}

// HandleKey moves between top-level entries, or opens and activates an entry.
func (i *NavigationMenuItemWidget) HandleKey(e ggui.KeyEvent) {
	if i.disabled() {
		return
	}
	i.Keyboard(e, nil)
	if i.owner != nil && e.Kind == ggui.KeyPress {
		n := i.owner
		cur := slices.Index(n.items, i)
		if next, ok := navTarget(directionalKey(e, n.rtl).Key, navHorizontal, cur, len(n.items)); ok {
			next = max(0, min(next, len(n.items)-1))
			direction := 1
			if next < cur {
				direction = -1
			}
			for next >= 0 && next < len(n.items) && !n.enabled(next) {
				next += direction
			}
			if next < 0 || next >= len(n.items) {
				next = cur
			}
			if next >= 0 {
				n.pending = n.items[next]
			}
			n.popup.Hide()
			return
		}
		if i.panel != nil && (e.Key == ggui.KeyArrowDown || e.Key == ggui.KeyArrowUp) {
			n.open(i)
			return
		}
	}
	if ggui.Activates(e) {
		i.activate()
	}
}

// HandlePointer activates only enabled navigation entries.
func (i *NavigationMenuItemWidget) HandlePointer(e ggui.PointerEvent) bool {
	if i.disabled() {
		return false
	}
	return i.Pointer(e, i.activate)
}

// Adopt preserves the button's input state across keyed rebuilds.
func (i *NavigationMenuItemWidget) Adopt(prev any) { i.Interactive.Adopt(prev) }

// NavigationMenuWidget is a row of independently focusable navigation links and
// dropdowns, with one open panel at a time. It is a navigation group, not an
// application action menubar. Popup content uses normal Tab navigation.
type NavigationMenuWidget struct {
	props property.Owner
	ggui.Interactive
	items   []*NavigationMenuItemWidget
	popup   *ggui.PopupWidget
	row     *ggui.RowWidget
	active  int
	pending *NavigationMenuItemWidget
	rtl     bool
}

// NavigationMenu creates a native navigation group. Left/Right and Home/End
// move between enabled top-level entries; Enter/Space activates a link or
// toggles a dropdown, and Up/Down opens a dropdown. Escape or an outside press
// dismisses it and restores focus. Entries belong to just one group.
func NavigationMenu(items ...*NavigationMenuItemWidget) *NavigationMenuWidget {
	n := &NavigationMenuWidget{items: slices.Clone(items), active: -1}
	n.Role = ggui.RoleGroup
	n.SetName("Navigation")
	n.AutoKey()
	children := make([]ggui.Widget, len(items))
	for j, item := range items {
		if item == nil {
			panic("ui: NavigationMenu entry must not be nil")
		}
		item.owner = n
		children[j] = item
	}
	n.row = ggui.Row(children...).Gap(4)
	n.popup = ggui.Popup(n.row, navigationMenuPanel{n}).Owner(n).Modal(false)
	return n
}

// Name names the navigation group.
func (n *NavigationMenuWidget) Name(name string) *NavigationMenuWidget { n.SetName(name); return n }

// BindName follows the navigation group's accessible name.
func (n *NavigationMenuWidget) BindName(r ggui.Readable[string]) *NavigationMenuWidget {
	n.Interactive.BindName(r)
	return n
}

// Key preserves the group and assigns stable positional identities to its entries.
func (n *NavigationMenuWidget) Key(k any) *NavigationMenuWidget {
	n.SetKey(k)
	n.popup.Key(childKey{owner: k, part: "navigation popup"})
	for j, item := range n.items {
		item.Key(childKey{owner: k, part: j})
	}
	return n
}

// Popup returns the shared popup for inspecting or dismissing the open panel.
func (n *NavigationMenuWidget) Popup() *ggui.PopupWidget { return n.popup }

// Disabled disables all entries and closes the open panel.
func (n *NavigationMenuWidget) Disabled(v bool) *NavigationMenuWidget {
	n.SetInert(v)
	if v {
		n.popup.Hide()
	}
	return n
}

// BindDisabled follows a disabled-state reader for the whole group.
func (n *NavigationMenuWidget) BindDisabled(v ggui.Readable[bool]) *NavigationMenuWidget {
	n.BindInert(v)
	return n
}
func (n *NavigationMenuWidget) enabled(j int) bool { return !n.IsInert() && !n.items[j].IsInert() }
func (n *NavigationMenuWidget) current() *NavigationMenuItemWidget {
	if n.active < 0 || n.active >= len(n.items) {
		return nil
	}
	return n.items[n.active]
}

// anchorPoint is where item's panel hangs: under its leading corner.
func (n *NavigationMenuWidget) anchorPoint(item *NavigationMenuItemWidget) ggui.Point {
	return item.rect.Origin.Add(ggui.Pt(pick(n.rtl, item.rect.Size.W, 0.0), item.rect.Size.H))
}

func (n *NavigationMenuWidget) open(item *NavigationMenuItemWidget) {
	if item.disabled() || item.panel == nil {
		return
	}
	n.active = slices.Index(n.items, item)
	n.popup.ShowAt(n.anchorPoint(item))
	n.popup.FocusContent()
}

// Layout implements ggui.Widget.
func (n *NavigationMenuWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer n.props.Layout().EndLayout()
	n.Sync()
	n.rtl = env.Direction() == ggui.DirRTL
	for _, item := range n.items {
		item.Sync()
	}
	if item := n.current(); n.IsInert() || item == nil || item.IsInert() {
		n.popup.Hide()
	}
	return n.popup.Layout(c, env)
}

// Paint implements ggui.Widget.
func (n *NavigationMenuWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.HitPointer(r, n)
	dst.DescribeNode(r, n, func(dst *ggui.Canvas) { dst.Paint(n.popup, r) })
	if item := n.current(); item != nil && n.popup.IsOpen() {
		n.popup.ShowAt(n.anchorPoint(item))
	}
	if n.pending != nil {
		dst.RequestFocus(n.pending)
		n.pending = nil
	}
}

// HandlePointer registers the group's retained identity without consuming input.
func (n *NavigationMenuWidget) HandlePointer(ggui.PointerEvent) bool { return false }

// Adopt retains the active dropdown across a keyed rebuild.
func (n *NavigationMenuWidget) Adopt(prev any) {
	n.Interactive.Adopt(prev)
	if p, ok := prev.(*NavigationMenuWidget); ok && p.active >= 0 && p.active < len(n.items) {
		n.active = p.active
		if p.popup.IsOpen() && n.enabled(n.active) && n.items[n.active].panel != nil {
			n.popup.Show()
		}
	}
}

type navigationMenuPanel struct{ n *NavigationMenuWidget }

func (p navigationMenuPanel) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	item := p.n.current()
	if item == nil || item.panel == nil {
		return ggui.Size{}
	}
	t := uitheme.Of(env)
	return panelBox(item.panel, t).Width(item.width).Pad(t.Space).Layout(c, env)
}
func (p navigationMenuPanel) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if item := p.n.current(); item != nil && item.panel != nil {
		dst.Paint(item.panel, r)
	}
	// Keep top-level triggers reachable above the popup scrim, so a click can
	// switch dropdowns or follow a link without first dismissing the old panel.
	for _, item := range p.n.items {
		if !item.disabled() {
			dst.HitPointer(item.rect, item)
		}
	}
}

// navigationMenuFace keeps the disclosure chevron separate from the label so
// both retain native geometry and the chevron reflects the open state.
type navigationMenuFace struct {
	item      *NavigationMenuItemWidget
	env       ggui.Env
	labelSize ggui.Size
}

func (f *navigationMenuFace) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	f.env = env
	gap := f.item.theme.Space / 2
	inner := c.Loosen()
	inner.MaxW = max(0, inner.MaxW-f.item.theme.IconSize-gap)
	f.labelSize = f.item.label.Layout(inner, env)
	return c.Constrain(ggui.Sz(f.labelSize.W+f.item.theme.IconSize+gap, max(f.labelSize.H, f.item.theme.IconSize)))
}
func (f *navigationMenuFace) Paint(dst *ggui.Canvas, r ggui.Rect) {
	i := f.item
	dst.Paint(i.label, ggui.Rct(r.Origin.Add(ggui.Pt(0, (r.Size.H-f.labelSize.H)/2)), f.labelSize))
	col := i.style.Label
	if i.disabled() {
		col = i.theme.Disabled(col)
	}
	chevron(dst, f.env, i.theme, ggui.Pt(r.Origin.X+r.Size.W-i.theme.IconSize/2, r.Center().Y), pick(i.isOpen(), -2.0, 2.0), col)
}
