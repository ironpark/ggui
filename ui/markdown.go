package ui

import (
	"image/color"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/property"
	"github.com/ironpark/ggui/runtime"
	"github.com/ironpark/ggui/ui/icons"
	"github.com/ironpark/ggui/ui/internal/controlkit"
	"github.com/ironpark/ggui/ui/internal/markdown"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

// MarkdownWidget renders Markdown in the theme's typography: headings,
// paragraphs with emphasis, strikethrough, inline code and links, lists
// and task lists, block quotes, fenced code in a CodeBlock, rules and
// tables. Build one with Markdown.
//
// Raw HTML shows as the text it is, and a reference link as written:
// nothing in the source reaches past what it shows.
type MarkdownWidget struct {
	props      property.Owner
	content    property.Value[string]
	streaming  property.Value[bool]
	onLink     func(url string)
	size       float64
	color      color.Color
	selectable bool

	doc  *mdStack
	area *widget.SelectionAreaWidget

	// What was last parsed, and the blocks it made.
	src    string
	stream bool
	parsed bool
	blocks []markdown.Block
	style  mdStyle
	fill   bool // a block spans the width: code, a quote, a table or a rule
	builds int  // the block widgets built, which the tests count
}

// mdStyle is what the blocks' widgets were built for. Colors are theme
// tokens, which follow a theme switch by themselves; a change to these
// builds them again.
type mdStyle struct {
	base       float64 // the body text's size
	mono       *ggui.Font
	selectable bool
}

// Markdown renders s, a Markdown document, as CommonMark reads it with
// GitHub's tables, task lists, strikethrough and autolinks:
//
//	ui.Markdown("# Notes\n\nSee the **guide** at <https://example.com>.")
//
// The text is selectable and copies across blocks, as a page's does. A
// link opens through OnLink, or else in the browser when it is an http,
// https or mailto link. It fills the width it is given when it holds
// code, a quote, a table or a rule, and is as wide as its text otherwise.
func Markdown(s string) *MarkdownWidget {
	m := &MarkdownWidget{selectable: true}
	m.content.Set(s)
	m.doc = &mdStack{m: m}
	m.area = widget.SelectionArea(m.doc)
	return m
}

// Content replaces the source and detaches any BindContent reader.
func (m *MarkdownWidget) Content(s string) *MarkdownWidget {
	property.Set(&m.props, &m.content, s)
	return m
}

// BindContent follows r during layout, as a reply streams into it: a
// change rebuilds only the blocks it changed, which as text is appended is
// the last one, and parses again only from there.
func (m *MarkdownWidget) BindContent(r ggui.Readable[string]) *MarkdownWidget {
	property.Bind(&m.props, &m.content, r, "BindContent")
	return m
}

// Streaming says the source is still arriving, cut off wherever the last
// piece ended: emphasis, a code span or a link opened in the last block
// and not yet closed shows as closed there, rather than as the syntax
// that opened it, until the rest arrives. An unclosed code fence shows
// the code so far either way. Turn it off when the stream ends, so the
// finished text reads as written.
func (m *MarkdownWidget) Streaming(on bool) *MarkdownWidget {
	property.Set(&m.props, &m.streaming, on)
	return m
}

// BindStreaming follows r for Streaming.
func (m *MarkdownWidget) BindStreaming(r ggui.Readable[bool]) *MarkdownWidget {
	property.Bind(&m.props, &m.streaming, r, "BindStreaming")
	return m
}

// OnLink runs fn with the destination of a link that is tapped, whatever
// its scheme, in place of the default, which opens http, https and mailto
// links with runtime.OpenURL and ignores the rest.
func (m *MarkdownWidget) OnLink(fn func(url string)) *MarkdownWidget { m.onLink = fn; return m }

// Size sets the body text's size in pixels; the headings and code scale
// with it. Zero follows the inherited text size, as a Text does.
func (m *MarkdownWidget) Size(px float64) *MarkdownWidget {
	defer property.Watch(&m.props, &m.size)()
	m.size = max(0, px)
	return m
}

// Color sets the body text's color; nil inherits it. Links, code and
// quotes keep the theme's colors.
func (m *MarkdownWidget) Color(c color.Color) *MarkdownWidget {
	defer property.Watch(&m.props, &m.color)()
	m.color = c
	return m
}

// Selectable lets the reader select and copy the text, across blocks;
// it is on by default.
func (m *MarkdownWidget) Selectable(on bool) *MarkdownWidget {
	defer property.Watch(&m.props, &m.selectable)()
	m.selectable = on
	return m
}

// Layout implements Widget.
func (m *MarkdownWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer m.props.Layout().EndLayout()
	t := uitheme.Of(env)
	base := m.size
	if base == 0 {
		base = pick(env.Text().Size > 0, env.Text().Size, t.Text.Size)
	}
	style := mdStyle{base: base, mono: t.Mono.Font, selectable: m.selectable}
	restyled := style != m.style
	if restyled {
		m.style = style
		m.doc.views = nil
	}
	src, stream := m.content.Get(), m.streaming.Get()
	if !m.parsed || src != m.src || stream != m.stream {
		switch {
		case m.parsed && stream == m.stream:
			m.blocks = markdown.Reparse(m.blocks, m.src, src, stream)
		case stream:
			m.blocks = markdown.ParseStream(src)
		default:
			m.blocks = markdown.Parse(src)
		}
		m.src, m.stream, m.parsed = src, stream, true
		m.fill = spansWidth(m.blocks)
		m.doc.sync(m.blocks, false)
	} else if restyled {
		m.doc.sync(m.blocks, false)
	}
	if m.fill && c.MaxW < ggui.Unbounded {
		c.MinW = c.MaxW
	}
	env = env.WithText(ggui.TextStyle{Size: style.base, Color: m.color})
	if m.selectable {
		return m.area.Layout(c, env)
	}
	return m.doc.Layout(c, env)
}

// Paint implements Widget.
func (m *MarkdownWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if m.selectable {
		dst.Paint(m.area, r)
		return
	}
	dst.Paint(m.doc, r)
}

// Baseline implements Baseliner: the first line's.
func (m *MarkdownWidget) Baseline() (float64, bool) { return m.doc.Baseline() }

// spansWidth reports whether any of blocks, or a block inside them, is
// one that spans the width it is given.
func spansWidth(blocks []markdown.Block) bool {
	for _, b := range blocks {
		switch b.Kind {
		case markdown.CodeBlock, markdown.Quote, markdown.Table, markdown.ThematicBreak:
			return true
		}
		if spansWidth(b.Children) {
			return true
		}
	}
	return false
}

// openLink opens url as OnLink says, or as the default does.
func (m *MarkdownWidget) openLink(u string) {
	if m.onLink != nil {
		m.onLink(u)
		return
	}
	if openable(u) {
		_ = runtime.OpenURL(u)
	}
}

// openable reports whether u is a link the default opens: a web address
// or a mail address, never a file, a script or another application's
// scheme a message could name.
func openable(u string) bool {
	p, err := url.Parse(u)
	if err != nil {
		return false
	}
	switch strings.ToLower(p.Scheme) {
	case "http", "https":
		return p.Host != ""
	case "mailto":
		return p.Opaque != ""
	}
	return false
}

// headingScale is a heading's size as a multiple of the body text's.
func headingScale(level int) float64 {
	switch level {
	case 1:
		return 1.6
	case 2:
		return 1.35
	case 3:
		return 1.15
	}
	return 1
}

// mdInline is the style inline content inherits from what it is inside.
type mdInline struct {
	bold, italic, strike, code, link bool
	url, name                        string
}

// spans renders inlines as spans of a paragraph whose text is size pixels.
func (m *MarkdownWidget) spans(inlines []markdown.Inline, size float64) []widget.TextSpan {
	return m.appendSpans(nil, inlines, mdInline{}, size)
}

func (m *MarkdownWidget) appendSpans(out []widget.TextSpan, inlines []markdown.Inline, st mdInline, size float64) []widget.TextSpan {
	for _, in := range inlines {
		s := st
		switch in.Kind {
		case markdown.Text:
			out = append(out, m.span(in.Text, s, size))
		case markdown.CodeSpan:
			s.code = true
			out = append(out, m.span(in.Text, s, size))
		case markdown.SoftBreak:
			out = append(out, m.span(" ", s, size))
		case markdown.HardBreak:
			out = append(out, m.span("\n", s, size))
		case markdown.Emphasis:
			s.italic = true
			out = m.appendSpans(out, in.Children, s, size)
		case markdown.Strong:
			s.bold = true
			out = m.appendSpans(out, in.Children, s, size)
		case markdown.Strike:
			s.strike = true
			out = m.appendSpans(out, in.Children, s, size)
		case markdown.Link, markdown.Image:
			out = m.appendLink(out, in, s, size)
		}
	}
	return out
}

// appendLink appends the spans of a link, or of an image, which shows as a
// link to it named by its description: what it shows is not fetched. One
// with no description reads as its destination, or as "image" without one.
func (m *MarkdownWidget) appendLink(out []widget.TextSpan, in markdown.Inline, st mdInline, size float64) []widget.TextSpan {
	st.link, st.url, st.name = true, in.URL, markdown.PlainText(in.Children)
	children := in.Children
	if len(children) == 0 {
		st.name = pick(in.URL != "", in.URL, "image")
		children = []markdown.Inline{{Kind: markdown.Text, Text: st.name}}
	}
	return m.appendSpans(out, children, st, size)
}

// span is one run of text in the style st says.
func (m *MarkdownWidget) span(text string, st mdInline, size float64) widget.TextSpan {
	sp := widget.Span(text)
	if st.bold {
		sp = sp.Weight(ggui.WeightBold)
	}
	if st.italic {
		sp = sp.Italic()
	}
	if st.strike {
		sp = sp.Strike()
	}
	if st.code {
		sp = sp.Font(m.style.mono).Size(size * .875).Background(uitheme.Muted)
	}
	if st.link {
		sp = sp.Color(uitheme.Primary).Underline()
		// A link still streaming in has no destination yet.
		if u := st.url; u != "" {
			sp = sp.OnTap(func() { m.openLink(u) }).Name(st.name)
		}
	}
	return sp
}

// mdBlock is the widget of one block.
type mdBlock interface {
	ggui.Widget
	// update shows b, a block of the kind the widget was built for, in
	// place of what it showed, and reports false when it cannot.
	update(b markdown.Block) bool
}

// mdView is a block and its widget, in a layout cache: a block that did
// not change is not laid out again.
type mdView struct {
	block markdown.Block
	w     mdBlock
	cache *ggui.CachedWidget
}

// mdStack lays blocks out one under another: the document's, or a quote's
// or a list item's.
type mdStack struct {
	m     *MarkdownWidget
	props property.Owner
	views []mdView
	rects []ggui.Rect
	tight bool // a tight list item's: its blocks closer together
	depth int  // how many lists it is inside
	rtl   bool
}

// sync shows blocks: a block equal to the one shown at its place keeps its
// widget untouched, one of the same kind updates it, and only one of
// another kind builds a new one. As a reply streams in, that is its last
// block alone.
func (s *mdStack) sync(blocks []markdown.Block, tight bool) {
	if s.tight != tight {
		s.tight = tight
		s.props.Changed()
	}
	for i, b := range blocks {
		if i < len(s.views) {
			v := &s.views[i]
			if markdown.Equal(v.block, b) {
				v.block = b
				continue
			}
			if v.block.Kind == b.Kind && v.w.update(b) {
				v.block = b
				continue
			}
		}
		w := s.build(b)
		v := mdView{block: b, w: w, cache: ggui.Cached(w)}
		if i < len(s.views) {
			s.views[i] = v
		} else {
			s.views = append(s.views, v)
		}
		s.props.Changed()
	}
	if len(s.views) > len(blocks) {
		clear(s.views[len(blocks):])
		s.views = s.views[:len(blocks)]
		s.props.Changed()
	}
}

// build makes the widget of b.
func (s *mdStack) build(b markdown.Block) mdBlock {
	m := s.m
	m.builds++
	var w mdBlock
	switch b.Kind {
	case markdown.CodeBlock:
		w = mdCode{CodeBlock("").Height(0)}
	case markdown.ThematicBreak:
		w = mdRule{Divider()}
	case markdown.Quote:
		w = &mdQuote{m: m, body: &mdStack{m: m, depth: s.depth}}
	case markdown.List:
		w = &mdList{m: m, depth: s.depth}
	case markdown.Table:
		w = &mdTable{m: m}
	default:
		w = &mdText{m: m}
	}
	w.update(b)
	return w
}

// gap is the space above block i.
func (s *mdStack) gap(i int) float64 {
	base := s.m.style.base
	prev, cur := s.views[i-1].block, s.views[i].block
	switch {
	case cur.Kind == markdown.Heading:
		return base * pick(cur.Level <= 2, 1.5, 1.0)
	case prev.Kind == markdown.Heading:
		return base * .5
	case s.tight:
		return base * .25
	}
	return base * .75
}

// Layout implements Widget.
func (s *mdStack) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer s.props.Layout().EndLayout()
	s.rtl = env.Direction() == ggui.DirRTL
	s.rects = s.rects[:0]
	cs := ggui.Constraints{MinW: c.MinW, MaxW: c.MaxW, MaxH: ggui.Unbounded}
	y, w := 0.0, c.MinW
	for i := range s.views {
		if i > 0 {
			y += s.gap(i)
		}
		sz := s.views[i].cache.Layout(cs, env)
		s.rects = append(s.rects, ggui.Rct(ggui.Pt(0, y), sz))
		y += sz.H
		w = max(w, sz.W)
	}
	if s.rtl {
		for i := range s.rects {
			s.rects[i].Origin.X = w - s.rects[i].Size.W
		}
	}
	return c.Constrain(ggui.Sz(w, y))
}

