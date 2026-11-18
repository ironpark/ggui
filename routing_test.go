package ggui

import (
	"fmt"
	"slices"
	"testing"
)

// The region tree: which region each region was painted inside.

// parents returns the parent of each region painted, as indices plus one,
// 0 for none.
func parents(in *inputState) []int32 {
	out := make([]int32, len(in.regions))
	for i := range in.regions {
		out[i] = in.regions[i].parent
	}
	return out
}

// wantParents fails t unless the regions in were painted inside want, as
// indices plus one.
func wantParents(t *testing.T, in *inputState, want ...int32) {
	t.Helper()
	got := parents(in)
	if len(got) != len(want) {
		t.Fatalf("%d regions with parents %v, want %d with %v", len(got), got, len(want), want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("parents = %v, want %v", got, want)
		}
	}
}

func TestARegionIsInsideTheRegionOfTheWidgetAroundIt(t *testing.T) {
	t.Parallel()
	inner := onPointer(sized(20, 20), takesNothing)
	outer := onPointer(padded(inner, 10), takesNothing)
	var in inputState
	paintFrame(&in, outer, Sz(40, 40))
	wantParents(t, &in, 0, 1)
	if got := in.parentOf(in.regionOf(inner, false)); got == nil || got.pointer != outer {
		t.Fatalf("parentOf(inner) = %+v, want the outer's region", got)
	}
	if got := in.parentOf(in.regionOf(outer, false)); got != nil {
		t.Fatalf("parentOf(outer) = %+v, want none", got)
	}
}

func TestWidgetsThatRegisterNothingDoNotBreakTheChain(t *testing.T) {
	t.Parallel()
	inner := onPointer(sized(20, 20), takesNothing)
	w := onPointer(column(sized(40, 5), padded(column(padded(inner, 2)), 3)), takesNothing)
	var in inputState
	paintFrame(&in, w, Sz(40, 40))
	wantParents(t, &in, 0, 1)
}

func TestRegionsOfSiblingsAreBesideEachOther(t *testing.T) {
	t.Parallel()
	w := stack(onPointer(sized(30, 30), takesNothing), onPointer(sized(20, 20), takesNothing))
	var in inputState
	paintFrame(&in, w, Sz(40, 40))
	wantParents(t, &in, 0, 0)
}

// after paints its child, then registers its own region over it, as a
// region that must hear what its child declined first does.
type after struct{ child Widget }

func (a *after) Layout(c Constraints, env Env) Size { return a.child.Layout(c, env) }
func (a *after) Paint(dst *Canvas, r Rect) {
	dst.Paint(a.child, r)
	dst.HitPointer(r, a)
}
func (a *after) HandlePointer(PointerEvent) bool { return false }

func TestARegionRegisteredAfterTheChildIsBesideIt(t *testing.T) {
	t.Parallel()
	inner := onPointer(sized(20, 20), takesNothing)
	w := onPointer(padded(&after{child: inner}, 5), takesNothing)
	var in inputState
	paintFrame(&in, w, Sz(40, 40))
	// The outer region, the child's inside it, and after's beside the
	// child's, inside the outer too.
	wantParents(t, &in, 0, 1, 1)
	if in.regions[2].pointer != PointerHandler(w.child.(*inset).child.(*after)) {
		t.Fatalf("the last region is %T, want after's", in.regions[2].pointer)
	}
}

func TestAWidgetsOwnRegionsNestByWhatCoversWhat(t *testing.T) {
	t.Parallel()
	slider := paintFunc(func(dst *Canvas, r Rect) {
		dst.HitPointer(Rct(Pt(0, 0), Sz(100, 20)), takesPointer{})  // the track
		dst.HitPointer(Rct(Pt(10, 0), Sz(20, 20)), takesPointer{})  // its knob
		dst.HitPointer(Rct(Pt(120, 0), Sz(20, 20)), takesPointer{}) // a button beside
	})
	var in inputState
	paintFrame(&in, slider, Sz(200, 40))
	wantParents(t, &in, 0, 1, 0)

	// A row of cells, each painting a control of its own after it: each
	// control is inside its cell, and no cell inside another.
	calendar := paintFunc(func(dst *Canvas, r Rect) {
		for i := range 3 {
			cell := Rct(Pt(float64(i)*30, 0), Sz(30, 30))
			dst.HitPointer(cell, takesPointer{})
			dst.Paint(onPointer(sized(10, 10), takesNothing), cell)
		}
	})
	paintFrame(&in, calendar, Sz(200, 40))
	wantParents(t, &in, 0, 1, 0, 3, 0, 5)
}

func TestMergedRegistrationsAreOneNode(t *testing.T) {
	t.Parallel()
	inner := onPointer(sized(20, 20), takesNothing)
	w := onPointer(onKey(inner, nil), takesNothing)
	var in inputState
	paintFrame(&in, w, Sz(20, 20))
	wantParents(t, &in, 0, 1)
	if in.regions[0].key == nil || in.regions[0].pointer != PointerHandler(w) {
		t.Fatalf("the first region is %+v, want the pointer and the key handler merged", in.regions[0])
	}
}

// overlaid registers a region and schedules an overlay painting over.
type overlaid struct{ over Widget }

func (o *overlaid) Layout(c Constraints, env Env) Size {
	o.over.Layout(Loose(c.Max()), env)
	return c.Constrain(Sz(100, 100))
}
func (o *overlaid) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, o)
	dst.Overlay(func(dst *Canvas) { dst.Paint(o.over, Rct(Pt(10, 10), Sz(50, 50))) })
}
func (o *overlaid) HandlePointer(PointerEvent) bool { return false }

