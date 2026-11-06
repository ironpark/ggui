package ggui

import (
	"errors"
	"image/color"
	"slices"
	"testing"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/third_party/text"
	"github.com/ironpark/ggui/runtime"
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

// A Probe answers dialogs with a stub that cancels until its Paths are set,
// and records what was asked, so code written against Host runs headless.
func TestProbeDialogsAreAStub(t *testing.T) {
	t.Parallel()
	p := NewProbe(Text("x"), Sz(10, 10))
	defer p.Close()
	var h Host = p
	if _, err := h.Dialogs().OpenFile(runtime.FileDialog{Title: "one"}); !errors.Is(err, runtime.ErrCanceled) {
		t.Fatalf("default stub err = %v, want ErrCanceled", err)
	}
	stub := p.Dialogs().(*runtime.StubFilePicker)
	if len(stub.Asked) != 1 || stub.Asked[0].Title != "one" {
		t.Fatalf("Asked = %+v", stub.Asked)
	}
	stub.Paths = []string{"/a"}
	if got, err := h.Dialogs().SaveFile(runtime.FileDialog{}); err != nil || got != "/a" {
		t.Fatalf("SaveFile = %q, %v", got, err)
	}
	if len(stub.Asked) != 2 {
		t.Fatalf("Asked = %+v, want both dialogs recorded", stub.Asked)
	}
}

// Each Probe builds its tree in a reactive runtime of its own, so a frame
// of one runs none of the other's bindings, even when both read the same
// signal. User effects were already kept to their own loop; the widget
// bindings a tree makes with Observe were not.
func TestProbesDoNotRunEachOthersEffects(t *testing.T) {
	t.Parallel()
	n := State(0)
	var inA, inB int
	a := ProbeBuilder(func() Widget { return Box() }, Sz(10, 10)).Setup(func() {
		reactive.Observe(func() { n.Get(); inA++ })
	})
	defer a.Close()
	b := ProbeBuilder(func() Widget { return Box() }, Sz(10, 10)).Setup(func() {
		reactive.Observe(func() { n.Get(); inB++ })
	})
	defer b.Close()
	a.Frame()
	b.Frame()
	inA, inB = 0, 0

	n.Set(1)
	a.Frame()
	if inA != 1 || inB != 0 {
		t.Fatalf("a's frame ran a=%d b=%d, want 1 0", inA, inB)
	}
	b.Frame()
	if inA != 1 || inB != 1 {
		t.Fatalf("b's frame ran a=%d b=%d, want 1 1", inA, inB)
	}
}

// A probe a test forgets to close keeps its effects to itself: the next
// probe's frames never run them.
func TestALeakedProbeStaysOutOfTheNextOnesFrames(t *testing.T) {
	t.Parallel()
	n := State(0)
	leaked := 0
	ProbeBuilder(func() Widget { return Box() }, Sz(10, 10)).Setup(func() {
		reactive.Observe(func() { n.Get(); leaked++ })
	}).Frame()
	leaked = 0

	p := NewProbe(Box(), Sz(100, 20))
	defer p.Close()
	n.Set(1)
	p.Frame()
	if leaked != 0 {
		t.Fatalf("a leaked probe's effect ran %d times in another probe's frame", leaked)
	}
}

// A widget built before its probe, outside any tree, still follows its
// signals: what has no owner belongs to the process's runtime, which every
// probe's frame flushes too.
func TestAWidgetBuiltBeforeItsProbeStillFollowsItsSignals(t *testing.T) {
	t.Parallel()
	v := State("a")
	in := TextInput(v)
	p := NewProbe(in, Sz(200, 30))
	defer p.Close()
	p.Frame()
	v.Set("changed")
	p.Frame()
	if in.ed.Text != "changed" {
		t.Fatalf("editor shows %q, want the signal's new value", in.ed.Text)
	}
}

// Advance moves one probe's clock: another probe, and code outside every
// probe, keep reading their own.
func TestAdvanceMovesOnlyItsOwnProbesClock(t *testing.T) {
	t.Parallel()
	var inA time.Time
	a := NewProbe(FromFuncs(func(c Constraints, _ Env) Size { return c.Constrain(Sz(1, 1)) }, func(*Canvas, Rect) { inA = Now() }), Sz(10, 10))
	defer a.Close()
	b := NewProbe(Box(), Sz(10, 10))
	defer b.Close()
	a.Frame()
	start := inA
	a.Advance(time.Hour)
	if got := inA.Sub(start); got < time.Hour {
		t.Fatalf("a painted %v after advancing an hour, want at least an hour on", got)
	}
	b.Frame()
	if got := Now(); got.Sub(start) >= time.Hour {
		t.Fatalf("after b's frame Now is %v on from a's start: a's Advance leaked out", got.Sub(start))
	}
}

// An animation keeps the world it was made in: another probe's frames do
// not step it.
func TestAnAnimationStepsOnlyInItsOwnProbe(t *testing.T) {
	t.Parallel()
	var tw *Tweened[float64]
	a := ProbeBuilder(func() Widget { return Box() }, Sz(10, 10)).Setup(func() {
		tw = Tween(0.0, time.Second).Easing(EaseLinear)
	})
	defer a.Close()
	b := NewProbe(Box(), Sz(10, 10))
	defer b.Close()
	a.Frame()
	tw.Set(100)
	a.Advance(0)
	b.Advance(0)
	b.Advance(time.Second)
	if got := Untrack(tw.Get); got != 0 {
		t.Fatalf("another probe's frames moved the tween to %v", got)
	}
	a.Advance(500 * time.Millisecond)
	if got := Untrack(tw.Get); got != 50 {
		t.Fatalf("its own probe moved it to %v, want 50", got)
	}
}

// SetEnv before a probe's first frame sets what that probe starts from, on
// the test's own goroutine alone, so parallel tests may each set their own.
func TestSetEnvBeforeAProbeStaysOnItsGoroutine(t *testing.T) {
	t.Parallel()
	for _, c := range []color.RGBA{{R: 1, A: 255}, {G: 2, A: 255}, {B: 3, A: 255}} {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			SetEnv(Env{}.With(BackgroundKey, color.Color(c)))
			var seen color.Color
			p := NewProbe(FromFuncs(func(cs Constraints, env Env) Size {
				seen = env.Background()
				return cs.Constrain(Sz(1, 1))
			}, func(*Canvas, Rect) {}), Sz(10, 10))
			defer p.Close()
			for range 20 {
				p.Frame()
				if seen != color.Color(c) {
					t.Fatalf("laid out under %v, want %v", seen, c)
				}
				p.Resize(Sz(10+float64(c.R+c.G+c.B), 10))
			}
		})
	}
}

func TestEachProbeHasItsOwnClipboard(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	a := NewProbe(TextInput(State("secret")), Sz(200, 20))
	defer a.Close()
	b := NewProbe(Text("other"), Sz(100, 20))
	defer b.Close()
	b.Frame()
	a.Click(Pt(190, 10))
	a.Key("cmd+a", "cmd+c")
	if got := a.Clipboard().Read(); got != "secret" {
		t.Fatalf("copy put %q on the probe's clipboard", got)
	}
	if got := b.Clipboard().Read(); got != "" {
		t.Fatalf("copy in one probe reached another's clipboard: %q", got)
	}
}

// A probe's frame advances the clock the glyph, shaping and face caches
// age by, once, as a window's frame does: under a probe they would
// otherwise keep every string ever shaped. Not parallel, so no other
// probe's frames run in between.
func TestProbeFramesAgeTheTextCaches(t *testing.T) {
	p := NewProbe(Text("aging"), Sz(100, 20))
	defer p.Close()
	p.Frame()
	before := text.Tick()
	for range 3 {
		p.Frame()
	}
	p.Advance(16 * time.Millisecond)
	if got := text.Tick() - before; got != 4 {
		t.Errorf("4 probe frames advanced the text tick by %d, want 4", got)
	}
}