// Paint implements Widget.
func (s *mdStack) Paint(dst *ggui.Canvas, r ggui.Rect) {
	for i := range s.views {
		rc := s.rects[i]
		dst.Paint(s.views[i].cache, ggui.Rct(r.Origin.Add(rc.Origin), rc.Size))
	}
}

// Baseline implements Baseliner: the first block's first line.
func (s *mdStack) Baseline() (float64, bool) {
	if len(s.views) == 0 || len(s.rects) == 0 {
		return 0, false
	}
	b, ok := ggui.BaselineOf(s.views[0].w)
	return s.rects[0].Origin.Y + b, ok
}

// mdText is a paragraph, or a heading of a level.
type mdText struct {
	m     *MarkdownWidget
	props property.Owner
	level int // 0 for a paragraph
	text  *widget.RichTextWidget
	name  string
}

func (t *mdText) update(b markdown.Block) bool {
	if t.text == nil {
		t.text = widget.RichText()
		if t.m.style.selectable {
			t.text.Selectable()
		}
	}
	if t.level != b.Level {
		t.level = b.Level
		t.props.Changed()
	}
	t.text.Spans(t.m.spans(b.Inlines, t.m.style.base*headingScale(b.Level))...)
	t.name = markdown.PlainText(b.Inlines)
	return true
}

