package ui

import (
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/property"
	"github.com/ironpark/ggui/ui/internal/controlkit"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
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
	popup    *widget.PopupWidget
	panel    *widget.BoxWidget
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
	search  typeahead // the search by typing under way
}

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
	m.panel = widget.Box(m.body)
	return m
}

// Name names a MenuOf for Probe.Find and the inspector.
func (m *MenuWidget) Name(s string) *MenuWidget { m.button.SetName(s); return m }

// Disabled disables this menu's trigger and closes it.
func (m *MenuWidget) Disabled(v bool) *MenuWidget {
	m.button.Disabled(v)
	if v {
		m.hide()
	}
	return m
}

// hide closes any open submenus and hides m's popup.
func (m *MenuWidget) hide() {
	m.closeSub()
	if m.popup != nil {
		m.popup.Hide()
	}
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
	m.popup = widget.Popup(m.button, menuSurface(m.panel)).Keys(m).Owner(m.button)
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
func (m *MenuWidget) Popup() *widget.PopupWidget { return m.popup }

func (m *MenuWidget) toggle() {
	if m.popup.IsOpen() {
		m.hide()
		return
	}
	m.closeSub()
	m.current, m.hover = -1, -1
	m.search.clear()
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
		// An assistive technology's open is not a pointer's: like Base
		// UI's virtual click, and like the keyboard, it starts the menu
		// on its first row.
		m.closeSub()
		m.current, m.hover = -1, -1
		m.search.clear()
		m.popup.Show()
		m.jumpFirst()
	case ggui.ActionCollapse:
		m.hide()
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
		m.hide()
	}
	t := uitheme.Of(env)
	m.theme = t
	m.chrome(t)
	return m.popup.Layout(c, env)
}

// Paint implements Widget.
func (m *MenuWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if !m.popup.IsOpen() && m.sub != nil {
		m.closeSub()
	}
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
		m.hide()
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
			m.hide()
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
		m.keyActivate(ev)
	default:
		m.button.HandleKey(ev)
	}
}

// keyActivate runs the highlighted row of an open menu, or opens a closed
// one on its first row.
func (m *MenuWidget) keyActivate(ev ggui.KeyEvent) {
	if m.popup.IsOpen() {
		// With no row highlighted there is nothing to run, and the
		// menu stays as it is.
		m.key(ev)
		return
	}
	m.button.HandleKey(ev)
	if m.popup.IsOpen() {
		// Opened from the keyboard, the menu starts on its first row.
		m.jumpFirst()
	}
}

// chrome dresses the panel. A menubar and a context menu paint the panel
// through their own popup, so the width lives here rather than in Layout.
func (m *MenuWidget) chrome(t *uitheme.Theme) *widget.BoxWidget {
	m.theme = t
	t = t.Menu()
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

// jumpFirst puts the highlight on the first enabled item from the top.
func (m *MenuWidget) jumpFirst() {
	m.current, m.pointed = controlkit.StepIndex(-1, 1, len(m.items), m.enabled), false
}

// reopen refreshes the items and, when first is set, highlights the first
// one: a menu opened from the keyboard starts there, and one the pointer
// opened shows no highlight until the pointer or a key gives it one.
// Owners that show the panel through their own popup call it in place of
// toggle.
func (m *MenuWidget) reopen(first bool) {
	m.closeSub()
	m.hover, m.current = -1, -1
	m.search.clear()
	for _, it := range m.items {
		it.Sync()
	}
	if first {
		m.jumpFirst()
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
	return m.search.active(ggui.FrameTime(), menuTypeaheadReset)
}

// typeahead gives typed text to the innermost open menu under m, which
// moves its highlight to the next enabled row whose label starts with what
// has been typed since the last pause, searching on from the row the
// highlight was on when the typing began. The same letter again moves on
// to the next row that starts with it.
func (m *MenuWidget) typeahead(text string) {
	leaf := m.leaf()
	i := leaf.search.search(text, ggui.FrameTime(), menuTypeaheadReset, max(leaf.current, -1),
		typeaheadRows{n: len(leaf.items), label: func(i int) string { return leaf.items[i].label }, available: leaf.enabled})
	if i < 0 {
		return
	}
	leaf.current, leaf.hover, leaf.pointed = i, -1, false
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
	text  *widget.TextWidget
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
			BindChecked(equalsReader[T]{r: value, v: option})
		row.stay = true
		g.rows = append(g.rows, row)
		g.entries = append(g.entries, row)
	}
	g.column = newMenuColumn(g.entries)
	return g
}

// Format sets how each option is shown.
func (g *MenuRadioGroupWidget[T]) Format(fn func(T) string) *MenuRadioGroupWidget[T] {
	if fn == nil {
		fn = sprint[T]
	}
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
