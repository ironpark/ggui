package ggui

import (
	"fmt"
	"math"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/pkg/kinetics"
	"github.com/ironpark/ggwd"
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// claimer is a keyed region that claims Tab and Ctrl chords, as a terminal
// does.
type claimer struct{ keyed }

func (k *claimer) Paint(dst *Canvas, r Rect) { dst.HitKey(r, k) }
func (k *claimer) ClaimsChord(ev KeyEvent) bool {
	return ev.Key == KeyTab || ev.Mods.Ctrl
}

func TestClaimedChordsSkipRuntimeBindings(t *testing.T) {
	t.Parallel()
	var log []string
	w := Row(&claimer{keyed{&log, "term"}}, &keyed{&log, "b"})
	var in inputState
	paintFrame(&in, w, Sz(100, 50))
	hooked := 0
	in.shortcuts = append(in.shortcuts, func(ev KeyEvent) bool { hooked++; return true })
	clickAt(&in, Pt(25, 25))
	log = nil
	in.dispatch(frameInput{keys: []KeyboardKey{KeyTab}})
	in.dispatch(frameInput{keys: []KeyboardKey{KeyTab}, mods: Mods{Shift: true}})
	in.dispatch(frameInput{keys: []KeyboardKey{KeyC}, mods: Mods{Ctrl: true}})
	if want := []string{"term:Tab", "term:Tab", "term:C"}; fmt.Sprint(log) != fmt.Sprint(want) {
		t.Fatalf("log = %v, want the claimed chords delivered and focus kept: %v", log, want)
	}
	if hooked != 0 {
		t.Fatalf("an OnKey hook saw %d claimed chords", hooked)
	}
	// An unclaimed key still goes to the hook first.
	in.dispatch(frameInput{keys: []KeyboardKey{KeyA}})
	if hooked != 1 {
		t.Fatalf("hook ran %d times for an unclaimed key, want 1", hooked)
	}
	// Focus elsewhere: Tab moves focus as usual, including onto the claimer.
	clickAt(&in, Pt(75, 25))
	log = nil
	in.shortcuts = nil
	in.dispatch(frameInput{keys: []KeyboardKey{KeyTab}})
	if want := []string{"-b", "term+tab"}; fmt.Sprint(log) != fmt.Sprint(want) {
		t.Fatalf("log = %v, want %v", log, want)
	}
}

func TestTabTellsApartRegionsSharingARect(t *testing.T) {
	t.Parallel()
	var log []string
	w := Stack(&keyed{&log, "a"}, &keyed{&log, "b"})
	var in inputState
	paintFrame(&in, w, Sz(50, 50))
	for range 3 {
		in.dispatch(frameInput{keys: []KeyboardKey{KeyTab}})
	}
	want := []string{"a+tab", "-a", "b+tab", "-b", "a+tab"}
	if fmt.Sprint(log) != fmt.Sprint(want) {
		t.Fatalf("focus log = %v\nwant %v", log, want)
	}
}

func TestOnKeyRunsBeforeTheFocusedWidgetAndCanConsume(t *testing.T) {
	t.Parallel()
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

// roving is a keyed region that is a Tab stop only when stop says so.
type roving struct {
	keyed
	stop bool
}

func (r *roving) Paint(dst *Canvas, rc Rect) { dst.HitKey(rc, r) }
func (r *roving) TabStop() bool              { return r.stop }

func TestTabSkipsWhatIsNotATabStop(t *testing.T) {
	t.Parallel()
	var log []string
	w := Row(&keyed{&log, "a"}, &roving{keyed{&log, "b"}, false}, &roving{keyed{&log, "c"}, true})
	var in inputState
	paintFrame(&in, w, Sz(200, 50))
	for range 3 {
		in.dispatch(frameInput{keys: []KeyboardKey{KeyTab}})
	}
	want := []string{"a+tab", "-a", "c+tab", "-c", "a+tab"}
	if fmt.Sprint(log) != fmt.Sprint(want) {
		t.Fatalf("focus log = %v\nwant %v", log, want)
	}
	log = nil
	clickAt(&in, Pt(75, 25))
	in.dispatch(frameInput{keys: []KeyboardKey{KeyTab}})
	if fmt.Sprint(log) != fmt.Sprint([]string{"-a", "b", "-b", "c+tab"}) {
		t.Fatalf("a click must still reach a region off the Tab order, and Tab leave it: %v", log)
	}
}

// hoverBox is a 50x50 control that tracks its hover and carries it across a
// rebuild, as Interactive does.
type hoverBox struct {
	id      string
	hovered bool
}

func (h *hoverBox) Layout(c Constraints, _ Env) Size { return c.Constrain(Sz(50, 50)) }
func (h *hoverBox) Paint(dst *Canvas, r Rect)        { dst.HitPointer(r, h) }
func (h *hoverBox) HitID() any                       { return h.id }
func (h *hoverBox) HandlePointer(ev PointerEvent) bool {
	switch ev.Kind {
	case PointerEnter, PointerMove:
		h.hovered = true
	case PointerExit:
		h.hovered = false
	}
	return true
}
func (h *hoverBox) Adopt(prev any) {
	if p, ok := prev.(*hoverBox); ok {
		h.hovered = p.hovered
	}
}

// A control rebuilt while hovered, as a tap that changes what is shown
// rebuilds it, takes the exit when the pointer leaves: the exit goes to the
// one holding the region, not to the one that was entered.
func TestHoverExitReachesARebuiltControl(t *testing.T) {
	t.Parallel()
	gen := State(0)
	var a *hoverBox
	p := ProbeBuilder(func() Widget {
		return Reactive(func() Widget {
			_ = gen.Get()
			a = &hoverBox{id: "a"}
			return Column(a, &hoverBox{id: "b"})
		})
	}, Sz(50, 100))
	defer p.Close()
	p.Move(Pt(25, 25))
	gen.Set(1) // rebuilt under the pointer
	p.Frame()
	if !a.hovered {
		t.Fatal("the rebuilt control did not take over the hover")
	}
	p.Move(Pt(25, 75))
	if a.hovered {
		t.Fatal("the rebuilt control is still hovered after the pointer left it")
	}
}

// modsBox records the keys held as the pointer moves over it.
type modsBox struct{ mods Mods }

func (b *modsBox) Layout(c Constraints, _ Env) Size { return c.Constrain(Sz(50, 50)) }
func (b *modsBox) Paint(dst *Canvas, r Rect)        { dst.HitPointer(r, b) }
func (b *modsBox) HandlePointer(ev PointerEvent) bool {
	if ev.Kind == PointerMove {
		b.mods = ev.Mods
	}
	return true
}

// A move says which keys are held, as ⌘ over a link that follows on click.
func TestPointerMoveCarriesTheHeldKeys(t *testing.T) {
	t.Parallel()
	b := &modsBox{}
	p := NewProbe(b, Sz(50, 50))
	defer p.Close()
	p.MoveWith(Pt(10, 10), Mods{Meta: true})
	if !b.mods.Meta {
		t.Fatal("the move did not say ⌘ was held")
	}
	p.Move(Pt(12, 10))
	if b.mods.Meta {
		t.Fatal("⌘ still held after it was let go")
	}
}

// A region that changes its cursor over the input of a frame shows the new
// one once that frame is painted, not at the next event: a key held over a
// link is not followed by a move to show the hand.
func TestCursorFollowsTheFramePainted(t *testing.T) {
	held := State(false)
	w := Reactive(func() Widget {
		shape := CursorShapeText
		if held.Get() {
			shape = CursorShapePointer
		}
		return Pointer(Box().Size(100, 100)).Cursor(shape).OnMove(func(ev PointerEvent) { held.Set(ev.Mods.Shift) })
	})
	p := NewProbe(w, Sz(100, 100))
	defer p.Close()
	p.Move(Pt(50, 50))
	p.Frame()
	if p.Cursor() != CursorShapeText {
		t.Fatalf("cursor %v, want text", p.Cursor())
	}
	p.MoveWith(Pt(50, 50), Mods{Shift: true})
	p.Frame()
	if p.Cursor() != CursorShapePointer {
		t.Fatalf("cursor %v after the frame, want the hand", p.Cursor())
	}
}

func TestARebuiltRegionIsToldItIsHovered(t *testing.T) {
	t.Parallel()
	var was, now []bool
	var in inputState
	paintFrame(&in, Pointer(Box().Size(50, 50)).OnHover(func(b bool) { was = append(was, b) }), Sz(50, 50))
	in.dispatch(frameInput{pos: Pt(10, 10)})
	// Rebuilt under the pointer, as a list redrawn for a change elsewhere
	// is: the new region holds a hover state of its own.
	paintFrame(&in, Pointer(Box().Size(50, 50)).OnHover(func(b bool) { now = append(now, b) }), Sz(50, 50))
	in.dispatch(frameInput{pos: Pt(10, 10)})
	in.dispatch(frameInput{pos: Pt(12, 12)})
	if len(now) != 1 || !now[0] {
		t.Fatalf("the rebuilt region heard %v of its hover, want [true]", now)
	}
	if len(was) != 1 || !was[0] {
		t.Fatalf("the region it replaced heard %v, want only its enter", was)
	}
	in.dispatch(frameInput{pos: Pt(80, 80)})
	if len(now) != 2 || now[1] {
		t.Fatalf("the rebuilt region heard %v as the pointer left, want [true false]", now)
	}
}

// releasing is a keyed region that is told of releases too.
type releasing struct{ keyed }

func (k *releasing) Paint(dst *Canvas, r Rect) { dst.HitKey(r, k) }
func (k *releasing) HandleKeyRelease(ev KeyEvent) {
	*k.log = append(*k.log, k.id+":-"+ev.Key.String())
}

func TestReleasesReachTheFocusedHandlerThatAsks(t *testing.T) {
	t.Parallel()
	var log []string
	w := Row(&releasing{keyed{&log, "a"}}, &keyed{&log, "b"})
	var in inputState
	paintFrame(&in, w, Sz(100, 50))
	clickAt(&in, Pt(25, 25))
	log = nil
	in.dispatch(frameInput{keys: []KeyboardKey{KeyA}, released: []KeyboardKey{KeyA}})
	if want := []string{"a:A", "a:-A"}; fmt.Sprint(log) != fmt.Sprint(want) {
		t.Fatalf("log = %v, want %v: the release after the press", log, want)
	}
	clickAt(&in, Pt(75, 25))
	log = nil
	in.dispatch(frameInput{keys: []KeyboardKey{KeyB}, released: []KeyboardKey{KeyB}})
	if want := []string{"b:B"}; fmt.Sprint(log) != fmt.Sprint(want) {
		t.Fatalf("log = %v, want %v: a handler that does not ask hears no releases", log, want)
	}
}

func TestTouchTapAndCapturedDrag(t *testing.T) {
	t.Parallel()
	var touch touchInput
	var in inputState
	var taps, ups, drags int
	var released Point
	w := Pointer(Box().Size(50, 50)).OnTap(func() { taps++ }).
		OnDrag(func(PointerEvent) { drags++ }).
		OnUp(func(ev PointerEvent) { ups++; released = ev.Pos })
	paintFrame(&in, w, Sz(50, 50))
	step := func(ids []ggwd.TouchID, pos Point) {
		f := frameInput{pos: Pt(999, 999)}
		touch.apply(&f, ids, func(ggwd.TouchID) Point { return pos })
		in.dispatch(f)
	}
	step([]ggwd.TouchID{0}, Pt(10, 10))
	step(nil, Point{})
	if taps != 1 || ups != 1 || released != Pt(10, 10) {
		t.Fatalf("tap/release = %d/%d at %v", taps, ups, released)
	}
	step([]ggwd.TouchID{1}, Pt(10, 10))
	step([]ggwd.TouchID{1}, Pt(80, 80))
	step(nil, Point{})
	if taps != 1 || ups != 2 || drags != 1 || released != Pt(80, 80) || in.pressed != nil {
		t.Fatalf("captured drag: taps=%d ups=%d drags=%d release=%v", taps, ups, drags, released)
	}
}

func TestTouchKeepsPrimaryAndWaitsForRemainingFingers(t *testing.T) {
	t.Parallel()
	var touch touchInput
	position := func(id ggwd.TouchID) Point { return Pt(float64(id), 10) }
	step := func(ids ...ggwd.TouchID) frameInput {
		f := frameInput{down: []MouseButton{MouseButtonRight}}
		touch.apply(&f, ids, position)
		return f
	}
	step(1)
	if f := step(2, 1); f.pos != position(1) || len(f.down) != 0 {
		t.Fatalf("second finger changed primary: %+v", f)
	}
	if f := step(2); len(f.up) != 1 || f.pos != position(1) {
		t.Fatalf("missing primary release: %+v", f)
	}
	if f := step(2); len(f.down) != 0 || len(f.up) != 0 {
		t.Fatalf("remaining finger started a new press: %+v", f)
	}
	step()
	if f := step(3); len(f.down) != 1 || f.down[0] != MouseButtonLeft || f.pos != position(3) {
		t.Fatalf("new gesture failed: %+v", f)
	}
}

func TestTouchPanScrollsAndCancelsTap(t *testing.T) {
	t.Parallel()
	var in inputState
	var touch touchInput
	taps, ups := 0, 0
	child := Pointer(Box().Size(100, 400)).OnTap(func() { taps++ }).OnUp(func(PointerEvent) { ups++ })
	s := Scroll(child).Speed(99)
	step := func(held bool, p Point) {
		paintFrame(&in, s, Sz(100, 100))
		var ids []ggwd.TouchID
		if held {
			ids = []ggwd.TouchID{1}
		}
		f := frameInput{}
		touch.apply(&f, ids, func(ggwd.TouchID) Point { return p })
		in.dispatch(f)
	}
	step(true, Pt(50, 80))
	step(true, Pt(50, 77))
	if s.position() != 0 {
		t.Fatal("small tap jitter scrolled")
	}
	step(false, Point{})
	if taps != 1 {
		t.Fatal("tap with jitter was lost")
	}
	step(true, Pt(50, 80))
	step(true, Pt(50, 60))
	if s.position() != 20 {
		t.Fatalf("offset = %v, want 20 logical pixels", s.position())
	}
	step(true, Pt(50, -10))
	if s.position() != 90 {
		t.Fatalf("captured offset = %v, want 90", s.position())
	}
	step(false, Point{})
	if taps != 1 || ups != 2 || in.pressed != nil || in.touchPan.Panning {
		t.Fatalf("pan did not cancel press: taps=%d ups=%d", taps, ups)
	}
}

func TestHorizontalTouchDragDoesNotScrollVerticalList(t *testing.T) {
	t.Parallel()
	var in inputState
	drags := 0
	s := Scroll(Pointer(Box().Size(100, 400)).OnDrag(func(PointerEvent) { drags++ }))
	paintFrame(&in, s, Sz(100, 100))
	in.dispatch(frameInput{touch: true, pos: Pt(10, 50), down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{touch: true, pos: Pt(40, 51)})
	if s.position() != 0 || drags != 1 || in.touchPan.Panning {
		t.Fatalf("horizontal drag stolen: offset=%v drags=%d", s.position(), drags)
	}
}

type touchDragControl struct{ *PointerWidget }

func (*touchDragControl) CaptureTouchDrag() bool { return true }
func (w *touchDragControl) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, w)
	dst.Paint(w.child, r)
}

func TestTouchCapturedControlDoesNotPanParent(t *testing.T) {
	t.Parallel()
	var in inputState
	drags, ups := 0, 0
	control := &touchDragControl{Pointer(Box().Size(100, 400)).
		OnDrag(func(PointerEvent) { drags++ }).
		OnUp(func(PointerEvent) { ups++ })}
	s := Scroll(control)
	paintFrame(&in, s, Sz(100, 100))
	in.dispatch(frameInput{touch: true, pos: Pt(50, 80), down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{touch: true, pos: Pt(50, 40)})
	in.dispatch(frameInput{touch: true, pos: Pt(50, -20)})
	in.dispatch(frameInput{touch: true, pos: Pt(50, -20), up: []MouseButton{MouseButtonLeft}})
	if drags != 2 || ups != 1 || in.pressed != nil {
		t.Fatalf("lost captured drag: drags=%d ups=%d pressed=%v", drags, ups, in.pressed)
	}
	if s.position() != 0 || in.touchPan.Panning || in.touchMotion.target != nil {
		t.Fatalf("captured control started scrolling: offset=%v", s.position())
	}
}

func TestTouchFlingSpeedHoldAndInterruption(t *testing.T) {
	t.Parallel()
	run := func(speed float64, hold bool, interrupt bool) float64 {
		var in inputState
		now := time.Unix(100, 0)
		restore := SetClock(func() time.Time { return now })
		defer restore()
		s := Scroll(Box().Size(100, 10000))
		paintFrame(&in, s, Sz(100, 100))
		p := Pt(50, 80)
		in.dispatch(frameInput{touch: true, pos: p, down: []MouseButton{MouseButtonLeft}})
		for i := 0; i < 10; i++ {
			now = now.Add(16 * time.Millisecond)
			p.Y -= speed * .016
			in.dispatch(frameInput{touch: true, pos: p})
		}
		if hold {
			for i := 0; i < 30; i++ {
				now = now.Add(16 * time.Millisecond)
				in.dispatch(frameInput{touch: true, pos: p})
			}
		}
		now = now.Add(16 * time.Millisecond)
		in.dispatch(frameInput{touch: true, pos: p, up: []MouseButton{MouseButtonLeft}})
		before := s.position()
		if interrupt {
			now = now.Add(16 * time.Millisecond)
			in.dispatch(frameInput{touch: true, pos: Pt(50, 50), down: []MouseButton{MouseButtonLeft}})
		}
		for i := 0; i < 180; i++ {
			now = now.Add(16 * time.Millisecond)
			in.dispatch(frameInput{})
		}
		if in.touchMotion.target != nil {
			t.Fatal("fling never settled")
		}
		return s.position() - before
	}
	slow, fast := run(200, false, false), run(1000, false, false)
	t.Logf("released travel: slow=%.1fpx fast=%.1fpx", slow, fast)
	if slow <= 0 || fast < slow*4 || fast < 250 {
		t.Fatalf("fling distance does not follow speed: slow=%v fast=%v", slow, fast)
	}
	if d := run(1000, true, false); d != 0 {
		t.Fatalf("holding before release still flings: %v", d)
	}
	if d := run(1000, false, true); d != 0 {
		t.Fatalf("new touch did not stop inertia: %v", d)
	}
}

func TestTouchMomentumFrameRateAndBounds(t *testing.T) {
	t.Parallel()
	run := func(step time.Duration, height float64) (float64, bool) {
		var in inputState
		s := Scroll(Box().Size(100, height))
		paintFrame(&in, s, Sz(100, 100))
		target := in.topmost(func(r *hitRegion) bool { return r.pointer == s })
		now := time.Unix(100, 0)
		in.touchMotion = touchMotion{target: keep(target), Tracker: kinetics.Tracker{Last: now, Velocity: Pt(0, -1000)}}
		for elapsed := time.Duration(0); elapsed < time.Second; elapsed += step {
			now = now.Add(step)
			in.stepTouchMomentum(now)
		}
		return s.position(), in.touchMotion.target != nil
	}
	a, _ := run(10*time.Millisecond, 10000)
	b, _ := run(20*time.Millisecond, 10000)
	if math.Abs(a-b) > 1e-8 {
		t.Fatalf("frame-dependent distance: %v vs %v", a, b)
	}
	if d, active := run(10*time.Millisecond, 150); d != 50 || active {
		t.Fatalf("did not stop at edge: %v active=%v", d, active)
	}
}

func TestTouchMomentumStopsOnPauseOrMissingTarget(t *testing.T) {
	t.Parallel()
	for _, missing := range []bool{false, true} {
		var in inputState
		s := Scroll(Box().Size(100, 1000))
		paintFrame(&in, s, Sz(100, 100))
		target := in.topmost(func(r *hitRegion) bool { return r.pointer == s })
		now := time.Unix(100, 0)
		in.touchMotion = touchMotion{target: keep(target), Tracker: kinetics.Tracker{Last: now, Velocity: Pt(0, -1000)}}
		dt := time.Second
		if missing {
			in.regions = nil
			dt = 16 * time.Millisecond
		}
		in.stepTouchMomentum(now.Add(dt))
		if s.position() != 0 || in.touchMotion.target != nil {
			t.Fatal("stale fling continued")
		}
	}
}

func TestParseChord(t *testing.T) {
	t.Parallel()
	cases := map[string]Chord{
		"cmd+s":        {Key: KeyS, Cmd: true},
		"Ctrl+Shift+Z": {Key: KeyZ, Mods: Mods{Ctrl: true, Shift: true}},
		"escape":       {Key: KeyEscape},
		"alt+enter":    {Key: KeyEnter, Mods: Mods{Alt: true}},
		"up":           {Key: KeyArrowUp},
		"f1":           {Key: KeyF1},
	}
	for in, want := range cases {
		got, err := ParseChord(in)
		if err != nil || got != want {
			t.Errorf("ParseChord(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ctrl+", "bogus", "s+ctrl"} {
		if _, err := ParseChord(bad); err == nil {
			t.Errorf("ParseChord(%q) accepted", bad)
		}
	}
	if s := MustChord("ctrl+shift+z").String(); s != "ctrl+shift+z" {
		t.Fatalf("String = %q", s)
	}
}

func TestKeyEventIs(t *testing.T) {
	t.Parallel()
	ev := KeyEvent{Kind: KeyPress, Key: KeyS, Mods: Mods{Ctrl: true}}
	if !ev.Is(MustChord("ctrl+s")) || ev.Is(MustChord("ctrl+shift+s")) || ev.Is(MustChord("s")) {
		t.Fatal("ctrl+s matched wrongly")
	}
	if (KeyEvent{Kind: KeyFocus, Key: KeyS, Mods: Mods{Ctrl: true}}).Is(MustChord("ctrl+s")) {
		t.Fatal("a focus event is not a press")
	}
}

// spaceEater is a focusable region that consumes Space.
type spaceEater struct {
	FocusWidget
	seen []KeyboardKey
}

func (e *spaceEater) HandleKey(ev KeyEvent) {
	if ev.Kind == KeyPress {
		e.seen = append(e.seen, ev.Key)
	}
}
func (e *spaceEater) ConsumesKey(ev KeyEvent) bool { return ev.Key == KeySpace }
func (e *spaceEater) Paint(dst *Canvas, r Rect)    { dst.HitKey(r, e) }

func TestShortcutsRunAroundTheFocusedWidget(t *testing.T) {
	t.Parallel()
	e := &spaceEater{FocusWidget: *Focus(Box().Size(20, 20))}
	p := NewProbe(e, Sz(50, 50))
	defer p.Close()
	runs := map[string]int{}
	p.Shortcut("space", func() { runs["space"]++ })
	p.Shortcut("s", func() { runs["s"]++ })
	p.Shortcut("ctrl+s", func() { runs["ctrl+s"]++ })
	p.Type(Mods{}, KeySpace) // nothing focused: the bare key runs
	if runs["space"] != 1 {
		t.Fatalf("space ran %d times with nothing focused, want 1", runs["space"])
	}
	p.Click(Pt(5, 5))
	p.Type(Mods{}, KeySpace, KeyS)
	if runs["space"] != 1 || runs["s"] != 1 || len(e.seen) != 2 {
		t.Fatalf("space %d s %d, widget saw %v; want the widget to keep Space and s to run after it", runs["space"], runs["s"], e.seen)
	}
	p.Type(Mods{Ctrl: true}, KeyS)
	if runs["ctrl+s"] != 1 || len(e.seen) != 2 {
		t.Fatalf("ctrl+s ran %d, widget saw %v; a modified chord goes before the widget", runs["ctrl+s"], e.seen)
	}
	p.Shortcut("space", func() { runs["excl"]++ }).Exclusive()
	p.Type(Mods{}, KeySpace)
	if runs["excl"] != 1 || len(e.seen) != 2 {
		t.Fatalf("exclusive ran %d, widget saw %v; want it to take the key", runs["excl"], e.seen)
	}
}

func TestPopupTrapsTabAndRestoresFocus(t *testing.T) {
	t.Parallel()
	focused := ""
	track := func(name string, size float64) *FocusWidget {
		return Focus(Box().Size(size, size)).OnFocus(func(on bool) {
			if on {
				focused = name
			} else if focused == name {
				focused = ""
			}
		})
	}
	a, b := track("a", 20), track("b", 20)
	c1, c2 := track("c1", 20), track("c2", 20)
	pop := Popup(a, Column(c1, c2))
	p := NewProbe(Column(pop, b), Sz(100, 200))
	defer p.Close()
	p.Click(Pt(10, 10))
	if focused != "a" {
		t.Fatalf("focused %q after clicking a", focused)
	}
	pop.Show()
	p.Frame()
	p.Type(Mods{}, KeyTab)
	if focused != "c2" {
		t.Fatalf("focused %q; the scope should take focus to c1 and Tab move it to c2", focused)
	}
	p.Type(Mods{}, KeyTab)
	if focused != "c1" {
		t.Fatalf("focused %q; Tab must wrap inside the popup, not reach b", focused)
	}
	p.Type(Mods{}, KeyEscape)
	if pop.IsOpen() {
		t.Fatal("Escape did not close the popup")
	}
	p.Move(Pt(90, 190)) // the next frame's dispatch restores focus
	if focused != "a" {
		t.Fatalf("focused %q after closing, want a", focused)
	}
}

func TestTabScrollsTheTargetIntoView(t *testing.T) {
	t.Parallel()
	items := Column(Focus(Box().Size(50, 50)), Focus(Box().Size(50, 50)), Focus(Box().Size(50, 50)))
	sc := Scroll(items)
	p := NewProbe(sc, Sz(50, 70))
	defer p.Close()
	p.Type(Mods{}, KeyTab, KeyTab)
	if sc.offset != 30 {
		t.Fatalf("offset = %v after tabbing to the second item, want 30 so it is fully shown", sc.offset)
	}
}

// RequestReveal scrolls its Rect into view, once, and leaves focus alone.
func TestRequestRevealScrollsWithoutFocusing(t *testing.T) {
	t.Parallel()
	reveal := -1
	rows := make([]Widget, 10)
	for i := range rows {
		rows[i] = &revealer{i: i, want: &reveal}
	}
	sc := Scroll(Column(rows...))
	p := NewProbe(sc, Sz(50, 70))
	defer p.Close()
	p.Frame()
	reveal = 5
	p.Frame()
	if sc.offset != 50*6-70 {
		t.Fatalf("offset = %v after revealing row 5, want %v", sc.offset, 50*6-70)
	}
	if p.in.focused != nil {
		t.Fatal("RequestReveal moved focus")
	}
	reveal = -1
	p.Scroll(Pt(10, 10), Pt(0, -40))
	p.Frame()
	if sc.offset == 50*6-70 {
		t.Fatal("the reveal held the scroll after it was asked once")
	}
}

type revealer struct {
	i    int
	want *int
}

func (w *revealer) Layout(Constraints, Env) Size { return Sz(50, 50) }
func (w *revealer) Paint(dst *Canvas, r Rect) {
	if *w.want == w.i {
		dst.RequestReveal(r)
	}
}

// PaintFocus tells the widget that painted the focused handler from one
// that only overlaps it: a badge floating over a focused editor, a parent
// around it.
func TestPaintFocusKnowsWhoPaintedTheFocus(t *testing.T) {
	t.Parallel()
	var inEditor, inBadge, inColumn bool
	tracked := func(w Widget, held *bool) Widget {
		return FromFuncs(
			func(c Constraints, env Env) Size { return w.Layout(c, env) },
			func(dst *Canvas, r Rect) { *held = dst.PaintFocus(w, r) },
		)
	}
	editor := Focus(Box().Size(200, 100))
	badge := Box().Size(40, 20)
	p := NewProbe(tracked(Stack(tracked(editor, &inEditor), Padding(tracked(badge, &inBadge), 10.5, 3.3)), &inColumn), Sz(200, 100))
	defer p.Close()
	p.Frame()
	if inEditor || inBadge || inColumn {
		t.Fatalf("before any focus: editor %v, badge %v, around both %v", inEditor, inBadge, inColumn)
	}
	p.Click(Pt(150, 80))
	p.Frame()
	if !inEditor || inBadge || !inColumn {
		t.Fatalf("with the editor focused: editor %v, badge over it %v, around both %v; want true, false, true", inEditor, inBadge, inColumn)
	}
}

// FocusRef focuses the first focusable widget in what it is attached to,
// from posted work, and leaves focus alone when a trap it is outside is open
// or the widget has nothing to focus.
func TestFocusRefFocusesTheAttachedWidget(t *testing.T) {
	t.Parallel()
	var first, second, empty FocusRef
	dialog := State(false)
	p := ProbeBuilder(func() Widget {
		return Column(
			first.Attach(Focus(Box().Size(20, 20))),
			second.Attach(Row(Box().Size(5, 5), Focus(Box().Size(20, 20)), Focus(Box().Size(20, 20)))),
			empty.Attach(Box().Size(20, 20)),
			If(dialog, func() Widget { return &shortcutKeeper{trap: true} }),
		)
	}, Sz(200, 200))
	defer p.Close()
	focusedOnly := func(what string, want *FocusRef) {
		t.Helper()
		p.Frame() // runs the posted Focus and paints the request
		p.Frame() // paints with the focus in place
		for name, ref := range map[string]*FocusRef{"first": &first, "second": &second, "empty": &empty} {
			if ref.Focused() != (ref == want) {
				t.Fatalf("%s: %s.Focused() = %v", what, name, ref.Focused())
			}
		}
	}
	p.Frame()
	focusedOnly("at start", nil)
	p.Post(second.Focus)
	focusedOnly("second", &second)
	p.Post(first.Focus)
	focusedOnly("first", &first)
	p.Post(empty.Focus)
	focusedOnly("nothing to focus", &first)
	dialog.Set(true)
	p.Frame()
	p.Post(second.Focus)
	p.Frame()
	p.Frame()
	if second.Focused() {
		t.Fatal("Focus reached a widget behind an open dialog")
	}
}

// A FocusRef whose widget was scrolled into view by Focus reports the focus
// once the scroll has painted, without waiting for input to refresh it.
func TestFocusRefFocusedAfterScrollingIntoView(t *testing.T) {
	t.Parallel()
	refs := make([]FocusRef, 30)
	rows := make([]Widget, len(refs))
	for i := range rows {
		rows[i] = refs[i].Attach(Focus(Box().Size(100, 20)))
	}
	p := NewProbe(Scroll(Column(rows...)), Sz(100, 100))
	defer p.Close()
	p.Frame()
	p.Post(refs[25].Focus)
	p.Frame() // focuses the row, out of view, and scrolls to it
	p.Frame() // paints it in view
	p.Frame() // paints with the focus where it now is
	if !refs[25].Focused() {
		t.Fatal("Focused() = false after the row scrolled into view")
	}
}

// A widget rebuilt where the focused one was is still told it has focus.
func TestRebuiltFocusedWidgetIsToldItHasFocus(t *testing.T) {
	t.Parallel()
	var got []string
	gen := State(0)
	var ref FocusRef
	p := NewProbe(ref.Attach(View(gen, func(n int) Widget { return &keyLog{n: n, got: &got} })), Sz(100, 100))
	defer p.Close()
	p.Frame()
	p.Post(ref.Focus)
	p.Frame()
	gen.Set(1)
	p.Type(Mods{}, KeyA)
	want := []string{"0 focus", "1 focus", "1 press"}
	if !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

type keyLog struct {
	n   int
	got *[]string
}

func (k *keyLog) Layout(Constraints, Env) Size { return Sz(20, 20) }
func (k *keyLog) Paint(dst *Canvas, r Rect)    { dst.HitKey(r, k) }
func (k *keyLog) HandleKey(e KeyEvent) {
	kind := map[KeyKind]string{KeyFocus: "focus", KeyPress: "press", KeyBlur: "blur"}[e.Kind]
	*k.got = append(*k.got, fmt.Sprint(k.n, " ", kind))
}

// Exercise real Interactive registrations: each control registers pointer,
// keyboard and cursor regions, and rebuilt controls adopt their prior state.
func BenchmarkInteractiveRegistration(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			c := Canvas{}
			old, next := make([]twice, n), make([]twice, n)
			for i := range old {
				old[i].SetKey(i)
				next[i].SetKey(i)
				old[i].Hit(&c, Rct(Pt(0, float64(i)*20), Sz(100, 20)), &old[i], CursorShapePointer)
			}
			c.prev, c.hits = c.hits, make([]hitRegion, 0, n)
			b.ReportAllocs()
			b.ResetTimer()
			for k := 0; k < b.N; k++ {
				c.hits = c.hits[:0]
				// A real frame starts here, so whatever adoption builds
				// once per frame is built again and shows up in the number.
				c.nextFrame()
				c.resetSemantics()
				for i := range next {
					next[i].Hit(&c, Rct(Pt(0, float64(i)*20), Sz(100, 20)), &next[i], CursorShapePointer)
				}
			}
		})
	}
}

func BenchmarkFocusedNode(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			c := Canvas{}
			for i := range n - 1 {
				c.Leaf(Rct(Pt(0, float64(i)), Sz(100, 1)), Node{Role: RoleText})
			}
			w := &twice{}
			w.Role = RoleTextField
			r := Rct(Pt(0, float64(n)), Sz(100, 20))
			c.Describe(r, w)
			focused := hitRegion{key: w, rect: r}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if focusedNode(&c, &focused) != n-1 {
					b.Fatal("focused node was lost")
				}
			}
		})
	}
}

