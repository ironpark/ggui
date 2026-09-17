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
}

// CommandItem creates an action. Labels need not be unique.
func CommandItem(label string, run func()) CommandEntry { return CommandEntry{label: label, run: run} }

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
	query       ggui.Binding[string]
	entries     []CommandEntry
	items       []*MenuItemWidget
	field       *TextFieldWidget
	results     *commandResults
	scroll      *ggui.ScrollWidget
	offset      *ggui.Signal[float64]
	panel       *ggui.BoxWidget
	body        *ggui.ColumnWidget
	matched     []int
	highlight   int
	lastQuery   string
	initialized bool
	height      float64
	env         ggui.Env
	rects       []ggui.Rect
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
	c.results = &commandResults{owner: c}
	c.scroll = ggui.Scroll(c.results).Offset(c.offset)
	c.rects = make([]ggui.Rect, len(entries))
	for i, e := range entries {
		c.items = append(c.items, MenuItem(e.label, func() { c.choose(i) }).Disabled(e.disabled))
	}
	return c
}

// Placeholder sets the search hint.
func (c *CommandWidget) Placeholder(s string) *CommandWidget { c.field.Placeholder(s); return c }

// Label names the search field for tests and the inspector.
func (c *CommandWidget) Label(s string) *CommandWidget { c.field.Label(s); return c }

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
	inner := t.PanelPad.Shrink(cs)
	natural := c.results.Layout(ggui.Loose(ggui.Sz(inner.MaxW, ggui.Unbounded)), env)
	results := ggui.Box(c.scroll).Height(min(c.height, natural.H))
	c.body = ggui.Column(c.field, results).Gap(t.Space / 2).Align(ggui.AlignStretch)
	c.panel = ggui.Box(c.body).Padding(t.PanelPad).Fill(t.Surface).Border(1, t.Border).Radius(t.Radius)
	return c.panel.Layout(cs, env)
}

// Paint implements ggui.Widget.
func (c *CommandWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(c.panel, r) }

type commandResults struct {
	owner *CommandWidget
	sizes []ggui.Size
	empty *ggui.TextWidget
}

func (r *commandResults) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	c := r.owner
	r.sizes = r.sizes[:0]
	if len(c.matched) == 0 {
		r.empty = ggui.Caption("No results")
		return r.empty.Layout(cs, env)
	}
	var w, h float64
	for _, i := range c.matched {
		sz := c.items[i].Layout(ggui.Loose(ggui.Sz(cs.MaxW, ggui.Unbounded)), env)
		r.sizes = append(r.sizes, sz)
		w = max(w, sz.W)
		h += sz.H
	}
	return cs.Constrain(ggui.Sz(bounded(cs.MaxW, w), h))
}
func (r *commandResults) Paint(dst *ggui.Canvas, rect ggui.Rect) {
	c := r.owner
	if len(c.matched) == 0 {
		dst.Paint(r.empty, rect)
		return
	}
	y := rect.Origin.Y
	for j, i := range c.matched {
		at := ggui.Rct(ggui.Pt(rect.Origin.X, y), ggui.Sz(rect.Size.W, r.sizes[j].H))
		c.rects[i] = at
		c.items[i].active = i == c.highlight
		dst.Paint(c.items[i], at)
		y += r.sizes[j].H
	}
}
