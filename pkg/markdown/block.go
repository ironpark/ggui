package markdown

import "strings"

// The block parser is CommonMark's: each line first continues the blocks
// still open, from the document down, then may start new ones, and what is
// left of it is text for the innermost. A paragraph left open by a line
// that continued fewer blocks takes the line as a lazy continuation.

// maxDepth is how deep quotes and lists nest. Past it a '>' or a list
// marker is text: a source of ten thousand '>' would otherwise be ten
// thousand nested widgets, for nothing a reader could tell apart.
const maxDepth = 32

// document is the kind of the root node, which no Block has.
const document Kind = 0

// node is a block as the parser builds it, before it is a Block.
type node struct {
	kind     Kind
	parent   *node
	children []*node
	open     bool

	// The lines the block spans, as indexes into the parser's lines.
	start, end int

	lines []string // a paragraph's, a code block's or a table's, as they came

	level int // a heading's

	fenced      bool
	fenceChar   byte
	fenceLen    int
	fenceIndent int
	info        string
	closed      bool // a fence's closing line came

	list  listData // a list's or an item's
	tight bool

	align []Align // a table's
}

// listData is what a list marker says: what kind of list, and where an
// item's content starts.
type listData struct {
	ordered      bool
	marker       byte // '-', '+' or '*' for a bullet, '.' or ')' after a number
	number       int
	markerOffset int // the columns of indent before the marker
	padding      int // the marker's width and the spaces after it
}

// parser holds the state of a parse: the document so far, and the line
// being read with the position in it.
type parser struct {
	src        string
	starts     []int // each line's first byte in src
	ends       []int // each line's end in src, before its line ending
	lastOpen   bool  // the source ends without a line ending
	stream     bool
	doc, tip   *node
	oldtip     *node
	matched    *node // the last container the line continued
	allClosed  bool
	line       int  // the index of the line being read
	consumed   bool // a block start took the rest of the line
	ln         string
	off, col   int
	partial    bool // the tab at off is partly consumed: col is inside it
	next       int  // where the next non-space character is
	nextCol    int
	indent     int
	indented   bool
	blank      bool
	inlineTail *node // under ParseStream, the leaf the source ends in
}

func parse(src string, stream bool) []Block {
	p := &parser{src: src, stream: stream}
	p.doc = &node{kind: document, open: true}
	p.tip = p.doc
	for i := 0; i <= len(src); {
		j := i
		for j < len(src) && src[j] != '\n' && src[j] != '\r' {
			j++
		}
		if j == len(src) && i == len(src) && i > 0 {
			break // the line ending of the last line ends the source
		}
		p.starts, p.ends = append(p.starts, i), append(p.ends, j)
		ln := src[i:j]
		if strings.IndexByte(ln, 0) >= 0 {
			ln = strings.ReplaceAll(ln, "\x00", "�")
		}
		p.lastOpen = j == len(src)
		p.incorporate(ln)
		p.line++
		switch {
		case j == len(src):
			i = j + 1
		case src[j] == '\r' && j+1 < len(src) && src[j+1] == '\n':
			i = j + 2
		default:
			i = j + 1
		}
	}
	// Only a block still open where the source ends can be cut off inside
	// it: one a blank line closed is whole.
	if leaf := lastLeaf(p.doc); stream && leaf.open {
		p.inlineTail = leaf
	}
	for p.tip != nil {
		p.finalize(p.tip)
	}
	return p.blocks(p.doc.children)
}

// lastLeaf is the last block of n with no blocks inside.
func lastLeaf(n *node) *node {
	for len(n.children) > 0 {
		n = n.children[len(n.children)-1]
	}
	return n
}

// peek returns the byte at i of the line, or 0 past its end.
func (p *parser) peek(i int) byte {
	if i < len(p.ln) {
		return p.ln[i]
	}
	return 0
}