func TestOverlayRegionsStartAChainOfTheirOwn(t *testing.T) {
	t.Parallel()
	popup := onPointer(padded(onPointer(sized(10, 10), takesNothing), 5), takesNothing)
	p := NewProbe(onPointer(&overlaid{over: popup}, takesNothing), Sz(200, 200))
	defer p.Close()
	p.Frame()
	// The tree's two, then the overlay's own chain: the popup's region is
	// inside nothing, its child inside the popup.
	wantParents(t, &p.input, 0, 1, 0, 3)
}

// clippedAway registers a region its clip cuts away, then paints child.
type clippedAway struct{ child Widget }

func (c *clippedAway) Layout(cs Constraints, env Env) Size { return c.child.Layout(cs, env) }
func (c *clippedAway) Paint(dst *Canvas, r Rect) {
	in := dst.Clip(r)
	in.HitPointer(Rct(r.Origin.Add(Pt(500, 500)), Sz(10, 10)), takesPointer{})
	in.Paint(c.child, r)
}

func TestARegionClippedAwayIsNoParent(t *testing.T) {
	t.Parallel()
	inner := onPointer(sized(20, 20), takesNothing)
	w := onPointer(padded(&clippedAway{child: inner}, 5), takesNothing)
	var in inputState
	paintFrame(&in, w, Sz(40, 40))
	wantParents(t, &in, 0, 1)
	if in.regions[1].pointer != PointerHandler(inner) {
		t.Fatalf("the second region is %T, want the inner one", in.regions[1].pointer)
	}
}

// TestARegionIsRegisteredAfterTheRegionItIsInside holds the invariant the
// routing relies on, over a tree with every way of registering: merges,
// clips, a scroll, several regions from one Paint, regions after a child,
// and overlays scheduled from inside it.
func TestARegionIsRegisteredAfterTheRegionItIsInside(t *testing.T) {
	t.Parallel()
	cell := func() Widget { return onKey(onPointer(sized(30, 20), takesNothing), nil) }
	list := &scroller{child: column(cell(), padded(cell(), 2), &after{child: cell()}, cell())}
	popup := onPointer(column(cell(), onPointer(&overlaid{over: cell()}, takesNothing)), takesNothing)
	w := column(
		onPointer(row(&overlaid{over: popup}, &clippedAway{child: cell()}), takesNothing),
		&held{child: list},
	)
	p := NewProbe(w, Sz(300, 300))
	defer p.Close()
	p.Frame()
	if len(p.input.regions) < 10 {
		t.Fatalf("only %d regions painted", len(p.input.regions))
	}
	nested := 0
	for i, r := range p.input.regions {
		if r.parent < 0 || int(r.parent) > i {
			t.Fatalf("region %d is inside %d, which is not before it", i, r.parent-1)
		}
		if r.parent > 0 {
			nested++
		}
	}
	if nested == 0 {
		t.Fatal("no region was inside another")
	}
}

// TestRegisteringRegionsAllocatesNothingMore: the tree is made with the
// list, and a paint of the same tree registers it in the buffers of the
// last.
func TestRegisteringRegionsAllocatesNothingMore(t *testing.T) {
	inner := onPointer(sized(20, 20), takesNothing)
	w := onPointer(column(padded(onKey(inner, nil), 2), onPointer(sized(10, 10), takesNothing)), takesNothing)
	w.Layout(Tight(Sz(40, 40)), Env{})
	c := &Canvas{}
	f := c.fs()
	paint := func() {
		f.hits = f.hits[:0]
		f.cur = regionCursor{}
		f.scopes = f.scopes[:0]
		c.Paint(w, Rct(Pt(0, 0), Sz(40, 40)))
	}
	paint()
	if allocs := testing.AllocsPerRun(100, paint); allocs != 0 {
		t.Fatalf("a paint of the same tree allocates %v times", allocs)
	}
	if len(f.hits) != 3 || f.hits[1].parent != 1 || f.hits[2].parent != 1 {
		t.Fatalf("parents %v", []int32{f.hits[0].parent, f.hits[1].parent, f.hits[2].parent})
	}
}

// Hover along the tree.

// hoverLog returns a handler that claims every move, or none when claims
// is false, and logs its enters and exits as name+ and name-, with a * for
// an enter with Inner set.
func hoverLog(log *[]string, name string, claims bool) func(PointerEvent) bool {
	return func(ev PointerEvent) bool {
		switch ev.Kind {
		case PointerEnter:
			if ev.Inner {
				*log = append(*log, name+"+*")
			} else {
				*log = append(*log, name+"+")
			}
		case PointerExit:
			*log = append(*log, name+"-")
		case PointerMove:
			return claims
		}
		return true
	}
}

// wantLog fails t unless log is want, and empties it.
func wantLog(t *testing.T, log *[]string, step string, want ...string) {
	t.Helper()
	if !slices.Equal(*log, want) {
		t.Fatalf("%s: heard %q, want %q", step, *log, want)
	}
	*log = (*log)[:0]
}

func TestHoverReachesTheRegionsAroundTheHotOne(t *testing.T) {
	t.Parallel()
	var log []string
	w := onPointer(padded(onPointer(sized(20, 20), hoverLog(&log, "inner", true)), 10), hoverLog(&log, "outer", true))
	var in inputState
	paintFrame(&in, w, Sz(40, 40))
	in.dispatch(frameInput{pos: Pt(100, 100)})
	wantLog(t, &log, "outside")
	in.dispatch(frameInput{pos: Pt(20, 20)})
	wantLog(t, &log, "straight onto the inner", "outer+*", "inner+")
	in.dispatch(frameInput{pos: Pt(22, 20)})
	wantLog(t, &log, "over the inner")
	in.dispatch(frameInput{pos: Pt(5, 5)})
	wantLog(t, &log, "onto the outer's padding", "inner-")
	in.dispatch(frameInput{pos: Pt(20, 20)})
	wantLog(t, &log, "back onto the inner", "inner+")
	in.dispatch(frameInput{pos: Pt(100, 100)})
	wantLog(t, &log, "out", "inner-", "outer-")
	in.dispatch(frameInput{pos: Pt(5, 5)})
	wantLog(t, &log, "onto the outer", "outer+")
}

