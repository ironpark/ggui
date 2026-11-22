package i18n

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Josa returns the Korean particle that follows word: withFinal after a
// syllable that ends in a consonant, without after one that ends in a
// vowel. A particle starting 으로 (으로, 으로서, 으로써) takes its without
// form after ㄹ as well, as 로 does. A word ending in digits is read as
// Korean reads the number ("3" 삼, "10" 십, "10000" 만), and a word ending
// in anything else that is not Hangul, a Latin name say, takes without.
// Closing quotes and brackets at the end are skipped.
//
//	i18n.Josa("사과", "을", "를")   // "를"
//	i18n.Josa("수박", "을", "를")   // "을"
//	i18n.Josa("서울", "으로", "로") // "로"
//	i18n.Josa("3", "이", "가")      // "이": 삼
//
// A Korean catalog needs it rarely: a format verb directly followed by
// the pair in braces, "%s{을/를} 삭제할까요?", is given the particle that
// fits the argument it formats.
func Josa(word, withFinal, without string) string {
	final, ok := finalConsonant(strings.TrimRight(word, " \t\n\"')]}>»”’」』》〉"))
	if !ok || final == 0 {
		return without
	}
	if final == rieul && strings.HasPrefix(withFinal, "으로") {
		return without
	}
	return withFinal
}

// The final consonants, by their index in a Hangul syllable, that the
// rules above name.
const (
	giyeok = 1
	nieun  = 4
	rieul  = 8
	mieum  = 16
	bieup  = 17
	ieung  = 21
)

// digitFinals is the final consonant of each digit read in Korean: 영, 일,
// 이, 삼, 사, 오, 육, 칠, 팔, 구.
var digitFinals = [10]int{ieung, rieul, 0, mieum, 0, 0, giyeok, rieul, rieul, 0}

// finalConsonant returns the index of the final consonant word ends in,
// 0 for none, and whether its end is something Korean reads: a Hangul
// syllable or a number.
func finalConsonant(word string) (int, bool) {
	r, _ := utf8.DecodeLastRuneInString(word)
	switch {
	case r >= 0xAC00 && r <= 0xD7A3:
		return int(r-0xAC00) % 28, true
	case r >= '0' && r <= '9':
		return numberFinal(word), true
	}
	return 0, false
}

// numberFinal returns the final consonant of the number word ends in as
// Korean reads it: by its last digit that is not 0, or by the unit its
// zeros make, 십, 백, 천, 만, 억, 조.
func numberFinal(word string) int {
	zeros := 0
scan:
	for i := len(word) - 1; i >= 0; i-- {
		switch c := word[i]; {
		case c == ',':
		case c == '0':
			zeros++
		case c >= '1' && c <= '9':
			return unitFinal(zeros, int(c-'0'))
		default:
			break scan
		}
	}
	if zeros > 0 {
		// Only zeros, or zeros after something not a digit ("v2.0"): 영.
		return ieung
	}
	return 0
}

// unitFinal returns the final consonant of a number ending in digit d
// followed by zeros zeros.
func unitFinal(zeros, d int) int {
	switch {
	case zeros == 0:
		return digitFinals[d]
	case zeros == 1:
		return bieup // 십
	case zeros == 2:
		return giyeok // 백
	case zeros == 3:
		return nieun // 천
	case zeros < 8:
		return nieun // 만
	case zeros < 12:
		return giyeok // 억
	case zeros < 16:
		return 0 // 조
	}
	return ieung // 경
}

// particle reports whether s starts with a particle pair, "{을/를}", and
// returns its two forms and its length.
func particle(s string) (withFinal, without string, n int, ok bool) {
	if !strings.HasPrefix(s, "{") {
		return "", "", 0, false
	}
	end := strings.IndexByte(s, '}')
	if end < 0 {
		return "", "", 0, false
	}
	body := s[1:end]
	withFinal, without, found := strings.Cut(body, "/")
	if !found || withFinal == "" || without == "" || strings.ContainsAny(body, "{% \t\n") || strings.Contains(without, "/") {
		return "", "", 0, false
	}
	return withFinal, without, end + 1, true
}

