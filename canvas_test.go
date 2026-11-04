package ggui

import (
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/third_party/text"
	"golang.org/x/image/font/gofont/goregular"
)

type adoptionCounter struct {
	twice
	calls int
	from  any
}

func (a *adoptionCounter) Adopt(prev any) { a.calls++; a.from = prev }

func TestAdoptPreservesHandlerKindsAndPaintOrder(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"pointer", "key", "both"} {
		t.Run(kind, func(t *testing.T) {
			first, last, next := &adoptionCounter{}, &adoptionCounter{}, &adoptionCounter{}
			r := Rct(Pt(0, 0), Sz(20, 20))
			c := Canvas{prev: []hitRegion{
				{rect: r, pointer: first, key: first},
				{rect: r, pointer: last, key: last},
			}}
			h := hitRegion{rect: r}
			if kind != "key" {
				h.pointer = next
			}
			if kind != "pointer" {
				h.key = next
			}
			c.adopt(&h)
			if next.calls != 1 || next.from != last {
				t.Fatalf("adopted %d times from %p, want once from %p", next.calls, next.from, last)
			}
		})
	}
}

func TestAdoptTakesTheLastRegionWithTheSameID(t *testing.T) {
	t.Parallel()
	first, last, next := &adoptionCounter{}, &adoptionCounter{}, &adoptionCounter{}
	id := new(byte)
	c := Canvas{prev: []hitRegion{
		{rect: Rct(Pt(0, 0), Sz(20, 20)), id: id, pointer: first},
		{rect: Rct(Pt(50, 50), Sz(20, 20)), id: id, pointer: last},
	}}
	h := hitRegion{rect: Rct(Pt(90, 90), Sz(20, 20)), id: id, pointer: next}
	c.adopt(&h)
	if next.calls != 1 || next.from != last {
		t.Fatalf("adopted %d times from %p, want once from %p", next.calls, next.from, last)
	}
}

func TestAdoptSkipsIDsTheLanguageCannotCompare(t *testing.T) {
	t.Parallel()
	// Interactive.SetKey takes any value, so an ID may be a slice. Matching it
	// is impossible, but it must not bring the frame down either.
	old, next := &adoptionCounter{}, &adoptionCounter{}
	r := Rct(Pt(0, 0), Sz(20, 20))
	c := Canvas{prev: []hitRegion{{rect: r, id: []int{1}, pointer: old}}}
	h := hitRegion{rect: r, id: []int{1}, pointer: next}
	c.adopt(&h)
	if next.calls != 0 {
		t.Fatalf("adopted %d times from an uncomparable ID, want none", next.calls)
	}
}

func TestAdoptIndexIsRebuiltEachFrame(t *testing.T) {
	t.Parallel()
	first, second := &adoptionCounter{}, &adoptionCounter{}
	r := Rct(Pt(0, 0), Sz(20, 20))
	c := Canvas{prev: []hitRegion{{rect: r, pointer: first}}}
	h := hitRegion{rect: r, pointer: &adoptionCounter{}}
	c.adopt(&h)

	// The next frame's prev holds someone else at the same Rect.
	c.prev, c.hits = []hitRegion{{rect: r, pointer: second}}, nil
	c.nextFrame()
	next := &adoptionCounter{}
	h = hitRegion{rect: r, pointer: next}
	c.adopt(&h)
	if next.calls != 1 || next.from != second {
		t.Fatalf("adopted %d times from %p, want once from %p", next.calls, next.from, second)
	}
}

func TestCanvasScaleConvertsToImagePixels(t *testing.T) {
	t.Parallel()
	c := &Canvas{scale: 2}
	if got := c.Px(7.5); got != 15 {
		t.Fatalf("Px(7.5) = %v at scale 2, want 15", got)
	}
	if got := c.physical(Rct(Pt(1.25, 2), Sz(3, 4.5))); got != image.Rect(2, 4, 9, 13) {
		t.Fatalf("physical = %v, want (2,4)-(9,13) rounded outwards", got)
	}
	g := c.Geo(Pt(10, 20))
	x, y := g.Apply(1, 1)
	if x != 22 || y != 42 {
		t.Fatalf("Geo maps (1,1) to (%v,%v), want (22,42)", x, y)
	}
	var zero Canvas
	if zero.Scale() != 1 || (*Canvas)(nil).Scale() != 1 {
		t.Fatal("zero and nil canvases must scale by 1")
	}
}

func TestClipKeepsScale(t *testing.T) {
	t.Parallel()
	c := &Canvas{scale: 2}
	if got := c.Clip(Rct(Pt(0, 0), Sz(10, 10))).Scale(); got != 2 {
		t.Fatalf("clipped canvas scale = %v, want 2", got)
	}
}

