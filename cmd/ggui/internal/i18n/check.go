package i18n

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Kind is what a Problem is.
type Kind string

const (
	Missing    Kind = "missing"    // a message the catalog has no translation of
	Unused     Kind = "unused"     // a key no source passes
	Verbs      Kind = "verbs"      // the translation's format verbs differ
	Whitespace Kind = "whitespace" // its leading or trailing space differs
	NoOther    Kind = "no other"   // a plural translation without Other
	BadForm    Kind = "form"       // a plural form that is not a CLDR one
	Invalid    Kind = "invalid"    // the catalog cannot be read
)

// Problem is one thing wrong with a catalog.
type Problem struct {
	Kind Kind
	Key  string
	// Error is a problem that fails ggui i18n check; the rest are
	// warnings.
	Error  bool
	Detail string
}

// Options are what Check counts.
type Options struct {
	// AllowMissing makes a missing translation a warning.
	AllowMissing bool
}

// Check compares a catalog with the messages of s. A message whose
// translation is empty counts as missing, as one ggui i18n extract added
// and nobody translated is.
func Check(c *Catalog, s *Sources, o Options) []Problem {
	if c.Err != nil {
		return []Problem{{Kind: Invalid, Error: true, Detail: c.Err.Error()}}
	}
	var ps []Problem
	add := func(kind Kind, key string, isErr bool, format string, args ...any) {
		ps = append(ps, Problem{Kind: kind, Key: key, Error: isErr, Detail: fmt.Sprintf(format, args...)})
	}
	keys, pkeys := s.Keys()
	for _, k := range keys {
		m := s.Messages[k]
		tr, ok := c.Messages[k]
		switch {
		case !ok:
			add(Missing, k, !o.AllowMissing, "missing %s, used at %s", strconv.Quote(k), m.Uses[0])
		case c.dynamic[k]:
		case tr == "":
			add(Missing, k, !o.AllowMissing, "%s is not translated (empty)", strconv.Quote(k))
		default:
			checkText(add, k, "", m.Source, tr, false)
		}
	}
	for _, k := range pkeys {
		m := s.Plurals[k]
		forms, ok := c.Plurals[k]
		switch {
		case !ok:
			add(Missing, k, !o.AllowMissing, "missing plural %s, used at %s", strconv.Quote(k), m.Uses[0])
			continue
		case c.dynamic[k]:
			continue
		}
		translated := false
		for _, f := range forms {
			if f != "" {
				translated = true
			}
		}
		if !translated {
			add(Missing, k, !o.AllowMissing, "plural %s is not translated (empty)", strconv.Quote(k))
			continue
		}
		for _, name := range sortedKeys(forms) {
			if !slices.Contains(FormNames, name) {
				add(BadForm, k, true, "plural %s has a form %q; the forms are %s", strconv.Quote(k), name, strings.Join(FormNames, ", "))
			}
		}
		if forms["other"] == "" {
			add(NoOther, k, true, "plural %s has no other form, which every language uses", strconv.Quote(k))
		}
		for _, name := range FormNames {
			tr := forms[name]
			if tr == "" {
				continue
			}
			// Other is the plural source's equal; the rest may leave
			// out what it says, as "one file" does the count, but not
			// say more.
			checkText(add, k, name, m.Other, tr, name != "other")
		}
	}

	// Keys nobody passes: in a Go catalog with keys that are not
	// literals, they may be what it was thought to lack.
	if !c.Partial {
		for _, k := range sortedKeys(c.Messages) {
			if _, ok := s.Messages[k]; !ok {
				add(Unused, k, false, "%s is not used%s", strconv.Quote(k), elsewhere(s.Plurals, k, "a plural message"))
			}
		}
		for _, k := range sortedKeys(c.Plurals) {
			if _, ok := s.Plurals[k]; !ok {
				add(Unused, k, false, "plural %s is not used%s", strconv.Quote(k), elsewhere(s.Messages, k, "a message without plural forms"))
			}
		}
	}
	// Each kind together, the worst first.
	slices.SortStableFunc(ps, func(a, b Problem) int { return kindRank(a.Kind) - kindRank(b.Kind) })
	return ps
}

// kindOrder is the order problems are reported in.
var kindOrder = []Kind{Invalid, Missing, Verbs, NoOther, BadForm, Whitespace, Unused}

func kindRank(k Kind) int { return slices.Index(kindOrder, k) }

func elsewhere(m map[string]*Message, k, what string) string {
	if _, ok := m[k]; ok {
		return "; the sources pass it as " + what
	}
	return ""
}