func TestARegionThatPassesTheMoveIsNotHovered(t *testing.T) {
	t.Parallel()
	var log []string
	inner := onPointer(sized(20, 20), hoverLog(&log, "inner", true))
	passes := onPointer(padded(inner, 5), hoverLog(&log, "passes", false))
	w := onPointer(padded(passes, 5), hoverLog(&log, "around", true))
	var in inputState
	paintFrame(&in, w, Sz(40, 40))
	in.dispatch(frameInput{pos: Pt(20, 20)})
	wantLog(t, &log, "onto the inner", "around+*", "inner+")
	in.dispatch(frameInput{pos: Pt(7, 7)})
	wantLog(t, &log, "onto the region that passes", "inner-")
	in.dispatch(frameInput{pos: Pt(100, 100)})
	wantLog(t, &log, "out", "around-")
}

func TestAClaimedMoveStillHidesWhatIsBeneathAndNotAround(t *testing.T) {
	t.Parallel()
	var log []string
	w := stack(onPointer(sized(40, 40), hoverLog(&log, "lower", true)), onPointer(sized(20, 20), hoverLog(&log, "upper", true)))
	var in inputState
	paintFrame(&in, w, Sz(40, 40))
	in.dispatch(frameInput{pos: Pt(10, 10)})
	wantLog(t, &log, "over both", "upper+")
	in.dispatch(frameInput{pos: Pt(30, 30)})
	wantLog(t, &log, "over the lower alone", "upper-", "lower+")
}

func TestAMoveSaysItIsAnInnerRegions(t *testing.T) {
	t.Parallel()
	var outer, inner []PointerEvent
	record := func(log *[]PointerEvent) func(PointerEvent) bool {
		return func(ev PointerEvent) bool {
			*log = append(*log, ev)
			return true
		}
	}
	w := onPointer(padded(onPointer(sized(20, 20), record(&inner)), 10), record(&outer))
	var in inputState
	paintFrame(&in, w, Sz(40, 40))
	in.dispatch(frameInput{pos: Pt(20, 20)})
	in.dispatch(frameInput{pos: Pt(5, 5)})
	kinds := func(evs []PointerEvent) (out []string) {
		for _, ev := range evs {
			out = append(out, fmt.Sprint(ev.Kind, ev.Inner))
		}
		return out
	}
	want := []string{fmt.Sprint(PointerMove, true), fmt.Sprint(PointerEnter, true), fmt.Sprint(PointerMove, false)}
	if got := kinds(outer); !slices.Equal(got, want) {
		t.Fatalf("the outer heard %v, want %v", got, want)
	}
	want = []string{fmt.Sprint(PointerMove, false), fmt.Sprint(PointerEnter, false), fmt.Sprint(PointerExit, false)}
	if got := kinds(inner); !slices.Equal(got, want) {
		t.Fatalf("the inner heard %v, want %v", got, want)
	}
}

// overflowing paints its child at a Rect of its own, past its own edge.
type overflowing struct {
	child Widget
	at    Rect
}

func (o *overflowing) Layout(c Constraints, env Env) Size {
	o.child.Layout(Tight(o.at.Size), env)
	return c.Constrain(Sz(20, 20))
}
func (o *overflowing) Paint(dst *Canvas, r Rect) { dst.Paint(o.child, o.at) }

func TestAChildPastItsParentsRectKeepsTheParentHovered(t *testing.T) {
	t.Parallel()
	var log []string
	menu := onPointer(sized(30, 30), hoverLog(&log, "child", true))
	w := onPointer(&overflowing{child: menu, at: Rct(Pt(50, 0), Sz(30, 30))}, hoverLog(&log, "parent", true))
	var in inputState
	paintFrame(&in, column(w), Sz(100, 100))
	in.dispatch(frameInput{pos: Pt(10, 10)})
	wantLog(t, &log, "onto the parent", "parent+")
	in.dispatch(frameInput{pos: Pt(60, 10)})
	wantLog(t, &log, "onto the child past its edge", "child+")
	in.dispatch(frameInput{pos: Pt(60, 90)})
	wantLog(t, &log, "out", "child-", "parent-")
}

// hoverAround is a control around a child that tracks its hover, counts
// the enters and exits it heard, and carries its hover across a rebuild.
type hoverAround struct {
	id            string
	child         Widget
	hovered       bool
	enters, exits int
}

func (h *hoverAround) Layout(c Constraints, env Env) Size {
	if h.child != nil {
		return h.child.Layout(c, env)
	}
	return c.Constrain(Sz(20, 20))
}
func (h *hoverAround) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, h)
	if h.child != nil {
		dst.Paint(h.child, r)
	}
}
func (h *hoverAround) HitID() any { return h.id }
func (h *hoverAround) HandlePointer(ev PointerEvent) bool {
	switch ev.Kind {
	case PointerEnter:
		h.hovered = true
		h.enters++
	case PointerMove:
		h.hovered = true
	case PointerExit:
		h.hovered = false
		h.exits++
	}
	return true
}
func (h *hoverAround) Adopt(prev any) {
	if p, ok := prev.(*hoverAround); ok {
		h.hovered = p.hovered
	}
}

