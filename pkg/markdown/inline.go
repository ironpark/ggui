package markdown

import (
	"html"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The inline parser is CommonMark's: text goes into a list of nodes as it
// is read, a run of *, _ or ~ as a node of its own on a stack of
// delimiters, and [ and ![ on a stack of brackets. A ] closes the bracket
// on top as a link when a destination follows, and emphasis is matched
// among the delimiters left, from the innermost out, when the text ends.

// inode is an inline as the parser builds it, in a list it can cut and
// splice.
type inode struct {
	kind        InlineKind
	text        string
	url, title  string
	first, last *inode // children
	prev, next  *inode
}

// delim is a run of delimiters on the stack.
type delim struct {
	node      *inode
	char      byte
	n, orig   int // left of the run, and its length as written
	seq       int // monotonic position on the stack, for openersBottom bounds
	canOpen   bool
	canClose  bool
	prev, nxt *delim
}

// bracket is a [ or ![ on the stack, waiting for its ].
type bracket struct {
	node   *inode
	prev   *bracket
	delims *delim // the delimiter on top when it was read
	image  bool
	active bool // false once a link closes around it: links do not nest
}

type inlineParser struct {
	s            string
	pos          int
	tail         bool // the end of s is where a stream was cut
	root         inode
	delims       *delim
	nextDelimSeq int
	brackets     *bracket
	noTicks      map[int]bool // backtick runs of a length with no closer left
}

// parseInlines parses the inline content s of a block; tail says it ends
// where a stream was cut, as ParseStream's last block does.
func parseInlines(s string, tail bool) []Inline {
	if s == "" {
		return nil
	}
	p := &inlineParser{s: s, tail: tail}
	for p.pos < len(s) {
		p.step()
	}
	if tail {
		// An open bracket at the cut is a link on its way; unlink it before
		// processEmphasis so wrapping open delimiters does not move it under
		// another parent where unlink would sever following siblings.
		for b := p.brackets; b != nil; b = b.prev {
			p.unlink(b.node)
		}
	}
	p.processEmphasis(nil, tail)
	return convert(p.root.first)
}

// append adds n at the end of the list.
func (p *inlineParser) append(n *inode) {
	n.prev = p.root.last
	if p.root.last != nil {
		p.root.last.next = n
	} else {
		p.root.first = n
	}
	p.root.last = n
}

// unlink takes n out of the list.
func (p *inlineParser) unlink(n *inode) {
	if n.prev != nil {
		n.prev.next = n.next
	} else if p.root.first == n {
		p.root.first = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	} else if p.root.last == n {
		p.root.last = n.prev
	}
	n.prev, n.next = nil, nil
}

// wrap moves the nodes after from up to the end of the list, or up to
// before to, into n, and puts n after from.
func (p *inlineParser) wrap(n, from, to *inode) {
	first := from.next
	var last *inode
	if to != nil {
		last = to.prev
	} else {
		last = p.root.last
	}
	if first != nil && first != to {
		n.first, n.last = first, last
		first.prev = nil
		if to != nil {
			to.prev = n
		} else {
			p.root.last = n
		}
		n.next = to
		last.next = nil
	} else {
		n.next = from.next
		if from.next != nil {
			from.next.prev = n
		} else {
			p.root.last = n
		}
	}
	from.next = n
	n.prev = from
}

// text adds s as text. Text read in pieces stays in pieces until convert
// joins them, once: joining as it is read would copy a long paragraph of
// escapes and entities once for each.
func (p *inlineParser) text(s string) { p.append(&inode{kind: Text, text: s}) }

// step reads what is at the position.
func (p *inlineParser) step() {
	s := p.s
	c := s[p.pos]
	switch c {
	case '\n':
		p.lineBreak(false)
	case '\\':
		switch {
		case p.pos+1 < len(s) && s[p.pos+1] == '\n':
			p.pos++
			p.lineBreak(true)
		case p.pos+1 < len(s) && isPunct(s[p.pos+1]):
			p.text(s[p.pos+1 : p.pos+2])
			p.pos += 2
		default:
			p.text(`\`)
			p.pos++
		}
	case '`':
		p.codeSpan()
	case '*', '_', '~':
		p.delimiterRun(c)
	case '[':
		p.openBracket(false, 1)
	case '!':
		if p.pos+1 < len(s) && s[p.pos+1] == '[' {
			p.openBracket(true, 2)
			return
		}
		p.text("!")
		p.pos++
	case ']':
		p.closeBracket()
	case '<':
		if !p.autolink() {
			p.text("<")
			p.pos++
		}
	case '&':
		p.entity()
	default:
		if (c == 'w' || c == 'h') && p.wordStart(p.pos) && p.extendedAutolink() {
			return
		}
		j := p.pos + 1
		for j < len(s) && !isSpecial(s[j]) && !((s[j] == 'w' || s[j] == 'h') && p.wordStart(j)) {
			j++
		}
		p.text(s[p.pos:j])
		p.pos = j
	}
}

// isSpecial reports whether c may start something other than text.
func isSpecial(c byte) bool {
	switch c {
	case '\n', '\\', '`', '*', '_', '~', '[', '!', ']', '<', '&':
		return true
	}
	return false
}

// isPunct reports whether c is ASCII punctuation, which a backslash
// escapes.
func isPunct(c byte) bool {
	return c >= '!' && c <= '/' || c >= ':' && c <= '@' || c >= '[' && c <= '`' || c >= '{' && c <= '~'
}

// lineBreak reads a line ending: a hard break after a backslash or two
// spaces, else a soft one. The spaces around it are dropped.
func (p *inlineParser) lineBreak(hard bool) {
	if l := p.root.last; l != nil && l.kind == Text {
		trimmed := strings.TrimRight(l.text, " ")
		hard = hard || len(l.text)-len(trimmed) >= 2
		l.text = trimmed
	}
	kind := SoftBreak
	if hard {
		kind = HardBreak
	}
	p.append(&inode{kind: kind})
	p.pos++
	for p.pos < len(p.s) && (p.s[p.pos] == ' ' || p.s[p.pos] == '\t') {
		p.pos++
	}
}

// codeSpan reads a run of backticks: a code span up to the next run as
// long, or the run as text when none follows.
func (p *inlineParser) codeSpan() {
	s := p.s
	start := p.pos
	n := run(s[start:], '`')
	after := start + n
	if !p.noTicks[n] {
		for i := after; i < len(s); {
			if s[i] != '`' {
				i++
				continue
			}
			j := i + run(s[i:], '`')
			if j-i == n {
				p.append(&inode{kind: CodeSpan, text: codeText(s[after:i])})
				p.pos = j
				return
			}
			i = j
		}
		if p.noTicks == nil {
			p.noTicks = map[int]bool{}
		}
		p.noTicks[n] = true
	}
	if p.tail {
		// A code span the stream has not closed yet runs to the cut.
		if after < len(s) {
			p.append(&inode{kind: CodeSpan, text: strings.TrimPrefix(strings.ReplaceAll(s[after:], "\n", " "), " ")})
		}
		p.pos = len(s)
		return
	}
	p.text(s[start:after])
	p.pos = after
}

// codeText is a code span's content: line endings as spaces, and one space
// at each end stripped when both have one and it is not all spaces.
func codeText(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) >= 2 && s[0] == ' ' && s[len(s)-1] == ' ' && strings.Trim(s, " ") != "" {
		s = s[1 : len(s)-1]
	}
	return s
}

// delimiterRun reads a run of *, _ or ~ and puts it on the stack, with
// whether it may open or close emphasis by what is on either side of it.
func (p *inlineParser) delimiterRun(c byte) {
	s := p.s
	start := p.pos
	n := run(s[start:], c)
	p.pos += n
	if c == '~' && n > 2 {
		p.text(s[start:p.pos])
		return
	}
	before, after := ' ', ' '
	if start > 0 {
		before, _ = utf8.DecodeLastRuneInString(s[:start])
	}
	if p.pos < len(s) {
		after, _ = utf8.DecodeRuneInString(s[p.pos:])
	} else if p.tail {
		// What follows a run at the cut has not arrived; it may open.
		after = 'a'
	}
	left := !isSpace(after) && (!isPunctuation(after) || isSpace(before) || isPunctuation(before))
	right := !isSpace(before) && (!isPunctuation(before) || isSpace(after) || isPunctuation(after))
	p.nextDelimSeq++
	d := &delim{char: c, n: n, orig: n, seq: p.nextDelimSeq, prev: p.delims}
	if c == '_' {
		d.canOpen = left && (!right || isPunctuation(before))
		d.canClose = right && (!left || isPunctuation(after))
	} else {
		d.canOpen, d.canClose = left, right
	}
	d.node = &inode{kind: Text, text: s[start:p.pos]}
	p.append(d.node)
	if p.delims != nil {
		p.delims.nxt = d
	}
	p.delims = d
}

// isSpace reports whether r is Unicode whitespace, as the flanking rules
// read it.
func isSpace(r rune) bool { return unicode.IsSpace(r) }

// isPunctuation reports whether r is Unicode punctuation or a symbol, as
// the flanking rules read it.
func isPunctuation(r rune) bool { return unicode.IsPunct(r) || unicode.IsSymbol(r) }

// removeDelim takes d off the stack; its node stays, as text.
func (p *inlineParser) removeDelim(d *delim) {
	if d.prev != nil {
		d.prev.nxt = d.nxt
	}
	if d.nxt != nil {
		d.nxt.prev = d.prev
	} else if p.delims == d {
		p.delims = d.prev
	}
	d.prev, d.nxt = nil, nil
}

// processEmphasis matches the delimiters above bottom on the stack into
// emphasis, strong emphasis and strikethrough, and takes them all off it.
// With closeOpen, those left open are closed at the end of the text, and
// a run with nothing after it is left out.
func (p *inlineParser) processEmphasis(bottom *delim, closeOpen bool) {
	// openersBottom remembers, for each kind of closer, the highest sequence
	// number down the stack where a search for its opener found none, so the
	// next search stops there even if earlier delimiters are removed or
	// spliced out: the matching stays linear rather than quadratic.
	var openersBottom [18]int
	bottomSeq := 0
	if bottom != nil {
		bottomSeq = bottom.seq
	}
	for i := range openersBottom {
		openersBottom[i] = bottomSeq
	}
	closer := p.delims
	if closer == bottom {
		closer = nil
	}
	for closer != nil && closer.prev != bottom {
		closer = closer.prev
	}
	for closer != nil {
		if !closer.canClose {
			closer = closer.nxt
			continue
		}
		idx := bottomIndex(closer)
		stop := openersBottom[idx]
		var opener *delim
		for o := closer.prev; o != nil && o.seq > stop; o = o.prev {
			if o.char != closer.char || !o.canOpen {
				continue
			}
			if closer.char == '~' {
				if o.n == closer.n {
					opener = o
					break
				}
				continue
			}
			// The rule of three: a run that may both open and close does not
			// pair with one whose lengths would add to a multiple of three,
			// unless both are such multiples.
			if (o.canClose || closer.canOpen) && (o.orig+closer.orig)%3 == 0 && !(o.orig%3 == 0 && closer.orig%3 == 0) {
				continue
			}
			opener = o
			break
		}
		if opener == nil {
			if closer.prev != nil && closer.prev.seq > openersBottom[idx] {
				openersBottom[idx] = closer.prev.seq
			}
			next := closer.nxt
			if !closer.canOpen {
				p.removeDelim(closer)
			}
			closer = next
			continue
		}
		use, kind := 1, Emphasis
		switch {
		case closer.char == '~':
			use, kind = closer.n, Strike
		case closer.n >= 2 && opener.n >= 2:
			use, kind = 2, Strong
		}
		opener.n -= use
		closer.n -= use
		opener.node.text = opener.node.text[:opener.n]
		closer.node.text = closer.node.text[:closer.n]
		p.wrap(&inode{kind: kind}, opener.node, closer.node)
		// The delimiters between are inside the emphasis now, as text.
		opener.nxt, closer.prev = closer, opener
		if opener.n == 0 {
			p.unlink(opener.node)
			p.removeDelim(opener)
		}
		if closer.n == 0 {
			next := closer.nxt
			p.unlink(closer.node)
			p.removeDelim(closer)
			closer = next
		}
	}
	if closeOpen {
		// What is still open was opened in the part of the stream that has
		// arrived; it closes at the cut, the innermost first.
		for d := p.delims; d != nil && d != bottom; d = d.prev {
			if !d.canOpen || d.n == 0 {
				continue
			}
			if d.node.next == nil {
				d.node.text = ""
				continue
			}
			switch {
			case d.char == '~':
				p.wrap(&inode{kind: Strike}, d.node, nil)
			case d.n == 1:
				p.wrap(&inode{kind: Emphasis}, d.node, nil)
			default:
				p.wrap(&inode{kind: Strong}, d.node, nil)
				if d.n >= 3 {
					p.wrap(&inode{kind: Emphasis}, d.node, nil)
				}
			}
			d.node.text = ""
		}
	}
	for p.delims != nil && p.delims != bottom {
		p.removeDelim(p.delims)
	}
}

// bottomIndex is the slot of openersBottom for a closer like d.
func bottomIndex(d *delim) int {
	i := 0
	switch d.char {
	case '_':
		i = 6
	case '~':
		i = 12
	}
	if d.canOpen {
		i += 3
	}
	return i + d.orig%3
}

// openBracket reads a [ or ![, of width bytes.
func (p *inlineParser) openBracket(image bool, width int) {
	n := &inode{kind: Text, text: p.s[p.pos : p.pos+width]}
	p.append(n)
	p.brackets = &bracket{node: n, prev: p.brackets, delims: p.delims, image: image, active: true}
	p.pos += width
}

// closeBracket reads a ]: the bracket on top and what follows it become a
// link when a destination follows, else the ] is text.
func (p *inlineParser) closeBracket() {
	p.pos++
	b := p.brackets
	if b == nil {
		p.text("]")
		return
	}
	p.brackets = b.prev
	if !b.active {
		p.text("]")
		return
	}
	url, title, end, ok, cut := p.linkTail(p.pos)
	if !ok && !(cut && p.tail) {
		p.text("]")
		return
	}
	link := &inode{kind: Link, url: url, title: title}
	if b.image {
		link.kind = Image
	}
	p.processEmphasis(b.delims, false)
	p.wrap(link, b.node, nil)
	p.unlink(b.node)
	p.pos = end
	if !b.image {
		p.deactivateLinks()
	}
}

// deactivateLinks marks all open non-image brackets inactive: a link holds
// no other link, so the brackets before it cannot close around it.
func (p *inlineParser) deactivateLinks() {
	for o := p.brackets; o != nil; o = o.prev {
		if !o.image {
			o.active = false
		}
	}
}

// linkTail reads an inline link's (destination "title") at i. cut reports
// that the text ended inside it before anything made it not one.
func (p *inlineParser) linkTail(i int) (url, title string, end int, ok, cut bool) {
	s := p.s
	if i >= len(s) || s[i] != '(' {
		return
	}
	i = skipSpace(s, i+1)
	if i >= len(s) {
		return "", "", len(s), false, true
	}
	// The destination: in <angle brackets>, or up to a space with its
	// parentheses balanced.
	switch {
	case s[i] == '<':
		j := i + 1
		for ; j < len(s) && s[j] != '>'; j++ {
			switch s[j] {
			case '\\':
				j++
			case '\n', '<':
				return
			}
		}
		if j >= len(s) {
			return "", "", len(s), false, true
		}
		url = unescape(s[i+1 : j])
		i = j + 1
	default:
		depth, j := 0, i
	dest:
		for ; j < len(s); j++ {
			switch c := s[j]; {
			case c == '\\' && j+1 < len(s) && isPunct(s[j+1]):
				j++
			case c == '(':
				depth++
				if depth > 32 {
					return
				}
			case c == ')':
				if depth == 0 {
					break dest
				}
				depth--
			case c <= ' ':
				break dest
			}
		}
		if j >= len(s) {
			return "", "", len(s), false, true
		}
		url = unescape(s[i:j])
		i = j
	}
	j := skipSpace(s, i)
	if j >= len(s) {
		return "", "", len(s), false, true
	}
	// A title, after a space: in "quotes", 'quotes' or (parentheses).
	if j > i && (s[j] == '"' || s[j] == '\'' || s[j] == '(') {
		closing := s[j]
		if closing == '(' {
			closing = ')'
		}
		k := j + 1
		for ; k < len(s) && s[k] != closing; k++ {
			if s[k] == '\\' {
				k++
			} else if closing == ')' && s[k] == '(' {
				return
			}
		}
		if k >= len(s) {
			return "", "", len(s), false, true
		}
		title = unescape(s[j+1 : k])
		j = skipSpace(s, k+1)
		if j >= len(s) {
			return "", "", len(s), false, true
		}
	}
	if s[j] != ')' {
		return "", "", 0, false, false
	}
	return url, title, j + 1, true, false
}

// skipSpace skips spaces, tabs and at most one line ending.
func skipSpace(s string, i int) int {
	newline := false
	for i < len(s) {
		switch s[i] {
		case ' ', '\t':
		case '\n':
			if newline {
				return i
			}
			newline = true
		default:
			return i
		}
		i++
	}
	return i
}

// autolink reads <scheme:destination> or <address@example.com> as a link.
func (p *inlineParser) autolink() bool {
	s := p.s
	j := p.pos + 1
	for j < len(s) && s[j] != '>' && s[j] != '<' && s[j] > ' ' {
		j++
	}
	if j >= len(s) || s[j] != '>' {
		return false
	}
	inner := s[p.pos+1 : j]
	var url string
	switch {
	case isURI(inner):
		url = inner
	case isEmail(inner):
		url = "mailto:" + inner
	default:
		return false
	}
	link := &inode{kind: Link, url: url}
	link.first = &inode{kind: Text, text: inner}
	link.last = link.first
	p.append(link)
	p.deactivateLinks()
	p.pos = j + 1
	return true
}

// isURI reports whether s is an absolute URI as an autolink takes one: a
// scheme of 2 to 32 characters, a colon, and no spaces or angle brackets.
func isURI(s string) bool {
	colon := strings.IndexByte(s, ':')
	if colon < 2 || colon > 32 {
		return false
	}
	for i := range colon {
		c := s[i]
		letter := c|0x20 >= 'a' && c|0x20 <= 'z'
		if i == 0 && !letter || !letter && !(c >= '0' && c <= '9') && c != '+' && c != '.' && c != '-' {
			return false
		}
	}
	return true
}

// isEmail reports whether s is an email address as an autolink takes one.
func isEmail(s string) bool {
	local, domain, ok := strings.Cut(s, "@")
	if !ok || local == "" || domain == "" {
		return false
	}
	for i := 0; i < len(local); i++ {
		c := local[i]
		if !(isAlnum(c) || strings.IndexByte(".!#$%&'*+/=?^_`{|}~-", c) >= 0) {
			return false
		}
	}
	for label := range strings.SplitSeq(domain, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			if !isAlnum(label[i]) && label[i] != '-' {
				return false
			}
		}
	}
	return true
}

func isAlnum(c byte) bool { return c >= '0' && c <= '9' || c|0x20 >= 'a' && c|0x20 <= 'z' }

// wordStart reports whether i of the text starts a word, where GitHub's
// extended autolinks may start: at the start, or after a space or one of
// *, _, ~ and (.
func (p *inlineParser) wordStart(i int) bool {
	if i == 0 {
		return true
	}
	switch p.s[i-1] {
	case ' ', '\t', '\n', '*', '_', '~', '(':
		return true
	}
	return false
}

// extendedAutolink reads a www. or http(s):// address that is not in
// angle brackets as a link, as GitHub does.
func (p *inlineParser) extendedAutolink() bool {
	s := p.s[p.pos:]
	var prefix string
	scheme := 0
	switch {
	case strings.HasPrefix(s, "www."):
		prefix = "http://"
	case strings.HasPrefix(s, "http://"):
		scheme = len("http://")
	case strings.HasPrefix(s, "https://"):
		scheme = len("https://")
	default:
		return false
	}
	end := 0
	for end < len(s) && s[end] > ' ' && s[end] != '<' && (p.brackets == nil || s[end] != ']') {
		end++
	}
	link := s[:end]
	// Punctuation at the end ends the sentence, not the address; so does a
	// closing parenthesis the address did not open, and an entity.
	for {
		trimmed := strings.TrimRight(link, "?!.,:*_~'\"")
		if strings.HasSuffix(trimmed, ")") && strings.Count(trimmed, ")") > strings.Count(trimmed, "(") {
			trimmed = trimmed[:len(trimmed)-1]
		}
		if strings.HasSuffix(trimmed, ";") {
			if amp := strings.LastIndexByte(trimmed, '&'); amp >= 0 && isEntityName(trimmed[amp+1:len(trimmed)-1]) {
				trimmed = trimmed[:amp]
			}
		}
		if trimmed == link {
			break
		}
		link = trimmed
	}
	if len(link) <= scheme {
		return false
	}
	host := link[scheme:]
	if i := strings.IndexAny(host, "/?#:"); i >= 0 {
		host = host[:i]
	}
	if !validDomain(host) {
		return false
	}
	n := &inode{kind: Link, url: prefix + link}
	n.first = &inode{kind: Text, text: link}
	n.last = n.first
	p.append(n)
	p.deactivateLinks()
	p.pos += len(link)
	return true
}

// validDomain reports whether host is a domain an extended autolink may
// name: two or more labels of letters, digits, - and _, with no _ in the
// last two.
func validDomain(host string) bool {
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for i, l := range labels {
		if l == "" {
			return false
		}
		for j := 0; j < len(l); j++ {
			c := l[j]
			if !isAlnum(c) && c != '-' && c != '_' && c < 0x80 {
				return false
			}
			if c == '_' && i >= len(labels)-2 {
				return false
			}
		}
	}
	return true
}

// isEntityName reports whether s is letters and digits, as the name of an
// entity is.
func isEntityName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isAlnum(s[i]) {
			return false
		}
	}
	return true
}

