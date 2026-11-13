package ui

import (
	"math"
	"slices"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// CommandEntry describes an action in a searchable command list.
type CommandEntry struct {
	label    string
	run      func()
	keywords []string
	disabled bool
	shortcut string
	group    string
	icon     ggui.Widget
	line     bool // a separator rather than an action
}

// CommandItem creates an action. Labels need not be unique.
func CommandItem(label string, run func()) CommandEntry { return CommandEntry{label: label, run: run} }

// CommandSeparator creates a line between entries, from one edge of the
// list to the other. It is shown while the search is empty and hidden
// once a query filters the list.
func CommandSeparator() CommandEntry { return CommandEntry{line: true, disabled: true} }

// Icon puts w, usually an Icon, before the label.
func (e CommandEntry) Icon(w ggui.Widget) CommandEntry { e.icon = w; return e }

// Group labels a contiguous group of entries. Empty groups disappear when filtering.
func (e CommandEntry) Group(name string) CommandEntry { e.group = name; return e }

// Shortcut displays a key hint without registering a global shortcut; a
// chord shows as MenuItemWidget.Shortcut shows it.
func (e CommandEntry) Shortcut(s string) CommandEntry { e.shortcut = s; return e }

// Keywords adds search terms that are not displayed.
func (e CommandEntry) Keywords(terms ...string) CommandEntry {
	e.keywords = append([]string(nil), terms...)
	return e
}

// Disabled leaves the entry visible but excludes it from selection.
func (e CommandEntry) Disabled(v bool) CommandEntry { e.disabled = v; return e }

// CommandWidget is an inline search field and scrollable action list. Put it in
// Dialog for a command palette. Query state belongs to the caller.
type CommandWidget struct {
	props              property.Owner
	query              ggui.Binding[string]
	entries            []CommandEntry
	items              []*MenuItemWidget
	field              *TextFieldWidget
	results            *commandResults
	scroll             *ggui.ScrollWidget
	list               *ggui.BoxWidget // the rows and the padding around them, inside the scroll
	offset             *ggui.StateValue[float64]
	panel              *ggui.BoxWidget
	body               *ggui.ColumnWidget
	styled             *ggui.StyledWidget
	resultsBox         *ggui.BoxWidget // the scroll at its height
	searchBox          *ggui.BoxWidget // the search field's pill
	searchRow          *commandSearchRow
	searchWrap         *ggui.BoxWidget  // the space around the pill
	hintBox            *ggui.BoxWidget  // the space around the hint
	hint               *ggui.TextWidget // the key hints under the results
	matched            []int
	highlight          int
	lastQuery          string
	initialized        bool
	height             float64
	stable, borderless bool
	hints              bool
	hintNav, hintSel   string
	emptyText          string
	dismiss            func() // closes the dialog it is in, set by CommandDialog
	inDialog           bool   // rows take the dialog's larger radius
	// menuRows draws the list as a combobox's: a menu's rows, padding and
	// panel, and a muted empty line.
	menuRows bool
	keepOpen bool                     // choosing leaves the popup open, as a multiple combobox does
	loop     bool                     // the arrows wrap past the ends of the list
	bare     bool                     // no search pill: a combobox's input above the list filters it
	onKey    func(ggui.KeyEvent) bool // sees a key before the list does, set by its owner
	theme    *uitheme.Theme
	env      ggui.Env
	rects    []ggui.Rect
}

// commandListHeight is the most a command list's rows take before the
// list scrolls: 288 with the padding around them.
const commandListHeight = 276

// Command keeps the entries whose label and Keywords the query abbreviates,
// as cmdk does: its letters in their order, in either case, so "tdt" finds
// "Toggle dark theme". The closer matches come first, inside each group and
// group by group, and the first of them is highlighted. Up/Down
// move through enabled matches and stop at the ends unless Loop is set, Home
// and End go to the first and last, and Enter runs the highlighted action.
// IME composition and the other editing keys stay with TextInput.
func Command(query ggui.Binding[string], entries ...CommandEntry) *CommandWidget {
	c := &CommandWidget{query: query, highlight: -1, height: commandListHeight, offset: ggui.State(0.0),
		hintNav: "Navigate", hintSel: "Select", emptyText: "No results"}
	c.field = TextField(query).Placeholder("Search commands…").Name("Search commands").OnKey(c.key)
	if c.field.input.HitID() == nil {
		c.field.input.Key(c)
	}
	c.field.plain = true
	c.results = &commandResults{owner: c}
	c.list = ggui.Box(c.results)
	c.scroll = ggui.Scroll(c.list).BindOffset(c.offset)
	c.setEntries(entries)
	return c
}

// setEntries refreshes results while retaining the editor, query and focus.
// Combobox uses it when its options change; callers arrange a fresh layout.
func (c *CommandWidget) setEntries(entries []CommandEntry) {
	c.entries = append([]CommandEntry(nil), entries...)
	c.items = nil
	c.matched = nil
	c.highlight = -1
	c.initialized = false
	c.rects = make([]ggui.Rect, len(entries))
	for i, e := range entries {
		item := MenuItem(e.label, func() { c.choose(i) }).Disabled(e.disabled)
		item.style = commandStyle
		item.SetKey(item)
		if e.shortcut != "" {
			item.Shortcut(e.shortcut)
		}
		if e.icon != nil {
			item.Icon(e.icon)
		}
		item.onHover = func() { c.highlight = i }
		c.items = append(c.items, item)
	}
}

// Entries replaces the actions, keeping the search field, its query and
// its focus: a palette built once lists the actions of the moment each
// time it opens.
func (c *CommandWidget) Entries(entries ...CommandEntry) *CommandWidget {
	c.setEntries(entries)
	c.props.Changed()
	return c
}

// CommandDialog creates a compact, accessible command palette. The command
// owns its spacing; no visible dialog title is added. Choosing an action
// closes the dialog before the action runs.
func CommandDialog(open ggui.Binding[bool], command *CommandWidget) *DialogWidget {
	command.dismiss = func() { open.Set(false) }
	command.inDialog = true
	return Dialog(open, command.Borderless()).CloseButton(false).Compact().Name("Commands").Width(384).TopAt(1.0 / 3)
}

// Placeholder sets the search hint.
func (c *CommandWidget) Placeholder(s string) *CommandWidget { c.field.Placeholder(s); return c }

// Name names the search field for tests and the inspector.
func (c *CommandWidget) Name(s string) *CommandWidget { c.field.Name(s); return c }

// InsetSearch does nothing: the search field is always a rounded pill
// inset in the panel. It remains for code written when that was an option.
func (c *CommandWidget) InsetSearch() *CommandWidget { return c }

// HintLabels words the footer Hints shows, as "Navigate" and "Select" in
// the application's language.
func (c *CommandWidget) HintLabels(navigate, choose string) *CommandWidget {
	defer property.Watch(&c.props, &c.hintNav)()
	defer property.Watch(&c.props, &c.hintSel)()
	c.hintNav, c.hintSel = navigate, choose
	if c.hint != nil {
		c.hint.Content(c.hintText())
	}
	return c
}

// hintText is the footer Hints shows.
func (c *CommandWidget) hintText() string { return "↑↓ " + c.hintNav + "   ↵ " + c.hintSel }

// Empty sets what the list says when nothing matches; "No results" by
// default.
func (c *CommandWidget) Empty(s string) *CommandWidget {
	defer property.Watch(&c.props, &c.emptyText)()
	c.emptyText = s
	if c.results.empty != nil {
		c.results.empty.Content(s)
	}
	return c
}

// Hints shows a compact keyboard navigation footer.
func (c *CommandWidget) Hints() *CommandWidget {
	defer property.Watch(&c.props, &c.hints)()
	c.hints = true
	return c
}

// Borderless removes the outer border when a Dialog or another panel supplies it.
func (c *CommandWidget) Borderless() *CommandWidget {
	defer property.Watch(&c.props, &c.borderless)()
	c.borderless = true
	return c
}

// Loop makes Down from the last match go to the first and Up from the first
// to the last. The highlight stops at either end by default, as cmdk's does
// without its loop option.
func (c *CommandWidget) Loop(v bool) *CommandWidget {
	c.loop = v
	return c
}

// StableHeight reserves Height pixels even when filtering leaves fewer results.
func (c *CommandWidget) StableHeight() *CommandWidget {
	defer property.Watch(&c.props, &c.stable)()
	c.stable = true
	return c
}

// Height limits the results area, excluding the search field and padding.
func (c *CommandWidget) Height(h float64) *CommandWidget {
	defer property.Watch(&c.props, &c.height)()
	c.height = max(0, h)
	return c
}

func (c *CommandWidget) filter() {
	query := strings.ToLower(strings.TrimSpace(c.query.Get()))
	if c.initialized && query == c.lastQuery {
		return
	}
	c.initialized = true
	c.lastQuery = query
	c.matched = c.matched[:0]
	c.highlight = -1
	if query != "" && !c.menuRows {
		// A palette finds what the query abbreviates, best match first; a
		// combobox's list, below, keeps what contains it, in its order.
		c.matched = append(c.matched, c.rank(strings.TrimSpace(c.query.Get()))...)
		for _, i := range c.matched {
			if !c.entries[i].disabled {
				c.highlight = i
				break
			}
		}
		c.offset.Set(0)
		return
	}
	for i, e := range c.entries {
		if e.line {
			// A separator divides the whole list, not what a query left of it.
			if query == "" {
				c.matched = append(c.matched, i)
			}
			continue
		}
		text := strings.ToLower(e.label + " " + strings.Join(e.keywords, " "))
		if strings.Contains(text, query) {
			c.matched = append(c.matched, i)
			if c.highlight < 0 && !e.disabled {
				c.highlight = i
			}
		}
	}
	c.offset.Set(0)
}

func (c *CommandWidget) choose(index int) {
	if index < 0 || index >= len(c.entries) || c.entries[index].disabled {
		return
	}
	if !c.keepOpen {
		if popup, ok := ggui.PopupOf(c.env); ok {
			popup.Hide()
		}
		if c.dismiss != nil {
			c.dismiss()
		}
	}
	if fn := c.entries[index].run; fn != nil {
		fn()
	}
}

func (c *CommandWidget) key(ev ggui.KeyEvent) bool {
	if ev.Mods != (ggui.Mods{}) {
		return false
	}
	if c.onKey != nil && c.onKey(ev) {
		return true
	}
	switch ev.Key {
	case ggui.KeyEnter, ggui.KeyNumpadEnter:
		c.filter()
		c.choose(c.highlight)
		return true
	case ggui.KeyHome, ggui.KeyEnd:
		if c.menuRows {
			// In a combobox these move the caret in its input.
			return false
		}
		fallthrough
	case ggui.KeyArrowDown, ggui.KeyArrowUp:
		c.filter()
		at := slices.Index(c.matched, c.highlight)
		next, _ := navIndex(ev, navVertical, at, len(c.matched), func(j int) bool { return !c.entries[c.matched[j]].disabled })
		if !c.loop && !c.menuRows && at >= 0 && (ev.Key == ggui.KeyArrowDown && next < at || ev.Key == ggui.KeyArrowUp && next > at) {
			// Past either end the highlight stays where it is.
			next = at
		}
		if next >= 0 {
			c.highlight = c.matched[next]
			c.scroll.Reveal(c.rects[c.highlight])
		}
		return true
	}
	return false
}

// Layout implements ggui.Widget.
func (c *CommandWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	defer c.props.Layout().EndLayout()
	c.env = env
	c.filter()
	t := uitheme.Of(env)
	c.theme = t
	// The rows sit a panel's padding inside their group, which sits another
	// inside the panel; the search pill is inset as far. The padding is
	// inside the scroll, so a separator can reach the panel's edges.
	// A combobox's rows sit only in the panel's padding, as a menu's do.
	edge := t.PanelPad.Left + t.PanelPad.Right
	listPad := ggui.Insets(t.PanelPad.Top, edge, t.PanelPad.Top+t.PanelPad.Bottom, edge)
	if c.menuRows {
		edge, listPad = t.PanelPad.Left, t.PanelPad
	}
	c.list.Padding(listPad)
	radius := 0.0
	if c.inDialog {
		radius = t.Radius
	}
	for _, item := range c.items {
		item.radius = radius
		item.style = pick(c.menuRows, menuStyle, commandStyle)
		item.stay = c.keepOpen
	}
	natural := c.list.Layout(ggui.Loose(ggui.Sz(cs.MaxW, ggui.Unbounded)), env)
	height := min(c.height, natural.H-listPad.Vertical())
	if c.stable {
		height = c.height
	}
	// Whole pixels, so a list that fits is never a fraction too short and
	// shows a scrollbar.
	height = math.Ceil(height + listPad.Vertical())
	// The panel is built once, and again only when the hints come or go,
	// so the hint's shaping and the boxes' layout caches survive a relayout.
	if c.panel == nil || (c.hint != nil) != c.hints {
		c.buildPanel()
	}
	c.resultsBox.Height(height)
	if !c.bare {
		c.searchWrap.Padding(ggui.Insets(edge, edge, 0, edge))
		// The pill is a field's height with a fainter fill and border than one.
		pill := fade(t.InputBorder, .3)
		// The icon leads, 8 from the pill's edge, as shadcn's inline-start
		// addon does.
		pad := ggui.Insets(t.FieldPad.Top, t.FieldPad.Right, t.FieldPad.Bottom, t.Space)
		if env.Direction() == ggui.DirRTL {
			pad.Left, pad.Right = pad.Right, pad.Left
		}
		c.searchBox.Padding(pad).Fill(pill).Border(t.BorderWidth, pill).Radius(t.Radius)
	}
	if c.hintBox != nil {
		c.hintBox.Padding(ggui.Insets(t.Space, t.Space*1.5))
	}
	c.styled.Color(t.PopoverFg)
	if c.menuRows {
		c.panel.Fill(t.Popover).Border(t.BorderWidth, fade(t.Fg, .1)).Radius(t.Radius)
	} else {
		c.panel.Fill(t.Popover).Border(t.BorderWidth, t.Border).Radius(t.RadiusLg)
	}
	if c.borderless {
		c.panel.Border(0, nil)
	}
	if _, ok := ggui.PopupOf(env); ok {
		c.panel.Shadow(t.PanelShadow)
	} else {
		c.panel.Shadow()
	}
	return c.panel.Layout(cs, env)
}

// buildPanel assembles the panel for the current hints; Layout styles it
// from the theme.
func (c *CommandWidget) buildPanel() {
	c.resultsBox = ggui.Box(c.scroll)
	parts := []ggui.Widget{c.resultsBox}
	if !c.bare {
		c.searchRow = &commandSearchRow{c: c}
		c.searchBox = ggui.Box(c.searchRow)
		c.searchWrap = ggui.Box(&commandSearch{c})
		parts = []ggui.Widget{c.searchWrap, c.resultsBox}
	}
	c.hint, c.hintBox = nil, nil
	if c.hints {
		c.hint = Caption(c.hintText())
		c.hintBox = ggui.Box(c.hint)
		parts = append(parts, Divider(), c.hintBox)
	}
	c.body = ggui.Column(parts...).Gap(0).Align(ggui.AlignStretch)
	c.styled = ggui.Styled(c.body)
	c.panel = ggui.Box(c.styled)
}

// commandSearch is the search pill: the whole of it, padding included,
// focuses and clicks into the editor, as a TextField's box does.
type commandSearch struct{ c *CommandWidget }

func (w *commandSearch) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	f := w.c.field
	f.Sync()
	return w.c.searchBox.Layout(cs, env.With(ggui.InputDisabledKey, f.IsInert()))
}

