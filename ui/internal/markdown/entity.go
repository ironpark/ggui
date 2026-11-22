package markdown

import (
	"html"
	"strings"
	"unicode/utf8"
)

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
		return numericEntity(s, name, end)
	}
	if !isEntityName(name) {
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

// numericEntity reads a &#123; or &#x1F; character reference.
func numericEntity(s, name string, end int) (string, int) {
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