// findNext finds the next non-space character from the position, and the
// indent before it in columns.
func (p *parser) findNext() {
	i, cols := p.off, p.col
	for i < len(p.ln) {
		switch p.ln[i] {
		case ' ':
			cols++
		case '\t':
			cols += 4 - cols%4
		default:
			goto found
		}
		i++
	}
found:
	p.next, p.nextCol = i, cols
	p.indent = cols - p.col
	p.indented = p.indent >= 4
	p.blank = i == len(p.ln)
}

// advanceNext moves the position to the next non-space character.
func (p *parser) advanceNext() {
	p.off, p.col, p.partial = p.next, p.nextCol, false
}

// advance moves the position count characters on, or with columns, count
// columns: a tab is as wide as the way to the next multiple of four, and
// may be consumed in part.
func (p *parser) advance(count int, columns bool) {
	for count > 0 && p.off < len(p.ln) {
		if p.ln[p.off] == '\t' {
			w := 4 - p.col%4
			if columns {
				if w > count {
					p.partial = true
					p.col += count
					return
				}
				p.partial = false
				p.col += w
				p.off++
				count -= w
				continue
			}
			p.partial = false
			p.col += w
			p.off++
			count--
			continue
		}
		p.partial = false
		p.off++
		p.col++
		count--
	}
}

// rest is the line from the position on, a partly consumed tab as the
// spaces left of it.
func (p *parser) rest() string {
	if p.off >= len(p.ln) {
		return ""
	}
	if p.partial {
		return strings.Repeat(" ", 4-p.col%4) + p.ln[p.off+1:]
	}
	return p.ln[p.off:]
}

// incorporate reads one line into the document.
func (p *parser) incorporate(ln string) {
	p.ln, p.off, p.col, p.partial, p.consumed = ln, 0, 0, false, false
	container := p.doc
	p.oldtip = p.tip
	// Continue the open blocks the line still belongs to.
open:
	for {
		n := len(container.children)
		if n == 0 || !container.children[n-1].open {
			break
		}
		container = container.children[n-1]
		p.findNext()
		switch p.continues(container) {
		case 1:
			container = container.parent
			break open
		case 2:
			return // a closing fence: nothing more on the line
		}
	}
	p.allClosed = container == p.oldtip
	p.matched = container
	// Start new blocks in what remains.
	leaf := container.kind != Paragraph && container.kind != Table && acceptsLines(container.kind)
	for !leaf {
		p.findNext()
		res := p.blockStart(container)
		if res == 0 {
			p.advanceNext()
			break
		}
		container = p.tip
		leaf = res == 2
	}
	if p.consumed {
		return
	}
	// What remains is text: a lazy continuation of an open paragraph, or a
	// line of the innermost block, or a new paragraph.
	if !p.allClosed && !p.blank && p.tip.kind == Paragraph {
		p.addLine()
		return
	}
	p.closeUnmatched()
	switch {
	case acceptsLines(container.kind):
		p.addLine()
	case p.off < len(p.ln) && !p.blank:
		p.addChild(Paragraph)
		p.advanceNext()
		p.addLine()
	}
}

// acceptsLines reports whether a block of kind k takes text lines.
func acceptsLines(k Kind) bool { return k == Paragraph || k == CodeBlock || k == Table }