func (w *commandSearch) Paint(dst *ggui.Canvas, r ggui.Rect) {
	f := w.c.field
	if f.input.IsDisabled() {
		dst.Describe(r, f.input)
	} else {
		f.Hit(dst, r, f.input, ggui.CursorShapeText)
	}
	dst.Paint(w.c.searchBox, r)
}

// Paint implements ggui.Widget.
func (c *CommandWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(c.panel, r) }

// commandRow is one matched entry, with the group heading above it when the
// group changes. Keeping the three together stops them drifting out of step.
type commandRow struct {
	gap         float64 // the space above it: between two groups, or beside a separator
	heading     ggui.Widget
	headingSize ggui.Size
	size        ggui.Size
}

type commandResults struct {
	owner     *CommandWidget
	rows      []commandRow
	headings  map[string]*ggui.BoxWidget // each group's heading, kept across layouts
	empty     *ggui.TextWidget
	emptySize ggui.Size
}

// heading is the kept heading of group: small, medium and muted, in a
// row's own padding.
func (r *commandResults) heading(group string, t *uitheme.Theme) *ggui.BoxWidget {
	h := r.headings[group]
	if h == nil {
		if r.headings == nil {
			r.headings = map[string]*ggui.BoxWidget{}
		}
		h = ggui.Box(Caption(group).Weight(ggui.WeightMedium))
		r.headings[group] = h
	}
	return h.Padding(ggui.Insets(t.Space*.75, t.Space))
}