// benchRow is a list row that takes the pointer and the keys.
type benchRow struct{}

func (*benchRow) HandlePointer(ev PointerEvent) bool { return true }
func (*benchRow) HandleKey(KeyEvent)                 {}

// benchInput is an inputState over n rows, the first focused and pressed,
// and observers watching the list, as a long list in an app at rest is.
func benchInput(n, observers int) (*inputState, Point) {
	rows := make([]benchRow, n)
	in := &inputState{}
	for i := range rows {
		r := Rct(Pt(0, float64(i)*20), Sz(100, 20))
		in.regions = append(in.regions, hitRegion{rect: r, full: r, pointer: &rows[i], key: &rows[i]})
	}
	for range observers {
		in.observers = append(in.observers, inputObserver{rect: Rct(Point{}, Sz(100, float64(n)*20)), notify: func() {}})
	}
	pos := Pt(50, 10) // over the first row, so the scans run the whole list
	in.dispatch(frameInput{pos: pos, down: []MouseButton{MouseButtonLeft}})
	return in, pos
}

// The input of a frame in which nothing happens: the pointer still over a
// long list, a row focused and pressed. Every frame of a large app pays it.
func BenchmarkDispatchStill(b *testing.B) {
	for _, n := range []int{100, 2000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			in, pos := benchInput(n, 0)
			b.ReportAllocs()
			for b.Loop() {
				in.dispatch(frameInput{pos: pos})
				in.takeTouched() // as the frame does
			}
		})
	}
}