func TestTheHoverPathSurvivesARebuild(t *testing.T) {
	t.Parallel()
	gen := State(0)
	var built [][2]*hoverAround
	p := ProbeBuilder(func() Widget {
		return Reactive(func() Widget {
			_ = gen.Get()
			inner := &hoverAround{id: "inner"}
			outer := &hoverAround{id: "outer", child: padded(inner, 10)}
			built = append(built, [2]*hoverAround{outer, inner})
			return around(60, 60, outer)
		})
	}, Sz(60, 60))
	defer p.Close()
	p.Move(Pt(20, 20))
	first := built[len(built)-1]
	if !first[0].hovered || !first[1].hovered {
		t.Fatal("the pointer over the inner did not hover both")
	}
	gen.Set(1) // rebuilt under a still pointer
	p.Frame()
	p.Move(Pt(20, 20))
	p.Frame()
	now := built[len(built)-1]
	if now == first {
		t.Fatal("not rebuilt")
	}
	for i, h := range now {
		if !h.hovered || h.enters != 1 || h.exits != 0 {
			t.Fatalf("rebuilt %d: hovered %v, %d enters, %d exits; want hovered, 1, 0", i, h.hovered, h.enters, h.exits)
		}
		if first[i].exits != 0 {
			t.Fatalf("the replaced %d heard an exit", i)
		}
	}
	p.Move(Pt(100, 100))
	for i, h := range now {
		if h.hovered || h.exits != 1 {
			t.Fatalf("rebuilt %d after the pointer left: hovered %v, %d exits", i, h.hovered, h.exits)
		}
	}
}

// A path that did not change costs the frame no allocation.
func TestHoverAllocatesNothing(t *testing.T) {
	var log []string
	deep := onPointer(sized(10, 10), hoverLog(&log, "a", true))
	for range 6 {
		deep = onPointer(padded(deep, 2), hoverLog(&log, "b", true))
	}
	var in inputState
	paintFrame(&in, deep, Sz(40, 40))
	in.dispatch(frameInput{pos: Pt(15, 15)})
	if len(in.hover.at) != 7 {
		t.Fatalf("the path holds %d regions, want 7", len(in.hover.at))
	}
	allocs := testing.AllocsPerRun(100, func() {
		in.dispatch(frameInput{pos: Pt(15, 15)})
		in.takeTouched()
	})
	if allocs != 0 {
		t.Fatalf("a frame of a pointer at rest allocates %v times", allocs)
	}
}

// What goes on inside a region.

// watcher registers a HitWithin region around its child, and a pointer
// region at the same Rect when pointer is set. It logs what it hears as
// name:down, name:scroll, name:focus+ and name:focus-, and takes a press
// or a scroll when take says so.
type watcher struct {
	child   Widget
	name    string
	log     *[]string
	take    func(PointerEvent) bool
	pointer func(PointerEvent) bool
}

func (w *watcher) Layout(c Constraints, env Env) Size { return w.child.Layout(c, env) }
func (w *watcher) Paint(dst *Canvas, r Rect) {
	dst.HitWithin(r, w)
	if w.pointer != nil {
		dst.HitPointer(r, w)
	}
	dst.Paint(w.child, r)
}
func (w *watcher) HandlePointer(ev PointerEvent) bool { return w.pointer(ev) }
func (w *watcher) HandlePointerWithin(ev PointerEvent) bool {
	switch ev.Kind {
	case PointerDown:
		*w.log = append(*w.log, w.name+":down")
	case PointerScroll:
		*w.log = append(*w.log, w.name+":scroll")
	default:
		*w.log = append(*w.log, w.name+":"+fmt.Sprint(ev.Kind))
	}
	return w.take != nil && w.take(ev)
}
func (w *watcher) HandleFocusWithin(within bool) {
	*w.log = append(*w.log, w.name+":focus"+map[bool]string{true: "+", false: "-"}[within])
}

// pressLog returns a handler that logs a press, release, tap or wheel as
// name:what, and takes the presses, releases and taps.
func pressLog(log *[]string, name string) func(PointerEvent) bool {
	return func(ev PointerEvent) bool {
		switch ev.Kind {
		case PointerDown:
			*log = append(*log, name+":down")
		case PointerUp:
			*log = append(*log, name+":up")
		case PointerTap:
			*log = append(*log, name+":tap")
		case PointerDrag:
			*log = append(*log, name+":drag")
		case PointerScroll:
			*log = append(*log, name+":scroll")
			return false
		default:
			return false
		}
		return true
	}
}

func pressAt(in *inputState, p Point) {
	in.dispatch(frameInput{pos: p, down: []MouseButton{MouseButtonLeft}})
}

func releaseAt(in *inputState, p Point) {
	in.dispatch(frameInput{pos: p, up: []MouseButton{MouseButtonLeft}})
}

func TestWithinHearsAPressBeforeWhatTakesIt(t *testing.T) {
	t.Parallel()
	var log []string
	w := &watcher{name: "pane", log: &log, child: padded(onPointer(sized(20, 20), pressLog(&log, "button")), 10)}
	var in inputState
	paintFrame(&in, column(w), Sz(100, 100))
	clickAt(&in, Pt(20, 20))
	wantLog(t, &log, "a click on the button", "pane:down", "button:down", "button:up", "button:tap")
}

func TestWithinHearsAPressNothingInsideTakes(t *testing.T) {
	t.Parallel()
	var log []string
	w := &watcher{name: "pane", log: &log, child: padded(onPointer(sized(20, 20), pressLog(&log, "button")), 10)}
	var in inputState
	paintFrame(&in, column(w), Sz(100, 100))
	clickAt(&in, Pt(5, 5))
	wantLog(t, &log, "a click on the pane's own area", "pane:down")
	if in.pressed != nil {
		t.Fatal("a region with no pointer handler holds the press")
	}
}

