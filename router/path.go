package router

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type segKind uint8

const (
	segStatic segKind = iota
	segParam
	segCatchAll
)

// seg is one segment of a registered pattern.
type seg struct {
	kind segKind
	text string // static text, or the parameter name
}

// parsePattern splits a registration path relative to its section. The
// leading slash is notation only; "" and "/" contribute no segments.
func parsePattern(path string) ([]seg, error) {
	p := strings.TrimPrefix(path, "/")
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return nil, nil
	}
	var out []seg
	for s := range strings.SplitSeq(p, "/") {
		switch {
		case s == "":
			return nil, fmt.Errorf("empty segment in %q", path)
		case s[0] == ':':
			if len(s) == 1 {
				return nil, fmt.Errorf("unnamed parameter in %q", path)
			}
			out = append(out, seg{segParam, s[1:]})
		case s[0] == '*':
			if len(s) == 1 {
				return nil, fmt.Errorf("unnamed catch-all in %q", path)
			}
			out = append(out, seg{segCatchAll, s[1:]})
		default:
			out = append(out, seg{segStatic, s})
		}
	}
	return out, nil
}

// patternString renders an effective pattern for messages and URLs.
func patternString(p []seg) string {
	if len(p) == 0 {
		return "/"
	}
	var b strings.Builder
	for _, s := range p {
		b.WriteByte('/')
		switch s.kind {
		case segParam:
			b.WriteByte(':')
		case segCatchAll:
			b.WriteByte('*')
		}
		b.WriteString(s.text)
	}
	return b.String()
}

// signature identifies a pattern up to parameter names: two leaf patterns
// with one signature are equally specific for every URL either matches.
func signature(p []seg) string {
	var b strings.Builder
	for _, s := range p {
		b.WriteByte('/')
		switch s.kind {
		case segParam:
			b.WriteString("\x00:")
		case segCatchAll:
			b.WriteString("\x00*")
		default:
			b.WriteString(s.text)
		}
	}
	return b.String()
}

// parsedURL is an application-absolute URL split for matching.
type parsedURL struct {
	raw      []string // escaped segments
	decoded  []string // unescaped segments; an encoded slash stays inside one
	rawQuery string
	fragment string
}

func (u parsedURL) path() string { return "/" + strings.Join(u.raw, "/") }

func (u parsedURL) location() Location {
	loc := Location{Path: u.path(), RawQuery: u.rawQuery, Fragment: u.fragment}
	if u.rawQuery != "" {
		v, _ := url.ParseQuery(u.rawQuery)
		loc.parsed = &parsedQuery{raw: u.rawQuery, values: v}
	}
	return loc
}

var errNotAbsolute = errors.New("router: URL must be application-absolute, such as /settings")

// parseURL normalizes an application-absolute URL. Bad escapes and internal
// duplicate slashes are errors, not silent corrections.
func parseURL(s string) (parsedURL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return parsedURL{}, fmt.Errorf("router: %w", err)
	}
	if u.Scheme != "" || u.Host != "" || u.Opaque != "" || !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") {
		return parsedURL{}, errNotAbsolute
	}
	raw := u.EscapedPath()
	raw = strings.TrimPrefix(raw, "/")
	raw = strings.TrimSuffix(raw, "/")
	out := parsedURL{rawQuery: u.RawQuery, fragment: u.Fragment}
	if raw == "" {
		return out, nil
	}
	for r := range strings.SplitSeq(raw, "/") {
		if r == "" {
			return parsedURL{}, fmt.Errorf("router: duplicate slash in %q", s)
		}
		d, err := url.PathUnescape(r)
		if err != nil {
			return parsedURL{}, fmt.Errorf("router: bad escape in %q", s)
		}
		out.raw = append(out.raw, r)
		out.decoded = append(out.decoded, d)
	}
	return out, nil
}

// matchSegs matches pattern against the decoded path. With prefix set it
// matches a section: the pattern may cover only the path's beginning.
func matchSegs(p []seg, path []string, prefix bool) (map[string]string, bool) {
	var params map[string]string
	for i, s := range p {
		if s.kind == segCatchAll {
			return matchCatchAll(s, path, i, params)
		}
		if i >= len(path) {
			return nil, false
		}
		v := path[i]
		switch s.kind {
		case segStatic:
			if v != s.text {
				return nil, false
			}
		case segParam:
			if v == "" || strings.Contains(v, "/") {
				return nil, false
			}
			params = setParam(params, s.text, v)
		}
	}
	if !prefix && len(p) != len(path) {
		return nil, false
	}
	return params, true
}

// matchCatchAll matches the catch-all s against every segment from i of
// path, joined with slashes.
func matchCatchAll(s seg, path []string, i int, params map[string]string) (map[string]string, bool) {
	if i >= len(path) {
		return nil, false
	}
	rest := path[i:]
	for _, v := range rest {
		if strings.Contains(v, "/") {
			return nil, false
		}
	}
	return setParam(params, s.text, strings.Join(rest, "/")), true
}

// setParam records name's value, making the table on first use.
func setParam(params map[string]string, name, value string) map[string]string {
	if params == nil {
		params = map[string]string{}
	}
	params[name] = value
	return params
}

// moreSpecific reports whether a beats b, comparing segment kinds from the
// left: static beats parameter beats catch-all, and at a position where one
// pattern has ended the longer one wins.
func moreSpecific(a, b []seg) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i].kind != b[i].kind {
			return a[i].kind < b[i].kind
		}
	}
	return len(a) > len(b)
}

// buildURL fills pattern from params, escaping each value.
func buildURL(p []seg, params Params) (string, error) {
	used := 0
	var b strings.Builder
	for _, s := range p {
		b.WriteByte('/')
		if s.kind == segStatic {
			b.WriteString(url.PathEscape(s.text))
			continue
		}
		v, ok := params[s.text]
		if !ok {
			return "", fmt.Errorf("router: missing parameter %q for %s", s.text, patternString(p))
		}
		used++
		if v == "" {
			return "", fmt.Errorf("router: empty parameter %q for %s", s.text, patternString(p))
		}
		if err := writeSegValue(&b, s, v); err != nil {
			return "", err
		}
	}
	if used != len(params) {
		for name := range params {
			if !hasParam(p, name) {
				return "", fmt.Errorf("router: unknown parameter %q for %s", name, patternString(p))
			}
		}
	}
	if b.Len() == 0 {
		return "/", nil
	}
	return b.String(), nil
}

// writeSegValue writes the escaped value of one param or catch-all
// segment.
func writeSegValue(b *strings.Builder, s seg, v string) error {
	if s.kind == segParam {
		if strings.Contains(v, "/") {
			return fmt.Errorf("router: parameter %q contains a slash", s.text)
		}
		b.WriteString(url.PathEscape(v))
		return nil
	}
	for i, part := range strings.Split(v, "/") {
		if part == "" {
			return fmt.Errorf("router: catch-all %q has an empty segment", s.text)
		}
		if i > 0 {
			b.WriteByte('/')
		}
		b.WriteString(url.PathEscape(part))
	}
	return nil
}

func hasParam(p []seg, name string) bool {
	for _, s := range p {
		if s.kind != segStatic && s.text == name {
			return true
		}
	}
	return false
}
