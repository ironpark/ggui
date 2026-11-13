package ui

import (
	"math"
	"slices"
	"strings"
	"unicode"
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
)

// commandScore is how well query abbreviates text, from 0 for not at all
// to 1 for the same text: the letters of query have to appear in text in
// their order, and score more the more of them run together or start a
// word. "tdt" finds "Toggle dark theme", and "dark" ranks it above
// "Drag marker".
func commandScore(text, query string) float64 {
	s, q := []rune(text), []rune(query)
	return commandScoreFrom(s, q, lowerRunes(s), lowerRunes(q), 0, 0, map[[2]int]float64{})
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

// countRunes counts the runes of s that is reports true for.
func countRunes(s []rune, is func(rune) bool) int {
	n := 0
	for _, r := range s {
		if is(r) {
			n++
		}
	}
	return n
}

// commandScoreFrom scores q[qi:] against s[si:].
func commandScoreFrom(s, q, ls, lq []rune, si, qi int, memo map[[2]int]float64) float64 {
	if qi == len(q) {
		if si == len(s) {
			return scoreContinue
		}
		return penaltyIncomplete
	}
	key := [2]int{si, qi}
	if v, ok := memo[key]; ok {
		return v
	}
	high := 0.0
	for at := si; at < len(s); at++ {
		if ls[at] != lq[qi] {
			continue
		}
		score := commandScoreFrom(s, q, ls, lq, at+1, qi+1, memo)
		if score > high {
			switch {
			case at == si:
				score *= scoreContinue
			case isGapBreak(s[at-1]):
				score *= scoreGapJump
				if n := countRunes(s[si:at-1], isGapBreak); n > 0 && si > 0 {
					score *= math.Pow(penaltySkipped, float64(n))
				}
			case isSpaceBreak(s[at-1]):
				score *= scoreSpaceJump
				if n := countRunes(s[si:at-1], isSpaceBreak); n > 0 && si > 0 {
					score *= math.Pow(penaltySkipped, float64(n))
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
		// Two letters the wrong way round still find the text, for little.
		if at > 0 && qi+1 < len(q) &&
			(score < scoreTransposed && ls[at-1] == lq[qi+1] || lq[qi+1] == lq[qi] && ls[at-1] != lq[qi]) {
			if swapped := commandScoreFrom(s, q, ls, lq, at+1, qi+2, memo) * scoreTransposed; swapped > score {
				score = swapped
			}
		}
		high = max(high, score)
	}
	memo[key] = high
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
		text := e.label
		if len(e.keywords) > 0 {
			text += " " + strings.Join(e.keywords, " ")
		}
		score := commandScore(text, query)
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
