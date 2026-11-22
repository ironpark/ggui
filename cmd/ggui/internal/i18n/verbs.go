package i18n

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

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
	sc := &verbScan{format: format, arg: 1}
	for i := 0; i < len(format); {
		if format[i] != '%' {
			i++
			continue
		}
		next, ok := sc.verb(i)
		if !ok {
			break
		}
		i = next
	}
	return sc.vs
}

// verbScan is the state of one ParseVerbs: the verbs found, the argument
// the next verb formats, and the format being read.
type verbScan struct {
	format   string
	vs       []Verb
	arg      int
	start    int
	badIndex bool
}

// verb reads the verb that starts at i, a '%', and returns where the
// format continues. It is false when the format ends inside it.
func (sc *verbScan) verb(i int) (int, bool) {
	sc.start = i
	sc.badIndex = false
	i++
	if i < len(sc.format) && sc.format[i] == '%' {
		return i + 1, true
	}
	for i < len(sc.format) && strings.IndexByte("+-# 0", sc.format[i]) >= 0 {
		i++
	}
	// width
	i = sc.scanMeasure(i)
	// precision
	if i < len(sc.format) && sc.format[i] == '.' {
		i++
		i = sc.scanMeasure(i)
	}
	i = sc.scanIndex(i)
	return sc.verbLetter(i)
}

// scanIndex reads an explicit argument index, [n], which sets the
// argument the next verb formats.
func (sc *verbScan) scanIndex(i int) int {
	if i >= len(sc.format) || sc.format[i] != '[' {
		return i
	}
	end := strings.IndexByte(sc.format[i:], ']')
	if end < 0 {
		sc.badIndex = true
		return len(sc.format)
	}
	idx := sc.format[i+1 : i+end]
	if n, err := strconv.Atoi(idx); err == nil && n > 0 && digitsOnly(idx) {
		sc.arg = n
	} else {
		sc.badIndex = true
	}
	return i + end + 1
}

// digitsOnly reports whether s is all digits.
func digitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for j := 0; j < len(s); j++ {
		if s[j] < '0' || s[j] > '9' {
			return false
		}
	}
	return true
}

// scanMeasure reads a width or precision: an index, then a * that takes
// an argument or digits.
func (sc *verbScan) scanMeasure(i int) int {
	i = sc.scanIndex(i)
	if i < len(sc.format) && sc.format[i] == '*' {
		return sc.star(i)
	}
	for i < len(sc.format) && sc.format[i] >= '0' && sc.format[i] <= '9' {
		i++
	}
	return i
}

// star reads a * width or precision at i, which takes an argument of its
// own, and returns where the format continues.
func (sc *verbScan) star(i int) int {
	v := '*'
	if sc.badIndex {
		v = '!'
		sc.badIndex = false
	}
	sc.vs = append(sc.vs, Verb{Arg: sc.arg, Verb: v, Text: sc.format[sc.start : i+1]})
	sc.arg++
	return i + 1
}

// verbLetter reads the verb's letter at i, or reports the format ended
// inside the verb.
func (sc *verbScan) verbLetter(i int) (int, bool) {
	if i >= len(sc.format) {
		sc.vs = append(sc.vs, Verb{Arg: sc.arg, Verb: '!', Text: sc.format[sc.start:]})
		return i, false
	}
	r, size := utf8.DecodeRuneInString(sc.format[i:])
	i += size
	if sc.badIndex || !unicode.IsLetter(r) || r > unicode.MaxASCII {
		// fmt prints %!{(BADVERB)} or %!(BADINDEX): a mistake, kept as
		// one so that it differs from the source.
		r = '!'
	}
	sc.vs = append(sc.vs, Verb{Arg: sc.arg, Verb: r, Text: sc.format[sc.start:i]})
	sc.arg++
	return i, true
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