// continues tries the line on an open block: 0 when it continues it, 1
// when it does not, 2 when it ends it and is used up.
func (p *parser) continues(c *node) int {
	switch c.kind {
	case Quote:
		if !p.indented && p.peek(p.next) == '>' {
			p.advanceNext()
			p.advance(1, false)
			if b := p.peek(p.off); b == ' ' || b == '\t' {
				p.advance(1, true)
			}
			c.end = p.line
			return 0
		}
		return 1
	case Item:
		if p.blank {
			if len(c.children) == 0 {
				return 1 // an item starts with at most one blank line
			}
			p.advanceNext()
			return 0
		}
		if p.indent >= c.list.markerOffset+c.list.padding {
			p.advance(c.list.markerOffset+c.list.padding, true)
			return 0
		}
		return 1
	case Heading, ThematicBreak:
		return 1
	case CodeBlock:
		if !c.fenced {
			switch {
			case p.indent >= 4:
				p.advance(4, true)
				return 0
			case p.blank:
				p.advanceNext()
				return 0
			}
			return 1
		}
		if !p.indented && p.peek(p.next) == c.fenceChar {
			s := p.ln[p.next:]
			n := run(s, c.fenceChar)
			if n >= c.fenceLen && strings.Trim(s[n:], " \t") == "" {
				c.closed = true
				c.end = p.line
				p.finalize(c)
				return 2
			}
		}
		for i := c.fenceIndent; i > 0; i-- {
			if b := p.peek(p.off); b != ' ' && b != '\t' {
				break
			}
			p.advance(1, true)
		}
		return 0
	case Paragraph, Table:
		if p.blank {
			return 1
		}
	}
	return 0
}

// run counts the bytes c that s starts with.
func run(s string, c byte) int {
	n := 0
	for n < len(s) && s[n] == c {
		n++
	}
	return n
}

// depth counts the containers n is inside.
func depth(n *node) int {
	d := 0
	for ; n != nil; n = n.parent {
		d++
	}
	return d
}

// blockStart tries each block start at the position: 0 when none starts, 1
// when a container did, which more may start inside, 2 when a leaf did.
func (p *parser) blockStart(container *node) int {
	s := p.ln[p.next:]
	var c byte
	if len(s) > 0 {
		c = s[0]
	}
	if !p.indented {
		switch {
		case c == '>' && depth(container) < maxDepth:
			p.advanceNext()
			p.advance(1, false)
			if b := p.peek(p.off); b == ' ' || b == '\t' {
				p.advance(1, true)
			}
			p.closeUnmatched()
			p.addChild(Quote)
			return 1
		case c == '#':
			if n := run(s, '#'); n <= 6 && (n == len(s) || s[n] == ' ' || s[n] == '\t') {
				p.closeUnmatched()
				h := p.addChild(Heading)
				h.level = n
				h.lines = []string{headingText(s[n:])}
				return p.useLine()
			}
		case c == '`' || c == '~':
			if n := run(s, c); n >= 3 && (c == '~' || strings.IndexByte(s[n:], '`') < 0) {
				p.closeUnmatched()
				f := p.addChild(CodeBlock)
				f.fenced, f.fenceChar, f.fenceLen, f.fenceIndent = true, c, n, p.indent
				f.info = unescape(strings.Trim(s[n:], " \t"))
				return p.useLine()
			}
		}
		if (c == '=' || c == '-') && container.kind == Paragraph {
			if n := run(s, c); strings.Trim(s[n:], " \t") == "" {
				p.closeUnmatched()
				h := &node{kind: Heading, parent: container.parent, open: true, start: container.start, end: p.line, level: 2}
				if c == '=' {
					h.level = 1
				}
				h.lines = []string{strings.TrimRight(strings.Join(container.lines, "\n"), " \t")}
				siblings := container.parent.children
				siblings[len(siblings)-1] = h
				p.tip = h
				return p.useLine()
			}
		}
		if isBreak(s) {
			p.closeUnmatched()
			p.addChild(ThematicBreak)
			return p.useLine()
		}
		if container.kind == Paragraph && p.tableStart(container, s) {
			return p.useLine()
		}
	}
	if (!p.indented || container.kind == List) && depth(container) < maxDepth {
		if data, ok := p.listMarker(container); ok {
			p.closeUnmatched()
			if p.tip.kind != List || p.tip.list.ordered != data.ordered || p.tip.list.marker != data.marker {
				l := p.addChild(List)
				l.list = data
			}
			p.addChild(Item).list = data
			return 1
		}
	}
	if p.indented && p.tip.kind != Paragraph && !p.blank {
		p.advance(4, true)
		p.closeUnmatched()
		p.addChild(CodeBlock)
		return 2
	}
	return 0
}

