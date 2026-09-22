package ggui

import (
	"testing"

	"github.com/ironpark/ggfx"
)

func TestImageLayoutKeepsAspect(t *testing.T) {
	img := ggfx.NewImage(200, 100)
	cases := []struct {
		name string
		w    *ImageWidget
		c    Constraints
		want Size
	}{
		{"natural", Image(img), Loose(Sz(Unbounded, Unbounded)), Sz(200, 100)},
		{"shrinks to width", Image(img), Loose(Sz(100, Unbounded)), Sz(100, 50)},
		{"shrinks to height", Image(img), Loose(Sz(Unbounded, 25)), Sz(50, 25)},
		{"width follows", Image(img).Height(50), Loose(Sz(Unbounded, Unbounded)), Sz(100, 50)},
		{"height follows", Image(img).Width(50), Loose(Sz(Unbounded, Unbounded)), Sz(50, 25)},
		{"fixed", Image(img).Size(30, 30), Loose(Sz(Unbounded, Unbounded)), Sz(30, 30)},
	}
	for _, tc := range cases {
		if got := tc.w.Layout(tc.c, Env{}); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestImagePlacement(t *testing.T) {
	img := ggfx.NewImage(200, 100)
	box := Rct(Pt(0, 0), Sz(100, 100))
	cases := []struct {
		fit  ImageFit
		want Rect
	}{
		{FitContain, Rct(Pt(0, 25), Sz(100, 50))},
		{FitCover, Rct(Pt(-50, 0), Sz(200, 100))},
		{FitFill, box},
		{FitNone, Rct(Pt(-50, 0), Sz(200, 100))},
	}
	for _, tc := range cases {
		if got := Image(img).Fit(tc.fit).placement(box); got != tc.want {
			t.Errorf("fit %d: %+v, want %+v", tc.fit, got, tc.want)
		}
	}
}
