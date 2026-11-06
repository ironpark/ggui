package textedit

import (
	"slices"
	"testing"
)

// clusters splits s by stepping NextGrapheme from the start.
func clusters(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		j := NextGrapheme(s, i)
		out = append(out, s[i:j])
		i = j
	}
	return out
}

func TestClustersKeepAttachedRunesWithTheirBase(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		s    string
		want []string
	}{
		{"text variation selector", "❤\ufe0ex", []string{"❤\ufe0e", "x"}},
		{"ideographic variation selector", "葛\U000E0100x", []string{"葛\U000E0100", "x"}},
		{"skin tone", "\U0001F44D\U0001F3FDx", []string{"\U0001F44D\U0001F3FD", "x"}},
		{"tag flag", "\U0001F3F4\U000E0067\U000E0062\U000E0073\U000E0063\U000E0074\U000E007Fx",
			[]string{"\U0001F3F4\U000E0067\U000E0062\U000E0073\U000E0063\U000E0074\U000E007F", "x"}},
		{"zero-width non-joiner", "a\u200cb", []string{"a\u200c", "b"}},
		{"conjoining jamo", "각가", []string{"각", "가"}},
		{"keycap", "1\ufe0f\u20e3#", []string{"1\ufe0f\u20e3", "#"}},
		{"lone regional indicator", "\U0001F1F0x", []string{"\U0001F1F0", "x"}},
		{"odd regional indicators", "\U0001F1F0\U0001F1F7\U0001F1FA", []string{"\U0001F1F0\U0001F1F7", "\U0001F1FA"}},
		{"lone line feed", "a\nb", []string{"a", "\n", "b"}},
		{"CR then LF", "\r\r\n", []string{"\r", "\r\n"}},
		{"mark at the start", "\u0301a", []string{"\u0301", "a"}},
		// What the segmenter gets right that runes alone do not tell.
		{"Devanagari conjunct", "क्षa", []string{"क्ष", "a"}},
		{"leading and vowel jamo", "\u1100\u1161\u1100", []string{"\u1100\u1161", "\u1100"}},
		{"prepended concatenation mark", "\u0600١٢x", []string{"\u0600١", "٢", "x"}},
		{"spacing mark", "कि", []string{"कि"}},
		{"mark after a tab", "\t\u0301", []string{"\t", "\u0301"}},
		{"ZWJ sequences in a row", "👩🏽\u200d💻👩🏽\u200d💻👨\u200d👩\u200d👧\u200d👦x",
			[]string{"👩🏽\u200d💻", "👩🏽\u200d💻", "👨\u200d👩\u200d👧\u200d👦", "x"}},
		{"joiner after a letter", "a\u200d💻", []string{"a\u200d", "💻"}},
	} {
		got := clusters(c.s)
		if len(got) != len(c.want) {
			t.Errorf("%s: %q splits into %q, want %q", c.name, c.s, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: %q splits into %q, want %q", c.name, c.s, got, c.want)
				break
			}
		}
	}
}

func TestPrevGraphemeStepsBackOverLineBreaks(t *testing.T) {
	t.Parallel()
	s := "a\nb\r\nc"
	want := []int{6, 5, 3, 2, 1, 0}
	i := len(s)
	for _, w := range want[1:] {
		i = PrevGrapheme(s, i)
		if i != w {
			t.Fatalf("PrevGrapheme(%q) stepped to %d, want %d (steps %v)", s, i, w, want)
		}
	}
}

