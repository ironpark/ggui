package config

import (
	"strings"

	"github.com/BurntSushi/toml"
)

// keyLine returns the line, from 1, that defines key in data, or 0 when it
// cannot be found. The TOML decoder keeps key positions to itself, so this
// reads the file again the way a person would: table headers set where
// the following keys live, and a line that starts with a key and an equals
// sign defines that key. It does not need to be a parser, only to find
// lines in a file the decoder has already accepted; a key it misses is
// still reported, only without its line.
func keyLine(data []byte, key toml.Key) int {
	sc := &keyScan{want: key.String()}
	for i, line := range strings.Split(string(data), "\n") {
		sc.s = strings.TrimSpace(line)
		if sc.inMulti != "" {
			sc.closeMulti()
			continue
		}
		if hit, done := sc.tableOrBlank(); done {
			if hit {
				return i + 1
			}
			continue
		}
		if sc.keyLine() {
			return i + 1
		}
	}
	return 0
}

// keyScan carries what keyLine needs while it reads the file line by
// line: the key it looks for, the table the last header opened, and a
// multi-line string being skipped.
type keyScan struct {
	want    string
	table   []string
	inMulti string
	s       string
}

func (sc *keyScan) closeMulti() {
	if strings.Contains(sc.s, sc.inMulti) {
		sc.inMulti = ""
	}
}

// tableOrBlank handles a line that is blank, a comment or a table
// header, reporting whether it is the line looked for and whether the
// line is done with.
func (sc *keyScan) tableOrBlank() (hit, done bool) {
	if sc.s == "" || sc.s[0] == '#' {
		return false, true
	}
	if !strings.HasPrefix(sc.s, "[") {
		return false, false
	}
	inner := strings.TrimLeft(sc.s, "[")
	head, _, found := strings.Cut(inner, "]")
	if !found {
		return false, true
	}
	parts, ok := splitKey(head)
	if !ok {
		return false, true
	}
	sc.table = parts
	return toml.Key(sc.table).String() == sc.want, true
}

// keyLine reads the key a line defines, when it does, and opens a
// multi-line string the line's value begins.
func (sc *keyScan) keyLine() bool {
	parts, rest, ok := leadingKey(sc.s)
	if !ok {
		return false
	}
	full := append(append([]string{}, sc.table...), parts...)
	// A key below an inline table on this line, such as
	// plist = { A = 1 }, is reported at the line of the table.
	if k := toml.Key(full).String(); k == sc.want || strings.HasPrefix(sc.want, k+".") {
		return true
	}
	for _, delim := range []string{`"""`, `'''`} {
		if strings.Count(rest, delim)%2 == 1 {
			sc.inMulti = delim
		}
	}
	return false
}

// leadingKey reads a key, possibly dotted and quoted, at the start of s
// and the equals sign after it, returning the key's parts and the value.
func leadingKey(s string) (parts []string, rest string, ok bool) {
	eq := -1
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '=':
			eq = i
		}
		if eq >= 0 {
			break
		}
	}
	if eq <= 0 {
		return nil, "", false
	}
	parts, ok = splitKey(s[:eq])
	return parts, s[eq+1:], ok
}

// splitKey splits a dotted key into its parts, unquoting each.
func splitKey(s string) ([]string, bool) {
	var parts []string
	s = strings.TrimSpace(s)
	for s != "" {
		part, rest, ok := keyPart(s)
		if !ok || part == "" {
			return nil, false
		}
		parts = append(parts, part)
		s = strings.TrimSpace(rest)
		if s == "" {
			break
		}
		if s[0] != '.' {
			return nil, false
		}
		s = strings.TrimSpace(s[1:])
	}
	return parts, len(parts) > 0
}

// keyPart reads one part of a dotted key at the start of s: a quoted
// string or a run of bare-key characters, and what follows it.
func keyPart(s string) (part, rest string, ok bool) {
	if s[0] == '"' || s[0] == '\'' {
		end := strings.IndexByte(s[1:], s[0])
		if end < 0 {
			return "", "", false
		}
		return s[1 : end+1], s[end+2:], true
	}
	end := strings.IndexAny(s, ". \t")
	if end < 0 {
		end = len(s)
	}
	for _, c := range s[:end] {
		if !(c == '-' || c == '_' || c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z') {
			return "", "", false
		}
	}
	return s[:end], s[end:], true
}
