package ui

import (
	"image/color"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// menuSubDelay is how long the pointer rests on a row before its submenu
// opens, or before the submenu of another row closes.
const menuSubDelay = 100 * time.Millisecond

// MenuWidget is a button that opens a list of actions. Build one with Menu
// or MenuOf, with MenuItem, MenuCheckbox, MenuRadioGroup, MenuSub, MenuLabel
// and MenuDivider as the entries.
type MenuWidget struct {
	props    property.Owner
	button   *ButtonWidget
	popup    *ggui.PopupWidget
	panel    *ggui.BoxWidget
	body     *menuBody
	items    []*MenuItemWidget
	current  int // the item the keyboard is on while open, or -1
	width    float64
	widthSet bool
	theme    *uitheme.Theme

	// fit sizes the panel to its entries, as a context menu, a menubar's
	// menu and a submenu are; a dropdown takes the theme's MenuWidth
	// instead.
	fit bool
	// checkLeft draws check marks in a gutter before the label, as a
	// menubar's menus do, instead of at the right edge.
	checkLeft bool

	// keys is the handler that has the keyboard while the menu is open: the
	// menu itself, or the context menu or menubar showing its panel.
	keys   ggui.KeyHandler
	parent *MenuItemWidget // the row this submenu opens from, nil for a top menu
	sub    *MenuItemWidget // the row whose submenu is showing, if any

	hover   int       // the row the pointer came to and has not been acted on, or -1
	hoverAt time.Time // when it came there
	took    time.Time // the frame a submenu last used Escape, Left or Right

	pointed bool      // the pointer, not the keyboard, put the highlight where it is
	typed   string    // what has been typed to find a row, lower-cased
	typedAt time.Time // when its last character came
}

// menuTypeReset is how long after a character the next one starts a new
// search rather than adding to it: Base UI's typeahead reset.
const menuTypeReset = 750 * time.Millisecond

// Menu creates a secondary button labelled label that opens entries below
// it. Space or Enter opens the menu, the arrow keys move through the
// items, Enter runs one and Escape closes it. Right or Enter opens a
// submenu, and Left or Escape closes just that submenu. Typing moves to the
// next row whose label starts with what was typed.
func Menu(label string, entries ...ggui.Widget) *MenuWidget {
	m := newMenu()
	m.button = Button(label, m.toggle).Outline()
	m.button.Role = ggui.RoleMenu
	m.init(entries)
	return m
}

// MenuOf is Menu with any content as the button's face.
func MenuOf(content ggui.Widget, entries ...ggui.Widget) *MenuWidget {
	m := newMenu()
	m.button = ButtonOf(content, m.toggle).Outline()
	m.button.Role = ggui.RoleMenu
	m.init(entries)
	return m
}

// newMenu is a menu with no entries, button or popup yet.
func newMenu() *MenuWidget {
	m := &MenuWidget{current: -1, hover: -1}
	m.body = &menuBody{m: m}
	m.panel = ggui.Box(m.body)
	return m
}

// Name names a MenuOf for Probe.Find and the inspector.
func (m *MenuWidget) Name(s string) *MenuWidget { m.button.SetName(s); return m }

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

// ConsumesKey implements ggui.KeyConsumer: Up and Down open and move,
// Home and End move while open, and Left, Right and Escape belong to an
// open submenu.
func (m *MenuWidget) ConsumesKey(ev ggui.KeyEvent) bool {
	return ggui.Activates(ev) || ev.Kind == ggui.KeyPress && (ev.Key == ggui.KeyArrowUp || ev.Key == ggui.KeyArrowDown) ||
		m.popup.IsOpen() && (navConsumes(ev, navVertical) || m.consumes(ev))
}

func (m *MenuWidget) init(entries []ggui.Widget) {
	m.body.content = m.column(entries)
	m.keys = m
	m.popup = ggui.Popup(m.button, menuSurface(m.panel)).Keys(m).Owner(m.button)
	m.button.Expands(m.popup.IsOpen).Opens(m)
}

// menuGroup is an entry that stands for several: a radio group. The menu
// takes the rows it lists as its own, so the keyboard moves through them.
type menuGroup interface{ menuEntries() []ggui.Widget }

// adopt makes the item rows among entries, and inside the groups among
// them, m's own.
func (m *MenuWidget) adopt(entries []ggui.Widget) {
	for _, e := range entries {
		switch e := e.(type) {
		case *MenuItemWidget:
			e.menu = m
			index := len(m.items)
			e.onHover = func() { m.pointAt(index) }
			m.items = append(m.items, e)
		case *MenuLabelWidget:
			e.menu = m
		case menuGroup:
			m.adopt(e.menuEntries())
		}
	}
}

// column makes m's items of entries and lays them out as its panel does.
func (m *MenuWidget) column(entries []ggui.Widget) ggui.Widget {
	m.adopt(entries)
	return newMenuColumn(entries)
}

// menuColumn stacks a menu's entries at one width: the widest entry's,
// within what its constraints allow. A Column would stretch them to all
// the room there is, and a panel as wide as its entries has none to give.
type menuColumn struct {
	entries []ggui.Widget
	sizes   []ggui.Size
	fill    bool // take all the width there is, as a Select's list does
}

func newMenuColumn(entries []ggui.Widget) *menuColumn {
	return &menuColumn{entries: entries, sizes: make([]ggui.Size, len(entries))}
}

func (col *menuColumn) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	w := c.MaxW
	if c.MinW < c.MaxW && !(col.fill && c.MaxW < ggui.Unbounded) {
		w = 0
		for _, e := range col.entries {
			w = max(w, e.Layout(ggui.Constraints{MaxW: c.MaxW, MaxH: ggui.Unbounded}, env).W)
		}
		w = clamp(w, c.MinW, c.MaxW)
	}
	h := 0.0
	for i, e := range col.entries {
		col.sizes[i] = e.Layout(ggui.Constraints{MinW: w, MaxW: w, MaxH: ggui.Unbounded}, env)
		h += col.sizes[i].H
	}
	return c.Constrain(ggui.Sz(w, h))
}

