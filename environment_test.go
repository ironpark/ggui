package ggui

import (
	"image/color"
	"testing"
	"time"
)

func TestEnvironmentReplacementsPreserveParentsWithoutAccumulatingBindings(t *testing.T) {
	t.Parallel()
	key := NewEnvKey[int]("value")
	other := NewEnvKey[string]("other")
	parent := Env{}.With(key, 1).With(other, "kept")
	env := parent
	for i := 2; i < 102; i++ {
		env = env.With(key, i)
	}
	if got, _ := parent.Get(key); got != 1 {
		t.Fatal("replacing an inherited value mutated the parent")
	}
	if got, _ := env.Get(other); got != "kept" {
		t.Fatal("replacement lost an unrelated key")
	}
	count := 0
	for n := env.vals; n != nil; n = n.next {
		count++
	}
	if count != 2 {
		t.Fatalf("repeated root changes retained %d bindings", count)
	}
}

func TestCoreStylesWorkWithoutTheme(t *testing.T) {
	t.Parallel()
	env := Env{}.With(EditorStyleKey, EditorStyle{Muted: color.White, Selection: color.Black}).
		With(ScrollStyleKey, ScrollStyle{Color: color.White, HoverColor: color.Black}).
		With(PopupDurationKey, time.Duration(0)).With(SpacingKey, 0.0)
	input := TextInput(State("text")).Disabled(true)
	input.Layout(Loose(Sz(200, 40)), env)
	if input.resolved.Color != color.White || input.selection != color.Black {
		t.Fatal("editor ignored explicit environment colors")
	}
	if env.ScrollStyle().Duration != 0 || env.PopupDuration() != 0 || env.Spacing() != 0 {
		t.Fatal("zero style settings were replaced by defaults")
	}
}

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

func TestEachKeysByValue(t *testing.T) {
	t.Parallel()
	tags := State([]string{"a", "b"})
	builds := 0
	list := Each(tags, func(rowItem EachItem[string]) Widget { r := rowItem.Value; builds++; return TextOf(r) })
	p := NewProbe(list, Sz(100, 100))
	defer p.Close()
	p.Frame()
	tags.Set([]string{"b", "a", "c"})
	p.Frame()
	if builds != 3 {
		t.Fatalf("builds = %d, want 3: a and b are kept by value", builds)
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