func (t *mdText) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer t.props.Layout().EndLayout()
	if t.level > 0 {
		t.text.Size(t.m.style.base * headingScale(t.level)).Weight(ggui.WeightSemibold).LineHeight(1.25).
			Color(pick[color.Color](t.level == 6, uitheme.MutedFg, nil))
	}
	return t.text.Layout(c, env)
}

func (t *mdText) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if t.level == 0 {
		dst.Paint(t.text, r)
		return
	}
	dst.Node(r, ggui.Node{Role: ggui.RoleHeading, Name: t.name, Level: t.level}, func(dst *ggui.Canvas) { dst.Paint(t.text, r) })
}

// Baseline implements Baseliner.
func (t *mdText) Baseline() (float64, bool) { return t.text.Baseline() }

// mdCode is a code block: a CodeBlock labelled with its language, as tall
// as its code, which follows the source as it streams in.
type mdCode struct{ *CodeBlockWidget }

func (c mdCode) update(b markdown.Block) bool {
	c.Code(b.Code).Language(b.Lang())
	return true
}

// mdRule is a thematic break, which a screen reader reads as one.
type mdRule struct{ *DividerWidget }

func (mdRule) update(markdown.Block) bool { return true }

func (d mdRule) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Leaf(r, ggui.Node{Role: ggui.RoleSeparator})
	d.DividerWidget.Paint(dst, r)
}

