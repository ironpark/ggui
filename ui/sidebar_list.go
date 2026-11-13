package ui

import (
	"image/color"
	"math"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// sidebarRailUnits is the width of an icon rail in units of the theme's
// Space: shadcn/ui's 3rem.
const sidebarRailUnits = 6

// sidebarMetrics are the sizes of a sidebar's rows, shadcn/ui's in units of
// the theme's Space.
type sidebarMetrics struct {
	pad       float64 // around a group and inside a row: p-2
	row       float64 // h-8
	rowSm     float64 // h-7
	rowLg     float64 // h-12
	icon      float64 // size-4
	gap       float64 // between an icon and its label: gap-2
	edge      float64 // from a row's end to its badge or action: right-1
	badge     float64 // the side of a badge or an action: size-5
	subIndent float64 // from a row's start to the line beside its sub-items: mx-3.5
	subPad    float64 // from that line to the sub-items: px-2.5
	subGap    float64 // between sub-items: gap-1
	subY      float64 // above and below them: py-0.5
	line      float64
}

func sidebarMetricsOf(t *uitheme.Theme) sidebarMetrics {
	u := t.Space
	return sidebarMetrics{
		pad: u, row: 4 * u, rowSm: 3.5 * u, rowLg: 6 * u, icon: t.IconSize, gap: u, edge: u / 2, badge: 2.5 * u,
		subIndent: 1.75 * u, subPad: 1.25 * u, subGap: u / 2, subY: u / 4, line: t.BorderWidth,
	}
}

// sidebarNode is one entry as the sidebar keeps it: what was asked for,
// the text it draws, whether it is folded open, and where the last layout
// put its parts.
type sidebarNode struct {
	e        SidebarEntry
	implicit bool // the group around entries no section heads
	index    int  // in SidebarWidget.rows; -1 for an implicit group
	parent   *sidebarNode
	depth    int // 0 in a group, 1 for a sub-item, and so on
	kids     []*sidebarNode

	label, detail, badge *ggui.TextWidget
	bones                [2]*SkeletonWidget // a skeleton's icon and its line
	open                 bool               // a collapsible row's own state
	reveal               ggui.Motion

	openProgress float64 // how far the fold is open
	progress     float64 // how much of what is under the row shows: the fold, and the rail
	rowH, fullH  float64 // the row's height now, and off a rail
	kidsH        float64 // of everything under the row, all of it showing

	labelSize, detailSize, badgeSize ggui.Size
	leadSize, actionSize, widgetSize ggui.Size
	// From the row's leading edge.
	labelX, badgeX, badgeW, chevronX, actionX float64
	rect                                      ggui.Rect // where the row was last painted
}

func newSidebarNode(e SidebarEntry, parent *sidebarNode, depth int) *sidebarNode {
	n := &sidebarNode{e: e, parent: parent, depth: depth, index: -1, open: e.open}
	switch e.kind {
	case sidebarRowItem:
		n.label = ggui.Text(e.label).NoWrap().Ellipsis()
		if e.detail != "" {
			n.detail = ggui.Text(e.detail).NoWrap().Ellipsis()
		}
		if e.badge != "" {
			n.badge = ggui.Text(e.badge).NoWrap()
		}
		for _, sub := range e.sub {
			if sub.kind != sidebarRowSection && sub.kind != sidebarRowSeparator {
				n.kids = append(n.kids, newSidebarNode(sub, n, depth+1))
			}
		}
	case sidebarRowSection:
		n.label = ggui.Text(e.label).NoWrap().Ellipsis()
	case sidebarRowSkeleton:
		n.bones = [2]*SkeletonWidget{Skeleton(0, 0), Skeleton(0, 0)}
	}
	return n
}

// buildSidebarNodes groups entries as they read: a section heads the items
// after it, a separator ends the group, and items no section heads make a
// group without a heading.
func buildSidebarNodes(entries []SidebarEntry) []*sidebarNode {
	var top []*sidebarNode
	var group *sidebarNode
	for _, e := range entries {
		switch e.kind {
		case sidebarRowSection:
			group = newSidebarNode(e, nil, 0)
			top = append(top, group)
		case sidebarRowSeparator:
			top = append(top, newSidebarNode(e, nil, 0))
			group = nil
		default:
			if group == nil {
				group = &sidebarNode{e: SidebarEntry{kind: sidebarRowSection}, implicit: true, index: -1}
				top = append(top, group)
			}
			group.kids = append(group.kids, newSidebarNode(e, group, 0))
		}
	}
	return top
}

// id names the entry across a rebuild: an item by its key, a section by
// its title. The rest have none.
func (n *sidebarNode) id() string {
	switch n.e.kind {
	case sidebarRowItem:
		return "item:" + n.e.key
	case sidebarRowSection:
		return "section:" + n.e.label
	}
	return ""
}

// isOpen reports whether what is under the row shows: always, unless the
// row is collapsible.
func (n *sidebarNode) isOpen() bool {
	switch {
	case !n.e.collapsible:
		return true
	case n.e.openBind != nil:
		return n.e.openBind.Get()
	}
	return n.open
}

// trail places what stands at the end of a row w wide, from the edge
// inward: the action, the chevron, the badge. It returns where the label
// has to end.
func (n *sidebarNode) trail(w float64, env ggui.Env, m sidebarMetrics) float64 {
	limit, edge, trailing := w-m.pad, w-m.edge, false
	if n.e.action != nil {
		n.actionSize = n.e.action.Layout(ggui.Loose(ggui.Sz(m.badge, n.fullH)), env)
		n.actionX = edge - n.actionSize.W
		edge, trailing = n.actionX-m.edge, true
	}
	if n.e.collapsible {
		n.chevronX = min(edge, limit) - m.icon
		edge, trailing = n.chevronX-m.edge, true
	}
	if n.badge != nil {
		n.badgeSize = n.badge.Layout(ggui.Loose(ggui.Sz(w, m.badge)), env)
		n.badgeW = max(n.badgeSize.W+m.pad, m.badge)
		n.badgeX = edge - n.badgeW
		edge, trailing = n.badgeX-m.edge, true
	}
	if trailing {
		limit = min(limit, edge-m.edge)
	}
	return limit
}

// sidebarList is a run of a sidebar's groups: the ones that scroll, or the
// header's or the footer's.
type sidebarList struct {
	s      *SidebarWidget
	top    []*sidebarNode // groups and the separators between them
	height float64
}

// Layout implements ggui.Widget. Rows are laid out for the sidebar's full
// width whatever it has just now, so a sidebar narrowing to a rail shapes
// no text on the way: Paint clips them.
func (l *sidebarList) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	s := l.s
	m := sidebarMetricsOf(s.theme)
	w := max(s.fullW-2*m.pad, 0)
	l.height = 0
	for _, n := range l.top {
		l.height += s.layoutNode(n, w, env, m)
	}
	return c.Constrain(ggui.Sz(bounded(c.MaxW, s.panelW), l.height))
}

