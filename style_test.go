package ggui

import (
	"image/color"
	"testing"
)

var (
	red  = color.RGBA{0xff, 0, 0, 0xff}
	blue = color.RGBA{0, 0, 0xff, 0xff}
)

func TestTextStyleMergeLetsSetFieldsWin(t *testing.T) {
	base := TextStyle{Size: 14, Color: red, LineHeight: 1.2}
	got := base.Merge(TextStyle{Color: blue})
	if got.Size != 14 || got.Color != blue || got.LineHeight != 1.2 {
		t.Fatalf("Merge = %+v, want size 14 and line height kept, color blue", got)
	}
	if got := base.Merge(TextStyle{}); got != base {
		t.Fatalf("merging the zero style changed %+v to %+v", base, got)
	}
}

func TestTextResolvesEnvThenOwnThenDefaults(t *testing.T) {
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
	w := Text("x").Size(30)
	Styled(Styled(w).Color(blue)).Color(red).Size(10).Layout(Loose(Sz(100, 100)), Env{})
	if w.resolved.Color != blue || w.resolved.Size != 30 {
		t.Fatalf("resolved = %+v, want the nearer Styled's blue and the widget's own 30", w.resolved)
	}
}

func TestInheritedSizeChangesLayout(t *testing.T) {
	small := Text("hello").Layout(Loose(Sz(1000, 1000)), Env{}.WithText(TextStyle{Size: 10}))
	big := Text("hello").Layout(Loose(Sz(1000, 1000)), Env{}.WithText(TextStyle{Size: 20}))
	if big.W <= small.W || big.H <= small.H {
		t.Fatalf("inherited size 20 = %+v not larger than 10 = %+v", big, small)
	}
}

func TestProvideAndGetTravelDownTheTree(t *testing.T) {
	disabled := NewKey[bool]("disabled")
	other := NewKey[bool]("other")
	var seen, seenOther, ok, okOther bool
	probe := FromFuncs(
		func(c Constraints, env Env) Size {
			seen, ok = env.Get(disabled)
			seenOther, okOther = env.Get(other)
			return Sz(1, 1)
		},
		func(*Canvas, Rect) {},
	)
	Provide(disabled, true, Column(probe)).Layout(Loose(Sz(10, 10)), Env{})
	if !ok || !seen || okOther || seenOther {
		t.Fatalf("Get(disabled) = %v,%v Get(other) = %v,%v; want true,true false,false", seen, ok, seenOther, okOther)
	}
	if v, ok := (Env{}).Get(disabled); ok || v {
		t.Fatal("Get on an empty Env reported a value")
	}
}

func TestEnvIsAValue(t *testing.T) {
	k := NewKey[int]("n")
	base := Env{}.With(k, 1)
	child := base.With(k, 2)
	if v, _ := base.Get(k); v != 1 {
		t.Fatalf("parent env changed to %d after a child added a value", v)
	}
	if v, _ := child.Get(k); v != 2 {
		t.Fatalf("child env = %d, want the nearer 2", v)
	}
}

func TestRootEnvStartsFromTheme(t *testing.T) {
	old := theme.Peek()
	defer SetTheme(old)
	SetTheme(Theme{Text: TextStyle{Size: 33, Color: red}, Bg: blue})
	w := Text("x")
	w.Layout(Loose(Sz(100, 100)), rootEnv())
	if w.resolved.Size != 33 || w.resolved.Color != red {
		t.Fatalf("resolved = %+v, want the theme's text style", w.resolved)
	}
}

func TestSetThemeRebuildsReaders(t *testing.T) {
	old := theme.Peek()
	defer SetTheme(old)
	builds := 0
	dispose := Effect(func() { builds++; _ = UseTheme().Primary })
	defer dispose()
	SetTheme(DarkTheme())
	effects.flush()
	if builds != 2 {
		t.Fatalf("builds = %d after SetTheme, want 2", builds)
	}
}

func TestBoxDecorationsTolerateNilCanvas(t *testing.T) {
	b := Box().Size(10, 10).Fill(red).Radius(4).Border(1, blue)
	b.Paint(nil, Rct(Pt(0, 0), b.Layout(Loose(Sz(10, 10)), Env{})))
	b.Paint(&Canvas{}, Rct(Pt(0, 0), Sz(10, 10)))
}