func TestTextRasterizesAtScaledSize(t *testing.T) {
	t.Parallel()
	w := Text("x").Font(MustFont(goregular.TTF).NoFallback()).Size(14)
	face, ok := w.faceAt(2).(*text.GoTextFace)
	if !ok || face.Size != 28 {
		t.Fatalf("face at scale 2 = %+v, want a GoTextFace of size 28", face)
	}
	if w.faceAt(1).(*text.GoTextFace).Size != 14 {
		t.Fatal("layout face must stay at the logical size")
	}
}

func TestFillRectAndTextTolerateNilTargets(t *testing.T) {
	t.Parallel()
	(*Canvas)(nil).FillRect(Rect{}, nil)
	(&Canvas{}).FillRect(Rct(Pt(0, 0), Sz(1, 1)), nil)
	w := Text("x")
	w.Layout(Loose(Sz(100, 100)), Env{})
	w.Paint(nil, Rct(Pt(0, 0), Sz(10, 10)))
	w.Paint(&Canvas{}, Rct(Pt(0, 0), Sz(10, 10)))
}

// A Canvas clipped into again is the Clip it would be made anew, with
// nothing kept of what was painted through it before.
func TestClipReusingIsAClipMadeAgain(t *testing.T) {
	t.Parallel()
	c := &Canvas{scale: 2, clip: Rct(Pt(0, 0), Sz(50, 50)), clipped: true}
	var reuse Canvas
	reuse.hits = []hitRegion{{}}
	reuse.inert = true
	r := Rct(Pt(10, 10), Sz(100, 100))
	got := c.ClipReusing(&reuse, r)
	want := c.Clip(r)
	if got != &reuse || got.clip != want.clip || got.parent != c || got.Scale() != 2 || got.inert || len(got.hits) != 0 {
		t.Fatalf("reused clip %+v; want %+v made anew", *got, *want)
	}
}

