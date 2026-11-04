package paragraph

import (
	"math"
	"os"
	"slices"
	"testing"

	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/third_party/typesetting/bidi"
	"golang.org/x/image/font/gofont/goregular"
)

func TestVisualOrderAppliesRuleL2(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		levels []bidi.Level
		want   []int
	}{
		{[]bidi.Level{0, 1, 1, 0}, []int{0, 2, 1, 3}},
		{[]bidi.Level{1, 1, 1}, []int{2, 1, 0}},
		{[]bidi.Level{1, 2, 1}, []int{2, 1, 0}},
		{[]bidi.Level{1, 2, 2, 1}, []int{3, 1, 2, 0}},
		{[]bidi.Level{0, 0}, []int{0, 1}},
	} {
		if got := visualOrder(tc.levels); !slices.Equal(got, tc.want) {
			t.Errorf("visualOrder(%v) = %v, want %v", tc.levels, got, tc.want)
		}
	}
}

func oneStyle(s string) []LineStyle {
	return []LineStyle{{Start: 0, End: len(s), Style: Style{Font: fontsys.Builtin(), Size: 14}}}
}

func TestLayoutLineOrdersRunsByDirection(t *testing.T) {
	t.Parallel()
	s := "abc שלום"
	ltr := LayoutLine(s, oneStyle(s), false, "")
	if len(ltr) != 2 || ltr[0].RTL || !ltr[1].RTL || s[ltr[0].Start:ltr[0].End] != "abc " {
		t.Fatalf("LTR pieces %+v, want \"abc \" then the Hebrew", ltr)
	}
	rtl := LayoutLine(s, oneStyle(s), true, "")
	if len(rtl) != 2 || !rtl[0].RTL || rtl[1].RTL || s[rtl[1].Start:rtl[1].End] != "abc" {
		t.Fatalf("RTL pieces %+v, want the Hebrew on the left and \"abc\" on the right", rtl)
	}
	if rtl[0].X != 0 || math.Abs(rtl[1].X-rtl[0].W) > 1e-9 {
		t.Fatalf("pieces do not abut: %+v", rtl)
	}
	// Inside a right-to-left piece the text runs leftwards.
	p := &rtl[0]
	if a, b := p.OffsetX(s, oneStyle(s), p.Start), p.OffsetX(s, oneStyle(s), p.End); a <= b {
		t.Fatalf("Hebrew starts at x %v and ends at %v, want it to run leftwards", a, b)
	}
	// The Hebrew's first letter is at its right edge, under the RTL caret.
	if got := CaretAt(s, rtl, oneStyle(s), Width(rtl)+5); got != 3 {
		t.Fatalf("past the right edge the caret is at %d, want the end of abc (3)", got)
	}
}

func TestLatinTextNeedsNoReordering(t *testing.T) {
	t.Parallel()
	if NeedsBidi("hello, world 123", false) {
		t.Fatal("plain Latin text in a left-to-right paragraph was reordered")
	}
	if !NeedsBidi("hello", true) || !NeedsBidi("x مرحبا", false) {
		t.Fatal("text that needs reordering was left alone")
	}
}

func TestShapingLanguage(t *testing.T) {
	t.Parallel()
	if got := shapingLanguage("", "مرحبا").String(); got != "und-Arab" {
		t.Fatalf("Arabic sample gave %q", got)
	}
	if got := shapingLanguage("he-IL", "x").String(); got != "he-IL" {
		t.Fatalf("explicit language gave %q", got)
	}
}

// TestRightToLeftFallbackRunsAreReversed checks a Hebrew phrase set in a
// font without Hebrew: each word comes from the fallback, the space from
// the font itself, and the words must still run right to left.
func TestRightToLeftFallbackRunsAreReversed(t *testing.T) {
	const path = "/System/Library/Fonts/SFHebrew.ttf"
	if _, err := os.Stat(path); err != nil {
		t.Skip("no Hebrew system font")
	}
	he, err := fontsys.LoadFile(path)
	if err != nil {
		t.Skip(err)
	}
	he.SetNoFallback()
	f, err := fontsys.Load(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	f.AddFallbacks(he)
	s := "שלום עולם"
	ps := LayoutLine(s, []LineStyle{{Start: 0, End: len(s), Style: Style{Font: f, Size: 14}}}, true, "he")
	if len(ps) != 1 || !ps[0].RTL {
		t.Fatalf("pieces %+v, want one right-to-left piece", ps)
	}
	segs := ps[0].segs
	if len(segs) != 3 || segs[0].face != 1 || segs[1].face != 0 || segs[2].face != 1 {
		t.Fatalf("segs %+v, want the fallback, the space in the font, the fallback", segs)
	}
	first := ps[0].OffsetX(s, []LineStyle{{Start: 0, End: len(s), Style: Style{Font: f, Size: 14}}}, 0)
	if math.Abs(first-ps[0].W) > 1e-6 {
		t.Fatalf("the first word starts at %v, want the right edge %v", first, ps[0].W)
	}
}