// Paint implements ggui.Widget.
func (l *sidebarList) Paint(dst *ggui.Canvas, r ggui.Rect) {
	s := l.s
	m := sidebarMetricsOf(s.theme)
	x, w := r.Origin.X+m.pad, max(r.Size.W-2*m.pad, 0)
	// One clip down the column cuts what every row holds while the rows
	// are narrower than they were laid out for.
	face := dst
	if w < s.fullW-2*m.pad-.5 {
		face = dst.Clip(ggui.Rct(ggui.Pt(x, r.Origin.Y), ggui.Sz(w, r.Size.H)))
	}
	y := r.Origin.Y
	for _, n := range l.top {
		y += s.paintNode(dst, face, n, x, y, w, m)
	}
}

// weights are the label weights of a row at rest and of the current one.
func sidebarWeights(t *uitheme.Theme) (regular, medium ggui.FontWeight) {
	regular, medium = t.Text.Weight, t.Label.Weight
	if regular == 0 {
		regular = ggui.WeightRegular
	}
	if medium == 0 {
		medium = ggui.WeightMedium
	}
	return regular, medium
}

// layoutNode lays out n and what is under it for a row w wide and returns
// the height it takes.
func (s *SidebarWidget) layoutNode(n *sidebarNode, w float64, env ggui.Env, m sidebarMetrics) float64 {
	t := s.theme
	switch n.e.kind {
	case sidebarRowSeparator:
		n.rowH = t.BorderWidth
		return n.rowH
	case sidebarRowSection:
		return s.layoutGroup(n, w, env, m)
	case sidebarRowSkeleton:
		n.rowH = pick(n.depth > 0, m.rowSm, m.row)
		lead := 0.0
		if n.e.icon != "" {
			n.bones[0].Layout(ggui.Tight(ggui.Sz(m.icon, m.icon)), env)
			lead = m.icon + m.gap
		}
		// shadcn/ui draws a width between half and nine tenths at random;
		// here the row's place picks it, so it holds still between frames.
		share := .5 + float64((max(n.index, 0)*37)%41)/100
		n.widgetSize = ggui.Sz(max(w-2*m.pad-lead, 0)*share, m.icon)
		n.bones[1].Layout(ggui.Tight(n.widgetSize), env)
		return n.rowH
	case sidebarRowCustom:
		n.widgetSize = n.e.widget.Layout(ggui.Constraints{MinW: w, MaxW: w, MaxH: ggui.Unbounded}, env)
		n.rowH = n.widgetSize.H * s.labelProgress
		return n.rowH
	}
	return s.layoutItem(n, w, env, m)
}