// mdQuote is a block quote: its blocks in the muted color, inset from a
// bar at the start edge.
type mdQuote struct {
	m     *MarkdownWidget
	props property.Owner
	body  *mdStack
	theme *uitheme.Theme
	bar   float64
	inset float64
	rtl   bool
	words *Strings // from Layout, for the node Paint describes
}

func (q *mdQuote) update(b markdown.Block) bool {
	q.body.sync(b.Children, false)
	return true
}

func (q *mdQuote) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer q.props.Layout().EndLayout()
	q.theme = uitheme.Of(env)
	q.rtl = env.Direction() == ggui.DirRTL
	q.words = stringsOf(env)
	q.bar = max(2, 2*q.theme.BorderWidth)
	q.inset = q.bar + q.m.style.base*.85
	sz := q.body.Layout(inset(c, q.inset), env.WithText(ggui.TextStyle{Color: uitheme.MutedFg}))
	return c.Constrain(ggui.Sz(sz.W+q.inset, sz.H))
}

func (q *mdQuote) Paint(dst *ggui.Canvas, r ggui.Rect) {
	bar := ggui.Rct(r.Origin, ggui.Sz(q.bar, r.Size.H))
	body := ggui.Rct(r.Origin.Add(ggui.Pt(q.inset, 0)), ggui.Sz(r.Size.W-q.inset, r.Size.H))
	if q.rtl {
		bar.Origin.X = r.Origin.X + r.Size.W - q.bar
		body.Origin.X = r.Origin.X
	}
	dst.FillRoundRect(bar, q.bar/2, q.theme.Border)
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup, Description: q.words.BlockQuote}, func(dst *ggui.Canvas) { dst.Paint(q.body, body) })
}

