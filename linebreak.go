package ggui

import (
	"sync"
	"unicode/utf8"

	"github.com/go-text/typesetting/segmenter"

	"github.com/ironpark/ggui/internal/textedit"
)

// lineBreaks returns where p may be broken across lines, by the rules of
// UAX #14 as go-text's segmenter finds them: the byte offsets after each
// break opportunity, the last being len(p). The spaces before an
// opportunity belong to the text before it, so a line broken there ends
// with them. Breaks are allowed after spaces and hyphens, between
// ideographs, and not before closing punctuation or after opening
// punctuation. Korean breaks at spaces only, as CSS's word-break: keep-all
// has it: UAX #14 would break between any two syllables.
//
// The breaks of the strings last asked about are kept, since text wraps
// again at each width it is laid out at, and finding them takes as long as
// the rest of wrapping. The slice returned is shared: do not change it.
func lineBreaks(p string) []int {
	c := &lineBreakCache
	c.Lock()
	defer c.Unlock()
	if breaks, ok := c.cur[p]; ok {
		return breaks
	}
	breaks, ok := c.prev[p]
	if !ok {
		breaks = findLineBreaks(p)
	}
	if len(c.cur) >= lineBreakCacheSize {
		// Forget what was not asked about since the last turnover.
		c.prev, c.cur = c.cur, make(map[string][]int, lineBreakCacheSize)
	}
	if c.cur == nil {
		c.cur = make(map[string][]int, lineBreakCacheSize)
	}
	c.cur[p] = breaks
	return breaks
}

// lineBreakCacheSize is how many paragraphs' breaks lineBreakCache holds at
// least: every paragraph of a window of text, which a resize wraps again
// all at once.
const lineBreakCacheSize = 1024

// lineBreakCache holds the breaks of the strings lineBreaks found last, in
// two generations: a lookup that finds its string in prev moves it to cur,
// and when cur is full, it becomes prev and the old prev is dropped.
var lineBreakCache struct {
	sync.Mutex
	cur, prev map[string][]int
}

func findLineBreaks(p string) []int {
	// Decode by hand rather than with InitWithString, so that a byte of
	// invalid UTF-8 is one rune and every rune has its offset.
	runes := make([]rune, 0, len(p))
	offsets := make([]int, 0, len(p)+1)
	for i := 0; i < len(p); {
		r, n := utf8.DecodeRuneInString(p[i:])
		runes = append(runes, r)
		offsets = append(offsets, i)
		i += n
	}
	offsets = append(offsets, len(p))
	var seg segmenter.Segmenter
	seg.Init(runes)
	var breaks []int
	for it := seg.LineIterator(); it.Next(); {
		l := it.Line()
		end := l.Offset + len(l.Text)
		if end < len(runes) && isHangul(runes[end-1]) && isHangul(runes[end]) {
			continue
		}
		// UAX #14 and #29 class some runes apart: an emoji modifier after a
		// combining mark is a break opportunity, though the two make one
		// grapheme cluster. A line never breaks a cluster.
		if b := offsets[end]; b < len(p) && textedit.NextGrapheme(p, textedit.PrevGrapheme(p, b)) != b {
			continue
		}
		breaks = append(breaks, offsets[end])
	}
	if len(breaks) == 0 || breaks[len(breaks)-1] != len(p) {
		breaks = append(breaks, len(p))
	}
	return breaks
}

// isHangul reports whether r is a Hangul syllable or jamo.
func isHangul(r rune) bool {
	return r >= 0xAC00 && r <= 0xD7A3 || r >= 0x1100 && r <= 0x11FF || r >= 0x3130 && r <= 0x318F ||
		r >= 0xA960 && r <= 0xA97F || r >= 0xD7B0 && r <= 0xD7FF
}