// layoutGroup lays out a group: its heading, which an icon rail folds
// away, and its rows, which a collapsible heading does.
func (s *SidebarWidget) layoutGroup(n *sidebarNode, w float64, env ggui.Env, m sidebarMetrics) float64 {
	t := s.theme
	n.rowH, n.fullH = 0, m.row
	if !n.implicit && n.e.label != "" {
		_, medium := sidebarWeights(t)
		st := t.Caption
		st.Color, st.Weight = nil, medium
		n.label.Style(st)
		limit := n.trail(w, env, m)
		n.labelSize = n.label.Layout(ggui.Loose(ggui.Sz(max(limit-m.pad, 0), m.row)), env)
		n.rowH = m.row * s.labelProgress
	}
	n.openProgress = 1
	if n.e.collapsible {
		n.openProgress = n.reveal.LayoutToggle(env, n.isOpen(), t.MotionSlow)
	}
	n.progress, n.kidsH = n.openProgress, 0
	if n.progress > 0 {
		for _, k := range n.kids {
			n.kidsH += s.layoutNode(k, w, env, m)
		}
	}
	return 2*m.pad + n.rowH + n.kidsH*n.progress
}

// layoutItem lays out a destination and the sub-items under it.
func (s *SidebarWidget) layoutItem(n *sidebarNode, w float64, env ggui.Env, m sidebarMetrics) float64 {
	t := s.theme
	small := n.e.size == SidebarSizeSmall
	n.fullH = m.row
	switch {
	case n.depth > 0, small:
		n.fullH = m.rowSm
	case n.e.size == SidebarSizeLarge:
		n.fullH = m.rowLg
	}
	// A large row is no taller than the rest on a rail.
	n.rowH = min(n.fullH, m.row) + max(n.fullH-m.row, 0)*s.labelProgress
	regular, medium := sidebarWeights(t)
	caption := ggui.TextStyle{Size: t.Caption.Size, Weight: regular}
	st := ggui.TextStyle{Weight: pick((n.index == s.laidCur && n.depth == 0) || n.detail != nil, medium, regular)}
	if small {
		st.Size = t.Caption.Size
	}
	n.label.Style(st)
	lead := 0.0
	n.leadSize = ggui.Size{}
	switch {
	case n.e.leading != nil:
		// Inside the row's padding, which is what a rail leaves a large
		// row: a 40px Avatar comes out at 32.
		n.leadSize = n.e.leading.Layout(ggui.Loose(ggui.Sz(w, max(n.fullH-2*m.pad, 0))), env)
		lead = n.leadSize.W + m.gap
	case n.e.icon != "":
		n.leadSize = ggui.Sz(m.icon, m.icon)
		lead = m.icon + m.gap
	}
	if n.badge != nil {
		n.badge.Style(ggui.TextStyle{Size: t.Caption.Size, Weight: medium})
	}
	limit := n.trail(w, env, m)
	n.labelX = m.pad + lead
	box := ggui.Loose(ggui.Sz(max(limit-n.labelX, 0), n.fullH))
	n.labelSize = n.label.Layout(box, env)
	if n.detail != nil {
		n.detail.Style(caption)
		n.detailSize = n.detail.Layout(box, env)
	}
	n.openProgress, n.progress, n.kidsH = 1, 0, 0
	if len(n.kids) == 0 {
		return n.rowH
	}
	if n.e.collapsible {
		n.openProgress = n.reveal.LayoutToggle(env, n.isOpen(), t.MotionSlow)
	}
	n.progress = n.openProgress * s.labelProgress
	if n.progress > 0 {
		sub := max(w-2*m.subIndent-m.line-2*m.subPad, 0)
		n.kidsH = 2 * m.subY
		for i, k := range n.kids {
			if i > 0 {
				n.kidsH += m.subGap
			}
			n.kidsH += s.layoutNode(k, sub, env, m)
		}
	}
	return n.rowH + n.kidsH*n.progress
}