// inset takes w off the widths c allows, for content set in by it.
func inset(c ggui.Constraints, w float64) ggui.Constraints {
	return ggui.Constraints{MinW: max(0, c.MinW-w), MaxW: max(0, c.MaxW-w), MaxH: ggui.Unbounded}
}

// mdList is a bulleted or numbered list, its items' markers in a column
// at the start edge: bullets that change shape with nesting, numbers, or a
// task list item's check box.
type mdList struct {
	m       *MarkdownWidget
	props   property.Owner
	depth   int
	ordered bool
	number  int
	tight   bool
	items   []*mdItem

	theme *uitheme.Theme
	env   ggui.Env
	ink   color.Color // the bullets'
	markW float64
	inset float64
	lineH float64
	rtl   bool
}

// mdItem is one item of a list.
type mdItem struct {
	body *mdStack
	task markdown.Task
	mark *widget.TextWidget // an ordered item's number
	name string             // a task's text, which names its check box
	rect ggui.Rect          // the body's, in the list
	mSz  ggui.Size
}

func (l *mdList) update(b markdown.Block) bool {
	if l.ordered != b.Ordered || l.number != b.Number || l.tight != b.Tight {
		l.ordered, l.number, l.tight = b.Ordered, b.Number, b.Tight
		l.props.Changed()
	}
	for i, ib := range b.Children {
		if i == len(l.items) {
			l.items = append(l.items, &mdItem{body: &mdStack{m: l.m, depth: l.depth + 1}, mark: widget.Text("").NoWrap()})
			l.props.Changed()
		}
		it := l.items[i]
		if it.task != ib.Task {
			it.task = ib.Task
			l.props.Changed()
		}
		it.body.sync(ib.Children, b.Tight)
		it.name = ""
		if ib.Task != markdown.NoTask && len(ib.Children) > 0 {
			it.name = markdown.PlainText(ib.Children[0].Inlines)
		}
		it.mark.Content(strconv.Itoa(b.Number+i) + ".")
	}
	if len(l.items) > len(b.Children) {
		clear(l.items[len(b.Children):])
		l.items = l.items[:len(b.Children)]
		l.props.Changed()
	}
	return true
}

