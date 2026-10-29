package ggui

import (
	"sync"
	"unicode/utf8"
	"unsafe"

	"github.com/go-text/typesetting/segmenter"
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
	for _, e := range c.entries {
		if len(e.s) == len(p) && unsafe.StringData(e.s) == unsafe.StringData(p) {
			return e.breaks
		}
	}
	breaks := findLineBreaks(p)
	c.entries[c.next] = lineBreakEntry{s: p, breaks: breaks}
	c.next = (c.next + 1) % len(c.entries)
	return breaks
}

// lineBreakCache holds the breaks of the strings lineBreaks found last. An
// entry keeps its string alive, so a string at the same address and length
// is the same string.
var lineBreakCache struct {
	sync.Mutex
	entries [16]lineBreakEntry
	next    int
}

type lineBreakEntry struct {
	s      string
	breaks []int
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