// place returns the rect of size at dx from the row's leading edge and dy
// from its top: from the left, or from the right under right-to-left.
func (s *SidebarWidget) place(row ggui.Rect, dx, dy float64, size ggui.Size) ggui.Rect {
	x := row.Origin.X + dx
	if s.rtl {
		x = row.Origin.X + row.Size.W - dx - size.W
	}
	return ggui.Rct(ggui.Pt(x, row.Origin.Y+dy), size)
}

// fold returns the canvases to paint what is under a row into: dst and
// face themselves when all of it shows, else clipped to block, and inert
// when it is folding away.
func (s *SidebarWidget) fold(dst, face *ggui.Canvas, n *sidebarNode, block ggui.Rect) (*ggui.Canvas, *ggui.Canvas) {
	if n.progress >= 1 {
		return dst, face
	}
	leaving := !ggui.Untrack(n.isOpen) || (n.e.kind == sidebarRowItem && s.railed())
	clip := func(c *ggui.Canvas) *ggui.Canvas {
		c = c.Clip(block)
		if leaving {
			c = c.Inert()
		}
		return c
	}
	if face == dst {
		c := clip(dst)
		return c, c
	}
	return clip(dst), clip(face)
}

// paintNode paints n at (x, y) in a column w wide and returns the height
// it took. Rows go on dst; what they hold goes on face, which is clipped
// to the column while the sidebar is narrower than its rows were laid out.
func (s *SidebarWidget) paintNode(dst, face *ggui.Canvas, n *sidebarNode, x, y, w float64, m sidebarMetrics) float64 {
	t := s.theme
	row := ggui.Rct(ggui.Pt(x, y), ggui.Sz(w, n.rowH))
	switch n.e.kind {
	case sidebarRowSeparator:
		dst.FillRect(row, t.Border)
		return n.rowH
	case sidebarRowSection:
		return s.paintGroup(dst, face, n, x, y, w, m)
	case sidebarRowSkeleton:
		at := m.pad
		if n.e.icon != "" {
			face.Paint(n.bones[0], s.place(row, at, (n.rowH-m.icon)/2, ggui.Sz(m.icon, m.icon)))
			at += m.icon + m.gap
		}
		face.Paint(n.bones[1], s.place(row, at, (n.rowH-n.widgetSize.H)/2, n.widgetSize))
		return n.rowH
	case sidebarRowCustom:
		if n.rowH <= 0 {
			return 0
		}
		c := face
		if n.rowH < n.widgetSize.H {
			c = face.Clip(row).Inert()
		}
		c.Paint(n.e.widget, s.place(row, 0, 0, n.widgetSize))
		return n.rowH
	}
	s.paintItem(dst, face, n, row, m)
	if n.progress <= 0 || len(n.kids) == 0 {
		return n.rowH
	}
	// The sub-items, beside a line down from the row.
	h := n.kidsH * n.progress
	block := ggui.Rct(ggui.Pt(x-m.pad, y+n.rowH), ggui.Sz(w+2*m.pad, h))
	bd, bf := s.fold(dst, face, n, block)
	under := ggui.Rct(ggui.Pt(x, y+n.rowH), ggui.Sz(w, n.kidsH))
	bd.FillRect(s.place(under, m.subIndent, 0, ggui.Sz(m.line, n.kidsH)), t.Border)
	sub := s.place(under, m.subIndent+m.line+m.subPad, m.subY, ggui.Sz(max(w-2*m.subIndent-m.line-2*m.subPad, 0), 0))
	sy := sub.Origin.Y
	for i, k := range n.kids {
		if i > 0 {
			sy += m.subGap
		}
		sy += s.paintNode(bd, bf, k, sub.Origin.X, sy, sub.Size.W, m)
	}
	return n.rowH + h
}

