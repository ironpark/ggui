// Package markdown parses Markdown into blocks of inline content, the
// shapes a renderer lays out: the CommonMark constructs a chat message or a
// README is written in, with GitHub's tables, task list items,
// strikethrough and autolinks.
//
// It covers headings (ATX and setext), paragraphs, block quotes, ordered,
// bullet and nested lists with lazy continuation lines, fenced and indented
// code, thematic breaks and tables; inline, it covers emphasis, strong
// emphasis, strikethrough, code spans, links, images, autolinks, backslash
// escapes, entities and hard breaks. Link reference definitions and HTML
// are not interpreted: a reference link and a tag are text as written, so
// nothing in a message can reach outside what it shows.
//
// Parse never panics and runs in time linear in its input, near enough, so
// that a renderer can parse a message again at every piece of it that
// arrives. ParseStream reads a source cut off mid-stream: what the end
// leaves open at the end, a strong span, a code span, a link, closes there,
// so a reply streaming in does not flash its syntax before the closing
// half arrives.
//
// The package depends on nothing of ggui and has no widgets: it is the
// engine ui.Markdown renders.
package markdown

import "strings"

// Kind is what a Block is.
type Kind uint8

// The kinds of block.
const (
	Paragraph     Kind = iota + 1 // Inlines
	Heading                       // Level and Inlines
	ThematicBreak                 // a rule between sections
	CodeBlock                     // Info and Code, fenced or indented
	Quote                         // Children
	List                          // Children, each an Item; Ordered, Number and Tight
	Item                          // Children and Task
	Table                         // Align, Header and Rows
)

// Task is the check box a list item of a task list starts with.
type Task uint8

// The states of a task list item.
const (
	NoTask   Task = iota // an ordinary item
	TaskOpen             // [ ]
	TaskDone             // [x]
)

// Align is how a table's column is aligned, as its delimiter row says.
type Align uint8

// The alignments of a table column.
const (
	AlignNone Align = iota
	AlignLeft
	AlignCenter
	AlignRight
)

// Cell is the content of one table cell.
type Cell []Inline

// Block is one block of a document. A field means something only for the
// kinds its comment names; the rest are zero.
type Block struct {
	Kind Kind

	// Pos and End are the byte offsets in the source of the lines the block
	// spans, from the start of its first to the end of its last, without
	// the line ending. A nested block's lines include its containers'
	// markers.
	Pos, End int

	Level   int      // Heading: 1 to 6
	Inlines []Inline // Paragraph and Heading

	Info string // CodeBlock: a fence's info string, its language first; empty for an indented block
	Code string // CodeBlock: its lines, a line feed apart, without the last line's
	Open bool   // CodeBlock: a fence the source ended inside, still to be closed

	Children []Block // Quote and Item: blocks; List: its Items

	Ordered bool // List: numbered rather than bulleted
	Number  int  // List: an ordered list's first number
	Tight   bool // List: no blank line parts its items or their blocks

	Task Task // Item

	Align  []Align  // Table: one per column
	Header []Cell   // Table: the heading row, one cell per column
	Rows   [][]Cell // Table: the body, each row one cell per column
}

// Lang is a code block's language: the first word of its info string.
func (b Block) Lang() string {
	lang, _, _ := strings.Cut(strings.TrimSpace(b.Info), " ")
	lang, _, _ = strings.Cut(lang, "\t")
	return lang
}

// InlineKind is what an Inline is.
type InlineKind uint8

// The kinds of inline content.
const (
	Text      InlineKind = iota + 1 // Text, as written: HTML included, never interpreted
	CodeSpan                        // Text
	Emphasis                        // Children
	Strong                          // Children
	Strike                          // Children
	Link                            // URL, Title and Children; URL is empty for a link still streaming in
	Image                           // URL, Title and Children, its alternative text
	SoftBreak                       // a line ending inside a paragraph, which wraps as a space would
	HardBreak                       // a line ending that breaks the line
)

// Inline is one piece of a paragraph, a heading or a table cell.
type Inline struct {
	Kind     InlineKind
	Text     string   // Text and CodeSpan
	URL      string   // Link and Image
	Title    string   // Link and Image
	Children []Inline // Emphasis, Strong, Strike, Link and Image
}

// Parse parses src as a whole document.
func Parse(src string) []Block { return parse(src, false) }

// ParseStream parses src as the part of a document received so far. It is
// Parse, but for the last block, which the next piece of the stream may
// still change: an emphasis, strong emphasis, strikethrough or code span
// opened in it and not yet closed is closed at the end, a link whose
// destination has not arrived is a link with no URL, and a run of
// delimiters at the very end, which may open one of those, is left out.
func ParseStream(src string) []Block { return parse(src, true) }

