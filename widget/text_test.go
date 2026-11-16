package widget

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/text"
	"github.com/ironpark/ggui/pkg/gem"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

func TestTextHeightFollowsLines(t *testing.T) {
	t.Parallel()
	one := Text("a").Layout(ggui.Loose(ggui.Sz(1000, 1000)), ggui.Env{})
	two := Text("a\nb").Layout(ggui.Loose(ggui.Sz(1000, 1000)), ggui.Env{})
	if one.H <= 0 || two.H <= one.H {
		t.Fatalf("heights %v and %v, want one line shorter than two", one.H, two.H)
	}
	if d := two.H - one.H - ggui.DefaultTextSize*1.2; d > 1e-9 || d < -1e-9 {
		t.Fatalf("second line added %v, want the line spacing %v", two.H-one.H, ggui.DefaultTextSize*1.2)
	}
}

func TestTextWrapsAtSpaces(t *testing.T) {
	t.Parallel()
	w := Text("aaa bbb ccc")
	wide := w.Layout(ggui.Loose(ggui.Sz(1000, 1000)), ggui.Env{})
	if len(w.lines) != 1 {
		t.Fatalf("wide: %d lines, want 1", len(w.lines))
	}
	narrow := w.Layout(ggui.Loose(ggui.Sz(wide.W*0.7, 1000)), ggui.Env{})
	if len(w.lines) != 2 || w.lines[0] != "aaa bbb" || w.lines[1] != "ccc" {
		t.Fatalf("narrow: lines = %q, want [aaa bbb, ccc]", w.lines)
	}
	if narrow.W > wide.W*0.7 || narrow.H <= wide.H {
		t.Fatalf("narrow = %+v, want no wider than %v and taller than %v", narrow, wide.W*0.7, wide.H)
	}
}

func TestTextBreaksLongWordsBetweenRunes(t *testing.T) {
	t.Parallel()
	w := Text(strings.Repeat("x", 40))
	full := w.Layout(ggui.Loose(ggui.Sz(1000, 1000)), ggui.Env{})
	w.Layout(ggui.Loose(ggui.Sz(full.W/4, 1000)), ggui.Env{})
	if len(w.lines) < 4 {
		t.Fatalf("%d lines, want at least 4", len(w.lines))
	}
	for _, l := range w.lines {
		if textlayout.Width(l, w.faceAt(1)) > full.W/4 {
			t.Fatalf("line %q is wider than the limit", l)
		}
	}
}

func TestTextNoWrapKeepsOneLine(t *testing.T) {
	t.Parallel()
	w := Text("aaa bbb ccc").NoWrap()
	w.Layout(ggui.Loose(ggui.Sz(5, 1000)), ggui.Env{})
	if len(w.lines) != 1 {
		t.Fatalf("%d lines, want 1", len(w.lines))
	}
}

func TestTextSizeScalesLayout(t *testing.T) {
	t.Parallel()
	small := Text("hello").Size(10).Layout(ggui.Loose(ggui.Sz(1000, 1000)), ggui.Env{})
	big := Text("hello").Size(20).Layout(ggui.Loose(ggui.Sz(1000, 1000)), ggui.Env{})
	if big.W <= small.W || big.H <= small.H {
		t.Fatalf("size 20 = %+v not larger than size 10 = %+v", big, small)
	}
}

func TestTextHeightFollowsInheritedLineHeight(t *testing.T) {
	t.Parallel()
	w := Text("first\nsecond\nthird")
	c := ggui.Loose(ggui.Sz(1000, 1000))
	env := ggui.Env{}.WithText(ggui.TextStyle{Size: 20, LineHeight: 1})
	before := w.Layout(c, env)
	after := w.Layout(c, env.WithText(ggui.TextStyle{LineHeight: 2}))
	// Each of the three lines is a box of its line height: 20 at 1, 40 at 2.
	// Font metrics can leave a rounding residual when the heights are subtracted.
	if after.W != before.W || math.Abs(before.H-60) > 1e-9 || math.Abs(after.H-120) > 1e-9 {
		t.Fatalf("line spacing: before=%v after=%v; want unchanged width, 60 high and then 120", before, after)
	}
}

