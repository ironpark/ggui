package ggui

import (
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui/internal/reactive"
)

// A direct write to Inert, bypassing SetInert, still asks for a layout
// once Sync sees it.
func TestInteractiveDirectInertWriteRequestsLayout(t *testing.T) {
	t.Parallel()
	var s Interactive
	s.Sync()
	before := reactive.LayoutGen()
	s.SetInert(true)
	s.Sync()
	if reactive.LayoutGen() == before {
		t.Fatal("direct Inert write not noticed by Sync")
	}
	before = reactive.LayoutGen()
	s.Sync()
	if reactive.LayoutGen() != before {
		t.Fatal("Sync requested a layout with nothing changed")
	}
}

func TestConfigInspectorChord(t *testing.T) {
	t.Parallel()
	a := New(Config{Inspector: "f1"}, func() Widget { return Box() })
	if a.inspectChord.Key != KeyF1 {
		t.Fatalf("chord key = %v", a.inspectChord.Key)
	}
	if New(Config{}, func() Widget { return Box() }).inspectChord.Key != 0 {
		t.Fatal("empty Inspector parsed a chord")
	}
}

func TestComponentSetupIsStable(t *testing.T) {
	t.Parallel()
	dep := State(0)
	setups := 0
	var local *StateValue[int]
	p := ProbeBuilder(func() Widget {
		return Component(func() Widget {
			setups++
			dep.Get()
			local = State(3)
			return View(local, func(n int) Widget { return Box().Size(float64(n), 1) })
		})
	}, Sz(100, 100))
	defer p.Close()
	p.Frame()
	local.Set(9)
	dep.Set(1)
	p.Frame()
	if setups != 1 || local.Get() != 9 {
		t.Fatal("setup reran or state was lost")
	}
}

func TestKeyDisposesAndRecreates(t *testing.T) {
	t.Parallel()
	key := State(1)
	setups, cleanups := 0, 0
	p := ProbeBuilder(func() Widget {
		return Key(key, func(int) Widget {
			setups++
			OnCleanup(func() { cleanups++ })
			return Box()
		})
	}, Sz(10, 10))
	p.Frame()
	key.Set(1)
	p.Frame()
	if setups != 1 || cleanups != 0 {
		t.Fatal("equal key remounted")
	}
	key.Set(2)
	p.Frame()
	if setups != 2 || cleanups != 1 {
		t.Fatal("changed key did not remount")
	}
	p.Close()
	if cleanups != 2 {
		t.Fatal("close did not clean up")
	}
}

func TestComponentPropsAreReadable(t *testing.T) {
	t.Parallel()
	n := State(1)
	var seen []int
	p := ProbeBuilder(func() Widget {
		return Component(func() Widget {
			return View(n, func(value int) Widget { seen = append(seen, value); return Box() })
		})
	}, Sz(10, 10))
	defer p.Close()
	p.Frame()
	n.Set(2)
	p.Frame()
	if len(seen) != 2 || seen[1] != 2 {
		t.Fatalf("props: %v", seen)
	}
}

func TestScrollAdoptsOffsetAcrossRebuild(t *testing.T) {
	t.Parallel()
	dep := State(0)
	p := ProbeBuilder(func() Widget {
		dep.Get()
		return Scroll(Box().Size(50, 500))
	}, Sz(50, 100))
	defer p.Close()
	p.Scroll(Pt(10, 10), Pt(0, -3))
	if off := p.root.(*ScrollWidget).offset; off != 60 {
		t.Fatalf("offset = %v, want 60", off)
	}
	dep.Set(1)
	p.Frame()
	if off := p.root.(*ScrollWidget).offset; off != 60 {
		t.Fatalf("after rebuild offset = %v, want 60", off)
	}
}

// keyedBox is a pointer handler with an identity and a counter it adopts.
type keyedBox struct {
	id    any
	count int
}

func (k *keyedBox) Layout(c Constraints, env Env) Size { return c.Constrain(Sz(10, 10)) }
func (k *keyedBox) Paint(dst *Canvas, r Rect)          { dst.HitPointer(r, k) }
func (k *keyedBox) HandlePointer(ev PointerEvent) bool { return true }
func (k *keyedBox) HitID() any                         { return k.id }
func (k *keyedBox) Adopt(prev any) {
	if p, ok := prev.(*keyedBox); ok {
		k.count = p.count
	}
}