// sprintfKorean formats like fmt.Sprintf, giving each particle pair that
// directly follows a verb the form that fits what the verb formatted. A
// verb with a * width or precision it cannot follow makes it format the
// whole with fmt, the pairs written "을(를)".
func sprintfKorean(format string, args []any) string {
	var b strings.Builder
	argNum := 0
	for i := 0; i < len(format); {
		c := format[i]
		if c != '%' {
			b.WriteByte(c)
			i++
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			b.WriteByte('%')
			i += 2
			continue
		}
		spec, index, verb, n, ok := directive(format[i:])
		if !ok {
			return fmt.Sprintf(writtenOut(format), args...)
		}
		if verb == 0 {
			b.WriteString("%!(NOVERB)")
			i += n
			continue
		}
		if index > 0 {
			argNum = index - 1
		}
		var text string
		if argNum >= 0 && argNum < len(args) {
			text = fmt.Sprintf(spec, args[argNum])
		} else {
			text = fmt.Sprintf(spec, []any(nil)...) // fmt says the argument is missing
		}
		argNum++
		b.WriteString(text)
		i += n
		if withFinal, without, pn, ok := particle(format[i:]); ok {
			b.WriteString(Josa(text, withFinal, without))
			i += pn
		}
	}
	return b.String()
}

// directive parses the fmt verb s starts with, "%-5.2[3]f" say: the verb
// with its flags, width and precision but without an argument index, the
// index (1-based, 0 for none), the verb's rune and the length of it all.
// The verb is 0 when s ends before it. It fails on a * width or
// precision.
func directive(s string) (spec string, index int, verb rune, n int, ok bool) {
	j := 1
	for j < len(s) && strings.IndexByte("+-# 0", s[j]) >= 0 {
		j++
	}
	var sb strings.Builder
	sb.WriteString(s[:j])
	if index, j, ok = readArgIndex(s, j, index); !ok {
		return "", 0, 0, 0, false
	}
	start := j
	j = skipDigits(s, j)
	if j < len(s) && s[j] == '.' {
		j = skipDigits(s, j+1)
	}
	if j < len(s) && s[j] == '*' {
		return "", 0, 0, 0, false
	}
	sb.WriteString(s[start:j])
	if index, j, ok = readArgIndex(s, j, index); !ok {
		return "", 0, 0, 0, false
	}
	if j >= len(s) {
		return "", index, 0, j, true
	}
	verb, size := utf8.DecodeRuneInString(s[j:])
	sb.WriteRune(verb)
	return sb.String(), index, verb, j + size, true
}

// readArgIndex reads "[n]", the argument index, at s[j]; j and index are
// unchanged when there is none. It fails on a bad index.
func readArgIndex(s string, j, index int) (int, int, bool) {
	if j >= len(s) || s[j] != '[' {
		return index, j, true
	}
	end := strings.IndexByte(s[j:], ']')
	if end < 0 {
		return 0, 0, false
	}
	v, err := strconv.Atoi(s[j+1 : j+end])
	if err != nil || v < 1 {
		return 0, 0, false
	}
	return v, j + end + 1, true
}

// skipDigits returns the first index at or after j whose byte is not a
// digit.
func skipDigits(s string, j int) int {
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	return j
}

// writtenOut returns format with each particle pair written as Korean
// writes a pair it cannot choose from: "{을/를}" as "을(를)".
func writtenOut(format string) string {
	var b strings.Builder
	for i := 0; i < len(format); {
		if withFinal, without, n, ok := particle(format[i:]); ok {
			b.WriteString(withFinal + "(" + without + ")")
			i += n
			continue
		}
		b.WriteByte(format[i])
		i++
	}
	return b.String()
}