// useLine marks the rest of the line as taken by the block just started.
func (p *parser) useLine() int {
	p.off, p.partial, p.consumed = len(p.ln), false, true
	return 2
}

// headingText is an ATX heading's text: s, after its #s, without the
// closing sequence of #s.
func headingText(s string) string {
	s = strings.TrimRight(s, " \t")
	body := strings.TrimRight(s, "#")
	if body == "" {
		return ""
	}
	if len(body) < len(s) && (body[len(body)-1] == ' ' || body[len(body)-1] == '\t') {
		s = body
	}
	return strings.Trim(s, " \t")
}

// isBreak reports whether s is a thematic break: three or more *, - or _,
// and spaces or tabs between them.
func isBreak(s string) bool {
	if len(s) == 0 || (s[0] != '*' && s[0] != '-' && s[0] != '_') {
		return false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case s[0]:
			n++
		case ' ', '\t':
		default:
			return false
		}
	}
	return n >= 3
}

// tableStart turns the last line of paragraph into the heading row of a
// table, when s is a delimiter row with as many cells as it has. Lines of
// the paragraph before it stay a paragraph.
func (p *parser) tableStart(para *node, s string) bool {
	if len(para.lines) == 0 || para.end != p.line-1 {
		return false
	}
	head := para.lines[len(para.lines)-1]
	if strings.IndexByte(s, '|') < 0 && strings.IndexByte(head, '|') < 0 {
		return false
	}
	align, ok := delimiterRow(s)
	if !ok {
		return false
	}
	cells := len(splitRow(head))
	// A stream cut off inside its delimiter row has its first cells; the
	// rest are on their way.
	partial := p.stream && p.lastOpen && len(align) < cells
	if len(align) != cells && !partial {
		return false
	}
	for len(align) < cells {
		align = append(align, AlignNone)
	}
	p.closeUnmatched()
	parent := para.parent
	if len(para.lines) > 1 {
		para.lines = para.lines[:len(para.lines)-1]
		para.end--
		p.finalize(para)
	} else {
		parent.children = parent.children[:len(parent.children)-1]
		p.tip = parent
	}
	t := p.addChild(Table)
	t.start = p.line - 1
	t.align = align
	t.lines = []string{head}
	return true
}

// delimiterRow reads s as a table's delimiter row: a cell of dashes per
// column, a colon at an end aligning the column to it.
func delimiterRow(s string) ([]Align, bool) {
	cells := splitRow(s)
	align := make([]Align, 0, len(cells))
	for _, c := range cells {
		c = strings.Trim(c, " \t")
		left, right := strings.HasPrefix(c, ":"), strings.HasSuffix(c, ":")
		dashes := strings.TrimSuffix(strings.TrimPrefix(c, ":"), ":")
		if dashes == "" || strings.Trim(dashes, "-") != "" {
			return nil, false
		}
		switch {
		case left && right:
			align = append(align, AlignCenter)
		case left:
			align = append(align, AlignLeft)
		case right:
			align = append(align, AlignRight)
		default:
			align = append(align, AlignNone)
		}
	}
	return align, len(align) > 0
}

// splitRow splits a table row at its unescaped pipes, leaving out the one
// it starts with and the one it ends with. An escaped pipe stays escaped:
// the cell's inline parse reads it.
func splitRow(s string) []string {
	s = strings.Trim(s, " \t")
	s = strings.TrimPrefix(s, "|")
	if strings.HasSuffix(s, "|") && !escapedAt(s, len(s)-1) {
		s = s[:len(s)-1]
	}
	var cells []string
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '|':
			cells = append(cells, s[start:i])
			start = i + 1
		}
	}
	return append(cells, s[start:])
}

