package ui

import (
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// CommandEntry describes an action in a searchable command list.
type CommandEntry struct {
	label    string
	run      func()
	keywords []string
	disabled bool
	shortcut string
	group    string
}

// CommandItem creates an action. Labels need not be unique.
func CommandItem(label string, run func()) CommandEntry { return CommandEntry{label: label, run: run} }

// Group labels a contiguous group of entries. Empty groups disappear when filtering.
func (e CommandEntry) Group(name string) CommandEntry { e.group = name; return e }

// Shortcut displays a key hint without registering a global shortcut.
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
	query              ggui.Binding[string]
	entries            []CommandEntry
	items              []*MenuItemWidget
	field              *TextFieldWidget
	results            *commandResults
	scroll             *ggui.ScrollWidget
	offset             *ggui.Signal[float64]
	panel              *ggui.BoxWidget
	body               *ggui.ColumnWidget
	matched            []int
	highlight          int
	lastQuery          string
	initialized        bool
	height             float64
	stable, borderless bool
	hints              bool
	insetSearch        bool
	theme              ggui.Theme
	env                ggui.Env
	rects              []ggui.Rect
}

// Command filters labels and Keywords by case-insensitive substring. Up/Down
// wrap through enabled matches; Enter runs the highlighted action. IME
// composition and normal editing keys stay with TextInput.
func Command(query ggui.Binding[string], entries ...CommandEntry) *CommandWidget {
	c := &CommandWidget{query: query, entries: append([]CommandEntry(nil), entries...), highlight: -1, height: 200, offset: ggui.State(0.0)}
	c.field = TextField(query).Placeholder("Search commands…").Label("Search commands").OnKey(c.key)
	if c.field.input.HitID() == nil {
		c.field.input.Key(c)
	}
	c.field.plain = true
	c.results = &commandResults{owner: c}
	c.scroll = ggui.Scroll(c.results).Offset(c.offset)
	c.rects = make([]ggui.Rect, len(entries))
	for i, e := range entries {
		item := MenuItem(e.label, func() { c.choose(i) }).Disabled(e.disabled)
		if e.shortcut != "" {
			item.Shortcut(e.shortcut)
		}
		item.onHover = func() { c.highlight = i }
		c.items = append(c.items, item)
	}
	return c
}

// CommandDialog creates a compact, accessible command palette. The command
// owns its spacing; no visible dialog title is added.
func CommandDialog(open ggui.Binding[bool], command *CommandWidget) *DialogWidget {
	return Dialog(open, command.Borderless().InsetSearch()).Compact().Named("Commands").Width(440)
}

// Placeholder sets the search hint.
func (c *CommandWidget) Placeholder(s string) *CommandWidget { c.field.Placeholder(s); return c }

// Label names the search field for tests and the inspector.
func (c *CommandWidget) Label(s string) *CommandWidget { c.field.Label(s); return c }

// InsetSearch gives the search field a muted, rounded background.
func (c *CommandWidget) InsetSearch() *CommandWidget { c.insetSearch = true; return c }

// Hints shows a compact keyboard navigation footer.
func (c *CommandWidget) Hints() *CommandWidget { c.hints = true; return c }

// Borderless removes the outer border when a Dialog or another panel supplies it.
func (c *CommandWidget) Borderless() *CommandWidget { c.borderless = true; return c }

// StableHeight reserves Height pixels even when filtering leaves fewer results.
func (c *CommandWidget) StableHeight() *CommandWidget { c.stable = true; return c }

// Height limits the results area, excluding the search field and padding.
func (c *CommandWidget) Height(h float64) *CommandWidget { c.height = max(0, h); return c }