func TestVisiblePaintBounds(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(100, 100)
	defer img.Deallocate()
	c := (&Canvas{Image: img, scale: 2}).Clip(Rct(Pt(10, 10), Sz(20, 20)))
	for _, tc := range []struct {
		name  string
		r     Rect
		extra float64
		want  bool
	}{
		{"inside", Rct(Pt(12, 12), Sz(4, 4)), 0, true},
		{"outside", Rct(Pt(35, 12), Sz(4, 4)), 0, false},
		{"stroke reaches clip", Rct(Pt(31, 12), Sz(4, 4)), 2, true},
		{"antialias edge", Rct(Pt(30.25, 12), Sz(4, 4)), 0, true},
		{"negative dimensions fallback", Rct(Pt(40, 12), Sz(-30, 4)), 0, true},
		{"nonfinite fallback", Rct(Pt(math.NaN(), 12), Sz(4, 4)), 0, true},
		{"large float32 fallback", Rct(Pt(1<<24, 12), Sz(4, 4)), 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.visiblePaintBounds(tc.r, tc.extra); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	empty := c.Clip(Rct(Pt(80, 80), Sz(1, 1)))
	if empty.visiblePaintBounds(Rct(Point{}, Sz(100, 100)), 0) {
		t.Fatal("empty clip draws")
	}
}

func TestEmptyClipRetainsTextSemanticsAndTrace(t *testing.T) {
	img := ggfx.NewImage(100, 100)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	inspectorEnabled.Store(true)
	c.fs().tracing = true
	txt := Text("offscreen text")
	size := txt.Layout(Tight(Sz(100, 20)), rootEnv())
	clipped := c.Clip(Rct(Pt(200, 200), Sz(10, 10)))
	clipped.Paint(txt, Rct(Pt(200, 200), size))
	if len(c.fs().sem) != 1 || c.fs().sem[0].node.Name != "offscreen text" {
		t.Fatal("drawing cull lost semantics")
	}
	// The trace is collected only once something asked for frames.
	if len(c.fs().trace) != 1 {
		t.Fatal("drawing cull lost inspection")
	}
}

func BenchmarkOffscreenPaintBounds(b *testing.B) {
	img := ggfx.NewImage(100, 100)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	r := Rct(Pt(0, 500), Sz(120, 40))
	b.ReportAllocs()
	for b.Loop() {
		c.visiblePaintBounds(r, 0)
	}
}

// TestTextSkipsLinesOutsideTheClip paints a long Text into a canvas that
// shows one line of it: the lines outside are not shaped and drawn, so it
// allocates a fraction of what painting all of them does.
func TestTextSkipsLinesOutsideTheClip(t *testing.T) {
	img := ggfx.NewImage(400, 400)
	defer img.Deallocate()
	var s strings.Builder
	for i := range 200 {
		fmt.Fprintf(&s, "line %d of a long text\n", i)
	}
	txt := Text(s.String()).NoWrap()
	size := txt.Layout(Loose(Sz(400, 1e6)), rootEnv())
	full := &Canvas{Image: img}
	shown := full.Clip(Rct(Pt(0, 0), Sz(400, 20)))
	all := testing.AllocsPerRun(5, func() { full.Paint(txt, Rct(Point{}, size)) })
	one := testing.AllocsPerRun(5, func() { shown.Paint(txt, Rct(Point{}, size)) })
	// One line allocates about a twelfth of what all do, but about a tenth
	// under the race detector; painting all of them would allocate as much.
	if one*5 > all {
		t.Errorf("painting one line of 200 allocates %v, all of them %v", one, all)
	}
}

// shortcutKeeper registers a chord from Paint, under a focus trap if trap
// is set and on an inert canvas while inert is, with a key region so the
// trap has something to hold.
type shortcutKeeper struct {
	chord Chord
	fn    func()
	trap  bool
	inert *StateValue[bool]
}

func (s *shortcutKeeper) Layout(c Constraints, _ Env) Size { return c.Constrain(Sz(20, 20)) }
func (s *shortcutKeeper) HandleKey(KeyEvent)               {}
func (s *shortcutKeeper) Paint(dst *Canvas, r Rect) {
	if s.inert != nil && Untrack(s.inert.Get) {
		dst = dst.Inert()
	}
	paint := func(dst *Canvas) {
		dst.HitKey(r, s)
		dst.Shortcut(s.chord, s.fn)
	}
	if s.trap {
		dst.FocusTrap(s, nil, paint)
		return
	}
	paint(dst)
}

// A painted shortcut runs while its widget is painted and stops when the
// widget goes, when it is inert, or while a trap it is outside is open.
func TestCanvasShortcutFollowsWhatIsPainted(t *testing.T) {
	t.Parallel()
	shown, inert, dialog := State(true), State(false), State(false)
	var saves, dialogSaves int
	p := ProbeBuilder(func() Widget {
		return Column(
			If(shown, func() Widget {
				return &shortcutKeeper{chord: MustChord("cmd+s"), fn: func() { saves++ }, inert: inert}
			}),
			If(dialog, func() Widget {
				return &shortcutKeeper{chord: MustChord("cmd+s"), fn: func() { dialogSaves++ }, trap: true}
			}),
		)
	}, Sz(100, 100))
	defer p.Close()
	press := func(what string, wantSaves, wantDialog int) {
		t.Helper()
		p.Key("cmd+s")
		if saves != wantSaves || dialogSaves != wantDialog {
			t.Fatalf("%s: saves %d, dialog saves %d; want %d, %d", what, saves, dialogSaves, wantSaves, wantDialog)
		}
	}
	p.Frame()
	press("painted", 1, 0)
	dialog.Set(true)
	p.Frame()
	press("behind a dialog", 1, 1)
	dialog.Set(false)
	inert.Set(true)
	p.Frame()
	press("inert", 1, 1)
	inert.Set(false)
	shown.Set(false)
	p.Frame()
	press("gone", 1, 1)
	shown.Set(true)
	p.Frame()
	press("back", 2, 1)
}

func TestClipTakesNoAllocationOnceItsCanvasesAreMade(t *testing.T) {
	c := &Canvas{}
	f := c.fs()
	paint := func() {
		f.resetCanvases()
		for i := range 8 {
			c.Clip(Rct(Pt(float64(i), 0), Sz(10, 10))).Clip(Rct(Point{}, Sz(5, 5)))
		}
	}
	paint()
	if n := testing.AllocsPerRun(100, paint); n != 0 {
		t.Fatalf("a paint's Clips took %v allocations", n)
	}
	if len(f.canvases) != 16 {
		t.Fatalf("%d canvases kept, want the 16 a paint takes", len(f.canvases))
	}
}

func TestAProbesClipsAreHandedOutAgainEachFrame(t *testing.T) {
	p := ProbeBuilder(func() Widget { return twoClips{} }, Sz(20, 20))
	for range 5 {
		p.Frame()
	}
	if n := len(p.canvas.fs().canvases); n != 2 {
		t.Fatalf("%d canvases after 5 frames of 2 Clips each", n)
	}
}

// twoClips paints through two nested Clips.
type twoClips struct{}

func (twoClips) Layout(cs Constraints, _ Env) Size { return cs.Max() }
func (twoClips) Paint(dst *Canvas, r Rect)         { dst.Clip(r).Clip(r) }
