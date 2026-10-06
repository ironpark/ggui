package ggui

import (
	"image/color"
	"testing"
)

// testColor is an EnvColor held under testColorKey.
type testColor struct{}

var testColorKey = NewEnvKey[color.Color]("test color")

func (testColor) Resolve(env Env) color.Color { c, _ := env.Get(testColorKey); return c }
func (testColor) RGBA() (r, g, b, a uint32)   { return 0, 0, 0, 0 }

// A Box and a Text paint an EnvColor as the Env they were laid out in has
// it, and setting one makes them lay out again to learn it.
func TestEnvColorsResolveAtLayout(t *testing.T) {
	red, blue := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}
	text := Text("x").Color(testColor{})
	box := Box(text).Fill(testColor{}).Border(1, testColor{}).Shadow(ShadowStyle{Color: testColor{}})
	for _, want := range []color.Color{red, blue} {
		box.Layout(Loose(Sz(100, 100)), Env{}.With(testColorKey, color.Color(want)))
		if box.fill.shown != want || box.borderColor.shown != want || box.shownShadows[0].Color != want {
			t.Errorf("box shows %v, %v, %v; want %v", box.fill.shown, box.borderColor.shown, box.shownShadows[0].Color, want)
		}
		if text.resolved.Color != want {
			t.Errorf("text shows %v, want %v", text.resolved.Color, want)
		}
	}
	before := box.props.Version()
	box.Fill(red)
	if box.props.Version() == before {
		t.Error("replacing an EnvColor fill did not ask for layout")
	}
	if box.fill.shown != red {
		t.Errorf("a plain fill shows %v at once, want red", box.fill.shown)
	}
	before = box.props.Version()
	box.Fill(blue)
	if box.props.Version() != before {
		t.Error("replacing one plain fill with another asked for layout")
	}
}
