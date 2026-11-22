package ui

import (
	"image/color"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/property"
	"github.com/ironpark/ggui/ui/icons"
	"github.com/ironpark/ggui/ui/internal/controlkit"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

// menuItemStyle is how a row is drawn: as a menu's, or as a command list's.
type menuItemStyle uint8

const (
	menuStyle    menuItemStyle = iota
	commandStyle               // taller, with a muted highlight
)

// MenuItemWidget is one row in a Menu: an action, a checkable action or the
// trigger of a submenu. Build one with MenuItem, MenuCheckbox or MenuSub.
type MenuItemWidget struct {
	props property.Owner
	ggui.Interactive
	text         *widget.TextWidget
	label        string
	onTap        func()
	menu         *MenuWidget
	active       bool
	shortcut     *widget.TextWidget
	shortcutSize ggui.Size
	onHover      func()
	motion       controlkit.PointerMotion

	icon        ggui.Widget
	iconSize    ggui.Size
	inset       bool
	destructive bool
	checkable   bool
	checked     ggui.Readable[bool]
	markHidden  bool // checkable, but with no mark drawn or room kept for one: a command entry beside its shortcut
	stay        bool // a click leaves the menu open, as a checkbox or radio row does
	style       menuItemStyle
	radius      float64 // the highlight's corner radius, or 0 for the style's own

	sub      *MenuWidget         // the submenu this row opens, if any
	subPopup *widget.PopupWidget // the popup it opens in

	pad      ggui.EdgeInsets
	textSize ggui.Size
	textX    float64 // where the label starts, from the row's left edge
	theme    *uitheme.Theme
	env      ggui.Env
	popup    *widget.PopupWidget
}

// MenuItem creates an entry that runs onTap and closes the menu.
func MenuItem(label string, onTap func()) *MenuItemWidget {
	it := &MenuItemWidget{text: widget.Text(label).NoWrap(), label: label, onTap: onTap}
	it.Role = ggui.RoleMenuItem
	it.SetName(label)
	it.AutoKey()
	return it
}

// MenuCheckbox creates an entry that toggles checked and shows a check
// mark while it is true. Choosing it leaves the menu open.
func MenuCheckbox(label string, checked ggui.Binding[bool]) *MenuItemWidget {
	property.Require(checked, "MenuCheckbox")
	it := MenuItem(label, func() { checked.Set(!ggui.Untrack(checked.Get)) }).BindChecked(checked)
	it.stay = true
	return it
}

// MenuSub creates an entry that opens entries as a submenu beside it. The
// submenu opens when the pointer rests on the row, on a click, and on
// Right, Enter or Space; Left or Escape closes it and leaves the menu
// around it open.
func MenuSub(label string, entries ...ggui.Widget) *MenuItemWidget {
	it := MenuItem(label, nil)
	it.sub = newMenu()
	it.sub.fit, it.sub.parent = true, it
	it.sub.body.content = it.sub.column(entries)
	it.subPopup = widget.Popup(menuItemFace{it}, menuSurface(it.sub.panel)).Gap(0).Modal(false).Owner(it).
		Side(widget.PopupInlineEnd).Align(widget.PopupStart)
	return it
}

// shortcutTracking is the letter spacing of a shortcut hint in a menu,
// command, context-menu or menubar row: shadcn's tracking-widest.
const shortcutTracking = 0.1

// Shortcut displays a right-aligned key hint; it does not register a
// shortcut. A chord such as "cmd+shift+z" shows as the platform writes it,
// "⇧⌘Z" on macOS and "Ctrl+Shift+Z" elsewhere; other text, such as "⌘N",
// shows as it is.
func (it *MenuItemWidget) Shortcut(s string) *MenuItemWidget {
	defer property.Watch(&it.props, &it.shortcut)()
	s = shortcutLabel(s)
	if it.shortcut == nil {
		it.shortcut = Caption(s).NoWrap().Tracking(shortcutTracking)
	} else {
		it.shortcut.Content(s)
	}
	return it
}

// Icon puts w, usually an Icon, before the label. It is drawn in the
// row's text color.
func (it *MenuItemWidget) Icon(w ggui.Widget) *MenuItemWidget {
	if !property.Same(it.icon, w) {
		it.icon = w
		it.props.Changed()
	}
	return it
}

// Inset starts the label where it would start after an icon, so a row
// without one lines up with the rows that have one.
func (it *MenuItemWidget) Inset() *MenuItemWidget {
	defer property.Watch(&it.props, &it.inset)()
	it.inset = true
	return it
}

// Destructive draws the row in the theme's Destructive color, for an
// action that deletes or cannot be undone.
func (it *MenuItemWidget) Destructive() *MenuItemWidget {
	defer property.Watch(&it.props, &it.destructive)()
	it.destructive = true
	return it
}

// Disabled greys the item out and ignores it while v is true.
func (it *MenuItemWidget) Disabled(v bool) *MenuItemWidget { it.SetInert(v); return it }

// BindDisabled follows r for Disabled without a rebuild.
func (it *MenuItemWidget) BindDisabled(r ggui.Readable[bool]) *MenuItemWidget {
	it.BindInert(r)
	return it
}

// Checked makes the row checkable and shows a check mark at its right
// edge while v is true. A checkable row keeps room for the mark either
// way, so the label does not move as it comes and goes.
func (it *MenuItemWidget) Checked(v bool) *MenuItemWidget { return it.BindChecked(ggui.Const(v)) }

// BindChecked is Checked following r.
func (it *MenuItemWidget) BindChecked(r ggui.Readable[bool]) *MenuItemWidget {
	property.Require(r, "BindChecked")
	defer property.Watch(&it.props, &it.checkable)()
	it.checked, it.checkable = r, true
	return it
}

// isChecked reports whether the row shows its check mark.
func (it *MenuItemWidget) isChecked() bool {
	return it.checked != nil && ggui.Untrack(it.checked.Get)
}

// Describe implements ggui.Describer: whether a checkable row is checked
// and whether a submenu is showing.
func (it *MenuItemWidget) Describe() ggui.Node {
	n := ggui.Node{Role: it.Role, Name: it.SemanticName(), Disabled: it.IsInert(), Actions: ggui.ActionPress | ggui.ActionFocus}
	if it.checkable {
		n.Checked = ggui.Tri(it.isChecked())
		// A combobox's option is selected, as a Select's is, when checked.
		n.Selected = it.Role == ggui.RoleOption && it.isChecked()
	}
	if it.sub != nil {
		open := it.subPopup.IsOpen()
		n.Expanded = ggui.Expandable(open)
		n.Actions |= pick(open, ggui.ActionCollapse, ggui.ActionExpand)
	}
	return n
}

// Act implements ggui.Actor: pressing the row, and opening or closing its
// submenu.
func (it *MenuItemWidget) Act(a ggui.Action) bool {
	if it.IsInert() {
		return false
	}
	switch {
	case a.Kind == ggui.ActionPress:
		it.run()
	case a.Kind == ggui.ActionExpand && it.sub != nil:
		it.openSub(true)
	case a.Kind == ggui.ActionCollapse && it.sub != nil && it.menu != nil:
		it.menu.closeSub()
	default:
		return false
	}
	return true
}

// dismiss closes the popup the row is in, and every menu it is a submenu
// of.
func (it *MenuItemWidget) dismiss() {
	for it != nil {
		if it.menu != nil {
			it.menu.closeSub()
		}
		if it.popup != nil {
			it.popup.Hide()
		}
		if it.menu == nil {
			return
		}
		it = it.menu.parent
	}
}

func (it *MenuItemWidget) run() {
	if it.IsInert() {
		return
	}
	if it.sub != nil {
		if it.menu == nil || it.menu.showing() != it {
			it.openSub(true)
		}
		return
	}
	if it.onTap != nil {
		it.onTap()
	}
	if !it.stay {
		it.dismiss()
	}
}

// openSub shows the row's submenu, closing any other its menu was showing.
// first highlights the submenu's first item, as opening from the keyboard
// does.
func (it *MenuItemWidget) openSub(first bool) {
	if it.sub == nil || it.IsInert() {
		return
	}
	if it.menu != nil {
		if it.menu.sub != it {
			it.menu.closeSub()
		}
		it.menu.sub = it
		it.menu.hover = -1
	}
	it.sub.closeSub()
	it.sub.current, it.sub.hover = -1, -1
	it.sub.search.clear()
	for _, row := range it.sub.items {
		row.Sync()
	}
	if first {
		it.sub.jumpFirst()
	}
	it.subPopup.Show()
}

// placeSub closes an open submenu whose menu has closed. The popup itself
// keeps it beside the row: at the end of it in the reading direction, or
// at the start where the window has no room, entering from that side.
func (it *MenuItemWidget) placeSub() {
	if !it.subPopup.IsOpen() {
		return
	}
	// A submenu goes with the menu it is in.
	if it.popup != nil && !it.popup.IsOpen() || it.IsInert() {
		it.sub.closeSub()
		it.subPopup.Hide()
	}
}

// menuItemFace is a submenu row as its popup's anchor: the row itself.
type menuItemFace struct{ it *MenuItemWidget }

func (f menuItemFace) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return f.it.layoutRow(c, env)
}
func (f menuItemFace) Paint(dst *ggui.Canvas, r ggui.Rect) { f.it.paintRow(dst, r) }