// paintGroup paints a group: its heading with the group's action, and its
// rows under that.
func (s *SidebarWidget) paintGroup(dst, face *ggui.Canvas, n *sidebarNode, x, y, w float64, m sidebarMetrics) float64 {
	t := s.theme
	y += m.pad
	if n.rowH > 0 {
		row := ggui.Rct(ggui.Pt(x, y), ggui.Sz(w, n.rowH))
		col := fade(t.Fg, .7*s.labelProgress)
		c := face
		if n.rowH < n.fullH {
			c = face.Clip(row)
		}
		trails := !s.railed()
		if n.e.collapsible && trails {
			item := sidebarItem{s, n.index}
			dst.Describe(row, item)
			if !s.IsInert() && !n.e.disabled {
				dst.HitPointer(row, item)
				dst.HitCursor(row, ggui.CursorShapePointer)
				if n.index == s.hover {
					dst.FillRoundRect(row, t.RadiusSm, t.Accent)
					col = t.AccentFg
				}
			}
			s.paintChevron(c, n, s.place(row, n.chevronX, (n.fullH-m.icon)/2, ggui.Sz(m.icon, m.icon)), col)
			if n.index == s.active {
				controlFocusRing(dst, &s.Interactive, t, row, t.RadiusSm, t.Ring)
				if s.reveal {
					dst.RequestReveal(row)
				}
			}
		}
		n.label.Color(col)
		c.Paint(n.label, s.place(row, m.pad, (n.fullH-n.labelSize.H)/2, n.labelSize))
		if n.e.action != nil && trails {
			dst.Paint(n.e.action, s.place(row, n.actionX, (n.fullH-n.actionSize.H)/2, n.actionSize))
		}
		n.rect = row
		y += n.rowH
	}
	if n.progress > 0 {
		h := n.kidsH * n.progress
		bd, bf := s.fold(dst, face, n, ggui.Rct(ggui.Pt(x-m.pad, y), ggui.Sz(w+2*m.pad, h)))
		ky := y
		for _, k := range n.kids {
			ky += s.paintNode(bd, bf, k, x, ky, w, m)
		}
	}
	return 2*m.pad + n.rowH + n.kidsH*n.progress
}

