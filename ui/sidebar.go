package ui

import (
	"github.com/ironpark/ggui"
)

// SidebarEntry is one line of a Sidebar: a destination, or a heading over
// the destinations that follow it. Build one with SidebarItem or
// SidebarSection.
type SidebarEntry struct {
	key, label string
	section    bool
	disabled   bool
}

// SidebarItem creates a destination. The key is what the binding holds when
// this item is the chosen one; keys must be unique within a sidebar.
func SidebarItem(key, label string) SidebarEntry {
	return SidebarEntry{key: key, label: label}
}

// SidebarSection creates a heading over the items that follow it. It takes
// no input and is skipped by the arrow keys.
func SidebarSection(title string) SidebarEntry {
	return SidebarEntry{label: title, section: true}
}

// Disabled shows the item greyed out and skips it.
func (e SidebarEntry) Disabled(v bool) SidebarEntry { e.disabled = v; return e }

// SidebarWidget is a column of destinations down the side of a window, one
// of which is the current one. Build one with Sidebar.
//
// The whole column is one keyboard tab stop, as a tab strip is: Up and Down
// move the highlight over the enabled items and wrap, Home and End go to
// the ends, and Space or Enter goes to the highlighted destination. A click
// goes there directly. Collapsed takes the sidebar off the page entirely,
// which is what a narrow window wants; there is no icon rail, since an item
// is named by its label alone.
type SidebarWidget struct {
	ggui.Interactive // hover holds the item under the pointer instead
	selected         ggui.Binding[string]
	entries          []SidebarEntry
	labels           []*ggui.TextWidget
	onChange         func(string)
	header, footer   ggui.Widget
	width            float64
	collapsed        ggui.Reader[bool]
	active           int
	hover            int

	theme                  ggui.Theme
	cur                    int // the current destination, found once a frame by paint
	labelSize              []ggui.Size
	rowH                   []float64
	headerSize, footerSize ggui.Size
	hidden                 bool
}

// Sidebar creates a navigation column bound to the key of the chosen item.
//
//	page := ggui.State("inbox")
//	ui.Sidebar(page,
//		ui.SidebarSection("Mail"),
//		ui.SidebarItem("inbox", "Inbox"),
//		ui.SidebarItem("sent", "Sent"),
//	).Header(ggui.Title("Acme")).Collapsed(narrow)
func Sidebar(selected ggui.Binding[string], entries ...SidebarEntry) *SidebarWidget {
	s := &SidebarWidget{selected: selected, entries: entries, width: 240, active: -1, hover: -1}
	s.Role = ggui.RoleTabs
	s.AutoKey()
	if s.HitID() == nil {
		s.Key(s)
	}
	seen := map[string]bool{}
	for i, e := range entries {
		if !e.section {
			if seen[e.key] {
				panic("ui: duplicate Sidebar key")
			}
			seen[e.key] = true
			if s.active < 0 && !e.disabled {
				s.active = i
			}
		}
		s.labels = append(s.labels, ggui.Text(e.label).NoWrap())
	}
	return s
}

// Width sets how wide the column is in logical pixels.
func (s *SidebarWidget) Width(w float64) *SidebarWidget { s.width = max(0, w); return s }

// Named sets the accessible name of the column.
func (s *SidebarWidget) Named(name string) *SidebarWidget { s.Name = name; return s }

// Header places a widget above the items: a product name, a workspace
// switcher, a search box.
func (s *SidebarWidget) Header(w ggui.Widget) *SidebarWidget { s.header = w; return s }

// Footer places a widget below the items.
func (s *SidebarWidget) Footer(w ggui.Widget) *SidebarWidget { s.footer = w; return s }

// Collapsed follows r: while it is true the column takes no width and
// paints nothing, so a narrow window gives its space back to the content.
func (s *SidebarWidget) Collapsed(r ggui.Reader[bool]) *SidebarWidget { s.collapsed = r; return s }

// Disabled greys the column out and ignores input while v is true.
func (s *SidebarWidget) Disabled(v bool) *SidebarWidget { s.SetInert(v); return s }

// OnChange fires with the key after the user went to another destination.
func (s *SidebarWidget) OnChange(fn func(string)) *SidebarWidget { s.onChange = fn; return s }

// enabled reports whether the arrow keys and the pointer may land on entry i.
func (s *SidebarWidget) enabled(i int) bool {
	return !s.entries[i].section && !s.entries[i].disabled
}

func (s *SidebarWidget) goTo(i int) {
	if i < 0 || i >= len(s.entries) || !s.enabled(i) || s.Inert {
		return
	}
	s.active = i
	setChanged(s.selected, s.entries[i].key, s.onChange)
}