func TestAdoptionFollowsIDNotRect(t *testing.T) {
	t.Parallel()
	a := &keyedBox{id: "a", count: 5}
	a2, b := &keyedBox{id: "a"}, &keyedBox{id: "b"}
	gen := State(0)
	p := ProbeBuilder(func() Widget {
		return Reactive(func() Widget {
			if gen.Get() == 0 {
				return Column(a, Box().Size(10, 10))
			}
			// Rebuilt and moved: a2 has a's id at another Rect; b sits where a was.
			return Column(b, a2)
		})
	}, Sz(10, 100))
	defer p.Close()
	p.Frame()
	gen.Set(1)
	p.Frame()
	if a2.count != 5 || b.count != 0 {
		t.Fatalf("a2.count = %d, b.count = %d; want 5 and 0", a2.count, b.count)
	}
}

func TestCachedSeesProvideChange(t *testing.T) {
	t.Parallel()
	k := NewEnvKey[float64]("w")
	layouts := 0
	leaf := FromFuncs(func(c Constraints, env Env) Size {
		layouts++
		w, _ := env.Get(k)
		return c.Constrain(Sz(w, 1))
	}, func(*Canvas, Rect) {})
	cached := Cached(leaf)
	env := rootEnv()
	at := func(v float64) Size { return Provide(k, v, cached).Layout(Loose(Sz(100, 100)), env) }
	if got := at(5).W; got != 5 {
		t.Fatalf("width = %v", got)
	}
	at(5)
	if layouts != 1 {
		t.Fatalf("same value laid out %d times, want 1", layouts)
	}
	if got := at(8).W; got != 8 || layouts != 2 {
		t.Fatalf("width = %v after %d layouts, want 8 after 2", got, layouts)
	}
}

func TestCachedTextInputFollowsSignal(t *testing.T) {
	t.Parallel()
	v := State("a")
	in := TextInput(v)
	p := NewProbe(Cached(in), Sz(200, 30))
	p.Frame()
	v.Set("bbbbbbbb")
	p.Frame()
	p.Frame()
	if in.ed.Text != "bbbbbbbb" {
		t.Fatalf("editor shows %q", in.ed.Text)
	}
}

func TestComponentGivesControlsStableIdentity(t *testing.T) {
	t.Parallel()
	value := State("hello")
	rebuild := State(0)
	var input *TextInputWidget
	p := ProbeBuilder(func() Widget {
		return Component(func() Widget {
			return Reactive(func() Widget { rebuild.Get(); input = TextInput(value); return input })
		})
	}, Sz(200, 100))
	defer p.Close()
	p.Frame()
	first := input
	if first.HitID() == nil {
		t.Fatal("missing identity")
	}
	p.Click(Pt(2, 5))
	p.Type(Mods{}, KeyArrowRight, KeyArrowRight)
	rebuild.Set(1)
	p.Frame()
	p.Type(Mods{}, KeyArrowRight)
	if first == input || first.HitID() != input.HitID() || input.ed.Caret != 3 || !input.Focused() {
		t.Fatal("rebuild lost identity, caret or focus")
	}
}

func TestTabReachesAControlOutsideTheScrollWindow(t *testing.T) {
	t.Parallel()
	items := Column(Focus(Box().Size(50, 50)), Focus(Box().Size(50, 50)), Focus(Box().Size(50, 50)))
	sc := Scroll(items)
	p := NewProbe(sc, Sz(50, 40))
	defer p.Close()
	p.Type(Mods{}, KeyTab, KeyTab, KeyTab)
	if sc.offset != 110 {
		t.Fatalf("offset = %v after tabbing to the third, fully hidden item; want 110", sc.offset)
	}
}

