package ggui

import (
	"math"
	"testing"

	"github.com/ironpark/ggui/internal/textedit"
)

func TestPrefixWidthsMatchMeasuringEachPrefix(t *testing.T) {
	t.Parallel()
	useTestEmoji(t)
	face := fallbackFont().face(20)
	for _, s := range []string{"", "Hello, world", "a b  c", "안녕하세요 세계", "été", "ok 👍🏽 then 👨‍👩‍👧‍👦!", "1️⃣2"} {
		width := prefixWidths(s, face)
		for b := 0; ; b = textedit.NextGrapheme(s, b) {
			if got, want := width(b), lineWidth(s[:b], face); math.Abs(got-want) > 0.5 {
				t.Errorf("%q[:%d]: width %v, measured %v", s, b, got, want)
			}
			if b >= len(s) {
				break
			}
		}
		// Inside a cluster the width grows with the runes before the
		// offset, and stays between the cluster's edges.
		last := 0.0
		for b := 0; ; b = textedit.NextRune(s, b) {
			w := width(b)
			if w < last {
				t.Errorf("%q[:%d]: width %v went back from %v", s, b, w, last)
			}
			last = w
			if b >= len(s) {
				break
			}
		}
	}
}