// current returns the entry the binding names, or -1.
func (s *SidebarWidget) current() int {
	key := s.selected.Peek()
	for i, e := range s.entries {
		if !e.section && e.key == key {
			return i
		}
	}
	return -1
}

// Layout implements ggui.Widget.
func (s *SidebarWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	s.Sync()
	t := env.Theme()
	t.Card, t.Fg = colorOr(t.Sidebar, t.Card), colorOr(t.SidebarFg, t.Fg)
	t.Muted = colorOr(t.SidebarAccent, t.Muted)
	t.Border, t.Ring = colorOr(t.SidebarBorder, t.Border), colorOr(t.SidebarRing, t.Ring)
	// Children see the remapped palette, not only its text color, so a
	// button or badge inside an item paints against the sidebar surface.
	t.Text.Color = t.Fg
	env = env.WithTheme(t).WithText(ggui.TextStyle{Color: t.Fg})
	s.theme = t
	s.hidden = s.collapsed != nil && s.collapsed.Get()
	if s.hidden {
		return c.Constrain(ggui.Size{})
	}
	w := min(s.width, c.MaxW)
	inner := ggui.Constraints{MaxW: max(w-t.Space*2, 0), MaxH: ggui.Unbounded}
	s.labelSize, s.rowH = s.labelSize[:0], s.rowH[:0]
	height := t.Space
	s.headerSize, s.footerSize = ggui.Size{}, ggui.Size{}
	if s.header != nil {
		s.headerSize = s.header.Layout(inner, env)
		height += s.headerSize.H + t.Space
	}
	cur := s.current()
	for i, e := range s.entries {
		l := s.labels[i]
		switch {
		case e.section:
			l.Style(t.Caption).Color(t.MutedFg)
		case e.disabled || s.Inert:
			l.Style(t.Text).Color(mix(t.MutedFg, t.Card, t.DisabledMix))
		default:
			l.Style(t.Text).Color(pick(i == cur, colorOr(t.SidebarAccentFg, t.Fg), t.MutedFg))
		}
		size := l.Layout(ggui.Loose(inner.Max()), env)
		s.labelSize = append(s.labelSize, size)
		pad := pick(e.section, t.Space/2, t.ItemPad.Top+t.ItemPad.Bottom)
		s.rowH = append(s.rowH, size.H+pad)
		height += s.rowH[i]
	}
	if s.footer != nil {
		s.footerSize = s.footer.Layout(inner, env)
		height += s.footerSize.H + t.Space
	}
	height += t.Space
	return c.Constrain(ggui.Sz(w, max(bounded(c.MaxH, height), height)))
}

// Paint implements ggui.Widget. The column is one node holding a
// destination each, which the hit regions alone could not say.
func (s *SidebarWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if s.hidden {
		return
	}
	dst.DescribeNode(r, s, func(dst *ggui.Canvas) { s.paint(dst, r) })
}

func (s *SidebarWidget) paint(dst *ggui.Canvas, r ggui.Rect) {
	t := s.theme
	dst.FillRect(r, t.Card)
	dst.FillRect(ggui.Rct(ggui.Pt(r.Origin.X+r.Size.W-t.BorderWidth, r.Origin.Y), ggui.Sz(t.BorderWidth, r.Size.H)), t.Border)
	dst = dst.Clip(r)
	if !s.Inert && s.active >= 0 {
		dst.HitKey(r, s)
	}
	x, w := r.Origin.X+t.Space, max(r.Size.W-t.Space*2, 0)
	y := r.Origin.Y + t.Space
	if s.header != nil {
		dst.Paint(s.header, ggui.Rct(ggui.Pt(x, y), ggui.Sz(w, s.headerSize.H)))
		y += s.headerSize.H + t.Space
	}
	// Layout is skipped on a still frame, so the current destination is
	// found here, where the items' own nodes read it back.
	s.cur = s.current()
	cur, hover := s.cur, pick(s.Inert, -1, s.hover)
	for i, e := range s.entries {
		row := ggui.Rct(ggui.Pt(x, y), ggui.Sz(w, s.rowH[i]))
		label := ggui.Rct(ggui.Pt(x+t.ItemPad.Left, y+(s.rowH[i]-s.labelSize[i].H)/2), s.labelSize[i])
		if e.section {
			dst.Paint(s.labels[i], label)
			y += s.rowH[i]
			continue
		}
		item := sidebarItem{s, i}
		dst.Describe(row, item)
		if !s.Inert && !e.disabled {
			dst.HitPointer(row, item)
			dst.HitCursor(row, ggui.CursorShapePointer)
		}
		switch {
		case i == cur:
			dst.FillRoundRect(row, t.RadiusSm, t.Muted)
		case i == hover:
			dst.FillRoundRect(row, t.RadiusSm, mix(t.Card, t.Fg, t.HoverMix))
		}
		dst.Paint(s.labels[i], label)
		if i == s.active {
			s.FocusRing(dst, row, t.RadiusSm, t.Ring)
		}
		y += s.rowH[i]
	}
	if s.footer != nil {
		bottom := r.Origin.Y + r.Size.H - t.Space - s.footerSize.H
		dst.Paint(s.footer, ggui.Rct(ggui.Pt(x, max(bottom, y)), ggui.Sz(w, s.footerSize.H)))
	}
}