// entity reads an entity or a numeric character reference as the
// character it names, or the & as text.
func (p *inlineParser) entity() {
	if dec, n := entityAt(p.s[p.pos:]); n > 0 {
		p.text(dec)
		p.pos += n
		return
	}
	p.text("&")
	p.pos++
}

// entityAt reads the entity s starts with: its text and its length, or 0
// when s does not start with one.
func entityAt(s string) (string, int) {
	end := strings.IndexByte(s, ';')
	if end < 2 || end > 33 {
		return "", 0
	}
	name := s[1:end]
	if name[0] == '#' {
		digits := name[1:]
		hex := len(digits) > 0 && (digits[0] == 'x' || digits[0] == 'X')
		if hex {
			digits = digits[1:]
		}
		if digits == "" || len(digits) > 7 || hex && len(digits) > 6 {
			return "", 0
		}
		for i := 0; i < len(digits); i++ {
			c := digits[i]
			if !(c >= '0' && c <= '9' || hex && c|0x20 >= 'a' && c|0x20 <= 'f') {
				return "", 0
			}
		}
		dec := html.UnescapeString(s[:end+1])
		if strings.HasPrefix(dec, "\x00") {
			dec = "�"
		}
		return dec, end + 1
	} else if !isEntityName(name) {
		return "", 0
	}
	dec := html.UnescapeString(s[:end+1])
	// An unknown name stays as written. A known one decodes to a character
	// or two; html also decodes a legacy name before more letters, which
	// leaves those and the ; behind.
	if dec == s[:end+1] || utf8.RuneCountInString(dec) > 2 {
		return "", 0
	}
	return dec, end + 1
}