func TestWithinIsNotToldOfAPressBesideItOrOverIt(t *testing.T) {
	t.Parallel()
	var log []string
	w := &watcher{name: "pane", log: &log, child: sized(40, 40)}
	beside := onPointer(sized(40, 40), pressLog(&log, "beside"))
	var in inputState
	paintFrame(&in, row(w, beside), Sz(100, 100))
	clickAt(&in, Pt(60, 20))
	wantLog(t, &log, "a click beside the pane", "beside:down", "beside:up", "beside:tap")

	// A popup the pane opens is painted over it by an overlay, inside no
	// region: a press on it is the popup's alone.
	over := onPointer(sized(30, 30), pressLog(&log, "popup"))
	opener := &overlaid{over: over}
	w = &watcher{name: "pane", log: &log, child: opener}
	p := NewProbe(column(w), Sz(200, 200))
	defer p.Close()
	p.Click(Pt(20, 20))
	wantLog(t, &log, "a click on the popup over the pane", "popup:down", "popup:up", "popup:tap")
	p.Click(Pt(80, 80))
	wantLog(t, &log, "a click on the pane beside the popup", "pane:down")
}

func TestWithinHandlersHearOutermostFirstAndOnce(t *testing.T) {
	t.Parallel()
	var log []string
	// Two regions inside the inner watcher pass the press on; each handler
	// still hears it once.
	inside := onPointer(padded(onPointer(sized(10, 10), takesNothing), 5), takesNothing)
	inner := &watcher{name: "inner", log: &log, child: padded(inside, 5)}
	outer := &watcher{name: "outer", log: &log, child: padded(inner, 5)}
	var in inputState
	paintFrame(&in, column(outer), Sz(100, 100))
	pressAt(&in, Pt(17, 17))
	wantLog(t, &log, "a press falling through", "outer:down", "inner:down")
	releaseAt(&in, Pt(17, 17))
	wantLog(t, &log, "its release")
}

func TestAWithinHandlerThatTakesAPressKeepsIt(t *testing.T) {
	t.Parallel()
	var log []string
	takeDown := func(ev PointerEvent) bool { return ev.Kind == PointerDown }
	w := &watcher{name: "row", log: &log, take: takeDown, pointer: pressLog(&log, "row"),
		child: padded(onPointer(sized(20, 20), pressLog(&log, "button")), 10)}
	var in inputState
	paintFrame(&in, column(w), Sz(100, 100))
	pressAt(&in, Pt(20, 20))
	in.dispatch(frameInput{pos: Pt(25, 25)})
	releaseAt(&in, Pt(20, 20))
	wantLog(t, &log, "a press the row takes", "row:down", "row:drag", "row:up", "row:tap")
}

func TestWithinHearsTheWheelAndNotTheMove(t *testing.T) {
	t.Parallel()
	var log []string
	inner := onPointer(sized(20, 20), func(ev PointerEvent) bool {
		if ev.Kind == PointerScroll {
			log = append(log, "inner:scroll")
			return true
		}
		return ev.Kind == PointerMove
	})
	w := &watcher{name: "pane", log: &log, child: padded(inner, 10)}
	var in inputState
	paintFrame(&in, column(w), Sz(100, 100))
	in.dispatch(frameInput{pos: Pt(20, 20)})
	in.dispatch(frameInput{pos: Pt(21, 20)})
	wantLog(t, &log, "moves")
	in.dispatch(frameInput{pos: Pt(20, 20), wheel: Pt(0, -1)})
	wantLog(t, &log, "a wheel", "pane:scroll", "inner:scroll")
	// One that takes it takes it from the view inside.
	w.take = func(ev PointerEvent) bool { return ev.Kind == PointerScroll }
	in.dispatch(frameInput{pos: Pt(20, 20), wheel: Pt(0, -1)})
	wantLog(t, &log, "a wheel taken", "pane:scroll")
}

func TestFocusWithinFollowsTheChain(t *testing.T) {
	t.Parallel()
	var log []string
	field := func() Widget { return onKey(sized(20, 20), nil) }
	inner := &watcher{name: "inner", log: &log, child: column(field(), field())}
	outer := &watcher{name: "outer", log: &log, child: column(field(), inner)}
	tab := func(in *inputState) { in.dispatch(frameInput{keys: []KeyboardKey{KeyTab}}) }
	var in inputState
	paintFrame(&in, column(outer, field()), Sz(100, 200))
	tab(&in)
	wantLog(t, &log, "Tab onto the outer's field", "outer:focus+")
	tab(&in)
	wantLog(t, &log, "Tab into the inner", "inner:focus+")
	tab(&in)
	wantLog(t, &log, "Tab to the inner's other field")
	tab(&in)
	wantLog(t, &log, "Tab out of both", "inner:focus-", "outer:focus-")
	tab(&in)
	wantLog(t, &log, "Tab round into both", "outer:focus+")
	clickAt(&in, Pt(10, 30))
	wantLog(t, &log, "a click into the inner", "outer:down", "inner:down", "inner:focus+")

	// The focused region removed: focus is in nothing.
	paintFrame(&in, column(field()), Sz(100, 200))
	in.dispatch(frameInput{})
	wantLog(t, &log, "the focused field gone", "inner:focus-", "outer:focus-")

	// A rebuild hands the handler on: the new one is told focus is in it.
	var built *watcher
	build := func() Widget {
		built = &watcher{name: "rebuilt", log: &log, child: column(onKey(sized(20, 20), nil))}
		return column(built)
	}
	paintFrame(&in, build(), Sz(100, 200))
	clickAt(&in, Pt(10, 10))
	wantLog(t, &log, "a click into it", "rebuilt:down", "rebuilt:focus+")
	paintFrame(&in, build(), Sz(100, 200))
	in.dispatch(frameInput{})
	wantLog(t, &log, "rebuilt with focus inside", "rebuilt:focus+")
	clickAt(&in, Pt(80, 150))
	wantLog(t, &log, "a click away", "rebuilt:focus-")
	_ = built
}