func (r *commandResults) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	c := r.owner
	t := uitheme.Of(env)
	r.rows = r.rows[:0]
	actions := 0
	for _, i := range c.matched {
		if !c.entries[i].line {
			actions++
		}
	}
	if actions == 0 {
		// Nothing matched: a line of body text 24 below the search, and as
		// far above the panel's own padding, with no group around it; a
		// combobox's is muted and 8 from either side.
		if r.empty == nil {
			r.empty = ggui.Text(c.emptyText)
		}
		room := t.Space * 6
		if c.menuRows {
			r.empty.Color(t.MutedFg)
			room = t.Space * 2
		}
		r.emptySize = r.empty.Layout(cs.Loosen(), env)
		height := r.emptySize.H + room - t.PanelPad.Vertical()
		if c.stable {
			height = max(height, c.height)
		}
		return cs.Constrain(ggui.Sz(bounded(cs.MaxW, r.emptySize.W), height))
	}
	// Each group has a panel's padding above and below its rows, so two
	// groups are that twice apart, with a separator, if any, between.
	var w, h float64
	previous, afterLine := "", false
	for j, i := range c.matched {
		if c.entries[i].line {
			row := commandRow{gap: pick(j > 0 && !afterLine, t.PanelPad.Bottom, 0), size: ggui.Sz(0, t.BorderWidth)}
			r.rows = append(r.rows, row)
			h += row.gap + row.size.H
			afterLine = true
			continue
		}
		var heading ggui.Widget
		var hs ggui.Size
		group := c.entries[i].group
		gap := 0.0
		switch {
		case afterLine:
			gap = t.PanelPad.Top
		case j > 0 && group != previous:
			gap = t.PanelPad.Vertical()
		}
		if group != "" && (j == 0 || group != previous || afterLine) {
			heading = r.heading(group, t)
			hs = heading.Layout(ggui.Loose(ggui.Sz(cs.MaxW, ggui.Unbounded)), env)
		}
		previous, afterLine = group, false
		sz := c.items[i].Layout(ggui.Loose(ggui.Sz(cs.MaxW, ggui.Unbounded)), env)
		r.rows = append(r.rows, commandRow{gap: gap, heading: heading, headingSize: hs, size: sz})
		w = max(w, hs.W, sz.W)
		h += gap + hs.H + sz.H
	}
	return cs.Constrain(ggui.Sz(bounded(cs.MaxW, w), h))
}
func (r *commandResults) Paint(dst *ggui.Canvas, rect ggui.Rect) {
	c := r.owner
	if len(r.rows) == 0 {
		s := r.emptySize
		dst.Paint(r.empty, ggui.Rct(ggui.Pt(rect.Origin.X+(rect.Size.W-s.W)/2, rect.Origin.Y+(rect.Size.H-s.H)/2), s))
		return
	}
	t := c.theme
	y := rect.Origin.Y
	for j, i := range c.matched {
		row := r.rows[j]
		y += row.gap
		if c.entries[i].line {
			// Across the list's own padding, to the panel's edges.
			bleed := t.PanelPad.Left + t.PanelPad.Right
			dst.Leaf(ggui.Rct(ggui.Pt(rect.Origin.X, y), ggui.Sz(rect.Size.W, row.size.H)), ggui.Node{Role: ggui.RoleSeparator})
			dst.FillRect(ggui.Rct(ggui.Pt(rect.Origin.X-bleed, y), ggui.Sz(rect.Size.W+2*bleed, row.size.H)), t.Border)
			y += row.size.H
			continue
		}
		if row.heading != nil {
			dst.Paint(row.heading, ggui.Rct(ggui.Pt(rect.Origin.X, y), ggui.Sz(rect.Size.W, row.headingSize.H)))
			y += row.headingSize.H
		}
		at := ggui.Rct(ggui.Pt(rect.Origin.X, y), ggui.Sz(rect.Size.W, row.size.H))
		c.rects[i] = at
		c.items[i].active = i == c.highlight
		dst.Paint(c.items[i], at)
		y += row.size.H
	}
}