// paintChevron draws a collapsible row's chevron, which points the way
// reading goes while the row is shut and turns down as it opens.
func (s *SidebarWidget) paintChevron(dst *ggui.Canvas, n *sidebarNode, r ggui.Rect, col color.Color) {
	role, turn := icons.ChevronRight, n.openProgress*math.Pi/2
	if s.rtl {
		role, turn = icons.ChevronLeft, -turn
	}
	paintIcon(dst, s.env, role, r, col, turn)
}

// paintItem paints one destination into row.
func (s *SidebarWidget) paintItem(dst, face *ggui.Canvas, n *sidebarNode, row ggui.Rect, m sidebarMetrics) {
	t := s.theme
	inert := s.IsInert()
	disabled := n.e.disabled || inert
	item := sidebarItem{s, n.index}
	dst.Describe(row, item)
	if !disabled {
		dst.HitPointer(row, item)
		dst.HitCursor(row, ggui.CursorShapePointer)
	}
	lit := n.index == s.cur || (!inert && n.index == s.hover)
	fg := t.Fg
	var fill color.Color
	if n.e.outline {
		fill = t.Bg
	}
	if lit {
		fill, fg = t.Accent, t.AccentFg
	}
	if disabled {
		fg = t.Disabled(fg)
		if fill != nil {
			fill = t.Disabled(fill)
		}
	}
	if n.e.outline {
		ring := pick(lit, t.Accent, t.Border)
		dst.StrokeRoundRect(row.Grow(t.BorderWidth), t.RadiusSm+t.BorderWidth, t.BorderWidth, pick(disabled, t.Disabled(ring), ring))
	}
	if fill != nil {
		dst.FillRoundRect(row, t.RadiusSm, fill)
	}
	// A large row gives its padding up on a rail, where what leads it, a
	// logo or an avatar, is all of it.
	pad := m.pad
	if n.depth == 0 && n.fullH > m.row {
		pad *= s.labelProgress
	}
	shift := pad - m.pad
	switch {
	case n.e.leading != nil:
		face.Paint(n.e.leading, s.place(row, pad, (n.rowH-n.leadSize.H)/2, n.leadSize))
	case n.e.icon != "":
		ink := pick(n.depth > 0 && !disabled, t.AccentFg, fg)
		paintIcon(face, s.env, n.e.icon, s.place(row, pad, (n.rowH-m.icon)/2, n.leadSize), ink, 0)
	}
	ty := (n.rowH - n.labelSize.H - n.detailSize.H) / 2
	n.label.Color(fg)
	face.Paint(n.label, s.place(row, n.labelX+shift, ty, n.labelSize))
	if n.detail != nil {
		n.detail.Color(fg)
		face.Paint(n.detail, s.place(row, n.labelX+shift, ty+n.labelSize.H, n.detailSize))
	}
	if !s.railed() {
		if n.e.collapsible {
			s.paintChevron(face, n, s.place(row, n.chevronX, (n.rowH-m.icon)/2, ggui.Sz(m.icon, m.icon)), fg)
		}
		if n.badge != nil {
			n.badge.Color(fg)
			box := s.place(row, n.badgeX, (n.rowH-m.badge)/2, ggui.Sz(n.badgeW, m.badge))
			face.Paint(n.badge, ggui.Rct(box.Origin.Add(ggui.Pt((box.Size.W-n.badgeSize.W)/2, (box.Size.H-n.badgeSize.H)/2)), n.badgeSize))
		}
		if n.e.action != nil {
			at := s.place(row, n.actionX, (n.rowH-n.actionSize.H)/2, n.actionSize)
			if s.showsAction(dst, n, row, at) {
				dst.Paint(n.e.action, at)
			}
		}
	}
	if n.index == s.active {
		controlFocusRing(dst, &s.Interactive, t, row, t.RadiusSm, t.Ring)
		if s.reveal {
			dst.RequestReveal(row)
		}
	}
	n.rect = row
}

