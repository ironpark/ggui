package ggui

import (
	"image/color"
	"math"
	"strings"
	"testing"
	"time"
)

// lineTexts returns what each line of a laid-out RichText draws.
func lineTexts(t *RichTextWidget) []string {
	var out []string
	for _, ln := range t.para.Lines {
		out = append(out, ln.S)
	}
	return out
}

func TestRichTextWrapsAcrossSpans(t *testing.T) {
	t.Parallel()
	w := RichText(Span("aaa "), Span("bbb").Weight(WeightBold), Span(" ccc"))
	wide := w.Layout(Loose(Sz(1000, 1000)), Env{})
	if got := lineTexts(w); len(got) != 1 || got[0] != "aaa bbb ccc" {
		t.Fatalf("wide: lines = %q, want one", got)
	}
	if w.Text() != "aaa bbb ccc" {
		t.Fatalf("Text = %q", w.Text())
	}
	narrow := w.Layout(Loose(Sz(wide.W*0.7, 1000)), Env{})
	if got := lineTexts(w); len(got) != 2 || got[0] != "aaa bbb" || got[1] != "ccc" {
		t.Fatalf("narrow: lines = %q, want [aaa bbb, ccc]", got)
	}
	if narrow.W > wide.W*0.7 || narrow.H <= wide.H {
		t.Fatalf("narrow = %+v, want no wider than %v and taller than %v", narrow, wide.W*0.7, wide.H)
	}
	// The first line holds the regular and the bold span, the bold one
	// in a style of its own.
	if n := len(w.para.Lines[0].Styles); n != 2 {
		t.Fatalf("first line has %d styles, want 2", n)
	}
}

func TestRichTextOneSpanMeasuresAsText(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"hello world", "a\nb\nc", ""} {
		got := RichText(Span(s)).Layout(Loose(Sz(1000, 1000)), Env{})
		want := Text(s).Layout(Loose(Sz(1000, 1000)), Env{})
		if math.Abs(got.W-want.W) > 0.01 || math.Abs(got.H-want.H) > 0.01 {
			t.Fatalf("%q: RichText %v, Text %v", s, got, want)
		}
	}
}

func TestRichTextBaselineIsTheTallestSpans(t *testing.T) {
	t.Parallel()
	small := RichText(Span("x"))
	small.Layout(Loose(Sz(1000, 1000)), Env{})
	mixed := RichText(Span("x"), Span("Y").Size(40))
	mixed.Layout(Loose(Sz(1000, 1000)), Env{})
	sb, ok1 := small.Baseline()
	mb, ok2 := mixed.Baseline()
	if !ok1 || !ok2 || mb <= sb {
		t.Fatalf("baselines %v (%v) and %v (%v), want the large span's to sit lower", sb, ok1, mb, ok2)
	}
	text := Text("x")
	text.Layout(Loose(Sz(1000, 1000)), Env{})
	if tb, _ := text.Baseline(); math.Abs(tb-sb) > 0.01 {
		t.Fatalf("one span's baseline %v, Text's %v", sb, tb)
	}
}

func TestRichTextMaxLinesAndEllipsis(t *testing.T) {
	t.Parallel()
	w := RichText(Span("aaa bbb "), Span("ccc ddd").Weight(WeightBold)).MaxLines(1)
	full := RichText(Span("aaa bbb ccc ddd")).Layout(Loose(Sz(1000, 1000)), Env{})
	w.Layout(Loose(Sz(full.W/2, 1000)), Env{})
	got := lineTexts(w)
	if len(got) != 1 || !strings.HasSuffix(got[0], "…") || strings.HasSuffix(got[0], " …") {
		t.Fatalf("lines = %q, want one cut with an ellipsis", got)
	}
	if w.para.Lines[0].Width > full.W/2+0.01 {
		t.Fatalf("cut line is %v wide, want at most %v", w.para.Lines[0].Width, full.W/2)
	}

	e := RichText(Span("aaa bbb ccc ddd")).NoWrap().Ellipsis()
	e.Layout(Loose(Sz(full.W/2, 1000)), Env{})
	if got := lineTexts(e); len(got) != 1 || !strings.HasSuffix(got[0], "…") {
		t.Fatalf("NoWrap Ellipsis lines = %q", got)
	}
	fits := RichText(Span("aaa")).Ellipsis()
	fits.Layout(Loose(Sz(1000, 1000)), Env{})
	if got := lineTexts(fits); got[0] != "aaa" {
		t.Fatalf("text that fits was cut: %q", got)
	}
}