// commandSearchRow is what the search pill holds: the search icon, drawn
// rather than typed so it needs no font to cover it, and after it the
// editor, which takes the rest of the pill. The icon leads in the reading
// direction, as the addon of shadcn's CommandInput does.
type commandSearchRow struct {
	c     *CommandWidget
	icon  ggui.Rect // where the icon was last painted
	input ggui.Size
	rtl   bool
}

func (w *commandSearchRow) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	w.rtl = env.Direction() == ggui.DirRTL
	lead := t.IconSize + t.Space*.75
	inner := cs
	inner.MinW, inner.MaxW = max(0, cs.MinW-lead), max(0, cs.MaxW-lead)
	if inner.MaxW < ggui.Unbounded {
		inner.MinW = inner.MaxW
	}
	w.input = w.c.field.input.Layout(inner, env)
	return cs.Constrain(ggui.Sz(w.input.W+lead, max(w.input.H, t.IconSize)))
}

func (w *commandSearchRow) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := w.c.theme
	lead := t.IconSize + t.Space*.75
	w.icon = ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+(r.Size.H-t.IconSize)/2), ggui.Sz(t.IconSize, t.IconSize))
	at := ggui.Rct(ggui.Pt(r.Origin.X+lead, r.Origin.Y+(r.Size.H-w.input.H)/2), ggui.Sz(max(0, r.Size.W-lead), w.input.H))
	if w.rtl {
		w.icon.Origin.X = r.Origin.X + r.Size.W - t.IconSize
		at.Origin.X = r.Origin.X
	}
	paintIcon(dst, w.c.env, icons.Search, w.icon, fade(t.PopoverFg, .5), 0)
	dst.Paint(w.c.field.input, at)
}

// BindName follows the accessible name on the same targets as Name.
func (c *CommandWidget) BindName(r ggui.Readable[string]) *CommandWidget {
	c.field.BindName(r)
	return c
}
