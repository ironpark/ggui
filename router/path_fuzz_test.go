package router

import (
	"maps"
	"net/url"
	"slices"
	"strings"
	"testing"
)

// FuzzParsePattern checks that a registration path either fails or parses
// into named, slash-free segments whose rendering parses back to the same
// pattern.
func FuzzParsePattern(f *testing.F) {
	for _, s := range []string{"", "/", "/users/:id", "users/:id/", "/docs/*rest", "/a//b", "/:", "/*", "//", "/a/:x/*y", "/%zz/:ü"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, path string) {
		p, err := parsePattern(path)
		if err != nil {
			return
		}
		for _, s := range p {
			if s.text == "" || strings.Contains(s.text, "/") {
				t.Fatalf("parsePattern(%q) produced segment %+v", path, s)
			}
		}
		rendered := patternString(p)
		again, err := parsePattern(rendered)
		if err != nil || !slices.Equal(again, p) {
			t.Fatalf("parsePattern(%q) = %v rendered as %q, which parses to %v, %v", path, p, rendered, again, err)
		}
		// The rendering is the input with its optional slashes normalized.
		want := "/" + strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/")
		if rendered != want {
			t.Fatalf("patternString(parsePattern(%q)) = %q, want %q", path, rendered, want)
		}
	})
}

// FuzzParseURL checks that parseURL never panics, that what it accepts is
// split into non-empty segments that decode, and that a Location written
// back out parses to the same URL.
func FuzzParseURL(f *testing.F) {
	for _, s := range []string{"/", "/a/b", "/a/b/", "/users/a%2Fb", "/x?q=1&q=2#frag", "/p#a%20b", "/é/%E2%9C%93", "relative", "//host", "/a//b", "/a/%zz", "https://x.test/", "/?", "/#", "/a?b#c#d"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		u, err := parseURL(s)
		if err != nil {
			return
		}
		if len(u.raw) != len(u.decoded) {
			t.Fatalf("parseURL(%q): %d raw segments but %d decoded", s, len(u.raw), len(u.decoded))
		}
		for i, r := range u.raw {
			d, err := url.PathUnescape(r)
			if r == "" || strings.Contains(r, "/") || err != nil || d != u.decoded[i] {
				t.Fatalf("parseURL(%q): segment %d is %q decoding to %q, stored as %q", s, i, r, d, u.decoded[i])
			}
		}
		loc := u.location()
		again, err := parseURL(loc.String())
		if err != nil {
			t.Fatalf("parseURL(%q) wrote %q, which does not parse: %v", s, loc.String(), err)
		}
		if !slices.Equal(again.raw, u.raw) || !slices.Equal(again.decoded, u.decoded) ||
			again.rawQuery != u.rawQuery || again.fragment != u.fragment {
			t.Fatalf("parseURL(%q) = %+v, written as %q, parses to %+v", s, u, loc.String(), again)
		}
	})
}

// FuzzMatchBuiltURL builds a URL from a pattern and random parameter values
// and checks that matching the pattern against it gives the values back:
// buildURL and parseURL plus matchSegs are inverses for every value buildURL
// accepts.
func FuzzMatchBuiltURL(f *testing.F) {
	f.Add("/users/:id", "42\x00")
	f.Add("/org/:org/users/:id", "a b\x00é/x")
	f.Add("/docs/*rest", "guide/intro page")
	f.Add("/p/:a/:b/*c", "%2F\x00?#\x00x/../y")
	f.Add("/s p/:x", "100%")
	f.Add("/%zz/:x", "%zz")
	f.Fuzz(func(t *testing.T, pattern, values string) {
		p, err := parsePattern(pattern)
		if err != nil || !valid(p) {
			return
		}
		vals := strings.Split(values, "\x00")
		params := Params{}
		for i, s := range p {
			if s.kind != segStatic {
				params[s.text] = vals[i%len(vals)]
			}
		}
		built, err := buildURL(p, params)
		if err != nil {
			return
		}
		u, err := parseURL(built)
		if err != nil {
			t.Fatalf("buildURL(%s, %v) = %q, which does not parse: %v", patternString(p), params, built, err)
		}
		got, ok := matchSegs(p, u.decoded, false)
		if !ok {
			t.Fatalf("%s does not match its own URL %q (segments %q)", patternString(p), built, u.decoded)
		}
		if got == nil {
			got = map[string]string{}
		}
		if !maps.Equal(got, map[string]string(params)) {
			t.Fatalf("%s matched %q with %v, want %v", patternString(p), built, got, params)
		}
	})
}

// valid reports whether New would accept p as a page pattern: a catch-all
// only at the end and no parameter name used twice.
func valid(p []seg) bool {
	names := map[string]bool{}
	for i, s := range p {
		if s.kind == segStatic {
			continue
		}
		if names[s.text] || (s.kind == segCatchAll && i != len(p)-1) {
			return false
		}
		names[s.text] = true
	}
	return true
}