func TestStepsClampAtTextEdges(t *testing.T) {
	t.Parallel()
	s := "héllo"
	for _, c := range []struct {
		name      string
		got, want int
	}{
		{"NextGrapheme at end", NextGrapheme(s, len(s)), len(s)},
		{"NextGrapheme past end", NextGrapheme(s, len(s)+3), len(s)},
		{"PrevGrapheme at start", PrevGrapheme(s, 0), 0},
		{"PrevGrapheme before start", PrevGrapheme(s, -1), 0},
		{"NextRune at end", NextRune(s, len(s)), len(s)},
		{"NextRune over é", NextRune(s, 1), 3},
		{"prevRune at start", prevRune(s, 0), 0},
		{"prevRune over é", prevRune(s, 3), 1},
		{"nextWord at end", nextWord(s, len(s)), len(s)},
		{"prevWord at start", prevWord(s, 0), 0},
		{"prevWord over leading spaces", prevWord("   x", 3), 0},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

// Word movement and double-click take a letter with its marks: Devanagari,
// Thai and decomposed Latin put them after it.
func TestWordMovementKeepsMarksWithTheirBase(t *testing.T) {
	t.Parallel()
	s := "café x"
	if got, want := nextWord(s, 0), len("café "); got != want {
		t.Errorf("nextWord(%q, 0) = %d, want %d past the accented word", s, got, want)
	}
	hindi := "नमस्ते" // नमस्ते, one word
	if got := nextWord(hindi, 0); got != len(hindi) {
		t.Errorf("nextWord(%q, 0) = %d, want %d: the virama and vowel sign are part of the word", hindi, got, len(hindi))
	}
	var e Editor
	e.SetText("café")
	e.SelectWord(0)
	if e.Selected() != e.Text {
		t.Errorf("SelectWord(0) selected %q, want the whole word %q", e.Selected(), e.Text)
	}
}

// A line feed is a cluster of its own both ways, so arrows and Backspace
// agree on where clusters are.
func TestLineFeedEndsCluster(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		s    string
		i    int
		want int
	}{
		{"\n\u0301", 0, 1},   // a mark does not attach to a line feed
		{"a\u200d\nb", 0, 4}, // a joiner does not glue a line feed
	} {
		if got := NextGrapheme(c.s, c.i); got != c.want {
			t.Errorf("NextGrapheme(%q, %d) = %d, want %d where PrevGrapheme(%q, %d) = %d",
				c.s, c.i, got, c.want, c.s, len(c.s), PrevGrapheme(c.s, len(c.s)))
		}
	}
}

// AppendBounds finds what Bounds does, and into a slice it is given, so a
// caller stepping through a new string each time allocates nothing.
func TestAppendBoundsFindsBoundsWithoutAllocating(t *testing.T) {
	texts := []string{"áb", "한글", "한", "👩\u200d💻 ok", "🇰🇷x", "a\r\nb\nc", "\xffz", "❯ go test"}
	for _, s := range texts {
		if got, want := AppendBounds(nil, s), Bounds(s); !slices.Equal(got, want) {
			t.Errorf("AppendBounds(%q) = %v, want %v", s, got, want)
		}
	}
	dst := make([]int, 0, 64)
	row := []byte("❯ go build ./... && echo ok")
	if allocs := testing.AllocsPerRun(100, func() {
		row[len(row)-1] ^= 1 // a new string each run, as a terminal's rows are
		dst = AppendBounds(dst[:0], string(row))
	}); allocs > 1 { // the string conversion
		t.Errorf("AppendBounds allocates %v times, want at most the string's", allocs)
	}
}

// NextGrapheme and PrevGrapheme segment only the line around the offset.
// What they find must be what segmenting the whole text finds, at every
// offset, inside a rune too: at a line's ends, where a mark, a joiner or
// a flag's half meets a line break, and across CR LF.
func TestSteppingALineFindsWhatTheWholeTextHas(t *testing.T) {
	t.Parallel()
	texts := []string{
		"é\né",
		"👩\u200d💻\n👩\u200d💻\n",
		"\n\ńx\u200d\n🇰🇷\n🇰",
		"a\r\nb\r\n\r\nc",
		"\xff\n\xe2\x80\nz",
		"한글\n안녕 👍🏽\nok",
		"no breaks at all, é and 👨\u200d👩\u200d👧",
		"",
		"\n",
	}
	for _, s := range texts {
		bounds := AppendBounds(nil, s) // the whole text segmented at once
		for i := -1; i <= len(s)+1; i++ {
			at := min(max(i, 0), len(s)) // an offset outside the text is its nearer end
			next := len(s)
			if k := slices.IndexFunc(bounds, func(b int) bool { return b > at }); k >= 0 {
				next = bounds[k]
			}
			prev := 0
			for _, b := range bounds {
				if b < at {
					prev = b
				}
			}
			if got := NextGrapheme(s, i); got != next {
				t.Errorf("NextGrapheme(%q, %d) = %d, want %d", s, i, got, next)
			}
			if got := PrevGrapheme(s, i); got != prev {
				t.Errorf("PrevGrapheme(%q, %d) = %d, want %d", s, i, got, prev)
			}
		}
	}
}

// The line last stepped in is remembered with the text it is in. A text
// of the same length with its lines elsewhere, as an edit that moves a
// line break makes, must not be stepped by the other's line.
func TestSteppingAnotherTextDoesNotReuseItsLine(t *testing.T) {
	t.Parallel()
	a, b := "ab\ncd é", "a\nbcd é"
	for _, c := range []struct {
		s             string
		i, next, prev int
	}{
		{a, 1, 2, 0}, {b, 1, 2, 0}, {a, 2, 3, 1}, {b, 2, 3, 1}, {a, len(a) - 1, len(a), len(a) - 3}, {b, 3, 4, 2},
	} {
		if got := NextGrapheme(c.s, c.i); got != c.next {
			t.Errorf("NextGrapheme(%q, %d) = %d, want %d", c.s, c.i, got, c.next)
		}
		if got := PrevGrapheme(c.s, c.i); got != c.prev {
			t.Errorf("PrevGrapheme(%q, %d) = %d, want %d", c.s, c.i, got, c.prev)
		}
	}
}
