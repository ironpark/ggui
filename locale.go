package ggui

import (
	"strings"
)

// SystemLanguages returns the languages the user prefers, most preferred
// first, as BCP 47 tags such as "ko-KR" and "en-US", for an app choosing
// which of its translations to show:
//
//	lang := i18n.Match([]string{"en", "ko", "ja"}, ggui.SystemLanguages())
//
// It asks the platform on every call, so a change the user made in the
// system's settings shows in the next one, and it may be called from any
// goroutine. Where the platform says nothing it returns none.
//
// macOS reads the list in Language & Region settings, Windows the user's
// preferred display languages, a browser navigator.languages, and other
// systems the LANGUAGE list and then the locale LC_ALL, LC_MESSAGES or
// LANG names, the POSIX "C" locale left out. Tags are written with
// hyphens, as in "zh-Hant-TW", and appear once each.
func SystemLanguages() []string {
	return normalizeLanguages(systemLanguages())
}

// normalizeLanguages writes each of tags as BCP 47 (see normalizeLanguage)
// and leaves out the empty ones, the POSIX locale and the repeats, keeping
// the order of the first of each.
func normalizeLanguages(tags []string) []string {
	out := []string{}
	for _, t := range tags {
		t = normalizeLanguage(t)
		if t == "" {
			continue
		}
		dup := false
		for _, o := range out {
			if strings.EqualFold(o, t) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, t)
		}
	}
	return out
}

// normalizeLanguage turns a language tag or a POSIX locale name into a
// BCP 47 tag in its usual case: "ko_KR.UTF-8@mod" becomes "ko-KR", and
// "zh_hant_tw" becomes "zh-Hant-TW". The POSIX locale, "C" or "POSIX",
// and an empty name become "".
func normalizeLanguage(tag string) string {
	tag = strings.TrimSpace(tag)
	// A POSIX name carries its encoding after a dot and a modifier after
	// an at sign, neither of which is part of the language.
	if i := strings.IndexAny(tag, ".@"); i >= 0 {
		tag = tag[:i]
	}
	if tag == "" || tag == "C" || tag == "POSIX" {
		return ""
	}
	parts := strings.FieldsFunc(tag, func(r rune) bool { return r == '_' || r == '-' })
	for i, p := range parts {
		switch {
		case i == 0:
			p = strings.ToLower(p)
		case len(p) == 4 && isLetters(p):
			// A script: Hant, Latn.
			p = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		case len(p) == 2 && isLetters(p), len(p) == 3 && !isLetters(p):
			// A region: KR, or 419 for Latin America.
			p = strings.ToUpper(p)
		default:
			p = strings.ToLower(p)
		}
		parts[i] = p
	}
	return strings.Join(parts, "-")
}

// isLetters reports whether s is ASCII letters alone.
func isLetters(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return s != ""
}

// posixLanguages is the language list of a POSIX environment, which getenv
// reads: the colon-separated LANGUAGE list GNU gettext follows, then the
// locale messages are shown in, which is the first of LC_ALL, LC_MESSAGES
// and LANG that is set. As in gettext, LANGUAGE counts for nothing while
// that locale is the POSIX one, which asks for untranslated text. The
// names are as the environment gives them, for normalizeLanguages to write
// as tags.
func posixLanguages(getenv func(string) string) []string {
	locale := ""
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if locale = getenv(name); locale != "" {
			break
		}
	}
	if locale != "" && normalizeLanguage(locale) == "" {
		return nil
	}
	var out []string
	for l := range strings.SplitSeq(getenv("LANGUAGE"), ":") {
		if l != "" {
			out = append(out, l)
		}
	}
	if locale != "" {
		out = append(out, locale)
	}
	return out
}
