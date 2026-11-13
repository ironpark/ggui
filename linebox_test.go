package ggui

import (
	"math"
	"testing"
)

// A line of text that was given a line height is a box of it, as a CSS
// line is, whatever the font's ascent and descent: one line at 1 is as
// tall as the size, and the glyphs are centered in the box.
func TestALineHeightIsTheHeightOfEveryLine(t *testing.T) {
	t.Parallel()
	free := Loose(Sz(500, 500))
	plain := Text("a").Size(16)
	glyphs := plain.Layout(free, Env{})
	plainBase, _ := plain.Baseline()
	for _, c := range []struct {
		lh    float64
		lines string
		want  float64
	}{
		{1, "a", 16}, {1.5, "a", 24}, {2, "a", 32},
		{1, "a\nb", 32}, {1.5, "a\nb\nc", 72},
	} {
		w := Text(c.lines).Size(16).LineHeight(c.lh)
		got := w.Layout(free, Env{})
		if math.Abs(got.H-c.want) > 1e-9 {
			t.Errorf("%q at line height %v is %v tall, want %v", c.lines, c.lh, got.H, c.want)
		}
		// Half of what the line leaves beside the glyphs is above them:
		// less than nothing when the line is shorter than they are.
		base, _ := w.Baseline()
		if want := plainBase + (16*c.lh-glyphs.H)/2; math.Abs(base-want) > 1e-9 {
			t.Errorf("%q at line height %v has its baseline at %v, want %v", c.lines, c.lh, base, want)
		}
	}
}

// Text no line height was set for is as tall as its glyphs, as it always
// was, and a line height inherited from the Env or a Styled counts as set.
func TestOnlyALineHeightThatWasSetBoxesTheLines(t *testing.T) {
	t.Parallel()
	free := Loose(Sz(500, 500))
	w := Text("a").Size(20)
	glyphs := w.Layout(free, Env{})
	face := w.faceAt(1).Metrics()
	if want := face.HAscent + face.HDescent; math.Abs(glyphs.H-want) > 1e-9 {
		t.Fatalf("text with no line height is %v tall, want its glyphs' %v", glyphs.H, want)
	}
	if got := Text("a").Size(20).Layout(free, Env{}.WithText(TextStyle{LineHeight: 1.5})); math.Abs(got.H-30) > 1e-9 {
		t.Errorf("under an Env with line height 1.5 the text is %v tall, want 30", got.H)
	}
	if got := Styled(Text("a").Size(20)).LineHeight(1.5).Layout(free, Env{}); math.Abs(got.H-30) > 1e-9 {
		t.Errorf("under a Styled with line height 1.5 the text is %v tall, want 30", got.H)
	}
}

// A RichText, and so a Selectable Text, boxes its lines as a Text does.
func TestRichTextBoxesItsLinesAsTextDoes(t *testing.T) {
	t.Parallel()
	free := Loose(Sz(500, 500))
	for _, lh := range []float64{0, 1, 1.5, 2} {
		for _, s := range []string{"a", "a\nb\nc"} {
			plain, rich, sel := Text(s).Size(16), RichText(Span(s)).Size(16), Text(s).Size(16).Selectable()
			if lh != 0 {
				plain.LineHeight(lh)
				rich.LineHeight(lh)
				sel.LineHeight(lh)
			}
			want := plain.Layout(free, Env{})
			wantBase, _ := plain.Baseline()
			for name, w := range map[string]interface {
				Widget
				Baseline() (float64, bool)
			}{"RichText": rich, "Selectable Text": sel} {
				got := w.Layout(free, Env{})
				base, _ := w.Baseline()
				if math.Abs(got.H-want.H) > 1e-9 || math.Abs(base-wantBase) > 1e-9 {
					t.Errorf("%s %q at line height %v: %v tall with its baseline at %v, want Text's %v and %v", name, s, lh, got.H, base, want.H, wantBase)
				}
			}
		}
	}
}