func TestTextEllipsisCutsWhatDoesNotFit(t *testing.T) {
	t.Parallel()
	w := Text("aaa bbb ccc\nshort").NoWrap().Ellipsis()
	full := w.Layout(ggui.Loose(ggui.Sz(1000, 1000)), ggui.Env{})
	if fmt.Sprint(w.lines) != "[aaa bbb ccc short]" {
		t.Fatalf("lines that fit were changed: %q", w.lines)
	}
	got := w.Layout(ggui.Loose(ggui.Sz(full.W*0.6, 1000)), ggui.Env{})
	if len(w.lines) != 2 || !strings.HasSuffix(w.lines[0], "…") || strings.HasSuffix(w.lines[0], " …") || w.lines[1] != "short" {
		t.Fatalf("lines = %q, want the first cut with an ellipsis and the second whole", w.lines)
	}
	if got.W > full.W*0.6 {
		t.Fatalf("width %v, want no more than %v", got.W, full.W*0.6)
	}
	w.Layout(ggui.Loose(ggui.Sz(1, 1000)), ggui.Env{})
	if w.lines[0] != "…" {
		t.Fatalf("with no room the line is %q, want the ellipsis alone", w.lines[0])
	}
}

func TestTextMaxLinesEndsTheLastShownLine(t *testing.T) {
	t.Parallel()
	w := Text("aaa bbb ccc ddd").MaxLines(2)
	wide := w.Layout(ggui.Loose(ggui.Sz(1000, 1000)), ggui.Env{})
	if len(w.lines) != 1 || w.lines[0] != "aaa bbb ccc ddd" {
		t.Fatalf("text that fits was cut: %q", w.lines)
	}
	narrow := w.Layout(ggui.Loose(ggui.Sz(wide.W*0.3, 1000)), ggui.Env{})
	if len(w.lines) != 2 || !strings.HasSuffix(w.lines[1], "…") {
		t.Fatalf("lines = %q, want two, the second ending in an ellipsis", w.lines)
	}
	for _, l := range w.lines {
		if textlayout.Width(l, w.faceAt(1)) > wide.W*0.3 {
			t.Fatalf("line %q is wider than the limit", l)
		}
	}
	two := Text("a\nb").Layout(ggui.Loose(ggui.Sz(1000, 1000)), ggui.Env{})
	if narrow.H != two.H {
		t.Fatalf("height %v, want two lines' %v", narrow.H, two.H)
	}
	p := ggui.NewProbe(w, ggui.Sz(wide.W*0.3, 100))
	defer p.Close()
	if _, ok := p.Semantics().Find(ggui.RoleText, "aaa bbb ccc ddd"); !ok {
		t.Fatal("the semantics tree lost the text that was cut")
	}
}

func TestDirectionalityMirrorsRowsAndColumns(t *testing.T) {
	t.Parallel()
	rtl := ggui.Env{}.With(ggui.DirectionKey, ggui.DirRTL)
	if rtl.Direction() != ggui.DirRTL || (ggui.Env{}).Direction() != ggui.DirLTR || ggui.DirRTL.String() != "rtl" {
		t.Fatal("Env.Direction does not follow DirectionKey")
	}
	row := Row(Box().Size(10, 10), Box().Size(20, 10))
	size := row.Layout(ggui.Loose(ggui.Sz(100, 100)), rtl)
	if row.rects[0].Origin.X != size.W-10 || row.rects[1].Origin.X != 0 {
		t.Fatalf("RTL row offsets %v in %v, want the first child rightmost", row.rects, size)
	}
	col := Column(Box().Size(10, 10), Box().Size(30, 10))
	col.Layout(ggui.Loose(ggui.Sz(100, 100)), rtl)
	if col.rects[0].Origin.X != 20 || col.rects[1].Origin.X != 0 {
		t.Fatalf("RTL column offsets %v, want AlignStart on the right", col.rects)
	}
	ltr := Row(Box().Size(10, 10), Box().Size(20, 10))
	ltr.Layout(ggui.Loose(ggui.Sz(100, 100)), ggui.Env{})
	if ltr.rects[0].Origin.X != 0 {
		t.Fatalf("LTR row moved: %v", ltr.rects)
	}
}

