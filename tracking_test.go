package ggui

import (
	"math"
	"testing"

	"github.com/ironpark/ggui/internal/third_party/text"
)

// trackingNear reports whether a and b are within a few 26.6 fixed-point steps, the
// grain advances are kept in: one step for each of n clusters.
func trackingNear(a, b float64, n int) bool {
	return math.Abs(a-b) <= float64(n+1)/64
}

func TestTrackingAddsItsShareOfTheSizeAfterEveryLetter(t *testing.T) {
	t.Parallel()
	const s = "abcd"
	plain := Text(s).Size(20).NoWrap().Layout(Loose(Sz(1000, 1000)), Env{})
	wide := Text(s).Size(20).Tracking(0.1).NoWrap().Layout(Loose(Sz(1000, 1000)), Env{})
	if want := plain.W + 4*2; !trackingNear(wide.W, want, 4) {
		t.Fatalf("tracked width = %v, want the plain %v and 2px after each of 4 letters, %v", wide.W, plain.W, want)
	}
	tight := Text(s).Size(20).Tracking(-0.025).NoWrap().Layout(Loose(Sz(1000, 1000)), Env{})
	if want := plain.W - 4*0.5; !trackingNear(tight.W, want, 4) {
		t.Fatalf("tight width = %v, want %v", tight.W, want)
	}
	if wide.H != plain.H {
		t.Fatalf("tracking changed the height: %v, plain %v", wide.H, plain.H)
	}
}

func TestTrackingIsInheritedAndScalesWithTheSize(t *testing.T) {
	t.Parallel()
	const s = "abcd"
	measure := func(size, tracking float64) float64 {
		w := Text(s).Size(size).NoWrap()
		return Styled(w).Tracking(tracking).Layout(Loose(Sz(1000, 1000)), Env{}).W
	}
	for _, size := range []float64{10, 30} {
		got := measure(size, 0.1) - Text(s).Size(size).NoWrap().Layout(Loose(Sz(1000, 1000)), Env{}).W
		if want := 4 * 0.1 * size; !trackingNear(got, want, 4) {
			t.Fatalf("at %vpx inherited tracking added %v, want %v", size, got, want)
		}
	}
}

// What is drawn is what was measured: each glyph starts where the advance
// to its letter says, so a caret or a selection sits between the letters
// drawn.
func TestTrackedGlyphsAreDrawnWhereTheyAreMeasured(t *testing.T) {
	t.Parallel()
	const s = "tracking"
	w := Text(s).Size(16).Tracking(0.1).NoWrap()
	w.Layout(Loose(Sz(1000, 1000)), Env{})
	face := w.faceAt(1)
	plain := Text(s).Size(16).NoWrap()
	plain.Layout(Loose(Sz(1000, 1000)), Env{})
	glyphs := text.AppendLazyGlyphs(nil, s, face, nil)
	if len(glyphs) != len(s) {
		t.Fatalf("%d glyphs for %q, want one a letter", len(glyphs), s)
	}
	for i, g := range glyphs {
		at := text.AdvanceAt(s, g.StartIndexInBytes, face)
		if math.Abs(g.OriginX-at) > 1e-9 {
			t.Fatalf("glyph %d drawn at %v, measured at %v", i, g.OriginX, at)
		}
		base := text.AdvanceAt(s, g.StartIndexInBytes, plain.faceAt(1))
		if want := base + float64(i)*1.6; !trackingNear(at, want, i) {
			t.Fatalf("letter %d at %v, want the plain %v and %d spaces of 1.6, %v", i, at, base, i, want)
		}
	}
}

func TestTrackedTextWrapsAtItsTrackedWidth(t *testing.T) {
	t.Parallel()
	const s = "aaaa bbbb cccc"
	plain := Text(s)
	full := plain.Layout(Loose(Sz(1000, 1000)), Env{})
	// Room for the plain line, and not for the tracked one.
	tracked := Text(s).Tracking(0.2)
	got := tracked.Layout(Loose(Sz(full.W+1, 1000)), Env{})
	if len(tracked.lines) < 2 {
		t.Fatalf("tracked text kept %d line in the plain text's width", len(tracked.lines))
	}
	if got.W > full.W+1 {
		t.Fatalf("tracked text is %v wide in %v", got.W, full.W+1)
	}
	for i, l := range tracked.lines {
		if lw := tracked.widths[i]; lw > full.W+1 {
			t.Fatalf("line %q is %v wide in %v", l, lw, full.W+1)
		}
	}
}

func TestTrackedTextIsEllipsizedToItsTrackedWidth(t *testing.T) {
	t.Parallel()
	w := Text("abcdefghijklmnopqrstuvwxyz").Tracking(0.3).NoWrap().Ellipsis()
	got := w.Layout(Loose(Sz(80, 1000)), Env{})
	if got.W > 80 || len(w.lines) != 1 || w.lines[0] == "abcdefghijklmnopqrstuvwxyz" {
		t.Fatalf("got %v wide, line %q, want it cut to 80", got.W, w.lines)
	}
}

func TestATextInputMeasuresItsCaretWithItsTracking(t *testing.T) {
	t.Parallel()
	plain := TextInput(State("")).Style(TextStyle{Size: 20})
	tracked := TextInput(State("")).Style(TextStyle{Size: 20, Tracking: 0.1})
	if got, want := tracked.advance("abc"), plain.advance("abc")+3*2; !trackingNear(got, want, 3) {
		t.Fatalf("caret after abc at %v, want %v", got, want)
	}
}

func TestRenderTextAndCanvasMeasureWithTracking(t *testing.T) {
	t.Parallel()
	const s = "abcd"
	st := TextStyle{Size: 20}
	plain := RenderText(s, st, 0).Bounds().Dx()
	st.Tracking = 0.5
	wide := RenderText(s, st, 0).Bounds().Dx()
	if d := wide - plain; d < 39 || d > 41 {
		t.Fatalf("RenderText grew by %d with 10px after each of 4 letters, want 40", d)
	}
	c := &Canvas{}
	if d := c.TextStyleWidth(s, st) - c.TextStyleWidth(s, TextStyle{Size: 20}); !trackingNear(d, 40, 4) {
		t.Fatalf("TextStyleWidth grew by %v, want 40", d)
	}
}

func TestTextStylesMergeAndCompareTracking(t *testing.T) {
	t.Parallel()
	a := TextStyle{Size: 12, Tracking: 0.1}
	if m := a.Merge(TextStyle{Size: 14}); m.Tracking != 0.1 || m.Size != 14 {
		t.Fatalf("merge lost the tracking: %+v", m)
	}
	if m := a.Merge(TextStyle{Tracking: -0.025}); m.Tracking != -0.025 {
		t.Fatalf("merge kept the tracking below: %+v", m)
	}
	if a.Equal(TextStyle{Size: 12}) {
		t.Fatal("styles of different tracking compare equal")
	}
}