// Reparse parses src as Parse does, or with stream as ParseStream does,
// given prev, the blocks Parse or ParseStream made of prevSrc. When src is
// prevSrc with more after it, as a stream grows, only its last blocks are
// read again: a reply streaming in costs a parse of its last block or two
// per piece, not of all of it. Otherwise it parses src whole.
//
// The parser reads a line once, so what it made of a whole line stays: a
// block that a whole line closed is closed for good. prevSrc's last line,
// unless a line ending ends it, may read otherwise once the rest of it
// arrives, and so may the one block that was open before it, which it
// may have closed, or split into a paragraph and a table whose heading
// row was the paragraph's last line. Those are read again: the last two
// blocks that started before that line, and any after.
func Reparse(prev []Block, prevSrc, src string, stream bool) []Block {
	if len(prev) == 0 || len(src) < len(prevSrc) || src[:len(prevSrc)] != prevSrc {
		return parse(src, stream)
	}
	whole := len(prevSrc)
	if i := strings.LastIndexAny(prevSrc, "\r\n"); i < len(prevSrc)-1 {
		whole = i + 1
	}
	m := len(prev) - 1
	for m > 0 && prev[m].Pos >= whole {
		m--
	}
	m = max(0, m-1)
	from := prev[m].Pos
	tail := parse(src[from:], stream)
	shift(tail, from)
	out := make([]Block, 0, m+len(tail))
	return append(append(out, prev[:m]...), tail...)
}

// shift moves blocks, and every block inside them, by n bytes.
func shift(blocks []Block, n int) {
	for i := range blocks {
		blocks[i].Pos += n
		blocks[i].End += n
		shift(blocks[i].Children, n)
	}
}

// Equal reports whether a and b are the same block: the same content,
// wherever in their sources they are.
func Equal(a, b Block) bool {
	if !a.sameShape(b) {
		return false
	}
	// A block Reparse kept shares its parts with the one it was: a widget
	// comparing a long reply's blocks at every piece of it compares little.
	if sameSlice(a.Children, b.Children) && sameSlice(a.Rows, b.Rows) {
		return true
	}
	for i := range a.Children {
		if !Equal(a.Children[i], b.Children[i]) {
			return false
		}
	}
	for i := range a.Rows {
		if !equalRow(a.Rows[i], b.Rows[i]) {
			return false
		}
	}
	return true
}

// sameShape reports whether a and b agree on everything but their
// children and rows.
func (a Block) sameShape(b Block) bool {
	return a.Kind == b.Kind && a.Level == b.Level && a.Info == b.Info && a.Code == b.Code && a.Open == b.Open &&
		a.Ordered == b.Ordered && a.Number == b.Number && a.Tight == b.Tight && a.Task == b.Task &&
		len(a.Children) == len(b.Children) && len(a.Rows) == len(b.Rows) &&
		equalInlines(a.Inlines, b.Inlines) && equalAlign(a.Align, b.Align) && equalRow(a.Header, b.Header)
}

func equalInlines(a, b []Inline) bool {
	if len(a) != len(b) {
		return false
	}
	if sameSlice(a, b) {
		return true
	}
	for i := range a {
		x, y := &a[i], &b[i]
		if x.Kind != y.Kind || x.Text != y.Text || x.URL != y.URL || x.Title != y.Title || !equalInlines(x.Children, y.Children) {
			return false
		}
	}
	return true
}

// sameSlice reports whether a and b, of one length, are the same elements
// in memory.
func sameSlice[T any](a, b []T) bool { return len(a) == 0 || &a[0] == &b[0] }

func equalAlign(a, b []Align) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalRow(a, b []Cell) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalInlines(a[i], b[i]) {
			return false
		}
	}
	return true
}

// PlainText returns the text of inlines as a reader would read it: the
// text of every span, a space for a soft break and a line feed for a hard
// one.
func PlainText(inlines []Inline) string {
	var b strings.Builder
	writePlain(&b, inlines)
	return b.String()
}

func writePlain(b *strings.Builder, inlines []Inline) {
	for _, in := range inlines {
		switch in.Kind {
		case Text, CodeSpan:
			b.WriteString(in.Text)
		case SoftBreak:
			b.WriteByte(' ')
		case HardBreak:
			b.WriteByte('\n')
		default:
			writePlain(b, in.Children)
		}
	}
}
