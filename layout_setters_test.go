package ggui

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func TestSpaceUsesTheEnvironmentUnit(t *testing.T) {
	t.Parallel()
	env := Env{}.With(SpacingKey, 12.0)
	free := Loose(Sz(500, 500))
	box := func() Widget { return Box().Size(10, 10) }
	cases := []struct {
		name string
		w    Widget
		c    Constraints
		want Size
	}{
		{"row", Row(box(), box()).Space(2), free, Sz(10+24+10, 10)},
		{"wrap", Wrap(box(), box()).Space(2), free, Sz(10+24+10, 10)},
		{"wrap across lines", Wrap(box(), box()).Space(2), Loose(Sz(30, 500)), Sz(10, 10+24+10)},
		{"grid", Grid(1, box(), box()).Space(1), free, Sz(500, 10+12+10)},
	}
	for _, tc := range cases {
		if got := tc.w.Layout(tc.c, env); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRunGapAndRowGapSeparateLinesOnly(t *testing.T) {
	t.Parallel()
	var last Rect
	w := Wrap(Box().Size(40, 10), Box().Size(40, 10), probe(40, 10, &last)).Gap(5).RunGap(20)
	size := w.Layout(Loose(Sz(90, 200)), Env{})
	if size != Sz(85, 40) {
		t.Fatalf("wrap = %v, want two per line 5 apart and lines 20 apart: {85 40}", size)
	}
	w.Paint(nil, Rct(Pt(100, 100), size))
	if last != Rct(Pt(100, 130), Sz(40, 10)) {
		t.Fatalf("third child painted at %+v, want the second line's start at (100,130)", last)
	}
	g := Grid(2, Box().Size(10, 10), Box().Size(10, 10), Box().Size(10, 10)).Gap(4).RowGap(16)
	if got := g.Layout(Loose(Sz(24, 200)), Env{}); got != Sz(24, 36) {
		t.Fatalf("grid = %v, want columns 4 apart and rows 16 apart: {24 36}", got)
	}
}

func TestLineHeightSpacesTextLines(t *testing.T) {
	t.Parallel()
	free := Loose(Sz(500, 500))
	one := Text("a\nb").LineHeight(1).Layout(free, Env{})
	three := Text("a\nb").LineHeight(3).Layout(free, Env{})
	if !closeTo(three.H-one.H, 2*DefaultTextSize) {
		t.Fatalf("two lines at line height 3 are %v taller than at 1, want %v", three.H-one.H, 2*DefaultTextSize)
	}
	inherited := Styled(Text("a\nb")).LineHeight(3).Layout(free, Env{})
	if inherited != three {
		t.Fatalf("an inherited line height of 3 gives %v, want %v", inherited, three)
	}
}

func TestStyledSetsFontAndMergesStyle(t *testing.T) {
	t.Parallel()
	font := MustFont(goregular.TTF)
	w := Text("x")
	Styled(w).Font(font).Style(TextStyle{Size: 20}).Style(TextStyle{LineHeight: 2}).Layout(Loose(Sz(100, 100)), Env{})
	if w.resolved.Font != font || w.resolved.Size != 20 || w.resolved.LineHeight != 2 {
		t.Fatalf("resolved %+v, want the Styled font, size 20 and line height 2 merged", w.resolved)
	}
}

func TestWrappersPassTheBaselineThrough(t *testing.T) {
	t.Parallel()
	free := Loose(Sz(200, 100))
	text := func() *TextWidget { return Text("x") }
	ascent := func(w Widget) float64 {
		w.Layout(free, Env{})
		b, ok := w.(Baseliner).Baseline()
		if !ok {
			t.Fatalf("%T has no baseline", w)
		}
		return b
	}
	want := ascent(text())
	for name, w := range map[string]Widget{
		"Styled":  Styled(text()),
		"Provide": Provide(SpacingKey, 3.0, text()),
		"WithEnv": WithEnv(func(e Env) Env { return e }, text()),
		"Flex":    Row(Flex(text(), 1)).Align(AlignStart),
	} {
		if got := ascent(w); !closeTo(got, want) {
			t.Errorf("%s: baseline %v, want the text's %v", name, got, want)
		}
	}
	// An Align moves the baseline with the child.
	al := Align(text()).Bottom()
	al.Layout(Tight(Sz(200, 100)), Env{})
	h := text().Layout(free, Env{}).H
	if got, _ := al.Baseline(); !closeTo(got, 100-h+want) {
		t.Fatalf("bottom-aligned baseline %v, want %v", got, 100-h+want)
	}
}

func TestScrollKeyKeepsItsOffsetWhenMoved(t *testing.T) {
	t.Parallel()
	moved := State(false)
	var s *ScrollWidget
	p := ProbeBuilder(func() Widget {
		return Reactive(func() Widget {
			s = Scroll(Box().Size(100, 1000)).Key("list")
			if moved.Get() {
				return Column(Box().Size(10, 20), Box(s).Size(100, 100))
			}
			return Column(Box(s).Size(100, 100))
		})
	}, Sz(100, 200))
	defer p.Close()
	p.Scroll(Pt(50, 50), Pt(0, -3))
	before := s.position()
	if before <= 0 {
		t.Fatalf("the wheel did not scroll: offset %v", before)
	}
	moved.Set(true)
	p.Frame()
	if got := s.position(); got != before {
		t.Fatalf("the rebuilt scroll at another place is at %v, want the %v it was scrolled to", got, before)
	}
	if s.CaptureTouchDrag() {
		t.Fatal("a scroll nobody is dragging captures touch drags")
	}
}