// checkText compares a translation with its source: their format verbs
// and the space they begin and end with. With subset, the translation may
// leave verbs out.
func checkText(add func(Kind, string, bool, string, ...any), key, form, source, tr string, subset bool) {
	what := strconv.Quote(key)
	if form != "" {
		what += " (" + form + ")"
	}
	sv, tv := ParseVerbs(source), ParseVerbs(tr)
	if !sameVerbs(sv, tv, subset) {
		add(Verbs, key, true, "%s: the translation has %s; the source has %s", what, verbList(tv), verbList(sv))
	}
	if lead(source) != lead(tr) || trail(source) != trail(tr) {
		add(Whitespace, key, false, "%s: the translation %s", what, spaceDiff(source, tr))
	}
}

// Verb is a format verb: the argument it formats, counting from 1, and
// its letter, or * for a width or precision taken from an argument.
type Verb struct {
	Arg  int
	Verb rune
	Text string // as written, such as %[2]d
}

// ParseVerbs returns the verbs of a format, as fmt reads them: %% is no
// verb, an explicit index such as %[2]d sets the argument and those after
// it, and a * width or precision takes an argument of its own. What
// follows a verb's letter is text, so a Korean particle choice such as
// {을/를} just after %s is not part of it.
func ParseVerbs(format string) []Verb {
	var vs []Verb
	arg := 1
	for i := 0; i < len(format); {
		if format[i] != '%' {
			i++
			continue
		}
		start := i
		i++
		if i < len(format) && format[i] == '%' {
			i++
			continue
		}
		// index reads [n], setting arg.
		index := func() {
			if i < len(format) && format[i] == '[' {
				end := strings.IndexByte(format[i:], ']')
				if end > 0 {
					if n, err := strconv.Atoi(format[i+1 : i+end]); err == nil && n > 0 {
						arg = n
					}
					i += end + 1
				}
			}
		}
		for i < len(format) && strings.IndexByte("+-# 0", format[i]) >= 0 {
			i++
		}
		// width
		index()
		if i < len(format) && format[i] == '*' {
			vs = append(vs, Verb{Arg: arg, Verb: '*', Text: format[start : i+1]})
			arg++
			i++
		} else {
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				i++
			}
		}
		// precision
		if i < len(format) && format[i] == '.' {
			i++
			index()
			if i < len(format) && format[i] == '*' {
				vs = append(vs, Verb{Arg: arg, Verb: '*', Text: format[start : i+1]})
				arg++
				i++
			} else {
				for i < len(format) && format[i] >= '0' && format[i] <= '9' {
					i++
				}
			}
		}
		index()
		if i >= len(format) {
			vs = append(vs, Verb{Arg: arg, Verb: '!', Text: format[start:]})
			break
		}
		r, size := utf8.DecodeRuneInString(format[i:])
		i += size
		if !unicode.IsLetter(r) || r > unicode.MaxASCII {
			// fmt prints %!{(BADVERB)} or the like: a mistake, kept as
			// one so that it differs from the source.
			r = '!'
		}
		vs = append(vs, Verb{Arg: arg, Verb: r, Text: format[start:i]})
		arg++
	}
	return vs
}

// sameVerbs reports whether two formats format the same arguments the
// same way: the same verbs for the same arguments, in any order. With
// subset, b may lack some of a's.
func sameVerbs(a, b []Verb, subset bool) bool {
	count := map[[2]int]int{}
	for _, v := range a {
		count[[2]int{v.Arg, int(v.Verb)}]++
	}
	for _, v := range b {
		k := [2]int{v.Arg, int(v.Verb)}
		if count[k] == 0 {
			return false
		}
		count[k]--
	}
	if subset {
		return true
	}
	for _, n := range count {
		if n != 0 {
			return false
		}
	}
	return true
}

func verbList(vs []Verb) string {
	if len(vs) == 0 {
		return "no verbs"
	}
	s := make([]string, len(vs))
	for i, v := range vs {
		s[i] = v.Text
	}
	return strings.Join(s, ", ")
}

func lead(s string) string  { return s[:len(s)-len(strings.TrimLeftFunc(s, unicode.IsSpace))] }
func trail(s string) string { return s[len(strings.TrimRightFunc(s, unicode.IsSpace)):] }

// spaceDiff says how the space around tr differs from that around src.
func spaceDiff(src, tr string) string {
	var parts []string
	for _, e := range []struct {
		where string
		a, b  string
	}{{"begins", lead(src), lead(tr)}, {"ends", trail(src), trail(tr)}} {
		if e.a == e.b {
			continue
		}
		switch {
		case e.a == "":
			parts = append(parts, fmt.Sprintf("%s with %s, the source does not", e.where, strconv.Quote(e.b)))
		case e.b == "":
			parts = append(parts, fmt.Sprintf("does not %s with %s, as the source does", strings.TrimSuffix(e.where, "s"), strconv.Quote(e.a)))
		default:
			parts = append(parts, fmt.Sprintf("%s with %s, the source with %s", e.where, strconv.Quote(e.b), strconv.Quote(e.a)))
		}
	}
	return strings.Join(parts, "; ")
}
