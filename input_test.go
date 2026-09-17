package ggui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// paintFrame lays out and paints w into a fresh region list, the way App
// does once per frame, and points in at it.
func paintFrame(in *inputState, w Widget, size Size) {
	var c Canvas
	w.Paint(&c, Rct(Pt(0, 0), w.Layout(Tight(size), Env{})))
	in.regions = c.hits
}

func TestTapFiresOnDownAndUpInside(t *testing.T) {
	taps := 0
	w := Column(Box().Size(50, 50), Tap(Box().Size(50, 50), func() { taps++ }))
	var in inputState
	paintFrame(&in, w, Sz(50, 100))

	inside, outside := Pt(10, 75), Pt(10, 25)
	in.dispatch(frameInput{pos: inside, down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	in.dispatch(frameInput{pos: inside, up: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	if taps != 1 {
		t.Fatalf("taps = %d after down+up inside, want 1", taps)
	}
	in.dispatch(frameInput{pos: inside, down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	in.dispatch(frameInput{pos: outside, up: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	if taps != 1 {
		t.Fatalf("taps = %d after releasing outside, want still 1", taps)
	}
	in.dispatch(frameInput{pos: outside, down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	in.dispatch(frameInput{pos: outside, up: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	if taps != 1 {
		t.Fatalf("taps = %d after clicking the plain box, want still 1", taps)
	}
}

func TestTapSurvivesRebuildBetweenDownAndUp(t *testing.T) {
	taps := 0
	build := func() Widget { return Tap(Box().Size(50, 50), func() { taps++ }) }
	var in inputState
	paintFrame(&in, build(), Sz(50, 50))
	in.dispatch(frameInput{pos: Pt(5, 5), down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	paintFrame(&in, build(), Sz(50, 50)) // state changed, tree rebuilt
	in.dispatch(frameInput{pos: Pt(5, 5), up: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	if taps != 1 {
		t.Fatalf("taps = %d, want 1 delivered to the rebuilt widget", taps)
	}
}

func TestTopmostRegionWins(t *testing.T) {
	var outer, inner int
	w := Tap(Padding(Tap(Box().Size(20, 20), func() { inner++ }), 10), func() { outer++ })
	var in inputState
	paintFrame(&in, w, Sz(40, 40))
	click := func(p Point) {
		in.dispatch(frameInput{pos: p, down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
		in.dispatch(frameInput{pos: p, up: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	}
	click(Pt(20, 20))
	click(Pt(2, 2))
	if inner != 1 || outer != 1 {
		t.Fatalf("inner = %d, outer = %d; want 1 and 1", inner, outer)
	}
}

func TestHoverEnterAndExit(t *testing.T) {
	var log []bool
	w := Pointer(Box().Size(50, 50)).OnHover(func(b bool) { log = append(log, b) })
	var in inputState
	paintFrame(&in, w, Sz(50, 50))
	in.dispatch(frameInput{pos: Pt(10, 10)})
	in.dispatch(frameInput{pos: Pt(20, 20)})
	in.dispatch(frameInput{pos: Pt(80, 80)})
	if len(log) != 2 || !log[0] || log[1] {
		t.Fatalf("hover log = %v, want [true false]", log)
	}
}

func TestUnhandledEventsFallThrough(t *testing.T) {
	var scrolled Point
	tapped := false
	w := Pointer(Tap(Box().Size(50, 50), func() { tapped = true })).OnScroll(func(d Point) { scrolled = d })
	var in inputState
	paintFrame(&in, w, Sz(50, 50))
	in.dispatch(frameInput{pos: Pt(10, 10), wheel: Pt(0, -3)})
	if scrolled != (Point{0, -3}) {
		t.Fatalf("scroll reached %v, want {0 -3} through the tap region", scrolled)
	}
	if tapped {
		t.Fatal("scrolling tapped")
	}
}

func TestFocusRoutesKeys(t *testing.T) {
	var keys []ebiten.Key
	var typed string
	var focus []bool
	w := Row(
		Focus(Box().Size(50, 50)).
			OnKey(func(k ebiten.Key) { keys = append(keys, k) }).
			OnText(func(s string) { typed += s }).
			OnFocus(func(b bool) { focus = append(focus, b) }),
		Box().Size(50, 50),
	)
	var in inputState
	paintFrame(&in, w, Sz(100, 50))

	in.dispatch(frameInput{pos: Pt(10, 10), keys: []ebiten.Key{ebiten.KeyA}})
	if len(keys) != 0 {
		t.Fatal("keys delivered before any click")
	}
	in.dispatch(frameInput{pos: Pt(10, 10), down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	in.dispatch(frameInput{pos: Pt(10, 10), keys: []ebiten.Key{ebiten.KeyA}, text: "a"})
	if len(keys) != 1 || keys[0] != ebiten.KeyA || typed != "a" {
		t.Fatalf("keys = %v, typed = %q; want [KeyA] and \"a\"", keys, typed)
	}
	in.dispatch(frameInput{pos: Pt(75, 10), down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	in.dispatch(frameInput{pos: Pt(75, 10), keys: []ebiten.Key{ebiten.KeyB}})
	if len(keys) != 1 {
		t.Fatalf("keys = %v, want none after clicking outside", keys)
	}
	if len(focus) != 2 || !focus[0] || focus[1] {
		t.Fatalf("focus log = %v, want [true false]", focus)
	}
}

func TestFocusLostWhenRegionDisappears(t *testing.T) {
	blurred := false
	w := Focus(Box().Size(50, 50)).OnFocus(func(b bool) { blurred = !b })
	var in inputState
	paintFrame(&in, w, Sz(50, 50))
	in.dispatch(frameInput{pos: Pt(10, 10), down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	paintFrame(&in, Box().Size(50, 50), Sz(50, 50))
	in.dispatch(frameInput{pos: Pt(10, 10)})
	if !blurred || in.focused != nil {
		t.Fatalf("blurred = %v, focused = %v; want blur and no focus", blurred, in.focused)
	}
}