// Layout implements Widget.
func (it *MenuItemWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer it.props.Layout().EndLayout()
	it.Sync()
	it.theme = uitheme.Of(env)
	it.env = env
	it.popup, _ = widget.PopupOf(env)
	if it.sub == nil {
		return it.layoutRow(c, env)
	}
	// A submenu lifts further than the menu it opens from: shadcn's
	// shadow-lg, the theme's OverlayShadow over its OverlayContact.
	layers := [2]ggui.ShadowStyle{it.theme.OverlayShadow, it.theme.OverlayContact}
	it.sub.chrome(it.theme).Shadow(layers[:]...)
	// The submenu's first row lines up with this one: it rises by the
	// panel's padding less its border, shadcn's alignOffset of -3.
	it.subPopup.AlignOffset(-(it.theme.PanelPad.Top - it.theme.BorderWidth))
	if it.menu != nil {
		root := it.menu.root()
		it.sub.checkLeft = root.checkLeft
		it.subPopup.Keys(root.keys)
	}
	return it.subPopup.Layout(c, env)
}

// ink is the color the row's label, icon and marks are drawn in.
func (it *MenuItemWidget) ink(lit bool) color.Color {
	t := it.theme
	var c color.Color = t.PopoverFg
	switch {
	case it.destructive:
		c = t.Destructive
	case it.style == commandStyle:
		c = t.Fg
	case lit:
		c = t.AccentFg
	}
	if it.IsInert() {
		c = t.Disabled(c)
	}
	return c
}

