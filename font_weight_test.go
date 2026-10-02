package ggui

import (
	"image/color"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

func TestTextWeightDrawsTheHeavierFace(t *testing.T) {
	t.Parallel()
	family := MustFont(goregular.TTF).WithWeight(WeightBold, MustFont(gobold.TTF))
	loose := Loose(Sz(1000, 1000))
	regular := Text("Weighty words").Font(family).Layout(loose, Env{})
	bold := Text("Weighty words").Font(family).Weight(WeightBold).Layout(loose, Env{})
	if bold.W <= regular.W {
		t.Fatalf("bold is %v wide, regular %v: want the bold face, which is wider", bold.W, regular.W)
	}
	inherited := Styled(Text("Weighty words")).Font(family).Weight(WeightBold)
	if got := inherited.Layout(loose, Env{}); got.W != bold.W {
		t.Fatalf("inherited bold is %v wide, want %v", got.W, bold.W)
	}
	// The built-in font has no bold, so it draws regular.
	if plain, heavy := Text("Weighty words").Layout(loose, Env{}), Text("Weighty words").Weight(WeightBold).Layout(loose, Env{}); plain != heavy {
		t.Fatalf("the built-in font drew bold %v against regular %v, want the same face", heavy, plain)
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

func TestDefaultMonoFontFollowsItsSetting(t *testing.T) {
	t.Parallel()
	mono := MustFont(gomono.TTF)
	p := NewProbe(Text("x"), Sz(10, 10)).Setup(func() { SetDefaultMonoFont(mono) })
	defer p.Close()
	p.Frame()
	var monoWidth, textWidth float64
	p.Post(func() {
		monoWidth = lineWidth("iiii", DefaultMonoFont().face(14, 0))
		textWidth = lineWidth("iiii", fallbackFont().face(14, 0))
	})
	p.Frame()
	if want := lineWidth("iiii", mono.face(14, 0)); monoWidth != want || textWidth == want {
		t.Fatalf("DefaultMonoFont drew %v wide and the text font %v, want Go Mono's %v for the first only", monoWidth, textWidth, want)
	}
}

func TestAProbeDrawsInTheBuiltInFont(t *testing.T) {
	t.Parallel()
	p := NewProbe(Text("x"), Sz(10, 10))
	defer p.Close()
	var got *Font
	p.Post(func() { got = fallbackFont() })
	p.Frame()
	if got != builtinFont() {
		t.Fatal("a Probe drew in a font other than the built-in one, so tests would vary by machine")
	}
}