// checkSide is a task's check box's side.
func (l *mdList) checkSide() float64 { return min(l.theme.ControlSize, l.m.style.base*1.1) }

func (l *mdList) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer l.props.Layout().EndLayout()
	t := uitheme.Of(env)
	l.theme, l.env = t, env
	l.rtl = env.Direction() == ggui.DirRTL
	base := l.m.style.base
	l.ink = env.ResolveText(ggui.TextStyle{}).Color
	l.lineH = base * pick(t.Text.LineHeight > 0, t.Text.LineHeight, 1.2)
	l.markW = base * .5
	loose := ggui.Loose(ggui.Sz(ggui.Unbounded, ggui.Unbounded))
	for _, it := range l.items {
		if it.task != markdown.NoTask {
			l.markW = max(l.markW, l.checkSide())
		} else if l.ordered {
			it.mSz = it.mark.Layout(loose, env)
			l.markW = max(l.markW, it.mSz.W)
		}
	}
	l.inset = max(base*1.25, l.markW+base*.5)
	cs := inset(c, l.inset)
	y, w := 0.0, c.MinW
	for i, it := range l.items {
		if i > 0 {
			y += base * pick(l.tight, .25, .75)
		}
		sz := it.body.Layout(cs, env)
		sz.H = max(sz.H, l.lineH)
		it.rect = ggui.Rct(ggui.Pt(l.inset, y), sz)
		y += sz.H
		w = max(w, sz.W+l.inset)
	}
	if l.rtl {
		for _, it := range l.items {
			it.rect.Origin.X = w - l.inset - it.rect.Size.W
		}
	}
	return c.Constrain(ggui.Sz(w, y))
}

func (l *mdList) Paint(dst *ggui.Canvas, r ggui.Rect) {
	n := float64(len(l.items))
	dst.Node(r, ggui.Node{Role: ggui.RoleList, Max: n}, func(dst *ggui.Canvas) {
		for i, it := range l.items {
			row := ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+it.rect.Origin.Y), ggui.Sz(r.Size.W, it.rect.Size.H))
			dst.Node(row, ggui.Node{Role: ggui.RoleListItem, Min: 1, Now: float64(i + 1), Max: n}, func(dst *ggui.Canvas) {
				l.paintMark(dst, r, it)
				dst.Paint(it.body, ggui.Rct(r.Origin.Add(it.rect.Origin), it.rect.Size))
			})
		}
	})
}

// paintMark paints an item's marker, its middle on the middle of the
// item's first line of text.
func (l *mdList) paintMark(dst *ggui.Canvas, r ggui.Rect, it *mdItem) {
	base := l.m.style.base
	baseline, ok := it.body.Baseline()
	if !ok {
		baseline = l.lineH * .75
	}
	top := r.Origin.Y + it.rect.Origin.Y
	mid := top + baseline - base*.33
	// x places a marker w wide at the end of the marker column, toward the
	// content, on whichever side the content starts.
	x := func(w float64) float64 {
		if l.rtl {
			return r.Origin.X + r.Size.W - l.inset + base*.5
		}
		return r.Origin.X + l.inset - base*.5 - w
	}
	switch {
	case it.task != markdown.NoTask:
		side := l.checkSide()
		box := ggui.Rct(ggui.Pt(x(side), mid-side/2), ggui.Sz(side, side))
		done := it.task == markdown.TaskDone
		t := l.theme
		radius := side / 4
		if done {
			dst.FillRoundRect(box, radius, t.Primary)
			controlkit.PaintIconAt(dst, l.env, controlkit.IconMark{Role: icons.Check, Center: box.Center(), Size: side * .875, Color: t.PrimaryFg})
		} else {
			dst.FillRoundRect(box, radius, t.Input)
			dst.StrokeRoundRect(box, radius, t.BorderWidth, t.InputBorder)
		}
		// Read only: it says what the source says, and nothing changes it.
		dst.Leaf(box, ggui.Node{Role: ggui.RoleCheckbox, Name: it.name, Checked: ggui.Tri(done)})
	case l.ordered:
		mb, _ := it.mark.Baseline()
		dst.Paint(it.mark, ggui.Rct(ggui.Pt(x(it.mSz.W), top+baseline-mb), it.mSz))
	default:
		d := base * .36
		c := ggui.Pt(x(d)+d/2, mid)
		switch l.depth % 3 {
		case 0:
			dst.FillCircle(c, d/2, l.ink)
		case 1:
			dst.StrokeRoundRect(ggui.Rct(ggui.Pt(c.X-d/2, c.Y-d/2), ggui.Sz(d, d)), d/2, max(1, d/5), l.ink)
		default:
			dst.FillRect(ggui.Rct(ggui.Pt(c.X-d/2, c.Y-d/2), ggui.Sz(d, d)), l.ink)
		}
	}
}