func TestPerformFocusTellsFocusWithin(t *testing.T) {
	t.Parallel()
	var log []string
	bell := &axBell{}
	bell.Role = RoleButton
	bell.SetName("ring")
	p := NewProbe(column(&watcher{name: "pane", log: &log, child: bell}), Sz(100, 100))
	defer p.Close()
	node, ok := p.Semantics().Find(RoleButton, "ring")
	if !ok {
		t.Fatal("no button")
	}
	p.Perform(node.ID, Action{Kind: ActionFocus})
	wantLog(t, &log, "focus performed", "pane:focus+")
}

// counter is a within handler that counts what it hears.
type counter struct {
	child        Widget
	downs, focus int
}

func (c *counter) Layout(cs Constraints, env Env) Size { return c.child.Layout(cs, env) }
func (c *counter) Paint(dst *Canvas, r Rect) {
	dst.HitWithin(r, c)
	dst.Paint(c.child, r)
}
func (c *counter) HandlePointerWithin(ev PointerEvent) bool {
	if ev.Kind == PointerDown {
		c.downs++
	}
	return false
}
func (c *counter) HandleFocusWithin(bool) { c.focus++ }

// Within handlers add no allocation to a frame: what a press costs, the
// copy of the region that holds it, it costs without them too.
func TestWithinAllocatesNothing(t *testing.T) {
	down, up := []MouseButton{MouseButtonLeft}, []MouseButton{MouseButtonLeft}
	keys := []KeyboardKey{KeyA}
	allocs := func(w Widget) float64 {
		var in inputState
		paintFrame(&in, w, Sz(100, 100))
		frame := func() {
			in.dispatch(frameInput{pos: Pt(12, 12), down: down})
			in.dispatch(frameInput{pos: Pt(12, 12), up: up})
			in.dispatch(frameInput{pos: Pt(12, 12), wheel: Pt(0, 1)})
			in.dispatch(frameInput{pos: Pt(12, 12), keys: keys})
			in.dispatch(frameInput{pos: Pt(5, 35), down: down})
			in.dispatch(frameInput{pos: Pt(5, 35), up: up})
			in.takeTouched()
		}
		frame()
		return testing.AllocsPerRun(100, frame)
	}
	control := func() Widget { return onKey(tapped(sized(10, 10), func() {}), nil) }
	inner := &counter{child: padded(control(), 5)}
	outer := &counter{child: padded(inner, 5)}
	with := allocs(column(outer, onKey(sized(10, 10), nil)))
	if outer.downs < 1 || inner.downs != outer.downs || outer.focus < 2 || inner.focus != outer.focus {
		t.Fatalf("downs %d %d, focus %d %d", outer.downs, inner.downs, outer.focus, inner.focus)
	}
	without := allocs(column(padded(padded(control(), 5), 5), onKey(sized(10, 10), nil)))
	if with != without {
		t.Fatalf("a frame allocates %v times with within handlers, %v without", with, without)
	}
}

// Keys within.

// keyWatcher is a HitWithin region around child that logs the key presses
// it is offered as name:key and takes those take holds.
type keyWatcher struct {
	child Widget
	name  string
	log   *[]string
	take  map[KeyboardKey]bool
}

func (w *keyWatcher) Layout(c Constraints, env Env) Size { return w.child.Layout(c, env) }
func (w *keyWatcher) Paint(dst *Canvas, r Rect) {
	dst.HitWithin(r, w)
	dst.Paint(w.child, r)
}
func (w *keyWatcher) HandleKeyWithin(ev KeyEvent) bool {
	*w.log = append(*w.log, w.name+":"+pressName(ev))
	return w.take[ev.Key]
}

// trapped paints its child in a focus trap whose Escape it counts.
type trapped struct {
	child   Widget
	escapes int
}

func (t *trapped) Layout(c Constraints, env Env) Size { return t.child.Layout(c, env) }
func (t *trapped) Paint(dst *Canvas, r Rect) {
	dst.FocusTrap(t, func() { t.escapes++ }, func(dst *Canvas) { dst.Paint(t.child, r) })
}

func TestBareKeysWithinFollowTheFocusedWidget(t *testing.T) {
	t.Parallel()
	var log []string
	field := &spaceEater{child: sized(20, 20)}
	inner := &keyWatcher{name: "inner", log: &log, child: field, take: map[KeyboardKey]bool{KeyS: true, KeyEscape: true}}
	outer := &keyWatcher{name: "outer", log: &log, child: padded(inner, 5), take: map[KeyboardKey]bool{KeyX: true}}
	trap := &trapped{child: outer}
	p := NewProbe(column(trap), Sz(100, 100))
	defer p.Close()
	runs := map[string]int{}
	for _, k := range []string{"s", "x", "y", "space"} {
		p.Shortcut(k, func() { runs[k]++ })
	}
	p.Click(Pt(10, 10))
	p.Type(Mods{}, KeySpace)
	wantLog(t, &log, "a key the field consumes")
	p.Type(Mods{}, KeyS)
	wantLog(t, &log, "a key the nearest takes", "inner:S")
	p.Type(Mods{}, KeyX)
	wantLog(t, &log, "a key the outer takes", "inner:X", "outer:X")
	p.Type(Mods{}, KeyY)
	wantLog(t, &log, "a key nobody takes", "inner:Y", "outer:Y")
	if want := map[string]int{"y": 1}; fmt.Sprint(runs) != fmt.Sprint(want) {
		t.Fatalf("shortcuts ran %v, want %v: only those of the keys nothing within took", runs, want)
	}
	if len(field.seen) != 4 {
		t.Fatalf("the field saw %v, want every key before the within handlers", field.seen)
	}
	p.Type(Mods{}, KeyEscape)
	wantLog(t, &log, "Escape", "inner:Escape")
	if trap.escapes != 0 {
		t.Fatal("an Escape taken within reached the trap")
	}
	inner.take[KeyEscape] = false
	p.Type(Mods{}, KeyEscape)
	wantLog(t, &log, "Escape left", "inner:Escape", "outer:Escape")
	if trap.escapes != 1 {
		t.Fatal("an Escape nothing within took did not reach the trap")
	}
}