func TestRichTextAlignAndSpansSetter(t *testing.T) {
	t.Parallel()
	w := RichText(Span("ab")).Align(1)
	w.Layout(Tight(Sz(200, 20)), Env{})
	var box Rect
	w.para.Rects(0, 2, 200, w.alignment(), func(r Rect) { box = r })
	if math.Abs(box.Origin.X+box.Size.W-200) > 0.01 {
		t.Fatalf("right-aligned text covers %v, want it to end at 200", box)
	}
	w.Spans(Span("one "), Span("two"))
	w.Layout(Loose(Sz(1000, 1000)), Env{})
	if w.Text() != "one two" {
		t.Fatalf("after Spans, Text = %q", w.Text())
	}
	spans := State([]TextSpan{Span("bound")})
	w.BindSpans(spans)
	w.Layout(Loose(Sz(1000, 1000)), Env{})
	if w.Text() != "bound" {
		t.Fatalf("after BindSpans, Text = %q", w.Text())
	}
}

func TestRichTextSpansInheritAndOverride(t *testing.T) {
	t.Parallel()
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	env := Env{}.WithText(TextStyle{Color: red, Size: 20})
	w := RichText(Span("a"), Span("b").Color(blue).Size(10))
	w.Layout(Loose(Sz(1000, 1000)), env)
	a, b := w.styles[0], w.styles[1]
	if a.Color != red || a.Size != 20 {
		t.Fatalf("plain span drew in %v at %v, want the inherited red at 20", a.Color, a.Size)
	}
	if b.Color != blue || b.Size != 10 {
		t.Fatalf("styled span drew in %v at %v, want blue at 10", b.Color, b.Size)
	}
	if Span("x").Underline().Name("n").Text() != "x" {
		t.Fatal("Span setters lost the text")
	}
}

func TestRichTextLinkTapsWithPointerCursorAndA11y(t *testing.T) {
	t.Parallel()
	taps := 0
	p := NewProbe(RichText(Span("Read the "), Span("guide").Underline().OnTap(func() { taps++ }), Span(" first.")), Sz(400, 40))
	defer p.Close()
	link, ok := p.FindRole(RoleLink, "guide")
	if !ok {
		t.Fatal("the tappable span registered no link")
	}
	p.Move(link.Center())
	if c := p.Cursor(); c != CursorShapePointer {
		t.Fatalf("cursor over the link is %v, want the pointer", c)
	}
	p.Click(link.Center())
	if taps != 1 {
		t.Fatalf("%d taps after a click, want 1", taps)
	}
	tree := p.Semantics()
	if _, ok := tree.Find(RoleText, "Read the guide first."); !ok {
		t.Fatal("the paragraph is missing from the semantics tree")
	}
	node, ok := tree.Find(RoleLink, "guide")
	if !ok || node.Actions&ActionPress == 0 {
		t.Fatalf("link node %+v, want a pressable link", node)
	}
	if node.Parent < 0 || tree.At(node.Parent).Role != RoleText {
		t.Fatal("the link is not a child of its paragraph")
	}
	p.Perform(node.ID, Action{Kind: ActionPress})
	if taps != 2 {
		t.Fatalf("%d taps after ActionPress, want 2", taps)
	}
}

// A paragraph laid out again with the same spans, as one in a long chat is
// whenever anything above it is, keeps what it made: its spans, its text
// and its run keys are not made again.
func TestRichTextRelayoutKeepsWhatItMade(t *testing.T) {
	spans := []TextSpan{Span("The agent ran "), Span("go test ./...").Weight(WeightBold), Span(" and every package passed.")}
	rt := RichText(spans...)
	env := rootEnv()
	cs := Loose(Sz(400, 1e6))
	rt.Layout(cs, env)
	if allocs := testing.AllocsPerRun(100, func() { rt.Layout(cs, env) }); allocs != 0 {
		t.Errorf("laying out unchanged spans allocates %v times, want 0", allocs)
	}
}

// A span struck out, or set apart by a background, is another span than
// the same text without: a paragraph changing one is laid out again, its
// background resolved for the paint.
func TestASpanStruckOutAndOneSetApart(t *testing.T) {
	t.Parallel()
	plain := Span("x")
	if sameSpan(plain, plain.Strike()) || sameSpan(plain, plain.Background(color.Black)) {
		t.Fatal("a span struck out or set apart is the same as the plain one")
	}
	if !sameSpan(plain.Strike(), Span("x").Strike()) {
		t.Fatal("two spans struck out alike differ")
	}
	r := RichText(Span("code").Background(color.RGBA{1, 2, 3, 255}).Strike())
	p := NewProbe(topLeft(r), Sz(200, 40))
	defer p.Close()
	p.Frame()
	if len(r.cur) != 1 || r.cur[0].background == nil || !r.cur[0].strike {
		t.Fatalf("laid out as %+v", r.cur)
	}
}

