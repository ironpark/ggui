package ggui

import (
	"image/color"
	"testing"

	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/property"
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

// TestCurrentEnvIsTheSubtreesInLayoutAndTheAppsElsewhere reads
// CurrentEnv from a widget under a Provide as it lays out, as it paints and
// from an overlay it schedules, from one beside it, and from outside any
// frame: the first three see the subtree's value, the one beside it the
// application's, and the last the application's too.
func TestCurrentEnvIsTheSubtreesInLayoutAndTheAppsElsewhere(t *testing.T) {
	t.Parallel()
	lang := NewEnvKey[string]("lang")
	read := func() string { v, _ := CurrentEnv().Get(lang); return v }
	var laid, painted, overlaid, beside string
	inner := FromFuncs(func(Constraints, Env) Size {
		laid = read()
		return Sz(1, 1)
	}, func(dst *Canvas, _ Rect) {
		painted = read()
		dst.Overlay(func(*Canvas) { overlaid = read() })
	})
	outer := FromFuncs(func(Constraints, Env) Size {
		beside = read()
		return Sz(1, 1)
	}, func(*Canvas, Rect) {})
	p := NewProbe(column(Provide(lang, "ko", inner), outer), Sz(10, 10))
	defer p.Close()
	p.Setup(func() { SetEnv(UseEnv().With(lang, "en")) })
	p.Frame()
	if laid != "ko" || painted != "ko" || overlaid != "ko" {
		t.Errorf("under the Provide: layout %q, paint %q, overlay %q; want ko", laid, painted, overlaid)
	}
	if beside != "en" {
		t.Errorf("beside the Provide = %q, want the application's en", beside)
	}
	if got := read(); got != "en" {
		t.Errorf("after the frame = %q, want the application's en", got)
	}
}

// TestCurrentEnvSubscribesOnlyOutsideLayout reads CurrentEnv in two
// observers, one of them as layout would: only the other runs again when
// the application environment is replaced.
func TestCurrentEnvSubscribesOnlyOutsideLayout(t *testing.T) {
	t.Parallel()
	k := NewEnvKey[int]("n")
	var outside, inside int
	stop := Observe(func() {
		outside++
		CurrentEnv()
	})
	defer stop()
	stopIn := Observe(func() {
		inside++
		defer reactive.EnterLayout()()
		CurrentEnv()
	})
	defer stopIn()
	SetEnv(UseEnv().With(k, 1))
	reactive.Base().Flush()
	if outside != 2 {
		t.Errorf("read outside layout ran %d times, want 2", outside)
	}
	if inside != 1 {
		t.Errorf("read in layout ran %d times, want 1: it subscribed", inside)
	}
}

type testEnvToken string

func (t testEnvToken) RGBA() (uint32, uint32, uint32, uint32) { return color.White.RGBA() }
func (t testEnvToken) Resolve(Env) color.Color                { return color.White }

func TestEqualColorHandlesEnvColorAndTypedNilAndEnvMemoNonComparable(t *testing.T) {
	t.Parallel()
	// Two distinct EnvColor tokens with the same root-theme RGBA must compare unequal.
	if property.Equal[color.Color](testEnvToken("a"), testEnvToken("b")) {
		t.Fatal("distinct EnvColor tokens compared equal")
	}
	if !property.Equal[color.Color](testEnvToken("a"), testEnvToken("a")) {
		t.Fatal("identical EnvColor tokens compared unequal")
	}
	if property.Equal[color.Color](testEnvToken("a"), color.White) {
		t.Fatal("EnvColor token compared equal to concrete color")
	}
	var nilRGBA *color.RGBA
	if !property.Equal[color.Color](nilRGBA, nil) || !(TextStyle{Color: nilRGBA}).Equal(TextStyle{}) {
		t.Fatal("typed-nil color did not compare equal to nil")
	}
	if (TextStyle{Color: testEnvToken("a")}).Equal(TextStyle{Color: testEnvToken("b")}) {
		t.Fatal("TextStyle.Equal treated distinct EnvColors as equal")
	}

	// Non-comparable Env values (including structs with uncomparable interfaces)
	// must not panic in Env.With / Env.memo.
	type wrap struct{ V any }
	kSlice := NewEnvKey[[]int]("slice")
	kWrap := NewEnvKey[wrap]("wrap")
	e := Env{}.With(kSlice, []int{1, 2}).With(kWrap, wrap{V: []int{3}})
	if got, ok := e.Get(kSlice); !ok || len(got) != 2 {
		t.Fatalf("slice env value = %v, %v", got, ok)
	}

	// envMemo.put updating an existing key at capacity must not rotate generations.
	var m envMemo
	for i := range envMemoMin {
		m.put(derivedKey{from: uint64(i), key: textKey, val: i}, Env{rev: uint64(i)})
	}
	m.put(derivedKey{from: 0, key: textKey, val: 0}, Env{rev: 999})
	if len(m.prev) != 0 {
		t.Fatalf("updating an existing envMemo key rotated generations (prev=%d)", len(m.prev))
	}
}