// keyedLog is a focusable that logs the presses it hears.
type keyedLog struct {
	focusable
	log *[]string
}

func (k *keyedLog) HandleKey(ev KeyEvent) {
	if ev.Kind == KeyPress {
		*k.log = append(*k.log, "field:"+pressName(ev))
	}
}

func (k *keyedLog) Paint(dst *Canvas, r Rect) { dst.HitKey(r, k) }

// claimingLog is a keyedLog that claims every chord with Ctrl.
type claimingLog struct{ keyedLog }

func (k *claimingLog) Paint(dst *Canvas, r Rect) { dst.HitKey(r, k) }

func (k *claimingLog) ClaimsChord(ev KeyEvent) bool { return ev.Mods.Ctrl }

func TestChordsWithinPrecedeTheFocusedWidgetAndTheWindow(t *testing.T) {
	t.Parallel()
	var log []string
	field := &keyedLog{child: sized(20, 20), log: &log}
	pane := &keyWatcher{name: "pane", log: &log, child: field, take: map[KeyboardKey]bool{KeyS: true}}
	term := &claimingLog{keyedLog{child: sized(20, 20), log: &log}}
	p := NewProbe(column(pane, &keyWatcher{name: "around", log: &log, child: term, take: map[KeyboardKey]bool{KeyS: true}}), Sz(100, 100))
	defer p.Close()
	runs := 0
	p.Shortcut("ctrl+s", func() { runs++ })
	p.Shortcut("ctrl+d", func() { runs++ })
	p.Click(Pt(10, 10))
	ctrl := Mods{Ctrl: true}
	p.Type(ctrl, KeyS)
	wantLog(t, &log, "a chord taken within", "pane:Ctrl+S")
	if runs != 0 {
		t.Fatal("the window's shortcut ran for a chord taken within")
	}
	p.Type(ctrl, KeyD)
	wantLog(t, &log, "a chord passed on to the window's shortcut", "pane:Ctrl+D")
	if runs != 1 {
		t.Fatal("the window's shortcut did not run for a chord passed on")
	}
	p.Type(ctrl, KeyE)
	wantLog(t, &log, "a chord passed on to the field", "pane:Ctrl+E", "field:Ctrl+E")
	p.Click(Pt(10, 30))
	p.Type(ctrl, KeyS)
	wantLog(t, &log, "a chord the focused widget claims", "field:Ctrl+S")
}

func TestTabIsNotOfferedWithin(t *testing.T) {
	t.Parallel()
	var log []string
	field := func() Widget { return &keyedLog{child: sized(20, 20), log: &log} }
	pane := &keyWatcher{name: "pane", log: &log, child: column(field(), field())}
	p := NewProbe(column(pane), Sz(100, 100))
	defer p.Close()
	p.Click(Pt(10, 10))
	p.Type(Mods{}, KeyTab)
	p.Type(Mods{Shift: true}, KeyTab)
	p.Type(Mods{Ctrl: true}, KeyTab)
	wantLog(t, &log, "Tab")
}

func TestKeysWithinNeedFocusInside(t *testing.T) {
	t.Parallel()
	var log []string
	pane := &keyWatcher{name: "pane", log: &log, child: sized(40, 40)}
	beside := &keyedLog{child: sized(20, 20), log: &log}
	p := NewProbe(column(pane, beside), Sz(100, 100))
	defer p.Close()
	p.Type(Mods{}, KeyA)
	p.Type(Mods{Ctrl: true}, KeyA)
	wantLog(t, &log, "nothing focused")
	p.Click(Pt(10, 10)) // the pane, which takes no focus
	p.Type(Mods{}, KeyA)
	wantLog(t, &log, "a press on the pane")
	p.Click(Pt(10, 50))
	p.Type(Mods{}, KeyA)
	p.Type(Mods{Ctrl: true}, KeyA)
	wantLog(t, &log, "focus beside", "field:A", "field:Ctrl+A")
}

// pressName writes a press as Ctrl+S, or S with no modifier.
func pressName(ev KeyEvent) string {
	if ev.Mods.Ctrl {
		return "Ctrl+" + ev.Key.String()
	}
	return ev.Key.String()
}

// The scroll latch.

// gestureFrame is a frame of a trackpad's gesture: pixels scrolled at pos,
// in phase.
func gestureFrame(pos, px Point, phase ScrollPhase) frameInput {
	return frameInput{pos: pos, wheelPx: px, phase: phase}
}

// nestedScrollers is an outer scroller 100 by 200 over a column of an inner
// scroller 100 by 100, of 200 of content, and 300 more below it.
func nestedScrollers() (outer, inner *scroller) {
	inner = &scroller{child: sized(100, 200)}
	outer = &scroller{child: column(around(100, 100, inner), sized(100, 300))}
	return outer, inner
}

func TestAGestureStaysWithTheViewThatTookIt(t *testing.T) {
	t.Parallel()
	outer, inner := nestedScrollers()
	var in inputState
	paintFrame(&in, outer, Sz(100, 200))
	at := Pt(50, 50)
	in.dispatch(gestureFrame(at, Pt(0, -60), ScrollPhaseBegan))
	in.dispatch(gestureFrame(at, Pt(0, -60), ScrollPhaseChanged))
	if inner.offset != 100 || outer.offset != 0 {
		t.Fatalf("inner at %v, outer at %v; want the inner at its end", inner.offset, outer.offset)
	}
	in.dispatch(gestureFrame(at, Pt(0, -60), ScrollPhaseChanged))
	in.dispatch(gestureFrame(at.Add(Pt(0, 120)), Pt(0, -60), ScrollPhaseChanged)) // the pointer off the inner
	in.dispatch(gestureFrame(at, Point{}, ScrollPhaseEnded))
	in.dispatch(gestureFrame(at, Pt(0, -30), ScrollPhaseMomentum))
	in.dispatch(gestureFrame(at, Pt(0, -10), ScrollPhaseMomentumEnded))
	if outer.offset != 0 {
		t.Fatalf("the outer moved to %v on a gesture and momentum the inner took", outer.offset)
	}
}