func (c *CommandWidget) filter() {
	query := strings.ToLower(strings.TrimSpace(c.query.Get()))
	if c.initialized && query == c.lastQuery {
		return
	}
	c.initialized = true
	c.lastQuery = query
	c.matched = c.matched[:0]
	c.highlight = -1
	for i, e := range c.entries {
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
	if popup, ok := ggui.PopupOf(c.env); ok {
		popup.Hide()
	}
	if fn := c.entries[index].run; fn != nil {
		fn()
	}
}

func (c *CommandWidget) key(ev ggui.KeyEvent) bool {
	if ev.Mods != (ggui.Mods{}) {
		return false
	}
	switch ev.Key {
	case ebiten.KeyEnter, ebiten.KeyNumpadEnter:
		c.filter()
		c.choose(c.highlight)
		return true
	case ebiten.KeyArrowDown, ebiten.KeyArrowUp:
		c.filter()
		at := -1
		for j, i := range c.matched {
			if i == c.highlight {
				at = j
				break
			}
		}
		next := stepIndex(at, pick(ev.Key == ebiten.KeyArrowUp, -1, 1), len(c.matched), func(j int) bool { return !c.entries[c.matched[j]].disabled })
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
	c.env = env
	c.filter()
	t := env.Theme()
	c.theme = t
	inner := t.PanelPad.Shrink(cs)
	natural := c.results.Layout(ggui.Loose(ggui.Sz(inner.MaxW, ggui.Unbounded)), env)
	height := min(c.height, natural.H)
	if c.stable {
		height = c.height
	}
	results := ggui.Box(c.scroll).Height(height + 8).Pad(4)
	search := ggui.Padding(ggui.Row(commandSearchIcon{c}, ggui.Expanded(c.field)).Gap(0), 0, 8)
	parts := []ggui.Widget{search, Divider(), results}
	if c.insetSearch {
		search = ggui.Padding(ggui.Box(search).Fill(mutedSurface(t)).Radius(t.Radius), 8, 8, 0, 8)
		parts = []ggui.Widget{search, results}
	}
	if c.hints {
		parts = append(parts, Divider(), ggui.Padding(ggui.Caption("↑↓ Navigate   ↵ Select"), 8, 12))
	}
	c.body = ggui.Column(parts...).Gap(0).Align(ggui.AlignStretch)
	c.panel = ggui.Box(c.body).Fill(t.Surface).Border(1, t.Border).Radius(t.Radius)
	if c.borderless {
		c.panel.Border(0, nil)
	}
	if _, ok := ggui.PopupOf(env); ok {
		c.panel.Shadow(panelShadow(t))
	}
	return c.panel.Layout(cs, env)
}

// Paint implements ggui.Widget.
func (c *CommandWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(c.panel, r) }

// commandRow is one matched entry, with the group heading above it when the
// group changes. Keeping the three together stops them drifting out of step.
type commandRow struct {
	heading     ggui.Widget
	headingSize ggui.Size
	size        ggui.Size
}

type commandResults struct {
	owner     *CommandWidget
	rows      []commandRow
	empty     *ggui.TextWidget
	emptySize ggui.Size
}

func (r *commandResults) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	c := r.owner
	r.rows = r.rows[:0]
	if len(c.matched) == 0 {
		if r.empty == nil {
			r.empty = ggui.Caption("No results")
		}
		r.emptySize = r.empty.Layout(cs.Loosen(), env)
		height := max(64, r.emptySize.H+24)
		if c.stable {
			height = max(height, c.height)
		}
		return cs.Constrain(ggui.Sz(bounded(cs.MaxW, r.emptySize.W), height))
	}
	var w, h float64
	previous := ""
	for j, i := range c.matched {
		var heading ggui.Widget
		var hs ggui.Size
		group := c.entries[i].group
		if group != "" && (j == 0 || group != previous) {
			label := ggui.Padding(ggui.Caption(group), 6, 8)
			heading = label
			if j > 0 {
				heading = ggui.Column(MenuDivider(), label).Gap(0).Align(ggui.AlignStretch)
			}
			hs = heading.Layout(ggui.Loose(ggui.Sz(cs.MaxW, ggui.Unbounded)), env)
		}
		previous = group
		sz := c.items[i].Layout(ggui.Loose(ggui.Sz(cs.MaxW, ggui.Unbounded)), env)
		r.rows = append(r.rows, commandRow{heading: heading, headingSize: hs, size: sz})
		w = max(w, hs.W, sz.W)
		h += hs.H + sz.H
	}
	return cs.Constrain(ggui.Sz(bounded(cs.MaxW, w), h))
}
func (r *commandResults) Paint(dst *ggui.Canvas, rect ggui.Rect) {
	c := r.owner
	if len(c.matched) == 0 {
		s := r.emptySize
		dst.Paint(r.empty, ggui.Rct(ggui.Pt(rect.Origin.X+(rect.Size.W-s.W)/2, rect.Origin.Y+(rect.Size.H-s.H)/2), s))
		return
	}
	y := rect.Origin.Y
	for j, i := range c.matched {
		row := r.rows[j]
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

// A drawn search glyph avoids relying on font coverage for an icon character.
type commandSearchIcon struct{ c *CommandWidget }

func (i commandSearchIcon) Layout(cs ggui.Constraints, _ ggui.Env) ggui.Size {
	return cs.Constrain(ggui.Sz(18, 18))
}
func (i commandSearchIcon) Paint(dst *ggui.Canvas, r ggui.Rect) {
	col := i.c.theme.Muted
	center := ggui.Pt(r.Origin.X+8, r.Origin.Y+r.Size.H/2-1)
	dst.StrokeRoundRect(ggui.Rct(center.Add(ggui.Pt(-4, -4)), ggui.Sz(8, 8)), 4, 1.4, col)
	dst.StrokeLine(center.Add(ggui.Pt(3, 3)), center.Add(ggui.Pt(7, 7)), 1.4, col)
}