func TestRetainKeepsTheFocusedRow(t *testing.T) {
	t.Parallel()
	items := State([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	built := map[int]int{}
	list := EachKeyed(items, func(i int) int { return i }, func(rowItem EachItem[int]) Widget {
		r := rowItem.Value
		built[r.Get()]++
		return Focus(Box().Size(50, 10))
	}).ItemExtent(10).Retain(1)
	sc := Scroll(list)
	p := NewProbe(sc, Sz(50, 30))
	defer p.Close()
	p.Click(Pt(5, 5)) // focus row 0
	p.Scroll(Pt(5, 5), Pt(0, -5))
	p.Frame()
	p.Frame()
	if sc.offset < 30 {
		t.Fatalf("offset %v, want the first rows out of view", sc.offset)
	}
	p.Scroll(Pt(5, 5), Pt(0, 5)) // back up
	p.Frame()
	if built[0] != 1 {
		t.Fatalf("row 0 built %d times: the focused row was evicted", built[0])
	}
	if built[1]+built[2] < 3 {
		t.Fatalf("rows 1 and 2 built %d and %d times: one unfocused row should have been evicted with Retain(1)", built[1], built[2])
	}
}

func TestFramesThatOnlyPaintKeepTheDerivedEnv(t *testing.T) {
	t.Parallel()
	k := NewEnvKey[float64]("w")
	gen := State(0)
	layouts := 0
	cached := Cached(FromFuncs(func(c Constraints, env Env) Size {
		layouts++
		return c.Constrain(Sz(10, 10))
	}, func(*Canvas, Rect) {}))
	provided := Provide(k, 1, cached)
	root := FromFuncs(func(c Constraints, env Env) Size {
		gen.Get()
		return provided.Layout(c, env)
	}, func(dst *Canvas, r Rect) { provided.Paint(dst, r) })
	p := NewProbe(root, Sz(100, 100))
	defer p.Close()
	p.Frame()
	p.Frame()
	p.Frame()
	gen.Set(1)
	p.Frame()
	if layouts != 1 {
		t.Fatalf("the cached leaf laid out %d times, want once: frames that only painted aged out the Env its cache is keyed by", layouts)
	}
}

// counting is a leaf that counts its layouts.
type counting struct{ layouts int }

func (c *counting) Layout(Constraints, Env) Size { c.layouts++; return Sz(10, 10) }
func (c *counting) Paint(*Canvas, Rect)          {}

func TestCachedSkipsLayoutUntilSomethingInsideChanges(t *testing.T) {
	t.Parallel()
	leaf := &counting{}
	dep := State(0)
	var inner Widget
	dispose := reactive.Observe(func() {
		inner = Reactive(func() Widget { dep.Get(); return Column(leaf) })
	})
	defer dispose()
	c := Cached(inner)
	tree := Column(c, Box().Size(10, 10))
	env := rootEnv()
	tree.Layout(Tight(Sz(100, 100)), env)
	tree.Layout(Tight(Sz(100, 100)), env)
	if leaf.layouts != 1 {
		t.Fatalf("leaf laid out %d times under unchanged constraints, want 1", leaf.layouts)
	}
	tree.Layout(Tight(Sz(200, 100)), env)
	if leaf.layouts != 2 {
		t.Fatalf("leaf laid out %d times after a resize, want 2", leaf.layouts)
	}
	dep.Set(1)
	reactive.Flush()
	tree.Layout(Tight(Sz(200, 100)), env)
	if leaf.layouts != 3 {
		t.Fatalf("leaf laid out %d times after a rebuild inside, want 3", leaf.layouts)
	}
	tree.Layout(Tight(Sz(200, 100)), env.WithText(TextStyle{Size: 20}))
	if leaf.layouts != 4 {
		t.Fatalf("leaf laid out %d times after the inherited style changed, want 4", leaf.layouts)
	}
}

func TestCachedNestsAndFollowsScrollAndFor(t *testing.T) {
	t.Parallel()
	leaf := &counting{}
	items := State([]todo{{1, "a"}})
	var list Widget
	dispose := reactive.Observe(func() {
		list = EachKeyed(items, func(t todo) int { return t.ID }, func(EachItem[todo]) Widget { return leaf })
	})
	defer dispose()
	inner := Cached(list)
	offset := State(0.0)
	s := Scroll(Column(inner, Box().Size(10, 500))).BindOffset(offset)
	outer := Cached(s)
	env := rootEnv()
	outer.Layout(Tight(Sz(100, 100)), env)
	outer.Layout(Tight(Sz(100, 100)), env)
	if leaf.layouts != 1 {
		t.Fatalf("leaf laid out %d times, want 1", leaf.layouts)
	}
	items.Set([]todo{{1, "a"}, {2, "b"}})
	reactive.Flush()
	outer.Layout(Tight(Sz(100, 100)), env)
	if leaf.layouts != 3 {
		t.Fatalf("leaf laid out %d times after the list grew through two caches, want 3", leaf.layouts)
	}
	// A scroll through the bound signal changes the viewport the inner
	// cache saw, so it lays out again when the outer one is asked.
	offset.Set(40)
	s.Paint(nil, Rct(Pt(0, 0), Sz(100, 100)))
	outer.Layout(Tight(Sz(100, 100)), env)
	if leaf.layouts != 5 {
		t.Fatalf("leaf laid out %d times after scrolling, want 5", leaf.layouts)
	}
	if !outer.dirty && outer.valid {
		outer.Layout(Tight(Sz(100, 100)), env)
		if leaf.layouts != 5 {
			t.Fatalf("leaf laid out %d times on a still frame, want 5", leaf.layouts)
		}
	}
}

// countingLeaf records how often it was measured.
type countingLeaf struct{ n int }

func (c *countingLeaf) Layout(cs Constraints, env Env) Size {
	c.n++
	return cs.Constrain(Sz(10, 10))
}
func (c *countingLeaf) Paint(dst *Canvas, r Rect) {}

// A rebuild boundary is a layout boundary: writing a signal one subtree reads
// must not re-measure its siblings, even though the runtime lays out from the
// root whenever anything was written.
func TestRebuildBoundaryConfinesLayout(t *testing.T) {
	t.Parallel()
	a, b := State(0), State(0)
	leafA, leafB := &countingLeaf{}, &countingLeaf{}
	p := ProbeBuilder(func() Widget {
		return Column(
			Reactive(func() Widget { a.Get(); return leafA }),
			Reactive(func() Widget { b.Get(); return leafB }),
		)
	}, Sz(200, 200))
	defer p.Close()
	p.Frame()

	wasA, wasB := leafA.n, leafB.n
	a.Set(1)
	p.Frame()
	if leafA.n == wasA {
		t.Fatalf("the subtree that changed was not measured again")
	}
	if leafB.n != wasB {
		t.Fatalf("the untouched subtree was measured %d more times", leafB.n-wasB)
	}

	wasA, wasB = leafA.n, leafB.n
	p.Frame()
	if leafA.n != wasA || leafB.n != wasB {
		t.Fatalf("a still frame measured again: A+%d B+%d", leafA.n-wasA, leafB.n-wasB)
	}
}

func TestACachedSkippedWhileOthersChangeKeepsTheCachesInside(t *testing.T) {
	t.Parallel()
	leaf := &counting{}
	depA, depB := State(0), State(0)
	a := Cached(Column(Reactive(func() Widget { depA.Get(); return Box().Size(10, 10) }), Cached(Column(leaf))))
	b := Cached(Column(Reactive(func() Widget { depB.Get(); return Box().Size(10, 10) })))
	p := NewProbe(Column(a, b), Sz(100, 100))
	defer p.Close()
	p.Frame()
	before := leaf.layouts
	// Frames that lay out only b leave a skipped: nothing inside it derives
	// its Envs, which must still be there when a is next laid out.
	for i := 1; i <= 5; i++ {
		depB.Set(i)
		p.Frame()
	}
	depA.Set(1)
	p.Frame()
	if leaf.layouts != before {
		t.Fatalf("a Cached inside a laid out again %d times after a sibling of it changed", leaf.layouts-before)
	}
}

func TestEnvMemoStaysBoundedAsDerivationsChurn(t *testing.T) {
	var m envMemo
	for i := range 100_000 {
		m.put(derivedKey{from: uint64(i), key: "k", val: i}, Env{})
	}
	if n := len(m.cur) + len(m.prev); n > 2*envMemoMin {
		t.Fatalf("the memo holds %d entries after churn, want at most %d", n, 2*envMemoMin)
	}
	k := derivedKey{from: 1, key: "kept"}
	m.put(k, Env{rev: 42})
	for i := range envMemoMin * 4 {
		m.get(k) // in use: it survives however many others come and go
		m.put(derivedKey{from: uint64(i), key: "other"}, Env{})
	}
	if e, ok := m.get(k); !ok || e.rev != 42 {
		t.Fatal("an entry in use aged out")
	}
}

// readsWidth is a leaf as wide as the signal it reads while laid out.
type readsWidth struct {
	w       *StateValue[float64]
	layouts int
}

func (r *readsWidth) Layout(cs Constraints, _ Env) Size {
	r.layouts++
	return cs.Constrain(Sz(r.w.Get(), 10))
}
func (r *readsWidth) Paint(*Canvas, Rect) {}

// A signal read deep inside If, Key, Each, View and Cached, each a cache of
// its own, measures the root afresh when it changes, and nothing at all
// when it does not.
func TestADeepReadReachesTheRootThroughEveryCache(t *testing.T) {
	t.Parallel()
	width := State(10.0)
	other := State(0)
	show := State(true)
	key := State(1)
	rows := State([]todo{{1, "a"}})
	leaf := &readsWidth{w: width}
	var tree Widget
	dispose := reactive.Observe(func() {
		tree = Cached(Column(
			View(other, func(int) Widget { return Box().Size(5, 5) }),
			If(show, func() Widget {
				return Key(key, func(int) Widget {
					return Cached(Column(EachKeyed(rows, func(t todo) int { return t.ID }, func(EachItem[todo]) Widget {
						return View(rows, func([]todo) Widget { return Cached(Row(leaf)) })
					})))
				})
			}),
		))
	})
	defer dispose()
	env := rootEnv()
	layout := func() Size {
		reactive.Flush()
		return tree.Layout(Loose(Sz(500, 500)), env)
	}
	if got := layout(); got.W != 10 {
		t.Fatalf("width %v, want 10", got.W)
	}
	n := leaf.layouts
	layout()
	other.Set(1) // a sibling rebuilds; the leaf's subtree stands
	layout()
	if leaf.layouts != n {
		t.Fatalf("the leaf was laid out %d more times with nothing it reads changed", leaf.layouts-n)
	}
	width.Set(30)
	if got := layout(); got.W != 30 {
		t.Fatalf("width %v after the deep read changed, want 30", got.W)
	}
	width.Set(20)
	if got := layout(); got.W != 20 {
		t.Fatalf("width %v after it changed again, want 20", got.W)
	}
	n = leaf.layouts
	layout()
	if leaf.layouts != n {
		t.Fatal("the leaf was laid out again after settling")
	}
}

func TestTextfFollowsSignals(t *testing.T) {
	t.Parallel()
	n := State(1)
	w := Textf("n=%d %s", n, "x")
	p := NewProbe(w, Sz(200, 30))
	p.Frame()
	if w.value != "n=1 x" {
		t.Fatalf("value = %q", w.value)
	}
	n.Set(2)
	p.Frame()
	if w.value != "n=2 x" {
		t.Fatalf("after set value = %q", w.value)
	}
}

func TestViewAndWhen(t *testing.T) {
	t.Parallel()
	s := State("a")
	on := State(true)
	then, other := Text("then"), Text("other")
	builds := 0
	root := Column(View(s, func(v string) *TextWidget { builds++; return Text(v) }), If(on, func() Widget {
		return then
	}).Else(func() Widget {
		return other
	}))
	p := NewProbe(root, Sz(200, 100))
	p.Frame()
	s.Set("b")
	p.Frame()
	if builds != 2 {
		t.Fatalf("builds = %d", builds)
	}
	w := root.children[1].(*IfWidget)
	if w.current() != then {
		t.Fatal("When did not show then")
	}
	on.Set(false)
	p.Frame()
	if w.current() != other {
		t.Fatal("When did not switch to other")
	}
}

// If reads every condition in order, so a later branch's condition is
// subscribed even though it was chained on after construction; without an
// Else, nothing is shown when no condition holds.
func TestIfPicksTheFirstTrueBranch(t *testing.T) {
	t.Parallel()
	a, b := State(false), State(true)
	ta, tb, tc := Text("A"), Text("B"), Text("C")
	w := If(a, func() Widget {
		return ta
	}).ElseIf(b, func() Widget {
		return tb
	}).Else(func() Widget {
		return tc
	})
	p := NewProbe(w, Sz(200, 100))
	p.Frame()
	if w.current() != tb {
		t.Fatal("B should show while a is false and b is true")
	}
	b.Set(false)
	p.Frame()
	if w.current() != tc {
		t.Fatal("changing only the ElseIf condition did not switch to Else")
	}
	a.Set(true)
	b.Set(true)
	p.Frame()
	if w.current() != ta {
		t.Fatal("A should win once a is true")
	}

	bare := If(a, func() Widget {
		return ta
	})
	a.Set(false)
	q := NewProbe(bare, Sz(200, 100))
	q.Frame()
	if bare.current() != nil {
		t.Fatalf("If without Else showed %v, want nothing", bare.current())
	}
	if _, ok := q.Semantics().Find(RoleText, ""); ok {
		t.Fatal("If without Else painted a text node")
	}
}

// An If showing nothing is left out of its flow: no gap beside it in a
// Column, Row or Wrap, and justification counts the children that are there.
func TestIfShowingNothingTakesNoGap(t *testing.T) {
	t.Parallel()
	off := State(false)
	col := Column(Box().Size(10, 10), If(off, func() Widget {
		return Box().Size(10, 10)
	}), Box().Size(10, 10)).Gap(4)
	p := NewProbe(Align(col).At(0, 0), Sz(100, 100))
	p.Frame()
	if h := col.rects[0].Size.H + col.rects[1].Size.H + col.rects[2].Size.H; h != 20 || col.rects[2].Origin.Y != 14 {
		t.Fatalf("third child at y=%v with %v of content, want 14 after one box and one gap", col.rects[2].Origin.Y, h)
	}
	off.Set(true)
	p.Frame()
	if got := col.rects[2].Origin.Y; got != 28 {
		t.Fatalf("third child at y=%v once shown, want 28 after two boxes and two gaps", got)
	}

	off.Set(false)
	p.Frame()
	row := Row(Box().Size(10, 10), If(off, func() Widget {
		return Box().Size(10, 10)
	}), Box().Size(10, 10)).Justify(SpaceBetween)
	row.Layout(Tight(Sz(100, 10)), rootEnv())
	if got := row.rects[2].Origin.X; got != 90 {
		t.Fatalf("space-between placed the last box at x=%v, want 90 with the absent child ignored", got)
	}

	wrap := Wrap(Box().Size(10, 10), If(off, func() Widget {
		return Box().Size(10, 10)
	}), Box().Size(10, 10)).Gap(4)
	if w := wrap.Layout(Loose(Sz(100, 100)), rootEnv()).W; w != 24 {
		t.Fatalf("wrap width = %v, want two boxes and one gap", w)
	}
}

// Branches are constructed once and kept, so the one shown keeps its state
// across being hidden; the choice rebuilds the If alone, not its parent.
func TestIfKeepsBranchesAndRebuildsAlone(t *testing.T) {
	t.Parallel()
	on := State(true)
	then := TextOf(State("kept"))
	parentBuilds := 0
	var w *IfWidget
	p := ProbeBuilder(func() Widget {
		parentBuilds++
		w = If(on, func() Widget {
			return then
		}).Else(func() Widget {
			return Text("other")
		})
		return w
	}, Sz(200, 100))
	p.Frame()
	on.Set(false)
	p.Frame()
	on.Set(true)
	p.Frame()
	if w.current() != then {
		t.Fatal("the then branch was not the same widget after being hidden")
	}
	if parentBuilds != 1 {
		t.Fatalf("parent built %d times, want 1: If must switch in its own effect", parentBuilds)
	}
}

// An If the parent rebuilt away stops reacting: its effect is owned by the
// builder run that made it.
func TestIfIsDisposedWithItsParent(t *testing.T) {
	t.Parallel()
	on := State(true)
	gen := State(0)
	var first *IfWidget
	p := ProbeBuilder(func() Widget {
		return Reactive(func() Widget {
			w := If(on, func() Widget {
				return Text("a")
			}).Else(func() Widget {
				return Text("b")
			})
			if gen.Get() == 0 {
				first = w
			}
			return w
		})
	}, Sz(200, 100))
	p.Frame()
	gen.Set(1)
	p.Frame()
	before := first.current()
	on.Set(false)
	p.Frame()
	if first.current() != before {
		t.Fatal("a replaced If kept switching branches after its parent rebuilt")
	}
}

func TestSpacingAndNamedTextStyles(t *testing.T) {
	t.Parallel()
	env := Env{}.With(SpacingKey, 12.0)
	key := NewEnvKey[TextStyle]("heading")
	env = env.With(key, TextStyle{Size: 24})
	col := Column(Box().Size(10, 10), Box().Size(10, 10)).Space(2)
	if h := col.Layout(Loose(Sz(100, 100)), env).H; h != 20+2*env.Spacing() {
		t.Fatalf("height = %v", h)
	}
	title := Text("x").StyleKey(key).Role(RoleHeading)
	title.Layout(Loose(Sz(100, 100)), env)
	if title.resolved.Size != 24 {
		t.Fatalf("title size = %v", title.resolved.Size)
	}
}

// ViewOf rebuilds when the part it picks changes and not when the rest of
// the state does, even under a parent that rebuilds it.
func TestViewOfRebuildsOnlyForItsPart(t *testing.T) {
	t.Parallel()
	type doc struct {
		Title    string
		Selected int
		Rows     []string // makes doc incomparable: every Set notifies
	}
	d := State(doc{Title: "a"})
	parent := State(0)
	var built []string
	p := ProbeBuilder(func() Widget {
		return Reactive(func() Widget {
			parent.Get()
			return ViewOf(d, func(d doc) string { return d.Title }, func(title string) *TextWidget {
				built = append(built, title)
				return Text(title)
			})
		})
	}, Sz(200, 30))
	defer p.Close()
	p.Frame()
	d.Update(func(v doc) doc { v.Selected = 3; v.Rows = []string{"x"}; return v })
	p.Frame()
	d.Update(func(v doc) doc { v.Title = "b"; return v })
	p.Frame()
	parent.Set(1)
	p.Frame()
	d.Update(func(v doc) doc { v.Selected = 4; return v })
	p.Frame()
	if want := []string{"a", "b", "b"}; !slices.Equal(built, want) {
		t.Fatalf("built %q, want %q: once, once for the title, once for the parent", built, want)
	}
}

// This reader is intentionally not comparable, but its underlying signal is
// still a measurement dependency.
type propertyNumberReader struct{ get func() float64 }

func (r propertyNumberReader) Get() float64 { return r.get() }

func measuredPropertyProbe(w Widget) (*Probe, *Size, *int) {
	size := new(Size)
	layouts := new(int)
	root := Cached(FromFuncs(func(cs Constraints, env Env) Size {
		*layouts++
		*size = w.Layout(Loose(cs.Max()), env)
		return cs.Constrain(*size)
	}, func(dst *Canvas, rc Rect) { dst.Paint(w, Rct(rc.Origin, *size)) }))
	return NewProbe(root, Sz(600, 400)), size, layouts
}

func TestPropertyDimensionsReplaceSourcesUnderNestedCaches(t *testing.T) {
	t.Parallel()
	a, b := State(80.0), State(80.0)
	height := State(20.0)
	box := Box().BindWidth(a).BindHeight(height)
	p, size, _ := measuredPropertyProbe(Cached(box))
	defer p.Close()
	p.Frame()
	if *size != Sz(80, 20) {
		t.Fatal(*size)
	}
	box.BindWidth(b) // Same result, different dependencies.
	p.Frame()
	a.Set(90)
	p.Frame()
	if size.W != 80 {
		t.Fatal("old reader remained connected", *size)
	}
	b.Set(140)
	height.Set(30)
	p.Frame()
	if *size != Sz(140, 30) {
		t.Fatal(*size)
	}
	box.Size(140, 30) // Same values still detach both sources.
	b.Set(160)
	height.Set(60)
	p.Frame()
	if *size != Sz(140, 30) {
		t.Fatal("Size did not detach", *size)
	}
	box.Size(100, 40).BindWidth(propertyNumberReader{a.Get})
	a.Set(110)
	p.Frame()
	if *size != Sz(110, 40) {
		t.Fatal("non-comparable width reader", *size)
	}
}

func TestPropertyLiteralGeometryUpdatesMountedCaches(t *testing.T) {
	t.Parallel()
	box := Box(Text("x")).Pad(1)
	p, size, layouts := measuredPropertyProbe(box)
	defer p.Close()
	p.Frame()
	before := *size
	box.Padding(Insets(12))
	p.Frame()
	if size.W != before.W+22 || size.H != before.H+22 {
		t.Fatalf("padding did not invalidate: %v -> %v", before, *size)
	}
	n := *layouts
	box.Pad(12)
	p.Frame()
	if *layouts != n {
		t.Fatal("equivalent shorthand caused layout")
	}
	box.Fill(color.NRGBA{R: 255, A: 255}).Border(2, color.White)
	p.Frame()
	if *layouts != n {
		t.Fatal("paint-only properties invalidated measurement")
	}
}

func TestPropertyParentConfigurationConverges(t *testing.T) {
	t.Parallel()
	text := Text("unchanged")
	box := Box(text)
	layouts := 0
	root := FromFuncs(func(cs Constraints, env Env) Size {
		layouts++
		// These intermediate styles differ each pass. The final configuration is
		// consumed in this layout, so it must not force another frame's layout.
		text.Style(TextStyle{Size: 12, Color: color.Black}).Color(color.White)
		box.Pad(2).Padding(Insets(3))
		return Cached(box).Layout(cs, env)
	}, box.Paint)
	p := NewProbe(root, Sz(200, 100))
	defer p.Close()
	p.Advance(0)
	p.Frame()
	p.Frame()
	if layouts != 1 {
		t.Fatalf("configuration did not settle: %d", layouts)
	}
}

func TestTextPropertiesPullWithoutEffectsAndDetach(t *testing.T) {
	t.Parallel()
	start := reactive.Count()
	source := State("a")
	args := []any{source, Const(3)}
	a := TextOf(source)
	b := Textf("%s %d", args...)
	args[0] = "caller mutation"
	if reactive.Count() != start {
		t.Fatal("text construction registered an effect")
	}
	p := NewProbe(Cached(Column(a, b)), Sz(300, 100))
	p.Frame()
	if b.value != "a 3" {
		t.Fatal("Textf did not own its argument list", b.value)
	}
	source.Set("b")
	p.Frame()
	if a.value != "b" || b.value != "b 3" {
		t.Fatal(a.value, b.value)
	}
	a.Content("b")
	b.Content("fixed")
	source.Set("c")
	p.Frame()
	if a.value != "b" || b.value != "fixed" {
		t.Fatal("Content did not detach")
	}
	b.BindContent(source)
	p.Frame()
	if b.value != "c" {
		t.Fatal("BindContent did not replace formatting")
	}
	p.Close()
	if reactive.Count() != start {
		t.Fatal("text leaked computations")
	}
	// Disposing a widget does not dispose a borrowed memo used elsewhere.
	memo := Derived(func() string { return source.Get() + "!" })
	defer memo.Dispose()
	q := NewProbe(TextOf(memo), Sz(100, 40))
	q.Frame()
	q.Close()
	source.Set("d")
	if memo.Get() != "d!" {
		t.Fatal("widget disposed borrowed memo")
	}
}

func TestPropertyBindingsRejectNilReaders(t *testing.T) {
	t.Parallel()
	var width *StateValue[float64]
	var text *StateValue[string]
	var flag *StateValue[bool]
	cases := map[string]func(){
		"width nil":       func() { Box().BindWidth(nil) },
		"width typed nil": func() { Box().BindWidth(width) },
		"height":          func() { Box().BindHeight(width) },
		"content":         func() { TextOf(text) },
		"name":            func() { new(Interactive).BindName(text) },
		"disabled":        func() { new(Interactive).BindInert(flag) },
		"offset":          func() { Scroll(Box()).BindOffset(width) },
		"popup":           func() { Popup(Box(), Box()).BindOpen(flag) },
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				v := recover()
				if v == nil || !strings.Contains(v.(string), "non-nil reader") {
					t.Fatalf("unexpected panic: %v", v)
				}
			}()
			run()
		})
	}
}

