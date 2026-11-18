package i18n

import (
	"strings"

	"github.com/ironpark/ggui"
)

// normalize returns tag lowercased with "-" between its subtags and any
// encoding or modifier dropped: "pt_BR.UTF-8" is "pt-br".
func normalize(tag string) string {
	if i := strings.IndexAny(tag, ".@"); i >= 0 {
		tag = tag[:i]
	}
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(tag), "_", "-"))
}

// base returns the language subtag of a normalized tag: "pt" of "pt-br".
func base(tag string) string {
	if i := strings.IndexByte(tag, '-'); i >= 0 {
		return tag[:i]
	}
	return tag
}

// fallbacks returns the tags a lookup for tag tries, most specific first:
// "zh-hant-tw", "zh-hant", "zh". It is empty for the empty tag.
func fallbacks(tag string) []string {
	tag = normalize(tag)
	var out []string
	for tag != "" {
		out = append(out, tag)
		i := strings.LastIndexByte(tag, '-')
		if i < 0 {
			break
		}
		tag = tag[:i]
	}
	return out
}

// Match returns the tag of supported that best serves a user who prefers
// the languages of preferred, most preferred first: for the first
// preference any supported tag serves, the supported tag equal to it, else
// one equal to it with subtags dropped ("pt" for "pt-BR"), else one of the
// same language ("en-US" for "en-GB"). It returns the first supported tag
// when no preference is served, and "" when supported is empty. Tags
// compare without regard to case or to "_" and "-".
//
//	lang := ggui.State(i18n.Match([]string{"en", "ko", "ja"}, ggui.SystemLanguages()))
func Match(supported, preferred []string) string {
	if len(supported) == 0 {
		return ""
	}
	norm := make([]string, len(supported))
	for i, s := range supported {
		norm[i] = normalize(s)
	}
	for _, p := range preferred {
		for _, f := range fallbacks(p) {
			for i, s := range norm {
				if s == f {
					return supported[i]
				}
			}
		}
		if b := base(normalize(p)); b != "" {
			for i, s := range norm {
				if base(s) == b {
					return supported[i]
				}
			}
		}
	}
	return supported[0]
}

// rtl lists the languages written right to left.
var rtl = map[string]bool{
	"ar": true, "arc": true, "ckb": true, "dv": true, "fa": true, "he": true,
	"iw": true, "ji": true, "ks": true, "ps": true, "sd": true,
	"syr": true, "ug": true, "ur": true, "yi": true,
}

// Direction returns the direction text in the language tag reads in:
// DirRTL for Arabic, Hebrew, Persian, Urdu and the other languages written
// right to left, or for a tag naming a right-to-left script ("az-Arab"),
// and DirLTR for the rest, the empty tag included. Provide and Install set
// it for the language they follow.
func Direction(tag string) ggui.TextDirection {
	t := normalize(tag)
	for _, sub := range strings.Split(t, "-")[1:] {
		switch sub {
		case "arab", "hebr", "thaa", "syrc", "nkoo", "adlm", "rohg", "mand", "samr":
			return ggui.DirRTL
		case "latn", "cyrl", "deva", "hans", "hant":
			return ggui.DirLTR
		}
	}
	if rtl[base(t)] {
		return ggui.DirRTL
	}
	return ggui.DirLTR
}
