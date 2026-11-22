package ui

import (
	"math"
	"slices"
	"strings"
	"unicode"

	"github.com/ironpark/ggui/pkg/hangul"
)

// The weights of commandScore, those of the command-score cmdk ranks its
// items with: a match that carries on from the last one keeps the whole
// score, one that jumps to the start of a word keeps most of it, and one
// that jumps into the middle of a word very little.
const (
	scoreContinue     = 1.0
	scoreSpaceJump    = .9  // to a letter after a space or a hyphen
	scoreGapJump      = .8  // to a letter after other punctuation
	scoreLetterJump   = .17 // to a letter inside a word
	scoreTransposed   = .1  // two letters typed the wrong way round
	penaltySkipped    = .999
	penaltyCase       = .9999
	penaltyIncomplete = .99 // the text goes on after the query ends
	// penaltyJamo is for a Hangul letter of the query that finds a
	// syllable by its keys, not as itself: see hangul.Match.
	penaltyJamo = .99
)

// commandScore is how well query abbreviates text, from 0 for not at all
// to 1 for the same text: the letters of query have to appear in text in
// their order, and score more the more of them run together or start a
// word. "tdt" finds "Toggle dark theme", and "dark" ranks it above
// "Drag marker".
//
// Korean is matched as it is typed: the query's last syllable, maybe
// still being composed, finds what its keys begin, and a lone consonant
// a syllable it begins, so "셋" and "ㅅㅅ" find "세션" (see hangul.Match).
// And a query typed in Hangul with the input method left on Korean finds
// what its keys spell on the 2-set layout, "ㄴㄷㅅ샤" finding "Settings".
func commandScore(text, query string) float64 {
	if text == "" {
		return 0
	}
	score := commandScoreOf(text, query)
	if keys, ok := hangul.Qwerty(query); ok {
		score = max(score, commandScoreOf(text, keys))
	}
	return score
}

// commandScoreOf is commandScore for query as it is.
func commandScoreOf(text, query string) float64 {
	s, q := []rune(text), []rune(query)
	if len(q) == 0 {
		if len(s) == 0 {
			return scoreContinue
		}
		return penaltyIncomplete
	}
	if len(s) == 0 {
		return 0
	}
	lq := lowerRunes(q)
	last := hangul.Keys([]rune{}, lq[len(lq)-1])
	m := scorer{s: s, q: q, ls: lowerRunes(s), lq: lq, last: last}
	if !m.canMatch() {
		return 0
	}
	m.memo = make([]float64, len(s)*len(q))
	for i := range m.memo {
		m.memo[i] = -1
	}
	return m.from(0, 0)
}

// scorer is one text scored against one query.
type scorer struct {
	s, q, ls, lq []rune
	last         []rune // the keys of the query's last rune
	memo         []float64
}

// canMatch reports in O(|s|) time whether q can match s under normal,
// transposed, doubled-letter, or Hangul transitions. It never returns false
// for a query that from(0, 0) would score > 0.
func (m *scorer) canMatch() bool {
	ls, lq := m.ls, m.lq
	qn := len(lq)
	if qn == 0 {
		return true
	}
	hi := 0
	for i := range ls {
		before := rune(-1)
		if i > 0 {
			before = ls[i-1]
		}
		if m.take(i, hi) > 0 {
			if hi+1 < qn && (before == lq[hi+1] || (lq[hi+1] == lq[hi] && before != lq[hi])) {
				hi += 2
			} else {
				hi++
			}
			if hi >= qn {
				return true
			}
		} else if hi > 0 && m.take(i, hi-1) > 0 {
			if before == lq[hi] || (lq[hi] == lq[hi-1] && before != lq[hi-1]) {
				hi++
				if hi >= qn {
					return true
				}
			}
		}
	}
	return false
}

// take is how many runes of the text from at the query's rune qi takes: 1
// for itself, as Hangul by its keys maybe 2, and 0 for none.
func (m *scorer) take(at, qi int) int {
	if m.ls[at] == m.lq[qi] {
		return 1
	}
	if !hangul.IsHangul(m.lq[qi]) {
		return 0
	}
	var last []rune
	if qi == len(m.lq)-1 {
		last = m.last
	}
	return hangul.Match(m.ls, at, m.lq[qi], last)
}

// commandText is what an entry is scored on, as cmdk builds it: the label
// and then the keywords, each trimmed and a space apart. An entry with no
// label is not found, whatever its keywords.
func commandText(label string, keywords []string) string {
	text := strings.TrimSpace(label)
	if text == "" {
		return ""
	}
	for _, k := range keywords {
		text += " " + strings.TrimSpace(k)
	}
	return text
}