func TestTextUnderRTLStartsAtTheRight(t *testing.T) {
	t.Parallel()
	rtl := ggui.Env{}.With(ggui.DirectionKey, ggui.DirRTL)
	w := Text("שלום world")
	w.Layout(ggui.Loose(ggui.Sz(300, 100)), rtl)
	if len(w.bidi.lines) != 1 || w.bidi.lines[0] == nil {
		t.Fatal("RTL text was not reordered")
	}
	if a := gem.Pick(w.alignSet, w.align, startAlign(w.bidi.dir)); a != 1 {
		t.Fatalf("default alignment under RTL is %v, want 1", a)
	}
	w.Align(0)
	if a := gem.Pick(w.alignSet, w.align, startAlign(w.bidi.dir)); a != 0 {
		t.Fatalf("explicit Align(0) gave %v", a)
	}
	// The Latin word comes first in the reordered line: it is on the left.
	ps := w.bidi.lines[0]
	if ps[0].RTL || w.lines[0][ps[0].Start:ps[0].End] != "world" {
		t.Fatalf("pieces %+v, want world on the left", ps)
	}

	r := RichText(Span("ab"))
	r.Layout(ggui.Tight(ggui.Sz(200, 20)), rtl)
	var box ggui.Rect
	r.para.Rects(0, 2, 200, r.alignment(), func(b ggui.Rect) { box = b })
	if math.Abs(box.Origin.X+box.Size.W-200) > 0.01 {
		t.Fatalf("RTL rich text covers %v, want it at the right edge", box)
	}
	// The reordered lines, a selection, an underline and a rotated line
	// all paint into an image.
	img := ggfx.NewImage(300, 120)
	defer img.Deallocate()
	sel := RichText(Span("שלום "), Span("bold").Weight(ggui.WeightBold).Underline().OnTap(func() {})).Selectable()
	rot := Text("مرحبا 😀 abc")
	p := ggui.NewProbe(ggui.Directionality(ggui.DirRTL, Column(Text("مرحبا بالعالم 123 😀"), sel, ggui.FromFuncs(
		func(c ggui.Constraints, env ggui.Env) ggui.Size { return rot.Layout(c, env) },
		func(dst *ggui.Canvas, r ggui.Rect) { rot.PaintRotated(dst, r, 0.3) },
	))), ggui.Sz(300, 120))
	defer p.Close()
	p.RenderTo(img)
	p.Frame()
	node, ok := p.Semantics().Find(ggui.RoleText, "שלום bold")
	if !ok || node.Rect.Origin.X+node.Rect.Size.W < 299 {
		t.Fatalf("selectable RTL text at %v, want it at the right of the column", node.Rect)
	}
	p.Click(ggui.Pt(node.Rect.Origin.X+node.Rect.Size.W-2, node.Rect.Origin.Y+node.Rect.Size.H/2))
	p.Key("cmd+a")
	p.Frame()
	if sel.SelectedText() != "שלום bold" {
		t.Fatalf("select all in RTL selected %q", sel.SelectedText())
	}
}

func TestTextRasterizesAtScaledSize(t *testing.T) {
	t.Parallel()
	w := Text("x").Font(ggui.MustFont(goregular.TTF).NoFallback()).Size(14)
	face, ok := w.faceAt(2).(*text.GoTextFace)
	if !ok || face.Size != 28 {
		t.Fatalf("face at scale 2 = %+v, want a GoTextFace of size 28", face)
	}
	if w.faceAt(1).(*text.GoTextFace).Size != 14 {
		t.Fatal("layout face must stay at the logical size")
	}
}

// TestTextSkipsLinesOutsideTheClip paints a long Text into a canvas that
// shows one line of it: the lines outside are not shaped and drawn, so it
// allocates a fraction of what painting all of them does.
func TestTextSkipsLinesOutsideTheClip(t *testing.T) {
	img := ggfx.NewImage(400, 400)
	defer img.Deallocate()
	var s strings.Builder
	for i := range 200 {
		fmt.Fprintf(&s, "line %d of a long text\n", i)
	}
	txt := Text(s.String()).NoWrap()
	size := txt.Layout(ggui.Loose(ggui.Sz(400, 1e6)), ggui.Untrack(ggui.UseEnv))
	full := &ggui.Canvas{Image: img}
	shown := full.Clip(ggui.Rct(ggui.Pt(0, 0), ggui.Sz(400, 20)))
	all := testing.AllocsPerRun(5, func() { full.Paint(txt, ggui.Rct(ggui.Point{}, size)) })
	one := testing.AllocsPerRun(5, func() { shown.Paint(txt, ggui.Rct(ggui.Point{}, size)) })
	// One line allocates about a twelfth of what all do, but about a tenth
	// under the race detector; painting all of them would allocate as much.
	if one*5 > all {
		t.Errorf("painting one line of 200 allocates %v, all of them %v", one, all)
	}
}

// A Text laid out paints nothing, and fails nothing, on no Canvas or on
// one with no image.
func TestTextToleratesNilTargets(t *testing.T) {
	t.Parallel()
	w := Text("x")
	w.Layout(ggui.Loose(ggui.Sz(100, 100)), ggui.Env{})
	w.Paint(nil, ggui.Rct(ggui.Pt(0, 0), ggui.Sz(10, 10)))
	w.Paint(&ggui.Canvas{}, ggui.Rct(ggui.Pt(0, 0), ggui.Sz(10, 10)))
}

