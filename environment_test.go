package ggui

import (
	"image/color"
	"testing"
	"time"
)

func TestEnvironmentReplacementsPreserveParentsWithoutAccumulatingBindings(t *testing.T) {
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
