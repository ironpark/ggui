package ggui

import (
	"image/color"
	"testing"
	"time"
)

func TestThemeTokens(t *testing.T) {
	k := NewEnvKey[color.Color]("danger")
	base := DefaultTheme()
	red := base.Set(k, color.Color(color.RGBA{0xd3, 0x2f, 0x2f, 0xff}))
	if _, ok := base.Get(k); ok {
		t.Fatal("Set changed its receiver")
	}
	if c, ok := red.Get(k); !ok || c != color.Color(color.RGBA{0xd3, 0x2f, 0x2f, 0xff}) {
		t.Fatalf("Get = %v, %v", c, ok)
	}
	if got, _ := red.Set(k, color.Color(color.Black)).Get(k); got != color.Color(color.Black) {
		t.Fatal("a later Set does not win")
	}
}

func TestEachKeysByValue(t *testing.T) {
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
