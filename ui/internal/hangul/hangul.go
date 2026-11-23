// Package hangul matches Korean text the way it is typed.
//
// A Korean word is typed a jamo at a time, and the input method composes
// the jamo into syllables as it goes, so what is typed partway seldom
// equals what it will become: "세션" is "셋" a key before its end, its last
// consonant taken as a final until the vowel after it moves it on. A
// search that compares runes misses it until the word is done. Match
// compares the last rune of a query by its keys instead, and a lone
// consonant before it as the initial of a syllable, so that "ㅅㅅ" finds
// "세션" as well, as Korean users expect of a search.
//
// And English typed with the input method left on Korean comes as Hangul:
// Qwerty reads it back as the keys that typed it on the 2-set layout.
package hangul

import "slices"

const (
	syllableBase = 0xAC00
	syllableLast = 0xD7A3
	jungCount    = 21
	jongCount    = 28
)

var (
	choRunes  = []rune("ㄱㄲㄴㄷㄸㄹㅁㅂㅃㅅㅆㅇㅈㅉㅊㅋㅌㅍㅎ")
	jungRunes = []rune("ㅏㅐㅑㅒㅓㅔㅕㅖㅗㅘㅙㅚㅛㅜㅝㅞㅟㅠㅡㅢㅣ")
	jongRunes = []rune(" ㄱㄲㄳㄴㄵㄶㄷㄹㄺㄻㄼㄽㄾㄿㅀㅁㅂㅄㅅㅆㅇㅈㅊㅋㅌㅍㅎ")
	// compounds are the jamo typed as two keys, the compound vowels and the
	// compound finals, by the keys.
	compounds = map[rune][]rune{
		'ㅘ': []rune("ㅗㅏ"), 'ㅙ': []rune("ㅗㅐ"), 'ㅚ': []rune("ㅗㅣ"), 'ㅝ': []rune("ㅜㅓ"),
		'ㅞ': []rune("ㅜㅔ"), 'ㅟ': []rune("ㅜㅣ"), 'ㅢ': []rune("ㅡㅣ"),
		'ㄳ': []rune("ㄱㅅ"), 'ㄵ': []rune("ㄴㅈ"), 'ㄶ': []rune("ㄴㅎ"), 'ㄺ': []rune("ㄹㄱ"),
		'ㄻ': []rune("ㄹㅁ"), 'ㄼ': []rune("ㄹㅂ"), 'ㄽ': []rune("ㄹㅅ"), 'ㄾ': []rune("ㄹㅌ"),
		'ㄿ': []rune("ㄹㅍ"), 'ㅀ': []rune("ㄹㅎ"), 'ㅄ': []rune("ㅂㅅ"),
	}
)

// IsHangul reports whether r is a syllable or a compatibility jamo, the
// runes Keys finds keys in.
func IsHangul(r rune) bool {
	return r >= syllableBase && r <= syllableLast || r >= 'ㄱ' && r <= 'ㅣ'
}

// Keys appends to dst the jamo keys r is typed with, in order: a
// syllable's initial, vowel and final, a compound vowel or final as its
// two, a lone jamo itself; and nothing for a rune that is not Hangul.
//
//	Keys(nil, '셋') // ㅅ ㅔ ㅅ
//	Keys(nil, '왜') // ㅇ ㅗ ㅐ
func Keys(dst []rune, r rune) []rune {
	key := func(j rune) {
		if c, ok := compounds[j]; ok {
			dst = append(dst, c...)
		} else if j != ' ' {
			dst = append(dst, j)
		}
	}
	switch {
	case r >= syllableBase && r <= syllableLast:
		i := int(r - syllableBase)
		key(choRunes[i/(jungCount*jongCount)])
		key(jungRunes[i/jongCount%jungCount])
		key(jongRunes[i%jongCount])
	case r >= 'ㄱ' && r <= 'ㅣ':
		key(r)
	}
	return dst
}

// Initial is a syllable's initial consonant, 0 for any other rune.
func Initial(r rune) rune {
	if r < syllableBase || r > syllableLast {
		return 0
	}
	return choRunes[int(r-syllableBase)/(jungCount*jongCount)]
}

// Match reports how many runes of s, from s[i], the query's rune q takes
// by its keys, 0 for none: a search calls it where q is not s[i] itself.
//
// For the query's last rune, which may still be being typed, last is its
// Keys: it takes the syllable at s[i] when its keys begin that syllable's,
// and the next syllable too when they reach into it, "셋" taking "세션"'s
// two. For any rune before it, last is nil: a lone consonant takes a
// syllable it is the initial of.
func Match(s []rune, i int, q rune, last []rune) int {
	if last == nil {
		if q >= 'ㄱ' && q <= 'ㅎ' && Initial(s[i]) == q {
			return 1
		}
		return 0
	}
	if len(last) == 0 {
		return 0
	}
	var buf [10]rune // a syllable is at most 5 keys, and the next too
	have := Keys(buf[:0], s[i])
	n := len(have)
	if n == 0 {
		return 0
	}
	if i+1 < len(s) {
		have = Keys(have, s[i+1])
	}
	if len(last) > len(have) || !slices.Equal(last, have[:len(last)]) {
		return 0
	}
	if len(last) > n {
		return 2
	}
	return 1
}

// qwertyKeys are the letters of the keys the 2-set layout types each jamo
// with, the shifted ones read in lower case as the rest.
var qwertyKeys = map[rune]rune{
	'ㅂ': 'q', 'ㅈ': 'w', 'ㄷ': 'e', 'ㄱ': 'r', 'ㅅ': 't', 'ㅛ': 'y', 'ㅕ': 'u', 'ㅑ': 'i', 'ㅐ': 'o', 'ㅔ': 'p',
	'ㅁ': 'a', 'ㄴ': 's', 'ㅇ': 'd', 'ㄹ': 'f', 'ㅎ': 'g', 'ㅗ': 'h', 'ㅓ': 'j', 'ㅏ': 'k', 'ㅣ': 'l',
	'ㅋ': 'z', 'ㅌ': 'x', 'ㅊ': 'c', 'ㅍ': 'v', 'ㅠ': 'b', 'ㅜ': 'n', 'ㅡ': 'm',
	'ㅃ': 'q', 'ㅉ': 'w', 'ㄸ': 'e', 'ㄲ': 'r', 'ㅆ': 't', 'ㅒ': 'o', 'ㅖ': 'p',
}

// Qwerty is s with its Hangul read as the keys that typed it on the 2-set
// Korean layout, as a query typed with the input method left on Korean
// was meant in English, and whether s had any Hangul.
//
//	Qwerty("ㄴㄷㅅ샤") // "setti", true
func Qwerty(s string) (string, bool) {
	var out, ks []rune
	hangul := false
	for _, r := range s {
		ks = Keys(ks[:0], r)
		if len(ks) == 0 {
			out = append(out, r)
			continue
		}
		hangul = true
		for _, k := range ks {
			if l, ok := qwertyKeys[k]; ok {
				out = append(out, l)
			}
		}
	}
	return string(out), hangul
}