// showsAction reports whether a row's action is on screen: always, or for
// ActionOnHover while the pointer is over the row, the keyboard is on it,
// or the action itself has the keyboard.
func (s *SidebarWidget) showsAction(dst *ggui.Canvas, n *sidebarNode, row, at ggui.Rect) bool {
	if !n.e.actionOnHover || n.index == s.hover || (n.index == s.active && s.Focused && s.FocusVisible) {
		return true
	}
	if p, ok := dst.Pointer(); ok && row.Contains(p) {
		return true
	}
	return dst.FocusWithin(at)
}

// sidebarItem is one row's pointer handler and node.
type sidebarItem struct {
	s *SidebarWidget
	i int
}

func (it sidebarItem) node() *sidebarNode {
	if it.i < 0 || it.i >= len(it.s.rows) {
		return nil
	}
	return it.s.rows[it.i]
}

// role is what the row is to a screen reader: a tab for a destination, a
// disclosure for a row that folds, a button for one with an OnTap.
func (it sidebarItem) role() ggui.Role {
	n := it.node()
	switch {
	case n == nil:
		return ggui.RoleTab
	case n.e.collapsible:
		return ggui.RoleDisclosure
	case n.e.onTap != nil:
		return ggui.RoleButton
	}
	return ggui.RoleTab
}

// Semantics implements ggui.Semantic.
func (it sidebarItem) Semantics() (ggui.Role, string) {
	if n := it.node(); n != nil {
		return it.role(), n.e.label
	}
	return ggui.RoleTab, ""
}

// Describe implements ggui.Describer: a destination says whether it is the
// one being shown, a row that folds whether it is open, and either reads
// its second line and its badge after its name.
func (it sidebarItem) Describe() ggui.Node {
	n := it.node()
	if n == nil {
		return ggui.Node{Role: ggui.RoleTab}
	}
	e := n.e
	node := ggui.Node{
		Role:        it.role(),
		Name:        e.label,
		Description: e.detail,
		Disabled:    e.disabled || it.s.IsInert(),
		Level:       n.depth + 1,
		Actions:     ggui.ActionPress | ggui.ActionFocus,
	}
	if e.badge != "" {
		node.Description = pick(e.detail != "", e.detail+", "+e.badge, e.badge)
	}
	switch node.Role {
	case ggui.RoleDisclosure:
		open := ggui.Untrack(n.isOpen)
		node.Expanded = ggui.Expandable(open)
		node.Actions |= pick(open, ggui.ActionCollapse, ggui.ActionExpand)
	case ggui.RoleTab:
		node.Selected = it.i == it.s.cur
		node.Actions |= ggui.ActionSelect
	}
	return node
}

// Act implements ggui.Actor.
func (it sidebarItem) Act(a ggui.Action) bool {
	n := it.node()
	if n == nil {
		return false
	}
	switch a.Kind {
	case ggui.ActionSelect, ggui.ActionPress:
		it.s.activate(it.i)
	case ggui.ActionExpand, ggui.ActionCollapse:
		if !n.e.collapsible || !it.s.enabled(it.i) || it.s.IsInert() {
			return false
		}
		it.s.setOpen(n, a.Kind == ggui.ActionExpand)
	default:
		return false
	}
	return true
}

func (it sidebarItem) HandlePointer(ev ggui.PointerEvent) bool {
	// The press moves the highlight even when the release lands elsewhere,
	// so the arrow keys carry on from where the pointer went.
	if ev.Kind == ggui.PointerDown {
		it.s.active = it.i
	}
	return hoverPick(ev, it.i, &it.s.hover, func() { it.s.activate(it.i) })
}

func (it sidebarItem) Adopt(prev any) {
	if p, ok := prev.(sidebarItem); ok && p.s.hover == p.i {
		it.s.hover = it.i
	}
}
