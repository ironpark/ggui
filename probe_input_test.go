package ggui

import (
	"image/color"
	"slices"
	"testing"

	"github.com/ironpark/ggfx"
)

func TestProbeFlushSettlesEffectsWithoutPainting(t *testing.T) {
	t.Parallel()
	n := State(1)
	var doubled *DerivedValue[int]
	paints := 0
	p := ProbeBuilder(func() Widget {
		doubled = Derived(func() int { return 2 * n.Get() })
		return FromFuncs(
			func(c Constraints, _ Env) Size { return c.Constrain(Sz(10, 10)) },
			func(*Canvas, Rect) { paints++ },
		)
	}, Sz(10, 10))
	defer p.Close()
	p.Flush()
	if doubled == nil || Untrack(doubled.Get) != 2 {
		t.Fatal("Flush before the first frame did not build the tree")
	}
	n.Set(5)
	p.Flush()
	if got := Untrack(doubled.Get); got != 10 || paints != 0 {
		t.Fatalf("after Flush: derived %d, paints %d; want 10 and no paint", got, paints)
	}
}

func TestProbeOnFrameRunsBeforeEachFrame(t *testing.T) {
	t.Parallel()
	var order []string
	p := NewProbe(FromFuncs(
		func(c Constraints, _ Env) Size { return c.Constrain(Sz(10, 10)) },
		func(*Canvas, Rect) { order = append(order, "paint") },
	), Sz(10, 10))
	defer p.Close()
	p.OnFrame(func() { order = append(order, "frame") })
	p.Frame()
	p.Frame()
	if want := []string{"frame", "paint", "frame", "paint"}; !slices.Equal(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestProbePressAndReleaseTapOnlyInside(t *testing.T) {
	t.Parallel()
	taps := 0
	p := NewProbe(Column(Tap(Box().Size(50, 50), func() { taps++ }), Box().Size(50, 50)), Sz(50, 100))
	defer p.Close()
	p.Press(Pt(10, 10))
	if taps != 0 {
		t.Fatal("a press alone tapped")
	}
	p.Release(Pt(10, 75))
	if taps != 0 {
		t.Fatal("a press released outside tapped")
	}
	p.Press(Pt(10, 10))
	p.Release(Pt(20, 20))
	if taps != 1 {
		t.Fatalf("a press released inside tapped %d times, want 1", taps)
	}
}

func TestProbeTextReachesTheFocusedWidget(t *testing.T) {
	t.Parallel()
	var typed string
	p := NewProbe(Focus(Box().Size(50, 50)).OnText(func(s string) { typed += s }), Sz(50, 50))
	defer p.Close()
	p.Text("ignored")
	if typed != "" {
		t.Fatalf("text reached an unfocused widget: %q", typed)
	}
	p.Click(Pt(10, 10))
	p.Text("hé")
	p.Text("llo")
	if typed != "héllo" {
		t.Fatalf("typed %q, want \"héllo\"", typed)
	}
}

func TestCanvasPointerAndFocusWithinReadTheFrameStart(t *testing.T) {
	t.Parallel()
	var (
		at        Point
		known     bool
		focusedIn bool
		elsewhere bool
	)
	field := Rct(Pt(0, 0), Sz(40, 20))
	recorder := FromFuncs(
		func(c Constraints, _ Env) Size { return c.Constrain(Sz(100, 20)) },
		func(dst *Canvas, _ Rect) {
			at, known = dst.Pointer()
			focusedIn = dst.FocusWithin(field)
			elsewhere = dst.FocusWithin(Rct(Pt(60, 60), Sz(10, 10)))
		},
	)
	p := NewProbe(Column(Focus(Box().Size(40, 20)), recorder), Sz(100, 100))
	defer p.Close()
	p.Frame()
	if known || focusedIn {
		t.Fatalf("before any input: pointer known %v, focus within %v; want neither", known, focusedIn)
	}
	p.Move(Pt(30, 30))
	if !known || at != Pt(30, 30) {
		t.Fatalf("pointer = %v, %v; want (30,30)", at, known)
	}
	p.Click(Pt(10, 10))
	p.Frame()
	if !focusedIn || elsewhere {
		t.Fatalf("with the field focused: within it %v, within an empty rect %v", focusedIn, elsewhere)
	}
	var nilCanvas *Canvas
	if _, ok := nilCanvas.Pointer(); ok || nilCanvas.FocusWithin(field) {
		t.Fatal("a nil canvas reports a pointer or focus")
	}
}

func TestCanvasShapesTolerateNothingToDrawOn(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(4, 4)
	red := color.RGBA{R: 255, A: 255}
	for _, c := range []*Canvas{nil, {}} {
		c.FillCircle(Pt(5, 5), 3, red)
		c.StrokeLine(Pt(0, 0), Pt(10, 10), 1, red)
		c.DrawImage(img, Rct(Pt(0, 0), Sz(4, 4)), ImageOptions{})
	}
	c := &Canvas{Image: img}
	// Nothing asked to be drawn: no colour, no size, or fully faded.
	c.FillCircle(Pt(5, 5), 0, red)
	c.FillCircle(Pt(5, 5), 3, nil)
	c.StrokeLine(Pt(0, 0), Pt(10, 10), 0, red)
	c.DrawImage(nil, Rct(Pt(0, 0), Sz(4, 4)), ImageOptions{})
	c.DrawImage(img, Rct(Pt(0, 0), Sz(4, 4)), ImageOptions{Fade: 1})
}