// layoutRow measures the row: an icon, the label, and a shortcut, a check
// mark or a submenu's chevron at the right edge.
func (it *MenuItemWidget) layoutRow(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := it.theme
	it.pad = t.ItemPad
	if it.style == commandStyle {
		it.pad = ggui.Insets(t.Space*.75, t.Space)
	}
	markLeft := it.menu != nil && it.menu.checkLeft
	switch {
	case it.inset, it.checkable && markLeft:
		it.pad.Left = menuInset(t)
	}
	if it.checkable && !markLeft && !it.markHidden {
		it.pad.Right = menuMarkRoom(t)
	}
	ink := it.ink(false)
	it.text.Color(ink) // Paint swaps in AccentFg under the highlight
	inner := it.pad.Shrink(c).Loosen()
	lead, trail := 0.0, 0.0
	it.iconSize = ggui.Size{}
	if it.icon != nil {
		it.iconSize = it.icon.Layout(ggui.Loose(ggui.Sz(t.IconSize, t.IconSize)), env.WithText(ggui.TextStyle{Color: ink}))
		lead = it.iconSize.W + pick(it.style == commandStyle, t.Space, menuGap(t))
	}
	it.shortcutSize = ggui.Size{}
	if it.shortcut != nil {
		it.shortcut.Color(pick(it.IsInert(), t.Disabled(t.MutedFg), t.MutedFg))
		it.shortcutSize = it.shortcut.Layout(inner, env)
		trail = it.shortcutSize.W + t.Space*3
	}
	if it.sub != nil {
		trail += t.IconSize + menuGap(t)
	}
	it.textX = it.pad.Left + lead
	inner.MaxW = max(0, inner.MaxW-lead-trail)
	it.textSize = it.text.Layout(inner, env)
	return c.Constrain(it.pad.Inflate(ggui.Sz(lead+it.textSize.W+trail, max(it.textSize.H, it.shortcutSize.H, it.iconSize.H))))
}

