package ggui

import (
	"image/color"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/goregular"
)

func TestTextWeightDrawsTheHeavierFace(t *testing.T) {
	t.Parallel()
	loose := Loose(Sz(1000, 1000))
	regular := Text("Weighty words").Layout(loose, Env{})
	bold := Text("Weighty words").Weight(WeightBold).Layout(loose, Env{})
	if bold.W <= regular.W {
		t.Fatalf("bold is %v wide, regular %v: want the built-in bold face, which is wider", bold.W, regular.W)
	}
	inherited := Styled(Text("Weighty words")).Weight(WeightBold)
	if got := inherited.Layout(loose, Env{}); got.W != bold.W {
		t.Fatalf("inherited bold is %v wide, want %v", got.W, bold.W)
	}
}

func TestFontPicksTheNearestWeight(t *testing.T) {
	t.Parallel()
	medium, bold := MustFont(gomedium.TTF), MustFont(gobold.TTF)
	f := MustFont(goregular.TTF).WithWeight(WeightMedium, medium).WithWeight(WeightBold, bold)
	for _, tc := range []struct {
		ask  FontWeight
		want *Font
	}{
		{0, f}, {WeightLight, f}, {WeightRegular, f}, {450, medium}, {WeightMedium, medium},
		{WeightSemibold, bold}, // equally near 500 and 700: the heavier, as CSS does
		{WeightBlack, bold},
	} {
		if got, _ := f.forWeight(tc.ask); got != tc.want {
			t.Errorf("weight %d drew the face of weight %d, want %d", tc.ask, got.weight, tc.want.weight)
		}
	}
	if f.weight != WeightRegular || medium.weight != WeightMedium {
		t.Fatalf("weights %d and %d, want the metadata's and the one WithWeight gave", f.weight, medium.weight)
	}
}

func TestTextRecolorIsPaintOnly(t *testing.T) {
	t.Parallel()
	w := Text("x")
	w.Layout(Loose(Sz(100, 100)), Env{})
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