func TestBoundDimensionsFollowAnimationWithoutRebuilding(t *testing.T) {
	t.Parallel()
	tween := Tween(10.0, 100*time.Millisecond)
	box := Box().Height(10).BindWidth(tween)
	q, size, _ := measuredPropertyProbe(box)
	defer q.Close()
	q.Advance(0)
	tween.Set(100)
	q.Advance(0)
	q.Advance(200 * time.Millisecond)
	if size.W != 100 {
		t.Fatalf("animation width=%v", size.W)
	}
}

func TestScrollOffsetDetachesAndClampsLocalPosition(t *testing.T) {
	t.Parallel()
	source := State(10.0)
	scroll := Scroll(Box().Size(20, 600)).BindOffset(source)
	p := NewProbe(scroll, Sz(100, 100))
	defer p.Close()
	p.Frame()
	scroll.Offset(200)
	p.Frame()
	source.Set(30)
	p.Frame()
	if scroll.position() != 200 {
		t.Fatal("old scroll source remained attached")
	}
	scroll.scrollTo(250)
	if source.Get() != 30 {
		t.Fatal("local scrolling wrote detached source")
	}
	scroll.BindOffset(source)
	p.Frame()
	scroll.scrollTo(70)
	if source.Get() != 70 {
		t.Fatal("scroll did not write back")
	}
	scroll.Offset(1000)
	p.Frame()
	if scroll.position() != 500 {
		t.Fatal("local scroll not clamped", scroll.position())
	}
}
