package i18n

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
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
	cs := checkSet{c: c, s: s, o: o}
	keys, pkeys := s.Keys()
	for _, k := range keys {
		cs.checkMessage(k)
	}
	for _, k := range pkeys {
		cs.checkPlural(k)
	}
	cs.checkUnused()
	// Each kind together, the worst first.
	slices.SortStableFunc(cs.ps, func(a, b Problem) int { return kindRank(a.Kind) - kindRank(b.Kind) })
	return cs.ps
}

// checkSet is what Check compares: the catalog, the sources and the
// options, with the problems found so far.
type checkSet struct {
	c  *Catalog
	s  *Sources
	o  Options
	ps []Problem
}

func (cs *checkSet) add(kind Kind, key string, isErr bool, format string, args ...any) {
	cs.ps = append(cs.ps, Problem{Kind: kind, Key: key, Error: isErr, Detail: fmt.Sprintf(format, args...)})
}

func (cs *checkSet) checkMessage(k string) {
	m := cs.s.Messages[k]
	tr, ok := cs.c.Messages[k]
	switch {
	case !ok:
		cs.add(Missing, k, !cs.o.AllowMissing, "missing %s, used at %s", strconv.Quote(k), m.Uses[0])
	case cs.c.dynamic[k]:
	case tr == "":
		cs.add(Missing, k, !cs.o.AllowMissing, "%s is not translated (empty)", strconv.Quote(k))
	default:
		checkText(cs.add, k, "", m.Source, tr, false)
	}
}

func (cs *checkSet) checkPlural(k string) {
	m := cs.s.Plurals[k]
	forms, ok := cs.c.Plurals[k]
	switch {
	case !ok:
		cs.add(Missing, k, !cs.o.AllowMissing, "missing plural %s, used at %s", strconv.Quote(k), m.Uses[0])
		return
	case cs.c.dynamicPlural[k]:
		return
	}
	translated := false
	for _, f := range forms {
		if f != "" {
			translated = true
		}
	}
	if !translated {
		cs.add(Missing, k, !cs.o.AllowMissing, "plural %s is not translated (empty)", strconv.Quote(k))
		return
	}
	for _, name := range sortedKeys(forms) {
		if !slices.Contains(FormNames, name) {
			cs.add(BadForm, k, true, "plural %s has a form %q; the forms are %s", strconv.Quote(k), name, strings.Join(FormNames, ", "))
		}
	}
	if forms["other"] == "" {
		cs.add(NoOther, k, true, "plural %s has no other form, which every language uses", strconv.Quote(k))
	}
	for _, name := range FormNames {
		tr := forms[name]
		if tr == "" {
			continue
		}
		// Other is the plural source's equal; the rest may leave
		// out what it says, as "one file" does the count, but not
		// say more.
		checkText(cs.add, k, name, m.Other, tr, name != "other")
	}
}

func (cs *checkSet) checkUnused() {
	// Keys nobody passes: in a Go catalog with keys that are not
	// literals, they may be what it was thought to lack.
	if cs.c.Partial {
		return
	}
	for _, k := range sortedKeys(cs.c.Messages) {
		if _, ok := cs.s.Messages[k]; !ok {
			cs.add(Unused, k, false, "%s is not used%s", strconv.Quote(k), elsewhere(cs.s.Plurals, k, "a plural message"))
		}
	}
	for _, k := range sortedKeys(cs.c.Plurals) {
		if _, ok := cs.s.Plurals[k]; !ok {
			cs.add(Unused, k, false, "plural %s is not used%s", strconv.Quote(k), elsewhere(cs.s.Messages, k, "a message without plural forms"))
		}
	}
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