// unescape resolves the backslash escapes and entities of s, as a link
// destination, a title or a fence's info string has them.
func unescape(s string) string {
	if strings.IndexByte(s, '\\') < 0 && strings.IndexByte(s, '&') < 0 {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		switch {
		case s[i] == '\\' && i+1 < len(s) && isPunct(s[i+1]):
			b.WriteByte(s[i+1])
			i += 2
		case s[i] == '&':
			if dec, n := entityAt(s[i:]); n > 0 {
				b.WriteString(dec)
				i += n
				continue
			}
			b.WriteByte('&')
			i++
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

// convert makes Inlines of the list from first, joining text that ended
// up side by side and leaving out what ended up empty.
func convert(first *inode) []Inline {
	var out []Inline
	var b strings.Builder
	for n := first; n != nil; n = n.next {
		switch n.kind {
		case Text:
			if n.next == nil || n.next.kind != Text {
				if n.text != "" {
					out = append(out, Inline{Kind: Text, Text: n.text})
				}
				continue
			}
			b.Reset()
			for ; ; n = n.next {
				b.WriteString(n.text)
				if n.next == nil || n.next.kind != Text {
					break
				}
			}
			if b.Len() > 0 {
				out = append(out, Inline{Kind: Text, Text: b.String()})
			}
		case CodeSpan, SoftBreak, HardBreak:
			out = append(out, Inline{Kind: n.kind, Text: n.text})
		default:
			out = append(out, Inline{Kind: n.kind, URL: n.url, Title: n.title, Children: convert(n.first)})
		}
	}
	return out
}