func TestANewGestureGoesToTheViewThatCanMove(t *testing.T) {
	t.Parallel()
	outer, inner := nestedScrollers()
	inner.offset = 100
	var in inputState
	paintFrame(&in, outer, Sz(100, 200))
	at := Pt(50, 50)
	in.dispatch(gestureFrame(at, Pt(0, -60), ScrollPhaseBegan))
	if outer.offset != 60 || inner.offset != 100 {
		t.Fatalf("outer at %v, inner at %v; want a gesture begun over an inner at its end to move the outer", outer.offset, inner.offset)
	}
	// Back up: the gesture stays with the outer, though the inner could
	// move now.
	in.dispatch(gestureFrame(at, Pt(0, 30), ScrollPhaseChanged))
	if outer.offset != 30 || inner.offset != 100 {
		t.Fatalf("outer at %v, inner at %v; want the gesture kept by the outer", outer.offset, inner.offset)
	}
	// A new gesture, whose ScrollPhaseBegan was merged away, starts afresh.
	in.dispatch(gestureFrame(at, Point{}, ScrollPhaseEnded))
	in.dispatch(gestureFrame(at.Add(Pt(0, -30)), Pt(0, 30), ScrollPhaseChanged))
	if inner.offset != 70 {
		t.Fatalf("inner at %v after a new gesture over it, want 70", inner.offset)
	}
}

func TestAWheelsNotchesAreNotLatched(t *testing.T) {
	t.Parallel()
	outer, inner := nestedScrollers()
	var in inputState
	paintFrame(&in, outer, Sz(100, 200))
	in.dispatch(gestureFrame(Pt(50, 50), Pt(0, -20), ScrollPhaseBegan))
	if inner.offset != 20 {
		t.Fatalf("inner at %v", inner.offset)
	}
	// Mid-gesture, a wheel's notch and a scroll with no phase go where the
	// pointer is.
	in.dispatch(frameInput{pos: Pt(50, 150), wheel: Pt(0, -1)})
	in.dispatch(frameInput{pos: Pt(50, 150), wheelPx: Pt(0, -10)})
	if outer.offset != WheelLine+10 || inner.offset != 20 {
		t.Fatalf("outer at %v, inner at %v; want the notch and the scroll with no phase to reach the outer under the pointer", outer.offset, inner.offset)
	}
}

func TestALatchedViewThatGoesLetsTheGestureOn(t *testing.T) {
	t.Parallel()
	outer, inner := nestedScrollers()
	var in inputState
	paintFrame(&in, outer, Sz(100, 200))
	at := Pt(50, 50)
	in.dispatch(gestureFrame(at, Pt(0, -20), ScrollPhaseBegan))
	if inner.offset != 20 {
		t.Fatalf("inner at %v", inner.offset)
	}
	outer.child = column(sized(100, 400)) // the inner is gone
	paintFrame(&in, outer, Sz(100, 200))
	in.dispatch(gestureFrame(at, Pt(0, -20), ScrollPhaseChanged))
	if outer.offset != 20 {
		t.Fatalf("outer at %v, want the gesture gone on to it", outer.offset)
	}
}

// hscroller is a scroller across: it takes a scroll that goes left or
// right, and of a gesture only one that moves it.
type hscroller struct {
	child  Widget
	offset float64
	max    float64
}

func (s *hscroller) Layout(c Constraints, env Env) Size {
	s.child.Layout(Loose(c.Max()), env)
	return c.Constrain(c.Max())
}
func (s *hscroller) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, s)
	dst.Paint(s.child, r)
}
func (s *hscroller) HandlePointer(ev PointerEvent) bool {
	dx := ev.ScrollBy().X
	if ev.Kind != PointerScroll || dx == 0 {
		return false
	}
	before := s.offset
	s.offset = min(max(s.offset-dx, 0), s.max)
	return s.offset != before
}

func TestADiagonalGestureLatchesEachAxisToItsView(t *testing.T) {
	t.Parallel()
	inner := &scroller{child: sized(100, 200)}
	across := &hscroller{child: column(around(100, 100, inner)), max: 500}
	var in inputState
	paintFrame(&in, across, Sz(300, 300))
	in.dispatch(gestureFrame(Pt(50, 50), Pt(-10, -10), ScrollPhaseBegan))
	if across.offset != 10 || inner.offset != 10 {
		t.Fatalf("across at %v, inner at %v; want each axis to its view", across.offset, inner.offset)
	}
	// Off the inner: each axis stays with its view.
	in.dispatch(gestureFrame(Pt(200, 200), Pt(-10, -10), ScrollPhaseChanged))
	if across.offset != 20 || inner.offset != 20 {
		t.Fatalf("across at %v, inner at %v off the inner; want each axis kept", across.offset, inner.offset)
	}
	// A press ends the gesture.
	in.dispatch(frameInput{pos: Pt(200, 200), down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{pos: Pt(200, 200), up: []MouseButton{MouseButtonLeft}})
	in.dispatch(gestureFrame(Pt(200, 200), Pt(-10, -10), ScrollPhaseChanged))
	if across.offset != 30 || inner.offset != 20 {
		t.Fatalf("across at %v, inner at %v after a press; want the vertical part to reach no view", across.offset, inner.offset)
	}
}
