package ggui

import (
	"image/color"
	"testing"

	"github.com/ironpark/ggui/internal/reactive"
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

func TestTextStyleMergeLetsSetFieldsWin(t *testing.T) {
	t.Parallel()
	red, blue := color.RGBA{0xff, 0, 0, 0xff}, color.RGBA{0, 0, 0xff, 0xff}
	base := TextStyle{Size: 14, Color: red, LineHeight: 1.2}
	got := base.Merge(TextStyle{Color: blue})
	if got.Size != 14 || got.Color != blue || got.LineHeight != 1.2 {
		t.Fatalf("Merge = %+v, want size 14 and line height kept, color blue", got)
	}
	if got := base.Merge(TextStyle{}); got != base {
		t.Fatalf("merging the zero style changed %+v to %+v", base, got)
	}
}

func TestProvideAndGetTravelDownTheTree(t *testing.T) {
	t.Parallel()
	disabled := NewEnvKey[bool]("disabled")
	other := NewEnvKey[bool]("other")
	var seen, seenOther, ok, okOther bool
	probe := FromFuncs(
		func(c Constraints, env Env) Size {
			seen, ok = env.Get(disabled)
			seenOther, okOther = env.Get(other)
			return Sz(1, 1)
		},
		func(*Canvas, Rect) {},
	)
	Provide(disabled, true, column(probe)).Layout(Loose(Sz(10, 10)), Env{})
	if !ok || !seen || okOther || seenOther {
		t.Fatalf("Get(disabled) = %v,%v Get(other) = %v,%v; want true,true false,false", seen, ok, seenOther, okOther)
	}
	if v, ok := (Env{}).Get(disabled); ok || v {
		t.Fatal("Get on an empty Env reported a value")
	}
}

func TestEnvIsAValue(t *testing.T) {
	t.Parallel()
	k := NewEnvKey[int]("n")
	base := Env{}.With(k, 1)
	child := base.With(k, 2)
	if v, _ := base.Get(k); v != 1 {
		t.Fatalf("parent env changed to %d after a child added a value", v)
	}
	if v, _ := child.Get(k); v != 2 {
		t.Fatalf("child env = %d, want the nearer 2", v)
	}
}

// The root Env starts from the text style SetEnv configured: what a Text
// laid out under it resolves, as TestResolveTextMatchesText has Text
// resolve as ResolveText does.
func TestRootEnvStartsFromConfiguredText(t *testing.T) {
	t.Parallel()
	red := color.RGBA{0xff, 0, 0, 0xff}
	old := Untrack(UseEnv)
	defer SetEnv(old)
	SetEnv(Env{}.WithText(TextStyle{Size: 33, Color: red}))
	if got := rootEnv().ResolveText(TextStyle{}); got.Size != 33 || got.Color != red {
		t.Fatalf("resolved = %+v, want the configured text style", got)
	}
}

// ResolveText takes the inherited style, scaled by the Env's text scale,
// under the style given, and the defaults under both.
func TestResolveTextScalesTheInheritedStyle(t *testing.T) {
	t.Parallel()
	red := color.RGBA{R: 255, A: 255}
	env := Env{}.WithText(TextStyle{Size: 20, Color: red}).With(TextScaleKey, 1.5)
	got := env.ResolveText(TextStyle{LineHeight: 2})
	want := TextStyle{Font: DefaultFont(), Size: 30, Color: red, LineHeight: 2}
	if got != want {
		t.Fatalf("ResolveText = %+v, want %+v", got, want)
	}
}

func TestSetEnvRebuildsReaders(t *testing.T) {
	t.Parallel()
	old := Untrack(UseEnv)
	defer SetEnv(old)
	builds := 0
	dispose := reactive.Observe(func() { builds++; _ = UseEnv().Background() })
	defer dispose()
	SetEnv(Env{}.With(BackgroundKey, color.Color(color.Black)))
	reactive.Flush()
	if builds != 2 {
		t.Fatalf("builds = %d after SetEnv, want 2", builds)
	}
}

type boxedValue struct{ n int }

func sameBoxed(a, b *boxedValue) bool { return *a == *b }

func TestEnvKeyFuncComparesByItsEquality(t *testing.T) {
	k := NewEnvKeyFunc("boxed", sameBoxed)
	base := Env{}.With(SpacingKey, 3.)
	first := base.With(k, &boxedValue{1})
	again := base.With(k, &boxedValue{1})
	if again.rev != first.rev {
		t.Fatal("an equal value under the same parent gave a new revision")
	}
	kept, _ := first.Get(k)
	if got, _ := again.Get(k); got != kept {
		t.Fatal("an equal value replaced the one stored first")
	}
	if first.With(k, &boxedValue{1}).rev != first.rev {
		t.Fatal("storing an equal value over itself changed the Env")
	}
	if base.With(k, &boxedValue{2}).rev == first.rev {
		t.Fatal("a different value kept the revision")
	}
	if k1, k2 := NewEnvKeyFunc("a", sameBoxed), NewEnvKeyFunc("a", sameBoxed); k1 == k2 {
		t.Fatal("two keys are the same key")
	}
}

// More values than an interning list would hold, derived from one parent
// in turn, as a page of many themed subtrees does each layout: each finds
// its Env again.
func TestEnvKeyFuncKeepsManyValuesUnderOneParent(t *testing.T) {
	k := NewEnvKeyFunc("boxed", sameBoxed)
	base := Env{}.With(SpacingKey, 4.)
	revs := make([]uint64, 48)
	for round := range 3 {
		for i := range revs {
			rev := base.With(k, &boxedValue{i}).rev
			if round > 0 && rev != revs[i] {
				t.Fatalf("value %d got a new revision in round %d", i, round)
			}
			revs[i] = rev
		}
	}
}