func TestSelectableRichTextLinkStillTaps(t *testing.T) {
	t.Parallel()
	taps := 0
	p := NewProbe(RichText(Span("Read the "), Span("guide").OnTap(func() { taps++ }), Span(" first.")).Selectable(), Sz(400, 40))
	defer p.Close()
	link, ok := p.FindRole(RoleLink, "guide")
	if !ok {
		t.Fatal("no link")
	}
	p.Move(link.Center())
	if c := p.Cursor(); c != CursorShapePointer {
		t.Errorf("cursor %v", c)
	}
	p.Click(link.Center())
	if taps != 1 {
		t.Fatalf("taps %d", taps)
	}
}

func TestRichTextRelayoutsWhenASpanResizes(t *testing.T) {
	t.Parallel()
	w := RichText(Span("abc").Size(10))
	a := w.Layout(Loose(Sz(1000, 1000)), Env{})
	w.Spans(Span("abc").Size(30))
	b := w.Layout(Loose(Sz(1000, 1000)), Env{})
	if b.W <= a.W || b.H <= a.H {
		t.Fatalf("size 10 %v, size 30 %v", a, b)
	}
}

func TestSelectableRTLWordsHitWhereTheyAreDrawn(t *testing.T) {
	t.Parallel()
	w := RichText(Span("שלום עולם")).Selectable()
	p := NewProbe(Directionality(DirRTL, w), Sz(300, 30))
	defer p.Close()
	p.Frame()
	node, ok := p.Semantics().Find(RoleText, "שלום עולם")
	if !ok {
		t.Fatal("no node")
	}
	r := node.Rect
	at := Pt(r.Origin.X+r.Size.W-3, r.Origin.Y+r.Size.H/2)
	p.Click(at)
	p.Click(at)
	if got := w.SelectedText(); got != "שלום" {
		t.Fatalf("double click at the right selected %q", got)
	}
	left := Pt(r.Origin.X+r.Size.W-w.para.Lines[0].Width+3, r.Origin.Y+r.Size.H/2)
	p.Frame()
	p.Click(left)
	p.Click(left)
	if got := w.SelectedText(); got != "עולם" {
		t.Fatalf("double click at the left selected %q", got)
	}
}

func TestASelectionRunsThroughTheTextsOfAnArea(t *testing.T) {
	t.Parallel()
	a, b, c := Text("first line").Selectable(), Text("second line").Selectable(), Text("third line").Selectable()
	area := SelectionArea(Column(a, b, c).Gap(10).Stretch())
	p := NewProbe(area, Sz(300, 200))
	defer p.Close()
	p.Frame()
	// From inside the first paragraph to inside the third.
	p.Press(textAt(&a.rich.w, a.rich.w.rect, 6))
	p.Move(textAt(&c.rich.w, c.rich.w.rect, 5))
	p.Release(textAt(&c.rich.w, c.rich.w.rect, 5))
	p.Frame()
	if got, want := area.SelectedText(), "line\nsecond line\nthird"; got != want {
		t.Fatalf("drag across three paragraphs selected %q, want %q", got, want)
	}
	p.Key("cmd+c")
	if got := p.Clipboard().Read(); got != "line\nsecond line\nthird" {
		t.Fatalf("copy put %q on the clipboard", got)
	}
	// Back up into the first: the selection runs the other way.
	p.Press(textAt(&c.rich.w, c.rich.w.rect, 5))
	p.Move(textAt(&a.rich.w, a.rich.w.rect, 6))
	p.Release(textAt(&a.rich.w, a.rich.w.rect, 6))
	p.Frame()
	if got, want := area.SelectedText(), "line\nsecond line\nthird"; got != want {
		t.Fatalf("drag up across three paragraphs selected %q, want %q", got, want)
	}
	p.Key("cmd+a", "cmd+c")
	if got := p.Clipboard().Read(); got != "first line\nsecond line\nthird line" {
		t.Fatalf("select all copied %q", got)
	}
	p.Key("escape")
	if got := area.SelectedText(); got != "" {
		t.Fatalf("escape left %q selected", got)
	}
}