// mdTable is a table: a grid of selectable cells, the heading row in the
// label weight, every other row on the muted surface. Columns are as
// wide as their widest cell when they fit; otherwise the wider ones give
// up width and their cells wrap.
type mdTable struct {
	m     *MarkdownWidget
	props property.Owner
	align []markdown.Align
	cells [][]*widget.RichTextWidget // the heading row first
	names [][]string
	theme *uitheme.Theme
	colW  []float64
	rowH  []float64
	padX  float64
	padY  float64
	width float64
	rtl   bool
}

func (t *mdTable) update(b markdown.Block) bool {
	if t.align != nil && !slices.Equal(t.align, b.Align) {
		return false // a column aligned another way: built again
	}
	t.align = b.Align
	all := append([][]markdown.Cell{b.Header}, b.Rows...)
	if len(all) != len(t.cells) {
		t.props.Changed()
	}
	for r, row := range all {
		if r == len(t.cells) {
			t.cells = append(t.cells, t.newRow(r, b.Align))
			t.names = append(t.names, make([]string, len(b.Align)))
		}
		for c, cell := range row {
			t.cells[r][c].Spans(t.m.spans(cell, t.m.style.base)...)
			t.names[r][c] = markdown.PlainText(cell)
		}
	}
	if len(all) < len(t.cells) {
		clear(t.cells[len(all):])
		clear(t.names[len(all):])
	}
	t.cells, t.names = t.cells[:len(all)], t.names[:len(all)]
	return true
}

// newRow makes the cells of row r, aligned as align says: selectable as
// the document is, and in the label weight in the heading row, row 0.
func (t *mdTable) newRow(r int, align []markdown.Align) []*widget.RichTextWidget {
	cells := make([]*widget.RichTextWidget, len(align))
	for c := range cells {
		cells[c] = widget.RichText()
		if t.m.style.selectable {
			cells[c].Selectable()
		}
		switch align[c] {
		case markdown.AlignCenter:
			cells[c].Align(.5)
		case markdown.AlignRight:
			cells[c].Align(1)
		case markdown.AlignLeft:
			cells[c].Align(0)
		}
		if r == 0 {
			cells[c].Weight(ggui.WeightSemibold)
		}
	}
	return cells
}

func (t *mdTable) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer t.props.Layout().EndLayout()
	th := uitheme.Of(env)
	t.theme = th
	t.rtl = env.Direction() == ggui.DirRTL
	base := t.m.style.base
	t.padX, t.padY = base*.75, base*.4
	n := len(t.align)
	t.colW = resized(t.colW, n)
	need := make([]float64, n)
	loose := ggui.Loose(ggui.Sz(ggui.Unbounded, ggui.Unbounded))
	for _, row := range t.cells {
		for i, cell := range row {
			need[i] = max(need[i], cell.Layout(loose, env).W)
		}
	}
	total := 0.0
	for _, w := range need {
		total += w
	}
	avail := c.MaxW
	// Columns that will not fit keep less padding, for more of their text.
	if total+float64(n)*2*t.padX > avail {
		t.padX = base * .4
	}
	for i := range need {
		need[i] += 2 * t.padX
	}
	total += float64(n) * 2 * t.padX
	copy(t.colW, need)
	if total > avail {
		t.shrinkColumns(need, avail)
	}
	t.width = 0
	for _, w := range t.colW {
		t.width += w
	}
	t.rowH = resized(t.rowH, len(t.cells))
	for r, row := range t.cells {
		h := 0.0
		for i, cell := range row {
			w := max(0, t.colW[i]-2*t.padX)
			h = max(h, cell.Layout(ggui.Constraints{MinW: w, MaxW: w, MaxH: ggui.Unbounded}, env).H)
		}
		t.rowH[r] = h + 2*t.padY
	}
	h := 0.0
	for _, rh := range t.rowH {
		h += rh
	}
	return c.Constrain(ggui.Sz(t.width, h))
}

