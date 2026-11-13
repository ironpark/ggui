package ui

import (
	"math"
	"slices"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// AccordionSection describes a keyed disclosure and its persistent content.
type AccordionSection struct {
	key, title string
	content    ggui.Widget
	header     ggui.Widget
	disabled   bool
}

// AccordionItem creates one section. Keys must be unique within an accordion.
func AccordionItem(key, title string, content ggui.Widget) AccordionSection {
	return AccordionSection{key: key, title: title, content: content}
}

// Header draws w in the header in place of the title's text, for a section
// headed by an icon and a label, say, or a title with a count beside it:
//
//	ui.AccordionItem("billing", "Billing", body).
//		Header(ggui.Row(ui.Icon(icons.File), ggui.Text("Billing")).Gap(8))
//
// The title still names the section to screen readers and Probe. Text in w
// takes the header's style and color unless it sets its own; the chevron
// stays at the end of the header, level with w's first line.
func (s AccordionSection) Header(w ggui.Widget) AccordionSection { s.header = w; return s }

// Disabled prevents user interaction with the section header.
func (s AccordionSection) Disabled(v bool) AccordionSection { s.disabled = v; return s }

// AccordionWidget gives each enabled header a keyboard tab stop.
// Space/Enter toggle it; arrow navigation is also supported. Open content remains tabbable.
type AccordionWidget struct {
	props property.Owner
	ggui.Interactive
	laidOpen    []string
	open        ggui.Binding[[]string]
	items       []AccordionSection
	headers     []*accordionHeader
	multiple    bool
	active      int
	focusHeader *accordionHeader
	onChange    func([]string)
	theme       *uitheme.Theme
	sizes       []ggui.Size
	heights     []float64
	env         ggui.Env
	line        float64 // one line of a header's text
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
		h := &accordionHeader{owner: a, index: i}
		if item.header == nil {
			h.text = ggui.Text(item.title)
			h.lined = ggui.RichText(ggui.Span(item.title).Underline())
		}
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
	return navConsumes(ev, navVertical) || ggui.Activates(ev)
}

// HandleKey implements ggui.KeyHandler.
func (a *AccordionWidget) HandleKey(ev ggui.KeyEvent) {
	a.Keyboard(ev, func() { a.toggle(a.active) })
	enabled := func(i int) bool { return !a.items[i].disabled }
	if i, ok := navIndex(ev, navVertical, a.active, len(a.items), enabled); ok {
		a.active = i
		a.focusHeader = a.headers[i]
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
	defer a.props.Layout().EndLayout()
	a.env, a.theme = env, uitheme.Of(env)
	a.laidOpen = append(a.laidOpen[:0], a.open.Get()...)
	a.sizes = slices.Grow(a.sizes[:0], len(a.items))[:len(a.items)]
	clear(a.sizes)
	a.heights = slices.Grow(a.heights[:0], len(a.items))[:len(a.items)]
	var width, height float64
	t := a.theme
	chevron := t.IconSize + t.Space // the room the chevron takes beside a header
	// One line of a header, which the chevron is centered on however many
	// lines the header runs to.
	label := env.ResolveText(t.Label)
	a.line = label.Size * label.LineHeight
	for i, item := range a.items {
		h := a.headers[i]
		h.SetKey(struct {
			Group any
			Item  string
		}{a.HitID(), item.key})
		fg := pick(item.disabled, t.Disabled(t.Fg), t.Fg)
		room := ggui.Loose(ggui.Sz(max(c.MaxW-chevron, 0), ggui.Unbounded))
		if h.text != nil {
			h.text.Style(t.Label).Color(fg)
			h.size = h.text.Layout(room, env)
			// The same lines, each underlined as far as it runs.
			h.lined.Style(t.Label).Color(fg)
			h.linedSize = h.lined.Layout(room, env)
		} else {
			h.size = item.header.Layout(room, env.WithText(t.Label.Merge(ggui.TextStyle{Color: fg})))
		}
		// shadcn's py-2.5 inside the trigger's border, which is drawn
		// only as the focus ring but always has its room.
		a.heights[i] = max(a.line, h.size.H) + 2.5*t.Space + 2*t.BorderWidth
		width = max(width, h.size.W+chevron)
		height += a.heights[i]
		h.open = a.isOpen(i)
		// animate-accordion-down and -up run on CSS's ease-out.
		h.progress = h.reveal.Easing(uitheme.EaseOutCSS.Easing()).LayoutToggle(env, h.open, t.MotionSlow)
		if h.open || h.progress > 0 {
			a.sizes[i] = item.content.Layout(ggui.Loose(ggui.Sz(c.MaxW, ggui.Unbounded)), env)
			width = max(width, a.sizes[i].W)
			height += (a.sizes[i].H + 1.25*t.Space) * h.progress
		}
		if i < len(a.items)-1 {
			height += t.BorderWidth // the line under the section, not-last:border-b
		}
	}
	return c.Constrain(ggui.Sz(width, height))
}

// Paint implements ggui.Widget.
func (a *AccordionWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.DescribeNode(r, a, func(dst *ggui.Canvas) { a.paint(dst, r) })
}

func (a *AccordionWidget) paint(dst *ggui.Canvas, r ggui.Rect) {
	// The sections ease by themselves; this is for a binding that is no
	// signal, whose change asks for no layout of its own.
	if !slices.Equal(a.laidOpen, ggui.Untrack(a.open.Get)) {
		ggui.Invalidate(a.env)
	}
	dst = dst.Clip(r)
	if a.focusHeader != nil {
		dst.RequestFocus(a.focusHeader)
		a.focusHeader = nil
	}
	y := r.Origin.Y
	t := a.theme
	for i, item := range a.items {
		h := a.headers[i]
		rect := ggui.Rct(ggui.Pt(r.Origin.X, y), ggui.Sz(r.Size.W, a.heights[i]))
		h.Hit(dst, rect, h, ggui.CursorShapeDefault)
		top := y + t.BorderWidth + 1.25*t.Space // of the header's text
		// The chevron sits level with the header's first line, as shadcn's
		// items-start puts it, which shows once a title wraps.
		first := pick(a.line > 0, a.line, h.size.H)
		paintIconAt(dst, a.env, pick(h.open, icons.ChevronUp, icons.ChevronDown), ggui.Pt(rect.Origin.X+rect.Size.W-t.IconSize/2, top+first/2), t.IconSize, pick(item.disabled, t.Disabled(t.MutedFg), t.MutedFg), 0)
		var face ggui.Widget = h.text
		if h.text == nil {
			face = item.header
		}
		at := ggui.Rct(ggui.Pt(rect.Origin.X, top), h.size)
		dst.Paint(face, at)
		hover := ease(dst, a.theme, h.Anchor(rect), disclosureHoverSlot, pick(h.Hovered && !item.disabled, 1.0, 0.0), a.env.Motion(t.MotionFast))
		if hover > 0 && h.text != nil {
			a.underline(dst, h, at, hover)
		}
		controlFocusRing(dst, &h.Interactive, t, rect, t.Radius, t.Ring)
		y += a.heights[i]
		paintDisclosure(dst, item.content, ggui.Pt(r.Origin.X, y), r.Size.W, a.sizes[i], h.progress, h.open)
		y += (a.sizes[i].H + 1.25*t.Space) * h.progress
		if i < len(a.items)-1 {
			dst.FillRect(ggui.Rct(ggui.Pt(r.Origin.X, y), ggui.Sz(r.Size.W, t.BorderWidth)), t.Border)
			y += t.BorderWidth
		}
	}
}

// underline draws shadcn's hover:underline under a header's title at r,
// hover of the way in: a line under every line of a title that wraps, as
// long as that line, where CSS draws one. The lines are the underlined
// copy's, shown through a band under each baseline, over the title
// already painted.
func (a *AccordionWidget) underline(dst *ggui.Canvas, h *accordionHeader, r ggui.Rect, hover float64) {
	base, ok := h.text.Baseline()
	// The two are the same lines when they are the same size, to within
	// what summing fractional advances twice can differ by.
	same := math.Abs(h.linedSize.W-h.size.W) < .01 && math.Abs(h.linedSize.H-h.size.H) < .01
	if !ok || a.line <= 0 || !same {
		dst.FillRect(ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+r.Size.H-1), ggui.Sz(r.Size.W, 1)), fade(a.theme.Fg, hover))
		return
	}
	lines := int(max(1, math.Round(h.size.H/a.line)))
	bands := func(dst *ggui.Canvas) {
		// A picture of the title: it is described and takes input once,
		// where it was painted.
		dst = dst.Inert()
		for i := range lines {
			band := ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+base+float64(i)*a.line+accordionUnderlineTop), ggui.Sz(r.Size.W, accordionUnderlineBand))
			dst.Clip(band).Paint(h.lined, r)
		}
	}
	if hover < 1 {
		dst.Layer(ggui.LayerOptions{Fade: 1 - hover}, bands)
		return
	}
	bands(dst)
}

// Where under a baseline the band an underline shows through starts, and
// how tall it is: a text's underline is a pixel under its baseline and
// about one thick.
const (
	accordionUnderlineTop  = .75
	accordionUnderlineBand = 2
)

type accordionHeader struct {
	ggui.Interactive
	owner *AccordionWidget
	index int
	text  *ggui.TextWidget
	size  ggui.Size
	// lined is the title again with every line underlined, painted in
	// text's place under the pointer.
	lined     *ggui.RichTextWidget
	linedSize ggui.Size
	reveal    ggui.Motion
	progress  float64
	open      bool // as laid out
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

func (h *accordionHeader) HandleKey(ev ggui.KeyEvent) {
	h.owner.active = h.index
	if navConsumes(ev, navVertical) {
		h.owner.HandleKey(ev)
		return
	}
	h.Keyboard(ev, func() { h.owner.toggle(h.index) })
}
func (h *accordionHeader) ConsumesKey(ev ggui.KeyEvent) bool {
	return navConsumes(ev, navVertical) || ggui.Activates(ev)
}

// Describe exposes the group as a container; its headers own focus actions.
func (a *AccordionWidget) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleAccordion, Name: a.name()}
}