// A click in one paragraph clears what a drag selected of the others, and
// has them painted again, not only the paragraph clicked.
func TestAClickClearsTheSelectionOfTheOtherTexts(t *testing.T) {
	t.Parallel()
	a, b := Text("first line").Selectable(), Text("second line").Selectable()
	area := SelectionArea(Column(a, b).Gap(10).Stretch())
	p := NewProbe(area, Sz(300, 100))
	defer p.Close()
	p.Frame()
	p.Press(textAt(&a.rich.w, a.rich.w.rect, 2))
	p.Move(textAt(&b.rich.w, b.rich.w.rect, 4))
	p.Release(textAt(&b.rich.w, b.rich.w.rect, 4))
	p.Frame()
	if area.SelectedText() == "" {
		t.Fatal("the drag selected nothing")
	}
	before := delivered.Load()
	p.Press(textAt(&a.rich.w, a.rich.w.rect, 3))
	p.Release(textAt(&a.rich.w, a.rich.w.rect, 3))
	p.Frame()
	if got := area.SelectedText(); got != "" {
		t.Fatalf("a click left %q selected", got)
	}
	if delivered.Load() == before {
		t.Fatal("the other paragraph was left to show its selection")
	}
}

func TestADragWithinOneTextOfAnAreaSelectsAsItsOwn(t *testing.T) {
	t.Parallel()
	a, b := Text("hello world").Selectable(), Text("other").Selectable()
	area := SelectionArea(Column(a, b).Gap(10))
	p := NewProbe(area, Sz(300, 100))
	defer p.Close()
	p.Frame()
	p.Press(textAt(&a.rich.w, a.rich.w.rect, 0))
	p.Move(textAt(&a.rich.w, a.rich.w.rect, 5))
	p.Release(textAt(&a.rich.w, a.rich.w.rect, 5))
	if got := area.SelectedText(); got != "hello" {
		t.Fatalf("drag within one paragraph selected %q, want hello", got)
	}
	p.Key("cmd+c")
	if got := p.Clipboard().Read(); got != "hello" {
		t.Fatalf("copy put %q on the clipboard", got)
	}
}

func TestFindSelectsTheMatchesOfAnAreaInTurn(t *testing.T) {
	t.Parallel()
	a, b := Text("Error: the path").Selectable(), Text("another error, and an ERROR").Selectable()
	area := SelectionArea(Column(a, b).Gap(10))
	p := NewProbe(area, Sz(400, 100))
	defer p.Close()
	p.Frame()
	for n, want := range []string{"Error", "error", "ERROR"} {
		at, ok := area.Find("error", 0, n)
		if !ok || area.SelectedText() != want {
			t.Fatalf("match %d: found %v, selected %q; want %q", n, ok, area.SelectedText(), want)
		}
		if at.Size.W <= 0 || (n == 0) != (at.Origin.Y < b.rich.w.rect.Origin.Y) {
			t.Fatalf("match %d painted at %v", n, at)
		}
	}
	if _, ok := area.Find("error", 0, 3); ok || area.SelectedText() != "ERROR" {
		t.Fatalf("a fourth match was found, or the third let go: %q", area.SelectedText())
	}
	// From below the first text, its match does not count.
	if _, ok := area.Find("error", b.rich.w.rect.Origin.Y, 0); !ok || area.SelectedText() != "error" {
		t.Fatalf("from the second text, the first match is %q", area.SelectedText())
	}
}

// textAt returns the window point over offset b of a laid-out paragraph
// painted at r, halfway down its line.
func textAt(w *RichTextWidget, r Rect, b int) Point {
	var at Point
	n := len(w.para.Text)
	lo, hi := b, min(b+1, n)
	if lo == hi {
		lo = max(b-1, 0)
	}
	w.para.Rects(lo, hi, r.Size.W, w.alignment(), func(box Rect) {
		x := box.Origin.X
		if b == n {
			x += box.Size.W
		}
		at = Pt(r.Origin.X+x+0.5, r.Origin.Y+box.Origin.Y+box.Size.H/2)
	})
	return at
}

func TestSelectableTextDoubleClickSelectsAWordAndCopies(t *testing.T) {
	t.Parallel()
	w := Text("hello world").Selectable()
	p := NewProbe(w, Sz(300, 30))
	defer p.Close()
	p.Frame()
	r := Rct(Pt(0, 0), Sz(300, 30))
	at := textAt(&w.rich.w, r, 7) // inside "world"
	p.Click(at)
	p.Click(at)
	if got := w.rich.w.SelectedText(); got != "world" {
		t.Fatalf("double click selected %q, want world", got)
	}
	p.Key("cmd+c")
	if got := p.Clipboard().Read(); got != "world" {
		t.Fatalf("copy put %q on the clipboard", got)
	}
	p.Key("cmd+a", "cmd+c")
	if got := p.Clipboard().Read(); got != "hello world" {
		t.Fatalf("select all copied %q", got)
	}
	p.Key("escape")
	if lo, hi := w.rich.w.Selection(); lo != hi {
		t.Fatalf("escape left %d..%d selected", lo, hi)
	}
}

