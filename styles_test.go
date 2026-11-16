package ggui

import (
	"image/color"
	"testing"
	"time"
)

// The tests here are of how the built-in widgets take their styles and
// colors from the Env.

var (
	red  = color.RGBA{0xff, 0, 0, 0xff}
	blue = color.RGBA{0, 0, 0xff, 0xff}
)

func TestTextStyleKeyFallbackAndExplicitOverrides(t *testing.T) {
	t.Parallel()
	key := NewEnvKey[TextStyle]("label")
	text := Text("hello").StyleKey(key, TextStyle{Size: 21, Color: color.White}).Color(color.Black)
	text.Layout(Loose(Sz(300, 100)), Env{})
	if text.resolved.Size != 21 || text.resolved.Color != color.Black {
		t.Fatal("fallback or explicit override lost")
	}
	text.Layout(Loose(Sz(300, 100)), Env{}.With(key, TextStyle{Size: 30, Color: color.White}))
	if text.resolved.Size != 30 || text.resolved.Color != color.Black {
		t.Fatal("inherited style did not override fallback below explicit setters")
	}
}

func TestTextScaleAndReducedMotionThroughTheEnv(t *testing.T) {
	t.Parallel()
	plain := Text("hello").Layout(Loose(Sz(500, 100)), Env{})
	scaled := Provide(TextScaleKey, 2.0, Text("hello")).Layout(Loose(Sz(500, 100)), Env{})
	if scaled.H < plain.H*1.8 {
		t.Fatalf("scaled text %v beside %v, want about twice as tall", scaled, plain)
	}
	var got Rect
	tr := Transition(probe(50, 20, &got)).Slide(0, 30).Duration(time.Second)
	p := NewProbe(Provide(ReducedMotionKey, true, Column(tr)), Sz(100, 100))
	defer p.Close()
	p.Frame()
	if got.Origin.Y != 0 {
		t.Fatalf("first frame at y=%v under reduced motion, want in place", got.Origin.Y)
	}
	if (Env{}).Motion(time.Second) != time.Second || (Env{}).With(ReducedMotionKey, true).Motion(time.Second) != 0 {
		t.Fatal("Env.Motion")
	}
}

// testColor is an EnvColor held under testColorKey.
type testColor struct{}

var testColorKey = NewEnvKey[color.Color]("test color")

func (testColor) Resolve(env Env) color.Color { c, _ := env.Get(testColorKey); return c }

func (testColor) RGBA() (r, g, b, a uint32) { return 0, 0, 0, 0 }

// A Box and a Text paint an EnvColor as the Env they were laid out in has
// it, and setting one makes them lay out again to learn it.
func TestEnvColorsResolveAtLayout(t *testing.T) {
	red, blue := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}
	text := Text("x").Color(testColor{})
	box := Box(text).Fill(testColor{}).Border(1, testColor{}).Shadow(ShadowStyle{Color: testColor{}})
	for _, want := range []color.Color{red, blue} {
		box.Layout(Loose(Sz(100, 100)), Env{}.With(testColorKey, color.Color(want)))
		if box.fill.shown != want || box.borderColor.shown != want || box.deco.shownShadows[0].Color != want {
			t.Errorf("box shows %v, %v, %v; want %v", box.fill.shown, box.borderColor.shown, box.deco.shownShadows[0].Color, want)
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

func TestTextResolvesEnvThenOwnThenDefaults(t *testing.T) {
	t.Parallel()
	env := Env{}.WithText(TextStyle{Size: 20, Color: red})
	w := Text("x").Color(blue)
	w.Layout(Loose(Sz(100, 100)), env)
	if w.resolved.Color != blue || w.resolved.Size != 20 {
		t.Fatalf("resolved = %+v, want own blue over inherited size 20", w.resolved)
	}
	if w.resolved.LineHeight != 1.2 || w.resolved.Font == nil {
		t.Fatalf("resolved = %+v, want defaults for line height and font", w.resolved)
	}
	plain := Text("x")
	plain.Layout(Loose(Sz(100, 100)), Env{})
	if plain.resolved.Size != DefaultTextSize || plain.resolved.Color != color.Black {
		t.Fatalf("resolved = %+v under an empty Env, want built-in defaults", plain.resolved)
	}
}

func TestStyledSetsInheritedStyleForSubtree(t *testing.T) {
	t.Parallel()
	inner, outer := Text("in"), Text("out")
	Column(Styled(Column(inner)).Color(red).Size(10), outer).Layout(Loose(Sz(100, 100)), Env{})
	if inner.resolved.Color != red || inner.resolved.Size != 10 {
		t.Fatalf("inner resolved = %+v, want red at 10", inner.resolved)
	}
	if outer.resolved.Color != color.Black || outer.resolved.Size != DefaultTextSize {
		t.Fatalf("outer resolved = %+v, want untouched defaults", outer.resolved)
	}
}

func TestStyledNestsAndOwnSettersWin(t *testing.T) {
	t.Parallel()
	w := Text("x").Size(30)
	Styled(Styled(w).Color(blue)).Color(red).Size(10).Layout(Loose(Sz(100, 100)), Env{})
	if w.resolved.Color != blue || w.resolved.Size != 30 {
		t.Fatalf("resolved = %+v, want the nearer Styled's blue and the widget's own 30", w.resolved)
	}
}

func TestInheritedSizeChangesLayout(t *testing.T) {
	t.Parallel()
	small := Text("hello").Layout(Loose(Sz(1000, 1000)), Env{}.WithText(TextStyle{Size: 10}))
	big := Text("hello").Layout(Loose(Sz(1000, 1000)), Env{}.WithText(TextStyle{Size: 20}))
	if big.W <= small.W || big.H <= small.H {
		t.Fatalf("inherited size 20 = %+v not larger than 10 = %+v", big, small)
	}
}

func TestBoxDecorationsTolerateNilCanvas(t *testing.T) {
	t.Parallel()
	b := Box().Size(10, 10).Fill(red).Radius(4).Border(1, blue)
	b.Paint(nil, Rct(Pt(0, 0), b.Layout(Loose(Sz(10, 10)), Env{})))
	b.Paint(&Canvas{}, Rct(Pt(0, 0), Sz(10, 10)))
}

// ResolveText is the style a Text draws in, so a label drawn with DrawText
// matches a Text beside it.
func TestResolveTextMatchesText(t *testing.T) {
	t.Parallel()
	red := color.RGBA{R: 255, A: 255}
	env := Env{}.WithText(TextStyle{Size: 20, Color: red}).With(TextScaleKey, 1.5)
	got := env.ResolveText(TextStyle{LineHeight: 2})
	want := TextStyle{Font: DefaultFont(), Size: 30, Color: red, LineHeight: 2}
	if got != want {
		t.Fatalf("ResolveText = %+v, want %+v", got, want)
	}
	txt := Text("x").LineHeight(2)
	txt.Layout(Loose(Sz(100, 100)), env)
	if txt.resolved != got {
		t.Fatalf("Text resolved %+v, ResolveText %+v", txt.resolved, got)
	}
}