func (col *menuColumn) Paint(dst *ggui.Canvas, r ggui.Rect) {
	y := r.Origin.Y
	for i, e := range col.entries {
		dst.Paint(e, ggui.Rct(ggui.Pt(r.Origin.X, y), ggui.Sz(r.Size.W, col.sizes[i].H)))
		y += col.sizes[i].H
	}
}

// Width sets the popup panel width, constrained to the available space.
// Without one the panel takes the theme's MenuWidth.
func (m *MenuWidget) Width(w float64) *MenuWidget {
	defer property.Watch(&m.props, &m.width)()
	defer property.Watch(&m.props, &m.widthSet)()
	m.width, m.widthSet = max(w, 0), true
	return m
}

// Popup returns the popup the entries open in.
func (m *MenuWidget) Popup() *ggui.PopupWidget { return m.popup }

func (m *MenuWidget) toggle() {
	if m.popup.IsOpen() {
		m.popup.Hide()
		return
	}
	m.closeSub()
	m.current, m.hover, m.typed = -1, -1, ""
	m.popup.Show()
}

// Act implements ggui.Actor. The node is the button's, so the button hands
// the action on to the menu that owns it.
func (m *MenuWidget) Act(a ggui.Action) bool {
	if m.button.IsInert() {
		return false
	}
	switch a.Kind {
	case ggui.ActionExpand:
		m.closeSub()
		m.current, m.hover = -1, -1
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
	defer m.props.Layout().EndLayout()
	m.button.Sync()
	if m.button.IsInert() {
		m.popup.Hide()
	}
	t := uitheme.Of(env)
	m.theme = t
	m.chrome(t)
	return m.popup.Layout(c, env)
}

// Paint implements Widget.
func (m *MenuWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Paint(m.popup, r)
	if !m.button.IsInert() {
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
	if ev.Kind == ggui.KeyText && m.popup.IsOpen() {
		m.typeahead(ev.Text)
		return
	}
	if ev.Kind != ggui.KeyPress || len(m.items) == 0 {
		m.button.HandleKey(ev)
		return
	}
	open := m.popup.IsOpen()
	switch ev.Key {
	case ggui.KeyEscape:
		if !open || !m.key(ev) {
			m.popup.Hide()
		}
	case ggui.KeyArrowUp, ggui.KeyArrowDown:
		if !open {
			m.toggle()
		}
		m.key(ev)
	case ggui.KeyHome, ggui.KeyEnd, ggui.KeyArrowLeft, ggui.KeyArrowRight:
		if !open || !m.key(ev) {
			m.button.HandleKey(ev)
		}
	case ggui.KeyEnter, ggui.KeyNumpadEnter, ggui.KeySpace:
		if open {
			// With no row highlighted there is nothing to run, and the
			// menu stays as it is.
			m.key(ev)
			return
		}
		m.button.HandleKey(ev)
		if m.popup.IsOpen() {
			// Opened from the keyboard, the menu starts on its first row.
			m.jump(1)
		}
	default:
		m.button.HandleKey(ev)
	}
}

// chrome dresses the panel. A menubar and a context menu paint the panel
// through their own popup, so the width lives here rather than in Layout.
func (m *MenuWidget) chrome(t *uitheme.Theme) *ggui.BoxWidget {
	m.theme = t
	if t.Menu().Popover != t.Popover {
		mt := t.Menu()
		t = &mt
	}
	panelBox(m.panel, t)
	switch {
	case m.widthSet:
		m.panel.Width(m.width)
	case !m.fit:
		m.panel.Width(t.MenuWidth)
	}
	return m.panel
}

// enabled reports whether item i can take the highlight.
func (m *MenuWidget) enabled(i int) bool { return !m.items[i].IsInert() }

// navigate moves the highlight for Up, Down, Home or End, wrapping and
// skipping disabled items, and reports whether ev was one of them.
func (m *MenuWidget) navigate(ev ggui.KeyEvent) bool {
	i, ok := navIndex(ev, navVertical, m.current, len(m.items), m.enabled)
	if ok {
		m.current, m.hover, m.pointed = i, -1, false
	}
	return ok
}

// jump puts the highlight on the first enabled item from the near end:
// the top for dir 1, the bottom for -1.
func (m *MenuWidget) jump(dir int) {
	m.current, m.pointed = stepIndex(-1, dir, len(m.items), m.enabled), false
}

// reopen refreshes the items and, when first is set, highlights the first
// one: a menu opened from the keyboard starts there, and one the pointer
// opened shows no highlight until the pointer or a key gives it one.
// Owners that show the panel through their own popup call it in place of
// toggle.
func (m *MenuWidget) reopen(first bool) {
	m.closeSub()
	m.hover, m.current, m.typed = -1, -1, ""
	for _, it := range m.items {
		it.Sync()
	}
	if first {
		m.jump(1)
	}
}

// activate runs the highlighted item, or opens its submenu, and reports
// whether there was one.
func (m *MenuWidget) activate() bool {
	if m.current < 0 || m.current >= len(m.items) {
		return false
	}
	m.items[m.current].run()
	return true
}

// root is the top menu m is a submenu of, or m itself.
func (m *MenuWidget) root() *MenuWidget {
	for m.parent != nil && m.parent.menu != nil {
		m = m.parent.menu
	}
	return m
}

// showing is the row of m whose submenu is open, or nil.
func (m *MenuWidget) showing() *MenuItemWidget {
	if m.sub != nil && m.sub.subPopup.IsOpen() {
		return m.sub
	}
	return nil
}

// leaf is the innermost open submenu under m, or m itself: the menu the
// keyboard is in.
func (m *MenuWidget) leaf() *MenuWidget {
	for {
		it := m.showing()
		if it == nil {
			return m
		}
		m = it.sub
	}
}

// closeSub closes the submenu m is showing, and the ones inside it.
func (m *MenuWidget) closeSub() {
	if it := m.sub; it != nil {
		it.sub.closeSub()
		it.subPopup.Hide()
	}
	m.sub = nil
}

// consumes reports whether ev is a key an open submenu of m keeps from its
// owner: Escape and Left close it. Right opens the submenu of the
// highlighted row.
func (m *MenuWidget) consumes(ev ggui.KeyEvent) bool {
	if ev.Kind != ggui.KeyPress {
		return false
	}
	leaf := m.leaf()
	switch ev.Key {
	case ggui.KeyEscape, ggui.KeyArrowLeft:
		// The focus scope asks after the key was handled, by when the
		// submenu it closed is gone: the frame it was used in still counts.
		return leaf != m || m.took.Equal(ggui.FrameTime()) && !m.took.IsZero()
	case ggui.KeyArrowRight:
		return leaf.opens() || m.took.Equal(ggui.FrameTime()) && !m.took.IsZero()
	}
	return false
}

// opens reports whether the highlighted row of m opens a submenu.
func (m *MenuWidget) opens() bool {
	return m.current >= 0 && m.current < len(m.items) && m.items[m.current].sub != nil && !m.items[m.current].IsInert()
}

// typing reports whether a search by typing is under way in m.
func (m *MenuWidget) typing() bool {
	return m.typed != "" && ggui.FrameTime().Sub(m.typedAt) < menuTypeReset
}

// typeahead gives typed text to the innermost open menu under m, which
// moves its highlight to the next enabled row whose label starts with what
// has been typed since the last pause. The same letter again moves on to
// the next row that starts with it. It reports whether a row was found.
func (m *MenuWidget) typeahead(text string) bool {
	leaf := m.leaf()
	text = strings.ToLower(text)
	if !leaf.typing() {
		leaf.typed = ""
		if strings.TrimSpace(text) == "" {
			// A space starts no search: it runs the highlighted row.
			return false
		}
	}
	from := max(leaf.current, -1)
	switch {
	case leaf.typed == text && utf8.RuneCountInString(text) == 1:
		// The same letter again: on to the next row under it.
	case leaf.typed != "":
		// A longer search may still be the row it is on.
		leaf.typed += text
		from = max(from-1, -1)
	default:
		leaf.typed = text
	}
	leaf.typedAt = ggui.FrameTime()
	n := len(leaf.items)
	for step := 1; step <= n; step++ {
		i := ((from+step)%n + n) % n
		if leaf.enabled(i) && strings.HasPrefix(strings.ToLower(leaf.items[i].label), leaf.typed) {
			leaf.current, leaf.hover, leaf.pointed = i, -1, false
			return true
		}
	}
	return false
}

// key gives a key press to the innermost open menu under m and reports
// whether it was used: the arrows, Home and End move, Enter and Space run
// the highlighted item or open its submenu, Right opens a submenu, and
// Left and Escape close one. With no submenu open, Left and Escape are
// left to the owner.
func (m *MenuWidget) key(ev ggui.KeyEvent) bool {
	if ev.Kind != ggui.KeyPress {
		return false
	}
	leaf := m.leaf()
	switch {
	case ev.Key == ggui.KeyEscape, ev.Key == ggui.KeyArrowLeft:
		if leaf == m {
			return false
		}
		leaf.parent.menu.closeSub()
		m.took = ggui.FrameTime()
		return true
	case ev.Key == ggui.KeyArrowRight:
		if !leaf.opens() {
			return false
		}
		leaf.items[leaf.current].openSub(true)
		m.took = ggui.FrameTime()
		return true
	case ev.Key == ggui.KeySpace && leaf.typing():
		// A space in the middle of a search is part of it.
		return true
	case ggui.Activates(ev):
		return leaf.activate()
	}
	return leaf.navigate(ev)
}

// pointAt moves the highlight to the row the pointer came to, and notes
// when, for the submenu that opens or closes once it has rested there.
func (m *MenuWidget) pointAt(i int) {
	m.current, m.pointed = i, true
	if p := m.parent; p != nil && p.menu != nil {
		// The pointer is inside this submenu: the row it opened from keeps
		// the highlight of the menu around it.
		p.menu.hover = -1
		for j, row := range p.menu.items {
			if row == p {
				p.menu.current = j
			}
		}
	}
	it, shown := m.items[i], m.showing()
	if it.sub != nil && shown == it || it.sub == nil && shown == nil {
		m.hover = -1
		return
	}
	if m.hover != i {
		m.hover, m.hoverAt = i, ggui.FrameTime()
	}
}

// pointLeft forgets a row the pointer left before resting on it, so a
// pointer crossing other rows on its way into a submenu does not close it,
// and takes the highlight off the row if the pointer gave it: a row is lit
// under the pointer, not where the pointer last was. The row of an open
// submenu stays lit through that submenu.
func (m *MenuWidget) pointLeft(it *MenuItemWidget) {
	if m.hover >= 0 && m.hover < len(m.items) && m.items[m.hover] == it {
		m.hover = -1
	}
	if m.current >= 0 && m.current < len(m.items) && m.items[m.current] == it && m.pointed {
		m.current, m.pointed = -1, false
	}
}

// settle acts on a pointer that has rested on a row: the row's submenu
// opens and any other closes.
func (m *MenuWidget) settle() {
	if m.hover < 0 || m.hover >= len(m.items) {
		return
	}
	due := m.hoverAt.Add(menuSubDelay)
	if ggui.FrameTime().Before(due) {
		ggui.WakeAt(due)
		return
	}
	it := m.items[m.hover]
	m.hover = -1
	switch {
	case it.sub != nil && !it.IsInert():
		if m.showing() != it {
			it.openSub(false)
		}
	case m.showing() != nil:
		m.closeSub()
	}
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

// menuBody is a menu panel's content: the column of entries, at least as
// wide as a fitted panel's minimum.
type menuBody struct {
	m       *MenuWidget
	content ggui.Widget
}

func (b *menuBody) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	m := b.m
	if m.fit && !m.widthSet {
		// A context menu or a menubar's menu is at least 144 wide, and a
		// submenu 96.
		t := uitheme.Of(env)
		least := t.Space * pick(m.parent != nil, 12.0, 18.0)
		c.MinW = max(c.MinW, min(least-t.PanelPad.Horizontal(), c.MaxW))
	} else if c.MaxW < ggui.Unbounded {
		c.MinW = c.MaxW
	}
	if b.content == nil {
		return c.Constrain(ggui.Size{})
	}
	return b.content.Layout(c, env)
}

func (b *menuBody) Paint(dst *ggui.Canvas, r ggui.Rect) {
	m := b.m
	m.settle()
	shown := m.showing()
	for i, it := range m.items {
		it.active = i == m.current || it == shown
	}
	if b.content != nil {
		dst.Paint(b.content, r)
	}
}

// menuGap is the space between the parts of a row: an icon and its label.
func menuGap(t *uitheme.Theme) float64 { return t.Space * .75 }

// menuInset is where the label of an inset row starts: past the room an
// icon takes in the rows around it.
func menuInset(t *uitheme.Theme) float64 { return t.ItemPad.Left + t.IconSize + menuGap(t) }

// menuMarkRoom is the right padding of a row that can carry a check mark.
func menuMarkRoom(t *uitheme.Theme) float64 { return t.IconSize + 2*t.Space }

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
	text         *ggui.TextWidget
	label        string
	onTap        func()
	menu         *MenuWidget
	active       bool
	shortcut     *ggui.TextWidget
	shortcutSize ggui.Size
	onHover      func()
	motion       pointerMotion

	icon        ggui.Widget
	iconSize    ggui.Size
	inset       bool
	destructive bool
	checkable   bool
	checked     ggui.Readable[bool]
	stay        bool // a click leaves the menu open, as a checkbox or radio row does
	style       menuItemStyle
	radius      float64 // the highlight's corner radius, or 0 for the style's own

	sub      *MenuWidget       // the submenu this row opens, if any
	subPopup *ggui.PopupWidget // the popup it opens in

	pad      ggui.EdgeInsets
	textSize ggui.Size
	textX    float64 // where the label starts, from the row's left edge
	theme    *uitheme.Theme
	env      ggui.Env
	popup    *ggui.PopupWidget
}

// MenuItem creates an entry that runs onTap and closes the menu.
func MenuItem(label string, onTap func()) *MenuItemWidget {
	it := &MenuItemWidget{text: ggui.Text(label).NoWrap(), label: label, onTap: onTap}
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
	it.subPopup = ggui.Popup(menuItemFace{it}, menuSurface(it.sub.panel)).Gap(0).Modal(false).Owner(it).
		Side(ggui.PopupInlineEnd).Align(ggui.PopupStart)
	return it
}

// Shortcut displays a right-aligned key hint; it does not register a
// shortcut. A chord such as "cmd+shift+z" shows as the platform writes it,
// "⇧⌘Z" on macOS and "Ctrl+Shift+Z" elsewhere; other text, such as "⌘N",
// shows as it is.
func (it *MenuItemWidget) Shortcut(s string) *MenuItemWidget {
	defer property.Watch(&it.props, &it.shortcut)()
	s = shortcutLabel(s)
	if it.shortcut == nil {
		it.shortcut = Caption(s).NoWrap()
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

// MenuDivider is a line between groups of items, from one edge of the
// panel to the other.
func MenuDivider() ggui.Widget { return &menuDivider{} }

// menuDivider is MenuDivider's line. The panel's padding is around the
// rows, so the line paints across it to reach the panel's edges.
type menuDivider struct{ theme *uitheme.Theme }

func (d *menuDivider) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	d.theme = uitheme.Of(env)
	t := d.theme
	return c.Constrain(ggui.Sz(c.MinW, t.PanelPad.Vertical()+t.BorderWidth))
}

func (d *menuDivider) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := d.theme
	dst.Leaf(r, ggui.Node{Role: ggui.RoleSeparator})
	dst.FillRect(ggui.Rct(ggui.Pt(r.Origin.X-t.PanelPad.Left, r.Origin.Y+(r.Size.H-t.BorderWidth)/2),
		ggui.Sz(r.Size.W+t.PanelPad.Horizontal(), t.BorderWidth)), t.Border)
}

// MenuLabelWidget is a heading over a group of menu entries. Build one
// with MenuLabel.
type MenuLabelWidget struct {
	props property.Owner
	text  *ggui.TextWidget
	menu  *MenuWidget // the menu the heading is in, which sets its size
	inset bool
	pad   ggui.EdgeInsets
	size  ggui.Size
}

// MenuLabel creates a small, muted heading for the entries after it. It
// takes no input and the keyboard passes over it. In a Menubar's menu it
// is set at the size of the rows.
func MenuLabel(label string) *MenuLabelWidget {
	return &MenuLabelWidget{text: Caption(label).Weight(ggui.WeightMedium).NoWrap()}
}

// Inset starts the heading where an inset row's label starts.
func (l *MenuLabelWidget) Inset() *MenuLabelWidget {
	defer property.Watch(&l.props, &l.inset)()
	l.inset = true
	return l
}

// Layout implements Widget.
func (l *MenuLabelWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer l.props.Layout().EndLayout()
	t := uitheme.Of(env)
	l.pad = t.ItemPad
	if l.inset {
		l.pad.Left = menuInset(t)
	}
	if l.menu != nil && l.menu.checkLeft {
		// shadcn's MenubarLabel passes text-sm over the dropdown label's
		// text-xs and keeps its text-muted-foreground.
		l.text.Style(ggui.TextStyle{Size: t.Text.Size, LineHeight: t.Text.LineHeight})
	}
	l.size = l.text.Color(t.MutedFg).Layout(l.pad.Shrink(c).Loosen(), env)
	return c.Constrain(l.pad.Inflate(l.size))
}

// Paint implements Widget.
func (l *MenuLabelWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Clip(r).Paint(l.text, ggui.Rct(ggui.Pt(r.Origin.X+l.pad.Left, r.Origin.Y+(r.Size.H-l.size.H)/2), l.size))
}

// MenuRadioGroupWidget is a run of menu rows of which one is chosen. Build
// one with MenuRadioGroup.
type MenuRadioGroupWidget[T comparable] struct {
	value    ggui.Binding[T]
	options  []T
	rows     []*MenuItemWidget
	entries  []ggui.Widget
	column   *menuColumn
	onChange func(T)
}

// MenuRadioGroup creates one row per option, with a check mark on the one
// value holds. Choosing a row writes its option to value and leaves the
// menu open. The rows show each option through fmt.Sprint until Format
// says otherwise.
func MenuRadioGroup[T comparable](value ggui.Binding[T], options ...T) *MenuRadioGroupWidget[T] {
	property.Require(value, "MenuRadioGroup")
	g := &MenuRadioGroupWidget[T]{value: value, options: append([]T(nil), options...)}
	for _, option := range g.options {
		row := MenuItem(sprint(option), func() { setChanged(value, option, g.onChange) }).
			BindChecked(ggui.Map(value, func(v T) bool { return v == option }))
		row.stay = true
		g.rows = append(g.rows, row)
		g.entries = append(g.entries, row)
	}
	g.column = newMenuColumn(g.entries)
	return g
}

// Format sets how each option is shown.
func (g *MenuRadioGroupWidget[T]) Format(fn func(T) string) *MenuRadioGroupWidget[T] {
	for i, row := range g.rows {
		row.label = fn(g.options[i])
		row.text.Content(row.label)
		row.SetName(row.label)
	}
	return g
}

// Inset starts every row's label where it would start after an icon.
func (g *MenuRadioGroupWidget[T]) Inset() *MenuRadioGroupWidget[T] {
	for _, row := range g.rows {
		row.Inset()
	}
	return g
}

// OnChange fires with the new value after the user picks a different row.
func (g *MenuRadioGroupWidget[T]) OnChange(fn func(T)) *MenuRadioGroupWidget[T] {
	g.onChange = fn
	return g
}

// Items returns the group's rows in option order, to give one an icon or
// disable it.
func (g *MenuRadioGroupWidget[T]) Items() []*MenuItemWidget { return g.rows }

func (g *MenuRadioGroupWidget[T]) menuEntries() []ggui.Widget { return g.entries }

// Layout implements Widget.
func (g *MenuRadioGroupWidget[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return g.column.Layout(c, env)
}

// Paint implements Widget.
func (g *MenuRadioGroupWidget[T]) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(g.column, r) }

// dismiss closes the popup the row is in, and every menu it is a submenu
// of.
func (it *MenuItemWidget) dismiss() {
	for it != nil {
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
	it.sub.current, it.sub.hover, it.sub.typed = -1, -1, ""
	for _, row := range it.sub.items {
		row.Sync()
	}
	if first {
		it.sub.jump(1)
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
	it.popup, _ = ggui.PopupOf(env)
	if it.sub == nil {
		return it.layoutRow(c, env)
	}
	// A submenu lifts further than the menu it opens from: shadcn's shadow-lg.
	it.sub.chrome(it.theme).Shadow(it.theme.OverlayShadow)
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
	if it.checkable && !markLeft {
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
				fill = fade(t.Destructive, pick(isDark(t.Bg), .2, .1))
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
	if it.checkable && it.isChecked() {
		x := r.Origin.X + r.Size.W - t.Space - t.IconSize/2
		if it.menu != nil && it.menu.checkLeft {
			x = r.Origin.X + t.ItemPad.Left + t.IconSize/2
		}
		paintIconAt(dst, it.env, icons.Check, ggui.Pt(x, r.Origin.Y+r.Size.H/2), t.IconSize, ink, 0)
	}
	if it.sub != nil {
		paintIconAt(dst, it.env, icons.ChevronRight, ggui.Pt(right-t.IconSize/2, r.Origin.Y+r.Size.H/2), t.IconSize, ink, 0)
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
	if ev.Kind == ggui.PointerMove && it.motion.moved(ev.Pos) && !it.IsInert() && it.onHover != nil {
		it.onHover()
	}
	if ev.Kind == ggui.PointerExit && it.menu != nil {
		it.menu.pointLeft(it)
	}
	return it.Pointer(ev, it.run)
}

// SetName names the trigger for Field.
func (m *MenuWidget) SetName(s string) { m.Name(s) }

// HasName reports whether the trigger has an explicit name.
func (m *MenuWidget) HasName() bool { return m.button.HasName() }

// BindDisabled follows r and closes the popup while disabled.
func (m *MenuWidget) BindDisabled(r ggui.Readable[bool]) *MenuWidget {
	m.button.BindDisabled(r)
	return m
}

// BindName follows the accessible name on the same targets as Name.
func (m *MenuWidget) BindName(r ggui.Readable[string]) *MenuWidget { m.button.BindName(r); return m }