// shrinkColumns narrows the columns, which need would make wider than
// avail. Each column keeps up to a few words' width; what is left goes to
// the columns in proportion to what more they would take.
func (t *mdTable) shrinkColumns(need []float64, avail float64) {
	base, n := t.m.style.base, len(need)
	floor := make([]float64, n)
	least, want := 0.0, 0.0
	for i, w := range need {
		floor[i] = min(w, base*6+2*t.padX)
		least += floor[i]
		want += w - floor[i]
	}
	spare := avail - least
	for i := range t.colW {
		switch {
		case spare <= 0:
			t.colW[i] = max(0, avail) / float64(n)
		case want > 0:
			t.colW[i] = floor[i] + spare*(need[i]-floor[i])/want
		default:
			t.colW[i] = floor[i]
		}
	}
}

// resized is s with n elements.
func resized(s []float64, n int) []float64 {
	if cap(s) < n {
		return make([]float64, n)
	}
	return s[:n]
}

func (t *mdTable) Paint(dst *ggui.Canvas, r ggui.Rect) {
	th := t.theme
	grid := ggui.Rct(r.Origin, ggui.Sz(min(t.width, r.Size.W), r.Size.H))
	if t.rtl {
		grid.Origin.X = r.Origin.X + r.Size.W - grid.Size.W
	}
	rows := float64(len(t.cells))
	dst.Node(grid, ggui.Node{Role: ggui.RoleTable}, func(dst *ggui.Canvas) {
		y := grid.Origin.Y
		for ri, row := range t.cells {
			rowRect := ggui.Rct(ggui.Pt(grid.Origin.X, y), ggui.Sz(grid.Size.W, t.rowH[ri]))
			if ri > 0 && ri%2 == 0 {
				dst.FillRect(rowRect, th.Muted)
			}
			dst.Node(rowRect, ggui.Node{Role: ggui.RoleRow, Min: 1, Now: float64(ri + 1), Max: rows}, func(dst *ggui.Canvas) {
				x := 0.0
				for ci, cell := range row {
					w := t.colW[ci]
					at := grid.Origin.X + x
					if t.rtl {
						at = grid.Origin.X + grid.Size.W - x - w
					}
					box := ggui.Rct(ggui.Pt(at, y), ggui.Sz(w, t.rowH[ri]))
					inner := ggui.Rct(box.Origin.Add(ggui.Pt(t.padX, t.padY)), ggui.Sz(max(0, w-2*t.padX), max(0, t.rowH[ri]-2*t.padY)))
					dst.Node(box, ggui.Node{Role: pick(ri == 0, ggui.RoleHeader, ggui.RoleCell), Name: t.names[ri][ci]}, func(dst *ggui.Canvas) {
						dst.Paint(cell, inner)
					})
					x += w
				}
			})
			y += t.rowH[ri]
		}
	})
	// The grid's lines: around it, under every row, between the columns.
	bw := th.BorderWidth
	dst.StrokeRoundRect(grid, 0, bw, th.Border)
	y := grid.Origin.Y
	for _, h := range t.rowH[:max(0, len(t.rowH)-1)] {
		y += h
		dst.FillRect(ggui.Rct(ggui.Pt(grid.Origin.X, y-bw/2), ggui.Sz(grid.Size.W, bw)), th.Border)
	}
	x := 0.0
	for _, w := range t.colW[:max(0, len(t.colW)-1)] {
		x += w
		at := pick(t.rtl, grid.Origin.X+grid.Size.W-x, grid.Origin.X+x)
		dst.FillRect(ggui.Rct(ggui.Pt(at-bw/2, grid.Origin.Y), ggui.Sz(bw, grid.Size.H)), th.Border)
	}
}
