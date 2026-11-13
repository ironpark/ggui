package ui

import (
	"image/color"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// sidebarKind is what one SidebarEntry is.
type sidebarKind uint8

const (
	sidebarRowItem sidebarKind = iota
	sidebarRowSection
	sidebarRowSeparator
	sidebarRowSkeleton
	sidebarRowCustom
)

// SidebarSize is how tall a sidebar row is; see SidebarEntry.Size.
type SidebarSize uint8

// The row sizes of shadcn/ui's sidebar menu button.
const (
	SidebarSizeDefault SidebarSize = iota // four units tall, with body text
	SidebarSizeSmall                      // three and a half units, with caption text
	SidebarSizeLarge                      // six units: a workspace or account row with a logo or an avatar
)

// SidebarCollapse is what a collapsed sidebar becomes; see
// SidebarWidget.Collapsible.
type SidebarCollapse uint8

// The ways a sidebar collapses.
const (
	SidebarCollapseOffcanvas SidebarCollapse = iota // slides off its edge and gives its width back
	SidebarCollapseIcon                             // narrows to a rail of icons
	SidebarCollapseNone                             // stays as it is
)

// SidebarVariant is the surface a sidebar is drawn on; see
// SidebarWidget.Variant.
type SidebarVariant uint8

// The sidebar surfaces.
const (
	SidebarVariantDefault  SidebarVariant = iota // flush with the window, a border on its inner edge
	SidebarVariantFloating                       // a rounded, bordered, raised panel inset from the window
	SidebarVariantInset                          // borderless, beside content raised in a SidebarInset
)

// SidebarSide is the edge a sidebar stands at; see SidebarWidget.Side.
type SidebarSide uint8

// The sides of a sidebar.
const (
	SidebarSideAuto  SidebarSide = iota // where reading starts: left, or right under right-to-left
	SidebarSideLeft                     // the left edge
	SidebarSideRight                    // the right edge
)

// SidebarEntry is one line of a Sidebar: a destination, a heading over the
// destinations that follow it, a separator, a loading placeholder or a
// widget of the caller's own. Build one with SidebarItem, SidebarSection,
// SidebarSeparator, SidebarSkeleton or SidebarCustom, and dress it with the
// methods below; each returns the entry changed, so they chain.
type SidebarEntry struct {
	kind          sidebarKind
	key, label    string
	disabled      bool
	icon          icons.Role
	leading       ggui.Widget
	detail        string
	badge         string
	action        ggui.Widget
	actionOnHover bool
	sub           []SidebarEntry
	collapsible   bool
	open          bool
	openBind      ggui.Binding[bool]
	size          SidebarSize
	outline       bool
	tooltip       string
	widget        ggui.Widget
	onTap         func()
}

// SidebarItem creates a destination. The key is what the binding holds when
// this item is the chosen one; keys must be unique within a sidebar.
func SidebarItem(key, label string) SidebarEntry {
	return SidebarEntry{key: key, label: label}
}

// SidebarSection creates a heading over the items that follow it, which
// make a group with it until the next section or separator. It takes no
// input and is skipped by the arrow keys, unless it is Collapsible.
func SidebarSection(title string) SidebarEntry {
	return SidebarEntry{kind: sidebarRowSection, label: title}
}

// SidebarSeparator creates a line across the sidebar. It ends the group
// before it; the entries after it start another.
func SidebarSeparator() SidebarEntry { return SidebarEntry{kind: sidebarRowSeparator} }

// SidebarSkeleton creates a pulsing placeholder the size of an item, for a
// list that is still loading. Icon, with any role, gives it the square an
// icon will take.
func SidebarSkeleton() SidebarEntry { return SidebarEntry{kind: sidebarRowSkeleton} }

// SidebarCustom places a widget among the entries, as wide as an item: a
// card, a progress bar, a form. An icon rail has no room for it and hides
// it.
func SidebarCustom(w ggui.Widget) SidebarEntry {
	return SidebarEntry{kind: sidebarRowCustom, widget: w}
}

// Disabled shows the item greyed out and skips it.
func (e SidebarEntry) Disabled(v bool) SidebarEntry { e.disabled = v; return e }

// Icon draws the icon for role before the label. It is what an icon rail
// shows of the item.
func (e SidebarEntry) Icon(role icons.Role) SidebarEntry { e.icon = role; return e }

// Leading draws a widget before the label in place of an icon: a logo or
// an Avatar on a SidebarSizeLarge row. It gets no more height than the row
// has inside its padding.
func (e SidebarEntry) Leading(w ggui.Widget) SidebarEntry { e.leading = w; return e }

// Detail adds a second, smaller line under the label, as a
// SidebarSizeLarge row has room for: an account's address under its name.
func (e SidebarEntry) Detail(text string) SidebarEntry { e.detail = text; return e }

// Badge shows a short text at the end of the row, such as a count.
func (e SidebarEntry) Badge(text string) SidebarEntry { e.badge = text; return e }

// Action places a small widget at the end of the row, above it, which
// takes its own input: a "more" or "add" button a fifth of a row wide. On a
// section it is the group's action, beside the heading.
func (e SidebarEntry) Action(w ggui.Widget) SidebarEntry { e.action = w; return e }

// ActionOnHover shows the Action only while the pointer is over the row or
// the keyboard is on it.
func (e SidebarEntry) ActionOnHover() SidebarEntry { e.actionOnHover = true; return e }

// Sub nests items under this one, indented beside a line. With Collapsible
// they fold away under it. An icon rail hides them.
func (e SidebarEntry) Sub(entries ...SidebarEntry) SidebarEntry { e.sub = entries; return e }

// Collapsible makes the row a disclosure: a chevron at its end turns as a
// press, Space or Enter folds what is under it in and out, and Right and
// Left open and close it. On an item that is its Sub items, and the item
// is then no destination itself; on a section it is the group. It starts
// closed unless Open says otherwise or it holds the chosen item.
func (e SidebarEntry) Collapsible() SidebarEntry { e.collapsible = true; return e }

// Open is whether a Collapsible entry starts open. The sidebar keeps the
// state from then on, across a rebuild too.
func (e SidebarEntry) Open(v bool) SidebarEntry { e.open = v; return e }

// BindOpen keeps a Collapsible entry's state in open, in place of the
// sidebar's own.
func (e SidebarEntry) BindOpen(open ggui.Binding[bool]) SidebarEntry {
	property.Require(open, "BindOpen")
	e.openBind = open
	return e
}

// Size sets how tall the row is.
func (e SidebarEntry) Size(size SidebarSize) SidebarEntry { e.size = size; return e }

// Outline draws the row on the page's background with a hairline around
// it, as shadcn/ui's outline menu button.
func (e SidebarEntry) Outline() SidebarEntry { e.outline = true; return e }

// Tooltip names the item in a tip beside it while the sidebar is collapsed
// to icons, where its label does not show.
func (e SidebarEntry) Tooltip(text string) SidebarEntry { e.tooltip = text; return e }

// OnTap makes the item a button: choosing it runs fn and leaves the
// binding as it is. A workspace switcher or an account row opens its menu
// this way.
func (e SidebarEntry) OnTap(fn func()) SidebarEntry { e.onTap = fn; return e }

// SidebarWidget is a column of destinations down the side of a window, one
// of which is the current one. Build one with Sidebar.
//
// The whole column is one keyboard tab stop, as a tab strip is: Up and Down
// move the highlight over the enabled items and wrap, Home and End go to
// the ends, Space or Enter goes to the highlighted destination, and Right
// and Left open and close a collapsible row. A click goes there directly.
// The header and the footer stay where they are and the groups scroll
// between them.
//
// Collapsed takes the sidebar away as Collapsible says: off its edge, or
// down to a rail of icons whose items show a Tooltip. Rail, SidebarTrigger
// and Shortcut toggle it.
type SidebarWidget struct {
	props            property.Owner
	ggui.Interactive // hover holds the item under the pointer instead
	selected         ggui.Binding[string]
	parts            [3][]SidebarEntry // header items, the groups, footer items
	lists            [3]*sidebarList
	rows             []*sidebarNode // every entry given, in the order the arrow keys walk
	scroll           *ggui.ScrollWidget
	onChange         func(string)
	header, footer   ggui.Widget
	width            float64
	collapsed        property.Value[bool]
	collapsedBind    ggui.Binding[bool] // what Toggle writes, when BindCollapsed was given one
	toggled          bool               // Toggle moved the sidebar's own state
	configured       bool               // what Collapsed last said, which Toggle leaves alone
	mode             SidebarCollapse
	variant          SidebarVariant
	side             SidebarSide
	rail             bool
	chord            ggui.Chord
	chorded          bool
	active           int
	hover            int
	railHover        bool
	pageFg           color.Color // the page's text color, which the palette here replaces
	reveal           bool        // the highlight moved by key: bring it into view

	theme                  *uitheme.Theme
	env                    ggui.Env
	rtl                    bool
	expand                 ggui.Motion
	progress               float64 // 1 expanded, 0 collapsed
	labelProgress          float64 // progress under an icon rail, where labels go; else 1
	laidCollapsed          bool
	cur, laidCur           int // the current destination: as paint finds it, as layout styled it
	panelW, fullW          float64
	headerSize, footerSize ggui.Size
	headerH, footerH       float64
	listH                  [3]float64
	hidden                 bool
	tip                    sidebarTip
}

// Sidebar creates a navigation column bound to the key of the chosen item.
//
//	page := ggui.State("inbox")
//	ui.Sidebar(page,
//		ui.SidebarSection("Mail"),
//		ui.SidebarItem("inbox", "Inbox").Icon(icons.File).Badge("12"),
//		ui.SidebarItem("sent", "Sent"),
//	).Header(Title("Acme")).BindCollapsed(narrow)
func Sidebar(selected ggui.Binding[string], entries ...SidebarEntry) *SidebarWidget {
	s := &SidebarWidget{selected: selected, width: 256, active: -1, hover: -1, cur: -1, laidCur: -1}
	s.Role = ggui.RoleTabs
	s.AutoKey()
	if s.HitID() == nil {
		s.Key(s)
	}
	for i := range s.lists {
		s.lists[i] = &sidebarList{s: s}
	}
	s.scroll = ggui.Scroll(s.lists[1]).Bar(nil)
	s.parts[1] = entries
	s.build()
	return s
}

// build makes the rows of the three lists anew and numbers them in the
// order the keyboard walks them: header, groups, footer.
func (s *SidebarWidget) build() {
	activeKey := ""
	if s.active >= 0 && s.active < len(s.rows) {
		activeKey = s.rows[s.active].id()
	}
	chosen := ggui.Untrack(s.selected.Get)
	s.rows = s.rows[:0]
	seen := map[string]bool{}
	for i, entries := range s.parts {
		s.lists[i].top = buildSidebarNodes(entries)
		s.number(s.lists[i].top, seen, chosen)
	}
	s.active = -1
	for i, n := range s.rows {
		if activeKey != "" && n.id() == activeKey && s.enabled(i) {
			s.active = i
			break
		}
	}
	if s.active < 0 {
		s.active = stepIndex(-1, 1, len(s.rows), s.enabled)
	}
}

// number appends the entries under nodes to rows, a parent before what it
// holds, and opens every collapsible row around the chosen item. It
// reports whether the chosen item is among them.
func (s *SidebarWidget) number(nodes []*sidebarNode, seen map[string]bool, chosen string) (holds bool) {
	for _, n := range nodes {
		n.index = -1
		if !n.implicit {
			n.index = len(s.rows)
			s.rows = append(s.rows, n)
		}
		if n.e.kind == sidebarRowItem {
			if seen[n.e.key] {
				panic("ui: duplicate Sidebar key")
			}
			seen[n.e.key] = true
			holds = holds || n.e.key == chosen
		}
		if s.number(n.kids, seen, chosen) {
			n.open, holds = true, true
		}
	}
	return holds
}

// Width sets how wide the column is in logical pixels, 256 until it is set.
func (s *SidebarWidget) Width(w float64) *SidebarWidget {
	defer property.Watch(&s.props, &s.width)()
	s.width = max(0, w)
	return s
}

// Name sets the accessible name of the column.
func (s *SidebarWidget) Name(name string) *SidebarWidget { s.SetName(name); return s }

// Header places a widget above the items: a product name, a search box. It
// stays in place while the groups scroll, and an icon rail, which has no
// room for it, folds it away; HeaderItems are what a rail keeps.
func (s *SidebarWidget) Header(w ggui.Widget) *SidebarWidget {
	s.header = w
	s.props.Changed()
	return s
}

// Footer places a widget below the items, at the bottom of the column. An
// icon rail folds it away, as it does the Header.
func (s *SidebarWidget) Footer(w ggui.Widget) *SidebarWidget {
	s.footer = w
	s.props.Changed()
	return s
}

// HeaderItems places entries in the header, under the Header widget: rows
// that look and behave as the sidebar's own and stay in place while the
// groups scroll, such as a SidebarSizeLarge workspace switcher with OnTap.
func (s *SidebarWidget) HeaderItems(entries ...SidebarEntry) *SidebarWidget {
	s.parts[0] = entries
	s.build()
	s.props.Changed()
	return s
}

// FooterItems places entries in the footer, above the Footer widget, as
// HeaderItems does in the header: an account row, a settings link.
func (s *SidebarWidget) FooterItems(entries ...SidebarEntry) *SidebarWidget {
	s.parts[2] = entries
	s.build()
	s.props.Changed()
	return s
}

// Collapsible sets what the collapsed sidebar becomes:
// SidebarCollapseOffcanvas, the default, slides it off its edge;
// SidebarCollapseIcon narrows it to a rail six units wide that keeps the
// icons and hides labels, headings, badges, actions and sub-items;
// SidebarCollapseNone ignores Collapsed. Either way the width moves over
// the theme's MotionSlow at a constant speed.
func (s *SidebarWidget) Collapsible(mode SidebarCollapse) *SidebarWidget {
	defer property.Watch(&s.props, &s.mode)()
	s.mode = mode
	return s
}

// Variant sets the surface: SidebarVariantDefault, SidebarVariantFloating
// or SidebarVariantInset. An inset sidebar keeps no padding on its inner
// side, which the SidebarInset beside it supplies.
func (s *SidebarWidget) Variant(v SidebarVariant) *SidebarWidget {
	defer property.Watch(&s.props, &s.variant)()
	s.variant = v
	return s
}

// Side says which edge of the window the sidebar stands at, which is where
// its border is not, and the edge it slides off. The default follows the
// reading direction. It does not move the sidebar: place it first or last
// in its Row.
func (s *SidebarWidget) Side(side SidebarSide) *SidebarWidget {
	defer property.Watch(&s.props, &s.side)()
	s.side = side
	return s
}

// Rail adds the thin strip along the sidebar's inner edge that toggles it
// on a click and shows a line under the pointer. A sidebar slid off its
// edge has none; a SidebarTrigger or Shortcut brings that back.
func (s *SidebarWidget) Rail(v bool) *SidebarWidget {
	defer property.Watch(&s.props, &s.rail)()
	s.rail = v
	return s
}

// Shortcut toggles the sidebar on chord while it is in the tree, collapsed
// or not; shadcn/ui's is "cmd+b", which is Ctrl+B off macOS. See
// ggui.Canvas.Shortcut for when it runs.
func (s *SidebarWidget) Shortcut(chord string) *SidebarWidget {
	s.chord, s.chorded = ggui.MustChord(chord), true
	return s
}

// BindCollapsed follows r: while it is true the sidebar is collapsed, as
// Collapsible says. Given a binding, such as a State, Rail, Shortcut and
// Toggle write it; a SidebarTrigger on the same binding then agrees with
// them.
func (s *SidebarWidget) BindCollapsed(r ggui.Readable[bool]) *SidebarWidget {
	if s.collapsed.Bind(r, "BindCollapsed") {
		s.props.Changed()
	}
	s.collapsedBind, _ = r.(ggui.Binding[bool])
	return s
}

// Collapsed sets a fixed collapsed state and detaches a reader.
func (s *SidebarWidget) Collapsed(v bool) *SidebarWidget {
	s.collapsedBind, s.toggled, s.configured = nil, false, v
	if s.collapsed.Set(v) {
		s.props.Changed()
	}
	return s
}

// Toggle collapses an expanded sidebar and expands a collapsed one, as its
// Rail and Shortcut do: through the binding BindCollapsed was given, or in
// the sidebar's own state. A sidebar following a reader it cannot write
// stays as it is.
func (s *SidebarWidget) Toggle() {
	next := !ggui.Untrack(s.collapsed.Get)
	switch {
	case s.collapsedBind != nil:
		s.collapsedBind.Set(next)
	case !s.collapsed.Bound():
		if s.collapsed.Set(next) {
			s.toggled = true
			s.props.Changed()
		}
	}
}

// Disabled greys the column out and ignores input while v is true.
func (s *SidebarWidget) Disabled(v bool) *SidebarWidget { s.SetInert(v); return s }

// OnChange fires with the key after the user went to another destination.
func (s *SidebarWidget) OnChange(fn func(string)) *SidebarWidget { s.onChange = fn; return s }

// railed reports whether the sidebar is, or is on its way to being, a rail
// of icons.
func (s *SidebarWidget) railed() bool {
	return s.mode == SidebarCollapseIcon && ggui.Untrack(s.collapsed.Get)
}

// enabled reports whether the arrow keys and the pointer may land on row i:
// an item or a collapsible heading that is not disabled and is showing.
func (s *SidebarWidget) enabled(i int) bool {
	n := s.rows[i]
	switch {
	case n.e.disabled:
		return false
	case n.e.kind == sidebarRowSection:
		if !n.e.collapsible || s.railed() {
			return false
		}
	case n.e.kind != sidebarRowItem:
		return false
	}
	for p := n.parent; p != nil; p = p.parent {
		if !ggui.Untrack(p.isOpen) || (p.e.kind == sidebarRowItem && s.railed()) {
			return false
		}
	}
	return true
}

// activate does what choosing row i does: folds a collapsible row, runs an
// OnTap, or goes to the destination.
func (s *SidebarWidget) activate(i int) {
	if i < 0 || i >= len(s.rows) || !s.enabled(i) || s.IsInert() {
		return
	}
	s.active = i
	n := s.rows[i]
	switch {
	case n.e.collapsible:
		s.setOpen(n, !ggui.Untrack(n.isOpen))
	case n.e.onTap != nil:
		n.e.onTap()
	default:
		setChanged(s.selected, n.e.key, s.onChange)
	}
}

// setOpen folds a collapsible row open or closed.
func (s *SidebarWidget) setOpen(n *sidebarNode, open bool) {
	if !n.e.collapsible || ggui.Untrack(n.isOpen) == open {
		return
	}
	if n.e.openBind != nil {
		n.e.openBind.Set(open)
		return
	}
	n.open = open
	s.props.Changed()
}

// current returns the row the binding names, or -1.
func (s *SidebarWidget) current() int {
	key := ggui.Untrack(s.selected.Get)
	for i, n := range s.rows {
		if n.e.kind == sidebarRowItem && n.e.key == key {
			return i
		}
	}
	return -1
}

// onRight reports whether the sidebar stands at the right edge.
func (s *SidebarWidget) onRight() bool {
	return s.side == SidebarSideRight || (s.side == SidebarSideAuto && s.rtl)
}

// chrome is how much of the column's width the variant keeps around the
// panel: none, a unit on both sides of a floating one, a unit on the outer
// side of an inset one.
func (s *SidebarWidget) chrome(t *uitheme.Theme) float64 {
	switch s.variant {
	case SidebarVariantFloating:
		return 2 * t.Space
	case SidebarVariantInset:
		return t.Space
	}
	return 0
}

// Layout implements ggui.Widget.
func (s *SidebarWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer s.props.Layout().EndLayout()
	s.Sync()
	// Children see the sidebar palette in place of the page's, so a button
	// or badge inside an item paints against the sidebar surface, and the
	// tokens that follow these follow them here too.
	t := uitheme.From(env)
	s.pageFg = t.Fg // what a Tooltip is filled with, on the page and here
	t.Card, t.Fg = t.Sidebar, t.SidebarFg
	t.Primary, t.PrimaryFg = t.SidebarPrimary, t.SidebarPrimaryFg
	t.Accent, t.AccentFg = t.SidebarAccent, t.SidebarAccentFg
	t.Border, t.Ring = t.SidebarBorder, t.SidebarRing
	env = t.Apply(env)
	s.theme = uitheme.Of(env) // t, resolved, shared with the children
	s.env = env
	s.rtl = env.Direction() == ggui.DirRTL
	th := s.theme
	collapsed := s.collapsed.Get() && s.mode != SidebarCollapseNone
	s.laidCollapsed = collapsed
	s.progress = s.expand.Easing(ggui.EaseLinear).LayoutToggle(env, !collapsed, th.MotionSlow)
	s.labelProgress = pick(s.mode == SidebarCollapseIcon, s.progress, 1)
	s.hidden = collapsed && s.mode == SidebarCollapseOffcanvas && s.progress <= 0
	if s.hidden {
		return c.Constrain(ggui.Size{})
	}
	// Read under the layout, so a destination chosen elsewhere lays the
	// column out again: the current row's label is a weight heavier.
	s.selected.Get()
	s.laidCur = s.current()

	pad, chrome := th.Space, s.chrome(th)
	full := min(s.width, c.MaxW)
	s.fullW = max(full-chrome, 0)
	s.panelW = s.fullW
	outer := full * s.progress
	if s.mode == SidebarCollapseIcon {
		rail := min(sidebarRailUnits*th.Space, s.fullW)
		s.panelW = rail + (s.fullW-rail)*s.progress
		outer = min(s.panelW+chrome, full)
	}
	// The header and the footer are laid out for the full width and fold
	// away under a rail, as a group's heading does.
	inner := ggui.Constraints{MaxW: max(s.fullW-2*pad, 0), MaxH: ggui.Unbounded}
	list := ggui.Constraints{MinW: s.panelW, MaxW: s.panelW, MaxH: ggui.Unbounded}
	fixed := 0.0
	if s.variant != SidebarVariantDefault {
		fixed = 2 * pad
	}
	s.headerSize, s.footerSize, s.headerH, s.footerH = ggui.Size{}, ggui.Size{}, 0, 0
	s.listH[0] = s.lists[0].Layout(list, env).H
	s.listH[2] = s.lists[2].Layout(list, env).H
	// A list pads itself; where there is none the widget's own padding
	// stands on that side of it.
	if s.header != nil {
		s.headerSize = s.header.Layout(inner, env)
		s.headerH = (pad + s.headerSize.H + pick(s.listH[0] > 0, 0, pad)) * s.labelProgress
	}
	if s.footer != nil {
		s.footerSize = s.footer.Layout(inner, env)
		s.footerH = (pick(s.listH[2] > 0, 0, pad) + s.footerSize.H + pad) * s.labelProgress
	}
	fixed += s.headerH + s.listH[0] + s.listH[2] + s.footerH
	if c.MaxH < ggui.Unbounded {
		list.MaxH = max(c.MaxH-fixed, 0)
	}
	s.listH[1] = s.scroll.Layout(list, env).H
	return c.Constrain(ggui.Sz(outer, bounded(c.MaxH, fixed+s.lists[1].height)))
}

// Paint implements ggui.Widget. The column is one node holding a
// destination each, which the hit regions alone could not say.
func (s *SidebarWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if s.chorded && !s.IsInert() && s.mode != SidebarCollapseNone {
		dst.Shortcut(s.chord, s.Toggle)
	}
	if s.hidden {
		return
	}
	dst.DescribeNode(r, s, func(dst *ggui.Canvas) { s.paint(dst, r) })
}

func (s *SidebarWidget) paint(dst *ggui.Canvas, r ggui.Rect) {
	t := s.theme
	pad, right := t.Space, s.onRight()
	root, outer := dst, r
	// A sidebar going off its edge keeps its width and slides under the
	// column's clip, taking no input on the way out: from the frame it is
	// collapsed in, when the slide has yet to move it.
	if s.mode != SidebarCollapseIcon && (s.progress < 1 || s.laidCollapsed) {
		w := s.fullW + s.chrome(t)
		outer = ggui.Rct(ggui.Pt(pick(right, r.Origin.X, r.Origin.X+r.Size.W-w), r.Origin.Y), ggui.Sz(w, r.Size.H))
		dst = dst.Clip(r)
		if s.laidCollapsed {
			dst = dst.Inert()
		}
	}
	panel := outer
	switch s.variant {
	case SidebarVariantDefault:
		dst.FillRect(panel, t.Card)
		edge := pick(right, panel.Origin.X, panel.Origin.X+panel.Size.W-t.BorderWidth)
		dst.FillRect(ggui.Rct(ggui.Pt(edge, panel.Origin.Y), ggui.Sz(t.BorderWidth, panel.Size.H)), t.Border)
	case SidebarVariantFloating:
		panel = ggui.Rct(outer.Origin.Add(ggui.Pt(pad, pad)), ggui.Sz(max(outer.Size.W-2*pad, 0), max(outer.Size.H-2*pad, 0)))
		dst.Shadow(panel, t.Radius, t.CardShadow)
		dst.FillRoundRect(panel, t.Radius, t.Card)
		dst.StrokeRoundRect(panel, t.Radius, t.BorderWidth, t.Border)
	case SidebarVariantInset:
		dst.FillRect(outer, t.Card)
		panel = ggui.Rct(outer.Origin.Add(ggui.Pt(pick(right, 0, pad), pad)), ggui.Sz(max(outer.Size.W-pad, 0), max(outer.Size.H-2*pad, 0)))
	}
	dst = dst.Clip(panel)
	if !s.IsInert() && s.active >= 0 {
		dst.HitKey(panel, s)
	}
	// Layout is skipped on a still frame, so the current destination is
	// found here, where the items' own nodes read it back. One the layout
	// did not style is laid out again.
	s.cur = s.current()
	if s.cur != s.laidCur {
		ggui.Invalidate(s.env)
	}
	x, w := panel.Origin.X, panel.Size.W
	y := panel.Origin.Y
	full := max(s.fullW-2*pad, 0)
	lead := func(top float64, size ggui.Size) ggui.Rect {
		return ggui.Rct(ggui.Pt(pick(s.rtl, x+w-pad-full, x+pad), top), ggui.Sz(full, size.H))
	}
	if s.header != nil && s.headerH > 0 {
		s.folding(dst, ggui.Rct(ggui.Pt(x, y), ggui.Sz(w, s.headerH))).Paint(s.header, lead(y+pad*s.labelProgress, s.headerSize))
		y += s.headerH
	}
	if s.listH[0] > 0 {
		dst.Paint(s.lists[0], ggui.Rct(ggui.Pt(x, y), ggui.Sz(w, s.listH[0])))
		y += s.listH[0]
	}
	bottom := max(panel.Origin.Y+panel.Size.H-s.footerH-s.listH[2], y)
	if bottom > y {
		dst.Paint(s.scroll, ggui.Rct(ggui.Pt(x, y), ggui.Sz(w, bottom-y)))
	}
	if s.listH[2] > 0 {
		dst.Paint(s.lists[2], ggui.Rct(ggui.Pt(x, bottom), ggui.Sz(w, s.listH[2])))
	}
	if s.footer != nil && s.footerH > 0 {
		top := bottom + s.listH[2]
		s.folding(dst, ggui.Rct(ggui.Pt(x, top), ggui.Sz(w, s.footerH))).Paint(s.footer, lead(top+s.footerH-(pad+s.footerSize.H)*s.labelProgress, s.footerSize))
	}
	s.reveal = false
	if s.rail && s.mode != SidebarCollapseNone {
		s.paintRail(root, r, right)
	}
	s.paintTip(root, right)
}

// folding returns the canvas the header or the footer paints on: dst, or
// dst clipped to r and inert while a rail folds it away.
func (s *SidebarWidget) folding(dst *ggui.Canvas, r ggui.Rect) *ggui.Canvas {
	if s.labelProgress >= 1 {
		return dst
	}
	return dst.Clip(r).Inert()
}

// HandleKey implements ggui.KeyHandler: Up and Down move the highlight,
// Space and Enter go where it is, and Right and Left open and close a
// collapsible row, Left on a sub-item going up to the row over it.
func (s *SidebarWidget) HandleKey(ev ggui.KeyEvent) {
	s.Keyboard(ev, func() { s.activate(s.active) })
	if ev.Kind != ggui.KeyPress || len(s.rows) == 0 {
		return
	}
	if i, ok := navIndex(ev, navVertical, s.active, len(s.rows), s.enabled); ok {
		s.active, s.reveal = i, true
		return
	}
	if s.active < 0 || s.active >= len(s.rows) {
		return
	}
	n := s.rows[s.active]
	switch s.foldKey(ev.Key) {
	case 1:
		s.setOpen(n, true)
	case -1:
		if n.e.collapsible && ggui.Untrack(n.isOpen) {
			s.setOpen(n, false)
		} else if p := n.parent; p != nil && p.index >= 0 && s.enabled(p.index) {
			s.active, s.reveal = p.index, true
		}
	}
}

// foldKey is 1 for the arrow that opens a row, pointing the way reading
// goes, -1 for the one that closes it, and 0 for any other key.
func (s *SidebarWidget) foldKey(key ggui.KeyboardKey) int {
	switch key {
	case ggui.KeyArrowRight:
		return pick(s.rtl, -1, 1)
	case ggui.KeyArrowLeft:
		return pick(s.rtl, 1, -1)
	}
	return 0
}

// ConsumesKey implements ggui.KeyConsumer.
func (s *SidebarWidget) ConsumesKey(ev ggui.KeyEvent) bool {
	if navMoveOf(ev.Key, navVertical) != navNone {
		return ev.Kind == ggui.KeyPress
	}
	if s.foldKey(ev.Key) != 0 && s.active >= 0 && s.active < len(s.rows) {
		n := s.rows[s.active]
		return ev.Kind == ggui.KeyPress && (n.e.collapsible || (n.parent != nil && n.parent.index >= 0))
	}
	return s.Interactive.ConsumesKey(ev)
}

// Adopt implements ggui.Adopter: the highlight and the hover follow the
// item they were on, by key, across a rebuild, a row folded open or shut
// stays so, and a sidebar Toggle collapsed stays collapsed.
func (s *SidebarWidget) Adopt(prev any) {
	s.Interactive.Adopt(prev)
	p, ok := prev.(*SidebarWidget)
	if !ok || p == s {
		return
	}
	byID := make(map[string]int, len(s.rows))
	for i, n := range s.rows {
		byID[n.id()] = i
	}
	find := func(i int) int {
		if i < 0 || i >= len(p.rows) {
			return -1
		}
		if j, ok := byID[p.rows[i].id()]; ok {
			return j
		}
		return -1
	}
	if i := find(p.active); i >= 0 && s.enabled(i) {
		s.active = i
	}
	s.hover = find(p.hover)
	s.railHover, s.tip = p.railHover, p.tip
	relaid := false
	for _, old := range p.rows {
		if j, ok := byID[old.id()]; ok && old.e.collapsible {
			n := s.rows[j]
			relaid = relaid || (n.e.openBind == nil && n.open != old.open)
			n.open, n.reveal = old.open, old.reveal
		}
	}
	// The collapse carries on from where it was, and a sidebar toggled by
	// hand keeps that over the same configured state.
	s.expand = p.expand
	relaid = relaid || p.laidCollapsed != s.laidCollapsed
	if p.toggled && !s.collapsed.Bound() && !p.collapsed.Bound() && s.configured == p.configured {
		s.toggled = true
		relaid = s.collapsed.Set(p.collapsed.Current()) || relaid
	}
	if relaid {
		s.props.Changed()
	}
}

// BindDisabled follows r for Disabled without rebuilding the control.
func (s *SidebarWidget) BindDisabled(r ggui.Readable[bool]) *SidebarWidget { s.BindInert(r); return s }

func (s *SidebarWidget) name() string {
	return pick(s.SemanticName() != "", s.SemanticName(), "Sidebar")
}

// Semantics implements ggui.Semantic, including the built-in fallback name.
func (s *SidebarWidget) Semantics() (ggui.Role, string) { return s.Role, s.name() }