func TestSelectableRichTextDragAndShiftClick(t *testing.T) {
	t.Parallel()
	w := RichText(Span("one "), Span("two").Weight(WeightBold), Span(" three")).Selectable()
	p := NewProbe(w, Sz(400, 30))
	defer p.Close()
	p.Frame()
	r := Rct(Pt(0, 0), Sz(400, 30))
	p.Press(textAt(w, r, 0))
	p.Move(textAt(w, r, 7))
	p.Release(textAt(w, r, 7))
	if got := w.SelectedText(); got != "one two" {
		t.Fatalf("drag selected %q, want across the spans to \"one two\"", got)
	}
	p.ClickWith(textAt(w, r, len("one two three")), Mods{Shift: true})
	if got := w.SelectedText(); got != "one two three" {
		t.Fatalf("shift-click extended to %q", got)
	}
	p.Key("cmd+c")
	if got := p.Clipboard().Read(); got != "one two three" {
		t.Fatalf("copy put %q on the clipboard", got)
	}
	if p.Cursor() != CursorShapeText {
		t.Fatalf("cursor over selectable text is %v, want the text cursor", p.Cursor())
	}
}

func TestSelectableTextDescribesItsSelection(t *testing.T) {
	t.Parallel()
	w := RichText(Span("abc def")).Selectable()
	p := NewProbe(w, Sz(300, 30))
	defer p.Close()
	tree := p.Semantics()
	node, ok := tree.Find(RoleText, "abc def")
	if !ok || node.Actions&ActionSetSelection == 0 {
		t.Fatalf("node %+v, want text that takes a selection", node)
	}
	p.Perform(node.ID, Action{Kind: ActionSetSelection, SelStart: 4, SelEnd: 7})
	if got := w.SelectedText(); got != "def" {
		t.Fatalf("ActionSetSelection selected %q", got)
	}
	node, _ = p.Semantics().Find(RoleText, "abc def")
	if node.SelStart != 4 || node.SelEnd != 7 {
		t.Fatalf("node reports %d..%d, want 4..7", node.SelStart, node.SelEnd)
	}
}

func TestSelectableTextKeepsTextLayout(t *testing.T) {
	t.Parallel()
	plain := Text("aaa bbb ccc").MaxLines(1)
	sel := Text("aaa bbb ccc").MaxLines(1).Selectable()
	a := plain.Layout(Loose(Sz(60, 100)), Env{})
	b := sel.Layout(Loose(Sz(60, 100)), Env{})
	if a != b {
		t.Fatalf("selectable text measured %v, plain %v", b, a)
	}
	pb, _ := plain.Baseline()
	sb, _ := sel.Baseline()
	if pb != sb {
		t.Fatalf("baselines %v and %v", pb, sb)
	}
}

// A frame painted in part around a hovered paragraph leaves the area the
// texts the last whole frame found: a click then clears the selection of
// every text, not only of those painted around the pointer.
func TestAClickAfterAFramePaintedInPartClearsTheWholeSelection(t *testing.T) {
	t.Parallel()
	var texts []Widget
	for range 8 {
		texts = append(texts, Text("an item of a list").Selectable())
	}
	area := SelectionArea(Scroll(Column(texts...).Gap(30).Stretch()))
	p := NewWindowProbe(func() Widget { return area }, Sz(300, 500))
	defer p.Close()
	step := func() WindowFrame { return p.Step(16 * time.Millisecond) }
	step()
	first, last, mid := &texts[0].(*TextWidget).rich.w, &texts[7].(*TextWidget).rich.w, &texts[4].(*TextWidget).rich.w
	p.Move(textAt(first, first.rect, 2))
	step()
	p.Press(textAt(first, first.rect, 2))
	step()
	p.Move(textAt(last, last.rect, 4))
	step()
	p.Release(textAt(last, last.rect, 4))
	step()
	p.Idle(16*time.Millisecond, time.Second)
	if area.SelectedText() == "" {
		t.Fatal("the drag selected nothing")
	}
	at := textAt(mid, mid.rect, 3)
	p.Move(at)
	if fr := step(); fr.Painted != FramePartial {
		t.Fatalf("hovering a paragraph painted the frame %v, want it in part", fr.Painted)
	}
	p.Click(at)
	step()
	if got := area.SelectedText(); got != "" {
		t.Fatalf("a click left %q selected", got)
	}
}