// A key typed into the focused row of a list some regions observe.
func BenchmarkDispatchKey(b *testing.B) {
	in, pos := benchInput(2000, 8)
	keys := []KeyboardKey{KeyA}
	b.ReportAllocs()
	for b.Loop() {
		in.dispatch(frameInput{pos: pos, keys: keys})
		in.takeTouched()
	}
}

// What a paint hands input: the focused region found where it painted.
func BenchmarkRefreshFocused(b *testing.B) {
	in, _ := benchInput(2000, 0)
	b.ReportAllocs()
	for b.Loop() {
		in.refreshFocused()
	}
}

// A slot keeps the copy before the last valid, as a hover's exit reads
// the region it leaves after the next is kept, and lets go of handlers
// once it keeps nothing.
func TestRegionSlotKeepsThePreviousCopy(t *testing.T) {
	t.Parallel()
	var s regionSlot
	a, b := &benchRow{}, &benchRow{}
	regions := []hitRegion{{pointer: a, rect: Rct(Point{}, Sz(1, 1))}, {pointer: b}}
	first := s.keep(&regions[0])
	regions[0] = hitRegion{} // the regions buffer is reused by the next paint
	second := s.keep(&regions[1])
	if first == second || first.pointer != a || second.pointer != b {
		t.Fatal("keeping the second overwrote the first")
	}
	if s.keep(nil) != nil {
		t.Fatal("kept a region from nil")
	}
	if second.pointer != b {
		t.Fatal("keeping nothing cleared the copy just replaced")
	}
	s.keep(nil)
	if s.buf[0].pointer != nil || s.buf[1].pointer != nil {
		t.Fatal("a slot keeping nothing holds a handler")
	}
}