// HandleKey implements ggui.KeyHandler: Up and Down move the highlight,
// Space and Enter go where it is.
func (s *SidebarWidget) HandleKey(ev ggui.KeyEvent) {
	s.Keyboard(ev, func() { s.goTo(s.active) })
	if ev.Kind != ggui.KeyPress || len(s.entries) == 0 {
		return
	}
	switch ev.Key {
	case ggui.KeyArrowDown:
		s.active = stepIndex(s.active, 1, len(s.entries), s.enabled)
	case ggui.KeyArrowUp:
		s.active = stepIndex(s.active, -1, len(s.entries), s.enabled)
	case ggui.KeyHome:
		s.active = stepIndex(-1, 1, len(s.entries), s.enabled)
	case ggui.KeyEnd:
		s.active = stepIndex(-1, -1, len(s.entries), s.enabled)
	}
}

// ConsumesKey implements ggui.KeyConsumer.
func (s *SidebarWidget) ConsumesKey(ev ggui.KeyEvent) bool {
	switch ev.Key {
	case ggui.KeyArrowUp, ggui.KeyArrowDown, ggui.KeyHome, ggui.KeyEnd:
		return ev.Kind == ggui.KeyPress
	}
	return s.Interactive.ConsumesKey(ev)
}

// Adopt implements ggui.Adopter: the highlight and the hover follow the
// item they were on, by key, across a rebuild.
func (s *SidebarWidget) Adopt(prev any) {
	s.Interactive.Adopt(prev)
	p, ok := prev.(*SidebarWidget)
	if !ok {
		return
	}
	find := func(i int) int {
		if i < 0 || i >= len(p.entries) {
			return -1
		}
		for j, e := range s.entries {
			if !e.section && e.key == p.entries[i].key {
				return j
			}
		}
		return -1
	}
	if i := find(p.active); i >= 0 {
		s.active = i
	}
	s.hover = find(p.hover)
}

// sidebarItem is one destination's pointer handler and node.
type sidebarItem struct {
	s *SidebarWidget
	i int
}

// Semantics implements ggui.Semantic.
func (it sidebarItem) Semantics() (ggui.Role, string) {
	return ggui.RoleTab, it.s.entries[it.i].label
}

// Describe implements ggui.Describer: a destination says whether it is the
// one being shown.
func (it sidebarItem) Describe() ggui.Node {
	e := it.s.entries[it.i]
	return ggui.Node{
		Role:     ggui.RoleTab,
		Name:     e.label,
		Selected: it.i == it.s.cur,
		Disabled: e.disabled || it.s.Inert,
		Actions:  ggui.ActionSelect | ggui.ActionPress | ggui.ActionFocus,
	}
}

// Act implements ggui.Actor.
func (it sidebarItem) Act(a ggui.Action) bool {
	if a.Kind != ggui.ActionSelect && a.Kind != ggui.ActionPress {
		return false
	}
	it.s.goTo(it.i)
	return true
}

func (it sidebarItem) HandlePointer(ev ggui.PointerEvent) bool {
	// The press moves the highlight even when the release lands elsewhere,
	// so the arrow keys carry on from where the pointer went.
	if ev.Kind == ggui.PointerDown {
		it.s.active = it.i
	}
	return hoverPick(ev, it.i, &it.s.hover, func() { it.s.goTo(it.i) })
}

func (it sidebarItem) Adopt(prev any) {
	if p, ok := prev.(sidebarItem); ok && p.s.hover == p.i {
		it.s.hover = it.i
	}
}

// DisabledWhen follows r for Disabled without rebuilding the control.
func (s *SidebarWidget) DisabledWhen(r ggui.Reader[bool]) *SidebarWidget { s.InertWhen(r); return s }

func (s *SidebarWidget) name() string { return pick(s.Name != "", s.Name, "Sidebar") }

// Semantics implements ggui.Semantic, including the built-in fallback name.
func (s *SidebarWidget) Semantics() (ggui.Role, string) { return s.Role, s.name() }