// Paint implements Widget.
func (it *MenuItemWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if it.sub == nil {
		it.paintRow(dst, r)
		return
	}
	it.placeSub()
	dst.Paint(it.subPopup, r)
}

// paintRow paints the row and registers its input.
func (it *MenuItemWidget) paintRow(dst *ggui.Canvas, r ggui.Rect) {
	t := it.theme
	// A disabled item takes no input but is still part of the menu.
	dst.Describe(r, it)
	lit := false
	if !it.IsInert() {
		dst.HitPointer(r, it)
		lit = it.active || (it.onHover == nil && it.Hovered)
		if lit {
			var fill color.Color = t.Accent
			radius := t.RadiusSm
			switch {
			case it.destructive:
				fill = fade(t.Destructive, pick(controlkit.IsDark(t.Bg), .2, .1))
			case it.style == commandStyle:
				fill, radius = t.Muted, t.Radius*.6
			}
			dst.FillRoundRect(r, pick(it.radius > 0, it.radius, radius), fill)
		}
	}
	ink := it.ink(lit)
	it.text.Color(ink)
	right := r.Origin.X + r.Size.W - it.pad.Right
	if it.icon != nil {
		if icon, ok := it.icon.(*icons.Widget); ok {
			icon.Color(ink)
		}
		dst.Paint(it.icon, ggui.Rct(ggui.Pt(r.Origin.X+it.pad.Left, r.Origin.Y+(r.Size.H-it.iconSize.H)/2), it.iconSize))
	}
	if it.checkable && it.isChecked() && !it.markHidden {
		x := r.Origin.X + r.Size.W - t.Space - t.IconSize/2
		if it.menu != nil && it.menu.checkLeft {
			x = r.Origin.X + t.ItemPad.Left + t.IconSize/2
		}
		controlkit.PaintIconAt(dst, it.env, controlkit.IconMark{Role: icons.Check, Center: ggui.Pt(x, r.Origin.Y+r.Size.H/2), Size: t.IconSize, Color: ink})
	}
	if it.sub != nil {
		controlkit.PaintIconAt(dst, it.env, controlkit.IconMark{Role: icons.ChevronRight, Center: ggui.Pt(right-t.IconSize/2, r.Origin.Y+r.Size.H/2), Size: t.IconSize, Color: ink})
		right -= t.IconSize + menuGap(t)
	}
	if it.shortcut != nil {
		if !it.IsInert() {
			it.shortcut.Color(pick(lit, pick(it.style == commandStyle, t.Fg, t.AccentFg), t.MutedFg))
		}
		dst.Paint(it.shortcut, ggui.Rct(ggui.Pt(right-it.shortcutSize.W, r.Origin.Y+(r.Size.H-it.shortcutSize.H)/2), it.shortcutSize))
		right -= it.shortcutSize.W
	}
	text := dst
	if it.textX+it.textSize.W > right-r.Origin.X {
		text = dst.Clip(r)
	}
	text.Paint(it.text, ggui.Rct(ggui.Pt(r.Origin.X+it.textX, r.Origin.Y+(r.Size.H-it.textSize.H)/2), it.textSize))
}

// HandlePointer implements PointerHandler.
func (it *MenuItemWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind == ggui.PointerMove && it.motion.Moved(ev.Pos) && !it.IsInert() && it.onHover != nil {
		it.onHover()
	}
	if ev.Kind == ggui.PointerExit && it.menu != nil {
		it.menu.pointLeft(it)
	}
	return it.Pointer(ev, it.run)
}