// escapedAt reports whether the byte at i of s follows an odd run of
// backslashes.
func escapedAt(s string, i int) bool {
	n := 0
	for i--; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

// listMarker reads a list item's marker at the position, and moves past
// it and the spaces that set where the item's content starts.
func (p *parser) listMarker(container *node) (listData, bool) {
	if p.indent >= 4 {
		return listData{}, false
	}
	s := p.ln[p.next:]
	var d listData
	n := 0
	switch {
	case len(s) == 0:
		return d, false
	case s[0] == '-' || s[0] == '+' || s[0] == '*':
		d.marker, n = s[0], 1
	case s[0] >= '0' && s[0] <= '9':
		for n < len(s) && n < 9 && s[n] >= '0' && s[n] <= '9' {
			d.number = d.number*10 + int(s[n]-'0')
			n++
		}
		if n == len(s) || (s[n] != '.' && s[n] != ')') {
			return d, false
		}
		// Only a list starting at 1 may interrupt a paragraph.
		if container.kind == Paragraph && d.number != 1 {
			return d, false
		}
		d.ordered, d.marker = true, s[n]
		n++
	default:
		return d, false
	}
	if n < len(s) && s[n] != ' ' && s[n] != '\t' {
		return d, false
	}
	// An empty item may not interrupt a paragraph.
	if container.kind == Paragraph && strings.Trim(s[n:], " \t") == "" {
		return d, false
	}
	d.markerOffset = p.indent
	p.advanceNext()
	p.advance(n, true)
	startCol, startOff, startPartial := p.col, p.off, p.partial
	for {
		p.advance(1, true)
		if b := p.peek(p.off); p.col-startCol >= 5 || (b != ' ' && b != '\t') {
			break
		}
	}
	spaces := p.col - startCol
	if spaces >= 5 || spaces < 1 || p.off >= len(p.ln) {
		// Code indented in the item, or an item starting blank: its content
		// starts one space after the marker.
		d.padding = n + 1
		p.col, p.off, p.partial = startCol, startOff, startPartial
		if b := p.peek(p.off); b == ' ' || b == '\t' {
			p.advance(1, true)
		}
	} else {
		d.padding = n + spaces
	}
	return d, true
}

// closeUnmatched closes the blocks the line did not continue.
func (p *parser) closeUnmatched() {
	if p.allClosed {
		return
	}
	for p.oldtip != p.matched && p.oldtip != nil {
		parent := p.oldtip.parent
		p.finalize(p.oldtip)
		p.oldtip = parent
	}
	p.allClosed = true
}

// addChild opens a block of kind k in the innermost open block that can
// hold it, closing the ones that cannot.
func (p *parser) addChild(k Kind) *node {
	for !canContain(p.tip.kind, k) {
		p.finalize(p.tip)
	}
	n := &node{kind: k, parent: p.tip, open: true, start: p.line, end: p.line}
	p.tip.children = append(p.tip.children, n)
	p.tip = n
	return n
}

// canContain reports whether a block of kind parent holds one of kind k.
func canContain(parent, k Kind) bool {
	switch parent {
	case document, Quote, Item:
		return k != Item
	case List:
		return k == Item
	}
	return false
}

// addLine adds the rest of the line to the innermost open block.
func (p *parser) addLine() {
	t := p.tip
	s := p.rest()
	switch t.kind {
	case Paragraph:
		t.lines = append(t.lines, strings.TrimLeft(s, " \t"))
	case CodeBlock:
		t.lines = append(t.lines, s)
		if t.fenced || strings.Trim(s, " \t") != "" {
			t.end = p.line
		}
		return
	case Table:
		t.lines = append(t.lines, s)
	}
	t.end = p.line
}

// finalize closes b, which is the innermost open block.
func (p *parser) finalize(b *node) {
	b.open = false
	p.tip = b.parent
	switch b.kind {
	case CodeBlock:
		if !b.fenced {
			// An indented block ends at its last line of code.
			for len(b.lines) > 0 && strings.Trim(b.lines[len(b.lines)-1], " \t") == "" {
				b.lines = b.lines[:len(b.lines)-1]
			}
		}
	case Item:
		if len(b.children) > 0 {
			b.end = b.children[len(b.children)-1].end
		} else {
			b.end = b.start
		}
	case List:
		b.tight = true
		for i, item := range b.children {
			last := i == len(b.children)-1
			if !last && endsBlank(item, b.children[i+1]) {
				b.tight = false
			}
			for j, sub := range item.children {
				if j+1 < len(item.children) && endsBlank(sub, item.children[j+1]) {
					b.tight = false
				}
			}
		}
		if len(b.children) > 0 {
			b.end = b.children[len(b.children)-1].end
		}
	}
}

// endsBlank reports whether a blank line parts b from next.
func endsBlank(b, next *node) bool { return b.end != next.start-1 }

// blocks makes Blocks of nodes.
func (p *parser) blocks(nodes []*node) []Block {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]Block, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, p.block(n))
	}
	return out
}

