package ui

import (
	"math"
	"slices"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/icons"
	"github.com/ironpark/ggui/internal/property"
)

// AccordionSection describes a keyed disclosure and its persistent content.
type AccordionSection struct {
	key, title string
	content    ggui.Widget
	disabled   bool
}

// AccordionItem creates one section. Keys must be unique within an accordion.
func AccordionItem(key, title string, content ggui.Widget) AccordionSection {
	return AccordionSection{key: key, title: title, content: content}
}

// Disabled prevents user interaction with the section header.
func (s AccordionSection) Disabled(v bool) AccordionSection { s.disabled = v; return s }

// AccordionWidget groups disclosures under one keyboard tab stop. Up/Down,
// Home/End select a header; Space/Enter toggle it. Open content remains tabbable.
type AccordionWidget struct {
	props property.Owner
	ggui.Interactive
	laidOpen []string
	open     ggui.Binding[[]string]
	items    []AccordionSection
	headers  []*accordionHeader
	multiple bool
	active   int
	onChange func([]string)
	theme    ggui.Theme
	sizes    []ggui.Size
	heights  []float64
	env      ggui.Env
}

// Accordion binds open section keys. By default opening a section closes the
// others; Multiple permits independent sections. Clicking an open section closes it.
func Accordion(open ggui.Binding[[]string], items ...AccordionSection) *AccordionWidget {
	a := &AccordionWidget{open: open, items: append([]AccordionSection(nil), items...), active: -1}
	a.Role = ggui.RoleAccordion
	a.AutoKey()
	if a.HitID() == nil {
		a.Key(a)
	}
	seen := map[string]bool{}
	for i, item := range items {
		if seen[item.key] {
			panic("ui: duplicate Accordion key")
		}
		seen[item.key] = true
		h := &accordionHeader{owner: a, index: i, text: ggui.Text(item.title)}
		h.Role = ggui.RoleDisclosure
		h.SetName(item.title)
		h.SetInert(item.disabled)
		h.AutoKey()
		a.headers = append(a.headers, h)
		if a.active < 0 && !item.disabled {
			a.active = i
		}
	}
	return a
}

// Multiple allows more than one open section.
func (a *AccordionWidget) Multiple() *AccordionWidget {
	defer property.Watch(&a.props, &a.multiple)()
	a.multiple = true
	return a
}

// Name names the group keyboard target.
func (a *AccordionWidget) Name(s string) *AccordionWidget { a.SetName(s); return a }

// OnChange reports a copy of the open keys after a user toggles a header.
func (a *AccordionWidget) OnChange(fn func([]string)) *AccordionWidget { a.onChange = fn; return a }
func (a *AccordionWidget) isOpen(i int) bool                           { return slices.Contains(a.laidOpen, a.items[i].key) }
func (a *AccordionWidget) toggle(i int) {
	if i < 0 || i >= len(a.items) || a.items[i].disabled {
		return
	}
	key := a.items[i].key
	next := slices.Clone(ggui.Untrack(a.open.Get))
	if slices.Contains(next, key) {
		next = slices.DeleteFunc(next, func(k string) bool { return k == key })
	} else if a.multiple {
		next = append(next, key)
	} else {
		next = []string{key}
	}
	a.open.Set(next)
	ggui.Invalidate(a.env)
	if a.onChange != nil {
		a.onChange(slices.Clone(next))
	}
}

// ConsumesKey implements ggui.KeyConsumer.
func (a *AccordionWidget) ConsumesKey(ev ggui.KeyEvent) bool {
	if ev.Kind != ggui.KeyPress {
		return false
	}
	switch ev.Key {
	case ggui.KeyArrowUp, ggui.KeyArrowDown, ggui.KeyHome, ggui.KeyEnd:
		return true
	}
	return ggui.Activates(ev)
}

// HandleKey implements ggui.KeyHandler.
func (a *AccordionWidget) HandleKey(ev ggui.KeyEvent) {
	a.Keyboard(ev, func() { a.toggle(a.active) })
	if ev.Kind != ggui.KeyPress {
		return
	}
	enabled := func(i int) bool { return !a.items[i].disabled }
	switch ev.Key {
	case ggui.KeyArrowDown:
		a.active = stepIndex(a.active, 1, len(a.items), enabled)
	case ggui.KeyArrowUp:
		a.active = stepIndex(a.active, -1, len(a.items), enabled)
	case ggui.KeyHome:
		a.active = stepIndex(-1, 1, len(a.items), enabled)
	case ggui.KeyEnd:
		a.active = stepIndex(-1, -1, len(a.items), enabled)
	}
}

// Adopt keeps the active header when the group is rebuilt.
func (a *AccordionWidget) Adopt(prev any) {
	a.Interactive.Adopt(prev)
	if p, ok := prev.(*AccordionWidget); ok && p.active >= 0 && p.active < len(p.items) {
		key := p.items[p.active].key
		for i, item := range a.items {
			if item.key == key && !item.disabled {
				a.active = i
				break
			}
		}
	}
}