func TestTextWeightDrawsTheHeavierFace(t *testing.T) {
	t.Parallel()
	family := ggui.MustFont(goregular.TTF).WithWeight(ggui.WeightBold, ggui.MustFont(gobold.TTF))
	loose := ggui.Loose(ggui.Sz(1000, 1000))
	regular := Text("Weighty words").Font(family).Layout(loose, ggui.Env{})
	bold := Text("Weighty words").Font(family).Weight(ggui.WeightBold).Layout(loose, ggui.Env{})
	if bold.W <= regular.W {
		t.Fatalf("bold is %v wide, regular %v: want the bold face, which is wider", bold.W, regular.W)
	}
	inherited := Styled(Text("Weighty words")).Font(family).Weight(ggui.WeightBold)
	if got := inherited.Layout(loose, ggui.Env{}); got.W != bold.W {
		t.Fatalf("inherited bold is %v wide, want %v", got.W, bold.W)
	}
	// The built-in font has no bold, so it draws regular.
	if plain, heavy := Text("Weighty words").Layout(loose, ggui.Env{}), Text("Weighty words").Weight(ggui.WeightBold).Layout(loose, ggui.Env{}); plain != heavy {
		t.Fatalf("the built-in font drew bold %v against regular %v, want the same face", heavy, plain)
	}
}

func TestTextRecolorIsPaintOnly(t *testing.T) {
	t.Parallel()
	w := Text("x")
	w.Layout(ggui.Loose(ggui.Sz(100, 100)), ggui.Env{})
	v := w.props.Version()
	w.Color(color.RGBA{R: 255, A: 255})
	if w.props.Version() == v {
		t.Fatal("setting a color over the inherited one did not lay the text out again")
	}
	v = w.props.Version()
	w.Color(color.RGBA{G: 255, A: 255})
	if w.props.Version() != v {
		t.Fatal("swapping one color for another laid the text out again")
	}
	w.Color(nil)
	if w.props.Version() == v {
		t.Fatal("going back to the inherited color did not lay the text out again")
	}
}

// A Text a cache holds measures again when the emoji font changes. The
// font is set in a probe's world, which the probe's frames make the one
// its goroutine's widgets read, so that no other test's text sees it.
func TestEmojiSwitchInvalidatesCachedMeasurement(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../testdata/emoji-subset.ttf")
	if err != nil {
		t.Fatal(err)
	}
	w := Text("👩🏽‍💻")
	c := ggui.Cached(w)
	p := ggui.NewProbe(c, ggui.Sz(1000, 100)).Setup(func() { ggui.SetEmojiFont(ggui.MustFont(data)) })
	defer p.Close()
	p.Frame()
	constraints := ggui.Loose(ggui.Sz(1000, 100))
	env := ggui.Env{}
	c.Layout(constraints, env)
	old := w.wrapped.generation
	ggui.SetEmojiFont(nil)
	c.Layout(constraints, env)
	if w.wrapped.generation == old {
		t.Fatal("cached text did not remeasure after font switch")
	}
}

// stamp paints its label twice in different colors, as a shimmer does.
type stamp struct{ label *TextWidget }

func (s *stamp) Layout(c ggui.Constraints, env ggui.Env) ggui.Size { return s.label.Layout(c, env) }

func (s *stamp) Paint(dst *ggui.Canvas, r ggui.Rect) {
	s.label.Color(color.White)
	dst.Paint(s.label, r)
	s.label.Color(color.Black)
	dst.Paint(s.label, r)
}

func TestPaintTimeWritesScheduleNoLayout(t *testing.T) {
	t.Parallel()
	p := ggui.NewProbe(&stamp{label: Text("hi")}, ggui.Sz(100, 20))
	defer p.Close()
	p.Frame()
	p.Frame()
	// The layout generation is this goroutine's, so nothing another test
	// does moves it.
	before := reactive.LayoutGen()
	p.Frame()
	if reactive.LayoutGen() != before {
		t.Fatal("configuring a child while painting scheduled a layout")
	}
}

func TestSemanticsTextRoles(t *testing.T) {
	t.Parallel()
	p := ggui.NewProbe(Column(Text("Heading").Role(ggui.RoleHeading), Text("body"), Text("")), ggui.Sz(200, 200))
	defer p.Close()
	tree := p.Semantics()
	want := `heading "Heading"
text "body"
`
	if got := tree.String(); got != want {
		t.Fatalf("tree =\n%s\nwant\n%s", got, want)
	}
}

// A Text scrolled out of view, whose drawing the clip culls, is still in
// the semantics tree, flagged as off screen.
func TestTextOutOfViewKeepsItsSemantics(t *testing.T) {
	t.Parallel()
	p := ggui.NewProbe(Scroll(Column(Box().Size(100, 500), Text("offscreen text"))), ggui.Sz(100, 100))
	defer p.Close()
	n, ok := p.Semantics().Find(ggui.RoleText, "offscreen text")
	if !ok || !n.Offscreen {
		t.Fatalf("found %v, offscreen %v; want the text, flagged", ok, n.Offscreen)
	}
}