// block makes a Block of n.
func (p *parser) block(n *node) Block {
	b := Block{Kind: n.kind, Pos: p.starts[n.start], End: p.ends[min(n.end, len(p.ends)-1)]}
	tail := n == p.inlineTail
	switch n.kind {
	case Paragraph:
		b.Inlines = parseInlines(strings.TrimRight(strings.Join(n.lines, "\n"), " \t"), tail)
	case Heading:
		b.Level = n.level
		b.Inlines = parseInlines(n.lines[0], tail)
	case CodeBlock:
		b.Info = n.info
		b.Code = strings.Join(n.lines, "\n")
		b.Open = n.fenced && !n.closed
	case Quote:
		b.Children = p.blocks(n.children)
	case List:
		b.Ordered, b.Number, b.Tight = n.list.ordered, n.list.number, n.tight
		b.Children = p.blocks(n.children)
	case Item:
		b.Task = taskOf(n)
		b.Children = p.blocks(n.children)
		if b.Task != NoTask && len(b.Children) > 0 && b.Children[0].Kind == Paragraph && len(b.Children[0].Inlines) == 0 {
			b.Children = b.Children[1:]
		}
	case Table:
		b.Align = n.align
		cols := len(n.align)
		b.Header = p.row(n.lines[0], cols, tail && len(n.lines) == 1)
		for i, ln := range n.lines[1:] {
			b.Rows = append(b.Rows, p.row(ln, cols, tail && i == len(n.lines)-2))
		}
	}
	return b
}

// row parses a table row into cols cells: those it lacks are empty, those
// past them are left out. An escaped pipe is a pipe, in a code span too.
func (p *parser) row(ln string, cols int, tail bool) []Cell {
	cells := splitRow(ln)
	row := make([]Cell, cols)
	for i := range min(cols, len(cells)) {
		text := strings.ReplaceAll(strings.Trim(cells[i], " \t"), `\|`, "|")
		row[i] = parseInlines(text, tail && i == min(cols, len(cells))-1)
	}
	return row
}

// taskOf reads the check box a list item's first paragraph starts with,
// and takes it out of the paragraph.
func taskOf(item *node) Task {
	if len(item.children) == 0 || item.children[0].kind != Paragraph || len(item.children[0].lines) == 0 {
		return NoTask
	}
	para := item.children[0]
	first := para.lines[0]
	if len(first) < 4 || first[0] != '[' || first[2] != ']' || (first[3] != ' ' && first[3] != '\t') {
		return NoTask
	}
	var t Task
	switch first[1] {
	case ' ':
		t = TaskOpen
	case 'x', 'X':
		t = TaskDone
	default:
		return NoTask
	}
	para.lines[0] = strings.TrimLeft(first[4:], " \t")
	return t
}
