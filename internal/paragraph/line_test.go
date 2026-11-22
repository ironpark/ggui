package paragraph

import (
	"math"
	"os"
	"slices"
	"testing"

	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/third_party/text"
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

// Chains at two sizes each find the system fonts in the order their own
// picks need them, so a segment measured with one is drawn with the face
// the other picks for it, not the face at its index there.
func TestARightToLeftSegmentDrawsInItsOwnFontAtAnotherSize(t *testing.T) {
	f := fontsys.Builtin()
	tag := newShapeLang("", "שלום").tag
	measured := newRTLChain(f, 14, 400, 0, nil, tag)
	own := len(measured.faces)
	hebrew, arabic := "שלום", "مرحبا"
	if measured.pick(hebrew) < own || measured.pick(arabic) < own {
		t.Skip("the built-in font draws Hebrew or Arabic itself, or no system font does")
	}
	segs, _ := rtlSegs(arabic, 0, measured)
	drawn := newRTLChain(f, 28, 400, 0, nil, tag) // has found no system font yet
	for _, sg := range segs {
		seg := arabic[sg.Start:sg.End]
		face, rtl := drawn.faceOf(seg)
		if !rtl {
			t.Fatalf("segment %q reported non-RTL face", seg)
		}
		font := measured.picker.Font(sg.face).Font
		j := slices.IndexFunc(drawn.faces, func(f text.Face) bool { return f == face })
		if j < 0 || drawn.picker.Font(j).Font != font {
			t.Fatalf("segment %q drawn in another font than it was measured in", seg)
		}
	}
}

func TestRTLChainFaceOfLazyFaceReportsRTL(t *testing.T) {
	const path = "/System/Library/Fonts/SFHebrew.ttf"
	if _, err := os.Stat(path); err != nil {
		t.Skip("no Hebrew system font")
	}
	he, err := fontsys.LoadFile(path)
	if err != nil {
		t.Skip(err)
	}
	he.SetNoFallback()
	chain := newRTLChain(he, 14, 400, 0, nil, newShapeLang("he", "שלום").tag)
	face, rtl := chain.faceOf("שלום")
	if !rtl {
		t.Fatalf("file-loaded RTL face %T reported rtl=false", face)
	}
	if _, ok := face.(*text.LazyFace); !ok {
		t.Logf("note: face type is %T", face)
	}
}

func TestShapeLangsCacheBounded(t *testing.T) {
	for i := range shapeLangsMin * 5 {
		newShapeLang("x-test-"+string(rune('a'+i%26))+string(rune('0'+(i/26)%10))+string(rune('0'+(i/260)%10)), "مرحبا")
	}
	shapeLangs.Lock()
	total := len(shapeLangs.cur) + len(shapeLangs.prev)
	shapeLangs.Unlock()
	if total > 2*shapeLangsMin {
		t.Fatalf("shapeLangs grew to %d entries, want <= %d", total, 2*shapeLangsMin)
	}
}

func TestParagraphBinarySearchRuns(t *testing.T) {
	t.Parallel()
	st := Style{Font: fontsys.Builtin(), Size: 14, Weight: 400, LineHeight: 1.2}
	p := &Paragraph{
		Text: "hello world test",
		Runs: []Run{
			{Start: 0, End: 0, Style: st, Span: 0},
			{Start: 0, End: 5, Style: st, Span: 1},
			{Start: 5, End: 5, Style: st, Span: 2},
			{Start: 5, End: 11, Style: st, Span: 3},
			{Start: 11, End: 16, Style: st, Span: 4},
			{Start: 16, End: 16, Style: st, Span: 5},
		},
		Base: st,
	}
	for pos, want := range map[int]int{
		-1: -1, 0: 1, 4: 1, 5: 3, 10: 3, 11: 4, 15: 4, 16: 5, 20: 5,
	} {
		if got := p.RunAt(pos); got != want {
			t.Errorf("RunAt(%d) = %d, want %d", pos, got, want)
		}
	}
	full := p.width(0, len(p.Text))
	if got := p.width(0, 5) + p.width(5, 11) + p.width(11, 16); math.Abs(got-full) > 1e-6 {
		t.Fatalf("partitioned width %v != full width %v", got, full)
	}
	pref := p.prefix(0, len(p.Text))
	if math.Abs(pref(len(p.Text))-full) > 1e-6 || math.Abs(pref(5)-p.width(0, 5)) > 1e-6 || pref(0) != 0 {
		t.Fatalf("prefix widths mismatch: pref(0)=%v pref(5)=%v pref(end)=%v full=%v", pref(0), pref(5), pref(len(p.Text)), full)
	}
	p.Layout(50, true, 0, false)
	if len(p.Lines) < 2 {
		t.Fatalf("expected wrapped lines, got %d", len(p.Lines))
	}
}

func TestRenderEmojiIncludesTracking(t *testing.T) {
	if fontsys.SystemEmoji() == nil {
		t.Skip("no system emoji font")
	}
	base := Style{Font: fontsys.Builtin(), Size: 20, Weight: 400, LineHeight: 1.2}
	tracked := base
	tracked.Tracking = 0.5
	w0 := Render("😀😀", base, nil, 0).Bounds().Dx()
	w1 := Render("😀😀", tracked, nil, 0).Bounds().Dx()
	if w1 <= w0 {
		t.Fatalf("tracked emoji width %d <= untracked %d", w1, w0)
	}
}