// Layout implements ggui.Widget.
func (a *AccordionWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer a.props.Layout()()
	a.env, a.theme = env, env.Theme()
	a.laidOpen = append(a.laidOpen[:0], a.open.Get()...)
	a.sizes = slices.Grow(a.sizes[:0], len(a.items))[:len(a.items)]
	clear(a.sizes)
	a.heights = slices.Grow(a.heights[:0], len(a.items))[:len(a.items)]
	now, motion := ggui.Now(), env.Motion(a.theme.MotionFast)
	var width, height float64
	for i, item := range a.items {
		h := a.headers[i]
		h.text.Color(pick(item.disabled, a.theme.MutedFg, a.theme.Fg))
		h.size = h.text.Layout(ggui.Loose(ggui.Sz(max(c.MaxW-24, 0), ggui.Unbounded)), env)
		a.heights[i] = h.size.H + 32
		width = max(width, h.size.W+24)
		height += a.heights[i]
		h.open = a.isOpen(i)
		h.progress = h.reveal.Toggle(h.open, now, motion)
		if h.open || h.progress > 0 {
			a.sizes[i] = item.content.Layout(ggui.Loose(ggui.Sz(c.MaxW, ggui.Unbounded)), env)
			width = max(width, a.sizes[i].W)
			height += (a.sizes[i].H + 16) * h.progress
		}
	}
	return c.Constrain(ggui.Sz(width, height))
}

// Paint implements ggui.Widget.
func (a *AccordionWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.DescribeNode(r, a, func(dst *ggui.Canvas) { a.paint(dst, r) })
}

func (a *AccordionWidget) paint(dst *ggui.Canvas, r ggui.Rect) {
	if !slices.Equal(a.laidOpen, ggui.Untrack(a.open.Get)) {
		ggui.Invalidate(a.env)
	}
	dst = dst.Clip(r)
	if a.active >= 0 {
		dst.HitKey(r, a)
	}
	y := r.Origin.Y
	t := a.theme
	for i, item := range a.items {
		h := a.headers[i]
		rect := ggui.Rct(ggui.Pt(r.Origin.X, y), ggui.Sz(r.Size.W, a.heights[i]))
		dst.Describe(rect, h)
		if !item.disabled {
			dst.HitPointer(rect, h)
			dst.HitCursor(rect, ggui.CursorShapePointer)
		}
		if h.Hovered && !item.disabled {
			dst.StrokeLine(ggui.Pt(rect.Origin.X, y+16+h.size.H-1), ggui.Pt(rect.Origin.X+h.size.W, y+16+h.size.H-1), 1, t.Fg)
		}
		paintIcon(dst, a.env, icons.ChevronDown, ggui.Rct(ggui.Pt(rect.Origin.X+rect.Size.W-16, y+(a.heights[i]-16)/2), ggui.Sz(16, 16)), t.MutedFg, h.progress*math.Pi)
		dst.Paint(h.text, ggui.Rct(ggui.Pt(rect.Origin.X, y+16), h.size))
		if i == a.active {
			a.FocusRing(dst, rect, t.Radius, t.Ring)
		}
		y += a.heights[i]
		paintDisclosure(dst, a.env, item.content, ggui.Pt(r.Origin.X, y), r.Size.W, a.sizes[i], h.progress, h.open)
		y += (a.sizes[i].H + 16) * h.progress
		if i < len(a.items)-1 {
			dst.StrokeLine(ggui.Pt(r.Origin.X, y), ggui.Pt(r.Origin.X+r.Size.W, y), 1, t.Border)
		}
	}
}

type accordionHeader struct {
	ggui.Interactive
	owner    *AccordionWidget
	index    int
	text     *ggui.TextWidget
	size     ggui.Size
	reveal   ggui.Motion
	progress float64
	open     bool // as laid out
}

// Describe implements ggui.Describer: a header reports whether its section
// is open, so expand and collapse mean something.
func (h *accordionHeader) Describe() ggui.Node {
	open := h.owner.isOpen(h.index)
	return ggui.Node{
		Role:     ggui.RoleDisclosure,
		Name:     h.SemanticName(),
		Expanded: ggui.Expandable(open),
		Disabled: h.IsInert(),
		Actions:  ggui.ActionPress | ggui.ActionFocus | pick(open, ggui.ActionCollapse, ggui.ActionExpand),
	}
}

// Act implements ggui.Actor.
func (h *accordionHeader) Act(a ggui.Action) bool {
	if h.IsInert() {
		return false
	}
	open := h.owner.isOpen(h.index)
	switch {
	case a.Kind == ggui.ActionExpand && !open, a.Kind == ggui.ActionCollapse && open, a.Kind == ggui.ActionPress:
		h.owner.active = h.index
		h.owner.toggle(h.index)
	case a.Kind == ggui.ActionExpand, a.Kind == ggui.ActionCollapse:
		// Already the way round it was asked to be.
	default:
		return false
	}
	return true
}

func (h *accordionHeader) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind == ggui.PointerDown && ev.Button == ggui.MouseButtonLeft {
		h.owner.active = h.index
	}
	return h.Pointer(ev, func() { h.owner.toggle(h.index) })
}

func (a *AccordionWidget) name() string {
	return pick(a.SemanticName() != "", a.SemanticName(), "Accordion")
}

// Semantics implements ggui.Semantic, including the built-in fallback name.
func (a *AccordionWidget) Semantics() (ggui.Role, string) { return a.Role, a.name() }