// lowerRunes is s in lower case with a hyphen or any space as one space,
// rune for rune.
func lowerRunes(s []rune) []rune {
	out := make([]rune, len(s))
	for i, r := range s {
		if isSpaceBreak(r) {
			out[i] = ' '
		} else {
			out[i] = unicode.ToLower(r)
		}
	}
	return out
}

func isSpaceBreak(r rune) bool { return r == '-' || unicode.IsSpace(r) }

func isGapBreak(r rune) bool { return strings.ContainsRune(`\/_+.#"@[({&`, r) }

// from scores q[qi:] against s[si:].
func (m *scorer) from(si, qi int) float64 {
	s, q, ls, lq := m.s, m.q, m.ls, m.lq
	if qi == len(q) {
		if si == len(s) {
			return scoreContinue
		}
		return penaltyIncomplete
	}
	if si >= len(s) {
		return 0
	}
	idx := si*len(q) + qi
	if v := m.memo[idx]; v >= 0 {
		return v
	}
	high := 0.0
	gaps, spaces := 0, 0
	for at := si; at < len(s); at++ {
		if at >= si+2 {
			prev := s[at-2]
			if isGapBreak(prev) {
				gaps++
			}
			if isSpaceBreak(prev) {
				spaces++
			}
		}
		n := m.take(at, qi)
		if n == 0 {
			continue
		}
		score := m.from(at+n, qi+1)
		if n > 1 || ls[at] != lq[qi] {
			score *= penaltyJamo
		}
		if score > high {
			switch {
			case at == si:
				score *= scoreContinue
			case isGapBreak(s[at-1]):
				score *= scoreGapJump
				if gaps > 0 && si > 0 {
					score *= math.Pow(penaltySkipped, float64(gaps))
				}
			case isSpaceBreak(s[at-1]):
				score *= scoreSpaceJump
				if spaces > 0 && si > 0 {
					score *= math.Pow(penaltySkipped, float64(spaces))
				}
			default:
				score *= scoreLetterJump
				if si > 0 {
					score *= math.Pow(penaltySkipped, float64(at-si))
				}
			}
			if s[at] != q[qi] {
				score *= penaltyCase
			}
		}
		// Two letters the wrong way round still find the text, for little,
		// and so does a letter the query doubles. before is the letter
		// ahead of this one, none at the start of the text, where cmdk
		// compares an empty string.
		before := rune(-1)
		if at > 0 {
			before = ls[at-1]
		}
		if qi+1 < len(q) &&
			(score < scoreTransposed && before == lq[qi+1] || lq[qi+1] == lq[qi] && before != lq[qi]) {
			if swapped := m.from(at+1, qi+2) * scoreTransposed; swapped > score {
				score = swapped
			}
		}
		high = max(high, score)
	}
	m.memo[idx] = high
	return high
}

// rank filters the command's entries by how well query abbreviates each
// one's label and keywords, and orders what is left as cmdk does: the
// entries outside any group first, then the groups, the group with the best
// match first; and inside each of those the best match first. Entries that
// score the same keep the order they were given in.
func (c *CommandWidget) rank(query string) []int {
	type hit struct {
		index int
		score float64
	}
	var loose []hit
	var names []string
	groups := map[string][]hit{}
	for i, e := range c.entries {
		if e.line {
			continue
		}
		score := commandScore(commandText(e.label, e.keywords), query)
		if score <= 0 {
			continue
		}
		if e.group == "" {
			loose = append(loose, hit{i, score})
			continue
		}
		if _, ok := groups[e.group]; !ok {
			names = append(names, e.group)
		}
		groups[e.group] = append(groups[e.group], hit{i, score})
	}
	best := func(a, b hit) int {
		switch {
		case a.score > b.score:
			return -1
		case a.score < b.score:
			return 1
		}
		return 0
	}
	slices.SortStableFunc(loose, best)
	for _, hits := range groups {
		slices.SortStableFunc(hits, best)
	}
	slices.SortStableFunc(names, func(a, b string) int { return best(groups[a][0], groups[b][0]) })
	out := make([]int, 0, len(c.entries))
	for _, h := range loose {
		out = append(out, h.index)
	}
	for _, name := range names {
		for _, h := range groups[name] {
			out = append(out, h.index)
		}
	}
	return out
}
