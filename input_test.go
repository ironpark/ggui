package ggui

import (
	"fmt"
	"github.com/ironpark/ggui/internal/reactive"
	"runtime"
	"testing"
)

// clickAt presses and releases the left button at p.
func clickAt(in *inputState, p Point) {
	in.dispatch(frameInput{pos: p, down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{pos: p, up: []MouseButton{MouseButtonLeft}})
}

// paintFrame lays out and paints w into a fresh region list, the way App
// does once per frame, and points in at it.
func paintFrame(in *inputState, w Widget, size Size) {
	// A frame settles its effects before it lays out, as the runtime's tick
	// does: a widget that follows a binding has then seen the last write.
	reactive.Flush()
	c := Canvas{prev: in.regions}
	w.Paint(&c, Rct(Pt(0, 0), w.Layout(Tight(size), Env{})))
	in.regions = c.hits
}

func TestTapFiresOnDownAndUpInside(t *testing.T) {
	taps := 0
	w := Column(Box().Size(50, 50), Tap(Box().Size(50, 50), func() { taps++ }))
	var in inputState
	paintFrame(&in, w, Sz(50, 100))

	inside, outside := Pt(10, 75), Pt(10, 25)
	in.dispatch(frameInput{pos: inside, down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{pos: inside, up: []MouseButton{MouseButtonLeft}})
	if taps != 1 {
		t.Fatalf("taps = %d after down+up inside, want 1", taps)
	}
	in.dispatch(frameInput{pos: inside, down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{pos: outside, up: []MouseButton{MouseButtonLeft}})
	if taps != 1 {
		t.Fatalf("taps = %d after releasing outside, want still 1", taps)
	}
	in.dispatch(frameInput{pos: outside, down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{pos: outside, up: []MouseButton{MouseButtonLeft}})
	if taps != 1 {
		t.Fatalf("taps = %d after clicking the plain box, want still 1", taps)
	}
}

func TestTapSurvivesRebuildBetweenDownAndUp(t *testing.T) {
	taps := 0
	build := func() Widget { return Tap(Box().Size(50, 50), func() { taps++ }) }
	var in inputState
	paintFrame(&in, build(), Sz(50, 50))
	in.dispatch(frameInput{pos: Pt(5, 5), down: []MouseButton{MouseButtonLeft}})
	paintFrame(&in, build(), Sz(50, 50)) // state changed, tree rebuilt
	in.dispatch(frameInput{pos: Pt(5, 5), up: []MouseButton{MouseButtonLeft}})
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
		in.dispatch(frameInput{pos: p, down: []MouseButton{MouseButtonLeft}})
		in.dispatch(frameInput{pos: p, up: []MouseButton{MouseButtonLeft}})
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
	if scrolled != (Point{X: 0, Y: -3}) {
		t.Fatalf("scroll reached %v, want {0 -3} through the tap region", scrolled)
	}
	if tapped {
		t.Fatal("scrolling tapped")
	}
}

func TestFocusRoutesKeys(t *testing.T) {
	var keys []KeyboardKey
	var typed string
	var focus []bool
	w := Row(
		Focus(Box().Size(50, 50)).
			OnKey(func(k KeyboardKey) { keys = append(keys, k) }).
			OnText(func(s string) { typed += s }).
			OnFocus(func(b bool) { focus = append(focus, b) }),
		Box().Size(50, 50),
	)
	var in inputState
	paintFrame(&in, w, Sz(100, 50))

	in.dispatch(frameInput{pos: Pt(10, 10), keys: []KeyboardKey{KeyA}})
	if len(keys) != 0 {
		t.Fatal("keys delivered before any click")
	}
	in.dispatch(frameInput{pos: Pt(10, 10), down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{pos: Pt(10, 10), keys: []KeyboardKey{KeyA}, text: "a"})
	if len(keys) != 1 || keys[0] != KeyA || typed != "a" {
		t.Fatalf("keys = %v, typed = %q; want [KeyA] and \"a\"", keys, typed)
	}
	in.dispatch(frameInput{pos: Pt(75, 10), down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{pos: Pt(75, 10), keys: []KeyboardKey{KeyB}})
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
	in.dispatch(frameInput{pos: Pt(10, 10), down: []MouseButton{MouseButtonLeft}})
	paintFrame(&in, Box().Size(50, 50), Sz(50, 50))
	in.dispatch(frameInput{pos: Pt(10, 10)})
	if !blurred || in.focused != nil {
		t.Fatalf("blurred = %v, focused = %v; want blur and no focus", blurred, in.focused)
	}
}

func TestPressedRegionCapturesDragAndRelease(t *testing.T) {
	var drags, ups int
	var last Point
	w := Pointer(Box().Size(50, 50)).
		OnDrag(func(ev PointerEvent) { drags++; last = ev.Pos }).
		OnUp(func(PointerEvent) { ups++ })
	var in inputState
	paintFrame(&in, w, Sz(50, 50))
	in.dispatch(frameInput{pos: Pt(10, 10), down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{pos: Pt(200, 200)})
	in.dispatch(frameInput{pos: Pt(300, 300)})
	in.dispatch(frameInput{pos: Pt(300, 300), up: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{pos: Pt(300, 300)})
	if drags != 2 || last != Pt(300, 300) || ups != 1 {
		t.Fatalf("drags = %d (last %v), ups = %d; want 2 drags outside and the release", drags, last, ups)
	}
}

func TestPointerAroundFocusSharesOneRegion(t *testing.T) {
	taps := 0
	focused := false
	w := Tap(Focus(Box().Size(50, 50)).OnFocus(func(b bool) { focused = b }), func() { taps++ })
	var in inputState
	paintFrame(&in, w, Sz(50, 50))
	if len(in.regions) != 1 {
		t.Fatalf("%d regions for one Rect, want 1 merged", len(in.regions))
	}
	in.dispatch(frameInput{pos: Pt(5, 5), down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{pos: Pt(5, 5), up: []MouseButton{MouseButtonLeft}})
	if taps != 1 || !focused {
		t.Fatalf("taps = %d focused = %v", taps, focused)
	}
}

func TestCursorFollowsTopmostRegion(t *testing.T) {
	w := Stack(
		Pointer(Box().Size(100, 100)).Cursor(CursorShapeCrosshair),
		Pointer(Box().Size(50, 50)).Cursor(CursorShapeText),
	)
	var in inputState
	paintFrame(&in, w, Sz(100, 100))
	in.dispatch(frameInput{pos: Pt(25, 25)})
	if in.cursor != CursorShapeText {
		t.Fatalf("cursor = %v, want text from the top region", in.cursor)
	}
	in.dispatch(frameInput{pos: Pt(75, 75)})
	if in.cursor != CursorShapeCrosshair {
		t.Fatalf("cursor = %v, want crosshair", in.cursor)
	}
	in.dispatch(frameInput{pos: Pt(150, 150)})
	if in.cursor != CursorShapeDefault {
		t.Fatalf("cursor = %v outside, want default", in.cursor)
	}
}

func TestPressedRegionSurvivesBufferReuse(t *testing.T) {
	var drags int
	w := Row(
		Pointer(Box().Size(50, 50)).OnDrag(func(PointerEvent) { drags++ }),
		Pointer(Box().Size(50, 50)).OnDrag(func(PointerEvent) { t.Fatal("drag went to the wrong region") }),
	)
	var in inputState
	paintFrame(&in, w, Sz(100, 50))
	in.dispatch(frameInput{pos: Pt(10, 10), down: []MouseButton{MouseButtonLeft}})
	// The runtime repaints into the same buffer; the pressed region must
	// not follow whatever lands at its old index.
	in.regions[0], in.regions[1] = in.regions[1], in.regions[0]
	in.dispatch(frameInput{pos: Pt(10, 10)})
	if drags != 1 {
		t.Fatalf("drags = %d, want 1 on the region that took the press", drags)
	}
}

func TestAdopterTakesOverAtTheSameRect(t *testing.T) {
	build := func() *adopting { return &adopting{} }
	var in inputState
	first := build()
	paintFrame(&in, first, Sz(50, 50))
	first.n = 7
	second := build()
	paintFrame(&in, second, Sz(50, 50))
	if second.n != 7 || second.from != first {
		t.Fatalf("second adopted n = %d from %p, want 7 from the first", second.n, second.from)
	}
	third := build()
	paintFrame(&in, Padding(third, 10), Sz(70, 70)) // a different Rect: nothing to adopt
	if third.from != nil {
		t.Fatal("adopted across different Rects")
	}
}

// adopting is a 50x50 pointer region that carries n across rebuilds.
type adopting struct {
	n    int
	from *adopting
}

func (a *adopting) Layout(c Constraints, _ Env) Size { return c.Constrain(Sz(50, 50)) }
func (a *adopting) Paint(dst *Canvas, r Rect)        { dst.HitPointer(r, a) }
func (a *adopting) HandlePointer(PointerEvent) bool  { return true }
func (a *adopting) Adopt(prev any) {
	if p, ok := prev.(*adopting); ok {
		a.n, a.from = p.n, p
	}
}

func TestModsCmdIsPlatformSpecific(t *testing.T) {
	m := Mods{Meta: true}
	if m.Cmd() != (runtime.GOOS == "darwin") {
		t.Fatalf("Meta counts as Cmd = %v on %s", m.Cmd(), runtime.GOOS)
	}
}

// keyed is a 50x50 focusable region that records focus events.
type keyed struct {
	log *[]string
	id  string
}

func (k *keyed) Layout(c Constraints, _ Env) Size { return c.Constrain(Sz(50, 50)) }
func (k *keyed) Paint(dst *Canvas, r Rect)        { dst.HitKey(r, k) }
func (k *keyed) HandleKey(ev KeyEvent) {
	switch ev.Kind {
	case KeyFocus:
		*k.log = append(*k.log, k.id+pick(ev.Key == KeyTab, "+tab", ""))
	case KeyBlur:
		*k.log = append(*k.log, "-"+k.id)
	case KeyPress:
		*k.log = append(*k.log, k.id+":"+ev.Key.String())
	}
}

func TestTabMovesFocusInPaintOrder(t *testing.T) {
	var log []string
	w := Row(&keyed{&log, "a"}, Box().Size(50, 50), &keyed{&log, "b"}, &keyed{&log, "c"})
	var in inputState
	paintFrame(&in, w, Sz(200, 50))
	tab := func(shift bool) { in.dispatch(frameInput{keys: []KeyboardKey{KeyTab}, mods: Mods{Shift: shift}}) }
	tab(false)
	tab(false)
	tab(false)
	tab(false) // wraps
	tab(true)  // back
	want := []string{"a+tab", "-a", "b+tab", "-b", "c+tab", "-c", "a+tab", "-a", "c+tab"}
	if fmt.Sprint(log) != fmt.Sprint(want) {
		t.Fatalf("focus log = %v\nwant %v", log, want)
	}
	log = nil
	in.dispatch(frameInput{keys: []KeyboardKey{KeyTab, KeyA}})
	if fmt.Sprint(log) != fmt.Sprint([]string{"-c", "a+tab", "a:A"}) {
		t.Fatalf("Tab must move focus and be withheld, other keys delivered: %v", log)
	}
	clickAt(&in, Pt(125, 25))
	if log[len(log)-1] != "b" {
		t.Fatalf("mouse focus must not carry the tab mark: %v", log)
	}
}

func TestTooltipAppearsAfterHover(t *testing.T) {
	tip := Tooltip(Box().Size(50, 50), "hint").Delay(0)
	var c Canvas
	c.Paint(tip, Rct(Pt(0, 0), tip.Layout(Loose(Sz(100, 100)), Env{})))
	if len(c.fs().overlays) != 0 {
		t.Fatal("tooltip queued without a pointer")
	}
	c.fs().pointer, c.fs().hasPointer = Pt(10, 10), true
	c.Paint(tip, Rct(Pt(0, 0), Sz(50, 50)))
	if len(c.fs().overlays) != 1 {
		t.Fatalf("%d overlays with the pointer inside, want 1", len(c.fs().overlays))
	}
	c.paintOverlays()
	if len(c.fs().overlays) != 0 {
		t.Fatal("overlays not cleared")
	}
	c.fs().pointer = Pt(80, 80)
	c.Paint(tip, Rct(Pt(0, 0), Sz(50, 50)))
	if len(c.fs().overlays) != 0 {
		t.Fatal("tooltip queued with the pointer outside")
	}
}

func TestOnKeyRunsBeforeTheFocusedWidgetAndCanConsume(t *testing.T) {
	var log []string
	w := &keyed{&log, "a"}
	p := NewProbe(w, Sz(50, 50))
	p.Click(Pt(10, 10))
	seen := 0
	p.OnKey(func(ev KeyEvent) bool { seen++; return ev.Key == KeyF1 })
	p.Type(Mods{}, KeyF1, KeyA)
	if seen != 2 {
		t.Fatalf("shortcut saw %d keys, want 2", seen)
	}
	log = log[len(log):]
	p.Type(Mods{}, KeyF1)
	if len(log) != 0 {
		t.Fatalf("widget got %v for a consumed key, want nothing", log)
	}
}
