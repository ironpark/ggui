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
	want := key.String()
	var table []string
	inMulti := "" // the delimiter of a multi-line string being skipped
	for i, line := range strings.Split(string(data), "\n") {
		n := i + 1
		s := strings.TrimSpace(line)
		if inMulti != "" {
			if strings.Contains(s, inMulti) {
				inMulti = ""
			}
			continue
		}
		switch {
		case s == "" || s[0] == '#':
			continue
		case strings.HasPrefix(s, "[[") || strings.HasPrefix(s, "["):
			inner := strings.TrimLeft(s, "[")
			end := strings.Index(inner, "]")
			if end < 0 {
				continue
			}
			parts, ok := splitKey(inner[:end])
			if !ok {
				continue
			}
			table = parts
			if toml.Key(table).String() == want {
				return n
			}
			continue
		}
		parts, rest, ok := leadingKey(s)
		if !ok {
			continue
		}
		full := append(append([]string{}, table...), parts...)
		// A key below an inline table on this line, such as
		// plist = { A = 1 }, is reported at the line of the table.
		if k := toml.Key(full).String(); k == want || strings.HasPrefix(want, k+".") {
			return n
		}
		for _, delim := range []string{`"""`, `'''`} {
			if strings.Count(rest, delim)%2 == 1 {
				inMulti = delim
			}
		}
	}
	return 0
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
		var part string
		switch s[0] {
		case '"', '\'':
			end := strings.IndexByte(s[1:], s[0])
			if end < 0 {
				return nil, false
			}
			part, s = s[1:end+1], s[end+2:]
		default:
			end := strings.IndexAny(s, ". \t")
			if end < 0 {
				end = len(s)
			}
			part, s = s[:end], s[end:]
			for _, c := range part {
				if !(c == '-' || c == '_' || c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z') {
					return nil, false
				}
			}
		}
		if part == "" {
			return nil, false
		}
		parts = append(parts, part)
		s = strings.TrimSpace(s)
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
